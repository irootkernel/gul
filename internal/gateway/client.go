package gateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1/dolgoraev1connect"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/recovery"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type pendingRead struct {
	done   chan struct{}
	result proto.Message
	err    error
}
type client struct {
	transport       *http.Transport
	runtime         dolgoraev1connect.RuntimeServiceClient
	run             dolgoraev1connect.RunServiceClient
	observation     dolgoraev1connect.ObservationServiceClient
	interaction     dolgoraev1connect.InteractionServiceClient
	writer          dolgoraev1connect.WriterServiceClient
	controller      dolgoraev1connect.ControllerServiceClient
	artifact        dolgoraev1connect.ArtifactServiceClient
	orchestration   dolgoraev1connect.OrchestrationServiceClient
	instance        string
	server          string
	protocol        uint32
	lifetime        context.Context
	cancel          context.CancelFunc
	reads           chan struct{}
	mutations       chan struct{}
	streams         chan struct{}
	mu              sync.Mutex
	pending         map[string]*pendingRead
	last            map[string]time.Time
	window          time.Time
	started         int
	closing         bool
	active          sync.WaitGroup
	validateCarrier func(context.Context, string, *publicv1.ControllerCarrierRef, *publicv1.StartRunRequest) error
}

// A one-shot body disables net/http's request replay, including HTTP/2 retries.
type oneShot struct{ io.ReadCloser }
type noReplay struct{ transport *http.Transport }

func (t noReplay) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.GetBody = nil
	if r.Body != nil {
		r.Body = oneShot{r.Body}
	}
	return t.transport.RoundTrip(r)
}

func newClient(socket string) *client {
	socketInfo, _ := os.Lstat(socket)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	t := &http.Transport{Protocols: protocols, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		if err := socketParents(socket); err != nil {
			return nil, err
		}
		current, err := os.Lstat(socket)
		if err != nil || !privateSocket(current) || socketInfo == nil || !os.SameFile(socketInfo, current) {
			return nil, ErrSocket
		}
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}, MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1}
	h := &http.Client{Transport: noReplay{t}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirect refused") }}
	options := []connect.ClientOption{connect.WithGRPC(), connect.WithReadMaxBytes(8 << 20), connect.WithSendMaxBytes(8 << 20)}
	ctx, cancel := context.WithCancel(context.Background())
	return &client{transport: t, runtime: dolgoraev1connect.NewRuntimeServiceClient(h, "http://localhost", options...),
		run:           dolgoraev1connect.NewRunServiceClient(h, "http://localhost", options...),
		observation:   dolgoraev1connect.NewObservationServiceClient(h, "http://localhost", options...),
		interaction:   dolgoraev1connect.NewInteractionServiceClient(h, "http://localhost", options...),
		writer:        dolgoraev1connect.NewWriterServiceClient(h, "http://localhost", options...),
		controller:    dolgoraev1connect.NewControllerServiceClient(h, "http://localhost", options...),
		artifact:      dolgoraev1connect.NewArtifactServiceClient(h, "http://localhost", options...),
		orchestration: dolgoraev1connect.NewOrchestrationServiceClient(h, "http://localhost", options...),
		instance:      uuid.NewString(), lifetime: ctx, cancel: cancel, reads: make(chan struct{}, 4), mutations: make(chan struct{}, 8), streams: make(chan struct{}, 8),
		pending: map[string]*pendingRead{}, last: map[string]time.Time{}}
}

func socketParents(socket string) error {
	parent := filepath.Dir(socket)
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(parent, string(filepath.Separator)), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrSocket
		}
		if current == parent {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm() != 0700 {
				return ErrSocket
			}
		}
	}
	return nil
}

