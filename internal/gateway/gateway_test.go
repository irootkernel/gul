package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	contract "github.com/rootkernel/gul/contract"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/reconnect/contractprovider"
	"google.golang.org/protobuf/proto"
)

func TestPrivateSocketPolicyAndRefusal(t *testing.T) {
	root, err := os.MkdirTemp("/private/tmp", "gul-private-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	home := filepath.Join(root, "home")
	runtime := filepath.Join(home, "Library", "Caches", "Gul", "runtime")
	if err = privateParents(home, runtime); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(runtime, 0755); err != nil {
		t.Fatal(err)
	}
	if err = privateParents(home, runtime); !errors.Is(err, ErrSocket) {
		t.Fatal("permissive directory accepted", err)
	}
	if err = os.Chmod(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	if outsideWorkspaces(filepath.Join(runtime, "rpc.sock"), []string{home}) {
		t.Fatal("workspace-contained socket")
	}
	link := filepath.Join(root, "link")
	if err = os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	if err = privateParents(link, filepath.Join(link, "Library", "Caches", "Gul", "runtime")); err == nil {
		t.Fatal("symlink parent")
	}
	socket := filepath.Join(runtime, "stale.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	defer listener.Close()
	if err = os.Chmod(socket, 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(socket)
	if !privateSocket(info) {
		t.Fatal("private socket rejected")
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid++
	if privateSocket(wrongOwner{info, &stat}) {
		t.Fatal("wrong owner admitted")
	}
	g := New(Config{})
	g.parent, g.socket = runtime, socket
	if err = g.cleanup(); !errors.Is(err, ErrSocket) {
		t.Fatal("stale socket was not reported", err)
	}
	if !g.Status().SocketCleanupUnsafe {
		t.Fatal("unsafe cleanup diagnostic missing")
	}
	if _, err = os.Lstat(socket); err != nil {
		t.Fatal("Gul unlinked provider socket", err)
	}
}

type wrongOwner struct {
	os.FileInfo
	stat *syscall.Stat_t
}

func (s wrongOwner) Sys() any { return s.stat }

func TestRestartBudgetRollingWindowJitterAndStableReset(t *testing.T) {
	g := New(Config{})
	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := g.admitStart(now.Add(time.Duration(i) * time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(g.admitStart(now.Add(59*time.Second)), ErrRestartBudget) {
		t.Fatal("sixth start admitted")
	}
	if err := g.admitStart(now.Add(60 * time.Second)); err != nil {
		t.Fatal("rolling budget", err)
	}
	g.resetAfterStable(now, now.Add(5*time.Minute-time.Nanosecond))
	if g.failures != 0 || len(g.starts) == 0 {
		t.Fatal("stable interval reset early")
	}
	g.failures = 5
	g.resetAfterStable(now, now.Add(5*time.Minute))
	if g.failures != 0 || len(g.starts) != 0 || g.admitStart(now.Add(5*time.Minute)) != nil {
		t.Fatal("stable reset did not reopen budget")
	}
	for n := 1; n < 12; n++ {
		for i := 0; i < 20; i++ {
			delay := restartDelay(n)
			base := time.Second << min(n-1, 4)
			if delay < base*8/10 || delay > base*12/10 || delay > 30*time.Second {
				t.Fatal("jitter out of bounds", delay)
			}
		}
	}
}

func TestMissingAndWrongExecutableRemainUnavailable(t *testing.T) {
	root, err := os.MkdirTemp("/private/tmp", "gul-missing-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	for _, name := range []string{"missing", "wrong"} {
		path := filepath.Join(root, name)
		if name == "wrong" {
			if err := os.WriteFile(path, []byte("#!/bin/sh\ntouch executed\n"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		g := New(Config{Home: filepath.Join(root, "home"), Executable: path})
		if err := g.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if g.Ready(t.Context()) == nil {
			t.Fatal("unqualified executable admitted")
		}
		if err := g.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "executed")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unqualified executable executed")
	}
}

func publishedExecutable(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("GUL_E2_DOLGORAE_EXECUTABLE")
	if binary == "" {
		t.Skip("published artifact qualification is opt-in")
	}
	archive, err := os.ReadFile(os.Getenv("GUL_E2_DOLGORAE_ARCHIVE"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	if hex.EncodeToString(digest[:]) != contract.QualifiedRelease().Archive.SHA256 {
		t.Fatal("published archive identity rejected")
	}
	qualified := filepath.Join(t.TempDir(), "dolgorae")
	if err := copyQualifiedExecutable(t.Context(), binary, qualified); err != nil {
		t.Fatal("published executable identity rejected", err)
	}
	return qualified
}

func TestSocketWorkspaceCaseAliasRefused(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(root), "WORKSPACE")
	original, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	same, err := os.Stat(alias)
	if err != nil || !os.SameFile(original, same) {
		t.Skip("case-sensitive fixture filesystem")
	}
	parent := filepath.Join(root, "gateway")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if outsideWorkspaces(filepath.Join(parent, "rpc.sock"), []string{alias}) {
		t.Fatal("physical Workspace case alias admitted provider socket")
	}
	outside := filepath.Join(t.TempDir(), "rpc.sock")
	if !outsideWorkspaces(outside, []string{alias}) {
		t.Fatal("unrelated private socket refused")
	}
}

func TestPublishedGatewayLifecycleRestartAndReplacement(t *testing.T) {
	binary := publishedExecutable(t)
	root, err := os.MkdirTemp("/private/tmp", "gul-live-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	selected := filepath.Join(root, "selected")
	bytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(selected, bytes, 0700); err != nil {
		t.Fatal(err)
	}
	home, work := filepath.Join(root, "home"), filepath.Join(root, "workspace")
	for _, path := range []string{home, work} {
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	// Isolated fixture bootstrap only; production never invokes init.
	init := exec.CommandContext(t.Context(), binary, "init", work, "--non-git")
	init.Env, init.Dir, init.Stdout, init.Stderr = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TMPDIR=" + root}, work, io.Discard, io.Discard
	if err = init.Run(); err != nil {
		t.Fatal("fixture bootstrap", err)
	}
	g := New(Config{Executable: selected, Home: home})
	if err = g.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := g.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	caps, err := g.GetCapabilities(t.Context(), &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		t.Fatal("published readiness", err, g.Status())
	}
	if caps.Features.ReaderWriterAccess || !g.Status().DedicatedWriter {
		t.Fatal("release flags rewritten", g.Status())
	}
	for _, change := range []func(*publicv1.GetCapabilitiesResponse){
		func(c *publicv1.GetCapabilitiesResponse) {
			c.ControllerCarrier.SchemaId = "dolgorae.controller-credential/v1"
		},
		func(c *publicv1.GetCapabilitiesResponse) { c.Features.PersistentRuns = false },
		func(c *publicv1.GetCapabilitiesResponse) { c.Protocol.RpcProtocolVersion++ },
		func(c *publicv1.GetCapabilitiesResponse) { c.Protocol.ProjectionProfiles = nil },
		func(c *publicv1.GetCapabilitiesResponse) {
			c.Protocol.ProjectionProfiles = []publicv1.ProjectionProfile{publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL}
		},
		func(c *publicv1.GetCapabilitiesResponse) { c.SupportedMethods = nil },
		func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts.MaximumChunkSize = 0 },
		func(c *publicv1.GetCapabilitiesResponse) { c.Interactions.MaximumResponseBytes = 0 },
		func(c *publicv1.GetCapabilitiesResponse) { c.Lanes.Items[0].Lane = 1000 },
	} {
		copy := proto.Clone(caps).(*publicv1.GetCapabilitiesResponse)
		change(copy)
		if contractprovider.ValidateCapabilities(copy) == nil {
			t.Fatal("altered release admitted")
		}
	}
	collision := New(Config{Executable: selected, Home: home})
	if err = collision.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if collision.Status().Health != "collision" || collision.Ready(t.Context()) == nil || g.Ready(t.Context()) != nil {
		t.Fatal("active gateway collision attached or disturbed its owner", collision.Status())
	}
	if err = collision.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	instance := caps.Context.ServerInstanceId
	g.mu.Lock()
	child := g.child
	g.mu.Unlock()
	if err = child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		caps, err = g.GetCapabilities(t.Context(), &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
		if err == nil && caps.Context.ServerInstanceId != instance {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || caps.Context.ServerInstanceId == instance {
		t.Fatal("gateway did not re-handshake", err, g.Status())
	}
	// Replacing the selection does not invalidate the verified running copy.
	if err = os.WriteFile(selected, []byte("unqualified replacement"), 0700); err != nil {
		t.Fatal(err)
	}
	if g.Ready(t.Context()) != nil {
		t.Fatal("running verified child followed replaced selection")
	}
	g.mu.Lock()
	child = g.child
	g.mu.Unlock()
	_ = child.Process.Kill()
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && g.Status().Health != "incompatible" {
		time.Sleep(20 * time.Millisecond)
	}
	if g.Status().Health != "incompatible" || g.Ready(t.Context()) == nil {
		t.Fatal("replacement admitted on restart", g.Status())
	}
}

func TestTransientRestartRetriesAndCancellationAndBudgetRemainBounded(t *testing.T) {
	t.Run("readiness failure then success", func(t *testing.T) {
		g := New(Config{})
		calls := 0
		err := g.restart(t.Context(), func(ctx context.Context) error {
			calls++
			if err := g.admitStart(time.Now()); err != nil {
				return err
			}
			if calls == 1 {
				return context.DeadlineExceeded
			}
			return nil
		})
		if err != nil || calls != 2 || g.Status().RestartsRemaining != 3 {
			t.Fatal(err, calls, g.Status())
		}
	})
	t.Run("rolling budget exhausted after transient failure", func(t *testing.T) {
		g := New(Config{})
		for i := 0; i < 4; i++ {
			if err := g.admitStart(time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		calls := 0
		err := g.restart(t.Context(), func(ctx context.Context) error {
			calls++
			if err := g.admitStart(time.Now()); err != nil {
				return err
			}
			return ErrUnavailable
		})
		if !errors.Is(err, ErrRestartBudget) || calls != 2 || g.Status().Health != "restart_exhausted" {
			t.Fatal(err, calls, g.Status())
		}
	})
	t.Run("cancel during backoff", func(t *testing.T) {
		g := New(Config{})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		calls := 0
		if err := g.restart(ctx, func(context.Context) error { calls++; return nil }); !errors.Is(err, context.Canceled) || calls != 0 {
			t.Fatal(err, calls)
		}
	})
}
