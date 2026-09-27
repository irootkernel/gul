package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

func TestActionMetadataReadsFloorAndUnresolvedBoundAttempts(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	ob := observationBinding(t, s)
	binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	b := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}, Carrier: session.Carrier{ControllerID: "controller"}}
	floor := observation.Stamp{Head: "3", Run: 3, Writer: 2, Interaction: 1}
	if err := s.Observation().Validate(t.Context(), ob, "3"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Observation().Commit(t.Context(), observation.Invalidation{Binding: ob, Cursor: "3", Floor: floor, Refresh: observation.Run, CorrelationID: "event", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	repo := s.Actions("dolgorae")
	state, err := repo.LocalState(t.Context(), b)
	if err != nil || state.Floor != floor || state.Operation != action.NoOperation || state.Ownership != action.OwnedSession {
		t.Fatal(state, err)
	}
	attempt := OperationAttempt{OperationID: "attempt", SubjectID: "owner", Kind: "SubmitTurn", RequestSHA256: strings.Repeat("0", 64), ControllerReferences: []ControllerReference{{Role: "source", ExpectedControllerID: "controller", BindingID: "controller"}}, State: "pending", CreatedAt: time.Now()}
	if err := s.Attempts().Record(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	state, err = repo.LocalState(t.Context(), b)
	if err != nil || state.Operation != action.OperationPending {
		t.Fatal(state, err)
	}
	if err := s.Attempts().MarkOutcomeUnknown(t.Context(), "attempt"); err != nil {
		t.Fatal(err)
	}
	state, err = repo.LocalState(t.Context(), b)
	if err != nil || state.Operation != action.OperationUnknown {
		t.Fatal(state, err)
	}
	if err := s.Attempts().MarkResolved(t.Context(), "attempt"); err != nil {
		t.Fatal(err)
	}
	state, err = repo.LocalState(t.Context(), b)
	if err != nil || state.Operation != action.NoOperation {
		t.Fatal(state, err)
	}
	b.Binding.SubjectID = "foreign"
	if _, err := repo.LocalState(t.Context(), b); err != action.ErrAuthority {
		t.Fatal("foreign binding admitted", err)
	}
}

func TestPendingAttemptBlocksRecoveryAlongsideUnknownAttempt(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			s, _ := openTestStore(t)
			t.Cleanup(func() { s.Close() })
			observationBinding(t, s)
			binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
			if err != nil {
				t.Fatal(err)
			}
			bound := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}, Carrier: session.Carrier{ControllerID: "controller"}}
			refs := []ControllerReference{{Role: "source", ExpectedControllerID: "controller", BindingID: "controller"}}
			ids := []string{"pending", "unknown"}
			if reverse {
				ids[0], ids[1] = ids[1], ids[0]
			}
			for _, id := range ids {
				attempt := OperationAttempt{OperationID: id, SubjectID: "owner", Kind: "SubmitTurn", RequestSHA256: strings.Repeat("0", 64), ControllerReferences: refs, State: "pending", CreatedAt: time.Now()}
				if legacy && id == "unknown" {
					attempt.ControllerReferences = nil
				}
				if err := s.Attempts().Record(t.Context(), attempt); err != nil {
					t.Fatal(err)
				}
				if id == "unknown" {
					if err := s.Attempts().MarkOutcomeUnknown(t.Context(), id); err != nil {
						t.Fatal(err)
					}
				}
			}
			repo := s.Actions("dolgorae")
			for _, resolved := range []bool{false, true} {
				if resolved {
					if err := s.Attempts().MarkResolved(t.Context(), "pending"); err != nil {
						t.Fatal(err)
					}
				}
				local, err := repo.LocalState(t.Context(), bound)
				if err != nil {
					t.Fatal(err)
				}
				want := action.OperationPending
				if resolved {
					want = action.OperationUnknown
				}
				if local.Operation != want {
					t.Fatalf("legacy=%v reverse=%v resolved=%v: %+v", legacy, reverse, resolved, local)
				}
				local.Credential = action.Healthy
				for _, directive := range []action.AggregateDirective{action.ReconcileAggregate, action.RecoverAggregate} {
					in := recoveryInput()
					in.Local = local
					in.Aggregate.Directive = directive
					got := action.Evaluate(in)
					if (got.Flags.CanRecover || got.Flags.CanReconcile) != resolved {
						t.Fatal("concurrent dispatch recovery admission", resolved, got)
					}
				}
			}
		}
	}
}

func recoveryInput() action.Input {
	stamp := observation.Stamp{Head: "4", Run: 4, Writer: 2, Interaction: 3}
	return action.Input{ControllerMatches: true, Checked: true, Freshness: action.Fresh, Run: action.RunFacts{Lifecycle: action.Idle, Variant: action.DedicatedReader, Control: action.Direct, Lane: action.Dedicated, Thread: action.Present, ActiveTurn: action.Missing, Access: action.Read, Verification: action.Verified, Authority: action.Unowned, Reconciliation: action.NoReconciliation, Requested: action.BestEffort, Achieved: action.BestEffort, Background: action.Absent, Server: action.ServerReady, Recovery: action.NoRecovery, RecoveryAction: action.NoRecoveryAction, Lineage: action.NoLineage, Stamp: stamp}, Writer: action.WriterProjection{Authority: action.Unowned, Access: action.UnknownAccess, Verification: action.Unverified, Owner: action.NoOwner, Reconciliation: action.NoReconciliation, Revision: 2, Stamp: observation.Stamp{Writer: 2}}, Profile: action.ProfileFacts{Compatibility: action.Compatible, Transition: action.Supported, BackgroundControl: action.Supported, MaximumAssurance: action.ProcessContained, SupportsLane: true}, Capabilities: action.Capabilities{Checked: true, Submit: true, Acquire: true, Release: true, Interrupt: true, Resolve: true, Recover: true, Reconcile: true, VerifyController: true, Pause: true, Resume: true, Close: true, ReaderWriter: true, DurableWriter: true, FirstWriteViaSubmit: true, Transition: action.Supported}, InteractionStamp: stamp, TimelineHead: "4", Aggregate: action.Aggregate{Directive: action.NoAggregateAction, Freshness: action.Fresh, Revision: 91, Lifecycle: action.SessionActive, CloseProgress: action.NoClose, Recovery: action.AggregateReady, CloseIntent: action.NoCloseIntent, NonretiredMembers: 1}, Local: action.LocalState{Ownership: action.OwnedSession, Credential: action.Healthy, Operation: action.NoOperation}, Request: action.Request{Intent: action.IntentRead, CloseIntent: action.CompleteSession}}
}