func requestCopy[T any](c *client, request *T, protocol uint32) (*T, error) {
	message, ok := any(request).(proto.Message)
	if !ok || !message.ProtoReflect().IsValid() {
		return nil, recovery.ErrIncompatible
	}
	copy := proto.Clone(message)
	m := copy.ProtoReflect()
	f := m.Descriptor().Fields().ByName("context")
	if f == nil {
		return nil, recovery.ErrIncompatible
	}
	m.Set(f, protoreflect.ValueOfMessage((&publicv1.RequestContext{ProtocolVersion: protocol, ClientRequestId: uuid.NewString(), ClientInstanceId: c.instance}).ProtoReflect()))
	return any(copy).(*T), nil
}

func (c *client) response(message proto.Message) error {
	if err := sanitizeResponse(message.ProtoReflect()); err != nil {
		return err
	}
	m := message.ProtoReflect()
	f := m.Descriptor().Fields().ByName("context")
	if f == nil {
		return nil
	} // WriterState has no top-level response context.
	if !m.Has(f) {
		return recovery.ErrIncompatible
	}
	r, ok := m.Get(f).Message().Interface().(*publicv1.ResponseContext)
	if !ok || r.ProtocolVersion != c.protocol || r.ServerInstanceId != c.server {
		return recovery.ErrIncompatible
	}
	return nil
}

func call[T, R any](ctx context.Context, c *client, method string, request *T, mutation bool, timeout time.Duration,
	invoke func(context.Context, *connect.Request[T]) (*connect.Response[R], error)) (*R, error) {
	if request == nil {
		return nil, recovery.ErrIncompatible
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := bounded.Err(); err != nil {
		return nil, err
	}
	perform := func(ctx context.Context) (proto.Message, error) {
		protocol := c.protocol
		if method == "RuntimeService.GetCapabilities" {
			protocol = 0
		}
		copy, err := requestCopy(c, request, protocol)
		if err != nil {
			return nil, err
		}
		slots := c.reads
		if mutation {
			slots = c.mutations
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.lifetime.Done():
			return nil, ErrUnavailable
		}
		defer func() { <-slots }()
		if c.validateCarrier != nil {
			m := any(copy).(proto.Message).ProtoReflect()
			field := m.Descriptor().Fields().ByName("controller")
			if field != nil && !m.Has(field) && method != "ArtifactService.GetArtifact" && method != "ArtifactService.ReadArtifactChunk" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("controller carrier unavailable"))
			}
			if field != nil && m.Has(field) {
				ref, ok := m.Get(field).Message().Interface().(*publicv1.ControllerCarrierRef)
				if !ok {
					return nil, recovery.ErrIncompatible
				}
				allocation, _ := any(copy).(*publicv1.StartRunRequest)
				if err = c.validateCarrier(ctx, method, ref, allocation); err != nil {
					return nil, err
				}
			}
		}
		response, err := invoke(ctx, connect.NewRequest(copy))
		if err != nil {
			return nil, err
		} // Preserve typed details for the existing domain adapters.
		message := any(response.Msg).(proto.Message)
		if err = c.response(message); err != nil {
			return nil, err
		}
		return message, nil
	}
	c.mu.Lock()
	if c.closing {
		c.mu.Unlock()
		return nil, ErrUnavailable
	}
	if mutation {
		c.active.Add(1)
		c.mu.Unlock()
		defer c.active.Done()
		stop := context.AfterFunc(c.lifetime, cancel)
		defer stop()
		result, err := perform(bounded)
		if err != nil {
			return nil, err
		}
		return any(result).(*R), nil
	}
	keyMessage := proto.Clone(any(request).(proto.Message))
	keyMessage.ProtoReflect().Clear(keyMessage.ProtoReflect().Descriptor().Fields().ByName("context"))
	bytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(keyMessage)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	digest := sha256.Sum256(bytes)
	key := method + string(digest[:])
	pending := c.pending[key]
	if pending == nil {
		if len(c.pending) >= 256 {
			c.mu.Unlock()
			return nil, ErrReadBudget
		}
		pending = &pendingRead{done: make(chan struct{})}
		c.pending[key] = pending
		c.active.Add(1)
		go func() {
			defer c.active.Done()
			readCtx, readCancel := context.WithTimeout(c.lifetime, timeout)
			defer readCancel()
			err := c.readBudget(readCtx, key)
			if err == nil {
				pending.result, err = perform(readCtx)
			}
			pending.err = err
			c.mu.Lock()
			delete(c.pending, key)
			close(pending.done)
			c.mu.Unlock()
		}()
	}
	c.mu.Unlock()
	select {
	case <-bounded.Done():
		return nil, bounded.Err()
	case <-pending.done:
		if pending.err != nil {
			return nil, pending.err
		}
		return any(proto.Clone(pending.result)).(*R), nil
	}
}

