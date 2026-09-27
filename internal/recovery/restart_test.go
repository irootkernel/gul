package recovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type fakeClock struct {
	now     time.Time
	sleeps  []time.Duration
	onSleep func()
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	if c.onSleep != nil {
		c.onSleep()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.now = c.now.Add(d)
	return nil
}

type lifecycleFunc func(context.Context) (Gateway, error)

func (f lifecycleFunc) Start(ctx context.Context) (Gateway, error) { return f(ctx) }

type gatewayFunc func(context.Context) error

func (f gatewayFunc) Wait(ctx context.Context) error { return f(ctx) }

func TestRestartExhaustionCountsInitialAndFailedStarts(t *testing.T) {
	for _, failReadiness := range []bool{false, true} {
		t.Run(fmt.Sprint(failReadiness), func(t *testing.T) {
			clock := &fakeClock{now: time.Unix(1, 0)}
			starts := 0
			s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
				starts++
				if failReadiness {
					return nil, errors.New("readiness unavailable")
				}
				return gatewayFunc(func(context.Context) error { return nil }), nil
			}), clock, func() float64 { return .5 })
			if err := s.Run(context.Background()); !errors.Is(err, ErrExhausted) {
				t.Fatal(err)
			}
			if starts != 5 || !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}) {
				t.Fatalf("starts=%d waits=%v", starts, clock.sleeps)
			}
			clock.now = clock.now.Add(time.Hour)
			if err := s.Run(context.Background()); !errors.Is(err, ErrExhausted) || starts != 5 {
				t.Fatalf("terminal exhaustion bypassed: starts=%d err=%v", starts, err)
			}
		})
	}
}

func TestRestartJitterSequenceAndCap(t *testing.T) {
	for _, value := range []float64{0, .5, 1} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			clock := &fakeClock{now: time.Unix(1, 0)}
			starts := 0
			s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
				starts++
				if starts == 9 {
					return nil, ErrIncompatible
				}
				return gatewayFunc(func(context.Context) error {
					// Long enough to age out starts, too short to reset backoff.
					clock.now = clock.now.Add(time.Minute)
					return errors.New("crashed")
				}), nil
			}), clock, func() float64 { return value })
			if err := s.Run(context.Background()); !errors.Is(err, ErrIncompatible) {
				t.Fatal(err)
			}
			bases := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
			if len(clock.sleeps) != len(bases) {
				t.Fatalf("waits=%v", clock.sleeps)
			}
			for i, base := range bases {
				want := min(time.Duration(float64(base)*(.8+.4*value)), 30*time.Second)
				if clock.sleeps[i] != want {
					t.Fatalf("wait %d=%v want %v", i, clock.sleeps[i], want)
				}
			}
		})
	}
}

func TestRestartRollingWindowBoundary(t *testing.T) {
	for _, finalUptime := range []time.Duration{45*time.Second - time.Nanosecond, 45 * time.Second} {
		t.Run(finalUptime.String(), func(t *testing.T) {
			clock := &fakeClock{now: time.Unix(1, 0)}
			starts := 0
			s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
				starts++
				if starts == 6 {
					return nil, ErrIncompatible
				}
				return gatewayFunc(func(context.Context) error {
					if starts == 5 {
						// The fifth start follows 1+2+4+8 seconds of delay.
						clock.now = clock.now.Add(finalUptime)
					}
					return nil
				}), nil
			}), clock, func() float64 { return .5 })
			wantErr, wantStarts := ErrExhausted, 5
			if finalUptime == 45*time.Second {
				wantErr, wantStarts = ErrIncompatible, 6
			}
			if err := s.Run(context.Background()); !errors.Is(err, wantErr) || starts != wantStarts {
				t.Fatalf("starts=%d err=%v", starts, err)
			}
		})
	}
}

func TestRestartStableReadinessResetsBackoff(t *testing.T) {
	for _, uptime := range []time.Duration{5*time.Minute - time.Nanosecond, 5 * time.Minute} {
		t.Run(uptime.String(), func(t *testing.T) {
			clock := &fakeClock{now: time.Unix(1, 0)}
			starts := 0
			s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
				starts++
				if starts == 3 {
					return nil, ErrIncompatible
				}
				return gatewayFunc(func(context.Context) error {
					if starts == 2 {
						clock.now = clock.now.Add(uptime)
					}
					return nil
				}), nil
			}), clock, func() float64 { return .5 })
			if err := s.Run(context.Background()); !errors.Is(err, ErrIncompatible) {
				t.Fatal(err)
			}
			want := 2 * time.Second
			if uptime == 5*time.Minute {
				want = time.Second
			}
			if !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second, want}) || len(s.starts) != 1 {
				t.Fatalf("waits=%v starts=%v", clock.sleeps, s.starts)
			}
		})
	}
}

func TestRestartSlowReadinessDoesNotResetBackoff(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1, 0)}
	starts := 0
	s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
		starts++
		if starts == 3 {
			return nil, ErrIncompatible
		}
		clock.now = clock.now.Add(5 * time.Minute)
		return nil, errors.New("readiness failed")
	}), clock, func() float64 { return .5 })
	if err := s.Run(context.Background()); !errors.Is(err, ErrIncompatible) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(clock.sleeps, []time.Duration{time.Second, 2 * time.Second}) {
		t.Fatal(clock.sleeps)
	}
}

func TestRestartIncompatibilityIsTerminal(t *testing.T) {
	for _, duringWait := range []bool{false, true} {
		t.Run(fmt.Sprint(duringWait), func(t *testing.T) {
			clock := &fakeClock{now: time.Unix(1, 0)}
			starts := 0
			s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
				starts++
				err := fmt.Errorf("handshake: %w", ErrIncompatible)
				if duringWait {
					return gatewayFunc(func(context.Context) error { return err }), nil
				}
				return nil, err
			}), clock, nil)
			for range 2 {
				if err := s.Run(context.Background()); !errors.Is(err, ErrIncompatible) {
					t.Fatal(err)
				}
			}
			if starts != 1 || len(clock.sleeps) != 0 {
				t.Fatalf("incompatible runtime retried: starts=%d waits=%v", starts, clock.sleeps)
			}
		})
	}
}

func TestRestartCancellationDuringBackoffDoesNotStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &fakeClock{now: time.Unix(1, 0), onSleep: cancel}
	starts := 0
	s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
		starts++
		return nil, errors.New("crashed")
	}), clock, nil)
	if err := s.Run(ctx); !errors.Is(err, context.Canceled) || starts != 1 {
		t.Fatalf("starts=%d err=%v", starts, err)
	}
}

func TestRestartWaitCancellationAndConcurrentRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) {
		return gatewayFunc(func(ctx context.Context) error {
			close(ready)
			<-ctx.Done()
			return ctx.Err()
		}), nil
	}), nil, nil)
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	<-ready
	if err := s.Run(ctx); !errors.Is(err, ErrRunning) {
		t.Fatalf("concurrent lifecycle allowed: %v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRestartRejectsMissingLifecycleAndGateway(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1, 0)}
	if err := NewSupervisor(nil, clock, nil).Run(t.Context()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	starts := 0
	s := NewSupervisor(lifecycleFunc(func(context.Context) (Gateway, error) { starts++; return nil, nil }), clock, nil)
	for range 2 {
		if err := s.Run(t.Context()); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if starts != 1 || len(clock.sleeps) != 0 {
		t.Fatal(starts, clock.sleeps)
	}
}
