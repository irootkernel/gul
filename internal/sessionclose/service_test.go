package sessionclose_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/action"
	actionprovider "github.com/rootkernel/gul/internal/action/contractprovider"
	historyprovider "github.com/rootkernel/gul/internal/history/contractprovider"
	interactionprovider "github.com/rootkernel/gul/internal/interaction/contractprovider"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	sessionprovider "github.com/rootkernel/gul/internal/session/contractprovider"
	"github.com/rootkernel/gul/internal/sessionclose"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type workspaceSource struct{ entry workspace.Attachment }

func (s workspaceSource) Revalidate(_ context.Context, subject, id string) (workspace.Attachment, error) {
	if subject != s.entry.SubjectID || id != s.entry.ID {
		return workspace.Attachment{}, workspace.ErrAttachmentNotFound
	}
	return s.entry, nil
}

type carrierSource struct{ missing bool }

func (s *carrierSource) Resolve(context.Context, string, string) (session.Carrier, error) {
	if s.missing {
		return session.Carrier{}, session.ErrCarrierUnavailable
	}
	return session.Carrier{AbsolutePath: "/carrier/controller", ControllerID: "controller", Generation: 1}, nil
}

type projections struct {
	mu                                       sync.Mutex
	input                                    action.Input
	offline                                  bool
	operation                                string
	snapshotRunDelta, snapshotAggregateDelta uint64
}

func (p *projections) change(fn func(*action.Input)) { p.mu.Lock(); defer p.mu.Unlock(); fn(&p.input) }
func (p *projections) Read(context.Context, action.Bound) (action.Input, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.offline {
		return action.Input{}, action.ErrUnavailable
	}
	return p.input, nil
}
func (*projections) Acquire(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	return action.WriterProjection{}, errors.New("unexpected acquire")
}
func (*projections) Release(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	return action.WriterProjection{}, errors.New("unexpected release")
}
func (p *projections) Snapshot(_ context.Context, w workspace.Attachment, run string, _ session.Carrier) (session.Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.offline {
		return session.Snapshot{}, session.ErrUnavailable
	}
	in := p.input
	lifecycle := map[action.SessionLifecycle]string{action.SessionActive: "active", action.SessionCompleting: "completing", action.SessionAborting: "aborting", action.SessionRecovering: "recovering", action.SessionCompleted: "completed", action.SessionAborted: "aborted"}[in.Aggregate.Lifecycle]
	progress := map[action.CloseProgress]string{action.NoClose: "none", action.CloseSettling: "settling", action.CloseCompleted: "confirmed", action.CloseAborted: "confirmed", action.CloseUnknown: "outcome_unknown", action.CloseRecovery: "recovery_required"}[in.Aggregate.CloseProgress]
	recovery := map[action.AggregateRecovery]string{action.AggregateReady: "none", action.AggregateUnknown: "outcome_unknown", action.AggregateSnapshotRequired: "snapshot_required", action.AggregateReconcileRequired: "reconcile_required"}[in.Aggregate.Recovery]
	return session.Snapshot{ProviderSessionID: "provider-session", PrimaryRunID: run, ProviderWorkspaceID: w.ProviderID, Lifecycle: lifecycle, Composition: "standalone_primary", ApprovalPolicy: "user_approval_required", CloseProgress: progress, Recovery: recovery, AggregateRevision: in.Aggregate.Revision + p.snapshotAggregateDelta, RunRevision: in.Run.Stamp.Run + p.snapshotRunDelta, CloseOperationID: p.operation, ObservedAt: time.Now(), Counts: session.Counts{NonretiredMembers: in.Aggregate.NonretiredMembers, NonterminalSpawns: in.Aggregate.NonterminalSpawns, PendingApprovals: in.Aggregate.PendingApprovals, AcceptedUnfinishedTasks: in.Aggregate.AcceptedUnfinishedTasks, UnknownOutcomeTasks: in.Aggregate.UnknownOutcomeTasks}}, nil
}

