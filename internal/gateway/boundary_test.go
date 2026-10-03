package gateway

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/app"
)

type boundaryAccess struct{}

func (boundaryAccess) Authorize(context.Context, app.Principal) error { return nil }
func (boundaryAccess) Ready(context.Context) error                    { return nil }

func TestDelayedReadinessDoesNotDispatchProductMutation(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint("timeout=", timeout), func(t *testing.T) {
			g, runtime, run := transportFixture(t)
			release := make(chan struct{})
			runtime.capabilityBlock = release
			core := app.NewCore(app.Dependencies{Provider: g, Persistence: boundaryAccess{}, Authorization: boundaryAccess{}})
			if err := core.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = core.Stop(context.Background()) })
			result := make(chan error, 1)
			started := time.Now()
			go func() {
				err := core.RequireProductAccess(t.Context(), app.Principal{Subject: "test-owner"})
				if err == nil {
					_, err = g.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{})
				}
				result <- err
			}()
			select {
			case request := <-runtime.requests:
				if request.ProtocolVersion != 0 {
					t.Fatal("readiness did not use protocol zero")
				}
			case <-time.After(time.Second):
				t.Fatal("readiness handshake did not reach the public socket")
			}
			select {
			case err := <-result:
				t.Fatal("product call settled before readiness", err)
			case <-time.After(250 * time.Millisecond):
			}
			if run.calls.Load() != 0 || g.Status().ChannelReady {
				t.Fatal("mutation dispatched while readiness was pending")
			}
			if !timeout {
				close(release)
			}
			select {
			case err := <-result:
				if timeout {
					if !errors.Is(err, app.ErrProviderUnavailable) || run.calls.Load() != 0 || g.Status().ChannelReady {
						t.Fatal("readiness timeout admitted mutation", err)
					}
					if elapsed := time.Since(started); elapsed < 4750*time.Millisecond || elapsed > 6*time.Second {
						t.Fatal("readiness did not expire at its five-second budget", elapsed)
					}
				} else if run.calls.Load() != 1 || !g.Status().ChannelReady || connect.CodeOf(err) != connect.CodeFailedPrecondition {
					t.Fatal("ready product call did not reach the provider exactly once", err)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("readiness exceeded its budget")
			}
		})
	}
}

func TestNegotiationExpiresWithDelayedPublicHandshake(t *testing.T) {
	g, runtime, _ := transportFixture(t)
	runtime.capabilityBlock = make(chan struct{})
	started := time.Now()
	caps, err := negotiate(t.Context(), g.channel)
	if caps != nil || connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Fatal("delayed startup handshake was accepted", caps, err)
	}
	if request := <-runtime.requests; request.ProtocolVersion != 0 {
		t.Fatal("startup handshake did not use protocol zero")
	}
	if elapsed := time.Since(started); elapsed < 4750*time.Millisecond || elapsed > 6*time.Second {
		t.Fatal("startup handshake exceeded its five-second budget", elapsed)
	}
}

func TestGatewayBoundaryChildProcess(t *testing.T) {
	if os.Getenv("GUL_GATEWAY_BOUNDARY_CHILD") != "1" {
		return
	}
	terminated := make(chan os.Signal, 1)
	signal.Notify(terminated, syscall.SIGTERM)
	fmt.Println("ready")
	for range terminated {
		fmt.Println("term")
	}
}

