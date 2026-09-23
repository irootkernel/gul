package desktop

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Core is the same lifecycle boundary used by the headless host.
type Core interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// WindowHost delivers the checked frontend bundle without owning product behavior.
type WindowHost interface {
	Run(onShutdown func()) error
}

type Shell struct {
	core Core
	host WindowHost
}

func NewShell(core Core, host WindowHost) *Shell {
	return &Shell{core: core, host: host}
}

// Run starts the shared core before the desktop window and stops it on every
// window shutdown or run failure. It never starts a separate provider path.
func (s *Shell) Run(ctx context.Context) error {
	if s == nil || s.core == nil || s.host == nil {
		return errors.New("Gul desktop shell is not configured")
	}
	if err := s.core.Start(ctx); err != nil {
		return fmt.Errorf("start Gul desktop core: %w", err)
	}
	var once sync.Once
	var stopErr error
	stop := func() {
		once.Do(func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stopErr = s.core.Stop(stopCtx)
		})
	}
	err := s.host.Run(stop)
	stop()
	return errors.Join(err, stopErr)
}
