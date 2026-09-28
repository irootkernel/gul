// Package reconnect coordinates checked reconnect reads without owning a
// provider mutation or treating a broken stream as a failed Run.
package reconnect

import (
	"context"
	"errors"
	"sync"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/recovery"
	"github.com/rootkernel/gul/internal/session"
)

var ErrUnavailable = errors.New("reconnect unavailable")

type Compatibility interface{ Check(context.Context) error }
type SnapshotReader interface {
	Read(context.Context, action.Bound) (action.Input, error)
}
type Invalidator interface {
	MarkProviderUnavailable(context.Context, action.Bound) error
}
type RecoveryNotifier interface {
	MarkProviderRecovered(context.Context, action.Bound) error
}
type Converger interface {
	Converge(context.Context, action.Bound, action.Input) (bool, error)
}
type Refresher interface {
	Refresh(context.Context, action.Bound, action.RunFacts) error
}
type ReadyRun struct {
	Bound action.Bound
	Input action.Input
}
type Observer interface {
	Resume(context.Context, []ReadyRun) (map[string]string, error)
}

type Status struct {
	Connection string
	Blocker    string
	Ready      bool
}

type sessionKey struct{ subject, session string }

// Service holds only a process-local startup gate. The durable invalidation and
// unresolved attempts live in Store, so sleep, wake, and restart use one path.
type Service struct {
	Compatibility Compatibility
	Snapshots     SnapshotReader
	Invalidator   Invalidator
	Notifier      RecoveryNotifier
	Converger     Converger
	Refresher     Refresher
	Observer      Observer
	runMu         sync.Mutex
	mu            sync.RWMutex
	states        map[sessionKey]Status
}

func (s *Service) refreshedInput(ctx context.Context, b action.Bound) (action.Input, string, error) {
	input, err := s.Snapshots.Read(ctx, b)
	if err != nil {
		return action.Input{}, "snapshot", err
	}
	if err := s.Refresher.Refresh(ctx, b, input.Run); err != nil {
		return action.Input{}, "snapshot", err
	}
	// Refresh may publish a newer floor than the first read.
	input, err = s.Snapshots.Read(ctx, b)
	if err != nil {
		return action.Input{}, "snapshot", err
	}
	ready, err := s.Converger.Converge(ctx, b, input)
	if err != nil {
		return action.Input{}, "convergence", err
	}
	if !ready {
		return action.Input{}, "convergence", ErrUnavailable
	}
	return input, "", nil
}

