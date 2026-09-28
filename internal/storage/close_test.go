package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/sessionclose"
	"github.com/rootkernel/gul/internal/workspace"
)

func TestCloseBindingChecksPreservePersistenceAndCancellationErrors(t *testing.T) {
	s, b := closeRefreshFixture(t)
	defer s.Close()
	conn, err := s.writer.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	checks := []struct {
		name    string
		call    func(context.Context, action.Bound) error
		missing error
	}{
		{"close", func(ctx context.Context, b action.Bound) error { return checkCloseBinding(ctx, conn, b.Binding) }, sessionclose.ErrConflict},
		{"refresh", func(ctx context.Context, b action.Bound) error { return checkCloseRefreshBinding(ctx, conn, b) }, sessionclose.ErrUnavailable},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(t.Context(), b); err != nil {
				t.Fatal(err)
			}
			foreign := b
			foreign.Binding.ID = "missing"
			if err := check.call(t.Context(), foreign); !errors.Is(err, check.missing) {
				t.Fatal(err)
			}
			cancelled, cancel := context.WithCancel(t.Context())
			cancel()
			if err := check.call(cancelled, b); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			deadline, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			defer stop()
			if err := check.call(deadline, b); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		})
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks {
		if err := check.call(t.Context(), b); !errors.Is(err, session.ErrPersistenceUnavailable) || errors.Is(err, check.missing) {
			t.Fatalf("%s persistence error = %v", check.name, err)
		}
	}
}

func closeBound(t *testing.T, s *Store) action.Bound {
	t.Helper()
	observationBinding(t, s)
	b, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil {
		t.Fatal(err)
	}
	return action.Bound{Binding: b, Workspace: workspace.Attachment{ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}, Carrier: session.Carrier{ControllerID: "controller"}}
}
func closeAttempt(id string, k sessionclose.Kind) sessionclose.Attempt {
	return sessionclose.Attempt{ID: id, RequestID: id, SubjectID: "owner", SessionID: "session", RequestSHA256: strings.Repeat("a", 64), Kind: k, CreatedAt: time.Now()}
}

func TestCloseConcurrentBeginAndStableRetry(t *testing.T) {
	s, filename := openTestStore(t)
	defer s.Close()
	b := closeBound(t, s)
	other, err := Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	const workers = 12
	var wg sync.WaitGroup
	dispatches := make(chan bool, workers)
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := closeAttempt(fmt.Sprint(i), sessionclose.Close)
			a.RequestID = "same-browser-request"
			repo := s.SessionClose()
			if i%2 == 0 {
				repo = other.SessionClose()
			}
			attempt, dispatch, err := repo.Begin(t.Context(), b, a)
			if err != nil {
				t.Error(err)
			}
			if attempt.Outcome.NextAction != "WAIT" {
				t.Errorf("concurrent attempt action = %q, want WAIT", attempt.Outcome.NextAction)
			}
			dispatches <- dispatch
		}(i)
	}
	wg.Wait()
	close(dispatches)
	count := 0
	for yes := range dispatches {
		if yes {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("dispatches %d", count)
	}
	got, found, err := s.SessionClose().Find(t.Context(), "owner", "session", "same-browser-request")
	if err != nil || !found || got.DispatchFinished || got.Outcome.NextAction != "WAIT" {
		t.Fatal(got, found, err)
	}
	pending, err := s.SessionClose().Pending(t.Context(), "owner", "session")
	if err != nil || len(pending) != 0 {
		t.Fatal("in-flight leaked into observation", pending, err)
	}
	generic, err := s.Attempts().Get(t.Context(), got.ID)
	if err != nil || generic.State != "pending" {
		t.Fatal(generic, err)
	}
	got.RequestSHA256 = strings.Repeat("b", 64)
	if _, _, err = s.SessionClose().Begin(t.Context(), b, got); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatal("changed retry", err)
	}
	if _, _, err = s.SessionClose().Begin(t.Context(), b, closeAttempt("recovery", sessionclose.Recover)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatal("pending recovery admitted", err)
	}
}