type mutationSource struct {
	calls atomic.Int32
	fn    func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error)
}

func (p *mutationSource) Mutate(ctx context.Context, b action.Bound, k sessionclose.Kind, r uint64, interrupt bool) (sessionclose.Mutation, error) {
	p.calls.Add(1)
	return p.fn(ctx, b, k, r, interrupt)
}

type refreshSource struct {
	calls atomic.Int32
	fail  bool
	last  action.RunFacts
}

func (r *refreshSource) Refresh(_ context.Context, _ action.Bound, run action.RunFacts) error {
	r.calls.Add(1)
	r.last = run
	if r.fail {
		return errors.New("independent aggregate refresh unavailable")
	}
	return nil
}

type closeFixture struct {
	store     *storage.Store
	service   *sessionclose.Service
	state     *projections
	mutations *mutationSource
	refresh   *refreshSource
	carrier   *carrierSource
}

func newStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := storage.Open(t.Context(), filepath.Join(dir, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	return s
}
func newFixture(t *testing.T) *closeFixture {
	t.Helper()
	s := newStore(t)
	ws := workspaceSource{workspace.Attachment{SubjectID: "owner", ID: "ws", CanonicalRoot: "/workspace", ProviderID: "provider-workspace", FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}}
	if err := s.Presentation().CreateAttachment(t.Context(), ws.entry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Presentation().InsertBinding(t.Context(), session.Binding{SubjectID: "owner", ID: "session", WorkspaceID: "ws", RunID: "root", ControllerBindingID: "controller", ProviderSessionID: "provider-session"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Cache().PutTimelineHead(t.Context(), "owner", "session", storage.UpstreamCursor{Value: "4"}); err != nil {
		t.Fatal(err)
	}
	p := &projections{input: readyInput()}
	carrier := &carrierSource{}
	refresh := &refreshSource{}
	mutation := &mutationSource{fn: func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
		return sessionclose.Mutation{Pending: true, OperationID: "provider-close"}, nil
	}}
	return &closeFixture{store: s, state: p, carrier: carrier, mutations: mutation, refresh: refresh, service: &sessionclose.Service{Repository: s.SessionClose(), Actions: &action.Service{Repository: s.Actions("dolgorae"), Workspaces: ws, Carriers: carrier, Provider: p, Gate: func(string, string) bool { return true }}, Sessions: session.NewService(ws, s.Presentation(), p, carrier), Provider: mutation, Refresh: refresh}}
}
func readyInput() action.Input {
	stamp := observation.Stamp{Head: "4", Run: 4, Writer: 2, Interaction: 3}
	return action.Input{ControllerMatches: true, Checked: true, Freshness: action.Fresh, Run: action.RunFacts{Lifecycle: action.Idle, Variant: action.DedicatedReader, Control: action.Direct, Lane: action.Dedicated, Thread: action.Present, ActiveTurn: action.Missing, Access: action.Read, Verification: action.Verified, Authority: action.Unowned, Reconciliation: action.NoReconciliation, Requested: action.BestEffort, Achieved: action.BestEffort, Background: action.Absent, Server: action.ServerReady, Recovery: action.NoRecovery, RecoveryAction: action.NoRecoveryAction, Lineage: action.NoLineage, Stamp: stamp}, Writer: action.WriterProjection{Authority: action.Unowned, Access: action.UnknownAccess, Verification: action.Unverified, Owner: action.NoOwner, Reconciliation: action.NoReconciliation, Revision: 2, Stamp: observation.Stamp{Writer: 2}}, Profile: action.ProfileFacts{Compatibility: action.Compatible, Transition: action.Supported, BackgroundControl: action.Supported, MaximumAssurance: action.ProcessContained, SupportsLane: true}, Capabilities: action.Capabilities{Checked: true, Close: true, Recover: true, Reconcile: true, Transition: action.Supported}, InteractionStamp: stamp, TimelineHead: "4", Aggregate: action.Aggregate{Directive: action.NoAggregateAction, Freshness: action.Fresh, Revision: 91, Lifecycle: action.SessionActive, CloseProgress: action.NoClose, Recovery: action.AggregateReady, CloseIntent: action.NoCloseIntent, NonretiredMembers: 1}}
}
func terminal(in *action.Input) {
	in.Run.Lifecycle = action.Closed
	in.Aggregate.Lifecycle = action.SessionCompleted
	in.Aggregate.CloseProgress = action.CloseCompleted
	in.Aggregate.CloseIntent = action.CompleteSession
}

func TestCloseReceiptAndStableAttemptRequireIndependentWholeSessionConfirmation(t *testing.T) {
	f := newFixture(t)
	f.mutations.fn = func(_ context.Context, b action.Bound, k sessionclose.Kind, rev uint64, interrupt bool) (sessionclose.Mutation, error) {
		if b.Binding.RunID != "root" || k != sessionclose.Close || rev != 4 || interrupt {
			t.Error("incorrect bound dispatch", b, k, rev, interrupt)
		}
		f.state.change(func(in *action.Input) {
			in.Aggregate.CloseProgress = action.CloseSettling
			in.Aggregate.Lifecycle = action.SessionCompleting
		})
		return sessionclose.Mutation{Pending: true, OperationID: "provider-close"}, nil
	}
	first, err := f.service.Close(t.Context(), "owner", "session", "request", false)
	if err != nil || first.Status != sessionclose.InProgress || first.OperationRef == "" || first.OperationRef == "provider-close" {
		t.Fatal(first, err)
	}
	second, err := f.service.Close(t.Context(), "owner", "session", "request", false)
	if err != nil || second.AttemptID != first.AttemptID || second.OperationRef != first.OperationRef || f.mutations.calls.Load() != 1 {
		t.Fatal(second, err)
	}
	f.state.change(terminal)
	// Closing the root while child work survives must never confirm the session.
	for name, block := range map[string]func(*action.Input){"spawn": func(in *action.Input) { in.Aggregate.NonterminalSpawns = 1 }, "approval": func(in *action.Input) { in.Aggregate.PendingApprovals = 1 }, "task": func(in *action.Input) { in.Aggregate.AcceptedUnfinishedTasks = 1 }, "unknown": func(in *action.Input) { in.Aggregate.UnknownOutcomeTasks = 1 }, "background": func(in *action.Input) { in.Run.Background = action.BackgroundActive }, "owned writer": func(in *action.Input) {
		in.Run.Authority = action.Active
		in.Run.Generation = 1
		in.Run.Access = action.Write
		in.Run.Variant = action.DedicatedActive
		in.Writer = action.WriterProjection{Authority: action.Active, Generation: 1, Access: action.Write, Verification: action.Verified, Owner: action.ThisSession, Lane: action.Dedicated, Requested: action.BestEffort, Achieved: action.BestEffort, Reconciliation: action.NoReconciliation, Revision: 2, Stamp: in.Run.Stamp}
	},
		"writer background": func(in *action.Input) { in.Writer.BackgroundBlocked = true }, "writer recovery": func(in *action.Input) { in.Writer.RecoveryBlocked = true }} {
		t.Run(name, func(t *testing.T) {
			f.state.change(func(in *action.Input) { *in = readyInput(); terminal(in); block(in) })
			out, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil || out.Status == sessionclose.Confirmed {
				t.Fatal(out, err)
			}
		})
	}
	f.state.operation = "provider-close"
	f.state.change(func(in *action.Input) { *in = readyInput(); terminal(in) })
	confirmed, err := f.service.Close(t.Context(), "owner", "session", "request", false)
	if err != nil || confirmed.Status != sessionclose.Confirmed || confirmed.AttemptID != first.AttemptID || f.mutations.calls.Load() != 1 || f.refresh.calls.Load() == 0 {
		t.Fatal(confirmed, err)
	}
	attempt, err := f.store.Attempts().Get(t.Context(), first.AttemptID)
	if err != nil || attempt.State != "resolved" {
		t.Fatal(attempt, err)
	}
}

func TestLostResponseNeverReplaysAndUnknownRetainsBlocker(t *testing.T) {
	f := newFixture(t)
	f.mutations.fn = func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
		f.state.mu.Lock()
		f.state.offline = true
		f.state.mu.Unlock()
		return sessionclose.Mutation{}, errors.New("lost reply")
	}
	first, err := f.service.Close(t.Context(), "owner", "session", "request", false)
	if err != nil || first.Status != sessionclose.OutcomeUnknown {
		t.Fatal(first, err)
	}
	again, err := f.service.Close(t.Context(), "owner", "session", "request", false)
	if err != nil || again.AttemptID != first.AttemptID || again.Status != sessionclose.OutcomeUnknown || f.mutations.calls.Load() != 1 {
		t.Fatal(again, err)
	}
	a, err := f.store.Attempts().Get(t.Context(), first.AttemptID)
	if err != nil || a.State != "outcome_unknown" {
		t.Fatal(a, err)
	}
	if _, err := f.service.Close(t.Context(), "owner", "session", "request", true); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatal("changed repeat admitted", err)
	}
}

func TestCloseDoesNotConfirmAnotherOperationOrOppositeIntent(t *testing.T) {
	for _, tc := range []struct {
		name string
		lost bool
	}{{name: "uncorrelated response loss", lost: true}, {name: "opposite intent", lost: false}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if tc.lost {
				f.mutations.fn = func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
					return sessionclose.Mutation{}, errors.New("lost reply")
				}
			}
			first, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil {
				t.Fatal(err)
			}
			f.state.operation = "provider-close"
			if tc.lost {
				f.state.operation = "another-close"
			}
			f.state.change(func(in *action.Input) {
				terminal(in)
				if !tc.lost {
					in.Aggregate.Lifecycle = action.SessionAborted
					in.Aggregate.CloseProgress = action.CloseAborted
					in.Aggregate.CloseIntent = action.AbortSession
				}
			})
			out, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil || out.Status == sessionclose.Confirmed || out.AttemptID != first.AttemptID || f.mutations.calls.Load() != 1 {
				t.Fatalf("foreign close confirmed: %+v, %v", out, err)
			}
		})
	}
}

