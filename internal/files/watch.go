package files

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

var ErrWatchLimit = errors.New("file watcher limit reached")
var ErrWatchUnavailable = errors.New("file watcher unavailable")

const MaxWatchNodes = 512
const MaxWatchScannedEntries = 4096
const MaxWorkspaceWatchers = 8
const WatchCoalesceInterval = 100 * time.Millisecond

type Invalidation struct {
	Revision     uint64
	RelativePath string
	Err          error
}

type watchedNode struct {
	relative string
	info     os.FileInfo
}

// Watch observes verified local filesystem nodes. Events only invalidate
// presentation; callers reread content through the guarded accessor.
func (s *Service) Watch(ctx context.Context, subject, workspaceID string) (<-chan Invalidation, error) {
	if s == nil {
		return nil, ErrReattachRequired
	}
	s.cursorMu.Lock()
	if s.watchers >= MaxWorkspaceWatchers {
		s.cursorMu.Unlock()
		return nil, ErrWatchLimit
	}
	s.watchers++
	s.cursorMu.Unlock()
	release := func() { s.cursorMu.Lock(); s.watchers--; s.cursorMu.Unlock() }
	root, _, err := s.Open(ctx, subject, workspaceID, ".")
	if err != nil {
		release()
		return nil, err
	}
	rootPath, err := descriptorPath(root)
	root.Close()
	if err != nil {
		release()
		return nil, ErrWatchUnavailable
	}
	watcher, err := newHostWatcher()
	if err != nil {
		release()
		return nil, ErrWatchUnavailable
	}
	paths := make(map[string]watchedNode)
	aliases := make(map[string]struct{})
	if err := s.addWatchTree(ctx, watcher, paths, aliases, rootPath, subject, workspaceID, "."); err != nil {
		watcher.Close()
		release()
		return nil, err
	}
	out := make(chan Invalidation, 1)
	go func() {
		defer close(out)
		defer watcher.Close()
		defer release()
		ticker := time.NewTicker(WatchCoalesceInterval)
		defer ticker.Stop()
		pending := false
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events():
				if !ok {
					sendInvalidation(out, Invalidation{Err: ErrWatchUnavailable})
					return
				}
				relative, accepted := s.watchEvent(ctx, watcher, paths, aliases, rootPath, subject, workspaceID, event)
				if !accepted {
					continue
				}
				if relative == "" {
					sendInvalidation(out, Invalidation{Err: ErrWatchLimit})
					return
				}
				pending = true
			case watchErr, ok := <-watcher.Errors():
				if !ok {
					return
				}
				if errors.Is(watchErr, ErrWatchLimit) {
					sendInvalidation(out, Invalidation{Err: ErrWatchLimit})
				} else {
					sendInvalidation(out, Invalidation{Err: ErrWatchUnavailable})
				}
				return
			case <-ticker.C:
				if !pending {
					continue
				}
				revision, err := s.Refresh(ctx, subject, workspaceID)
				if err != nil {
					sendInvalidation(out, Invalidation{Err: err})
					return
				}
				sendInvalidation(out, Invalidation{Revision: revision, RelativePath: "."})
				pending = false
			}
		}
	}()
	return out, nil
}

func sendInvalidation(out chan Invalidation, event Invalidation) {
	select {
	case out <- event:
		return
	default:
	}
	select {
	case <-out:
	default:
	}
	out <- event // a newer root invalidation supersedes the older one
}

