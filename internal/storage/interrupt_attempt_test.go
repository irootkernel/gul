package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/interrupt"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type interruptActions struct {
	bound         action.Bound
	input         action.Input
	calls         int
	fail          error
	timeline      history.Timeline
	reads         int
	providerReads int
}

func (f *interruptActions) Binding(_ context.Context, subject, id string) (session.Binding, error) {
	if subject != f.bound.Binding.SubjectID || id != f.bound.Binding.ID {
		return session.Binding{}, action.ErrAuthority
	}
	return f.bound.Binding, nil
}
func (f *interruptActions) Converge(context.Context, action.Bound, action.Input) (bool, error) {
	return true, nil
}
func (f *interruptActions) LocalState(context.Context, action.Bound) (action.LocalState, error) {
	return action.LocalState{Ownership: action.OwnedSession, Operation: action.NoOperation}, nil
}
func (f *interruptActions) Revalidate(context.Context, string, string) (workspace.Attachment, error) {
	return f.bound.Workspace, nil
}
func (f *interruptActions) Resolve(context.Context, string, string) (session.Carrier, error) {
	return f.bound.Carrier, nil
}
func (f *interruptActions) Read(context.Context, action.Bound) (action.Input, error) {
	f.providerReads++
	return f.input, nil
}
func (f *interruptActions) Acquire(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	panic("unexpected writer mutation")
}
func (f *interruptActions) Release(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	panic("unexpected writer mutation")
}
func (f *interruptActions) Interrupt(_ context.Context, b action.Bound, revision uint64) error {
	f.calls++
	if b.Binding.RunID != f.bound.Binding.RunID || revision != f.input.Run.Stamp.Run {
		return action.ErrAuthority
	}
	return f.fail
}
func (f *interruptActions) Timeline(context.Context, history.Bound, string, uint32) (history.Timeline, error) {
	f.reads++
	return f.timeline, nil
}
func interruptService(s *Store, f *interruptActions) *interrupt.Service {
	return &interrupt.Service{Actions: &action.Service{Repository: f, Workspaces: f, Carriers: f, Provider: f, Gate: func(string, string) bool { return true }}, Repository: s.InterruptAttempts(), Provider: f, Timeline: f}
}
func busyInterrupt(t *testing.T, s *Store) *interruptActions {
	in := recoveryInput()
	in.Request.CloseIntent = action.NoCloseIntent
	in.Run.Lifecycle = action.Running
	in.Run.ActiveTurn = action.Present
	in.Run.ActiveTurnID = "captured-turn"
	b := closeBound(t, s)
	b.Carrier.AbsolutePath, b.Carrier.Generation = "/private/carrier", 1
	return &interruptActions{bound: b, input: in}
}
func TestPrimaryInterruptUnknownSurvivesRestartWithoutReplay(t *testing.T) {
	s, filename := openTestStore(t)
	f := busyInterrupt(t, s)
	f.fail = errors.New("lost provider receipt")
	service := interruptService(s, f)
	outcome, _, err := service.Send(t.Context(), "owner", "session", "request", false)
	if err != nil || outcome != interrupt.Rejected || f.calls != 0 {
		t.Fatalf("missing consent: %v %v calls%d", outcome, err, f.calls)
	}
	outcome, _, err = service.Send(t.Context(), "owner", "session", "request", true)
	if err != nil || outcome != interrupt.Unknown || f.calls != 1 {
		t.Fatalf("lost receipt: %v %v calls%d", outcome, err, f.calls)
	}
	a, found, err := s.InterruptAttempts().Find(t.Context(), f.bound, "request")
	if err != nil || !found || !a.DispatchFinished || a.State != "outcome_unknown" || a.TurnID != "captured-turn" {
		t.Fatalf("durable attempt: %+v %t %v", a, found, err)
	}
	raw, err := s.Attempts().Mutation(t.Context(), a.ID)
	if err != nil || raw.ReplayAvailable || raw.ReplayKey != "" {
		t.Fatalf("tokenless replay: %+v %v", raw, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	service = interruptService(s, f)
	for _, request := range []string{"request", "another-request"} {
		outcome, _, err = service.Send(t.Context(), "owner", "session", request, true)
		if err != nil || outcome != interrupt.Unknown || f.calls != 1 {
			t.Fatalf("reopened %s replayed: %v %v calls%d", request, outcome, err, f.calls)
		}
	}
	f.input.Run.ActiveTurn = action.Missing
	f.input.Run.ActiveTurnID = ""
	f.input.Run.Lifecycle = action.Idle
	f.input.Run.Stamp.Run = 5
	f.input.Run.Stamp.Head = "5"
	f.input.InteractionStamp = f.input.Run.Stamp
	f.input.TimelineHead = "5"
	f.timeline = history.Timeline{Stamp: f.input.Run.Stamp, Head: "5", Items: []history.SourceEntry{{Kind: history.TurnTerminal, TurnID: "other-turn", Status: "interrupted"}}}
	if err := service.ObservePending(t.Context(), "owner", "session"); err != nil {
		t.Fatal(err)
	}
	a, _, err = s.InterruptAttempts().Find(t.Context(), f.bound, "request")
	if err != nil || a.State != "outcome_unknown" {
		t.Fatalf("another Turn settled request: %+v %v", a, err)
	}
	f.timeline.Items[0].TurnID = "captured-turn"
	f.timeline.Items[0].Status = "outcome_unknown"
	if err := service.ObservePending(t.Context(), "owner", "session"); err != nil {
		t.Fatal(err)
	}
	a, _, err = s.InterruptAttempts().Find(t.Context(), f.bound, "request")
	if err != nil || a.State != "outcome_unknown" {
		t.Fatalf("unknown terminal settled request: %+v %v", a, err)
	}
	f.timeline.Items[0].Status = "interrupted"
	f.timeline.Stamp.Run = 4
	if err := service.ObservePending(t.Context(), "owner", "session"); err != nil {
		t.Fatal(err)
	}
	a, _, err = s.InterruptAttempts().Find(t.Context(), f.bound, "request")
	if err != nil || a.State != "outcome_unknown" {
		t.Fatalf("stale timeline settled request: %+v %v", a, err)
	}
	f.timeline.Stamp = f.input.Run.Stamp
	if err := service.ObservePending(t.Context(), "owner", "session"); err != nil {
		t.Fatal(err)
	}
	outcome, _, err = service.Send(t.Context(), "owner", "session", "request", true)
	if err != nil || outcome != interrupt.StateObserved || f.calls != 1 {
		t.Fatalf("observed state replayed/claimed acceptance: %v %v calls%d", outcome, err, f.calls)
	}
}
func TestPrimaryInterruptAcceptedReceiptAndPendingCannotReplay(t *testing.T) {
	s, _ := openTestStore(t)
	f := busyInterrupt(t, s)
	service := interruptService(s, f)
	for range 2 {
		outcome, _, err := service.Send(t.Context(), "owner", "session", "request", true)
		if err != nil || outcome != interrupt.Accepted || f.calls != 1 {
			t.Fatalf("accepted request replay: %v %v calls%d", outcome, err, f.calls)
		}
	}
	before := f.providerReads
	if err := service.ObservePending(t.Context(), "owner", "session"); err != nil {
		t.Fatal(err)
	}
	if f.providerReads != before {
		t.Fatal("no pending Interrupt triggered provider snapshot reads")
	}
	pending, dispatch, err := s.InterruptAttempts().Begin(t.Context(), f.bound, "pending", "captured-turn", f.input.Run.Stamp)
	if err != nil || !dispatch {
		t.Fatalf("begin pending: %+v %t %v", pending, dispatch, err)
	}
	if err := s.InterruptAttempts().Finish(t.Context(), pending.ID, "state_observed"); !errors.Is(err, ErrMutationConflict) {
		t.Fatalf("in-flight settlement: %v", err)
	}
	outcome, _, err := service.Send(t.Context(), "owner", "session", "pending", true)
	if err != nil || outcome != interrupt.Unknown || f.calls != 1 {
		t.Fatalf("pending request replayed: %v %v calls%d", outcome, err, f.calls)
	}
	if err := service.ObservePending(t.Context(), "owner", "session"); err != nil {
		t.Fatal(err)
	}
	if f.reads != 0 {
		t.Fatal("in-flight request was reconciled")
	}
}
func TestPrimaryInterruptFreshAuthorityAndEligibility(t *testing.T) {
	for name, change := range map[string]func(*interruptActions){
		"stale":              func(f *interruptActions) { f.input.Freshness = action.Stale },
		"foreign Controller": func(f *interruptActions) { f.input.ControllerMatches = false },
		"missing Turn":       func(f *interruptActions) { f.input.Run.ActiveTurn = action.Missing; f.input.Run.ActiveTurnID = "" },
		"recovery":           func(f *interruptActions) { f.input.Run.Recovery = action.RecoveryRequired },
		"capability":         func(f *interruptActions) { f.input.Capabilities.Interrupt = false },
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := openTestStore(t)
			f := busyInterrupt(t, s)
			change(f)
			outcome, _, err := interruptService(s, f).Send(t.Context(), "owner", "session", "request", true)
			if err != nil || outcome != interrupt.Rejected || f.calls != 0 {
				t.Fatalf("admitted %s: %v %v calls%d", name, outcome, err, f.calls)
			}
		})
	}
}
