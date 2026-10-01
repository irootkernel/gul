package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/session"
)

var _ session.Repository = PresentationRepository{}

// InsertBinding creates the local presentation and Primary binding atomically.
// Rediscovery of the same subject/workspace/Run retains local metadata.
func (r PresentationRepository) InsertBinding(ctx context.Context, binding session.Binding) (session.Binding, error) {
	if binding.SubjectID == "" || binding.ID == "" || binding.WorkspaceID == "" || binding.RunID == "" ||
		binding.ControllerBindingID == "" || binding.ProviderSessionID == "" {
		return session.Binding{}, session.ErrInvalid
	}
	config, err := json.Marshal(binding.Configuration)
	if err != nil {
		return session.Binding{}, err
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return session.Binding{}, err
	}
	defer tx.Rollback()
	var existingID, existingController, existingProvider string
	err = tx.QueryRowContext(ctx, `SELECT session_id, controller_binding_id, provider_session_id FROM primary_session_bindings
WHERE subject_id = ? AND workspace_id = ? AND run_id = ?`, binding.SubjectID, binding.WorkspaceID, binding.RunID).
		Scan(&existingID, &existingController, &existingProvider)
	if err == nil {
		if existingController != binding.ControllerBindingID || existingProvider != binding.ProviderSessionID {
			return session.Binding{}, session.ErrBindingConflict
		}
		return r.Binding(ctx, binding.SubjectID, existingID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return session.Binding{}, err
	}
	name := binding.Configuration.PurposeLabel
	if !presentation.ValidName(name) {
		name = "Session"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO direct_session_presentations(subject_id, session_id, workspace_id, display_name)
VALUES (?, ?, ?, ?)`, binding.SubjectID, binding.ID, binding.WorkspaceID, name); err != nil {
		return session.Binding{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO primary_session_bindings
(subject_id, session_id, workspace_id, run_id, controller_binding_id, provider_session_id, configuration_json)
VALUES (?, ?, ?, ?, ?, ?, ?)`, binding.SubjectID, binding.ID, binding.WorkspaceID, binding.RunID,
		binding.ControllerBindingID, binding.ProviderSessionID, string(config)); err != nil {
		return session.Binding{}, err
	}
	if err := tx.Commit(); err != nil {
		return session.Binding{}, err
	}
	return binding, nil
}

func (r PresentationRepository) Binding(ctx context.Context, subjectID, sessionID string) (session.Binding, error) {
	binding := session.Binding{SubjectID: subjectID, ID: sessionID}
	var config string
	err := r.store.reader.QueryRowContext(ctx, `SELECT workspace_id, run_id, controller_binding_id, provider_session_id, configuration_json
FROM primary_session_bindings WHERE subject_id = ? AND session_id = ?`, subjectID, sessionID).
		Scan(&binding.WorkspaceID, &binding.RunID, &binding.ControllerBindingID, &binding.ProviderSessionID, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Binding{}, session.ErrNotFound
	}
	if err != nil {
		return session.Binding{}, err
	}
	if err := json.Unmarshal([]byte(config), &binding.Configuration); err != nil {
		return session.Binding{}, err
	}
	return binding, nil
}

func (r PresentationRepository) ListBindings(ctx context.Context, subjectID, workspaceID string) ([]session.Binding, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT session_id, run_id, controller_binding_id, provider_session_id, configuration_json
FROM primary_session_bindings WHERE subject_id = ? AND workspace_id = ? ORDER BY session_id`, subjectID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := []session.Binding{}
	for rows.Next() {
		binding := session.Binding{SubjectID: subjectID, WorkspaceID: workspaceID}
		var config string
		if err := rows.Scan(&binding.ID, &binding.RunID, &binding.ControllerBindingID, &binding.ProviderSessionID, &config); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(config), &binding.Configuration); err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (r PresentationRepository) UpdateSnapshot(ctx context.Context, binding session.Binding, snapshot session.Snapshot) error {
	if binding.SubjectID == "" || binding.ID == "" || snapshot.ProviderSessionID != binding.ProviderSessionID ||
		snapshot.PrimaryRunID != binding.RunID || snapshot.ObservedAt.IsZero() {
		return session.ErrInvalidProjection
	}
	config, err := json.Marshal(snapshot.Configuration)
	if err != nil {
		return err
	}
	result, err := r.store.writer.ExecContext(ctx, `UPDATE primary_session_bindings SET configuration_json = ?, observed_at = ?
WHERE subject_id = ? AND session_id = ? AND workspace_id = ? AND run_id = ? AND provider_session_id = ?`,
		string(config), snapshot.ObservedAt.UTC().Format(time.RFC3339Nano), binding.SubjectID, binding.ID,
		binding.WorkspaceID, binding.RunID, binding.ProviderSessionID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return session.ErrNotFound
	}
	return nil
}

// BoundSessions supplies persisted backend bindings to host startup recovery.
// It is never exposed as a cross-subject browser listing.
func (r PresentationRepository) BoundSessions(ctx context.Context) ([]session.Binding, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT subject_id, session_id, workspace_id, run_id, controller_binding_id, provider_session_id, configuration_json FROM primary_session_bindings ORDER BY subject_id, session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []session.Binding
	for rows.Next() {
		var b session.Binding
		var config string
		if err = rows.Scan(&b.SubjectID, &b.ID, &b.WorkspaceID, &b.RunID, &b.ControllerBindingID, &b.ProviderSessionID, &config); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(config), &b.Configuration); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BindingByRun resolves one trusted subject's persisted Run without a host-wide scan.
// Ambiguous identity fails closed rather than choosing another binding.
func (r PresentationRepository) BindingByRun(ctx context.Context, subject, run string) (session.Binding, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT session_id, workspace_id, controller_binding_id, provider_session_id, configuration_json FROM primary_session_bindings WHERE subject_id = ? AND run_id = ? LIMIT 2`, subject, run)
	if err != nil {
		return session.Binding{}, err
	}
	defer rows.Close()
	var b session.Binding
	var config string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return session.Binding{}, err
		}
		return session.Binding{}, session.ErrNotFound
	}
	b.SubjectID, b.RunID = subject, run
	if err := rows.Scan(&b.ID, &b.WorkspaceID, &b.ControllerBindingID, &b.ProviderSessionID, &config); err != nil {
		return session.Binding{}, err
	}
	if rows.Next() {
		return session.Binding{}, session.ErrBindingConflict
	}
	if err := rows.Err(); err != nil {
		return session.Binding{}, err
	}
	if err := json.Unmarshal([]byte(config), &b.Configuration); err != nil {
		return session.Binding{}, err
	}
	return b, nil
}