// Reads use four workers, at most 16 starts per second, and a 250ms per-key floor.
// Only concurrent reads share results; protected reads are never retained in a cache.
func (c *client) readBudget(ctx context.Context, key string) error {
	for {
		now := time.Now()
		c.mu.Lock()
		for k, t := range c.last {
			if now.Sub(t) >= time.Second {
				delete(c.last, k)
			}
		}
		if now.Sub(c.window) >= time.Second {
			c.window = now
			c.started = 0
		}
		delay := time.Until(c.last[key].Add(250 * time.Millisecond))
		if c.started >= 16 {
			delay = max(delay, time.Until(c.window.Add(time.Second)))
		}
		if delay <= 0 {
			c.last[key] = now
			c.started++
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

func (c *client) close(ctx context.Context) {
	c.mu.Lock()
	c.closing = true
	c.mu.Unlock()
	// Streams stop immediately; unaries retain at most five seconds to drain.
	c.cancel()
	done := make(chan struct{})
	go func() { c.active.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
	c.transport.CloseIdleConnections()
}

type eventStream struct {
	stream  *connect.ServerStreamForClient[publicv1.RunEventEnvelope]
	client  *client
	cancel  context.CancelFunc
	release func()
	once    sync.Once
}

func (s *eventStream) Receive() (*publicv1.RunEventEnvelope, error) {
	if s.stream.Receive() {
		message := s.stream.Msg()
		if err := s.client.response(message); err != nil {
			_ = s.Close()
			return nil, err
		}
		return message, nil
	}
	err := s.stream.Err()
	_ = s.Close()
	if err == nil {
		err = io.EOF
	}
	return nil, err
}
func (s *eventStream) Close() error {
	var err error
	s.once.Do(func() { s.cancel(); err = s.stream.Close(); s.release() })
	return err
}
func (g *Gateway) WatchRunEvents(ctx context.Context, request *publicv1.WatchRunEventsRequest) (port.EventStream, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	select {
	case c.streams <- struct{}{}:
	default:
		return nil, ErrReadBudget
	}
	release := func() { <-c.streams }
	copy, err := requestCopy(c, request, c.protocol)
	if err != nil {
		release()
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.lifetime, cancel)
	stream, err := c.observation.WatchRunEvents(ctx, connect.NewRequest(copy))
	if err != nil {
		stop()
		cancel()
		release()
		return nil, err
	}
	return &eventStream{stream: stream, client: c, cancel: cancel, release: func() { stop(); release() }}, nil
}

// Optional unknown wire data is discarded before any domain adapter can retain
// it. Unknown typed enum values cannot grant authority and reject the response.
func sanitizeResponse(m protoreflect.Message) error {
	m.SetUnknown(nil)
	var err error
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		check := func(v protoreflect.Value) error {
			if f.Kind() == protoreflect.EnumKind && f.Enum().Values().ByNumber(v.Enum()) == nil {
				return recovery.ErrIncompatible
			}
			if f.Kind() == protoreflect.MessageKind {
				return sanitizeResponse(v.Message())
			}
			return nil
		}
		if f.IsList() {
			for i := 0; i < v.List().Len(); i++ {
				if err = check(v.List().Get(i)); err != nil {
					return false
				}
			}
		} else {
			err = check(v)
		}
		return err == nil
	})
	return err
}