func TestCloseOutcomeAcceptsMappedContractAndRejectsUnknownValues(t *testing.T) {
	for _, code := range port.MappedErrorCodes() {
		if !validCloseOutcome(sessionclose.Outcome{Status: sessionclose.Rejected, Code: code}) {
			t.Errorf("mapped error code %q rejected", code)
		}
	}
	for _, next := range port.MappedActions() {
		if !validCloseOutcome(sessionclose.Outcome{Status: sessionclose.Rejected, NextAction: next}) {
			t.Errorf("mapped action %q rejected", next)
		}
	}
	for _, outcome := range []sessionclose.Outcome{
		{Status: "unknown-status"},
		{Status: sessionclose.Rejected, Code: "unknown-code"},
		{Status: sessionclose.Rejected, NextAction: "unknown-action"},
	} {
		if validCloseOutcome(outcome) {
			t.Errorf("unknown outcome accepted: %+v", outcome)
		}
	}
}

func TestCloseOperationReferenceValidatesBeforePersistence(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	b := closeBound(t, s)
	repo := s.SessionClose()
	for name, id := range map[string]string{
		"empty":         "",
		"oversized":     strings.Repeat("a", 1025),
		"invalid UTF-8": string([]byte{0xff}),
	} {
		t.Run(name, func(t *testing.T) {
			if ref, err := repo.OperationReference(t.Context(), b.Binding, id); !errors.Is(err, sessionclose.ErrInvalid) || ref != "" {
				t.Fatalf("invalid operation produced %q, %v", ref, err)
			}
		})
	}
	var count int
	if err := s.reader.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM session_close_operations").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid operation mappings persisted: %d, %v", count, err)
	}
	id := strings.Repeat("é", 512)
	ref, err := repo.OperationReference(t.Context(), b.Binding, id)
	if err != nil || ref == "" || ref == id {
		t.Fatalf("maximum operation reference = %q, %v", ref, err)
	}
	repeated, err := repo.OperationReference(t.Context(), b.Binding, id)
	if err != nil || repeated != ref {
		t.Fatalf("maximum operation reference changed: %q, %v", repeated, err)
	}
	if err := s.reader.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM session_close_operations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("maximum operation mappings = %d, %v", count, err)
	}
}

