// Package recovery coordinates bounded recovery through explicit runtime ports.
package recovery

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

var (
	ErrIncompatible = errors.New("runtime protocol incompatible")
	ErrExhausted    = errors.New("runtime restart budget exhausted")
	ErrRunning      = errors.New("runtime supervisor already running")
	ErrInvalid      = errors.New("invalid runtime lifecycle")
)

const (
	restartWindow = time.Minute
	stableWindow  = 5 * time.Minute
	maxStarts     = 5
	maxDelay      = 30 * time.Second
)

// Lifecycle verifies the executable and starts the owned gateway. Start returns
// only after readiness and protocol compatibility have been checked. It owns
// startup deadlines and cleanup of unsuccessful starts. It must wrap
// ErrIncompatible when compatibility prevents an automatic retry.
type Lifecycle interface {
	Start(context.Context) (Gateway, error)
}

// Gateway waits for the ready gateway to exit. On cancellation it must complete
// bounded shutdown of its owned child before returning. A clean unexpected exit
// (nil) is also a crash for supervision purposes; intentional shutdown cancels
// the context. Neither this port nor Supervisor changes durable Run state.
type Gateway interface {
	Wait(context.Context) error
}

type Clock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
func (systemClock) Sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Supervisor runs one lifecycle at a time and retains budget and terminal
// blockers across calls to Run. Construct a new supervisor only for an explicit
// new host lifecycle, never to bypass an exhausted automatic-restart budget.
type Supervisor struct {
	lifecycle Lifecycle
	clock     Clock
	random    func() float64
	mu        sync.Mutex
	running   bool
	terminal  error
	starts    []time.Time
	failures  int
}

// NewSupervisor defaults to the system clock and uniform randomness. A supplied
// random function must return values in [0, 1], and can provide deterministic
// boundary values in fixtures.
func NewSupervisor(lifecycle Lifecycle, clock Clock, random func() float64) *Supervisor {
	if clock == nil {
		clock = systemClock{}
	}
	if random == nil {
		random = rand.Float64
	}
	return &Supervisor{lifecycle: lifecycle, clock: clock, random: random}
}

// Run starts immediately on its first invocation. Each subsequent crash waits
// for jittered exponential backoff. All starts, including the first and failed
// readiness attempts, consume the rolling budget. Exhaustion is terminal rather
// than an instruction to wait for the oldest start to expire.
func (s *Supervisor) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrRunning
	}
	if s.terminal != nil {
		err := s.terminal
		s.mu.Unlock()
		return err
	}
	if s.lifecycle == nil {
		s.mu.Unlock()
		return ErrInvalid
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.canStart() {
			return s.stop(ErrExhausted)
		}
		if s.failures > 0 {
			if err := s.clock.Sleep(ctx, s.delay()); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !s.canStart() {
			return s.stop(ErrExhausted)
		}
		s.starts = append(s.starts, s.clock.Now())
		gateway, err := s.lifecycle.Start(ctx)
		if err == nil {
			if gateway == nil {
				return s.stop(ErrInvalid)
			}
			readyAt := s.clock.Now()
			err = gateway.Wait(ctx)
			if s.clock.Now().Sub(readyAt) >= stableWindow {
				s.starts = nil
				s.failures = 0
			}
		}
		if errors.Is(err, ErrIncompatible) {
			return s.stop(ErrIncompatible)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Saturating the exponent also prevents overflow after long operation.
		if s.failures < 6 {
			s.failures++
		}
	}
}

func (s *Supervisor) stop(err error) error {
	s.mu.Lock()
	s.terminal = err
	s.mu.Unlock()
	return err
}

func (s *Supervisor) canStart() bool {
	cutoff := s.clock.Now().Add(-restartWindow)
	first := 0
	for first < len(s.starts) && !s.starts[first].After(cutoff) {
		first++
	}
	s.starts = s.starts[first:]
	return len(s.starts) < maxStarts
}

func (s *Supervisor) delay() time.Duration {
	base := min(time.Second<<uint(s.failures-1), maxDelay)
	return min(time.Duration(float64(base)*(0.8+0.4*s.random())), maxDelay)
}
