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

type CloseRefreshRepository struct{ store *Store }

func (s *Store) CloseRefresh() CloseRefreshRepository { return CloseRefreshRepository{s} }

func closeStamp(s observation.Stamp) ProjectionStamp {
	return ProjectionStamp{CapturedHeadCursor: string(s.Head), RunStateRevision: s.Run, WriterStateRevision: s.Writer, InteractionStateRevision: s.Interaction}
}
func observationStamp(s ProjectionStamp) observation.Stamp {
	return observation.Stamp{Head: observation.Cursor(s.CapturedHeadCursor), Run: s.RunStateRevision, Writer: s.WriterStateRevision, Interaction: s.InteractionStateRevision}
}

func checkCloseRefreshBinding(ctx context.Context, tx *sql.Conn, b action.Bound) error {
	var run, ws, controller, providerSession, providerWorkspace, root string
	err := tx.QueryRowContext(ctx, `SELECT b.run_id,b.workspace_id,b.controller_binding_id,b.provider_session_id,a.provider_workspace_id,a.canonical_root
FROM primary_session_bindings b JOIN workspace_attachments a ON a.subject_id=b.subject_id AND a.workspace_id=b.workspace_id
WHERE b.subject_id=? AND b.session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&run, &ws, &controller, &providerSession, &providerWorkspace, &root)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return closePersistenceError(err)
	}
	if err != nil || run != b.Binding.RunID || ws != b.Binding.WorkspaceID || controller != b.Binding.ControllerBindingID || providerSession != b.Binding.ProviderSessionID || providerWorkspace != b.Workspace.ProviderID || root != b.Workspace.CanonicalRoot {
		return sessionclose.ErrUnavailable
	}
	return nil
}

func (r CloseRefreshRepository) BeginRefresh(ctx context.Context, b action.Bound, stamp observation.Stamp) (observation.Cursor, error) {
	if !stamp.Valid() {
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
	if err = checkCloseRefreshBinding(ctx, tx, b); err != nil {
		return "", err
	}
	after := observation.Cursor("0")
	err = tx.QueryRowContext(ctx, `SELECT captured_head_cursor FROM runtime_timeline_cache WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&after)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if !after.Valid() {
		return "", sessionclose.ErrUnavailable
	}
	encoded, _ := json.Marshal(closeStamp(stamp))
	for _, kind := range []string{"run", "writer", "interaction"} {
		floor := stamp
		var oldFloor string
		err := tx.QueryRowContext(ctx, `SELECT invalidation_floor FROM runtime_projection_cache WHERE subject_id=? AND session_id=? AND aggregate_kind=?`, b.Binding.SubjectID, b.Binding.ID, kind).Scan(&oldFloor)
		if err == nil {
			var previous ProjectionStamp
			if json.Unmarshal([]byte(oldFloor), &previous) != nil {
				return "", sessionclose.ErrUnavailable
			}
			old := observationStamp(previous)
			if !old.Valid() {
				return "", sessionclose.ErrUnavailable
			}
			if old.Head.Compare(floor.Head) > 0 {
				floor.Head = old.Head
			}
			floor.Run = max(floor.Run, old.Run)
			floor.Writer = max(floor.Writer, old.Writer)
			floor.Interaction = max(floor.Interaction, old.Interaction)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		floorJSON, _ := json.Marshal(closeStamp(floor))
		freshness := "stale"
		if kind == "run" && stamp.Covers(floor) {
			freshness = "fresh"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO runtime_projection_cache(subject_id,session_id,aggregate_kind,projection_stamp,freshness,invalidation_floor)
VALUES(?,?,?,?,?,?) ON CONFLICT(subject_id,session_id,aggregate_kind) DO UPDATE SET
projection_stamp=CASE WHEN excluded.aggregate_kind='run' AND excluded.freshness='fresh' THEN excluded.projection_stamp ELSE projection_stamp END,
freshness=excluded.freshness,invalidation_floor=excluded.invalidation_floor`, b.Binding.SubjectID, b.Binding.ID, kind, string(encoded), freshness, string(floorJSON))
		if err != nil {
			return "", err
		}
	}
	mask := observation.Writer | observation.Interaction | observation.Timeline | observation.Artifacts | observation.Session
	if _, err = tx.ExecContext(ctx, `INSERT INTO observation_refreshes(subject_id,session_id,refresh_mask) VALUES(?,?,?)
ON CONFLICT(subject_id,session_id) DO UPDATE SET refresh_mask=refresh_mask|excluded.refresh_mask`, b.Binding.SubjectID, b.Binding.ID, uint16(mask)); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "COMMIT"); err != nil {
		return "", err
	}
	return after, nil
}

func (r CloseRefreshRepository) CompleteRefresh(ctx context.Context, b action.Bound, kind observation.Refresh, stamp observation.Stamp) error {
	writerOnly := kind == observation.Writer
	if !stamp.Valid() && !(writerOnly && stamp.Head == "" && stamp.Run == 0 && stamp.Interaction == 0) {
		return sessionclose.ErrInvalid
	}
	aggregate := ""
	switch kind {
	case observation.Run:
		aggregate = "run"
	case observation.Writer:
		aggregate = "writer"
	case observation.Interaction:
		aggregate = "interaction"
	case observation.Timeline, observation.Artifacts, observation.Session:
	default:
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
	if err = checkCloseRefreshBinding(ctx, tx, b); err != nil {
		return err
	}
	// A late completion cannot clear an invalidation recorded while the read
	// was in flight. Complete stamps are checked, not just one revision number.
	// Workspace Writer status is the exception: an ownerless stamp has only a
	// Writer revision, and another owner's Run/Interaction fields are in that
	// other Run's domain. Preserve those native fields without synthesizing a
	// target Run stamp; only the Workspace Writer revision establishes freshness.
	for _, name := range []string{"run", "writer", "interaction"} {
		var encoded, cached, freshness string
		if err = tx.QueryRowContext(ctx, `SELECT invalidation_floor,projection_stamp,freshness FROM runtime_projection_cache WHERE subject_id=? AND session_id=? AND aggregate_kind=?`, b.Binding.SubjectID, b.Binding.ID, name).Scan(&encoded, &cached, &freshness); err != nil {
			return err
		}
		var floor, previous ProjectionStamp
		if json.Unmarshal([]byte(encoded), &floor) != nil || !observationStamp(floor).Valid() {
			return sessionclose.ErrUnavailable
		}
		if writerOnly && stamp.Writer < floor.WriterStateRevision || !writerOnly && !stamp.Covers(observationStamp(floor)) {
			return sessionclose.ErrUnavailable
		}
		if name == aggregate && freshness == "fresh" {
			if json.Unmarshal([]byte(cached), &previous) != nil || writerOnly && stamp.Writer < previous.WriterStateRevision || !writerOnly && !stamp.Covers(observationStamp(previous)) {
				return sessionclose.ErrUnavailable
			}
		}
	}
	if aggregate != "" {
		encoded, _ := json.Marshal(closeStamp(stamp))
		_, err = tx.ExecContext(ctx, `UPDATE runtime_projection_cache SET projection_stamp=?,freshness='fresh' WHERE subject_id=? AND session_id=? AND aggregate_kind=?`, string(encoded), b.Binding.SubjectID, b.Binding.ID, aggregate)
	} else if kind == observation.Timeline {
		var current observation.Cursor
		err = tx.QueryRowContext(ctx, `SELECT captured_head_cursor FROM runtime_timeline_cache WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && !current.Valid() {
			return sessionclose.ErrUnavailable
		}
		if errors.Is(err, sql.ErrNoRows) || stamp.Head.Compare(current) >= 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO runtime_timeline_cache(subject_id,session_id,captured_head_cursor) VALUES(?,?,?)
ON CONFLICT(subject_id,session_id) DO UPDATE SET captured_head_cursor=excluded.captured_head_cursor`, b.Binding.SubjectID, b.Binding.ID, string(stamp.Head))
		} else {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE observation_refreshes SET refresh_mask=refresh_mask & ? WHERE subject_id=? AND session_id=?`, ^int64(kind), b.Binding.SubjectID, b.Binding.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "COMMIT")
	return err
}

var _ sessionclose.RefreshCache = CloseRefreshRepository{}
