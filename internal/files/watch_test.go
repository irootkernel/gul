package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExplicitRefreshInvalidatesDirectoryCursorOffline(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "a.txt", []byte("a"))
	writePreviewFile(t, root, "b.txt", []byte("b"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	page, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 1, "")
	if err != nil || page.NextToken == "" {
		t.Fatalf("page = %+v, %v", page, err)
	}
	revision, err := service.Refresh(t.Context(), "owner", "entry")
	if err != nil || revision != 1 {
		t.Fatalf("refresh = %d, %v", revision, err)
	}
	if _, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 1, page.NextToken); !errors.Is(err, ErrPageTokenExpired) {
		t.Fatalf("stale cursor = %v", err)
	}
}

func TestWatcherIgnoresPrivateSubtreeAndInvalidatesPublicFile(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "public.txt", []byte("before"))
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret", []byte("before"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changes, err := service.Watch(ctx, "owner", "entry")
	if err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret", []byte("after"))
	select {
	case event := <-changes:
		t.Fatalf("private event affected watcher: %+v", event)
	case <-time.After(300 * time.Millisecond):
	}
	writePreviewFile(t, root, "public.txt", []byte("after"))
	select {
	case event := <-changes:
		if event.Err != nil || event.Revision == 0 || event.RelativePath != "." {
			t.Fatalf("public event = %+v", event)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("public change was not observed")
	}
}

func TestWatcherNodeCountIsBounded(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < MaxWatchNodes+2; i++ {
		writePreviewFile(t, root, fmt.Sprintf("f-%03d", i), []byte("x"))
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	if changes, err := service.Watch(t.Context(), "owner", "entry"); changes != nil || !errors.Is(err, ErrWatchLimit) {
		t.Fatalf("oversized watcher = %v, %v", changes, err)
	}
}

func TestWatcherSlotLimitAndReleaseOnCancel(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "public.txt", []byte("x"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	cancels := make([]context.CancelFunc, 0, MaxWorkspaceWatchers)
	var first <-chan Invalidation
	for i := 0; i < MaxWorkspaceWatchers; i++ {
		ctx, cancel := context.WithCancel(t.Context())
		changes, err := service.Watch(ctx, "owner", "entry")
		if err != nil {
			cancel()
			t.Fatalf("watch %d = %v", i, err)
		}
		cancels = append(cancels, cancel)
		if i == 0 {
			first = changes
		}
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()
	if changes, err := service.Watch(t.Context(), "owner", "entry"); changes != nil || !errors.Is(err, ErrWatchLimit) {
		t.Fatalf("ninth watch = %v, %v", changes, err)
	}
	cancels[0]()
	select {
	case _, open := <-first:
		if open {
			t.Fatal("cancelled watch emitted an event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled watch did not release its slot")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, err := service.Watch(ctx, "owner", "entry"); err != nil {
		t.Fatalf("replacement watch = %v", err)
	}
}

func TestWatcherReattachesAfterDirectoryReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, "dir"), "old.txt", []byte("old"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changes, err := service.Watch(ctx, "owner", "entry")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "dir"), filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	awaitWatchRevision(t, changes)
	if err := os.Mkdir(filepath.Join(root, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, "dir"), "new.txt", []byte("first"))
	awaitWatchRevision(t, changes)
	// After the replacement has settled, a write to an existing child must
	// still reach the watcher rather than a stale descriptor for the old dir.
	time.Sleep(150 * time.Millisecond)
	writePreviewFile(t, filepath.Join(root, "dir"), "new.txt", []byte("second"))
	awaitWatchRevision(t, changes)
}

func TestWatcherInvalidatesContainedAliasRemovalButNotPrivateAlias(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "target.txt", []byte("public"))
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret", []byte("private"))
	if err := os.Symlink("target.txt", filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".dolgorae/secret", filepath.Join(root, "private-alias")); err != nil {
		t.Fatal(err)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changes, err := service.Watch(ctx, "owner", "entry")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "private-alias")); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-changes:
		t.Fatalf("private alias removal affected watcher: %+v", event)
	case <-time.After(300 * time.Millisecond):
	}
	if err := os.Remove(filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	awaitWatchRevision(t, changes)
}

func awaitWatchRevision(t *testing.T, changes <-chan Invalidation) {
	t.Helper()
	select {
	case event := <-changes:
		if event.Err != nil || event.Revision == 0 {
			t.Fatalf("watch change = %+v", event)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("watch change missing")
	}
}
