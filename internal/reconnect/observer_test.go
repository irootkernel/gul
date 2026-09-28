package reconnect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type observationRepo struct{}

func (observationRepo) Checkpoint(context.Context, observation.Binding) (observation.Checkpoint, error) {
	return observation.Checkpoint{Committed: "0", Validated: "0"}, nil
}
func (observationRepo) Validate(context.Context, observation.Binding, observation.Cursor) error {
	return nil
}
func (observationRepo) Commit(context.Context, observation.Invalidation) (observation.Notification, error) {
	return observation.Notification{}, nil
}

type checkpointFaultRepo struct{ observationRepo }

func (checkpointFaultRepo) Checkpoint(ctx context.Context, b observation.Binding) (observation.Checkpoint, error) {
	if b.RunID == "bad" {
		return observation.Checkpoint{}, errors.New("checkpoint unavailable")
	}
	return observationRepo{}.Checkpoint(ctx, b)
}

type observationProvider struct {
	mu         sync.Mutex
	bindings   map[string]observation.Binding
	fail       bool
	failedRun  string
	watchBlock <-chan struct{}
}

func (p *observationProvider) Watch(ctx context.Context, b observation.Binding, _ observation.Cursor) (observation.Stream, error) {
	p.mu.Lock()
	if p.bindings == nil {
		p.bindings = map[string]observation.Binding{}
	}
	p.bindings[b.RunID] = b
	fail := p.fail || p.failedRun == b.RunID
	block := p.watchBlock
	p.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if fail {
		return nil, errors.New("watch unavailable")
	}
	return waitingStream{ctx}, nil
}

type waitingStream struct{ ctx context.Context }

func (s waitingStream) Receive() (observation.Envelope, error) {
	<-s.ctx.Done()
	return observation.Envelope{}, s.ctx.Err()
}
func (waitingStream) Close() error { return nil }

type observationRefresh struct{}

func (observationRefresh) Refresh(context.Context, observation.Binding, observation.Refresh, observation.Stamp) error {
	return nil
}

type pollingRecorder struct{ targets []interaction.PollTarget }

func (p *pollingRecorder) Set(_ context.Context, targets []interaction.PollTarget) error {
	p.targets = targets
	return nil
}
func (*pollingRecorder) Close() {}

func observerReady(id string) ReadyRun {
	return ReadyRun{Bound: action.Bound{
		Binding:   session.Binding{SubjectID: "owner", ID: "session-" + id, RunID: id},
		Workspace: workspace.Attachment{ProviderID: "workspace", CanonicalRoot: "/workspace"},
	}, Input: action.Input{Run: action.RunFacts{Recovery: action.NoRecovery, ActiveTurn: action.Missing}}}
}

func TestManagerObserverAdmitsLiveAndSchedulesPolling(t *testing.T) {
	lifetime, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := &observationProvider{}
	manager := observation.NewManager(observationRepo{}, provider, observationRefresh{})
	t.Cleanup(manager.Stop)
	poller := &pollingRecorder{}
	adapter := ManagerObserver{Observer: &interaction.Observer{Live: manager, Poll: poller}, ProviderID: "checked-provider", Lifetime: lifetime}
	var runs []ReadyRun
	for i := range observation.LiveLimit + 1 {
		runs = append(runs, observerReady(fmt.Sprintf("%02d", i)))
	}
	runs[len(runs)-1].Input.Run.Pending = 1
	modes, err := adapter.Resume(t.Context(), runs)
	if err != nil || len(poller.targets) != 1 || modes["08"] != "connected" {
		t.Fatal(modes, poller.targets, err)
	}
	for _, run := range runs {
		mode := modes[run.Bound.Binding.RunID]
		if mode != "connected" && mode != "polling" {
			t.Fatal(run.Bound.Binding.RunID, mode)
		}
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.bindings) != observation.LiveLimit {
		t.Fatal(provider.bindings)
	}
	for _, binding := range provider.bindings {
		if binding.ProviderID != "checked-provider" || binding.WorkspaceID != "workspace" || binding.SubjectID != "owner" {
			t.Fatal(binding)
		}
	}
}

func TestManagerObserverExcludesCheckpointFailureFromPolling(t *testing.T) {
	lifetime, cancel := context.WithCancel(t.Context())
	defer cancel()
	manager := observation.NewManager(checkpointFaultRepo{}, &observationProvider{}, observationRefresh{})
	t.Cleanup(manager.Stop)
	poller := &pollingRecorder{}
	adapter := ManagerObserver{Observer: &interaction.Observer{Live: manager, Poll: poller}, ProviderID: "checked-provider", Lifetime: lifetime}
	runs := []ReadyRun{observerReady("bad")}
	for i := range observation.LiveLimit + 1 {
		runs = append(runs, observerReady(fmt.Sprintf("%02d", i)))
	}
	modes, err := adapter.Resume(t.Context(), runs)
	if err != nil || modes["bad"] != "stale" || modes["08"] != "polling" || len(poller.targets) != 1 || poller.targets[0].SessionID != "session-08" {
		t.Fatal(modes, poller.targets, err)
	}
}

func TestManagerObserverDeadlineLeavesOnlyConnectingRunStale(t *testing.T) {
	lifetime, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := &observationProvider{watchBlock: make(chan struct{})}
	manager := observation.NewManager(observationRepo{}, provider, observationRefresh{})
	t.Cleanup(manager.Stop)
	adapter := ManagerObserver{Observer: &interaction.Observer{Live: manager, Poll: &pollingRecorder{}}, ProviderID: "checked-provider", Lifetime: lifetime}
	ctx, stop := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer stop()
	modes, err := adapter.Resume(ctx, []ReadyRun{observerReady("run")})
	if err != nil || modes["run"] != "stale" {
		t.Fatal(modes, err)
	}
}

func TestManagerObserverRejectsFailedWatchAndMissingPollingWorker(t *testing.T) {
	lifetime, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := &observationProvider{fail: true}
	manager := observation.NewManager(observationRepo{}, provider, observationRefresh{})
	t.Cleanup(manager.Stop)
	adapter := ManagerObserver{Observer: &interaction.Observer{Live: manager, Poll: &pollingRecorder{}}, ProviderID: "checked-provider", Lifetime: lifetime}
	if modes, err := adapter.Resume(t.Context(), []ReadyRun{observerReady("run")}); err != nil || modes["run"] != "stale" {
		t.Fatal(modes, err)
	}
	provider.mu.Lock()
	provider.fail = false
	provider.mu.Unlock()
	adapter.Observer.Poll = nil
	var runs []ReadyRun
	for i := range observation.LiveLimit + 1 {
		runs = append(runs, observerReady(fmt.Sprintf("%02d", i)))
	}
	if _, err := adapter.Resume(t.Context(), runs); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := (ManagerObserver{}).Resume(t.Context(), runs); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestManagerObserverIsolatesFailedLiveRun(t *testing.T) {
	lifetime, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := &observationProvider{failedRun: "bad"}
	manager := observation.NewManager(observationRepo{}, provider, observationRefresh{})
	t.Cleanup(manager.Stop)
	adapter := ManagerObserver{Observer: &interaction.Observer{Live: manager, Poll: &pollingRecorder{}}, ProviderID: "checked-provider", Lifetime: lifetime}
	modes, err := adapter.Resume(t.Context(), []ReadyRun{observerReady("bad"), observerReady("good")})
	if err != nil || modes["bad"] != "stale" || modes["good"] != "connected" {
		t.Fatal(modes, err)
	}
}
