package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/action"
)

func (h *DirectPresentationHandler) Submit(ctx context.Context, q *connect.Request[gulv1.SubmitRequest]) (*connect.Response[gulv1.SubmitResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	if h.Submissions == nil {
		return nil, actionError(action.ErrUnavailable)
	}
	if q == nil || q.Msg == nil || len(q.Msg.Text) > gulv1.MaximumSubmitTextBytes || q.Msg.WriteIntent != gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ && q.Msg.WriteIntent != gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE {
		return nil, actionError(action.ErrInvalid)
	}
	intent := action.IntentRead
	if q.Msg.WriteIntent == gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE {
		intent = action.IntentWrite
	}
	state, err := h.Submissions.Send(ctx, subject, q.Msg.SessionId, q.Msg.AttemptId, q.Msg.Text, intent)
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
