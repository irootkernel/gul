package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
)

// ActionRepository reads coordination metadata only. It never stores a writer
// owner, grants authority, or queues a writer transfer.
type ActionRepository struct {
	store      *Store
	providerID string
}

func (s *Store) Actions(providerID string) ActionRepository { return ActionRepository{s, providerID} }
func (r ActionRepository) Binding(ctx context.Context, subject, id string) (session.Binding, error) {
	return r.store.Presentation().Binding(ctx, subject, id)
}
func (r ActionRepository) LocalState(ctx context.Context, b action.Bound) (action.LocalState, error) {
	if r.providerID == "" {
		return action.LocalState{}, action.ErrInvalid
	}
	current, err := r.Binding(ctx, b.Binding.SubjectID, b.Binding.ID)
	if err != nil || current.RunID != b.Binding.RunID || current.WorkspaceID != b.Binding.WorkspaceID || current.ControllerBindingID != b.Binding.ControllerBindingID {
		return action.LocalState{}, action.ErrAuthority
	}
	cp, err := r.store.Observation().Checkpoint(ctx, observation.Binding{SubjectID: b.Binding.SubjectID, SessionID: b.Binding.ID, ProviderID: r.providerID, RunID: b.Binding.RunID, WorkspaceID: b.Workspace.ProviderID, AbsoluteRoot: b.Workspace.CanonicalRoot})
	if err != nil {
		return action.LocalState{}, err
	}
	out := action.LocalState{Ownership: action.OwnedSession, Operation: action.NoOperation, Floor: cp.Stamp}
	out.ProjectionsStale = true
	var run, writer, interaction observation.Stamp
	var freshCount int
	projectionRows, err := r.store.reader.QueryContext(ctx, `SELECT aggregate_kind,projection_stamp,invalidation_floor,freshness FROM runtime_projection_cache WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID)
	if err != nil {
		return action.LocalState{}, err
	}
	for projectionRows.Next() {
		var kind, encoded, floorJSON, freshness string
		if err = projectionRows.Scan(&kind, &encoded, &floorJSON, &freshness); err != nil {
			break
		}
		var value, floor ProjectionStamp
		if json.Unmarshal([]byte(encoded), &value) != nil || json.Unmarshal([]byte(floorJSON), &floor) != nil || !observationStamp(floor).Valid() {
			err = observation.ErrRefresh
			break
		}
		stamp := observationStamp(value)
		if freshness != "fresh" || !coversProjectionFloor(kind, stamp, observationStamp(floor)) {
			continue
		}
		switch kind {
		case "run":
			run = stamp
		case "writer":
			writer = stamp
		case "interaction":
			interaction = stamp
		default:
			continue
		}
		freshCount++
	}
	if err == nil {
		err = projectionRows.Err()
	}
	projectionRows.Close()
	if err != nil {
		return action.LocalState{}, err
	}
	var timeline string
	timelineErr := r.store.reader.QueryRowContext(ctx, `SELECT captured_head_cursor FROM runtime_timeline_cache WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&timeline)
	var pendingMask int64
	maskErr := r.store.reader.QueryRowContext(ctx, `SELECT refresh_mask FROM observation_refreshes WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&pendingMask)
	if timelineErr != nil && timelineErr != sql.ErrNoRows || maskErr != nil && maskErr != sql.ErrNoRows {
		return action.LocalState{}, errors.Join(timelineErr, maskErr)
	}
	out.ProjectionsStale = freshCount != len(projectionKinds) || !run.Valid() || interaction != run || writer.Writer != run.Writer ||
		!observation.Cursor(timeline).Valid() || observation.Cursor(timeline) != run.Head ||
		cp.Stamp.Valid() && (!run.Covers(cp.Stamp) || writer.Writer < cp.Stamp.Writer) ||
		pendingMask&int64(observation.AllAggregates) != 0
	rows, err := r.store.reader.QueryContext(ctx, `SELECT controller_references,state FROM provider_operation_attempts WHERE subject_id=? AND state IN ('pending','outcome_unknown')`, b.Binding.SubjectID)
	if err != nil {
		return action.LocalState{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded, state string
		if err := rows.Scan(&encoded, &state); err != nil {
			return action.LocalState{}, err
		}
		var refs []ControllerReference
		if json.Unmarshal([]byte(encoded), &refs) != nil {
			return action.LocalState{}, action.ErrBlocked
		}
		// Older attempts without a resolvable binding cannot prove another session
		// owns the effect, so they conservatively block this subject's mutations.
		matches := len(refs) == 0
		for _, ref := range refs {
			matches = matches || ref.BindingID == b.Binding.ID || ref.BindingID == b.Binding.ControllerBindingID || b.Carrier.ControllerID != "" && ref.ExpectedControllerID == b.Carrier.ControllerID
		}
		if matches {
			if state == "pending" {
				out.Operation = action.OperationPending
			} else if out.Operation != action.OperationPending {
				out.Operation = action.OperationUnknown
			}
		}
	}
	return out, rows.Err()
}

var _ action.Repository = ActionRepository{}