func boundaryChild(t *testing.T) (*exec.Cmd, chan struct{}, <-chan string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestGatewayBoundaryChildProcess$")
	cmd.Dir = home
	cmd.Env = []string{"HOME=" + home, "TMPDIR=" + home, "PATH=/usr/bin:/bin", "GUL_GATEWAY_BOUNDARY_CHILD=1"}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited, messages := make(chan struct{}), make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			messages <- scanner.Text()
		}
		close(messages)
	}()
	go func() { _ = cmd.Wait(); close(exited) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-exited })
	select {
	case message := <-messages:
		if message != "ready" {
			t.Fatal("child failed to install its signal handler", message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child did not become ready")
	}
	return cmd, exited, messages
}

func TestShutdownDrainsUnaryAndKillsOnlyOwnedChildAfterGrace(t *testing.T) {
	owned, exited, signals := boundaryChild(t)
	other, otherExited, otherSignals := boundaryChild(t)
	g, runtime, _ := transportFixture(t)
	g.child, g.exited = owned, exited
	_, g.cancel = context.WithCancel(context.Background())
	g.done = make(chan struct{})
	close(g.done)
	read := make(chan error, 1)
	go func() { _, err := g.ListProfiles(t.Context(), &publicv1.ListProfilesRequest{}); read <- err }()
	select {
	case <-runtime.requests:
	case <-time.After(time.Second):
		t.Fatal("unary did not start")
	}
	started := time.Now()
	stopped := make(chan error, 1)
	go func() { stopped <- g.Stop(t.Context()) }()
	select {
	case err := <-read:
		if err == nil {
			t.Fatal("blocked unary returned success during shutdown")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unary did not drain within five seconds")
	}
	if _, err := g.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("shutdown kept mutation admission open", err)
	}
	select {
	case message := <-signals:
		if message != "term" {
			t.Fatal("owned child did not receive graceful termination", message)
		}
	case <-time.After(time.Second):
		t.Fatal("owned child did not receive SIGTERM")
	}
	select {
	case <-exited:
		t.Fatal("owned child exited before grace expiry")
	case <-time.After(time.Second):
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal("shutdown did not reap the owned child within its budget", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown exceeded its ten-second budget")
	}
	if elapsed := time.Since(started); elapsed < 9500*time.Millisecond || elapsed > 10500*time.Millisecond {
		t.Fatal("owned child was killed outside the shutdown grace boundary", elapsed)
	}
	if status := owned.ProcessState.Sys().(syscall.WaitStatus); !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatal("stubborn owned child was not killed after grace expiry", status)
	}
	if err := other.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("shutdown terminated an unrelated process", err)
	}
	select {
	case <-otherExited:
		t.Fatal("unrelated process exited")
	case message := <-otherSignals:
		t.Fatal("unrelated process received a termination signal", message)
	default:
	}
}

func TestPublishedStartupDeadlineKeepsMutationClosed(t *testing.T) {
	binary := publishedExecutable(t)
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(tmp, "gul-edge-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	g := New(Config{Executable: binary, Home: filepath.Join(root, "h")})
	t.Cleanup(func() { _ = g.Stop(context.Background()) })
	started := time.Now()
	result := make(chan error, 1)
	go func() { result <- g.Start(t.Context()) }()
	var child *exec.Cmd
	for child == nil {
		g.mu.Lock()
		child = g.child
		if child != nil {
			if g.channel != nil || g.err == nil {
				g.mu.Unlock()
				t.Fatal("fixture missed the startup boundary")
			}
			err = child.Process.Signal(syscall.SIGSTOP)
		}
		g.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if child == nil {
			select {
			case err := <-result:
				t.Fatal("published child did not enter startup", err, g.Status())
			case <-time.After(time.Millisecond):
			}
		}
	}
	if _, err := g.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("startup admitted mutation before readiness", err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(16 * time.Second):
		t.Fatal("stalled published startup exceeded fifteen seconds")
	}
	if elapsed := time.Since(started); elapsed < 4750*time.Millisecond || elapsed > 15500*time.Millisecond {
		t.Fatal("stalled startup did not respect its readiness/startup deadline", elapsed)
	}
	if g.Status().ChannelReady || g.Status().Health != "unavailable" {
		t.Fatal("expired startup enabled the provider", g.Status())
	}
	if _, err := g.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{}); err == nil {
		t.Fatal("expired startup admitted mutation")
	}
	select {
	case <-g.exited:
	default:
		t.Fatal("startup deadline left its owned child alive")
	}
}
