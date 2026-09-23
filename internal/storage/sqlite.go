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

const schemaVersion = 1

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
	switch version {
	case 0:
		for _, statement := range schemaStatements {
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("Gul migration 1: %w", err)
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO schema_migrations(version, digest, applied_at) VALUES (1, ?, ?)", schemaDigest(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO client_delivery_counter(id, next_sequence) VALUES (1, 0)"); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
			return err
		}
	case schemaVersion:
		var digest string
		var count, recordedVersion int
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*), MAX(version), MAX(digest) FROM schema_migrations").Scan(&count, &recordedVersion, &digest); err != nil {
			return err
		}
		if count != 1 || recordedVersion != schemaVersion || digest != schemaDigest() {
			return ErrSchemaDrift
		}
	default:
		return fmt.Errorf("%w: unsupported version %d", ErrSchemaDrift, version)
	}
	if err := verifySchema(ctx, conn); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

func schemaDigest() string {
	digest := sha256.Sum256([]byte(strings.Join(schemaStatements, "\n")))
	return hex.EncodeToString(digest[:])
}

func verifySchema(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, "SELECT type, name, sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'")
	if err != nil {
		return err
	}
	defer rows.Close()
	expected := make(map[string]string, len(schemaStatements))
	for _, statement := range schemaStatements {
		name := strings.Fields(statement)[2]
		expected[name] = statement
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
