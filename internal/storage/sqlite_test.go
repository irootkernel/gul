package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenMigratesAndRejectsDrift(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "gul.sqlite")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("database mode = %v, %v", info, err)
	}
	for name, query := range map[string]string{
		"journal":      "PRAGMA journal_mode",
		"foreign keys": "PRAGMA foreign_keys",
		"synchronous":  "PRAGMA synchronous",
		"timeout":      "PRAGMA busy_timeout",
	} {
		var value any
		if err := store.writer.QueryRowContext(t.Context(), query).Scan(&value); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := map[string]any{"journal": "wal", "foreign keys": int64(1), "synchronous": int64(2), "timeout": int64(5000)}[name]
		if value != want {
			t.Errorf("%s = %v, want %v", name, value, want)
		}
	}
	if store.writer.Stats().MaxOpenConnections != 1 || store.reader.Stats().MaxOpenConnections != 4 {
		t.Fatalf("connection caps: writer %d reader %d", store.writer.Stats().MaxOpenConnections, store.reader.Stats().MaxOpenConnections)
	}
	if err := store.Ready(t.Context()); err != nil {
		t.Fatalf("ready store: %v", err)
	}
	if _, err := store.reader.ExecContext(t.Context(), "INSERT INTO app_account(subject_id, created_at) VALUES ('forbidden', 'now')"); err == nil {
		t.Fatal("read-only pool accepted a write")
	}
	rows, err := store.reader.QueryContext(t.Context(), "SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"capability", "carrier_path", "socket_path", "prompt", "protected_input", "authoritative_run", "writer_lock"} {
			if strings.Contains(name+definition, forbidden) {
				t.Errorf("Gul schema contains prohibited field or aggregate %q", forbidden)
			}
		}
		count++
	}
	if err := rows.Err(); err != nil || count != len(schemaStatements) {
		t.Fatalf("schema inventory = %d, %v", count, err)
	}
	rows.Close()
	for _, db := range []*sql.DB{store.writer, store.reader} {
		for query, want := range map[string]int64{"PRAGMA foreign_keys": 1, "PRAGMA synchronous": 2, "PRAGMA busy_timeout": 5000} {
			var got int64
			if err := db.QueryRowContext(t.Context(), query).Scan(&got); err != nil || got != want {
				t.Errorf("%s = %d, %v; want %d", query, got, err, want)
			}
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Ready(t.Context()); err == nil {
		t.Fatal("closed store reported ready")
	}
	var absent *Store
	if err := absent.Ready(t.Context()); err == nil {
		t.Fatal("nil store reported ready")
	}
	store, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, err := store.writer.ExecContext(t.Context(), "ALTER TABLE app_account ADD COLUMN unexpected TEXT"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if _, err := Open(t.Context(), path); !errors.Is(err, ErrSchemaDrift) {
		t.Fatalf("schema drift = %v", err)
	}
}

func TestOpenRejectsMigrationAndSchemaObjectDrift(t *testing.T) {
	for name, mutation := range map[string]string{
		"digest":  "UPDATE schema_migrations SET digest = 'wrong'",
		"version": "PRAGMA user_version = 2",
		"missing": "DROP TABLE navigation_state",
		"trigger": "CREATE TRIGGER unexpected AFTER INSERT ON app_account BEGIN DELETE FROM app_account; END",
		"view":    "CREATE VIEW unexpected AS SELECT subject_id FROM app_account",
		"index":   "CREATE INDEX unexpected ON app_account(created_at)",
	} {
		t.Run(name, func(t *testing.T) {
			store, filename := openTestStore(t)
			if _, err := store.writer.ExecContext(t.Context(), mutation); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(t.Context(), filename); !errors.Is(err, ErrSchemaDrift) {
				t.Fatalf("accepted %s drift: %v", name, err)
			}
		})
	}
}

func TestFailedFirstMigrationRollsBackItsTables(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "partial.sqlite")
	db, err := sql.Open("sqlite", databaseURL(filename, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE app_account(unexpected TEXT)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.Chmod(filename, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), filename); err == nil {
		t.Fatal("partial incompatible schema migrated")
	}
	db, err = sql.Open("sqlite", databaseURL(filename, false))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sqlite_master WHERE name = 'schema_migrations'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed migration left schema table: %d, %v", count, err)
	}
}

func TestOpenRejectsRelativePath(t *testing.T) {
	if _, err := Open(context.Background(), "gul.sqlite"); err == nil {
		t.Fatal("relative path accepted")
	}
}

func TestOpenRejectsUnsafeDatabasePaths(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "gul.sqlite")
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("world-readable database directory accepted")
	}
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("world-readable database file accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, "missing"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("symlinked database file accepted")
	}
}

func TestBackupPublishesConsistentSnapshotWithoutOverwrite(t *testing.T) {
	store, filename := openTestStore(t)
	for _, destination := range []string{"relative.sqlite", filename, filename + "-wal", filename + "-shm", filename + "-journal"} {
		if err := store.Backup(t.Context(), destination); err == nil {
			t.Errorf("accepted unsafe backup destination %q", destination)
		}
	}
	backup := filepath.Join(filepath.Dir(filename), "backup.sqlite")
	if err := os.Chmod(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.Backup(t.Context(), backup); err == nil {
		t.Fatal("world-readable backup directory accepted")
	}
	if err := os.Chmod(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.Auth().CreateAccount(t.Context(), "account-1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.Backup(t.Context(), backup); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("backup mode = %v, %v", info, err)
	}
	if err := store.Backup(t.Context(), backup); err == nil {
		t.Fatal("existing backup overwritten")
	}
	copy, err := Open(t.Context(), backup)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	var count int
	if err := copy.reader.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM app_account").Scan(&count); err != nil || count != 1 {
		t.Fatalf("backup accounts = %d, %v", count, err)
	}
}

func TestBackupRefusesBusyWALCheckpoint(t *testing.T) {
	store, filename := openTestStore(t)
	backup := filepath.Join(filepath.Dir(filename), "busy-backup.sqlite")
	reader, err := store.reader.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback()
	var count int
	if err := reader.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM app_account").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := store.Auth().CreateAccount(t.Context(), "account-after-snapshot", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.Backup(t.Context(), backup); err == nil || !strings.Contains(err.Error(), "checkpoint is busy") {
		t.Fatalf("backup with held WAL reader = %v, want busy checkpoint", err)
	}
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("busy checkpoint published backup: %v", err)
	}
	if err := reader.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := store.Backup(t.Context(), backup); err != nil {
		t.Fatalf("backup after releasing reader: %v", err)
	}
}
