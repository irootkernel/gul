package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gv "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/launch"
	"github.com/rootkernel/gul/internal/session"
)

func (h *DirectPresentationHandler) CreateSession(ctx context.Context, q *connect.Request[gv.CreateSessionRequest]) (*connect.Response[gv.CreateSessionResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	if h.Creator == nil {
		return nil, sessionError(session.ErrUnavailable)
	}
	if q == nil || q.Msg == nil || q.Msg.Choice == nil || len(q.Msg.ProtoReflect().GetUnknown()) != 0 || len(q.Msg.Choice.ProtoReflect().GetUnknown()) != 0 {
		return nil, sessionError(session.ErrInvalid)
	}
	c := q.Msg.Choice
	out, err := h.Creator.CreateSession(ctx, subject, q.Msg.WorkspaceId, q.Msg.AttemptId, launch.Choice{ProfileName: c.ProfileName, ModelID: c.ModelId, Effort: c.Effort, Lane: domainLane(c.Lane), RequiredAssurance: domainAssurance(c.RequiredAssurance), PolicyName: c.PolicyName, AcknowledgeSharedReadOnly: c.AcknowledgeSharedReadonly})
	if err != nil {
		if errors.Is(err, launch.ErrInvalidChoice) || errors.Is(err, launch.ErrUnsupported) || errors.Is(err, launch.ErrInvalidProjection) {
			return nil, launchError(err)
		}
		return nil, sessionError(err)
	}
	return connect.NewResponse(&gv.CreateSessionResponse{SessionId: out.SessionID, OutcomeUnknown: out.Unknown}), nil
}
func (h *DirectPresentationHandler) RecoverCreation(ctx context.Context, q *connect.Request[gv.RecoverCreationRequest]) (*connect.Response[gv.CreateSessionResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	if h.Creator == nil {
		return nil, sessionError(session.ErrUnavailable)
	}
	if q == nil || q.Msg == nil || len(q.Msg.ProtoReflect().GetUnknown()) != 0 {
		return nil, sessionError(session.ErrInvalid)
	}
	out, err := h.Creator.RecoverCreation(ctx, subject, q.Msg.WorkspaceId, q.Msg.AttemptId)
	if err != nil {
		return nil, sessionError(err)
	}
	return connect.NewResponse(&gv.CreateSessionResponse{SessionId: out.SessionID, OutcomeUnknown: out.Unknown}), nil
}
