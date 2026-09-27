package interaction

import (
	"context"
	"sync"
	"time"

	"github.com/rootkernel/gul/internal/observation"
)

const PollInterval = 5 * time.Second
const PollTimeout = 5 * time.Second

type PollTarget struct{ SubjectID, SessionID string }
type PollState struct {
	Stamp       observation.Stamp
	Invalidated bool
}
type pollWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Poller runs independent bounded observer reads for each non-live session.
// Slow Runs do not starve other Runs, and no Controller carrier is requested.
// A successful read completes within ten seconds of the prior discovery cycle.
type Poller struct {
	Service *Service
	Notify  Notifier
	mu      sync.Mutex
	workers map[PollTarget]pollWorker
}

func (p *Poller) Set(ctx context.Context, targets []PollTarget) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Service == nil || p.Notify == nil {
		return ErrInvalid
	}
	desired := map[PollTarget]bool{}
	for _, target := range targets {
		if target.SubjectID == "" || target.SessionID == "" {
			return ErrInvalid
		}
		desired[target] = true
	}
	if p.workers == nil {
		p.workers = map[PollTarget]pollWorker{}
	}
	for target, worker := range p.workers {
		if !desired[target] {
			worker.cancel()
			<-worker.done
			delete(p.workers, target)
		}
	}
	for target := range desired {
		if _, ok := p.workers[target]; ok {
			continue
		}
		workerCtx, cancel := context.WithCancel(ctx)
		worker := pollWorker{cancel: cancel, done: make(chan struct{})}
		p.workers[target] = worker
		go p.run(workerCtx, target, worker.done)
	}
	return nil
}
func (p *Poller) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range p.workers {
		w.cancel()
	}
	for key, w := range p.workers {
		<-w.done
		delete(p.workers, key)
	}
}
func (p *Poller) run(ctx context.Context, target PollTarget, done chan struct{}) {
	defer close(done)
	timer := time.NewTicker(PollInterval)
	defer timer.Stop()
	var previous PollState
	for {
		previous, _ = p.Check(ctx, target, previous)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

// Check leaves the last successful stamp untouched on read or publication failure.
func (p *Poller) Check(ctx context.Context, target PollTarget, previous PollState) (PollState, error) {
	ctx, cancel := context.WithTimeout(ctx, PollTimeout)
	defer cancel()
	failed := PollState{Stamp: previous.Stamp, Invalidated: true}
	b, err := p.Service.bound(ctx, target.SubjectID, target.SessionID, false)
	if err != nil {
		return failed, err
	}
	readCtx, readCancel := context.WithTimeout(ctx, PollTimeout-time.Second)
	state, err := p.Service.Provider.Observe(readCtx, b)
	readCancel()
	if err != nil {
		// Failed or incompatible snapshots still wake clients so a blocker is
		// visible. An empty stamp is an invalidation only, never fresh state.
		if notifyErr := p.Notify.InteractionChanged(ctx, b, observation.Stamp{}); notifyErr != nil {
			return failed, notifyErr
		}
		return failed, err
	}
	if !state.Stamp.Valid() || previous.Stamp.Valid() && !state.Stamp.Covers(previous.Stamp) {
		if err := p.Notify.InteractionChanged(ctx, b, observation.Stamp{}); err != nil {
			return failed, err
		}
		return failed, ErrBlocked
	}
	if state.Stamp != previous.Stamp || previous.Invalidated {
		if err := p.Notify.InteractionChanged(ctx, b, state.Stamp); err != nil {
			return failed, err
		}
	}
	return PollState{Stamp: state.Stamp}, nil
}

// Observer connects live-window admission to non-live unary discovery. It does
// not own E5 reconnect or mutation convergence.
type Observer struct {
	Live *observation.Manager
	Poll *Poller
}

func (o *Observer) Update(ctx context.Context, now time.Time, runs []observation.WatchedRun) (map[string]string, error) {
	if o == nil || o.Live == nil || o.Poll == nil {
		return nil, ErrInvalid
	}
	modes, err := o.Live.Update(ctx, now, runs)
	if err != nil {
		return nil, err
	}
	var targets []PollTarget
	for _, run := range runs {
		if modes[run.RunID] != "live" {
			targets = append(targets, PollTarget{SubjectID: run.Binding.SubjectID, SessionID: run.Binding.SessionID})
		}
	}
	if err = o.Poll.Set(ctx, targets); err != nil {
		return nil, err
	}
	return modes, nil
}
func (o *Observer) Close() { o.Poll.Close(); o.Live.Stop() }
