package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/sessionclose"
)

// Converge publishes checked provider reads only while the binding and every
// event invalidation floor still match. A later event wins the writer lock and
// makes the aggregate stale again; an older read never clears its floor.
func (r ActionRepository) Converge(ctx context.Context, b action.Bound, in action.Input) (bool, error) {
	if r.providerID == "" {
		return false, action.ErrInvalid
	}
	run := in.Run.Stamp
	writer := in.Writer.Stamp
	if !in.Checked || !run.Valid() || in.InteractionStamp != run || in.TimelineHead != run.Head ||
		writer.Writer != run.Writer || (in.Writer.Owner == action.ThisSession && writer != run) {
		return false, nil
	}
	conn, err := r.store.writer.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return false, err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	if err = checkCloseRefreshBinding(ctx, conn, b); err != nil {
		if errors.Is(err, sessionclose.ErrUnavailable) {
			return false, action.ErrAuthority
		}
		return false, err
	}
	var committed string
	var encoded sql.NullString
	err = conn.QueryRowContext(ctx, `SELECT c.last_committed_cursor,s.stamp FROM observation_checkpoints c
LEFT JOIN observation_checkpoint_stamps s USING(provider_id,runtime_object_id,stream_kind)
WHERE c.provider_id=? AND c.runtime_object_id=? AND c.stream_kind='run_events'`, r.providerID, b.Binding.RunID).Scan(&committed, &encoded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var checkpoint observation.Stamp
	if err == nil && committed != "0" {
		if !encoded.Valid || json.Unmarshal([]byte(encoded.String), &checkpoint) != nil || !checkpoint.Valid() || checkpoint.Head != observation.Cursor(committed) {
			return false, observation.ErrRefresh
		}
	}
	if checkpoint.Valid() && (!run.Covers(checkpoint) || writer.Writer < checkpoint.Writer || in.TimelineHead.Compare(checkpoint.Head) < 0) {
		return false, nil
	}
	stamps := map[string]observation.Stamp{"run": run, "writer": writer, "interaction": in.InteractionStamp}
	for _, kind := range projectionKinds {
		stamp := stamps[kind]
		floor := checkpoint
		var oldStamp, oldFloor, freshness string
		err = conn.QueryRowContext(ctx, `SELECT projection_stamp,invalidation_floor,freshness FROM runtime_projection_cache
WHERE subject_id=? AND session_id=? AND aggregate_kind=?`, b.Binding.SubjectID, b.Binding.ID, kind).Scan(&oldStamp, &oldFloor, &freshness)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if err == nil {
			var previous, invalidated ProjectionStamp
			if json.Unmarshal([]byte(oldStamp), &previous) != nil || json.Unmarshal([]byte(oldFloor), &invalidated) != nil {
				return false, observation.ErrRefresh
			}
			old := observationStamp(previous)
			floor = observationStamp(invalidated)
			if !floor.Valid() || !coversProjectionFloor(kind, stamp, floor) ||
				freshness == "fresh" && !coversProjectionFloor(kind, stamp, old) {
				return false, nil
			}
			if checkpoint.Valid() && checkpoint.Covers(floor) {
				floor = checkpoint
			}
		} else if !floor.Valid() {
			floor = run
		}
		stampJSON, _ := json.Marshal(closeStamp(stamp))
		floorJSON, _ := json.Marshal(closeStamp(floor))
		if _, err = conn.ExecContext(ctx, `INSERT INTO runtime_projection_cache(subject_id,session_id,aggregate_kind,projection_stamp,freshness,invalidation_floor)
VALUES(?,?,?,?,'fresh',?) ON CONFLICT(subject_id,session_id,aggregate_kind) DO UPDATE SET
projection_stamp=excluded.projection_stamp,freshness='fresh',invalidation_floor=excluded.invalidation_floor`,
			b.Binding.SubjectID, b.Binding.ID, kind, string(stampJSON), string(floorJSON)); err != nil {
			return false, err
		}
	}
	var previousHead string
	err = conn.QueryRowContext(ctx, `SELECT captured_head_cursor FROM runtime_timeline_cache WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&previousHead)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	// Only the timeline refresher may advance this verified-progress cursor.
	// A snapshot head ahead of it still needs artifact/timeline verification.
	if errors.Is(err, sql.ErrNoRows) || !observation.Cursor(previousHead).Valid() || in.TimelineHead != observation.Cursor(previousHead) {
		return false, nil
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO observation_refreshes(subject_id,session_id,refresh_mask) VALUES(?,?,0)
ON CONFLICT(subject_id,session_id) DO UPDATE SET refresh_mask=refresh_mask & ?`, b.Binding.SubjectID, b.Binding.ID, ^int64(observation.Run|observation.Writer|observation.Interaction)); err != nil {
		return false, err
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return false, err
	}
	return true, nil
}
