//go:build !darwin

package files

import (
	"os"

	"github.com/fsnotify/fsnotify"
)

type fsHostWatcher struct{ *fsnotify.Watcher }

func newHostWatcher() (hostWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &fsHostWatcher{watcher}, nil
}

func (w *fsHostWatcher) Events() <-chan fsnotify.Event { return w.Watcher.Events }
func (w *fsHostWatcher) Errors() <-chan error          { return w.Watcher.Errors }
func (w *fsHostWatcher) Add(path string, _ *os.File) error {
	return w.Watcher.Add(path)
}
