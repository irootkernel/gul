package storage

import (
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
)

func TestConcurrentSameKeyMutationBegin(t *testing.T) {
	for _, kind := range []string{"StartRun", "SubmitTurn"} {
		t.Run(kind, func(t *testing.T) {
			s, filename := openTestStore(t)
			now := time.Now().UTC()
			if err := s.Auth().CreateAccount(t.Context(), "owner", now); err != nil {
				t.Fatal(err)
			}
			other, err := Open(t.Context(), filename)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			candidate := MutationAttempt{OperationAttempt: OperationAttempt{
				OperationID: "same-key", SubjectID: "owner", Kind: kind, RequestSHA256: strings.Repeat("a", 64),
				State: "pending", CreatedAt: now,
				ControllerReferences: []ControllerReference{{Role: "source", BindingID: "binding-1", ExpectedControllerID: "controller"}},
			}, TargetRef: "run-1", DeadlineAt: now.Add(20 * time.Second), ReconciliationRoute: "run_timeline"}
			if kind == "StartRun" {
				candidate.ReplayKey, candidate.ReplayAvailable = candidate.OperationID, true
				candidate.ControllerReferences[0] = ControllerReference{Role: "destination", CredentialKey: "controllers/destination", ExpectedControllerID: "controller"}
				candidate.TargetRef, candidate.ReconciliationRoute = "workspace", "exact_start_then_controller_list"
			}
			const workers = 12
			start := make(chan struct{})
			var wg sync.WaitGroup
			var dispatches atomic.Int32
			for i := range workers {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					repo := s.Attempts()
					if i%2 == 0 {
						repo = other.Attempts()
					}
					got, dispatch, err := repo.BeginMutation(t.Context(), candidate)
					if err != nil || got.RequestSHA256 != candidate.RequestSHA256 || got.State != "pending" {
						t.Errorf("concurrent begin = %+v, %t, %v", got, dispatch, err)
					}
					if dispatch {
						dispatches.Add(1)
					}
				}(i)
			}
			close(start)
			wg.Wait()
			if got := dispatches.Load(); got != 1 {
				t.Fatalf("provider dispatches = %d", got)
			}
			var rows int
			if err := s.reader.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM provider_operation_attempts WHERE operation_id=?", candidate.OperationID).Scan(&rows); err != nil || rows != 1 {
				t.Fatalf("stored attempts = %d, %v", rows, err)
			}
			candidate.RequestSHA256 = strings.Repeat("b", 64)
			if _, _, err := s.Attempts().BeginMutation(t.Context(), candidate); !errors.Is(err, ErrMutationConflict) {
				t.Fatalf("changed same-key request = %v", err)
			}
		})
	}
}

func TestTokenlessWriterAttemptSurvivesRestartAndBlocksConflict(t *testing.T) {
	s, filename := openTestStore(t)
	b := closeBound(t, s)
	id, err := s.WriterAttempts().BeginWriter(t.Context(), b, true, 4, action.WriterProjection{Revision: 2})
	if err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "pending" || a.ReplayAvailable || a.ReconciliationRoute != "run_writer" {
		t.Fatalf("pretransmission attempt = %+v, %v", a, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("restart attempt = %+v, %v", a, err)
	}
	var finished int
	if err := s.reader.QueryRowContext(t.Context(), "SELECT dispatch_finished FROM writer_attempt_details WHERE operation_id=?", id).Scan(&finished); err != nil || finished != 1 {
		t.Fatalf("reopened dispatch = %d, %v", finished, err)
	}
	if _, err := s.WriterAttempts().BeginWriter(t.Context(), b, false, 5, action.WriterProjection{Revision: 3}); !errors.Is(err, action.ErrOutcomeUnknown) {
		t.Fatalf("conflicting writer mutation = %v", err)
	}
	stamp := observation.Stamp{Head: "4", Run: 4, Writer: 3, Interaction: 2}
	settled := action.Input{Checked: true, ControllerMatches: true, Freshness: action.Fresh, Aggregate: action.Aggregate{Freshness: action.Fresh},
		Run:              action.RunFacts{Recovery: action.NoRecovery, Reconciliation: action.NoReconciliation, Stamp: stamp},
		Writer:           action.WriterProjection{Owner: action.ThisSession, Revision: 3, Generation: 1, Reconciliation: action.NoReconciliation, Stamp: stamp},
		InteractionStamp: stamp, TimelineHead: "4"}
	if _, err := s.writer.ExecContext(t.Context(), `INSERT INTO runtime_timeline_cache(subject_id,session_id,captured_head_cursor) VALUES ('owner','session','4')`); err != nil {
		t.Fatal(err)
	}
	if converged, err := s.Actions("dolgorae").Converge(t.Context(), b, settled); err != nil || !converged {
		t.Fatalf("writer state did not converge: %t, %v", converged, err)
	}
	// The timeline cursor has been verified independently of aggregate reads.
	if _, err := s.writer.ExecContext(t.Context(), "UPDATE observation_refreshes SET refresh_mask=0 WHERE subject_id='owner' AND session_id='session'"); err != nil {
		t.Fatal(err)
	}
	stale := settled
	stale.Writer.Revision = 2
	if err := s.WriterAttempts().ReconcileWriter(t.Context(), b, stale); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("stale writer state settled attempt: %+v, %v", a, err)
	}
	if _, err := s.writer.ExecContext(t.Context(), "UPDATE runtime_projection_cache SET freshness='stale' WHERE subject_id='owner' AND session_id='session' AND aggregate_kind='writer'"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriterAttempts().ReconcileWriter(t.Context(), b, settled); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("invalidated writer state settled attempt: %+v, %v", a, err)
	}
	if converged, err := s.Actions("dolgorae").Converge(t.Context(), b, settled); err != nil || !converged {
		t.Fatalf("fresh writer state did not converge: %t, %v", converged, err)
	}
	if err := s.WriterAttempts().ReconcileWriter(t.Context(), b, settled); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "resolved" || a.OutcomeRef != "writer_state:3" {
		t.Fatalf("authoritative writer state did not settle attempt: %+v, %v", a, err)
	}
	legacy, err := s.WriterAttempts().BeginWriter(t.Context(), b, false, 5, action.WriterProjection{Revision: 3, Generation: 1})
	if err != nil {
		t.Fatalf("resolved writer blocked: %v", err)
	}
	if err := s.WriterAttempts().FinishWriter(t.Context(), legacy, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.ExecContext(t.Context(), "DELETE FROM writer_attempt_details WHERE operation_id=?", legacy); err != nil {
		t.Fatal(err)
	}
	settled.Writer.Owner, settled.Writer.Revision, settled.Writer.Generation = action.NoOwner, 4, 2
	settled.Run.Stamp.Writer, settled.Writer.Stamp.Writer, settled.InteractionStamp.Writer = 4, 4, 4
	if converged, err := s.Actions("dolgorae").Converge(t.Context(), b, settled); err != nil || !converged {
		t.Fatalf("later writer state did not converge: %t, %v", converged, err)
	}
	if err := s.WriterAttempts().ReconcileWriter(t.Context(), b, settled); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), legacy); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("legacy attempt without baseline was settled: %+v, %v", a, err)
	}
}