func TestBrowserCancellationAfterTransmissionStillPersistsUnknown(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.mutations.fn = func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
		cancel()
		return sessionclose.Mutation{}, context.Canceled
	}
	out, err := f.service.Close(ctx, "owner", "session", "cancelled", false)
	if err != nil || out.Status != sessionclose.OutcomeUnknown {
		t.Fatal(out, err)
	}
	a, found, err := f.store.SessionClose().Find(t.Context(), "owner", "session", "cancelled")
	if err != nil || !found || !a.DispatchFinished || a.Outcome.Status != sessionclose.OutcomeUnknown {
		t.Fatal(a, found, err)
	}
	if err := f.service.ObservePending(t.Context(), "owner", "session"); err != nil {
		t.Fatal(err)
	}
	if f.mutations.calls.Load() != 1 {
		t.Fatal("observation replayed mutation")
	}
}

func TestCloseRejectsForeignCredentialStaleAndUnconfirmedInterrupt(t *testing.T) {
	for name, change := range map[string]func(*closeFixture){"credential": func(f *closeFixture) { f.carrier.missing = true }, "stale": func(f *closeFixture) { f.state.change(func(in *action.Input) { in.Freshness = action.Stale }) }, "active": func(f *closeFixture) {
		f.state.change(func(in *action.Input) { in.Run.ActiveTurn = action.Present; in.Run.Lifecycle = action.Running })
	}} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			change(f)
			out, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil || out.Status != sessionclose.Rejected || f.mutations.calls.Load() != 0 {
				t.Fatal(out, err)
			}
			if _, found, err := f.store.SessionClose().Find(t.Context(), "owner", "session", "request"); err != nil || found {
				t.Fatal("rejection created attempt", found, err)
			}
		})
	}
	f := newFixture(t)
	out, err := f.service.Close(t.Context(), "foreign", "session", "request", false)
	if !errors.Is(err, session.ErrNotFound) || out.AttemptID != "" || f.mutations.calls.Load() != 0 {
		t.Fatal(out, err)
	}
}

