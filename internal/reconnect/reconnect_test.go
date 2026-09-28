package reconnect

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/recovery"
	"github.com/rootkernel/gul/internal/session"
)

type probeFake struct {
	steps *[]string
	err   error
}

func (p probeFake) Check(context.Context) error { *p.steps = append(*p.steps, "probe"); return p.err }

type readerFake struct {
	steps *[]string
	input action.Input
	err   error
}

func (r readerFake) Read(context.Context, action.Bound) (action.Input, error) {
	*r.steps = append(*r.steps, "read")
	return r.input, r.err
}

type invalidatorFake struct{ steps *[]string }

func (i invalidatorFake) MarkProviderUnavailable(context.Context, action.Bound) error {
	*i.steps = append(*i.steps, "stale")
	return nil
}

type failedInvalidator struct{ steps *[]string }

func (i failedInvalidator) MarkProviderUnavailable(context.Context, action.Bound) error {
	*i.steps = append(*i.steps, "stale")
	return errors.New("journal fault")
}

type recoveryNotifierFake struct{ err error }

func (n recoveryNotifierFake) MarkProviderRecovered(context.Context, action.Bound) error {
	return n.err
}

type convergerFake struct {
	steps *[]string
	ready bool
}

func (c convergerFake) Converge(context.Context, action.Bound, action.Input) (bool, error) {
	*c.steps = append(*c.steps, "converge")
	return c.ready, nil
}

type refresherFake struct {
	steps *[]string
	err   error
}

func (r refresherFake) Refresh(context.Context, action.Bound, action.RunFacts) error {
	*r.steps = append(*r.steps, "refresh")
	return r.err
}

type observerFake struct {
	steps *[]string
	err   error
	modes map[string]string
}

func (o observerFake) Resume(_ context.Context, runs []ReadyRun) (map[string]string, error) {
	*o.steps = append(*o.steps, "observe")
	modes := make(map[string]string, len(runs))
	for _, run := range runs {
		mode := o.modes[run.Bound.Binding.RunID]
		if o.modes == nil {
			mode = "connected"
		}
		modes[run.Bound.Binding.RunID] = mode
	}
	return modes, o.err
}

func TestRecoveryStatusMatchesObservationMode(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want Status
	}{
		{"polling", Status{Connection: "polling", Ready: true}},
		{"", Status{Connection: "stale", Blocker: "observation"}},
	} {
		var steps []string
		s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: readerFake{steps: &steps},
			Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{}, Refresher: refresherFake{steps: &steps},
			Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps, modes: map[string]string{"run": tc.mode}}}
		err := s.Recover(t.Context(), []action.Bound{reconnectBound()})
		if (err == nil) != (tc.mode == "polling") || s.State("owner", "session") != tc.want {
			t.Fatal(tc.mode, err, s.State("owner", "session"))
		}
	}
}

func TestObservationFailureDoesNotBlockHealthySibling(t *testing.T) {
	var steps []string
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: readerFake{steps: &steps},
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{},
		Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true},
		Observer: observerFake{steps: &steps, modes: map[string]string{"run": "stale", "other": "connected"}}}
	other := reconnectBound()
	other.Binding.ID, other.Binding.RunID = "other-session", "other"
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound(), other}); err == nil || s.Ready("owner", "session") || !s.Ready("owner", "other-session") {
		t.Fatal(err, s.State("owner", "session"), s.State("owner", "other-session"))
	}
}

func TestRecoveryGateSeparatesSubjectsWithSameSessionID(t *testing.T) {
	var steps []string
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: readerFake{steps: &steps},
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{}, Refresher: refresherFake{steps: &steps},
		Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err != nil {
		t.Fatal(err)
	}
	if !s.Ready("owner", "session") || s.Ready("another", "session") {
		t.Fatal("readiness crossed subject boundary")
	}
}

func TestRecoveryNotificationFailureKeepsGateClosed(t *testing.T) {
	var steps []string
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: readerFake{steps: &steps},
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{err: errors.New("journal fault")},
		Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err == nil || s.Ready("owner", "session") || s.State("owner", "session").Blocker != "persistence" {
		t.Fatal(err, s.State("owner", "session"))
	}
}

func TestTerminalRecoveryNotificationFailureKeepsGateClosed(t *testing.T) {
	var steps []string
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: readerFake{steps: &steps, input: action.Input{Run: action.RunFacts{Lifecycle: action.Closed}}},
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{err: errors.New("journal fault")},
		Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err == nil || s.Ready("owner", "session") || s.State("owner", "session").Blocker != "persistence" {
		t.Fatal(err, s.State("owner", "session"))
	}
}

