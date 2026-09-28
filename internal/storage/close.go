package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/sessionclose"
)

// CloseRepository persists coordination facts, never provider authority.
type CloseRepository struct{ store *Store }

func (s *Store) SessionClose() CloseRepository { return CloseRepository{s} }
func (r CloseRepository) Binding(ctx context.Context, subject, id string) (session.Binding, error) {
	return r.store.Presentation().Binding(ctx, subject, id)
}

type closeQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func findClose(ctx context.Context, q closeQuerier, subject, id, request string) (sessionclose.Attempt, bool, error) {
	var a sessionclose.Attempt
	var at string
	err := q.QueryRowContext(ctx, `SELECT attempt_id,request_sha256,operation_kind,interrupt,created_at,dispatch_finished,outcome_status,operation_ref,code,next_action FROM session_close_attempts WHERE subject_id=? AND session_id=? AND request_id=?`, subject, id, request).Scan(&a.ID, &a.RequestSHA256, &a.Kind, &a.Interrupt, &at, &a.DispatchFinished, &a.Outcome.Status, &a.Outcome.OperationRef, &a.Outcome.Code, &a.Outcome.NextAction)
	if errors.Is(err, sql.ErrNoRows) {
		return a, false, nil
	}
	if err != nil {
		return a, false, err
	}
	a.SubjectID, a.SessionID, a.RequestID = subject, id, request
	a.Outcome.AttemptID = a.ID
	a.CreatedAt, err = time.Parse(time.RFC3339Nano, at)
	return a, true, err
}
func (r CloseRepository) Find(ctx context.Context, subject, id, request string) (sessionclose.Attempt, bool, error) {
	return findClose(ctx, r.store.reader, subject, id, request)
}

func checkCloseBinding(ctx context.Context, q closeQuerier, b session.Binding) error {
	var workspace, run, controller, provider string
	err := q.QueryRowContext(ctx, `SELECT workspace_id,run_id,controller_binding_id,provider_session_id FROM primary_session_bindings WHERE subject_id=? AND session_id=?`, b.SubjectID, b.ID).Scan(&workspace, &run, &controller, &provider)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return closePersistenceError(err)
	}
	if err != nil || workspace != b.WorkspaceID || run != b.RunID || controller != b.ControllerBindingID || provider != b.ProviderSessionID {
		return sessionclose.ErrConflict
	}
	return nil
}

func closePersistenceError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.Join(session.ErrPersistenceUnavailable, err)
}