func TestWriterDisconnectCannotSettleLiveDispatch(t *testing.T) {
	s, _ := openTestStore(t)
	b := closeBound(t, s)
	id, err := s.WriterAttempts().BeginWriter(t.Context(), b, true, 4, action.WriterProjection{Revision: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Provider connectivity can fail while the original RPC is still running.
	if err := s.Attempts().MarkOutcomeUnknown(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	stamp := observation.Stamp{Head: "4", Run: 4, Writer: 3, Interaction: 2}
	in := action.Input{Checked: true, ControllerMatches: true, Freshness: action.Fresh, Aggregate: action.Aggregate{Freshness: action.Fresh},
		Run:              action.RunFacts{Recovery: action.NoRecovery, Reconciliation: action.NoReconciliation, Stamp: stamp},
		Writer:           action.WriterProjection{Owner: action.ThisSession, Revision: 3, Generation: 1, Reconciliation: action.NoReconciliation, Stamp: stamp},
		InteractionStamp: stamp, TimelineHead: "4"}
	if _, err := s.writer.ExecContext(t.Context(), `INSERT INTO runtime_timeline_cache(subject_id,session_id,captured_head_cursor) VALUES ('owner','session','4')`); err != nil {
		t.Fatal(err)
	}
	if converged, err := s.Actions("dolgorae").Converge(t.Context(), b, in); err != nil || !converged {
		t.Fatalf("writer state did not converge: %t, %v", converged, err)
	}
	if _, err := s.writer.ExecContext(t.Context(), "UPDATE observation_refreshes SET refresh_mask=0 WHERE subject_id='owner' AND session_id='session'"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriterAttempts().ReconcileWriter(t.Context(), b, in); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("live dispatch was settled: %+v, %v", a, err)
	}
	if err := s.WriterAttempts().FinishWriter(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.WriterAttempts().ReconcileWriter(t.Context(), b, in); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "resolved" {
		t.Fatalf("finished dispatch remained unresolved: %+v, %v", a, err)
	}
}

func TestProtectedInteractionAttemptStoresOnlyMetadata(t *testing.T) {
	s, filename := openTestStore(t)
	if err := s.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	b := interaction.Bound{Binding: session.Binding{SubjectID: "owner", RunID: "run", ControllerBindingID: "binding"}, Carrier: session.Carrier{ControllerID: "controller"}}
	id, err := s.InteractionAttempts().BeginInteraction(t.Context(), b, "interaction", "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filename, filename + "-wal"} {
		data, err := os.ReadFile(path)
		if err == nil && (strings.Contains(string(data), "protected-canary") || strings.Contains(string(data), "stable-key")) {
			t.Fatalf("protected material in %s", path)
		}
	}
	if err := s.InteractionAttempts().FinishInteraction(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InteractionAttempts().BeginInteraction(t.Context(), b, "interaction", "another-key"); !errors.Is(err, interaction.ErrAttemptConflict) {
		t.Fatalf("unknown protected response allowed duplicate: %v", err)
	}
	if err := s.Attempts().ResolveMutation(t.Context(), id, "resolved-by-fresh-card"); err != nil {
		t.Fatal(err)
	}
}

func TestLaterAuthoritativeInteractionCardSettlesUnknownAttempt(t *testing.T) {
	s, _ := openTestStore(t)
	if err := s.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	b := interaction.Bound{Binding: session.Binding{SubjectID: "owner", RunID: "run", ControllerBindingID: "binding"}, Carrier: session.Carrier{ControllerID: "controller"}}
	id, err := s.InteractionAttempts().BeginInteraction(t.Context(), b, "interaction", "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InteractionAttempts().FinishInteraction(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.InteractionAttempts().ReconcileInteraction(t.Context(), b, "interaction", interaction.Pending); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("pending card cleared uncertainty = %+v, %v", a, err)
	}
	if err := s.InteractionAttempts().ReconcileInteraction(t.Context(), b, "interaction", interaction.Resolved); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "resolved" || a.OutcomeRef != "resolved" {
		t.Fatalf("resolved card did not settle attempt = %+v, %v", a, err)
	}
}