func reconnectBound() action.Bound {
	return action.Bound{Binding: session.Binding{SubjectID: "owner", ID: "session", RunID: "run"}}
}

func TestStartupGateOrdersRecoveryAndKeepsFailureVisible(t *testing.T) {
	for _, tc := range []struct {
		name      string
		probe     error
		ready     bool
		lifecycle action.Lifecycle
		want      []string
		state     Status
	}{
		{"healthy", nil, true, action.Idle, []string{"stale", "probe", "read", "refresh", "read", "converge", "observe"}, Status{Connection: "connected", Ready: true}},
		{"incompatible", recovery.ErrIncompatible, true, action.Idle, []string{"stale", "probe"}, Status{Connection: "incompatible", Blocker: "compatibility"}},
		{"advancing", nil, false, action.Idle, []string{"stale", "probe", "read", "refresh", "read", "converge"}, Status{Connection: "stale", Blocker: "convergence"}},
		{"terminal", nil, true, action.Closed, []string{"stale", "probe", "read", "refresh", "read", "converge"}, Status{Connection: "terminal", Blocker: "run_terminal", Ready: true}},
		{"start-failed", nil, true, action.StartFailed, []string{"stale", "probe", "read", "refresh", "read", "converge"}, Status{Connection: "terminal", Blocker: "run_terminal", Ready: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var steps []string
			s := &Service{Compatibility: probeFake{&steps, tc.probe}, Snapshots: readerFake{steps: &steps, input: action.Input{Run: action.RunFacts{Lifecycle: tc.lifecycle}}}, Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{}, Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, tc.ready}, Observer: observerFake{steps: &steps}}
			if got := s.State("owner", "session"); got.Ready || got.Blocker != "startup" {
				t.Fatal(got)
			}
			err := s.Recover(t.Context(), []action.Bound{reconnectBound()})
			if (err == nil) != tc.state.Ready {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(steps, tc.want) || s.State("owner", "session") != tc.state {
				t.Fatal(steps, s.State("owner", "session"))
			}
		})
	}
}

type browserPresentation struct{ steps *[]string }

func (p browserPresentation) DirectSession(context.Context, string, string) (presentation.DirectSession, error) {
	*p.steps = append(*p.steps, "presentation")
	return presentation.DirectSession{SubjectID: "owner", SessionID: "session"}, nil
}

type browserSession struct {
	steps     *[]string
	freshness string
}

func (s browserSession) GetExecutionState(context.Context, string, string) (session.ExecutionState, error) {
	*s.steps = append(*s.steps, "provider")
	return session.ExecutionState{SessionID: "session", Freshness: s.freshness}, nil
}

type browserEvents struct{ steps *[]string }

func (e browserEvents) ReadDelivery(_ context.Context, _, _ string, after observation.Sequence) (observation.DeliveryBatch, error) {
	*e.steps = append(*e.steps, "delivery")
	return observation.DeliveryBatch{Events: []observation.Notification{{Sequence: after + 1, Kind: "projection_invalidated"}}, Head: after + 1}, nil
}

type wrongBrowserPresentation struct{ browserPresentation }

func (p wrongBrowserPresentation) DirectSession(ctx context.Context, subject, sessionID string) (presentation.DirectSession, error) {
	state, err := p.browserPresentation.DirectSession(ctx, subject, sessionID)
	state.SubjectID = "another"
	return state, err
}

type wrongBrowserSession struct{ browserSession }

func (s wrongBrowserSession) GetExecutionState(ctx context.Context, subject, sessionID string) (session.ExecutionState, error) {
	state, err := s.browserSession.GetExecutionState(ctx, subject, sessionID)
	state.SessionID = "another"
	return state, err
}

