package contractprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/mutation"
	"github.com/rootkernel/gul/internal/operation"
	"github.com/rootkernel/gul/internal/session"
)

type predispatchAttempts struct {
	mutation.SubmitAttempts
	calls      int
	err        error
	state      string
	outcome    string
	dispatch   bool
	resolveErr error
}

func (a *predispatchAttempts) BeginMutation(_ context.Context, candidate operation.MutationAttempt) (operation.MutationAttempt, bool, error) {
	a.calls++
	if a.err != nil {
		return operation.MutationAttempt{}, false, a.err
	}
	candidate.State, candidate.OutcomeRef = a.state, a.outcome
	return candidate, a.dispatch, nil
}

type undispatchedProvider struct{ mutation.SubmitProvider }

func TestTextDispatcherSeparatesPredispatchRejectionsFromUnknown(t *testing.T) {
	for _, q := range []struct {
		name     string
		gate     bool
		err      error
		state    string
		want     error
		attempts int
	}{
		{"gate closed", false, nil, "", action.ErrBlocked, 0},
		{"identity conflict", true, operation.ErrConflict, "", action.ErrInvalid, 1},
		{"persistence unavailable", true, errors.New("storage unavailable"), "", action.ErrPersistence, 1},
		{"durable unknown", true, nil, "outcome_unknown", mutation.ErrUnknown, 1},
	} {
		t.Run(q.name, func(t *testing.T) {
			attempts := &predispatchAttempts{err: q.err, state: q.state}
			service := &mutation.SubmitService{Attempts: attempts, Provider: undispatchedProvider{}, Gate: func(context.Context, string, string) bool { return q.gate }}
			d := TextDispatcher{Service: service}
			err := d.Submit(t.Context(), action.Bound{Binding: session.Binding{SubjectID: "owner", RunID: "run", ControllerBindingID: "binding"}, Carrier: session.Carrier{ControllerID: "controller", Generation: 1}}, action.Input{}, "attempt", "prompt", action.IntentRead)
			if !errors.Is(err, q.want) || attempts.calls != q.attempts {
				t.Fatalf("classification=%v, attempts=%d", err, attempts.calls)
			}
		})
	}
}

func (a *predispatchAttempts) ResolveMutation(context.Context, string, string) error {
	return a.resolveErr
}

func TestTextDispatcherRejectsPersistedNoEffectAndClassifiesGateFlipStorageFailure(t *testing.T) {
	for _, q := range []struct {
		name       string
		dispatch   bool
		outcome    string
		resolveErr error
		want       error
	}{
		{"durable rejection retry", false, "rejected", nil, action.ErrBlocked},
		{"gate flip rejection", true, "", nil, action.ErrBlocked},
		{"gate flip persistence failure", true, "", errors.New("storage unavailable"), action.ErrPersistence},
	} {
		t.Run(q.name, func(t *testing.T) {
			state := "resolved"
			if q.dispatch {
				state = "pending"
			}
			attempts := &predispatchAttempts{state: state, outcome: q.outcome, dispatch: q.dispatch, resolveErr: q.resolveErr}
			gates := 0
			service := &mutation.SubmitService{Attempts: attempts, Provider: undispatchedProvider{}, Gate: func(context.Context, string, string) bool { gates++; return gates != 2 }}
			d := TextDispatcher{Service: service}
			err := d.Submit(t.Context(), action.Bound{Binding: session.Binding{SubjectID: "owner", RunID: "run", ControllerBindingID: "binding"}, Carrier: session.Carrier{ControllerID: "controller", Generation: 1}}, action.Input{}, "attempt", "prompt", action.IntentRead)
			if !errors.Is(err, q.want) || attempts.calls != 1 {
				t.Fatalf("classification=%v, attempts=%d", err, attempts.calls)
			}
		})
	}
}
