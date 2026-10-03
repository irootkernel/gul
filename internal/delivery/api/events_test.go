package api

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/observation"
)

type eventReader struct {
	mu       sync.Mutex
	events   []observation.Notification
	snapshot bool
}

func (r *eventReader) ReadDelivery(_ context.Context, subject, sessionID string, after observation.Sequence) (observation.DeliveryBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if subject != "owner" || sessionID != "session" {
		return observation.DeliveryBatch{}, observation.ErrUnbound
	}
	batch := observation.DeliveryBatch{Head: 3, SnapshotRequired: r.snapshot}
	for _, e := range r.events {
		if e.Sequence > batch.Head {
			batch.Head = e.Sequence
		}
		if e.Sequence > after {
			batch.Events = append(batch.Events, e)
		}
	}
	return batch, nil
}
func eventClient(t *testing.T, reader *eventReader, subject string) gulv1connect.ClientEventServiceClient {
	t.Helper()
	core := app.NewCore(app.Dependencies{Provider: unavailable{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { core.Stop(context.Background()) })
	h := &ClientEventHandler{Core: core, Principal: func(context.Context) (app.Principal, error) { return app.Principal{Subject: subject}, nil }, Events: reader}
	_, handler := gulv1connect.NewClientEventServiceHandler(h)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return gulv1connect.NewClientEventServiceClient(server.Client(), server.URL)
}
func eventNotification(seq observation.Sequence) observation.Notification {
	return observation.Notification{Sequence: seq, SessionID: "session", CorrelationID: "gul-correlation", Kind: "projection_invalidated", CreatedAt: time.Now()}
}
func TestClientEventRPCReplayAndLiveDelivery(t *testing.T) {
	r := &eventReader{events: []observation.Notification{eventNotification(2), eventNotification(3)}}
	client := eventClient(t, r, "owner")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	stream, err := client.WatchClientEvents(ctx, connect.NewRequest(&gulv1.WatchClientEventsRequest{SessionId: "session", AfterDeliverySequence: 1}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for _, sequence := range []uint64{2, 3} {
		if !stream.Receive() {
			t.Fatal(stream.Err())
		}
		e := stream.Msg()
		if e.DeliverySequence != sequence || e.Kind != gulv1.ClientEventKind_CLIENT_EVENT_KIND_PROJECTION_INVALIDATED || e.SessionId != "session" {
			t.Fatal(e)
		}
	}
	r.mu.Lock()
	r.events = append(r.events, eventNotification(4))
	r.mu.Unlock()
	if !stream.Receive() || stream.Msg().DeliverySequence != 4 {
		t.Fatal("live delivery", stream.Err())
	}
}
func TestClientEventRPCSnapshotAndAccessBoundaries(t *testing.T) {
	for _, initial := range []bool{true, false} {
		t.Run(map[bool]string{true: "initial", false: "retention"}[initial], func(t *testing.T) {
			reader := &eventReader{snapshot: !initial}
			client := eventClient(t, reader, "owner")
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			after := uint64(0)
			if !initial {
				after = 1
			}
			stream, err := client.WatchClientEvents(ctx, connect.NewRequest(&gulv1.WatchClientEventsRequest{SessionId: "session", AfterDeliverySequence: after}))
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if !stream.Receive() || stream.Msg().Kind != gulv1.ClientEventKind_CLIENT_EVENT_KIND_SNAPSHOT_REQUIRED || stream.Msg().DeliverySequence != 3 {
				t.Fatal("missing snapshot boundary", stream.Err())
			}
		})
	}
	for _, subject := range []string{"", "other"} {
		t.Run("subject-"+subject, func(t *testing.T) {
			client := eventClient(t, &eventReader{}, subject)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			stream, err := client.WatchClientEvents(ctx, connect.NewRequest(&gulv1.WatchClientEventsRequest{SessionId: "session"}))
			if err == nil {
				defer stream.Close()
				if stream.Receive() {
					t.Fatal("unauthorized event")
				}
				err = stream.Err()
			}
			if err == nil {
				t.Fatal("access not rejected")
			}
		})
	}
}
func TestWriterModeMappingFailsClosed(t *testing.T) {
	if browserWriterMode(action.WriterUnverified) != gulv1.WriterAccessMode_WRITER_ACCESS_MODE_UNVERIFIED || browserActionState(action.Evaluation{Mode: action.WriterUnverified}).Mode != gulv1.WriterAccessMode_WRITER_ACCESS_MODE_UNVERIFIED {
		t.Fatal("unverified presentation was lost")
	}
	for _, mode := range []action.WriterMode{action.WriterWrite, action.WriterReadOnly, action.WriterBlocked, action.WriterUnverified, "unknown"} {
		if (browserWriterMode(mode) == gulv1.WriterAccessMode_WRITER_ACCESS_MODE_WRITE) != (mode == action.WriterWrite) {
			t.Fatal(mode)
		}
	}
}
