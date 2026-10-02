package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	contract "github.com/rootkernel/gul/contract"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/reconnect/contractprovider"
	"github.com/rootkernel/gul/internal/recovery"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

var ErrUnavailable = errors.New("qualified provider unavailable")
var ErrIdentity = errors.New("provider executable identity rejected")
var ErrSocket = errors.New("provider socket ownership rejected")
var ErrAlreadyRunning = errors.New("RpcServerAlreadyRunning")
var ErrReadBudget = errors.New("provider read budget exhausted")
var ErrRestartBudget = errors.New("provider restart budget exhausted")

type Config struct {
	Executable, Home string
	WorkspaceRoots   []string
}
type Gateway struct {
	config     Config
	operation  sync.Mutex
	mu         sync.Mutex
	channel    *client
	negotiated *publicv1.GetCapabilitiesResponse
	child      *exec.Cmd
	exited     chan struct{}
	parent     string
	socket     string
	err        error
	lifetime   context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	starts     []time.Time
	failures   int
	status     app.ProviderStatus
}

func New(c Config) *Gateway {
	return &Gateway{config: c, err: ErrUnavailable, status: app.ProviderStatus{Health: "unavailable", RestartsRemaining: 5}}
}
func (g *Gateway) client() (*client, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return nil, g.err
	}
	if g.channel == nil {
		return nil, ErrUnavailable
	}
	return g.channel, nil
}
func (g *Gateway) Status() app.ProviderStatus { g.mu.Lock(); defer g.mu.Unlock(); return g.status }

// NegotiatedCapabilities returns the last accepted startup contract, not current
// readiness. Runtime gates still probe the live channel before admitting work.
func (g *Gateway) NegotiatedCapabilities() *publicv1.GetCapabilitiesResponse {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.negotiated == nil {
		return nil
	}
	return proto.Clone(g.negotiated).(*publicv1.GetCapabilitiesResponse)
}
func (g *Gateway) Ready(ctx context.Context) error {
	_, err := g.GetCapabilities(ctx, &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	return err
}
func (g *Gateway) GetCapabilities(ctx context.Context, request *publicv1.GetCapabilitiesRequest) (*publicv1.GetCapabilitiesResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	caps, err := call(ctx, c, "RuntimeService.GetCapabilities", request, false, 5*time.Second, c.runtime.GetCapabilities)
	if err == nil {
		err = contractprovider.ValidateCapabilities(caps)
	}
	if err != nil {
		if errors.Is(err, recovery.ErrIncompatible) {
			g.setFailure(err)
		}
		g.mu.Lock()
		g.status.ChannelReady = false
		g.mu.Unlock()
		return nil, err
	}
	g.mu.Lock()
	g.status.ChannelReady = true
	g.mu.Unlock()
	return caps, nil
}

// Start admits the local host even when the provider is absent or incompatible.
// Only a verified child and semantic handshake open the provider gate.
func (g *Gateway) Start(ctx context.Context) error {
	g.operation.Lock()
	defer g.operation.Unlock()
	g.mu.Lock()
	if g.cancel != nil {
		g.mu.Unlock()
		return nil
	}
	g.lifetime, g.cancel = context.WithCancel(context.Background())
	g.done = make(chan struct{})
	g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		g.cancel()
		close(g.done)
		return err
	}
	err := g.startChild(ctx)
	if err != nil {
		g.setFailure(err)
		close(g.done)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}
	go g.monitor()
	return nil
}
func (g *Gateway) setFailure(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.err = err
	g.status.ChannelReady = false
	g.status.Health = "unavailable"
	if errors.Is(err, ErrIdentity) || errors.Is(err, recovery.ErrIncompatible) {
		g.status.Health = "incompatible"
	}
	if errors.Is(err, ErrAlreadyRunning) {
		g.status.Health = "collision"
	}
	if errors.Is(err, ErrRestartBudget) {
		g.status.Health = "restart_exhausted"
	}
}