func (s *Service) addWatchTree(ctx context.Context, watcher hostWatcher, paths map[string]watchedNode, aliases map[string]struct{}, rootPath, subject, workspaceID, relative string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, _, err := s.Open(ctx, subject, workspaceID, relative)
	if err != nil {
		return err
	}
	defer file.Close()
	actual, err := descriptorPath(file)
	if err != nil {
		return ErrWatchUnavailable
	}
	canonicalRelative, err := filepath.Rel(rootPath, actual)
	if err != nil {
		return ErrWatchUnavailable
	}
	if filepath.ToSlash(canonicalRelative) != relative {
		aliases[filepath.Join(rootPath, filepath.FromSlash(relative))] = struct{}{}
		return nil // the canonical node is reached through the ordinary tree walk
	}
	info, err := file.Stat()
	if err != nil {
		return ErrWatchUnavailable
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil
	}
	if previous, exists := paths[actual]; exists {
		if os.SameFile(previous.info, info) {
			return nil
		}
		removeWatchSubtree(watcher, paths, aliases, actual)
	}
	if len(paths) >= MaxWatchNodes {
		return ErrWatchLimit
	}
	var scan *os.File
	if info.IsDir() {
		scan, err = openWatchScan(file)
		if err != nil {
			return ErrWatchUnavailable
		}
		defer scan.Close()
	}
	if err := watcher.Add(actual, file); err != nil {
		if errors.Is(err, ErrWatchLimit) {
			return ErrWatchLimit
		}
		return ErrWatchUnavailable
	}
	paths[actual] = watchedNode{relative: relative, info: info}
	if !info.IsDir() {
		return nil
	}
	scanned := 0
	for {
		names, readErr := scan.Readdirnames(64)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return ErrWatchUnavailable
		}
		scanned += len(names)
		if scanned > MaxWatchScannedEntries {
			return ErrWatchLimit
		}
		for _, name := range names {
			if !utf8.ValidString(name) || !fs.ValidPath(name) || privateComponent(name) {
				continue
			}
			child := name
			if relative != "." {
				child = path.Join(relative, name)
			}
			if err := s.addWatchTree(ctx, watcher, paths, aliases, rootPath, subject, workspaceID, child); err != nil {
				if errors.Is(err, ErrPathUnavailable) {
					continue
				} // inaccessible aliases stay outside the watch set
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

// A new open file description keeps the tree scan independent of snapshot reads
// on a watcher descriptor duplicated from file.
func openWatchScan(file *os.File) (*os.File, error) {
	fd, err := unix.Openat(int(file.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "watch-scan"), nil
}

func removeWatchSubtree(watcher hostWatcher, paths map[string]watchedNode, aliases map[string]struct{}, name string) {
	for watched := range paths {
		if watched == name || strings.HasPrefix(watched, name+string(filepath.Separator)) {
			_ = watcher.Remove(watched)
			delete(paths, watched)
		}
	}
	for alias := range aliases {
		if alias == name || strings.HasPrefix(alias, name+string(filepath.Separator)) {
			delete(aliases, alias)
		}
	}
}

func (s *Service) watchEvent(ctx context.Context, watcher hostWatcher, paths map[string]watchedNode, aliases map[string]struct{}, rootPath, subject, workspaceID string, event fsnotify.Event) (string, bool) {
	relative, err := filepath.Rel(rootPath, event.Name)
	if err != nil {
		return ".", false
	}
	relative = filepath.ToSlash(relative)
	if !fs.ValidPath(relative) || privateComponent(filepath.FromSlash(relative)) {
		return ".", false
	}
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		if _, known := aliases[event.Name]; known {
			delete(aliases, event.Name)
			return relative, true
		}
		previous, known := paths[event.Name]
		if !known {
			return ".", false
		}
		if current, _, openErr := s.Open(ctx, subject, workspaceID, previous.relative); openErr == nil {
			currentInfo, statErr := current.Stat()
			current.Close()
			if statErr == nil && os.SameFile(previous.info, currentInfo) {
				return ".", false // stale removal for a path still at its watched identity
			}
		}
		removeWatchSubtree(watcher, paths, aliases, event.Name)
		return relative, true
	}
	file, _, err := s.Open(ctx, subject, workspaceID, relative)
	if err != nil {
		return ".", false
	}
	actual, err := descriptorPath(file)
	file.Close()
	if err != nil || !strings.HasPrefix(actual+string(filepath.Separator), rootPath+string(filepath.Separator)) {
		return ".", false
	}
	if event.Has(fsnotify.Create) {
		if err := s.addWatchTree(ctx, watcher, paths, aliases, rootPath, subject, workspaceID, relative); err != nil {
			if errors.Is(err, ErrWatchLimit) {
				return "", true
			}
			return ".", false
		}
	}
	return relative, true
}
