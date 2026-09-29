//go:build darwin

package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

// darwinHostWatcher registers only paths accepted by the guarded FileService.
// fsnotify's kqueue backend implicitly registers every directory child, so it
// cannot enforce a hard node cap or exclude provider-private children here.
type darwinHostWatcher struct {
	mu     sync.Mutex
	kq     int
	closed bool
	byPath map[string]*darwinWatch
	byFD   map[int]*darwinWatch
	events chan fsnotify.Event
	errors chan error
	done   chan struct{}
}

type darwinWatch struct {
	path     string
	fd       int
	dir      bool
	children map[string]watchIdentity
}

type watchIdentity struct{ device, inode uint64 }

func newHostWatcher() (hostWatcher, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	w := &darwinHostWatcher{
		kq: kq, byPath: make(map[string]*darwinWatch), byFD: make(map[int]*darwinWatch),
		events: make(chan fsnotify.Event, 64), errors: make(chan error, 1), done: make(chan struct{}),
	}
	go w.run()
	return w, nil
}

func (w *darwinHostWatcher) Events() <-chan fsnotify.Event { return w.events }
func (w *darwinHostWatcher) Errors() <-chan error          { return w.errors }

func publicChildren(fd int) (map[string]watchIdentity, error) {
	copy, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(copy), "watch-directory")
	defer f.Close()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(MaxWatchScannedEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > MaxWatchScannedEntries {
		return nil, ErrWatchLimit
	}
	children := make(map[string]watchIdentity, len(names))
	for _, name := range names {
		if utf8.ValidString(name) && !privateComponent(name) {
			var stat unix.Stat_t
			if err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err == nil {
				children[name] = watchIdentity{device: uint64(stat.Dev), inode: stat.Ino}
			}
		}
	}
	return children, nil
}

func (w *darwinHostWatcher) Add(name string, file *os.File) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrWatchUnavailable
	}
	if _, exists := w.byPath[name]; exists {
		return nil
	}
	if len(w.byPath) >= MaxWatchNodes {
		return ErrWatchLimit
	}
	if actual, err := descriptorPath(file); err != nil || actual != name {
		return ErrWatchUnavailable
	}
	fd, err := unix.Dup(int(file.Fd()))
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		unix.Close(fd)
		return err
	}
	watch := &darwinWatch{path: name, fd: fd, dir: info.IsDir()}
	if watch.dir {
		watch.children, err = publicChildren(fd)
		if err != nil {
			unix.Close(fd)
			return err
		}
	}
	change := make([]unix.Kevent_t, 1)
	unix.SetKevent(&change[0], fd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_CLEAR|unix.EV_ENABLE)
	change[0].Fflags = unix.NOTE_WRITE | unix.NOTE_DELETE | unix.NOTE_RENAME | unix.NOTE_ATTRIB
	if _, err := unix.Kevent(w.kq, change, nil, nil); err != nil {
		unix.Close(fd)
		return err
	}
	w.byPath[name] = watch
	w.byFD[fd] = watch
	if actual, err := descriptorPath(file); err != nil || actual != name {
		delete(w.byPath, name)
		delete(w.byFD, fd)
		_ = unix.Close(fd)
		return ErrWatchUnavailable
	}
	return nil
}

func (w *darwinHostWatcher) Remove(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	watch := w.byPath[name]
	if watch == nil {
		return nil
	}
	delete(w.byPath, name)
	delete(w.byFD, watch.fd)
	change := make([]unix.Kevent_t, 1)
	unix.SetKevent(&change[0], watch.fd, unix.EVFILT_VNODE, unix.EV_DELETE)
	_, _ = unix.Kevent(w.kq, change, nil, nil)
	return unix.Close(watch.fd)
}

func (w *darwinHostWatcher) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	<-w.done
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, watch := range w.byPath {
		_ = unix.Close(watch.fd)
	}
	w.byPath = nil
	w.byFD = nil
	return unix.Close(w.kq)
}

func (w *darwinHostWatcher) emit(event fsnotify.Event) {
	select {
	case w.events <- event:
	default:
		select {
		case w.errors <- ErrWatchLimit:
		default:
		}
	}
}

func (w *darwinHostWatcher) run() {
	defer close(w.done)
	defer close(w.events)
	defer close(w.errors)
	buffer := make([]unix.Kevent_t, 32)
	for {
		w.mu.Lock()
		closed := w.closed
		w.mu.Unlock()
		if closed {
			return
		}
		timeout := unix.NsecToTimespec((100 * time.Millisecond).Nanoseconds())
		count, err := unix.Kevent(w.kq, nil, buffer, &timeout)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			select {
			case w.errors <- err:
			default:
			}
			return
		}
		for _, event := range buffer[:count] {
			w.process(event)
		}
	}
}

func (w *darwinHostWatcher) process(event unix.Kevent_t) {
	w.mu.Lock()
	watch := w.byFD[int(event.Ident)]
	if watch == nil || w.closed {
		w.mu.Unlock()
		return
	}
	name := watch.path
	flags := event.Fflags
	if flags&(unix.NOTE_DELETE|unix.NOTE_RENAME) != 0 {
		w.mu.Unlock()
		w.emit(fsnotify.Event{Name: name, Op: fsnotify.Remove})
		return
	}
	if watch.dir && flags&unix.NOTE_WRITE != 0 {
		current, err := publicChildren(watch.fd)
		if err != nil {
			w.mu.Unlock()
			select {
			case w.errors <- err:
			default:
			}
			return
		}
		previous := watch.children
		watch.children = current
		w.mu.Unlock()
		for child, oldID := range previous {
			if currentID, exists := current[child]; !exists || currentID != oldID {
				w.emit(fsnotify.Event{Name: filepath.Join(name, child), Op: fsnotify.Remove})
			}
		}
		for child, currentID := range current {
			if oldID, exists := previous[child]; !exists || currentID != oldID {
				w.emit(fsnotify.Event{Name: filepath.Join(name, child), Op: fsnotify.Create})
			}
		}
		return
	}
	w.mu.Unlock()
	if flags&unix.NOTE_WRITE != 0 {
		w.emit(fsnotify.Event{Name: name, Op: fsnotify.Write})
	} else if flags&unix.NOTE_ATTRIB != 0 {
		w.emit(fsnotify.Event{Name: name, Op: fsnotify.Chmod})
	}
}