func (g *Gateway) admitStart(now time.Time) error {
	i := 0
	for i < len(g.starts) && now.Sub(g.starts[i]) >= time.Minute {
		i++
	}
	g.starts = g.starts[i:]
	if len(g.starts) >= 5 {
		return ErrRestartBudget
	}
	g.starts = append(g.starts, now)
	g.mu.Lock()
	g.status.RestartsRemaining = uint32(5 - len(g.starts))
	g.mu.Unlock()
	return nil
}

func (g *Gateway) startChild(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	stop := context.AfterFunc(g.lifetime, cancel)
	defer stop()
	// A failed prior startup must settle its owned process before another start.
	g.mu.Lock()
	child := g.child
	g.mu.Unlock()
	if child != nil {
		if err := g.terminate(ctx); err != nil {
			return errors.Join(ErrSocket, err)
		}
	}
	if err := g.admitStart(time.Now()); err != nil {
		return err
	}
	g.mu.Lock()
	previousSocket := g.socket
	g.mu.Unlock()
	home := g.config.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return ErrSocket
		}
	}
	executable := g.config.Executable
	if executable == "" {
		var err error
		executable, err = exec.LookPath("dolgorae")
		if err != nil {
			return ErrUnavailable
		}
	}
	if _, err := os.Lstat(executable); err != nil {
		return ErrUnavailable
	}
	root := filepath.Join(home, "Library", "Caches", "Gul", "runtime")
	if err := privateParents(home, root); err != nil {
		return err
	}
	parent, err := os.MkdirTemp(root, "g-")
	if err != nil {
		return ErrSocket
	}
	socket := filepath.Join(parent, "rpc.sock")
	cleanup := func() { _ = os.Remove(filepath.Join(parent, "dolgorae")); _ = os.Remove(parent) }
	if len(socket) >= 104 || !outsideWorkspaces(socket, g.config.WorkspaceRoots) {
		cleanup()
		return ErrSocket
	}
	if _, err := os.Lstat(socket); !errors.Is(err, os.ErrNotExist) {
		cleanup()
		return ErrAlreadyRunning
	}
	copy := filepath.Join(parent, "dolgorae")
	if err = copyQualifiedExecutable(ctx, executable, copy); err != nil {
		cleanup()
		return err
	}
	cmd := exec.Command(copy, "serve", "--socket", socket)
	cmd.Dir = parent
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TMPDIR=" + parent}
	ready := &readiness{}
	cmd.Stdout, cmd.Stderr = ready, io.Discard
	if err := ctx.Err(); err != nil {
		cleanup()
		return err
	}
	if err = cmd.Start(); err != nil {
		cleanup()
		return ErrUnavailable
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	g.mu.Lock()
	g.child, g.exited, g.parent, g.socket = cmd, exited, parent, socket
	g.mu.Unlock()
	// Keep a reap interval inside the overall startup deadline.
	deadline, _ := ctx.Deadline()
	readinessCtx, readinessCancel := context.WithDeadline(ctx, deadline.Add(-250*time.Millisecond))
	defer readinessCancel()
	failure := func(err error) error { g.terminate(ctx); return err }
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := os.Lstat(socket)
		if err == nil {
			if !privateSocket(info) {
				return failure(ErrSocket)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return failure(ErrSocket)
		}
		select {
		case <-readinessCtx.Done():
			return failure(readinessCtx.Err())
		case <-exited:
			return failure(ready.failure())
		case <-ticker.C:
		}
	}
	c := newClient(socket)
	caps, err := negotiate(readinessCtx, c)
	if err != nil {
		c.close(ctx)
		return failure(err)
	}
	c.protocol, c.server = caps.Context.ProtocolVersion, caps.Context.ServerInstanceId
	if err := ctx.Err(); err != nil {
		c.close(ctx)
		return failure(err)
	}
	select {
	case <-exited:
		c.close(ctx)
		return failure(ErrUnavailable)
	default:
	}
	unsafePrevious := false
	if previousSocket != "" {
		_, err := os.Lstat(previousSocket)
		unsafePrevious = !errors.Is(err, os.ErrNotExist)
	}
	g.mu.Lock()
	g.status.SocketCleanupUnsafe = unsafePrevious
	g.channel, g.err = c, nil
	g.negotiated = proto.Clone(caps).(*publicv1.GetCapabilitiesResponse)
	f := caps.Features
	g.status.PersistentRuns, g.status.EventReplay, g.status.ControllerBinding, g.status.Artifacts = f.PersistentRuns, f.EventReplay, f.ControllerBinding, f.ArtifactRetrieval
	g.status.ReaderWriter, g.status.DurableWriter, g.status.FirstWriteViaSubmit = f.ReaderWriterAccess, f.DurableWriterAuthority, f.FirstWriteViaSubmitTurn
	for _, lane := range caps.GetLanes().GetItems() {
		if lane.GetLane() == publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED {
			g.status.DedicatedWriter = lane.GetWriterSupport()
		}
	}
	g.status.Version = contract.QualifiedRelease().Version
	g.status.Protocol = c.protocol
	g.status.Health, g.status.ChannelReady = "ready", true
	g.mu.Unlock()
	return nil
}

