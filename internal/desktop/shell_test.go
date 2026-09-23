package desktop

import (
	"context"
	"errors"
	"testing"
)

type fakeCore struct {
	started         int
	stopped         int
	startErr        error
	stopErr         error
	stopCtxErr      error
	stopHasDeadline bool
}

func (core *fakeCore) Start(context.Context) error {
	core.started++
	return core.startErr
}

func (core *fakeCore) Stop(ctx context.Context) error {
	core.stopped++
	core.stopCtxErr = ctx.Err()
	_, core.stopHasDeadline = ctx.Deadline()
	return core.stopErr
}

type fakeHost struct {
	runs            int
	runErr          error
	shutdowns       int
	observedStarted bool
	core            *fakeCore
	onRun           func()
}

func (host *fakeHost) Run(onShutdown func()) error {
	host.runs++
	host.observedStarted = host.core.started == 1 && host.core.stopped == 0
	if host.onRun != nil {
		host.onRun()
	}
	for range host.shutdowns {
		onShutdown()
	}
	return host.runErr
}

func TestShellStartsAndStopsSharedCoreOnce(t *testing.T) {
	core := &fakeCore{}
	host := &fakeHost{core: core, shutdowns: 2}
	if err := NewShell(core, host).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !host.observedStarted || host.runs != 1 || core.started != 1 || core.stopped != 1 {
		t.Fatalf("lifecycle = host %+v, core %+v", host, core)
	}
}

func TestShellFailurePaths(t *testing.T) {
	startError := errors.New("start rejected")
	core := &fakeCore{startErr: startError}
	host := &fakeHost{core: core}
	if err := NewShell(core, host).Run(t.Context()); !errors.Is(err, startError) || host.runs != 0 || core.stopped != 0 {
		t.Fatalf("failed start = %v, host %+v, core %+v", err, host, core)
	}
	runError := errors.New("window failed")
	stopError := errors.New("stop failed")
	core = &fakeCore{stopErr: stopError}
	host = &fakeHost{core: core, runErr: runError}
	if err := NewShell(core, host).Run(t.Context()); !errors.Is(err, runError) || !errors.Is(err, stopError) || core.stopped != 1 {
		t.Fatalf("failed window = %v, core %+v", err, core)
	}
	if err := NewShell(nil, host).Run(t.Context()); err == nil {
		t.Fatal("accepted missing shared core")
	}
	if err := NewShell(core, nil).Run(t.Context()); err == nil {
		t.Fatal("accepted missing window host")
	}
}

func TestShellStopsWithFreshBoundedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	core := &fakeCore{}
	host := &fakeHost{core: core, onRun: cancel}
	if err := NewShell(core, host).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if core.stopped != 1 || core.stopCtxErr != nil || !core.stopHasDeadline {
		t.Fatalf("stop context = stopped %d, error %v, deadline %t", core.stopped, core.stopCtxErr, core.stopHasDeadline)
	}
}

type concurrentHost struct {
	done chan struct{}
}

func (host *concurrentHost) Run(onShutdown func()) error {
	go func() {
		defer close(host.done)
		onShutdown()
	}()
	return nil
}

func TestShellConcurrentShutdownStopsOnce(t *testing.T) {
	core := &fakeCore{}
	host := &concurrentHost{done: make(chan struct{})}
	if err := NewShell(core, host).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-host.done
	if core.stopped != 1 {
		t.Fatalf("stop calls = %d", core.stopped)
	}
}
