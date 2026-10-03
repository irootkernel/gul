package api

import (
	"connectrpc.com/connect"
	"context"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/action"
)

func (h *DirectPresentationHandler) InterruptPrimary(ctx context.Context, q *connect.Request[gulv1.InterruptPrimaryRequest]) (*connect.Response[gulv1.InterruptPrimaryResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	if h.Interrupts == nil {
		return nil, actionError(action.ErrUnavailable)
	}
	if q == nil || q.Msg == nil || len(q.Msg.ProtoReflect().GetUnknown()) != 0 {
		return nil, actionError(action.ErrInvalid)
	}
	outcome, state, err := h.Interrupts.Send(ctx, subject, q.Msg.SessionId, q.Msg.AttemptId, q.Msg.InterruptConfirmed)
	if err != nil {
		return nil, actionError(err)
	}
	return connect.NewResponse(&gulv1.InterruptPrimaryResponse{Outcome: gulv1.InterruptOutcome(outcome), State: browserActionState(state)}), nil
}
