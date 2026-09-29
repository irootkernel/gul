//go:build darwin

package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestDarwinWatcherTracksOnlyExplicitNodes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret", []byte("private"))
	writePreviewFile(t, root, "public", []byte("public"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changes, err := service.Watch(ctx, "owner", "entry")
	if err != nil {
		t.Fatal(err)
	}
	_ = changes
	// A direct watcher registration never expands into its private sibling.
	w, err := newHostWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	file, _, err := service.Open(t.Context(), "owner", "entry", ".")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	actual, err := descriptorPath(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add(actual, file); err != nil {
		t.Fatal(err)
	}
	native := w.(*darwinHostWatcher)
	native.mu.Lock()
	count := len(native.byPath)
	native.mu.Unlock()
	if count != 1 {
		t.Fatalf("implicit watches: %d", count)
	}
}

func TestDarwinWatcherSeesSameNameDirectorySwap(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"dir", "replacement"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
		writePreviewFile(t, filepath.Join(root, name), "file.txt", []byte(name))
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changes, err := service.Watch(ctx, "owner", "entry")
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.RenameatxNp(unix.AT_FDCWD, filepath.Join(root, "dir"), unix.AT_FDCWD, filepath.Join(root, "replacement"), unix.RENAME_SWAP); err != nil {
		t.Fatal(err)
	}
	awaitWatchRevision(t, changes)
	time.Sleep(300 * time.Millisecond)
	for {
		select {
		case <-changes:
		default:
			goto settled
		}
	}
settled:
	writePreviewFile(t, filepath.Join(root, "dir"), "file.txt", []byte("updated"))
	awaitWatchRevision(t, changes)
}
