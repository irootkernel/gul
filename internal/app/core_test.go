package app

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestDefaultCompositionStartsHeadlesslyAndDeniesProductAccess(t *testing.T) {
	core := NewCore(Dependencies{})
	if err := core.Start(t.Context()); err != nil {
		t.Fatalf("start default core: %v", err)
	}
	if !core.Running() {
		t.Fatal("default core should be running")
	}
	if err := core.RequireProductAccess(t.Context(), Principal{Subject: "test"}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("default access error = %v, want access denied", err)
	}
	if err := core.Stop(t.Context()); err != nil {
		t.Fatalf("stop default core: %v", err)
	}
	if core.Running() {
		t.Fatal("stopped default core should not be running")
	}
}

func TestNilProviderAndPersistenceFailClosed(t *testing.T) {
	t.Run("provider", func(t *testing.T) {
		var calls []string
		core := NewCore(Dependencies{
			Authorization: recordingAuthorization{},
			Persistence:   recordingReadiness{name: "persistence", calls: &calls},
		})
		if err := core.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := core.RequireProductAccess(t.Context(), Principal{}); !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("access error = %v, want provider unavailable", err)
		}
		if len(calls) != 0 {
			t.Fatalf("provider failure called persistence: %v", calls)
		}
	})

	t.Run("persistence", func(t *testing.T) {
		core := NewCore(Dependencies{
			Authorization: recordingAuthorization{},
			Provider:      recordingReadiness{},
		})
		if err := core.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := core.RequireProductAccess(t.Context(), Principal{}); !errors.Is(err, ErrPersistenceUnavailable) {
			t.Fatalf("access error = %v, want persistence unavailable", err)
		}
	})
}

