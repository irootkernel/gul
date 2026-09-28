package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/operation"
)

var ErrMutationConflict = errors.New("conflicting mutation attempt")

// MutationAttempt extends the common non-secret attempt with its recovery
// route. OutcomeRef is a validated provider identity, never history authority.
type MutationAttempt = operation.MutationAttempt

type mutationQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readMutation(ctx context.Context, q mutationQuerier, id string) (MutationAttempt, error) {
	var a MutationAttempt
	var replay sql.NullString
	var refs, created, deadline string
	err := q.QueryRowContext(ctx, `SELECT p.subject_id,p.operation_kind,p.request_sha256,p.replay_key,p.replay_available,
p.controller_references,p.state,p.created_at,d.target_ref,d.deadline_at,d.reconciliation_route,d.outcome_ref
FROM provider_operation_attempts p JOIN mutation_attempt_details d ON d.operation_id=p.operation_id WHERE p.operation_id=?`, id).
		Scan(&a.SubjectID, &a.Kind, &a.RequestSHA256, &replay, &a.ReplayAvailable, &refs, &a.State, &created,
			&a.TargetRef, &deadline, &a.ReconciliationRoute, &a.OutcomeRef)
	if err != nil {
		return MutationAttempt{}, err
	}
	a.OperationID, a.ReplayKey = id, replay.String
	if err := json.Unmarshal([]byte(refs), &a.ControllerReferences); err != nil {
		return MutationAttempt{}, err
	}
	a.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return MutationAttempt{}, err
	}
	a.DeadlineAt, err = time.Parse(time.RFC3339Nano, deadline)
	return a, err
}

func (r AttemptRepository) Mutation(ctx context.Context, id string) (MutationAttempt, error) {
	a, err := readMutation(ctx, r.store.reader, id)
	if errors.Is(err, sql.ErrNoRows) {
		return MutationAttempt{}, operation.ErrNotFound
	}
	return a, err
}

// BeginMutation serializes same-key browser retries and overlapping unknown
// Controller effects before any provider call. No transaction spans file or RPC I/O.
func (r AttemptRepository) BeginMutation(ctx context.Context, candidate MutationAttempt) (MutationAttempt, bool, error) {
	if err := validateAttempt(candidate.OperationAttempt); err != nil || candidate.TargetRef == "" || len(candidate.TargetRef) > 256 ||
		candidate.DeadlineAt.IsZero() || !candidate.DeadlineAt.After(candidate.CreatedAt) || candidate.ReconciliationRoute == "" || candidate.OutcomeRef != "" ||
		len(candidate.ControllerReferences) == 0 {
		return MutationAttempt{}, false, ErrMutationConflict
	}
	conn, err := r.store.writer.Conn(ctx)
	if err != nil {
		return MutationAttempt{}, false, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return MutationAttempt{}, false, err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	old, err := readMutation(ctx, conn, candidate.OperationID)
	if err == nil {
		if old.SubjectID != candidate.SubjectID || old.Kind != candidate.Kind || old.RequestSHA256 != candidate.RequestSHA256 ||
			old.TargetRef != candidate.TargetRef || old.ReconciliationRoute != candidate.ReconciliationRoute {
			return MutationAttempt{}, false, ErrMutationConflict
		}
		return old, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MutationAttempt{}, false, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT controller_references FROM provider_operation_attempts WHERE subject_id=? AND state IN ('pending','outcome_unknown')`, candidate.SubjectID)
	if err != nil {
		return MutationAttempt{}, false, err
	}
	conflict := false
	for rows.Next() {
		var encoded string
		if err = rows.Scan(&encoded); err != nil {
			break
		}
		var refs []ControllerReference
		if json.Unmarshal([]byte(encoded), &refs) != nil || len(refs) == 0 {
			conflict = true
			break
		}
		for _, existing := range refs {
			for _, proposed := range candidate.ControllerReferences {
				if existing.ExpectedControllerID == proposed.ExpectedControllerID ||
					existing.BindingID != "" && existing.BindingID == proposed.BindingID {
					conflict = true
				}
			}
		}
		if conflict {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return MutationAttempt{}, false, err
	}
	if conflict {
		return MutationAttempt{}, false, ErrMutationConflict
	}
	refs, err := json.Marshal(candidate.ControllerReferences)
	if err != nil {
		return MutationAttempt{}, false, err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO provider_operation_attempts
(operation_id,subject_id,operation_kind,request_sha256,replay_key,replay_available,controller_references,state,created_at)
VALUES (?,?,?,?,?,?,?,?,?)`, candidate.OperationID, candidate.SubjectID, candidate.Kind, candidate.RequestSHA256,
		nullIfEmpty(candidate.ReplayKey), candidate.ReplayAvailable, string(refs), candidate.State, timestamp(candidate.CreatedAt))
	if err != nil {
		return MutationAttempt{}, false, err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO mutation_attempt_details(operation_id,target_ref,deadline_at,reconciliation_route,outcome_ref)
VALUES (?,?,?,?,?)`, candidate.OperationID, candidate.TargetRef, timestamp(candidate.DeadlineAt), candidate.ReconciliationRoute, "")
	if err != nil {
		return MutationAttempt{}, false, err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return candidate, err == nil, err
}

// ResolveMutation records an exact response or authoritative reconciliation.
// The coordinator must first remove any durable replay file.
func (r AttemptRepository) ResolveMutation(ctx context.Context, id, outcomeRef string) error {
	if id == "" || outcomeRef == "" || len(outcomeRef) > 256 {
		return ErrMutationConflict
	}
	conn, err := r.store.writer.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	old, err := readMutation(ctx, conn, id)
	if err != nil {
		return err
	}
	if old.State == "resolved" {
		if old.OutcomeRef != outcomeRef {
			return ErrMutationConflict
		}
		return nil
	}
	if _, err := conn.ExecContext(ctx, `UPDATE mutation_attempt_details SET outcome_ref=? WHERE operation_id=?`, outcomeRef, id); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `UPDATE provider_operation_attempts SET state='resolved', replay_key=NULL,replay_available=0 WHERE operation_id=?`, id); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

func (r AttemptRepository) Replayable(ctx context.Context) ([]MutationAttempt, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT operation_id FROM provider_operation_attempts WHERE replay_available=1 ORDER BY created_at,operation_id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var attempts []MutationAttempt
	for _, id := range ids {
		a, err := r.Mutation(ctx, id)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, nil
}
