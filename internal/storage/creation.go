package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/session"
)

var creationStatements = []string{`CREATE TABLE session_creations (
operation_id TEXT PRIMARY KEY,
subject_id TEXT NOT NULL,
workspace_id TEXT NOT NULL,
attempt_id TEXT NOT NULL,
choice_json TEXT NOT NULL,
controller_binding_id TEXT NOT NULL,
created_at TEXT NOT NULL
)`}

// Creation reserves public launch settings and the logical destination before
// dispatch. It contains no provider path, prompt, image or capability material.
type Creation struct {
	OperationID, SubjectID, WorkspaceID, AttemptID, ChoiceJSON, ControllerBindingID string
	CreatedAt                                                                       time.Time
}

func (r PresentationRepository) Creation(ctx context.Context, id string) (Creation, error) {
	var c Creation
	var created string
	err := r.store.reader.QueryRowContext(ctx, `SELECT operation_id,subject_id,workspace_id,attempt_id,choice_json,controller_binding_id,created_at FROM session_creations WHERE operation_id=?`, id).Scan(&c.OperationID, &c.SubjectID, &c.WorkspaceID, &c.AttemptID, &c.ChoiceJSON, &c.ControllerBindingID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return c, session.ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return c, err
}

func (r PresentationRepository) ReserveCreation(ctx context.Context, c Creation) error {
	result, err := r.store.writer.ExecContext(ctx, `INSERT INTO session_creations(operation_id,subject_id,workspace_id,attempt_id,choice_json,controller_binding_id,created_at) SELECT ?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM controller_credential_metadata m JOIN controller_binding_references b USING(binding_id) WHERE m.binding_id=? AND b.subject_id=? AND m.removing=0)`, c.OperationID, c.SubjectID, c.WorkspaceID, c.AttemptID, c.ChoiceJSON, c.ControllerBindingID, c.CreatedAt.UTC().Format(time.RFC3339Nano), c.ControllerBindingID, c.SubjectID)
	if err == nil {
		var n int64
		n, err = result.RowsAffected()
		if err == nil && n != 1 {
			return ErrMutationConflict
		}
	}
	return err
}

// PendingCreationLabels exposes only public retry labels. A completed binding
// removes a request from this view even after its Controller is adopted.
func (r PresentationRepository) PendingCreationLabels(ctx context.Context, subject, workspace string) ([]string, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT c.attempt_id FROM session_creations c WHERE c.subject_id=? AND c.workspace_id=? AND NOT EXISTS(SELECT 1 FROM mutation_attempt_details d JOIN primary_session_bindings b ON b.run_id=d.outcome_ref WHERE d.operation_id=c.operation_id AND b.subject_id=c.subject_id) ORDER BY c.created_at,c.operation_id`, subject, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}