func (g *Gateway) monitor() {
	defer close(g.done)
	for {
		g.mu.Lock()
		exited := g.exited
		g.mu.Unlock()
		started := time.Now()
		select {
		case <-g.lifetime.Done():
			return
		case <-exited:
		}
		g.mu.Lock()
		incompatible := errors.Is(g.err, recovery.ErrIncompatible)
		g.mu.Unlock()
		if !incompatible {
			g.setFailure(ErrUnavailable)
		}
		g.mu.Lock()
		c := g.channel
		g.channel = nil
		g.mu.Unlock()
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if c != nil {
			c.close(drain)
		}
		cancel()
		_ = g.cleanup()
		if incompatible {
			return
		}
		g.resetAfterStable(started, time.Now())
		if err := g.restart(g.lifetime, g.startChild); err != nil {
			if g.lifetime.Err() == nil {
				g.setFailure(err)
			}
			return
		}
	}
}

func negotiate(ctx context.Context, c *client) (*publicv1.GetCapabilitiesResponse, error) {
	handshake, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, _ := requestCopy(c, &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1}, 0)
	response, err := c.runtime.GetCapabilities(handshake, connect.NewRequest(request))
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeUnavailable, connect.CodeDeadlineExceeded, connect.CodeCanceled, connect.CodeResourceExhausted:
			return nil, err
		default:
			return nil, errors.Join(recovery.ErrIncompatible, err)
		}
	}
	if err = sanitizeResponse(response.Msg.ProtoReflect()); err == nil {
		err = contractprovider.ValidateCapabilities(response.Msg)
	}
	if err != nil {
		return nil, errors.Join(recovery.ErrIncompatible, err)
	}
	return response.Msg, nil
}

// Each failed restart consumes its admitted start and the next backoff step.
// Transient transport/readiness failures do not end supervision.
func (g *Gateway) restart(ctx context.Context, start func(context.Context) error) error {
	for {
		g.failures++
		g.mu.Lock()
		g.status.Health = "restarting"
		g.mu.Unlock()
		timer := time.NewTimer(restartDelay(g.failures))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		err := start(ctx)
		if err == nil {
			return nil
		}
		g.setFailure(err)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, recovery.ErrIncompatible) || errors.Is(err, ErrIdentity) || errors.Is(err, ErrSocket) || errors.Is(err, ErrAlreadyRunning) || errors.Is(err, ErrRestartBudget) {
			return err
		}
	}
}

func (g *Gateway) resetAfterStable(started, now time.Time) {
	if now.Sub(started) >= 5*time.Minute {
		g.failures = 0
		g.starts = nil
	}
}

