package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/interrupt"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/operation"
)

type InterruptAttemptRepository struct{ store *Store }

func (s *Store) InterruptAttempts() InterruptAttemptRepository { return InterruptAttemptRepository{s} }
func interruptID(b action.Bound, request string) string {
	h := sha256.Sum256([]byte(b.Binding.SubjectID + "\x00" + b.Binding.ID + "\x00" + request))
	return "interrupt_" + hex.EncodeToString(h[:])
}
func interruptAttempt(a MutationAttempt, b action.Bound) (interrupt.Attempt, error) {
	if a.Kind != "InterruptTurn" || a.SubjectID != b.Binding.SubjectID || a.TargetRef != b.Binding.RunID || a.InterruptTurnID == "" || a.InterruptRevisionBefore == 0 ||
		a.InterruptHeadBefore == "" || a.ReplayAvailable || len(a.ControllerReferences) != 1 || a.ControllerReferences[0].BindingID != b.Binding.ControllerBindingID || a.ControllerReferences[0].ExpectedControllerID != b.Carrier.ControllerID {
		return interrupt.Attempt{}, ErrMutationConflict
	}
	return interrupt.Attempt{ID: a.OperationID, TurnID: a.InterruptTurnID, Head: a.InterruptHeadBefore, Revision: a.InterruptRevisionBefore, State: a.State, Outcome: a.OutcomeRef, DispatchFinished: a.InterruptDispatchFinished}, nil
}
func (r InterruptAttemptRepository) Find(ctx context.Context, b action.Bound, request string) (interrupt.Attempt, bool, error) {
	a, err := r.store.Attempts().Mutation(ctx, interruptID(b, request))
	if errors.Is(err, operation.ErrNotFound) {
		return interrupt.Attempt{}, false, nil
	}
	if err != nil {
		return interrupt.Attempt{}, false, err
	}
	out, err := interruptAttempt(a, b)
	return out, err == nil, err
}
func (r InterruptAttemptRepository) Begin(ctx context.Context, b action.Bound, request, turn string, stamp observation.Stamp) (interrupt.Attempt, bool, error) {
	if b.Binding.SubjectID == "" || b.Binding.ID == "" || b.Binding.RunID == "" || b.Binding.ControllerBindingID == "" || b.Carrier.ControllerID == "" || request == "" || turn == "" || !stamp.Valid() {
		return interrupt.Attempt{}, false, action.ErrInvalid
	}
	now := time.Now().UTC()
	digest := sha256.Sum256([]byte(fmt.Sprintf("InterruptTurn\x00%s\x00%s\x00%s\x00%s\x00%d", b.Binding.SubjectID, b.Binding.RunID, turn, b.Carrier.ControllerID, stamp.Run)))
	a, dispatch, err := r.store.Attempts().BeginMutation(ctx, MutationAttempt{OperationAttempt: OperationAttempt{
		OperationID: interruptID(b, request), SubjectID: b.Binding.SubjectID, Kind: "InterruptTurn", RequestSHA256: hex.EncodeToString(digest[:]), State: "pending", CreatedAt: now,
		ControllerReferences: []ControllerReference{{Role: "source", BindingID: b.Binding.ControllerBindingID, ExpectedControllerID: b.Carrier.ControllerID}}},
		TargetRef: b.Binding.RunID, DeadlineAt: now.Add(15 * time.Second), ReconciliationRoute: "exact_turn_terminal",
		InterruptTurnID: turn, InterruptRevisionBefore: stamp.Run, InterruptHeadBefore: string(stamp.Head)})
	if errors.Is(err, ErrMutationConflict) {
		return interrupt.Attempt{}, false, action.ErrOutcomeUnknown
	}
	if err != nil {
		return interrupt.Attempt{}, false, err
	}
	out, err := interruptAttempt(a, b)
	return out, dispatch, err
}
func (r InterruptAttemptRepository) Finish(ctx context.Context, id, outcome string) error {
	if outcome != "" && outcome != "accepted" && outcome != "rejected" && outcome != "state_observed" {
		return ErrMutationConflict
	}
	conn, err := r.store.writer.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	a, err := readMutation(ctx, conn, id)
	if err != nil || a.Kind != "InterruptTurn" {
		return errors.Join(err, ErrMutationConflict)
	}
	state := "outcome_unknown"
	if outcome != "" {
		state = "resolved"
	}
	if a.State == "resolved" {
		if state != "resolved" || a.OutcomeRef != outcome {
			return ErrMutationConflict
		}
		return nil
	}
	if outcome == "state_observed" && (!a.InterruptDispatchFinished || a.State != "outcome_unknown") {
		return ErrMutationConflict
	}
	if _, err = conn.ExecContext(ctx, `UPDATE provider_operation_attempts SET state=? WHERE operation_id=?`, state, id); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `UPDATE mutation_attempt_details SET outcome_ref=? WHERE operation_id=?`, outcome, id); err != nil {
		return err
	}
	result, err := conn.ExecContext(ctx, `UPDATE interrupt_attempt_details SET dispatch_finished=1 WHERE operation_id=?`, id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(err, ErrMutationConflict)
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
func (r InterruptAttemptRepository) Pending(ctx context.Context, b action.Bound) ([]interrupt.Attempt, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT p.operation_id FROM provider_operation_attempts p
 JOIN mutation_attempt_details d ON d.operation_id=p.operation_id JOIN interrupt_attempt_details i ON i.operation_id=p.operation_id
 WHERE p.subject_id=? AND d.target_ref=? AND p.operation_kind='InterruptTurn' AND p.state='outcome_unknown' AND i.dispatch_finished=1 LIMIT 257`, b.Binding.SubjectID, b.Binding.RunID)
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
	if len(ids) > 256 {
		return nil, action.ErrPersistence
	}
	var out []interrupt.Attempt
	for _, id := range ids {
		a, err := r.store.Attempts().Mutation(ctx, id)
		if err != nil {
			return nil, err
		}
		v, err := interruptAttempt(a, b)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

var _ interrupt.Repository = InterruptAttemptRepository{}