func (r CloseRepository) Begin(ctx context.Context, b action.Bound, a sessionclose.Attempt) (sessionclose.Attempt, bool, error) {
	if a.ID == "" || a.RequestID == "" || a.SubjectID != b.Binding.SubjectID || a.SessionID != b.Binding.ID || a.CreatedAt.IsZero() || !sha256Hex.MatchString(a.RequestSHA256) || !closeKind(a.Kind) || b.Carrier.ControllerID == "" {
		return sessionclose.Attempt{}, false, sessionclose.ErrInvalid
	}
	tx, err := r.store.writer.Conn(ctx)
	if err != nil {
		return sessionclose.Attempt{}, false, err
	}
	defer tx.Close()
	if _, err = tx.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return sessionclose.Attempt{}, false, err
	}
	defer tx.ExecContext(context.Background(), "ROLLBACK")
	if err = checkCloseBinding(ctx, tx, b.Binding); err != nil {
		return sessionclose.Attempt{}, false, err
	}
	old, found, err := findClose(ctx, tx, a.SubjectID, a.SessionID, a.RequestID)
	if err != nil {
		return sessionclose.Attempt{}, false, err
	}
	if found {
		if old.RequestSHA256 != a.RequestSHA256 || old.Kind != a.Kind || old.Interrupt != a.Interrupt {
			return sessionclose.Attempt{}, false, sessionclose.ErrConflict
		}
		return old, false, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT p.controller_references,p.state,COALESCE(c.session_id,'') FROM provider_operation_attempts p LEFT JOIN session_close_attempts c ON c.attempt_id=p.operation_id AND c.subject_id=p.subject_id WHERE p.subject_id=? AND p.state IN ('pending','outcome_unknown')`, a.SubjectID)
	if err != nil {
		return sessionclose.Attempt{}, false, err
	}
	blocked := false
	for rows.Next() {
		var encoded, state, closeSession string
		if err = rows.Scan(&encoded, &state, &closeSession); err != nil {
			rows.Close()
			return sessionclose.Attempt{}, false, err
		}
		var refs []ControllerReference
		if json.Unmarshal([]byte(encoded), &refs) != nil {
			blocked = true
			break
		}
		matches := len(refs) == 0
		for _, ref := range refs {
			matches = matches || ref.BindingID == b.Binding.ID || ref.BindingID == b.Binding.ControllerBindingID || ref.ExpectedControllerID == b.Carrier.ControllerID
		}
		// Explicit recovery may inspect the same Controller even while an older
		// mutation outcome is unknown. A pending call, an unscoped legacy row,
		// or an ordinary Close still owns the overlap lock.
		recoverable := state == "outcome_unknown" && a.Kind != sessionclose.Close &&
			(closeSession == a.SessionID || closeSession == "" && len(refs) == 1 &&
				refs[0].BindingID == b.Binding.ControllerBindingID && refs[0].ExpectedControllerID == b.Carrier.ControllerID)
		if matches && !recoverable {
			blocked = true
			break
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return sessionclose.Attempt{}, false, err
	}
	if blocked {
		return sessionclose.Attempt{}, false, sessionclose.ErrConflict
	}
	refs, _ := json.Marshal([]ControllerReference{{Role: "source", ExpectedControllerID: b.Carrier.ControllerID, BindingID: b.Binding.ControllerBindingID}})
	_, err = tx.ExecContext(ctx, `INSERT INTO provider_operation_attempts(operation_id,subject_id,operation_kind,request_sha256,replay_available,controller_references,state,created_at) VALUES (?,?,?,?,0,?,'pending',?)`, a.ID, a.SubjectID, a.Kind, a.RequestSHA256, string(refs), timestamp(a.CreatedAt))
	if err != nil {
		return sessionclose.Attempt{}, false, err
	}
	deadline := sessionclose.CloseTimeout
	if a.Kind != sessionclose.Close {
		deadline = sessionclose.RecoveryTimeout
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mutation_attempt_details(operation_id,target_ref,deadline_at,reconciliation_route) VALUES (?,?,?,?)`, a.ID, a.SessionID, timestamp(a.CreatedAt.Add(deadline)), "aggregate_reads")
	if err != nil {
		return sessionclose.Attempt{}, false, err
	}
	a.DispatchFinished = false
	a.Outcome = sessionclose.Outcome{Status: sessionclose.InProgress, AttemptID: a.ID, NextAction: "WAIT"}
	_, err = tx.ExecContext(ctx, `INSERT INTO session_close_attempts(attempt_id,subject_id,session_id,request_id,request_sha256,operation_kind,interrupt,created_at,dispatch_finished,outcome_status,operation_ref,code,next_action) VALUES (?,?,?,?,?,?,?,?,0,?,'','','WAIT')`, a.ID, a.SubjectID, a.SessionID, a.RequestID, a.RequestSHA256, a.Kind, a.Interrupt, timestamp(a.CreatedAt), a.Outcome.Status)
	if err != nil {
		return sessionclose.Attempt{}, false, err
	}
	_, err = tx.ExecContext(ctx, "COMMIT")
	return a, err == nil, err
}