func restartDelay(failures int) time.Duration {
	base := time.Second << min(max(failures-1, 0), 4)
	var bytes [8]byte
	_, _ = rand.Read(bytes[:])
	jitter := 0.8 + float64(binary.LittleEndian.Uint64(bytes[:])%4001)/10000
	return min(time.Duration(float64(base)*jitter), 30*time.Second)
}

func (g *Gateway) Stop(ctx context.Context) error {
	g.operation.Lock()
	defer g.operation.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	g.mu.Lock()
	lifetimeCancel, done, c := g.cancel, g.done, g.channel
	g.channel = nil
	g.err = ErrUnavailable
	g.status.ChannelReady = false
	g.status.Health = "stopped"
	g.mu.Unlock()
	if lifetimeCancel == nil {
		return nil
	}
	lifetimeCancel()
	if c != nil {
		drain, stop := context.WithTimeout(ctx, 5*time.Second)
		c.close(drain)
		stop()
	}
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	g.mu.Lock()
	remaining := g.channel
	g.channel = nil
	g.mu.Unlock()
	if remaining != nil && remaining != c {
		remaining.close(ctx)
	}
	err := g.terminate(ctx)
	g.mu.Lock()
	g.cancel = nil
	g.mu.Unlock()
	return err
}
func (g *Gateway) terminate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	g.mu.Lock()
	cmd, exited := g.child, g.exited
	g.mu.Unlock()
	if cmd == nil {
		return nil
	}
	select {
	case <-exited:
	default:
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	deadline, _ := ctx.Deadline()
	grace := max(time.Until(deadline)-250*time.Millisecond, 0)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-exited:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case <-timer.C:
		_ = cmd.Process.Kill()
		select {
		case <-exited:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return g.cleanup()
}
func (g *Gateway) cleanup() error {
	g.mu.Lock()
	socket, parent := g.socket, g.parent
	g.child = nil
	g.mu.Unlock()
	if parent == "" {
		return nil
	}
	// Dolgorae owns all socket unlinking. Retain an unsafe/stale socket untouched.
	_, err := os.Lstat(socket)
	_ = os.Remove(filepath.Join(parent, "dolgorae"))
	if !errors.Is(err, os.ErrNotExist) {
		g.mu.Lock()
		g.status.SocketCleanupUnsafe = true
		g.mu.Unlock()
		return ErrSocket
	}
	_ = os.Remove(parent)
	return nil
}

func privateParents(home, root string) error {
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return ErrSocket
	}
	// Check every component, including ancestors of the selected home.
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(root, string(filepath.Separator)), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(current, 0700); err != nil {
				return ErrSocket
			}
			info, err = os.Lstat(current)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrSocket
		}
		if current == home || strings.HasPrefix(current, home+string(filepath.Separator)) {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0022 != 0 {
				return ErrSocket
			}
			if current == root || current == filepath.Dir(root) {
				if info.Mode().Perm() != 0700 {
					return ErrSocket
				}
			}
		}
	}
	return nil
}
func privateSocket(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid()) && info.Mode()&os.ModeSocket != 0 && info.Mode().Perm() == 0600
}
func outsideWorkspaces(socket string, roots []string) bool {
	for _, root := range roots {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return false
		}
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			return false
		}
		rel, err := filepath.Rel(resolved, socket)
		if err != nil || rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}
	}
	return true
}
func copyQualifiedExecutable(ctx context.Context, source, destination string) error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return ErrIdentity
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return ErrIdentity
	}
	resolved, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return ErrIdentity
	}
	fd, err := unix.Open(filepath.Join(resolved, filepath.Base(absolute)), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrIdentity
	}
	input := os.NewFile(uintptr(fd), "provider")
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Size() > 256<<20 {
		return ErrIdentity
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return ErrIdentity
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, (256<<20)+1))
	if err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != contract.QualifiedRelease().Executable.SHA256 {
		return ErrIdentity
	}
	return ctx.Err()
}