func TestInjectedPortsRunInFailClosedOrder(t *testing.T) {
	var calls []string
	core := NewCore(Dependencies{
		Lifecycle:     &recordingLifecycle{calls: &calls},
		Authorization: recordingAuthorization{calls: &calls},
		Provider:      recordingReadiness{name: "provider", calls: &calls},
		Persistence:   recordingReadiness{name: "persistence", calls: &calls},
	})

	if err := core.Start(t.Context()); err != nil {
		t.Fatalf("start core: %v", err)
	}
	if err := core.RequireProductAccess(t.Context(), Principal{Subject: "fixture-principal"}); err != nil {
		t.Fatalf("require product access: %v", err)
	}
	if err := core.Stop(t.Context()); err != nil {
		t.Fatalf("stop core: %v", err)
	}

	want := []string{"start", "authorize:fixture-principal", "provider", "persistence", "stop"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestAuthorizationFailureDoesNotProbeAvailability(t *testing.T) {
	var calls []string
	core := NewCore(Dependencies{
		Authorization: recordingAuthorization{calls: &calls, err: errors.New("no principal")},
		Provider:      recordingReadiness{name: "provider", calls: &calls},
		Persistence:   recordingReadiness{name: "persistence", calls: &calls},
	})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	err := core.RequireProductAccess(t.Context(), Principal{})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("access error = %v, want access denied", err)
	}
	if want := []string{"authorize:"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestAvailabilityFailuresStayTyped(t *testing.T) {
	tests := []struct {
		name           string
		providerErr    error
		persistenceErr error
		want           error
	}{
		{name: "provider", providerErr: errors.New("offline"), want: ErrProviderUnavailable},
		{name: "persistence", persistenceErr: errors.New("closed"), want: ErrPersistenceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core := NewCore(Dependencies{
				Authorization: recordingAuthorization{},
				Provider:      recordingReadiness{err: test.providerErr},
				Persistence:   recordingReadiness{err: test.persistenceErr},
			})
			if err := core.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := core.RequireProductAccess(t.Context(), Principal{Subject: "fixture"}); !errors.Is(err, test.want) {
				t.Fatalf("access error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestLifecycleFailureDoesNotExposeRunningCore(t *testing.T) {
	startErr := errors.New("bootstrap failed")
	core := NewCore(Dependencies{Lifecycle: &recordingLifecycle{startErr: startErr}})
	if err := core.Start(t.Context()); !errors.Is(err, startErr) {
		t.Fatalf("start error = %v, want %v", err, startErr)
	}
	if core.Running() {
		t.Fatal("failed core must not be running")
	}
	if err := core.RequireProductAccess(t.Context(), Principal{}); !errors.Is(err, ErrCoreNotRunning) {
		t.Fatalf("access error = %v, want core not running", err)
	}
	if err := core.Start(t.Context()); !errors.Is(err, startErr) {
		t.Fatalf("retry start error = %v, want %v", err, startErr)
	}
}

func TestFailedStopLeavesCoreRunning(t *testing.T) {
	stopErr := errors.New("shutdown failed")
	core := NewCore(Dependencies{Lifecycle: &recordingLifecycle{stopErr: stopErr}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := core.Stop(t.Context()); !errors.Is(err, stopErr) {
		t.Fatalf("stop error = %v, want %v", err, stopErr)
	}
	if !core.Running() {
		t.Fatal("failed stop must leave core running")
	}
}

func TestStopWaitsForStartAndThenStops(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	lifecycle := &blockingLifecycle{started: started, release: release}
	core := NewCore(Dependencies{Lifecycle: lifecycle})
	startResult := make(chan error, 1)

	go func() { startResult <- core.Start(t.Context()) }()
	<-started
	stopContext, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := core.Stop(stopContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("overlapping stop error = %v, want deadline exceeded", err)
	}
	close(release)
	if err := <-startResult; err != nil {
		t.Fatalf("start core: %v", err)
	}
	if err := core.Stop(t.Context()); err != nil {
		t.Fatalf("stop core: %v", err)
	}
	if core.Running() {
		t.Fatal("core should be stopped")
	}
}

func TestStopClosesAccessGateBeforeProviderProbe(t *testing.T) {
	authorized := make(chan struct{})
	release := make(chan struct{})
	var calls []string
	stopEntered := make(chan struct{})
	lifecycle := &observingLifecycle{stopEntered: stopEntered}
	core := NewCore(Dependencies{
		Lifecycle:     lifecycle,
		Authorization: blockingAuthorization{started: authorized, release: release},
		Provider:      recordingReadiness{name: "provider", calls: &calls},
		Persistence:   recordingReadiness{name: "persistence", calls: &calls},
	})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	accessResult := make(chan error, 1)
	stopResult := make(chan error, 1)
	go func() { accessResult <- core.RequireProductAccess(t.Context(), Principal{}) }()
	<-authorized
	go func() { stopResult <- core.Stop(t.Context()) }()
	deadline := time.Now().Add(time.Second)
	for core.Running() && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if core.Running() {
		t.Fatal("stop did not close the access gate")
	}
	select {
	case <-stopEntered:
		t.Fatal("lifecycle stop started before in-flight access drained")
	default:
	}
	close(release)
	if err := <-accessResult; !errors.Is(err, ErrCoreNotRunning) {
		t.Fatalf("access error = %v, want core not running", err)
	}
	if len(calls) != 0 {
		t.Fatalf("stop allowed readiness probes: %v", calls)
	}
	if err := <-stopResult; err != nil {
		t.Fatalf("stop core: %v", err)
	}
	select {
	case <-stopEntered:
	default:
		t.Fatal("lifecycle stop was not called after access drained")
	}
}

func TestStopDuringProviderProbeSkipsPersistence(t *testing.T) {
	providerEntered := make(chan struct{})
	releaseProvider := make(chan struct{})
	var calls []string
	core := NewCore(Dependencies{
		Lifecycle:     &recordingLifecycle{},
		Authorization: recordingAuthorization{},
		Provider:      blockingReadiness{started: providerEntered, release: releaseProvider},
		Persistence:   recordingReadiness{name: "persistence", calls: &calls},
	})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	accessResult := make(chan error, 1)
	stopResult := make(chan error, 1)
	go func() { accessResult <- core.RequireProductAccess(t.Context(), Principal{}) }()
	<-providerEntered
	go func() { stopResult <- core.Stop(t.Context()) }()
	waitUntilNotRunning(t, core)
	close(releaseProvider)
	if err := <-accessResult; !errors.Is(err, ErrCoreNotRunning) {
		t.Fatalf("access error = %v, want core not running", err)
	}
	if len(calls) != 0 {
		t.Fatalf("stop allowed persistence probe: %v", calls)
	}
	if err := <-stopResult; err != nil {
		t.Fatalf("stop core: %v", err)
	}
}

func TestCanceledLifecycleRequestsDoNotChangeState(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	core := NewCore(Dependencies{})
	if err := core.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled start error = %v, want context canceled", err)
	}
	if core.Running() {
		t.Fatal("canceled start must not leave core running")
	}
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := core.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled stop error = %v, want context canceled", err)
	}
	if !core.Running() {
		t.Fatal("canceled stop must leave core running")
	}
}

func waitUntilNotRunning(t *testing.T, core *Core) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for core.Running() && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if core.Running() {
		t.Fatal("stop did not close the access gate")
	}
}

func TestStoppedCoreFailsBeforeInjectedPorts(t *testing.T) {
	var calls []string
	core := NewCore(Dependencies{
		Authorization: recordingAuthorization{calls: &calls},
		Provider:      recordingReadiness{name: "provider", calls: &calls},
		Persistence:   recordingReadiness{name: "persistence", calls: &calls},
	})
	if err := core.RequireProductAccess(context.Background(), Principal{Subject: "fixture"}); !errors.Is(err, ErrCoreNotRunning) {
		t.Fatalf("access error = %v, want core not running", err)
	}
	if len(calls) != 0 {
		t.Fatalf("stopped core called injected ports: %v", calls)
	}
}

type recordingLifecycle struct {
	calls    *[]string
	startErr error
	stopErr  error
}

type blockingLifecycle struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (l *blockingLifecycle) Start(context.Context) error {
	close(l.started)
	<-l.release
	return nil
}

func (*blockingLifecycle) Stop(context.Context) error { return nil }

type observingLifecycle struct {
	stopEntered chan<- struct{}
}

func (*observingLifecycle) Start(context.Context) error { return nil }

func (l *observingLifecycle) Stop(context.Context) error {
	close(l.stopEntered)
	return nil
}

func (l *recordingLifecycle) Start(context.Context) error {
	if l.calls != nil {
		*l.calls = append(*l.calls, "start")
	}
	return l.startErr
}

func (l *recordingLifecycle) Stop(context.Context) error {
	if l.calls != nil {
		*l.calls = append(*l.calls, "stop")
	}
	return l.stopErr
}

type recordingAuthorization struct {
	calls *[]string
	err   error
}

type blockingAuthorization struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (a blockingAuthorization) Authorize(context.Context, Principal) error {
	close(a.started)
	<-a.release
	return nil
}

func (a recordingAuthorization) Authorize(_ context.Context, principal Principal) error {
	if a.calls != nil {
		*a.calls = append(*a.calls, "authorize:"+principal.Subject)
	}
	return a.err
}

type recordingReadiness struct {
	name  string
	calls *[]string
	err   error
}

type blockingReadiness struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (r blockingReadiness) Ready(context.Context) error {
	close(r.started)
	<-r.release
	return nil
}

func (r recordingReadiness) Ready(context.Context) error {
	if r.calls != nil {
		*r.calls = append(*r.calls, r.name)
	}
	return r.err
}
