package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"

	"github.com/rootkernel/gul/internal/workspace"
)

var _ workspace.Repository = PresentationRepository{}

// CreateAttachment commits the local presentation and verified provider
// reference together. Provider I/O has already finished before this transaction.
func (r PresentationRepository) CreateAttachment(ctx context.Context, entry workspace.Attachment) error {
	if !validAttachment(entry) {
		return errors.New("invalid workspace attachment")
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_entries(subject_id, workspace_id, display_name, hidden) VALUES (?, ?, ?, ?)`,
		entry.SubjectID, entry.ID, entry.DisplayName, entry.Hidden); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_attachments(subject_id, workspace_id, canonical_root, provider_workspace_id, file_device, file_inode) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.SubjectID, entry.ID, entry.CanonicalRoot, entry.ProviderID, entry.FileDevice, entry.FileInode); err != nil {
		return err
	}
	return tx.Commit()
}

func validAttachment(entry workspace.Attachment) bool {
	return entry.SubjectID != "" && entry.ID != "" && entry.ProviderID != "" && entry.FileDevice != "" && entry.FileInode != "" && entry.DisplayName != "" &&
		filepath.IsAbs(entry.CanonicalRoot) && filepath.Clean(entry.CanonicalRoot) == entry.CanonicalRoot
}

// ReplaceAttachment preserves the Gul presentation ID and metadata during an
// explicit reattachment after the selected directory has been re-inspected.
func (r PresentationRepository) ReplaceAttachment(ctx context.Context, entry workspace.Attachment) error {
	if !validAttachment(entry) {
		return errors.New("invalid workspace attachment")
	}
	result, err := r.store.writer.ExecContext(ctx, `UPDATE workspace_attachments
SET canonical_root = ?, provider_workspace_id = ?, file_device = ?, file_inode = ?
WHERE subject_id = ? AND workspace_id = ?`, entry.CanonicalRoot, entry.ProviderID, entry.FileDevice, entry.FileInode, entry.SubjectID, entry.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r PresentationRepository) Attachment(ctx context.Context, subjectID, entryID string) (workspace.Attachment, error) {
	entry := workspace.Attachment{SubjectID: subjectID, ID: entryID}
	var hidden bool
	err := r.store.reader.QueryRowContext(ctx, `SELECT a.canonical_root, a.provider_workspace_id, a.file_device, a.file_inode, e.display_name, e.hidden, f.workspace_id IS NOT NULL
FROM workspace_attachments AS a JOIN workspace_entries AS e USING(subject_id, workspace_id)
LEFT JOIN workspace_favorites AS f USING(subject_id, workspace_id)
WHERE a.subject_id = ? AND a.workspace_id = ?`, subjectID, entryID).
		Scan(&entry.CanonicalRoot, &entry.ProviderID, &entry.FileDevice, &entry.FileInode, &entry.DisplayName, &hidden, &entry.Favorite)
	if errors.Is(err, sql.ErrNoRows) {
		return workspace.Attachment{}, workspace.ErrAttachmentNotFound
	}
	entry.Hidden = hidden
	return entry, err
}

func (r PresentationRepository) ListAttachments(ctx context.Context, subjectID string) ([]workspace.Attachment, error) {
	rows, err := r.store.reader.QueryContext(ctx, `SELECT a.workspace_id, a.canonical_root, a.provider_workspace_id, a.file_device, a.file_inode, e.display_name, e.hidden, f.workspace_id IS NOT NULL
FROM workspace_attachments AS a JOIN workspace_entries AS e USING(subject_id, workspace_id)
LEFT JOIN workspace_favorites AS f USING(subject_id, workspace_id)
WHERE a.subject_id = ? ORDER BY a.workspace_id`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []workspace.Attachment{}
	for rows.Next() {
		entry := workspace.Attachment{SubjectID: subjectID}
		var hidden bool
		if err := rows.Scan(&entry.ID, &entry.CanonicalRoot, &entry.ProviderID, &entry.FileDevice, &entry.FileInode, &entry.DisplayName, &hidden, &entry.Favorite); err != nil {
			return nil, err
		}
		entry.Hidden = hidden
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// WorkspaceRoots includes unattached-to-session roots when selecting a private
// gateway socket. It is host-only and exposes no provider endpoint to clients.
func (r PresentationRepository) WorkspaceRoots(ctx context.Context) ([]string, error) {
	rows, err := r.store.reader.QueryContext(ctx, "SELECT DISTINCT canonical_root FROM workspace_attachments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roots []string
	for rows.Next() {
		var root string
		if err = rows.Scan(&root); err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}
