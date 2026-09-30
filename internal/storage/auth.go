package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/auth"
)

var passwordAccountStatements = []string{
	`CREATE TABLE password_account (
singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
subject_id TEXT NOT NULL UNIQUE REFERENCES app_account(subject_id) ON DELETE RESTRICT,
password_hash TEXT NOT NULL
)`,
}

func (r AuthRepository) PasswordAccount(ctx context.Context) (auth.PasswordAccount, error) {
	var account auth.PasswordAccount
	err := r.store.reader.QueryRowContext(ctx, `SELECT p.subject_id,p.password_hash
FROM password_account p WHERE singleton=1 AND (SELECT COUNT(*) FROM app_account)=1`).Scan(&account.SubjectID, &account.Hash)
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err := r.store.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM password_account").Scan(&count); err != nil || count != 0 {
			return auth.PasswordAccount{}, auth.ErrUnavailable
		}
		return auth.PasswordAccount{}, auth.ErrNotConfigured
	}
	if err != nil || account.SubjectID == "" || !auth.ValidPasswordHash(account.Hash) {
		return auth.PasswordAccount{}, auth.ErrUnavailable
	}
	return account, nil
}

// InitializePasswordAccount never holds a transaction during hash work. An
// existing sole foundation subject is retained so its local metadata survives.
// Ambiguous legacy data is refused; a password is never silently reassigned.
func (r AuthRepository) InitializePasswordAccount(ctx context.Context, account auth.PasswordAccount, at time.Time) (string, error) {
	if account.SubjectID == "" || !auth.ValidPasswordHash(account.Hash) || at.IsZero() {
		return "", auth.ErrUnavailable
	}
	conn, err := r.store.writer.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return "", err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var configured, subjects int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM password_account").Scan(&configured); err != nil {
		return "", err
	}
	if configured != 0 {
		return "", auth.ErrAlreadyConfigured
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_account").Scan(&subjects); err != nil {
		return "", err
	}
	switch subjects {
	case 0:
		if _, err := conn.ExecContext(ctx, "INSERT INTO app_account(subject_id,created_at) VALUES (?,?)", account.SubjectID, timestamp(at)); err != nil {
			return "", err
		}
	case 1:
		if err := conn.QueryRowContext(ctx, "SELECT subject_id FROM app_account").Scan(&account.SubjectID); err != nil || account.SubjectID == "" {
			return "", auth.ErrUnavailable
		}
	default:
		return "", auth.ErrUnavailable
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO password_account(singleton,subject_id,password_hash) VALUES (1,?,?)", account.SubjectID, account.Hash); err != nil {
		return "", err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return "", err
	}
	return account.SubjectID, nil
}