func closeKind(k sessionclose.Kind) bool {
	return k == sessionclose.Close || k == sessionclose.Recover || k == sessionclose.Reconcile
}
func validCloseOutcome(o sessionclose.Outcome) bool {
	switch o.Status {
	case sessionclose.Rejected, sessionclose.InProgress, sessionclose.Confirmed, sessionclose.OutcomeUnknown, sessionclose.RecoveryRequired:
	default:
		return false
	}
	// Only closed, Gul-owned codes cross the persistence boundary.
	switch o.Code {
	case "", "TRANSPORT_UNAVAILABLE", "DEADLINE_EXCEEDED", "PROTOCOL_INCOMPATIBLE", "CONTROLLER_MISMATCH", "CONTROLLER_CARRIER_INVALID", "WRITE_CONTINUATION_CONTROLLER_INVALID", "WRITER_CONFLICT", "THREADLESS_REQUIRES_WRITE_TURN", "INTERACTION_STALE", "INTERACTION_ALREADY_RESOLVED", "RECOVERY_REQUIRED", "OUTCOME_UNKNOWN", "SLOW_CONSUMER", "ARTIFACT_UNAVAILABLE", "UNSUPPORTED_PATH_ENCODING", "RUN_STATE_CONFLICT", "RPC_SERVER_ALREADY_RUNNING", "OPERATOR_ACTION_REQUIRED", "INVALID_PAGE_TOKEN", "PAGE_TOKEN_EXPIRED", "INVALID_REQUEST", "UNAUTHORIZED", "SOURCE_UNAVAILABLE", "LIMIT_EXCEEDED", "PROVIDER_BLOCKED", "RUNTIME_PATH_UNAVAILABLE", "WORKSPACE_SELECTION_UNAVAILABLE", "WORKSPACE_NOT_PROVISIONED", "PROFILE_MISSING", "PROFILE_SERVER_UNAVAILABLE", "WORKSPACE_IDENTITY_MISMATCH", "PERSISTENCE_UNAVAILABLE":
	default:
		return false
	}
	switch o.NextAction {
	case "", "ABORT", "FIX_REQUEST", "REFRESH_CAPABILITIES", "REFRESH_SNAPSHOT", "VERIFY_CONTROLLER", "USE_COMPATIBLE_CONTROLLER", "USE_NEW_SAME_PRINCIPAL_CONTROLLER", "USE_SUPPORTED_PROFILE", "USE_TERMINAL_SOURCE", "SUBMIT_WRITE_TURN", "WAIT", "RETRY_EXACT_IDEMPOTENCY_KEY", "REFETCH_INTERACTION", "RECONCILE_RUN", "RECOVER_RUN", "RECONNECT_FROM_COMMITTED_CURSOR", "RESTART_GATEWAY", "OPERATOR_REPAIR", "FIX_SOCKET_PATH":
	default:
		return false
	}
	return true
}
func (r CloseRepository) Save(ctx context.Context, a sessionclose.Attempt) error {
	if !validCloseOutcome(a.Outcome) || a.Outcome.AttemptID != a.ID {
		return sessionclose.ErrInvalid
	}
	tx, err := r.store.writer.Conn(ctx)
	if err != nil {
		return err
	}
	defer tx.Close()
	if _, err = tx.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer tx.ExecContext(context.Background(), "ROLLBACK")
	old, found, err := findClose(ctx, tx, a.SubjectID, a.SessionID, a.RequestID)
	if err != nil {
		return err
	}
	if !found || old.ID != a.ID || old.RequestSHA256 != a.RequestSHA256 || old.Kind != a.Kind || old.Interrupt != a.Interrupt {
		return sessionclose.ErrConflict
	}
	if old.Outcome.Status == sessionclose.Confirmed || old.Outcome.Status == sessionclose.Rejected {
		return nil
	}
	if !a.DispatchFinished {
		return sessionclose.ErrConflict
	}
	if old.Outcome.OperationRef != "" {
		a.Outcome.OperationRef = old.Outcome.OperationRef
	}
	if a.Outcome.OperationRef != "" {
		var present int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_close_operations WHERE subject_id=? AND session_id=? AND operation_ref=?`, a.SubjectID, a.SessionID, a.Outcome.OperationRef).Scan(&present); err != nil {
			return err
		}
		if present != 1 {
			return sessionclose.ErrInvalid
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE session_close_attempts SET dispatch_finished=1,outcome_status=?,operation_ref=?,code=?,next_action=? WHERE attempt_id=? AND subject_id=? AND session_id=? AND request_id=? AND request_sha256=? AND operation_kind=?`, a.Outcome.Status, a.Outcome.OperationRef, a.Outcome.Code, a.Outcome.NextAction, a.ID, a.SubjectID, a.SessionID, a.RequestID, a.RequestSHA256, a.Kind)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sessionclose.ErrConflict
	}
	state := "outcome_unknown"
	if a.Outcome.Status == sessionclose.Rejected || a.Outcome.Status == sessionclose.Confirmed {
		state = "resolved"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE provider_operation_attempts SET state=? WHERE operation_id=? AND subject_id=? AND state!='resolved'`, state, a.ID, a.SubjectID); err != nil {
		return err
	}
	if a.Outcome.Status == sessionclose.Confirmed && a.Kind == sessionclose.Close {
		if _, err = tx.ExecContext(ctx, `UPDATE provider_operation_attempts SET state='resolved' WHERE subject_id=? AND state='outcome_unknown' AND operation_id IN (SELECT attempt_id FROM session_close_attempts WHERE subject_id=? AND session_id=?)`, a.SubjectID, a.SubjectID, a.SessionID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, "COMMIT")
	return err
}

func (r PresentationRepository) OperationReference(ctx context.Context, b session.Binding, providerID string) (string, error) {
	return r.store.SessionClose().OperationReference(ctx, b, providerID)
}
func (r CloseRepository) OperationReference(ctx context.Context, b session.Binding, providerID string) (string, error) {
	if !session.ValidCloseOperationID(providerID) {
		return "", sessionclose.ErrInvalid
	}
	tx, err := r.store.writer.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Close()
	if _, err = tx.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return "", err
	}
	defer tx.ExecContext(context.Background(), "ROLLBACK")
	if err = checkCloseBinding(ctx, tx, b); err != nil {
		return "", err
	}
	var bytes [24]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		return "", err
	}
	ref := "close_" + hex.EncodeToString(bytes[:])
	_, err = tx.ExecContext(ctx, `INSERT INTO session_close_operations(subject_id,session_id,controller_binding_id,provider_operation_id,operation_ref) VALUES (?,?,?,?,?) ON CONFLICT(subject_id,session_id,controller_binding_id,provider_operation_id) DO NOTHING`, b.SubjectID, b.ID, b.ControllerBindingID, providerID, ref)
	if err != nil {
		return "", err
	}
	err = tx.QueryRowContext(ctx, `SELECT operation_ref FROM session_close_operations WHERE subject_id=? AND session_id=? AND controller_binding_id=? AND provider_operation_id=?`, b.SubjectID, b.ID, b.ControllerBindingID, providerID).Scan(&ref)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, "COMMIT")
	return ref, err
}

var _ sessionclose.Repository = CloseRepository{}

// Pending selects only completed transmissions. A live provider dispatch retains
// its exclusive blocker until its owner persists the response or uncertainty.
func (r CloseRepository) Pending(ctx context.Context, subject, id string) ([]sessionclose.Attempt, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT c.request_id FROM session_close_attempts c JOIN provider_operation_attempts p ON p.operation_id=c.attempt_id WHERE c.subject_id=? AND c.session_id=? AND c.dispatch_finished=1 AND p.state='outcome_unknown' ORDER BY c.created_at,c.attempt_id LIMIT 257`, subject, id)
	if err != nil {
		return nil, err
	}
	var requests []string
	for rows.Next() {
		var request string
		if err := rows.Scan(&request); err != nil {
			rows.Close()
			return nil, err
		}
		requests = append(requests, request)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(requests) > 256 {
		return nil, sessionclose.ErrUnavailable
	}
	attempts := make([]sessionclose.Attempt, 0, len(requests))
	for _, request := range requests {
		a, found, err := r.Find(ctx, subject, id, request)
		if err != nil {
			return nil, err
		}
		if found {
			attempts = append(attempts, a)
		}
	}
	return attempts, nil
}