func TestRecoveryRequiresExplicitTypedInstructionAndIndependentRefresh(t *testing.T) {
	for _, kind := range []sessionclose.Kind{sessionclose.Recover, sessionclose.Reconcile} {
		t.Run(string(kind), func(t *testing.T) {
			f := newFixture(t)
			call := f.service.Recover
			if kind == sessionclose.Reconcile {
				call = f.service.Reconcile
			}
			denied, err := call(t.Context(), "owner", "session", "denied", false)
			if err != nil || denied.Status != sessionclose.Rejected || f.mutations.calls.Load() != 0 {
				t.Fatal(denied, err)
			}
			f.state.change(func(in *action.Input) {
				in.Aggregate.Recovery = action.AggregateUnknown
				in.Aggregate.CloseProgress = action.CloseUnknown
				if kind == sessionclose.Recover {
					in.Aggregate.Directive = action.RecoverAggregate
				} else {
					in.Aggregate.Directive = action.ReconcileAggregate
				}
			})
			f.refresh.fail = true
			f.mutations.fn = func(_ context.Context, _ action.Bound, k sessionclose.Kind, _ uint64, _ bool) (sessionclose.Mutation, error) {
				if k != kind {
					t.Error(k)
				}
				in := readyInput()
				terminal(&in)
				return sessionclose.Mutation{Run: in.Run}, nil
			}
			out, err := call(t.Context(), "owner", "session", "recovery", false)
			if err != nil || out.Status == sessionclose.Confirmed || f.mutations.calls.Load() != 1 || f.refresh.calls.Load() != 1 || f.refresh.last.Lifecycle != action.Closed {
				t.Fatal(out, err)
			}
			// Immediate Run success cannot replace the stale Session/Writer/Interaction read.
			f.refresh.fail = false
			again, err := call(t.Context(), "owner", "session", "recovery", false)
			if err != nil || again.Status != sessionclose.RecoveryRequired || again.AttemptID != out.AttemptID || f.mutations.calls.Load() != 1 {
				t.Fatal(again, err)
			}
		})
	}
}

