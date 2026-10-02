package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1/dolgoraev1connect"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/recovery"
)

type testRuntime struct {
	capabilityError   error
	alterCapabilities func(*publicv1.GetCapabilitiesResponse)
	dolgoraev1connect.UnimplementedRuntimeServiceHandler
	requests chan *publicv1.RequestContext
	reads    atomic.Int32
	block    chan struct{}
	active   atomic.Int32
	peak     atomic.Int32
}

func (r *testRuntime) GetCapabilities(ctx context.Context, q *connect.Request[publicv1.GetCapabilitiesRequest]) (*connect.Response[publicv1.GetCapabilitiesResponse], error) {
	r.requests <- q.Msg.Context
	if r.capabilityError != nil {
		return nil, r.capabilityError
	}
	if q.Msg.Context.ProtocolVersion != 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("protocol"))
	}
	caps, err := scenario.New(time.Now()).GetCapabilities(ctx, q.Msg)
	caps.Context.ServerInstanceId = "test-server"
	if r.alterCapabilities != nil {
		r.alterCapabilities(caps)
	}
	return connect.NewResponse(caps), err
}
func (r *testRuntime) ListProfiles(ctx context.Context, q *connect.Request[publicv1.ListProfilesRequest]) (*connect.Response[publicv1.ListProfilesResponse], error) {
	r.reads.Add(1)
	r.requests <- q.Msg.Context
	select {
	case <-r.block:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return connect.NewResponse(&publicv1.ListProfilesResponse{Context: &publicv1.ResponseContext{ProtocolVersion: 1, ServerInstanceId: "test-server"}}), nil
}

func (r *testRuntime) GetProfile(ctx context.Context, q *connect.Request[publicv1.GetProfileRequest]) (*connect.Response[publicv1.GetProfileResponse], error) {
	r.reads.Add(1)
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for peak := r.peak.Load(); active > peak && !r.peak.CompareAndSwap(peak, active); peak = r.peak.Load() {
	}
	r.requests <- q.Msg.Context
	select {
	case <-r.block:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return connect.NewResponse(&publicv1.GetProfileResponse{Context: &publicv1.ResponseContext{ProtocolVersion: 1, ServerInstanceId: "test-server"}}), nil
}

type testRun struct {
	dolgoraev1connect.UnimplementedRunServiceHandler
	calls atomic.Int32
}

type testObservation struct {
	dolgoraev1connect.UnimplementedObservationServiceHandler
}

func (testObservation) WatchRunEvents(ctx context.Context, q *connect.Request[publicv1.WatchRunEventsRequest], stream *connect.ServerStream[publicv1.RunEventEnvelope]) error {
	if q.Msg.Context.ProtocolVersion != 1 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("protocol"))
	}
	message := &publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_Heartbeat{Heartbeat: &publicv1.RunEventHeartbeat{}}}
	// An unread stream exhausts its own HTTP/2 receive window.
	message.ProtoReflect().SetUnknown(append([]byte{0xfa, 0x7f, 0x80, 0x80, 0x40}, make([]byte, 1<<20)...))
	for i := 0; i < 32; i++ {
		if err := stream.Send(message); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (r *testRun) SubmitTurn(context.Context, *connect.Request[publicv1.SubmitTurnRequest]) (*connect.Response[publicv1.SubmitTurnAccepted], error) {
	r.calls.Add(1)
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("untrusted prose"))
	detail, _ := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: "SESSION_CLOSE_IN_PROGRESS", RetryClassification: publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE, Action: publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT})
	err.AddDetail(detail)
	return nil, err
}

func transportFixture(t *testing.T) (*Gateway, *testRuntime, *testRun) {
	t.Helper()
	root, err := os.MkdirTemp("/private/tmp", "gul-rpc-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "rpc.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(socket, 0600); err != nil {
		t.Fatal(err)
	} // Test server owns this socket.
	r := &testRuntime{requests: make(chan *publicv1.RequestContext, 256), block: make(chan struct{})}
	run := &testRun{}
	mux := http.NewServeMux()
	mux.Handle(dolgoraev1connect.NewRuntimeServiceHandler(r))
	mux.Handle(dolgoraev1connect.NewRunServiceHandler(run))
	mux.Handle(dolgoraev1connect.NewObservationServiceHandler(testObservation{}))
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Handler: mux, Protocols: protocols}
	go func() { _ = server.Serve(listener) }()
	c := newClient(socket)
	c.protocol, c.server = 1, "test-server"
	g := New(Config{})
	g.channel, g.err = c, nil
	t.Cleanup(func() { close(r.block); c.close(context.Background()); _ = server.Close(); _ = os.RemoveAll(root) })
	return g, r, run
}

func TestUnreadStreamsRespectLimitAndDoNotDelayUnaryMutation(t *testing.T) {
	g, _, run := transportFixture(t)
	var streams []port.EventStream
	for i := 0; i < 8; i++ {
		stream, err := g.WatchRunEvents(t.Context(), &publicv1.WatchRunEventsRequest{AfterCursor: fmt.Sprint(i)})
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, stream)
	}
	defer func() {
		for _, stream := range streams {
			_ = stream.Close()
		}
	}()
	if _, err := g.WatchRunEvents(t.Context(), &publicv1.WatchRunEventsRequest{}); !errors.Is(err, ErrReadBudget) {
		t.Fatal("ninth stream admitted", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := g.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{})
	if run.calls.Load() != 1 || port.MapProviderError(err).Code != "RUN_STATE_CONFLICT" {
		t.Fatal("slow streams blocked mutation", err)
	}
}

