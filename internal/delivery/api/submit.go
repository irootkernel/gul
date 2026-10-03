package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/files"
	"github.com/rootkernel/gul/internal/submit"
)

func (h *DirectPresentationHandler) Submit(ctx context.Context, q *connect.Request[gulv1.SubmitRequest]) (*connect.Response[gulv1.SubmitResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	if h.Submissions == nil {
		return nil, actionError(action.ErrUnavailable)
	}
	if q == nil || q.Msg == nil || len(q.Msg.ProtoReflect().GetUnknown()) != 0 || len(q.Msg.Images) > 16 || len(q.Msg.Text) > gulv1.MaximumSubmitTextBytes || q.Msg.WriteIntent != gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ && q.Msg.WriteIntent != gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE {
		return nil, actionError(action.ErrInvalid)
	}
	intent := action.IntentRead
	if q.Msg.WriteIntent == gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE {
		intent = action.IntentWrite
	}
	options := submit.Options{Effort: q.Msg.Effort}
	for _, image := range q.Msg.Images {
		if image == nil || len(image.ProtoReflect().GetUnknown()) != 0 {
			return nil, actionError(action.ErrInvalid)
		}
		options.Images = append(options.Images, files.ImageReference{WorkspaceID: image.WorkspaceId, RelativePath: image.RelativePath, Detail: image.Detail})
	}
	state, err := h.Submissions.SendInput(ctx, subject, q.Msg.SessionId, q.Msg.AttemptId, q.Msg.Text, intent, options)
	outcome := gulv1.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED
	switch {
	case errors.Is(err, action.ErrOutcomeUnknown):
		outcome = gulv1.SubmitOutcome_SUBMIT_OUTCOME_UNKNOWN
	case errors.Is(err, action.ErrBlocked):
		outcome = gulv1.SubmitOutcome_SUBMIT_OUTCOME_REJECTED
	case err != nil:
		return nil, actionError(err)
	}
	return connect.NewResponse(&gulv1.SubmitResponse{Outcome: outcome, State: browserActionState(state)}), nil
}
