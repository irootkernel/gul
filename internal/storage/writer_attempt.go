package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
)

type WriterAttemptRepository struct{ store *Store }

func (s *Store) WriterAttempts() WriterAttemptRepository { return WriterAttemptRepository{s} }

func (r WriterAttemptRepository) BeginWriter(ctx context.Context, b action.Bound, acquire bool, revision uint64, before action.WriterProjection) (string, error) {
	if b.Binding.SubjectID == "" || b.Binding.RunID == "" || b.Binding.ControllerBindingID == "" || b.Carrier.ControllerID == "" || revision == 0 || before.Revision == 0 {
		return "", action.ErrInvalid
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	id := "writer_" + hex.EncodeToString(random[:])
	kind := "ReleaseWriter"
	if acquire {
		kind = "AcquireWriter"
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", kind, b.Binding.SubjectID, b.Binding.RunID, b.Carrier.ControllerID, revision)))
	now := time.Now().UTC()
	_, dispatch, err := r.store.Attempts().BeginMutation(ctx, MutationAttempt{OperationAttempt: OperationAttempt{
		OperationID: id, SubjectID: b.Binding.SubjectID, Kind: kind, RequestSHA256: hex.EncodeToString(digest[:]),
		ControllerReferences: []ControllerReference{{Role: "source", BindingID: b.Binding.ControllerBindingID, ExpectedControllerID: b.Carrier.ControllerID}},
		State:                "pending", CreatedAt: now}, TargetRef: b.Binding.RunID, DeadlineAt: now.Add(15 * time.Second), ReconciliationRoute: "run_writer",
		WriterRevisionBefore: before.Revision, WriterGenerationBefore: before.Generation})
	if err != nil {
		if errors.Is(err, ErrMutationConflict) {
			return "", action.ErrOutcomeUnknown
		}
		return "", err
	}
	if !dispatch {
		return "", ErrMutationConflict
	}
	return id, nil
}

// ReconcileWriter settles only an unknown, finished tokenless call whose exact
// bound Run has a newer, fully converged Writer state with the intended owner.
// This records current authoritative state, not proof that the original RPC
// itself succeeded.
func (r WriterAttemptRepository) ReconcileWriter(ctx context.Context, b action.Bound, in action.Input) error {
	if !in.Checked || !in.ControllerMatches || in.Freshness != action.Fresh || in.Aggregate.Freshness != action.Fresh ||
		in.Writer.Reconciliation != action.NoReconciliation ||
		in.Writer.RecoveryBlocked || in.Writer.BackgroundBlocked || in.Run.Recovery != action.NoRecovery ||
		in.Run.Reconciliation != action.NoReconciliation || in.Writer.Revision == 0 || b.Carrier.ControllerID == "" {
		return nil
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
	if err := checkCloseRefreshBinding(ctx, conn, b); err != nil {
		return err
	}
	for kind, want := range map[string]observation.Stamp{"run": in.Run.Stamp, "writer": in.Writer.Stamp, "interaction": in.InteractionStamp} {
		var encoded, freshness string
		if err := conn.QueryRowContext(ctx, `SELECT projection_stamp,freshness FROM runtime_projection_cache
WHERE subject_id=? AND session_id=? AND aggregate_kind=?`, b.Binding.SubjectID, b.Binding.ID, kind).Scan(&encoded, &freshness); err != nil {
			return err
		}
		var stamp ProjectionStamp
		if json.Unmarshal([]byte(encoded), &stamp) != nil || freshness != "fresh" || observationStamp(stamp) != want {
			return nil
		}
	}
	var mask int64
	if err := conn.QueryRowContext(ctx, `SELECT refresh_mask FROM observation_refreshes WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&mask); err != nil {
		return err
	}
	if mask&int64(observation.AllAggregates) != 0 {
		return nil
	}
	var timeline string
	if err := conn.QueryRowContext(ctx, `SELECT captured_head_cursor FROM runtime_timeline_cache WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&timeline); err != nil {
		return err
	}
	if timeline != string(in.TimelineHead) {
		return nil
	}
	rows, err := conn.QueryContext(ctx, `SELECT p.operation_id FROM provider_operation_attempts p
JOIN mutation_attempt_details d ON d.operation_id=p.operation_id
JOIN writer_attempt_details w ON w.operation_id=p.operation_id
WHERE p.subject_id=? AND d.target_ref=? AND p.operation_kind IN ('AcquireWriter','ReleaseWriter')
AND p.state='outcome_unknown' AND w.dispatch_finished=1 ORDER BY p.created_at,p.operation_id LIMIT 257`, b.Binding.SubjectID, b.Binding.RunID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) > 256 {
		return action.ErrPersistence
	}
	for _, id := range ids {
		a, err := readMutation(ctx, conn, id)
		if err != nil {
			return err
		}
		if a.WriterRevisionBefore == 0 || len(a.ControllerReferences) != 1 || a.ControllerReferences[0].BindingID != b.Binding.ControllerBindingID ||
			a.ControllerReferences[0].ExpectedControllerID != b.Carrier.ControllerID ||
			in.Writer.Revision <= a.WriterRevisionBefore || in.Writer.Generation <= a.WriterGenerationBefore {
			continue
		}
		if a.Kind == "AcquireWriter" && in.Writer.Owner != action.ThisSession ||
			a.Kind == "ReleaseWriter" && in.Writer.Owner != action.NoOwner {
			continue
		}
		if _, err := conn.ExecContext(ctx, `UPDATE mutation_attempt_details SET outcome_ref=? WHERE operation_id=?`, "writer_state:"+strconv.FormatUint(in.Writer.Revision, 10), id); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE provider_operation_attempts SET state='resolved' WHERE operation_id=? AND state='outcome_unknown'`, id); err != nil {
			return err
		}
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

func (r WriterAttemptRepository) FinishWriter(ctx context.Context, id, outcome string) error {
	if id == "" || len(outcome) > 256 {
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
	a, err := readMutation(ctx, conn, id)
	if err != nil || a.Kind != "AcquireWriter" && a.Kind != "ReleaseWriter" {
		return errors.Join(err, ErrMutationConflict)
	}
	state := "outcome_unknown"
	if outcome != "" {
		state = "resolved"
	}
	if a.State == "resolved" && (state != "resolved" || a.OutcomeRef != outcome) {
		return ErrMutationConflict
	}
	if _, err := conn.ExecContext(ctx, `UPDATE provider_operation_attempts SET state=? WHERE operation_id=?`, state, id); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `UPDATE mutation_attempt_details SET outcome_ref=? WHERE operation_id=?`, outcome, id); err != nil {
		return err
	}
	result, err := conn.ExecContext(ctx, `UPDATE writer_attempt_details SET dispatch_finished=1 WHERE operation_id=?`, id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(err, ErrMutationConflict)
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

var _ action.MutationAttempts = WriterAttemptRepository{}