func TestConcurrentDuplicateCannotReleaseLiveDispatch(t *testing.T) {
	f := newFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	f.mutations.fn = func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
		close(started)
		<-release
		return sessionclose.Mutation{Pending: true}, nil
	}
	done := make(chan error, 1)
	go func() { _, err := f.service.Close(t.Context(), "owner", "session", "request", false); done <- err }()
	<-started
	duplicate, err := f.service.Close(t.Context(), "owner", "session", "request", false)
	if err != nil || duplicate.Status != sessionclose.InProgress || duplicate.NextAction != "WAIT" || f.mutations.calls.Load() != 1 {
		t.Fatal(duplicate, err)
	}
	generic, err := f.store.Attempts().Get(t.Context(), duplicate.AttemptID)
	if err != nil || generic.State != "pending" {
		t.Fatal(generic, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type failedBindingRepository struct {
	sessionclose.Repository
	err error
}

func (r failedBindingRepository) Binding(context.Context, string, string) (session.Binding, error) {
	return session.Binding{}, r.err
}

func TestBindingFailurePreservesUnavailableAndNotFoundDistinction(t *testing.T) {
	for _, test := range []struct {
		name      string
		err, want error
	}{
		{"persistence", errors.New("database unavailable"), session.ErrPersistenceUnavailable},
		{"missing", session.ErrNotFound, session.ErrNotFound},
		{"cancelled", context.Canceled, context.Canceled},
		{"deadline", context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.service.Repository = failedBindingRepository{Repository: f.service.Repository, err: test.err}
			if _, err := f.service.Close(t.Context(), "owner", "session", "request", false); !errors.Is(err, test.want) {
				t.Fatal(err)
			}
			if err := f.service.ObservePending(t.Context(), "owner", "session"); !errors.Is(err, test.want) {
				t.Fatal(err)
			}
			if f.mutations.calls.Load() != 0 {
				t.Fatal("binding failure dispatched mutation")
			}
		})
	}
}

func TestRecoveryCanResolveWithoutClosingIdleSession(t *testing.T) {
	f := newFixture(t)
	f.state.change(func(in *action.Input) {
		in.Aggregate.Recovery = action.AggregateUnknown
		in.Aggregate.Directive = action.RecoverAggregate
	})
	f.mutations.fn = func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
		f.state.change(func(in *action.Input) { *in = readyInput() })
		return sessionclose.Mutation{Run: readyInput().Run}, nil
	}
	out, err := f.service.Recover(t.Context(), "owner", "session", "recover-idle", false)
	if err != nil || out.Status != sessionclose.Confirmed {
		t.Fatal(out, err)
	}
	a, err := f.store.Attempts().Get(t.Context(), out.AttemptID)
	if err != nil || a.State != "resolved" {
		t.Fatal(a, err)
	}
	state, err := f.service.Sessions.GetExecutionState(t.Context(), "owner", "session")
	if err != nil || state.Snapshot.Lifecycle != "active" || state.Snapshot.CloseProgress != "none" {
		t.Fatal(state, err)
	}
}

