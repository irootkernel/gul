package storage

import (
	"context"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
)

// MarkProviderUnavailable invalidates one bound session's projections during a
// provider-wide disconnect. Pending attempts are subject-scoped and all become
// unresolved; Run identity, committed cursor, and browser sequence survive.
func (s *Store) MarkProviderUnavailable(ctx context.Context, b action.Bound) error {
	conn, err := s.writer.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	if err = checkCloseRefreshBinding(ctx, conn, b); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `UPDATE runtime_projection_cache SET freshness='stale' WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO observation_refreshes(subject_id,session_id,refresh_mask) VALUES(?,?,?)
ON CONFLICT(subject_id,session_id) DO UPDATE SET refresh_mask=refresh_mask|excluded.refresh_mask`, b.Binding.SubjectID, b.Binding.ID, uint16(observation.AllAggregates)); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `UPDATE session_close_attempts SET outcome_status='outcome_unknown',next_action='REFRESH_SNAPSHOT'
WHERE subject_id=? AND outcome_status='in_progress' AND attempt_id IN
(SELECT operation_id FROM provider_operation_attempts WHERE subject_id=? AND state='pending')`, b.Binding.SubjectID, b.Binding.SubjectID); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `UPDATE provider_operation_attempts SET state='outcome_unknown' WHERE subject_id=? AND state='pending'`, b.Binding.SubjectID); err != nil {
		return err
	}
	if _, err = allocateNotification(ctx, conn, b.Binding.SubjectID, b.Binding.ID, "provider-unavailable", time.Now()); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

// MarkProviderRecovered wakes an existing browser tail after checked snapshots
// have converged and observation has resumed. It carries no provider payload.
func (s *Store) MarkProviderRecovered(ctx context.Context, b action.Bound) error {
	conn, err := s.writer.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	if err = checkCloseRefreshBinding(ctx, conn, b); err != nil {
		return err
	}
	if _, err = allocateNotification(ctx, conn, b.Binding.SubjectID, b.Binding.ID, "provider-recovered", time.Now()); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
