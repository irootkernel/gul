package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/rootkernel/gul/internal/observation"
)

// allocateNotification is called inside the caller's BEGIN IMMEDIATE transaction.
func allocateNotification(ctx context.Context, conn *sql.Conn, subject, sessionID, correlation string, at time.Time) (observation.Notification, error) {
	n := observation.Notification{SessionID: sessionID, CorrelationID: correlation, Kind: "projection_invalidated", CreatedAt: at}
	if err := conn.QueryRowContext(ctx, `UPDATE client_delivery_counter SET next_sequence=next_sequence+1 WHERE id=1 RETURNING next_sequence`).Scan(&n.Sequence); err != nil {
		return observation.Notification{}, err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO client_event_journal(sequence,subject_id,event_kind,created_at) VALUES(?,?,?,?)`, int64(n.Sequence), subject, n.Kind, timestamp(at)); err != nil {
		return observation.Notification{}, err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO client_projection_notifications(sequence,session_id,correlation_id) VALUES(?,?,?)`, int64(n.Sequence), sessionID, correlation); err != nil {
		return observation.Notification{}, err
	}
	return n, nil
}
