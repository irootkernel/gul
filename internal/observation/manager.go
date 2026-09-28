package observation

import (
	"context"
	"sync"
	"time"
)

type WatchedRun struct {
	Candidate
	Binding Binding
}
type managedStream struct {
	cancel context.CancelFunc
	done   chan struct{}
	bridge *Bridge
}

// Manager is the sole stream admission owner for one reusable Provider. Its
// caller supplies trusted bindings; Repository rechecks each persisted binding.
// Unary clients and browser delivery have no dependency on this control lock.
type Manager struct {
	mu       sync.Mutex
	window   Window
	repo     Repository
	provider Provider
	refresh  Refresher
	streams  map[string]managedStream
	bridges  map[string]*Bridge
}

func NewManager(repo Repository, provider Provider, refresh Refresher) *Manager {
	return &Manager{repo: repo, provider: provider, refresh: refresh, streams: make(map[string]managedStream), bridges: make(map[string]*Bridge)}
}

func (m *Manager) Update(ctx context.Context, now time.Time, runs []WatchedRun) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.repo == nil || m.provider == nil || m.refresh == nil {
		return nil, ErrUnbound
	}
	candidates := make([]Candidate, 0, len(runs))
	bindings := make(map[string]Binding)
	failed := make(map[string]bool)
	for _, r := range runs {
		if !r.Binding.Valid() || r.Candidate.RunID != r.Binding.RunID {
			return nil, ErrUnbound
		}
		if _, duplicate := bindings[r.Candidate.RunID]; duplicate {
			return nil, ErrInvalid
		}
		bindings[r.Candidate.RunID] = r.Binding
		if _, err := m.repo.Checkpoint(ctx, r.Binding); err != nil {
			failed[r.Candidate.RunID] = true
			continue
		}
		candidates = append(candidates, r.Candidate)
	}
	modes := m.window.Select(now, candidates)
	for id := range failed {
		modes[id] = WindowStale
	}
	for id, s := range m.streams {
		select {
		case <-s.done:
			delete(m.streams, id)
		default:
		}
	}
	for id, s := range m.streams {
		if modes[id] != WindowLive || s.bridge.binding != bindings[id] {
			s.cancel()
			select {
			case <-s.done:
				delete(m.streams, id)
				s.bridge.update(func(state *SubscriptionState) {
					if state.Connection != ConnectionTerminal {
						state.Connection = WindowPolling
					}
				})
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	for id, bridge := range m.bridges {
		if binding, ok := bindings[id]; !ok || binding != bridge.binding {
			delete(m.bridges, id)
		}
	}
	for id, mode := range modes {
		if mode != WindowLive {
			continue
		}
		if _, exists := m.streams[id]; exists {
			continue
		}
		// Retain bridges across subscriptions so Generation tracks reconnects.
		b := m.bridges[id]
		if b != nil && b.State().Connection == ConnectionTerminal {
			continue
		}
		if b == nil {
			var err error
			b, err = NewBridge(m.repo, m.provider, m.refresh, bindings[id])
			if err != nil {
				return nil, err
			}
			m.bridges[id] = b
		}
		streamCtx, cancel := context.WithCancel(ctx)
		s := managedStream{cancel: cancel, done: make(chan struct{}), bridge: b}
		m.streams[id] = s
		go func() { defer close(s.done); _ = b.Run(streamCtx) }()
	}
	return modes, nil
}
func (m *Manager) State(runID string) (SubscriptionState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bridges[runID]
	if !ok {
		return SubscriptionState{}, false
	}
	return b.State(), true
}
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.streams {
		s.cancel()
	}
	for id, s := range m.streams {
		<-s.done
		delete(m.streams, id)
	}
}