func TestCloseRecoveryAndFinalOutcomeSurviveRestart(t *testing.T) {
	s, filename := openTestStore(t)
	b := closeBound(t, s)
	repo := s.SessionClose()
	a, dispatch, err := repo.Begin(t.Context(), b, closeAttempt("close", sessionclose.Close))
	if err != nil || !dispatch {
		t.Fatal(a, dispatch, err)
	}
	ref, err := repo.OperationReference(t.Context(), b.Binding, "provider-close-id")
	if err != nil || strings.Contains(ref, "provider") {
		t.Fatal(ref, err)
	}
	a.DispatchFinished = true
	a.Outcome.Status = sessionclose.InProgress
	a.Outcome.OperationRef = ref
	if err = repo.Save(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.Pending(t.Context(), "owner", "session")
	if err != nil || len(pending) != 1 || pending[0].ID != a.ID {
		t.Fatal(pending, err)
	}
	unsafe := a
	unsafe.Outcome.Code = "provider-private-payload"
	if err := repo.Save(t.Context(), unsafe); !errors.Is(err, sessionclose.ErrInvalid) {
		t.Fatal("raw provider code persisted", err)
	}
	unsafe = a
	unsafe.Outcome.NextAction = "provider-private-action"
	if err := repo.Save(t.Context(), unsafe); !errors.Is(err, sessionclose.ErrInvalid) {
		t.Fatal("raw provider action persisted", err)
	}
	generic, err := s.Attempts().Get(t.Context(), a.ID)
	if err != nil || generic.State != "outcome_unknown" {
		t.Fatal(generic, err)
	}
	s.Close()
	s, err = Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	repo = s.SessionClose()
	gotRef, err := s.Presentation().OperationReference(t.Context(), b.Binding, "provider-close-id")
	if err != nil || gotRef != ref {
		t.Fatal(gotRef, ref, err)
	}
	if _, _, err = repo.Begin(t.Context(), b, closeAttempt("second-close", sessionclose.Close)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatal("second close admitted", err)
	}
	recovery, dispatch, err := repo.Begin(t.Context(), b, closeAttempt("recover", sessionclose.Recover))
	if err != nil || !dispatch {
		t.Fatal(recovery, dispatch, err)
	}
	recovery.DispatchFinished = true
	recovery.Outcome.Status = sessionclose.Confirmed
	recovery.Outcome.OperationRef = ref
	if err = repo.Save(t.Context(), recovery); err != nil {
		t.Fatal(err)
	}
	// Resolving recovery alone does not prove a prior close completed.
	generic, err = s.Attempts().Get(t.Context(), a.ID)
	if err != nil || generic.State != "outcome_unknown" {
		t.Fatal(generic, err)
	}
	a.Outcome.Status = sessionclose.Confirmed
	if err = repo.Save(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.ID, recovery.ID} {
		generic, err = s.Attempts().Get(t.Context(), id)
		if err != nil || generic.State != "resolved" {
			t.Fatal(generic, err)
		}
	}
	// An earlier observation that completes after aggregate confirmation cannot
	// restore a blocker that the confirmed aggregate already resolved.
	a.Outcome.Status = sessionclose.OutcomeUnknown
	if err = repo.Save(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	prior, err := s.Attempts().Get(t.Context(), a.ID)
	if err != nil || prior.State != "resolved" {
		t.Fatal(prior, err)
	}
	recovery.Outcome.Status = sessionclose.OutcomeUnknown
	recovery.Outcome.OperationRef = ""
	if err = repo.Save(t.Context(), recovery); err != nil {
		t.Fatal(err)
	}
	got, _, err := repo.Find(t.Context(), "owner", "session", "recover")
	if err != nil || got.Outcome.Status != sessionclose.Confirmed || got.Outcome.OperationRef != ref {
		t.Fatal(got, err)
	}
	foreign := b.Binding
	foreign.SubjectID = "foreign"
	if _, err = repo.OperationReference(t.Context(), foreign, "provider-close-id"); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatal("foreign mapping", err)
	}
	changed := b
	changed.Binding.RunID = "different"
	if _, _, err = repo.Begin(t.Context(), changed, closeAttempt("stale", sessionclose.Close)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatal("changed binding", err)
	}
}

func TestCloseBeginRollsBackAndRejectsUnrelatedUnknown(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	b := closeBound(t, s)
	if _, err := s.writer.ExecContext(t.Context(), `CREATE TRIGGER close_fault BEFORE INSERT ON session_close_attempts BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, dispatch, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("fault", sessionclose.Close)); err == nil || dispatch {
		t.Fatal(dispatch, err)
	}
	var n int
	if err := s.reader.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM provider_operation_attempts`).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, err := s.writer.ExecContext(t.Context(), `DROP TRIGGER close_fault`); err != nil {
		t.Fatal(err)
	}
	if err := s.Attempts().Record(t.Context(), OperationAttempt{OperationID: "submit", SubjectID: "owner", Kind: "SubmitTurn", RequestSHA256: strings.Repeat("0", 64), State: "pending", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Attempts().MarkOutcomeUnknown(t.Context(), "submit"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("recovery", sessionclose.Reconcile)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatal("unrelated unknown admitted", err)
	}
}

func TestExplicitRecoveryCanInspectControllerWithUnknownWriterAttempt(t *testing.T) {
	s, filename := openTestStore(t)
	b := closeBound(t, s)
	id, err := s.WriterAttempts().BeginWriter(t.Context(), b, true, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if attempt, err := s.Attempts().Mutation(t.Context(), id); err != nil || attempt.State != "outcome_unknown" {
		t.Fatalf("reopened writer attempt = %+v, %v", attempt, err)
	}
	if _, _, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("close", sessionclose.Close)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatalf("ordinary close admitted during uncertainty: %v", err)
	}
	recovery, dispatch, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("recover", sessionclose.Recover))
	if err != nil || !dispatch {
		t.Fatalf("explicit recovery blocked: %+v %t %v", recovery, dispatch, err)
	}
	if _, _, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("reconcile", sessionclose.Reconcile)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatalf("second recovery admitted while first is pending: %v", err)
	}
	if attempt, err := s.Attempts().Mutation(t.Context(), id); err != nil || attempt.State != "outcome_unknown" {
		t.Fatalf("recovery cleared unrelated uncertainty: %+v, %v", attempt, err)
	}
}

func TestExplicitReconcileCanInspectControllerWithUnknownSubmitAttempt(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	b := closeBound(t, s)
	now := time.Now().UTC()
	_, dispatch, err := s.Attempts().BeginMutation(t.Context(), MutationAttempt{OperationAttempt: OperationAttempt{
		OperationID: "submit", SubjectID: b.Binding.SubjectID, Kind: "SubmitTurn", RequestSHA256: strings.Repeat("a", 64),
		ControllerReferences: []ControllerReference{{Role: "source", BindingID: b.Binding.ControllerBindingID, ExpectedControllerID: b.Carrier.ControllerID}},
		State:                "pending", CreatedAt: now}, TargetRef: b.Binding.RunID, DeadlineAt: now.Add(20 * time.Second), ReconciliationRoute: "run_timeline"})
	if err != nil || !dispatch {
		t.Fatalf("submit attempt = %t %v", dispatch, err)
	}
	if _, _, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("pending-reconcile", sessionclose.Reconcile)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatalf("reconcile passed pending submit: %v", err)
	}
	if err := s.Attempts().MarkOutcomeUnknown(t.Context(), "submit"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("ordinary-close", sessionclose.Close)); !errors.Is(err, sessionclose.ErrConflict) {
		t.Fatalf("close passed unknown submit: %v", err)
	}
	if _, dispatch, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("reconcile", sessionclose.Reconcile)); err != nil || !dispatch {
		t.Fatalf("explicit reconcile blocked by unknown submit: %t %v", dispatch, err)
	}
}