func TestTransportNegotiatesFreshContextsAndCoalescesReadsWithoutBlockingMutation(t *testing.T) {
	g, r, run := transportFixture(t)
	caller := &publicv1.GetCapabilitiesRequest{Context: &publicv1.RequestContext{ProtocolVersion: 99, ClientRequestId: "caller"}, MinimumProtocolVersion: 1, MaximumProtocolVersion: 1}
	for i := 0; i < 2; i++ {
		if _, err := g.GetCapabilities(t.Context(), caller); err != nil {
			t.Fatal(err)
		}
	}
	first, second := <-r.requests, <-r.requests
	if first.ClientRequestId == second.ClientRequestId || first.ClientInstanceId != second.ClientInstanceId || caller.Context.ProtocolVersion != 99 {
		t.Fatal("contexts were reused or caller was mutated")
	}
	for _, c := range []*publicv1.RequestContext{first, second} {
		if _, err := uuid.Parse(c.ClientRequestId); err != nil {
			t.Fatal(err)
		}
	}
	var wait sync.WaitGroup
	for i := 0; i < 40; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ctx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
			defer cancel()
			_, _ = g.ListProfiles(ctx, &publicv1.ListProfilesRequest{})
		}()
	}
	select {
	case c := <-r.requests:
		if c.ProtocolVersion != 1 {
			t.Fatal("negotiated protocol")
		}
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}
	start := time.Now()
	_, err := g.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{})
	if time.Since(start) > time.Second || run.calls.Load() != 1 {
		t.Fatal("mutation was delayed or retried")
	}
	if mapped := port.MapProviderError(err); mapped.Code != "RUN_STATE_CONFLICT" {
		t.Fatal("typed error detail was lost", mapped)
	}
	wait.Wait()
	if r.reads.Load() != 1 {
		t.Fatal("burst was not single-flight", r.reads.Load())
	}
}

func TestResponseRejectsRequiredEnumAndDropsOptionalWireData(t *testing.T) {
	caps, _ := scenario.New(time.Now()).GetCapabilities(t.Context(), &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	caps.ProtoReflect().SetUnknown([]byte{0xf8, 0x7f, 0x01})
	if err := sanitizeResponse(caps.ProtoReflect()); err != nil || len(caps.ProtoReflect().GetUnknown()) != 0 {
		t.Fatal("unknown optional bytes escaped")
	}
	caps.AccessPolicyTransition = 1000
	if err := sanitizeResponse(caps.ProtoReflect()); !errors.Is(err, recovery.ErrIncompatible) {
		t.Fatal("unknown required enum", err)
	}
	c := &client{protocol: 1, server: "expected"}
	if err := c.response(&publicv1.GetRunResponse{Context: &publicv1.ResponseContext{ProtocolVersion: 1, ServerInstanceId: "other"}}); err == nil {
		t.Fatal("old generation admitted")
	}
	if err := c.response(&publicv1.GetRunResponse{}); err == nil {
		t.Fatal("missing response context admitted")
	}
}

func TestReadBudgetAndCancellation(t *testing.T) {
	g, r, _ := transportFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := g.ListProfiles(ctx, &publicv1.ListProfilesRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Different request keys remain bounded by the provider's four workers.
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			_, _ = g.GetProfile(ctx, &publicv1.GetProfileRequest{ProfileName: fmt.Sprint(i)})
		}(i)
	}
	wait.Wait()
	if r.peak.Load() > 4 || r.peak.Load() == 0 {
		t.Fatal("provider read concurrency exceeded its worker bound", r.peak.Load())
	}
}

func TestHandshakeDistinguishesTransientTransportFromSemanticIncompatibility(t *testing.T) {
	for _, code := range []connect.Code{connect.CodeUnavailable, connect.CodeDeadlineExceeded, connect.CodeResourceExhausted, connect.CodeFailedPrecondition, connect.CodeUnimplemented} {
		t.Run(code.String(), func(t *testing.T) {
			g, runtime, _ := transportFixture(t)
			runtime.capabilityError = connect.NewError(code, errors.New("fixture handshake failure"))
			caps, err := negotiate(t.Context(), g.channel)
			terminal := code == connect.CodeFailedPrecondition || code == connect.CodeUnimplemented
			if caps != nil || err == nil || errors.Is(err, recovery.ErrIncompatible) != terminal {
				t.Fatal(caps, err)
			}
		})
	}
	t.Run("invalid contract", func(t *testing.T) {
		g, runtime, _ := transportFixture(t)
		runtime.alterCapabilities = func(caps *publicv1.GetCapabilitiesResponse) { caps.DescriptorSha256 = "wrong" }
		if caps, err := negotiate(t.Context(), g.channel); caps != nil || !errors.Is(err, recovery.ErrIncompatible) {
			t.Fatal(caps, err)
		}
	})
}