func TestBrowserReconnectRejectsInvalidIdentityBeforeDelivery(t *testing.T) {
	var steps []string
	b := Browser{browserPresentation{&steps}, browserSession{&steps, "fresh"}, browserEvents{&steps}}
	if _, err := b.Reconnect(t.Context(), "owner", "session", -1); !errors.Is(err, ErrUnavailable) || len(steps) != 0 {
		t.Fatal(err, steps)
	}
	b.Presentation = wrongBrowserPresentation{browserPresentation{&steps}}
	if _, err := b.Reconnect(t.Context(), "owner", "session", 1); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(steps, []string{"presentation", "provider"}) {
		t.Fatal(err, steps)
	}
	steps = nil
	b.Presentation = browserPresentation{&steps}
	b.Sessions = wrongBrowserSession{browserSession{&steps, "fresh"}}
	if _, err := b.Reconnect(t.Context(), "owner", "session", 1); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(steps, []string{"presentation", "provider"}) {
		t.Fatal(err, steps)
	}
}
func TestBrowserReconnectReadsFreshStateBeforeDelivery(t *testing.T) {
	for _, tc := range []struct {
		after     observation.Sequence
		freshness string
		snapshot  bool
	}{{5, "fresh", false}, {0, "fresh", true}, {5, "stale", true}} {
		var steps []string
		b := Browser{browserPresentation{&steps}, browserSession{&steps, tc.freshness}, browserEvents{&steps}}
		state, err := b.Reconnect(t.Context(), "owner", "session", tc.after)
		if err != nil || !reflect.DeepEqual(steps, []string{"presentation", "provider", "delivery"}) || state.Delivery.SnapshotRequired != tc.snapshot || len(state.Delivery.Events) != map[bool]int{true: 0, false: 1}[tc.snapshot] {
			t.Fatal(state, steps, err)
		}
	}
}

func TestObservationFailureKeepsStartupBlocked(t *testing.T) {
	var steps []string
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: readerFake{steps: &steps}, Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{}, Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps, err: errors.New("stream lost")}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err == nil || s.State("owner", "session").Ready || s.State("owner", "session").Blocker != "observation" {
		t.Fatal(err, s.State("owner", "session"))
	}
}

func TestSnapshotReadAndRefreshFailuresKeepGateClosed(t *testing.T) {
	for _, tc := range []struct {
		name, failedStep string
	}{
		{"read", "read"}, {"refresh", "refresh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var steps []string
			snapshot := readerFake{steps: &steps}
			refresh := refresherFake{steps: &steps}
			if tc.failedStep == "read" {
				snapshot.err = errors.New("read failed")
			} else {
				refresh.err = errors.New("refresh failed")
			}
			s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: snapshot,
				Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{},
				Refresher: refresh, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
			if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err == nil || s.State("owner", "session") != (Status{Connection: "stale", Blocker: "snapshot"}) {
				t.Fatal(err, s.State("owner", "session"))
			}
			for _, step := range steps {
				if step == "observe" || step == "converge" {
					t.Fatal(steps)
				}
			}
		})
	}
}

func TestInvalidationFailureStopsBeforeProbe(t *testing.T) {
	var steps []string
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: readerFake{steps: &steps},
		Invalidator: failedInvalidator{&steps}, Notifier: recoveryNotifierFake{},
		Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err == nil || s.State("owner", "session") != (Status{Connection: "disconnected", Blocker: "persistence"}) || !reflect.DeepEqual(steps, []string{"stale"}) {
		t.Fatal(err, s.State("owner", "session"), steps)
	}
}

func TestDisconnectPersistenceFailureKeepsGateClosed(t *testing.T) {
	s := &Service{Invalidator: failedInvalidator{steps: new([]string)}}
	if err := s.Disconnect(t.Context(), []action.Bound{reconnectBound()}); err == nil || s.Ready("owner", "session") || s.State("owner", "session") != (Status{Connection: "disconnected", Blocker: "persistence"}) {
		t.Fatal(err, s.State("owner", "session"))
	}
}

type selectiveReader struct {
	steps     *[]string
	failedRun string
}

type advancingReader struct{ reads int }

func (r *advancingReader) Read(context.Context, action.Bound) (action.Input, error) {
	r.reads++
	return action.Input{Run: action.RunFacts{Lifecycle: action.Idle, Stamp: observation.Stamp{Head: observation.Cursor(strconv.Itoa(r.reads + 3)), Run: uint64(r.reads + 3)}}}, nil
}

type secondReadFailure struct{ reads int }

func (r *secondReadFailure) Read(context.Context, action.Bound) (action.Input, error) {
	r.reads++
	if r.reads == 2 {
		return action.Input{}, errors.New("post-refresh read failed")
	}
	return action.Input{Run: action.RunFacts{Lifecycle: action.Idle}}, nil
}