func TestCloseMigrationFromVersionFivePreservesBinding(t *testing.T) {
	s, filename := openTestStore(t)
	b := closeBound(t, s)
	for _, query := range []string{`DROP TABLE mutation_attempt_details`, `DROP TABLE session_close_operations`, `DROP TABLE session_close_attempts`, `DELETE FROM schema_migrations WHERE version IN (6,7)`, `PRAGMA user_version=5`} {
		if _, err := s.writer.ExecContext(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err := Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Presentation().Binding(t.Context(), "owner", "session")
	if err != nil || got.RunID != b.Binding.RunID {
		t.Fatal(got, err)
	}
	if _, dispatch, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("migrated", sessionclose.Close)); err != nil || !dispatch {
		t.Fatal(dispatch, err)
	}
}

func TestClosePendingObservationCap(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	b := closeBound(t, s)
	for index := range 257 {
		a, dispatch, err := s.SessionClose().Begin(t.Context(), b, closeAttempt(fmt.Sprint(index), sessionclose.Recover))
		if err != nil || !dispatch {
			t.Fatal(a, dispatch, err)
		}
		a.DispatchFinished = true
		a.Outcome.Status = sessionclose.OutcomeUnknown
		if err := s.SessionClose().Save(t.Context(), a); err != nil {
			t.Fatal(err)
		}
		if index == 255 {
			if pending, err := s.SessionClose().Pending(t.Context(), "owner", "session"); err != nil || len(pending) != 256 {
				t.Fatal(len(pending), err)
			}
		}
	}
	if pending, err := s.SessionClose().Pending(t.Context(), "owner", "session"); !errors.Is(err, sessionclose.ErrUnavailable) || pending != nil {
		t.Fatal(len(pending), err)
	}
}

func TestCrashBetweenCloseBeginAndSaveBecomesObservableUnknown(t *testing.T) {
	s, filename := openTestStore(t)
	b := closeBound(t, s)
	a, dispatch, err := s.SessionClose().Begin(t.Context(), b, closeAttempt("lost-before-save", sessionclose.Close))
	if err != nil || !dispatch {
		t.Fatal(a, dispatch, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pending, err := s.SessionClose().Pending(t.Context(), b.Binding.SubjectID, b.Binding.ID)
	if err != nil || len(pending) != 1 || pending[0].ID != a.ID || pending[0].Outcome.Status != sessionclose.OutcomeUnknown || !pending[0].DispatchFinished {
		t.Fatalf("orphaned close = %+v, %v", pending, err)
	}
	generic, err := s.Attempts().Get(t.Context(), a.ID)
	if err != nil || generic.State != "outcome_unknown" {
		t.Fatalf("generic attempt = %+v, %v", generic, err)
	}
}
