package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rootkernel/gul/internal/presentation"
)

var _ presentation.Repository = PresentationRepository{}

// RemoveWorkspace deletes only Gul-owned rows. It never invokes the provider.
func (r PresentationRepository) RemoveWorkspace(ctx context.Context, subjectID, workspaceID string) error {
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE navigation_state SET workspace_id = NULL, session_id = NULL
WHERE subject_id = ? AND workspace_id = ?`, subjectID, workspaceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM direct_session_presentations WHERE subject_id = ? AND workspace_id = ?`, subjectID, workspaceID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM workspace_entries WHERE subject_id = ? AND workspace_id = ?`, subjectID, workspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return presentation.ErrNotFound
	}
	return tx.Commit()
}

func changedOne(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return presentation.ErrNotFound
	}
	return nil
}

func (r PresentationRepository) RenameWorkspace(ctx context.Context, subjectID, workspaceID, name string) error {
	if subjectID == "" || workspaceID == "" || !presentation.ValidName(name) {
		return presentation.ErrInvalid
	}
	return changedOne(r.store.writer.ExecContext(ctx, `UPDATE workspace_entries SET display_name = ?
WHERE subject_id = ? AND workspace_id = ? AND EXISTS (
  SELECT 1 FROM workspace_attachments WHERE subject_id = ? AND workspace_id = ?)`,
		name, subjectID, workspaceID, subjectID, workspaceID))
}

func (r PresentationRepository) SetWorkspaceFavorite(ctx context.Context, subjectID, workspaceID string, favorite bool) error {
	if subjectID == "" || workspaceID == "" {
		return presentation.ErrInvalid
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM workspace_attachments WHERE subject_id = ? AND workspace_id = ?`, subjectID, workspaceID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return presentation.ErrNotFound
		}
		return err
	}
	if favorite {
		_, err = tx.ExecContext(ctx, `INSERT INTO workspace_favorites(subject_id, workspace_id) VALUES (?, ?)
ON CONFLICT(subject_id, workspace_id) DO NOTHING`, subjectID, workspaceID)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM workspace_favorites WHERE subject_id = ? AND workspace_id = ?`, subjectID, workspaceID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r PresentationRepository) SetWorkspaceHidden(ctx context.Context, subjectID, workspaceID string, hidden bool) error {
	if subjectID == "" || workspaceID == "" {
		return presentation.ErrInvalid
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := changedOne(tx.ExecContext(ctx, `UPDATE workspace_entries SET hidden = ?
WHERE subject_id = ? AND workspace_id = ? AND EXISTS (
  SELECT 1 FROM workspace_attachments WHERE subject_id = ? AND workspace_id = ?)`,
		hidden, subjectID, workspaceID, subjectID, workspaceID)); err != nil {
		return err
	}
	if hidden {
		if _, err := tx.ExecContext(ctx, `DELETE FROM navigation_state WHERE subject_id = ? AND workspace_id = ?`, subjectID, workspaceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r PresentationRepository) RenameDirectSession(ctx context.Context, subjectID, sessionID, name string) error {
	if subjectID == "" || sessionID == "" || !presentation.ValidName(name) {
		return presentation.ErrInvalid
	}
	return changedOne(r.store.writer.ExecContext(ctx, `UPDATE direct_session_presentations SET display_name = ?
WHERE subject_id = ? AND session_id = ?`, name, subjectID, sessionID))
}

func (r PresentationRepository) SetDirectSessionFavorite(ctx context.Context, subjectID, sessionID string, favorite bool) error {
	if subjectID == "" || sessionID == "" {
		return presentation.ErrInvalid
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM direct_session_presentations WHERE subject_id = ? AND session_id = ?`, subjectID, sessionID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return presentation.ErrNotFound
		}
		return err
	}
	if favorite {
		_, err = tx.ExecContext(ctx, `INSERT INTO direct_session_favorites(subject_id, session_id) VALUES (?, ?)
ON CONFLICT(subject_id, session_id) DO NOTHING`, subjectID, sessionID)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM direct_session_favorites WHERE subject_id = ? AND session_id = ?`, subjectID, sessionID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r PresentationRepository) SetDirectSessionArchived(ctx context.Context, subjectID, sessionID string, archived bool) error {
	if subjectID == "" || sessionID == "" {
		return presentation.ErrInvalid
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := changedOne(tx.ExecContext(ctx, `UPDATE direct_session_presentations SET hidden = ?
WHERE subject_id = ? AND session_id = ?`, archived, subjectID, sessionID)); err != nil {
		return err
	}
	if archived {
		if _, err := tx.ExecContext(ctx, `UPDATE navigation_state SET session_id = NULL WHERE subject_id = ? AND session_id = ?`, subjectID, sessionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r PresentationRepository) Navigation(ctx context.Context, subjectID string) (presentation.Navigation, error) {
	if subjectID == "" {
		return presentation.Navigation{}, presentation.ErrInvalid
	}
	var value presentation.Navigation
	err := r.store.reader.QueryRowContext(ctx, `SELECT COALESCE(workspace_id, ''), COALESCE(session_id, '')
FROM navigation_state WHERE subject_id = ?`, subjectID).Scan(&value.WorkspaceID, &value.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return presentation.Navigation{}, nil
	}
	return value, err
}

func (r PresentationRepository) SetNavigation(ctx context.Context, subjectID string, value presentation.Navigation) error {
	if subjectID == "" || value.SessionID != "" && value.WorkspaceID == "" {
		return presentation.ErrInvalid
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if value.WorkspaceID == "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM navigation_state WHERE subject_id = ?`, subjectID); err != nil {
			return err
		}
		return tx.Commit()
	}
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM workspace_entries AS e
JOIN workspace_attachments AS a USING(subject_id, workspace_id)
WHERE e.subject_id = ? AND e.workspace_id = ? AND e.hidden = 0`, subjectID, value.WorkspaceID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return presentation.ErrNotFound
	}
	if err != nil {
		return err
	}
	if value.SessionID != "" {
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM direct_session_presentations
WHERE subject_id = ? AND session_id = ? AND workspace_id = ? AND hidden = 0`,
			subjectID, value.SessionID, value.WorkspaceID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return presentation.ErrNotFound
		}
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO navigation_state(subject_id, workspace_id, session_id) VALUES (?, ?, ?)
ON CONFLICT(subject_id) DO UPDATE SET workspace_id = excluded.workspace_id, session_id = excluded.session_id`,
		subjectID, value.WorkspaceID, value.SessionID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
