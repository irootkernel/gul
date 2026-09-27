package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/observation"
)

type ObservationRepository struct{ store *Store }

func (s *Store) Observation() ObservationRepository { return ObservationRepository{s} }

type observationQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r ObservationRepository) binding(ctx context.Context, db observationQuery, b observation.Binding) error {
	if !b.Valid() {
		return observation.ErrUnbound
	}
	var run, workspace, root string
	err := db.QueryRowContext(ctx, `SELECT b.run_id, a.provider_workspace_id, a.canonical_root
FROM primary_session_bindings b JOIN workspace_attachments a ON a.subject_id=b.subject_id AND a.workspace_id=b.workspace_id
WHERE b.subject_id=? AND b.session_id=?`, b.SubjectID, b.SessionID).Scan(&run, &workspace, &root)
	if err != nil || run != b.RunID || workspace != b.WorkspaceID || root != b.AbsoluteRoot {
		return observation.ErrUnbound
	}
	return nil
}
func (r ObservationRepository) Checkpoint(ctx context.Context, b observation.Binding) (observation.Checkpoint, error) {
	if err := r.binding(ctx, r.store.reader, b); err != nil {
		return observation.Checkpoint{}, err
	}
	var cp observation.Checkpoint
	var encoded sql.NullString
	err := r.store.reader.QueryRowContext(ctx, `SELECT c.last_validated_cursor,c.last_committed_cursor,s.stamp
FROM observation_checkpoints c LEFT JOIN observation_checkpoint_stamps s USING(provider_id,runtime_object_id,stream_kind)
WHERE c.provider_id=? AND c.runtime_object_id=? AND c.stream_kind='run_events'`, b.ProviderID, b.RunID).Scan(&cp.Validated, &cp.Committed, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return observation.Checkpoint{Validated: "0", Committed: "0"}, nil
	}
	if err != nil {
		return observation.Checkpoint{}, err
	}
	if encoded.Valid {
		if err := json.Unmarshal([]byte(encoded.String), &cp.Stamp); err != nil {
			return observation.Checkpoint{}, err
		}
	}
	return cp, nil
}
func (r ObservationRepository) Validate(ctx context.Context, b observation.Binding, c observation.Cursor) error {
	if !c.Valid() {
		return observation.ErrInvalid
	}
	if err := r.binding(ctx, r.store.reader, b); err != nil {
		return err
	}
	_, err := r.store.writer.ExecContext(ctx, `INSERT INTO observation_checkpoints(provider_id,runtime_object_id,stream_kind,last_validated_cursor,last_committed_cursor)
VALUES (?,?,'run_events',?,'0') ON CONFLICT(provider_id,runtime_object_id,stream_kind) DO UPDATE SET last_validated_cursor=excluded.last_validated_cursor
WHERE length(excluded.last_validated_cursor)>length(last_validated_cursor) OR (length(excluded.last_validated_cursor)=length(last_validated_cursor) AND excluded.last_validated_cursor>=last_validated_cursor)`, b.ProviderID, b.RunID, string(c))
	return err
}
func (r ObservationRepository) Commit(ctx context.Context, in observation.Invalidation) (observation.Notification, error) {
	var n observation.Notification
	b := in.Binding
	if !in.Cursor.Valid() || !in.Floor.Valid() || in.Cursor != in.Floor.Head || in.At.IsZero() || in.CorrelationID == "" {
		return n, observation.ErrInvalid
	}
	tx, err := r.store.writer.Conn(ctx)
	if err != nil {
		return n, err
	}
	defer tx.Close()
	if _, err = tx.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return n, err
	}
	defer tx.ExecContext(context.Background(), "ROLLBACK")
	if err := r.binding(ctx, tx, b); err != nil {
		return n, err
	}
	var validated, committed observation.Cursor
	if err := tx.QueryRowContext(ctx, `SELECT last_validated_cursor,last_committed_cursor FROM observation_checkpoints WHERE provider_id=? AND runtime_object_id=? AND stream_kind='run_events'`, b.ProviderID, b.RunID).Scan(&validated, &committed); err != nil {
		return n, err
	}
	if !validated.Valid() || !committed.Valid() || in.Cursor.Compare(validated) > 0 || in.Cursor.Compare(committed) <= 0 {
		return n, observation.ErrInvalid
	}
	if committed != "0" {
		var encoded string
		if err := tx.QueryRowContext(ctx, `SELECT stamp FROM observation_checkpoint_stamps WHERE provider_id=? AND runtime_object_id=? AND stream_kind='run_events'`, b.ProviderID, b.RunID).Scan(&encoded); err != nil {
			return n, observation.ErrRefresh
		}
		var previous observation.Stamp
		if json.Unmarshal([]byte(encoded), &previous) != nil || !previous.Valid() || previous.Head != committed || !in.Floor.Covers(previous) {
			return n, observation.ErrInvalid
		}
	}
	floor, err := json.Marshal(ProjectionStamp{CapturedHeadCursor: string(in.Floor.Head), RunStateRevision: in.Floor.Run, WriterStateRevision: in.Floor.Writer, InteractionStateRevision: in.Floor.Interaction})
	if err != nil {
		return n, err
	}
	for _, aggregate := range []struct {
		mask observation.Refresh
		name string
	}{{observation.Run, "run"}, {observation.Writer, "writer"}, {observation.Interaction, "interaction"}} {
		if in.Refresh&aggregate.mask == 0 {
			continue
		}
		// Preserve the cached stamp: the event is an invalidation, not a snapshot.
		_, err = tx.ExecContext(ctx, `INSERT INTO runtime_projection_cache(subject_id,session_id,aggregate_kind,projection_stamp,freshness,invalidation_floor)
VALUES (?,?,?,?,'stale',?) ON CONFLICT(subject_id,session_id,aggregate_kind) DO UPDATE SET freshness='stale',invalidation_floor=excluded.invalidation_floor`, b.SubjectID, b.SessionID, aggregate.name, string(floor), string(floor))
		if err != nil {
			return n, err
		}
	}
	// Pending mandatory reads remain metadata, never authoritative runtime state.
	_, err = tx.ExecContext(ctx, `INSERT INTO observation_refreshes(subject_id,session_id,refresh_mask) VALUES (?,?,?)
ON CONFLICT(subject_id,session_id) DO UPDATE SET refresh_mask=refresh_mask|excluded.refresh_mask`, b.SubjectID, b.SessionID, uint16(in.Refresh))
	if err != nil {
		return n, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE observation_checkpoints SET last_committed_cursor=? WHERE provider_id=? AND runtime_object_id=? AND stream_kind='run_events'`, string(in.Cursor), b.ProviderID, b.RunID)
	if err != nil {
		return n, err
	}
	stamp, err := json.Marshal(in.Floor)
	if err != nil {
		return n, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO observation_checkpoint_stamps(provider_id,runtime_object_id,stream_kind,stamp) VALUES (?,?,'run_events',?)
ON CONFLICT(provider_id,runtime_object_id,stream_kind) DO UPDATE SET stamp=excluded.stamp`, b.ProviderID, b.RunID, string(stamp))
	if err != nil {
		return n, err
	}
	// Allocation follows projection commit work and shares its transaction.
	if err = tx.QueryRowContext(ctx, `UPDATE client_delivery_counter SET next_sequence=next_sequence+1 WHERE id=1 RETURNING next_sequence`).Scan(&n.Sequence); err != nil {
		return n, err
	}
	n.SessionID = b.SessionID
	n.CorrelationID = in.CorrelationID
	n.Kind = "projection_invalidated"
	n.CreatedAt = in.At
	_, err = tx.ExecContext(ctx, `INSERT INTO client_event_journal(sequence,subject_id,event_kind,created_at) VALUES (?,?,?,?)`, int64(n.Sequence), b.SubjectID, n.Kind, timestamp(n.CreatedAt))
	if err != nil {
		return n, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO client_projection_notifications(sequence,session_id,correlation_id) VALUES (?,?,?)`, int64(n.Sequence), n.SessionID, n.CorrelationID)
	if err != nil {
		return n, err
	}
	if _, err = tx.ExecContext(ctx, "COMMIT"); err != nil {
		return observation.Notification{}, err
	}
	return n, nil
}

var _ observation.Repository = ObservationRepository{}

// ReadDelivery captures one bounded page in a read transaction. Excess history
// becomes a snapshot request; no provider operation is replayed by delivery.
func (r ObservationRepository) ReadDelivery(ctx context.Context, subject, sessionID string, after observation.Sequence) (observation.DeliveryBatch, error) {
	var batch observation.DeliveryBatch
	if subject == "" || sessionID == "" || after < 0 {
		return batch, observation.ErrInvalid
	}
	tx, err := r.store.reader.BeginTx(ctx, nil)
	if err != nil {
		return batch, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM primary_session_bindings WHERE subject_id=? AND session_id=?`, subject, sessionID).Scan(&exists); err != nil {
		return batch, observation.ErrUnbound
	}
	if err := tx.QueryRowContext(ctx, `SELECT next_sequence FROM client_delivery_counter WHERE id=1`).Scan(&batch.Head); err != nil {
		return batch, err
	}
	if after > batch.Head {
		return batch, observation.ErrInvalid
	}
	rows, err := tx.QueryContext(ctx, `SELECT j.sequence,n.session_id,n.correlation_id,j.event_kind,j.created_at
FROM client_event_journal j JOIN client_projection_notifications n USING(sequence)
WHERE j.subject_id=? AND n.session_id=? AND j.sequence>? AND j.sequence<=? ORDER BY j.sequence LIMIT ?`, subject, sessionID, int64(after), int64(batch.Head), observation.MaximumReplay+1)
	if err != nil {
		return batch, err
	}
	defer rows.Close()
	for rows.Next() {
		var n observation.Notification
		var at string
		if err := rows.Scan(&n.Sequence, &n.SessionID, &n.CorrelationID, &n.Kind, &at); err != nil {
			return batch, err
		}
		n.CreatedAt, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return batch, err
		}
		batch.Events = append(batch.Events, n)
	}
	if err := rows.Err(); err != nil {
		return batch, err
	}
	if len(batch.Events) > observation.MaximumReplay {
		batch.Events = nil
		batch.SnapshotRequired = true
	}
	return batch, nil
}
