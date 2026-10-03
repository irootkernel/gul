package api

import (
	"context"
	"errors"
	"math"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/observation"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ClientEventHandler is unmounted until authenticated product assembly.
type ClientEventHandler struct {
	Core      *app.Core
	Principal PrincipalResolver
	Events    observation.DeliveryReader
}

func (h *ClientEventHandler) access(ctx context.Context) (string, error) {
	if h == nil || h.Core == nil || h.Principal == nil || h.Events == nil {
		return "", accessError(connect.CodeUnauthenticated, "event access unavailable")
	}
	return localAccess(ctx, h.Core, h.Principal)
}
func (h *ClientEventHandler) WatchClientEvents(ctx context.Context, request *connect.Request[gulv1.WatchClientEventsRequest], stream *connect.ServerStream[gulv1.ClientEvent]) error {
	subject, err := h.access(ctx)
	if err != nil {
		return err
	}
	sessionID := request.Msg.GetSessionId()
	if sessionID == "" || len(sessionID) > 256 || request.Msg.GetAfterDeliverySequence() > math.MaxInt64 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid event request"))
	}
	after := observation.Sequence(request.Msg.GetAfterDeliverySequence())
	initial := after == 0
	timer := time.NewTicker(observation.CoalesceInterval)
	defer timer.Stop()
	for {
		current, err := h.access(ctx)
		if err != nil {
			return err
		}
		if current != subject {
			return accessError(connect.CodePermissionDenied, "event principal changed")
		}
		batch, err := h.Events.ReadDelivery(ctx, subject, sessionID, after)
		if err != nil {
			code := connect.CodeUnavailable
			if errors.Is(err, observation.ErrUnbound) {
				code = connect.CodeNotFound
			}
			if errors.Is(err, observation.ErrInvalid) {
				code = connect.CodeInvalidArgument
			}
			return connect.NewError(code, errors.New("event delivery unavailable"))
		}
		if initial || batch.SnapshotRequired {
			if err := stream.Send(&gulv1.ClientEvent{DeliverySequence: uint64(batch.Head), SessionId: sessionID, Kind: gulv1.ClientEventKind_CLIENT_EVENT_KIND_SNAPSHOT_REQUIRED, CreatedAt: timestamppb.Now()}); err != nil {
				return err
			}
			after = batch.Head
			initial = false
		} else {
			for _, event := range batch.Events {
				if event.Sequence <= after || event.SessionID != sessionID || event.Kind != "projection_invalidated" {
					return connect.NewError(connect.CodeInternal, errors.New("invalid event delivery projection"))
				}
				if err := stream.Send(&gulv1.ClientEvent{DeliverySequence: uint64(event.Sequence), SessionId: event.SessionID, CorrelationId: event.CorrelationID, Kind: gulv1.ClientEventKind_CLIENT_EVENT_KIND_PROJECTION_INVALIDATED, CreatedAt: timestamppb.New(event.CreatedAt)}); err != nil {
					return err
				}
				after = event.Sequence
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

var _ gulv1connect.ClientEventServiceHandler = (*ClientEventHandler)(nil)

func browserWriterMode(mode action.WriterMode) gulv1.WriterAccessMode {
	switch mode {
	case action.WriterWrite:
		return gulv1.WriterAccessMode_WRITER_ACCESS_MODE_WRITE
	case action.WriterReadOnly:
		return gulv1.WriterAccessMode_WRITER_ACCESS_MODE_READ_ONLY
	case action.WriterUnverified:
		return gulv1.WriterAccessMode_WRITER_ACCESS_MODE_UNVERIFIED
	default:
		return gulv1.WriterAccessMode_WRITER_ACCESS_MODE_BLOCKED
	}
}
