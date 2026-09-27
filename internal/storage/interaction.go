package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
)

// InteractionChanged journals only a safe invalidation after observer polling.
// It deliberately leaves the independent event resume checkpoints untouched.
func (r ObservationRepository) InteractionChanged(ctx context.Context, b interaction.Bound, stamp observation.Stamp) error {
	if stamp != (observation.Stamp{}) && !stamp.Valid() {
		return interaction.ErrBlocked
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
	var run, workspace, root string
	err = conn.QueryRowContext(ctx, `SELECT b.run_id,a.provider_workspace_id,a.canonical_root FROM primary_session_bindings b JOIN workspace_attachments a ON a.subject_id=b.subject_id AND a.workspace_id=b.workspace_id WHERE b.subject_id=? AND b.session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&run, &workspace, &root)
	if err != nil || run != b.Binding.RunID || workspace != b.Workspace.ProviderID || root != b.Workspace.CanonicalRoot {
		return interaction.ErrAuthority
	}
	// Existing aggregates remain non-authoritative until their own refresh ports
	// replace them; the poll itself supplies only Run/Interaction discovery.
	if _, err = conn.ExecContext(ctx, `UPDATE runtime_projection_cache SET freshness='stale' WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID); err != nil {
		return err
	}
	var sequence int64
	if err = conn.QueryRowContext(ctx, `UPDATE client_delivery_counter SET next_sequence=next_sequence+1 WHERE id=1 RETURNING next_sequence`).Scan(&sequence); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO client_event_journal(sequence,subject_id,event_kind,created_at) VALUES (?,?,'projection_invalidated',?)`, sequence, b.Binding.SubjectID, timestamp(time.Now())); err != nil {
		return err
	}
	correlation := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%v", b.Binding.ID, b.Binding.RunID, stamp)))
	if _, err = conn.ExecContext(ctx, `INSERT INTO client_projection_notifications(sequence,session_id,correlation_id) VALUES (?,?,?)`, sequence, b.Binding.ID, fmt.Sprintf("%x", correlation)); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

var _ interaction.Notifier = ObservationRepository{}
