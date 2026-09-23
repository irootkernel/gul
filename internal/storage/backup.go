package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Backup checkpoints WAL and publishes a consistent SQLite image. The caller
// owns a trusted destination directory; existing backups are never replaced.
func (s *Store) Backup(ctx context.Context, destination string) error {
	if s == nil || s.writer == nil || !filepath.IsAbs(destination) {
		return errors.New("invalid Gul backup destination")
	}
	destination = filepath.Clean(destination)
	for _, reserved := range []string{s.path, s.path + "-wal", s.path + "-shm", s.path + "-journal"} {
		if destination == reserved {
			return errors.New("invalid Gul backup destination")
		}
	}
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("Gul backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(destination)
	if err := validateOwnerOnlyComponent(parent, true); err != nil {
		return fmt.Errorf("unsafe Gul backup directory: %w", err)
	}
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return err
	}
	temporary := filepath.Join(parent, ".gul-backup-"+hex.EncodeToString(suffix[:]))
	defer os.Remove(temporary)
	conn, err := s.writer.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var busy, logFrames, checkpointed int
	if err := conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return err
	}
	if busy != 0 {
		return errors.New("Gul WAL checkpoint is busy")
	}
	quoted := strings.ReplaceAll(temporary, "'", "''")
	if _, err := conn.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0600); err != nil {
		return err
	}
	file, err := os.OpenFile(temporary, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := syncDirectory(parent); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("Gul backup destination appeared during publication")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := unix.RenamexNp(temporary, destination, unix.RENAME_EXCL); err != nil {
		return fmt.Errorf("publish Gul backup: %w", err)
	}
	return syncDirectory(parent)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
