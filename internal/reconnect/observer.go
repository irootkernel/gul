package reconnect

import (
	"context"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
)

// ManagerObserver maps recovered bindings into the shared live/polling observer.
// Its caller continues steady-state Update calls after recovery.
type ManagerObserver struct {
	Observer   *interaction.Observer
	ProviderID string
	Lifetime   context.Context
}

func (o ManagerObserver) Resume(ctx context.Context, ready []ReadyRun) (map[string]string, error) {
	if o.Observer == nil || o.Observer.Live == nil || o.Observer.Poll == nil || o.ProviderID == "" || o.Lifetime == nil || o.Lifetime.Err() != nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	watch := make([]observation.WatchedRun, 0, len(ready))
	for _, run := range ready {
		b, input := run.Bound, run.Input
		watch = append(watch, observation.WatchedRun{
			Candidate: observation.Candidate{RunID: b.Binding.RunID, PendingInteraction: input.Run.Pending > 0,
				Recovery: input.Run.Recovery != action.NoRecovery, ActiveTurn: input.Run.ActiveTurn == action.Present},
			Binding: observation.Binding{SubjectID: b.Binding.SubjectID, SessionID: b.Binding.ID,
				ProviderID: o.ProviderID, RunID: b.Binding.RunID, WorkspaceID: b.Workspace.ProviderID,
				AbsoluteRoot: b.Workspace.CanonicalRoot},
		})
	}
	modes, err := o.Observer.Update(o.Lifetime, time.Now(), watch)
	if err != nil {
		return nil, err
	}
	// Live admission starts streams asynchronously. Require a real Watch before
	// opening the gate; polling Runs use the independently scheduled unary worker.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending := false
		result := make(map[string]string, len(modes))
		for _, run := range ready {
			id := run.Bound.Binding.RunID
			if modes[id] == observation.WindowPolling {
				result[id] = "polling"
				continue
			}
			if modes[id] != observation.WindowLive {
				result[id] = "stale"
				continue
			}
			state, ok := o.Observer.Live.State(id)
			if ok && state.Connection == observation.ConnectionTerminal {
				result[id] = "terminal"
				continue
			}
			if !ok || state.Connection == observation.ConnectionDisconnected || state.Connection == observation.ConnectionRestarting || state.Connection == observation.ConnectionSlowConsumer {
				result[id] = "stale"
				continue
			}
			if state.Connection == observation.ConnectionConnected {
				result[id] = "connected"
				continue
			}
			pending = true
			result[id] = "stale"
		}
		if !pending {
			return result, nil
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return result, nil
			}
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