func TestCheckedScenarioCloseWaitsForAggregateAndKeepsOperationReference(t *testing.T) {
	ctx := t.Context()
	s := newStore(t)
	h := scenario.New(time.Now())
	inspected, err := h.InspectWorkspace(ctx, &pb.InspectWorkspaceRequest{AbsolutePath: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	ws := workspaceSource{workspace.Attachment{SubjectID: "owner", ID: "ws", CanonicalRoot: "/workspace", ProviderID: inspected.WorkspaceId, FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}}
	if err = s.Presentation().CreateAttachment(ctx, ws.entry); err != nil {
		t.Fatal(err)
	}
	if err = h.RegisterController(scenario.ControllerSpec{ID: "controller", Generation: 1, CarrierPath: "/carrier/controller", OrchestrationLaunch: true, PolicyName: "preprovisioned"}); err != nil {
		t.Fatal(err)
	}
	root, err := h.StartRun(ctx, &pb.StartRunRequest{Workspace: &pb.WorkspaceRef{AbsolutePath: "/workspace", ExpectedWorkspaceId: ws.entry.ProviderID}, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: "/carrier/controller", ExpectedControllerId: "controller", ExpectedControllerGeneration: 1}, IdempotencyKey: "launch", ProfileName: "default", ControlMode: pb.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: pb.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: pb.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	carrier := &carrierSource{}
	sessions := session.NewService(ws, s.Presentation(), sessionprovider.Provider{Port: h}, carrier)
	binding, err := sessions.BindPrimary(ctx, "owner", "ws", root.Run.RunId, "controller")
	if err != nil {
		t.Fatal(err)
	}
	caps, err := h.GetCapabilities(ctx, &pb.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Explicit negotiation fixture supplies fields omitted by the generic
	// scenario catalog, as in the existing history integration fixture.
	caps.Artifacts.MaximumInlineResponseBytes = 1 << 20
	caps.Interactions = &pb.InteractionCapabilities{MaximumResponseBytes: 64 << 10, MaximumSafePayloadBytes: 8 << 20,
		KnownKinds: []pb.InteractionKind{pb.InteractionKind_INTERACTION_KIND_COMMAND_EXECUTION_APPROVAL},
		Items:      []*pb.InteractionCapability{{Kind: pb.InteractionKind_INTERACTION_KIND_COMMAND_EXECUTION_APPROVAL, Support: pb.InteractionSupport_INTERACTION_SUPPORT_SUPPORTED}}}
	actions, err := actionprovider.New(h, caps)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := actionprovider.NewSessionControl(h)
	if err != nil {
		t.Fatal(err)
	}
	history, err := historyprovider.New(h, caps)
	if err != nil {
		t.Fatal(err)
	}
	interactions, err := interactionprovider.New(h, caps)
	if err != nil {
		t.Fatal(err)
	}
	refresh := &sessionclose.AggregateRefresher{Cache: s.CloseRefresh(), Sessions: sessions, Writer: actions, Interactions: interactions, History: history}
	svc := &sessionclose.Service{Repository: s.SessionClose(), Sessions: sessions, Actions: &action.Service{Repository: s.Actions("dolgorae"), Workspaces: ws, Carriers: carrier, Provider: actions, Gate: func(string, string) bool { return true }}, Provider: mutation, Refresh: refresh}
	child, err := h.SpawnSpecialist(root.Run.RunId, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	bound := action.Bound{Binding: binding, Workspace: ws.entry, Carrier: session.Carrier{AbsolutePath: "/carrier/controller", ControllerID: "controller", Generation: 1}}
	initial, err := actions.Read(ctx, bound)
	if err != nil {
		t.Fatal(err)
	}
	if err = refresh.Refresh(ctx, bound, initial.Run); err != nil {
		t.Fatal(err)
	}
	pending, err := svc.Close(ctx, "owner", binding.ID, "close", true)
	if err != nil || pending.Status != sessionclose.InProgress || pending.OperationRef == "" {
		t.Fatal(pending, err)
	}
	state, err := sessions.GetExecutionState(ctx, "owner", binding.ID)
	if err != nil || state.CloseOperationRef != pending.OperationRef {
		t.Fatal(state, err)
	}
	if err = h.SetCloseProgress(root.Run.RunId, pb.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED); err == nil {
		t.Fatal("scenario allowed uncompleted child")
	}
	if err = h.CompleteSpecialist(child.RunId); err != nil {
		t.Fatal(err)
	}
	if err = h.SetCloseProgress(root.Run.RunId, pb.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED); err != nil {
		t.Fatal(err)
	}
	// A transport failure during independent observation leaves the old pending outcome.
	if err = h.FaultNext("GetRun", scenario.BeforeCommit, connect.NewError(connect.CodeUnavailable, errors.New("offline"))); err != nil {
		t.Fatal(err)
	}
	stale, err := svc.Close(ctx, "owner", binding.ID, "close", true)
	if err != nil || stale.Status == sessionclose.Confirmed {
		t.Fatal(stale, err)
	}
	confirmed, err := svc.Close(ctx, "owner", binding.ID, "close", true)
	if err != nil || confirmed.Status != sessionclose.Confirmed || confirmed.AttemptID != pending.AttemptID || confirmed.OperationRef != pending.OperationRef {
		t.Fatal(confirmed, err)
	}
}

func TestCloseObservationRejectsDisagreeingReadsAndForeignOperation(t *testing.T) {
	for _, name := range []string{"run revision", "aggregate revision", "operation"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			first, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil || first.Status != sessionclose.OutcomeUnknown || first.OperationRef == "" {
				t.Fatal(first, err)
			}
			f.state.change(terminal)
			switch name {
			case "run revision":
				f.state.snapshotRunDelta = 1
			case "aggregate revision":
				f.state.snapshotAggregateDelta = 1
			case "operation":
				f.state.operation = "other-provider-operation"
			}
			out, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil || out != first || f.mutations.calls.Load() != 1 {
				t.Fatal(out, err)
			}
			f.state.snapshotRunDelta, f.state.snapshotAggregateDelta = 0, 0
			f.state.operation = "provider-close"
			out, err = f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil || out.Status != sessionclose.Confirmed || out.OperationRef != first.OperationRef || f.mutations.calls.Load() != 1 {
				t.Fatal(out, err)
			}
		})
	}
}

type faultCloseRepository struct {
	sessionclose.Repository
	failSave, failReference bool
	failure                 error
}

func (r *faultCloseRepository) Save(ctx context.Context, attempt sessionclose.Attempt) error {
	if r.failSave {
		return r.failure
	}
	return r.Repository.Save(ctx, attempt)
}
func (r *faultCloseRepository) OperationReference(ctx context.Context, binding session.Binding, operation string) (string, error) {
	if r.failReference {
		return "", r.failure
	}
	return r.Repository.OperationReference(ctx, binding, operation)
}
func TestClosePostTransmissionStorageFailuresNeverPermitResend(t *testing.T) {
	for _, point := range []string{"reference", "save", "observation-reference"} {
		t.Run(point, func(t *testing.T) {
			f := newFixture(t)
			failure := errors.New("injected persistence failure")
			repository := &faultCloseRepository{Repository: f.service.Repository, failure: failure}
			f.service.Repository = repository
			if point == "observation-reference" {
				f.mutations.fn = func(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
					return sessionclose.Mutation{}, errors.New("lost response")
				}
				if _, err := f.service.Close(t.Context(), "owner", "session", "request", false); err != nil {
					t.Fatal(err)
				}
				f.state.operation = "observed-operation"
			}
			repository.failReference, repository.failSave = point != "save", point == "save"
			out, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if point == "observation-reference" {
				if err != nil || out.Status != sessionclose.OutcomeUnknown || out.OperationRef != "" {
					t.Fatalf("unrelated provider operation was attributed: %+v, %v", out, err)
				}
			} else if !errors.Is(err, failure) {
				t.Fatalf("failure lost: %v", err)
			}
			retained, found, err := f.store.SessionClose().Find(t.Context(), "owner", "session", "request")
			if err != nil || !found || f.mutations.calls.Load() != 1 {
				t.Fatalf("retained=%+v found=%v err=%v", retained, found, err)
			}
			want := sessionclose.OutcomeUnknown
			if point == "save" {
				want = sessionclose.InProgress
			}
			if retained.Outcome.Status != want || retained.DispatchFinished != (point != "save") {
				t.Fatalf("unsafe retained state: %+v", retained)
			}
			repository.failSave, repository.failReference = false, false
			if point == "save" {
				if err := f.service.ObservePending(t.Context(), "owner", "session"); err != nil {
					t.Fatal(err)
				}
				retained, found, err = f.store.SessionClose().Find(t.Context(), "owner", "session", "request")
				if err != nil || !found || !retained.DispatchFinished || retained.Outcome.Status == sessionclose.InProgress && retained.Outcome.NextAction == "WAIT" {
					t.Fatalf("completed dispatch was not restored for observation: %+v, %v", retained, err)
				}
			}
			recovered, err := f.service.Close(t.Context(), "owner", "session", "request", false)
			if err != nil {
				t.Fatal(err)
			}
			if point == "reference" && recovered.OperationRef == "" {
				t.Fatal("original provider operation identity was lost after storage recovered")
			}
			if out, err := f.service.Close(t.Context(), "owner", "session", "different", false); err != nil || out.Status != sessionclose.Rejected {
				t.Fatalf("conflicting retry admitted: %+v, %v", out, err)
			}
			if f.mutations.calls.Load() != 1 {
				t.Fatal("storage failure allowed resend")
			}
		})
	}
}
