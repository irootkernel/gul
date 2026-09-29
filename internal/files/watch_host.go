package files

import (
	"os"

	"github.com/fsnotify/fsnotify"
)

type hostWatcher interface {
	Add(string, *os.File) error
	Remove(string) error
	Close() error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
}
