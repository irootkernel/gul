package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schemaVersion = 2

var ErrSchemaDrift = errors.New("Gul SQLite schema drift")

// Store keeps one serialized writer and at most four read-only connections.
// Callers must never hold a write transaction across provider or file I/O.
type Store struct {
	writer *sql.DB
	reader *sql.DB
	path   string
}

func Open(ctx context.Context, filename string) (*Store, error) {
	if filename == "" || !filepath.IsAbs(filename) {
		return nil, errors.New("Gul database path must be absolute")
	}
	path := filepath.Clean(filename)
	if err := prepareDatabaseFile(path); err != nil {
		return nil, err
	}
	writer, err := sql.Open("sqlite", databaseURL(path, false))
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	if err := writer.PingContext(ctx); err != nil {
		writer.Close()
		return nil, err
	}
	if err := migrate(ctx, writer); err != nil {
		writer.Close()
		return nil, err
	}
	if _, err := writer.ExecContext(ctx, "UPDATE runtime_projection_cache SET freshness = 'stale'"); err != nil {
		writer.Close()
		return nil, err
	}
	reader, err := sql.Open("sqlite", databaseURL(path, true))
	if err != nil {
		writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(4)
	reader.SetMaxIdleConns(4)
	if err := reader.PingContext(ctx); err != nil {
		reader.Close()
		writer.Close()
		return nil, err
	}
	return &Store{writer: writer, reader: reader, path: path}, nil
}

func prepareDatabaseFile(filename string) error {
	if err := validateOwnerOnlyComponent(filepath.Dir(filename), true); err != nil {
		return fmt.Errorf("unsafe Gul database directory: %w", err)
	}
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		if err := file.Close(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := validateOwnerOnlyComponent(filename, false); err != nil {
		return fmt.Errorf("unsafe Gul database file: %w", err)
	}
	return nil
}

func databaseURL(path string, readOnly bool) string {
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_busy_timeout", "5000")
	if readOnly {
		q.Set("mode", "ro")
	} else {
		q.Add("_pragma", "journal_mode(WAL)")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Store) Ready(ctx context.Context) error {
	if s == nil || s.writer == nil || s.reader == nil {
		return errors.New("Gul persistence unavailable")
	}
	if err := s.writer.PingContext(ctx); err != nil {
		return err
	}
	return s.reader.PingContext(ctx)
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.reader != nil {
		err = s.reader.Close()
	}
	if s.writer != nil {
		err = errors.Join(err, s.writer.Close())
	}
	return err
}

func migrate(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version < 0 || version > schemaVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrSchemaDrift, version)
	}
	for next := version + 1; next <= schemaVersion; next++ {
		statements := migrationStatements(next)
		if len(statements) == 0 {
			return fmt.Errorf("%w: unregistered migration %d", ErrSchemaDrift, next)
		}
		for _, statement := range statements {
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("Gul migration %d: %w", next, err)
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO schema_migrations(version, digest, applied_at) VALUES (?, ?, ?)", next, digestStatements(statements), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if next == 1 {
			if _, err := conn.ExecContext(ctx, "INSERT INTO client_delivery_counter(id, next_sequence) VALUES (1, 0)"); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", next)); err != nil {
			return err
		}
	}
	for migration := 1; migration <= schemaVersion; migration++ {
		statements := migrationStatements(migration)
		if len(statements) == 0 {
			return fmt.Errorf("%w: unregistered migration %d", ErrSchemaDrift, migration)
		}
		var digest string
		if err := conn.QueryRowContext(ctx, "SELECT digest FROM schema_migrations WHERE version = ?", migration).Scan(&digest); err != nil || digest != digestStatements(statements) {
			return ErrSchemaDrift
		}
	}
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil || count != schemaVersion {
		return ErrSchemaDrift
	}
	if err := verifySchema(ctx, conn); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

func digestStatements(statements []string) string {
	digest := sha256.Sum256([]byte(strings.Join(statements, "\n")))
	return hex.EncodeToString(digest[:])
}

func migrationStatements(version int) []string {
	switch version {
	case 1:
		return schemaStatements
	case 2:
		return attachmentStatements
	default:
		return nil
	}
}

func verifySchema(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, "SELECT type, name, sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'")
	if err != nil {
		return err
	}
	defer rows.Close()
	expected := make(map[string]string)
	for version := 1; version <= schemaVersion; version++ {
		for _, statement := range migrationStatements(version) {
			name := strings.Fields(statement)[2]
			expected[name] = statement
		}
	}
	count := 0
	for rows.Next() {
		var kind, name, statement string
		if err := rows.Scan(&kind, &name, &statement); err != nil {
			return err
		}
		if kind != "table" || expected[name] != statement {
			return fmt.Errorf("%w: schema object %s", ErrSchemaDrift, name)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(expected) {
		return fmt.Errorf("%w: table inventory", ErrSchemaDrift)
	}
	return nil
}
