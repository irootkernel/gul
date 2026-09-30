package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/host"
)

func commandConfig(t *testing.T) host.Config {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return host.Config{DataDirectory: filepath.Join(base, "Gul"), Port: port}
}

func noDesktop(host.Attachment, func()) error { return errors.New("unexpected desktop launch") }

func TestCommandDispatchRejectsBadInputWithoutCreatingOwner(t *testing.T) {
	for _, test := range []struct {
		args    []string
		message string
	}{
		{[]string{"unknown"}, "serve|launchd-plist|diagnose"},
		{[]string{"serve", "--unknown"}, "flag provided but not defined"},
		{[]string{"launchd-plist"}, "data root must be"},
		{[]string{"diagnose"}, "no Gul owner state"},
	} {
		c := commandConfig(t)
		args := append(test.args, "--data-directory", c.DataDirectory)
		err := runCommand(t.Context(), args, noDesktop)
		if err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("%v: %v", args, err)
		}
		if _, err := os.Stat(c.DataDirectory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dispatch wrote owner state: %v", err)
		}
	}
}

func TestAttachedDesktopClosePreservesHeadlessOwner(t *testing.T) {
	c := commandConfig(t)
	owner := host.New(c)
	if err := owner.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	opened := false
	err := runCommand(t.Context(), []string{"--data-directory", c.DataDirectory}, func(view host.Attachment, closeWindow func()) error {
		opened = true
		if view.Origin != owner.Origin() || len(view.CertificateDER) == 0 {
			t.Fatal("unverified desktop attachment")
		}
		closeWindow()
		return nil
	})
	if err != nil || !opened {
		t.Fatalf("attach/close: %v", err)
	}
	if _, err := host.Verify(t.Context(), c.DataDirectory); err != nil {
		t.Fatalf("window close stopped owner: %v", err)
	}
	if err := host.New(c).Start(t.Context()); !errors.Is(err, host.ErrAlreadyRunning) {
		t.Fatalf("window close released owner lock: %v", err)
	}
	if err := runCommand(t.Context(), []string{"diagnose", "--data-directory", c.DataDirectory}, noDesktop); err != nil {
		t.Fatalf("diagnose owner: %v", err)
	}
	if err := runCommand(t.Context(), []string{"serve", "--data-directory", c.DataDirectory}, noDesktop); !errors.Is(err, host.ErrAlreadyRunning) {
		t.Fatalf("second headless invocation: %v", err)
	}
}

func TestServeCancellationStopsOwnedHost(t *testing.T) {
	c := commandConfig(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- runCommand(ctx, []string{"serve", "--data-directory", c.DataDirectory, "--port", strconv.Itoa(c.Port)}, noDesktop)
	}()
	ready := false
	for !ready {
		if _, err := host.Verify(ctx, c.DataDirectory); err == nil {
			ready = true
			break
		}
		select {
		case err := <-result:
			t.Fatalf("serve ended before readiness: %v", err)
		case <-ctx.Done():
			t.Fatal("serve readiness timed out")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve cancel: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("serve drain timed out")
	}
	if _, err := host.Verify(t.Context(), c.DataDirectory); !errors.Is(err, host.ErrUnverifiedOwner) {
		t.Fatalf("stopped owner still healthy: %v", err)
	}
	next := host.New(c)
	if err := next.Start(t.Context()); err != nil {
		t.Fatalf("serve retained released lock: %v", err)
	}
	if err := next.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOwnedDesktopCloseDrainsHost(t *testing.T) {
	c := commandConfig(t)
	opened := false
	err := runCommand(t.Context(), []string{"--data-directory", c.DataDirectory, "--port", strconv.Itoa(c.Port)}, func(view host.Attachment, closeWindow func()) error {
		opened = true
		if _, err := host.Verify(t.Context(), c.DataDirectory); err != nil {
			t.Fatalf("desktop owner unverified: %v", err)
		}
		closeWindow()
		return nil
	})
	if err != nil || !opened {
		t.Fatalf("owned desktop close: %v", err)
	}
	if _, err := host.Verify(t.Context(), c.DataDirectory); !errors.Is(err, host.ErrUnverifiedOwner) {
		t.Fatalf("owned desktop did not drain: %v", err)
	}
	next := host.New(c)
	if err := next.Start(t.Context()); err != nil {
		t.Fatalf("owned desktop retained lock: %v", err)
	}
	if err := next.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDiagnoseSeparatesInvalidPathAndUnsafeState(t *testing.T) {
	if err := runCommand(t.Context(), []string{"diagnose", "--data-directory", "Gul"}, noDesktop); err == nil || !strings.Contains(err.Error(), "absolute and clean") {
		t.Fatalf("invalid input misclassified: %v", err)
	}
	c := commandConfig(t)
	if err := os.Mkdir(c.DataDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(t.Context(), []string{"diagnose", "--data-directory", c.DataDirectory}, noDesktop); !errors.Is(err, host.ErrUnsafeState) {
		t.Fatalf("unsafe state misclassified: %v", err)
	}
}
