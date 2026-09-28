package storage

import (
	"strings"
	"sync"
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
	seedVerifiedTimeline(t, s)
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
			seedVerifiedTimeline(t, s)
			binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
			if err != nil {
				t.Fatal(err)
			}
			bound := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}, Carrier: session.Carrier{ControllerID: "controller"}}
			repo := s.Actions("dolgorae")
			if fresh, err := repo.Converge(t.Context(), bound, recoveryInput()); err != nil || !fresh {
				t.Fatalf("initial provider snapshot: %v, %v", fresh, err)
			}
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

func seedVerifiedTimeline(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Cache().PutTimelineHead(t.Context(), "owner", "session", UpstreamCursor{Value: "4"}); err != nil {
		t.Fatal(err)
	}
}

func TestActionConvergenceSurvivesInvalidationAndRestart(t *testing.T) {
	s, filename := openTestStore(t)
	ob := observationBinding(t, s)
	seedVerifiedTimeline(t, s)
	binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	bound := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}}
	repo := s.Actions("dolgorae")
	initial := recoveryInput()
	if ready, err := repo.Converge(t.Context(), bound, initial); err != nil || !ready {
		t.Fatal(ready, err)
	}
	state, err := repo.LocalState(t.Context(), bound)
	if err != nil || state.ProjectionsStale {
		t.Fatal(state, err)
	}
	floor := observation.Stamp{Head: "5", Run: 5, Writer: 2, Interaction: 4}
	if err := s.Observation().Validate(t.Context(), ob, "5"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Observation().Commit(t.Context(), observation.Invalidation{Binding: ob, Cursor: "5", Floor: floor, Refresh: observation.Run | observation.Interaction | observation.Timeline, CorrelationID: "event", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if ready, err := repo.Converge(t.Context(), bound, initial); err != nil || ready {
		t.Fatal("old snapshot cleared event", ready, err)
	}
	state, err = repo.LocalState(t.Context(), bound)
	if err != nil || !state.ProjectionsStale {
		t.Fatal(state, err)
	}
	current := recoveryInput()
	current.Run.Stamp = floor
	current.InteractionStamp = floor
	current.TimelineHead = floor.Head
	if ready, err := repo.Converge(t.Context(), bound, current); err != nil || ready {
		t.Fatal("unverified timeline head was admitted", ready, err)
	}
	state, err = repo.LocalState(t.Context(), bound)
	if err != nil || !state.ProjectionsStale {
		t.Fatal("timeline refresh was skipped", state, err)
	}
	if err := s.CloseRefresh().CompleteRefresh(t.Context(), bound, observation.Timeline, floor); err != nil {
		t.Fatal(err)
	}
	if ready, err := repo.Converge(t.Context(), bound, current); err != nil || !ready {
		t.Fatal("verified timeline did not converge", ready, err)
	}
	state, err = repo.LocalState(t.Context(), bound)
	if err != nil || state.ProjectionsStale {
		t.Fatal(state, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	repo = s.Actions("dolgorae")
	state, err = repo.LocalState(t.Context(), bound)
	if err != nil || !state.ProjectionsStale {
		t.Fatal("restart did not stale cache", state, err)
	}
	if ready, err := repo.Converge(t.Context(), bound, current); err != nil || !ready {
		t.Fatal(ready, err)
	}
	state, err = repo.LocalState(t.Context(), bound)
	if err != nil || !state.ProjectionsStale {
		t.Fatal("action read cleared timeline work", state, err)
	}
	if err := s.CloseRefresh().CompleteRefresh(t.Context(), bound, observation.Timeline, floor); err != nil {
		t.Fatal(err)
	}
	if ready, err := repo.Converge(t.Context(), bound, current); err != nil || !ready {
		t.Fatal("restart refresh failed", ready, err)
	}
}

func TestConvergenceRejectsInconsistentInputAndRollsBackFault(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	observationBinding(t, s)
	seedVerifiedTimeline(t, s)
	binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	bound := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}}
	repo := s.Actions("dolgorae")
	base := recoveryInput()
	if ready, err := repo.Converge(t.Context(), bound, base); err != nil || !ready {
		t.Fatal(ready, err)
	}
	var before string
	if err := s.writer.QueryRowContext(t.Context(), `SELECT projection_stamp FROM runtime_projection_cache WHERE subject_id='owner' AND session_id='session' AND aggregate_kind='run'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*action.Input){
		func(in *action.Input) { in.Checked = false },
		func(in *action.Input) { in.InteractionStamp.Run++ },
		func(in *action.Input) { in.TimelineHead = "5" },
		func(in *action.Input) { in.Writer.Stamp.Writer++ },
	} {
		in := recoveryInput()
		change(&in)
		if ready, err := repo.Converge(t.Context(), bound, in); err != nil || ready {
			t.Fatal(ready, err)
		}
		var after string
		if err := s.writer.QueryRowContext(t.Context(), `SELECT projection_stamp FROM runtime_projection_cache WHERE subject_id='owner' AND session_id='session' AND aggregate_kind='run'`).Scan(&after); err != nil || after != before {
			t.Fatal(after, err)
		}
	}
	if err := s.MarkProviderUnavailable(t.Context(), bound); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.ExecContext(t.Context(), `CREATE TRIGGER convergence_fault BEFORE UPDATE ON observation_refreshes BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	if ready, err := repo.Converge(t.Context(), bound, base); err == nil || ready {
		t.Fatal(ready, err)
	}
	var after, freshness string
	if err := s.writer.QueryRowContext(t.Context(), `SELECT projection_stamp,freshness FROM runtime_projection_cache WHERE subject_id='owner' AND session_id='session' AND aggregate_kind='run'`).Scan(&after, &freshness); err != nil || after != before || freshness != "stale" {
		t.Fatal(after, freshness, err)
	}
	var mask int
	if err := s.writer.QueryRowContext(t.Context(), `SELECT refresh_mask FROM observation_refreshes WHERE subject_id='owner' AND session_id='session'`).Scan(&mask); err != nil || mask != int(observation.AllAggregates) {
		t.Fatal(mask, err)
	}
}

func TestProviderDisconnectPreservesRunAndMarksAttemptUnresolved(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	observationBinding(t, s)
	seedVerifiedTimeline(t, s)
	binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	bound := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}}
	repo := s.Actions("dolgorae")
	if fresh, err := repo.Converge(t.Context(), bound, recoveryInput()); err != nil || !fresh {
		t.Fatal(fresh, err)
	}
	attempt := OperationAttempt{OperationID: "in-flight", SubjectID: "owner", Kind: "SubmitTurn", RequestSHA256: strings.Repeat("0", 64), ControllerReferences: []ControllerReference{{Role: "source", BindingID: "controller", ExpectedControllerID: "controller"}}, State: "pending", CreatedAt: time.Now()}
	if err := s.Attempts().Record(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.ExecContext(t.Context(), `INSERT INTO session_close_attempts
(attempt_id,subject_id,session_id,request_id,request_sha256,operation_kind,interrupt,created_at,dispatch_finished,outcome_status,operation_ref,code,next_action)
VALUES (?,?,?,?,?,'CloseRun',0,?,1,'in_progress','','','WAIT')`, attempt.OperationID, "owner", "session", "request", attempt.RequestSHA256, timestamp(attempt.CreatedAt)); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkProviderUnavailable(t.Context(), bound); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Attempts().Get(t.Context(), "in-flight")
	if err != nil || stored.State != "outcome_unknown" {
		t.Fatal(stored, err)
	}
	var closeStatus, nextAction string
	if err := s.writer.QueryRowContext(t.Context(), `SELECT outcome_status,next_action FROM session_close_attempts WHERE attempt_id=?`, attempt.OperationID).Scan(&closeStatus, &nextAction); err != nil || closeStatus != "outcome_unknown" || nextAction != "REFRESH_SNAPSHOT" {
		t.Fatal(closeStatus, nextAction, err)
	}
	state, err := repo.LocalState(t.Context(), bound)
	if err != nil || !state.ProjectionsStale || state.Operation != action.OperationUnknown {
		t.Fatal(state, err)
	}
	after, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil || after.RunID != binding.RunID {
		t.Fatal(after, err)
	}
	delivery, err := s.Observation().ReadDelivery(t.Context(), "owner", "session", 0)
	if err != nil || len(delivery.Events) != 1 || delivery.Events[0].Kind != "projection_invalidated" {
		t.Fatal(delivery, err)
	}
	if ready, err := repo.Converge(t.Context(), bound, recoveryInput()); err != nil || !ready {
		t.Fatal(ready, err)
	}
	if err := s.MarkProviderRecovered(t.Context(), bound); err != nil {
		t.Fatal(err)
	}
	delivery, err = s.Observation().ReadDelivery(t.Context(), "owner", "session", 0)
	if err != nil || len(delivery.Events) != 2 || delivery.Events[1].Sequence <= delivery.Events[0].Sequence || delivery.Events[1].CorrelationID != "provider-recovered" {
		t.Fatal(delivery, err)
	}
}

func TestProviderDisconnectRollsBackOnDeliveryFault(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	observationBinding(t, s)
	seedVerifiedTimeline(t, s)
	binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	bound := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}}
	if ready, err := s.Actions("dolgorae").Converge(t.Context(), bound, recoveryInput()); err != nil || !ready {
		t.Fatal(ready, err)
	}
	attempt := OperationAttempt{OperationID: "in-flight", SubjectID: "owner", Kind: "SubmitTurn", RequestSHA256: strings.Repeat("0", 64), State: "pending", CreatedAt: time.Now()}
	if err := s.Attempts().Record(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.ExecContext(t.Context(), `CREATE TRIGGER reconnect_fault BEFORE INSERT ON client_event_journal BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkProviderUnavailable(t.Context(), bound); err == nil {
		t.Fatal("disconnect partially persisted")
	}
	stored, err := s.Attempts().Get(t.Context(), attempt.OperationID)
	if err != nil || stored.State != "pending" {
		t.Fatal(stored, err)
	}
	state, err := s.Actions("dolgorae").LocalState(t.Context(), bound)
	if err != nil || state.ProjectionsStale {
		t.Fatal(state, err)
	}
}

func TestConcurrentInvalidationNeverLeavesOldProjectionFresh(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	ob := observationBinding(t, s)
	seedVerifiedTimeline(t, s)
	binding, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	bound := action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}}
	repo := s.Actions("dolgorae")
	old := recoveryInput()
	if ready, err := repo.Converge(t.Context(), bound, old); err != nil || !ready {
		t.Fatal(ready, err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := repo.Converge(t.Context(), bound, old)
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := s.Observation().Validate(t.Context(), ob, "5"); err != nil {
			errs <- err
			return
		}
		_, err := s.Observation().Commit(t.Context(), observation.Invalidation{Binding: ob, Cursor: "5", Floor: observation.Stamp{Head: "5", Run: 5, Writer: 2, Interaction: 4}, Refresh: observation.Run | observation.Timeline, CorrelationID: "event", At: time.Now()})
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := repo.LocalState(t.Context(), bound)
	if err != nil || !state.ProjectionsStale {
		t.Fatal("old read won over later event", state, err)
	}
}