func TestPostRefreshReadFailureKeepsGateClosed(t *testing.T) {
	var steps []string
	reader := &secondReadFailure{}
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: reader,
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{},
		Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err == nil || reader.reads != 2 || s.State("owner", "session").Blocker != "snapshot" || s.Ready("owner", "session") {
		t.Fatal(err, reader.reads, s.State("owner", "session"))
	}
}

type terminalAfterAdmissionReader struct{ reads int }

func (r *terminalAfterAdmissionReader) Read(context.Context, action.Bound) (action.Input, error) {
	r.reads++
	lifecycle := action.Idle
	if r.reads > 2 {
		lifecycle = action.Closed
	}
	return action.Input{Run: action.RunFacts{Lifecycle: lifecycle}}, nil
}

func TestTerminalDuringAdmissionGetsFinalCheckedRead(t *testing.T) {
	var steps []string
	reader := &terminalAfterAdmissionReader{}
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: reader,
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{},
		Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true},
		Observer: observerFake{steps: &steps, modes: map[string]string{"run": "terminal"}}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err != nil || reader.reads != 4 || s.State("owner", "session") != (Status{Connection: "terminal", Blocker: "run_terminal", Ready: true}) {
		t.Fatal(err, reader.reads, s.State("owner", "session"))
	}
}

type capturingConverger struct{ stamp observation.Stamp }

func (c *capturingConverger) Converge(_ context.Context, _ action.Bound, in action.Input) (bool, error) {
	c.stamp = in.Run.Stamp
	return true, nil
}

func TestRecoveryConvergesPostRefreshSnapshot(t *testing.T) {
	var steps []string
	reader := &advancingReader{}
	converger := &capturingConverger{}
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: reader,
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{},
		Refresher: refresherFake{steps: &steps}, Converger: converger, Observer: observerFake{steps: &steps}}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err != nil || reader.reads != 2 || converger.stamp.Run != 5 {
		t.Fatal(err, reader.reads, converger.stamp)
	}
}

func (r selectiveReader) Read(_ context.Context, b action.Bound) (action.Input, error) {
	*r.steps = append(*r.steps, "read")
	if b.Binding.RunID == r.failedRun {
		return action.Input{}, errors.New("run unavailable")
	}
	return action.Input{Run: action.RunFacts{Lifecycle: action.Idle}}, nil
}

func TestSnapshotFailureDoesNotHideSibling(t *testing.T) {
	var steps []string
	s := &Service{Compatibility: probeFake{steps: &steps}, Snapshots: selectiveReader{steps: &steps, failedRun: "run"},
		Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{},
		Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
	other := reconnectBound()
	other.Binding.ID, other.Binding.RunID = "other-session", "other"
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound(), other}); err == nil || s.State("owner", "session").Blocker != "snapshot" || !s.Ready("owner", "other-session") {
		t.Fatal(err, s.State("owner", "session"), s.State("owner", "other-session"))
	}
}

func TestTransientLossCanRecoverWithoutReplayingMutation(t *testing.T) {
	var steps []string
	probe := &changingProbe{steps: &steps, failures: 1}
	s := &Service{Compatibility: probe, Snapshots: readerFake{steps: &steps, input: action.Input{Run: action.RunFacts{Lifecycle: action.Idle}}}, Invalidator: invalidatorFake{&steps}, Notifier: recoveryNotifierFake{}, Refresher: refresherFake{steps: &steps}, Converger: convergerFake{&steps, true}, Observer: observerFake{steps: &steps}}
	if err := s.Disconnect(t.Context(), []action.Bound{reconnectBound()}); err != nil || s.Ready("owner", "session") || s.State("owner", "session").Connection != "disconnected" {
		t.Fatal(err, s.State("owner", "session"))
	}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err == nil || s.Ready("owner", "session") || s.State("owner", "session").Connection != "disconnected" {
		t.Fatal(err, s.State("owner", "session"))
	}
	if err := s.Recover(t.Context(), []action.Bound{reconnectBound()}); err != nil || !s.Ready("owner", "session") {
		t.Fatal(err, s.State("owner", "session"))
	}
	if !reflect.DeepEqual(steps, []string{"stale", "stale", "probe", "stale", "probe", "read", "refresh", "read", "converge", "observe"}) {
		t.Fatal(steps)
	}
}

type changingProbe struct {
	steps    *[]string
	failures int
}

func (p *changingProbe) Check(context.Context) error {
	*p.steps = append(*p.steps, "probe")
	if p.failures > 0 {
		p.failures--
		return ErrUnavailable
	}
	return nil
}