func (s *Service) State(subject, sessionID string) Status {
	if s == nil {
		return Status{Connection: "unavailable", Blocker: "startup"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[sessionKey{subject, sessionID}]
	if !ok {
		return Status{Connection: "unavailable", Blocker: "startup"}
	}
	return state
}

func (s *Service) Ready(subject, sessionID string) bool { return s.State(subject, sessionID).Ready }

func (s *Service) set(subject, id string, state Status) {
	s.mu.Lock()
	if s.states == nil {
		s.states = make(map[sessionKey]Status)
	}
	s.states[sessionKey{subject, id}] = state
	s.mu.Unlock()
}

// Disconnect revokes the process gate before durable invalidation. It is for
// provider-wide loss; a browser-only tailnet interruption uses Browser.Reconnect.
func (s *Service) Disconnect(ctx context.Context, runs []action.Bound) error {
	if s == nil || s.Invalidator == nil {
		return ErrUnavailable
	}
	// Revoke readiness immediately, even while another recovery holds runMu.
	for _, b := range runs {
		s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: "disconnected", Blocker: "provider"})
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	var failures []error
	for _, b := range runs {
		s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: "disconnected", Blocker: "provider"})
		if err := s.Invalidator.MarkProviderUnavailable(ctx, b); err != nil {
			s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: "disconnected", Blocker: "persistence"})
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// Recover is called at startup and after provider transport becomes available.
// All local authority is revoked before the compatibility probe. Each Run then
// converges independently; one broken read does not hide a sibling's blocker.
func (s *Service) Recover(ctx context.Context, runs []action.Bound) error {
	if s == nil || s.Compatibility == nil || s.Snapshots == nil || s.Invalidator == nil || s.Notifier == nil || s.Converger == nil || s.Refresher == nil || s.Observer == nil {
		return ErrUnavailable
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	seen := make(map[sessionKey]bool, len(runs))
	var observable []ReadyRun
	for _, b := range runs {
		id := b.Binding.ID
		key := sessionKey{b.Binding.SubjectID, id}
		if id == "" || b.Binding.SubjectID == "" || b.Binding.RunID == "" || seen[key] {
			return ErrUnavailable
		}
		seen[key] = true
		s.set(b.Binding.SubjectID, id, Status{Connection: "restarting", Blocker: "startup"})
	}
	var failures []error
	for _, b := range runs {
		if err := s.Invalidator.MarkProviderUnavailable(ctx, b); err != nil {
			s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: "disconnected", Blocker: "persistence"})
			failures = append(failures, err)
		}
	}
	if len(failures) != 0 {
		return errors.Join(failures...)
	}
	if err := s.Compatibility.Check(ctx); err != nil {
		connection, blocker := "disconnected", "provider"
		if errors.Is(err, recovery.ErrIncompatible) {
			connection, blocker = "incompatible", "compatibility"
		}
		for _, b := range runs {
			s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: connection, Blocker: blocker})
		}
		return err
	}
	for _, b := range runs {
		input, blocker, err := s.refreshedInput(ctx, b)
		if err != nil {
			s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: "stale", Blocker: blocker})
			failures = append(failures, err)
			continue
		}
		if input.Run.Lifecycle == action.Closed || input.Run.Lifecycle == action.StartFailed {
			if err := s.Notifier.MarkProviderRecovered(ctx, b); err != nil {
				s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: "stale", Blocker: "persistence"})
				failures = append(failures, err)
				continue
			}
			s.set(b.Binding.SubjectID, b.Binding.ID, Status{Connection: "terminal", Blocker: "run_terminal", Ready: true})
			continue
		}
		observable = append(observable, ReadyRun{Bound: b, Input: input})
	}
	if len(observable) != 0 {
		if modes, err := s.Observer.Resume(ctx, observable); err != nil {
			for _, run := range observable {
				s.set(run.Bound.Binding.SubjectID, run.Bound.Binding.ID, Status{Connection: "stale", Blocker: "observation"})
			}
			failures = append(failures, err)
		} else {
			for _, run := range observable {
				mode := modes[run.Bound.Binding.RunID]
				if mode == "terminal" {
					input, blocker, err := s.refreshedInput(ctx, run.Bound)
					if err != nil || input.Run.Lifecycle != action.Closed && input.Run.Lifecycle != action.StartFailed {
						if err == nil {
							blocker, err = "observation", ErrUnavailable
						}
						s.set(run.Bound.Binding.SubjectID, run.Bound.Binding.ID, Status{Connection: "stale", Blocker: blocker})
						failures = append(failures, err)
						continue
					}
					if err := s.Notifier.MarkProviderRecovered(ctx, run.Bound); err != nil {
						s.set(run.Bound.Binding.SubjectID, run.Bound.Binding.ID, Status{Connection: "stale", Blocker: "persistence"})
						failures = append(failures, err)
						continue
					}
					s.set(run.Bound.Binding.SubjectID, run.Bound.Binding.ID, Status{Connection: "terminal", Blocker: "run_terminal", Ready: true})
					continue
				}
				if mode != "connected" && mode != "polling" {
					s.set(run.Bound.Binding.SubjectID, run.Bound.Binding.ID, Status{Connection: "stale", Blocker: "observation"})
					failures = append(failures, ErrUnavailable)
					continue
				}
				if err := s.Notifier.MarkProviderRecovered(ctx, run.Bound); err != nil {
					s.set(run.Bound.Binding.SubjectID, run.Bound.Binding.ID, Status{Connection: "stale", Blocker: "persistence"})
					failures = append(failures, err)
					continue
				}
				s.set(run.Bound.Binding.SubjectID, run.Bound.Binding.ID, Status{Connection: mode, Ready: true})
			}
		}
	}
	return errors.Join(failures...)
}

type PresentationReader interface {
	DirectSession(context.Context, string, string) (presentation.DirectSession, error)
}
type SessionReader interface {
	GetExecutionState(context.Context, string, string) (session.ExecutionState, error)
}

type Browser struct {
	Presentation PresentationReader
	Sessions     SessionReader
	Events       observation.DeliveryReader
}
type BrowserState struct {
	Presentation presentation.DirectSession
	Execution    session.ExecutionState
	Delivery     observation.DeliveryBatch
}

// Reconnect reconstructs presentation and provider projections before replaying
// Gul delivery records. Replay contains invalidations only, never RPC intents.
func (b Browser) Reconnect(ctx context.Context, subject, sessionID string, after observation.Sequence) (BrowserState, error) {
	if b.Presentation == nil || b.Sessions == nil || b.Events == nil || subject == "" || sessionID == "" || after < 0 {
		return BrowserState{}, ErrUnavailable
	}
	p, err := b.Presentation.DirectSession(ctx, subject, sessionID)
	if err != nil {
		return BrowserState{}, err
	}
	state, err := b.Sessions.GetExecutionState(ctx, subject, sessionID)
	if err != nil {
		return BrowserState{}, err
	}
	if p.SubjectID != subject || p.SessionID != sessionID || state.SessionID != sessionID {
		return BrowserState{}, ErrUnavailable
	}
	delivery, err := b.Events.ReadDelivery(ctx, subject, sessionID, after)
	if err != nil {
		return BrowserState{}, err
	}
	if after == 0 || state.Freshness != "fresh" {
		delivery.Events = nil
		delivery.SnapshotRequired = true
	}
	return BrowserState{Presentation: p, Execution: state, Delivery: delivery}, nil
}
