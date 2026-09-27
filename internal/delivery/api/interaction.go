package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/interaction"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InteractionHandler remains unmounted until authenticated product assembly.
type InteractionHandler struct {
	Core         *app.Core
	Principal    PrincipalResolver
	Interactions *interaction.Service
}

func (h *InteractionHandler) access(ctx context.Context) (string, error) {
	if h == nil || h.Core == nil || h.Principal == nil || h.Interactions == nil {
		return "", accessError(connect.CodeUnauthenticated, "interaction access unavailable")
	}
	return localAccess(ctx, h.Core, h.Principal)
}
func (h *InteractionHandler) ListPending(ctx context.Context, request *connect.Request[gulv1.ListPendingRequest]) (*connect.Response[gulv1.ListPendingResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	state, err := h.Interactions.List(ctx, subject, request.Msg.SessionId)
	if err != nil {
		return nil, interactionError(err)
	}
	out := &gulv1.ListPendingResponse{}
	for _, summary := range state.Items {
		out.Summaries = append(out.Summaries, browserInteractionSummary(summary))
	}
	return connect.NewResponse(out), nil
}
func (h *InteractionHandler) GetCard(ctx context.Context, request *connect.Request[gulv1.GetCardRequest]) (*connect.Response[gulv1.GetCardResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	card, err := h.Interactions.Get(ctx, subject, request.Msg.SessionId, request.Msg.InteractionId)
	if err != nil {
		return nil, interactionError(err)
	}
	return connect.NewResponse(&gulv1.GetCardResponse{Card: browserCard(card)}), nil
}
func (h *InteractionHandler) Resolve(ctx context.Context, request *connect.Request[gulv1.ResolveRequest]) (*connect.Response[gulv1.ResolveResponse], error) {
	defer clear(request.Msg.ResponseJson)
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Interactions.Resolve(ctx, subject, request.Msg.SessionId, request.Msg.InteractionId, request.Msg.ResponseJson)
	if err != nil {
		return nil, interactionError(err)
	}
	outcome := gulv1.InteractionResolutionOutcome_INTERACTION_RESOLUTION_OUTCOME_UNKNOWN
	switch result.Outcome {
	case interaction.OutcomeResolved:
		outcome = gulv1.InteractionResolutionOutcome_INTERACTION_RESOLUTION_OUTCOME_RESOLVED
	case interaction.OutcomeReenter:
		outcome = gulv1.InteractionResolutionOutcome_INTERACTION_RESOLUTION_OUTCOME_REENTER
	case interaction.OutcomeStale:
		outcome = gulv1.InteractionResolutionOutcome_INTERACTION_RESOLUTION_OUTCOME_STALE
	}
	out := &gulv1.ResolveResponse{Outcome: outcome, ResolutionReceipt: result.Receipt}
	if result.Card != nil {
		out.PendingCard = browserCard(*result.Card)
	}
	return connect.NewResponse(out), nil
}
func browserInteractionSummary(s interaction.Summary) *gulv1.InteractionCardSummary {
	out := &gulv1.InteractionCardSummary{InteractionId: s.ID, Kind: gulv1.InteractionCardKind(s.Kind), Status: gulv1.InteractionCardStatus(s.Status), CreatedAt: timestamppb.New(s.CreatedAt), ContainsProtectedInput: s.Protected, RequiresUserEscalation: s.RequiresUser}
	if s.ExpiresAt != nil {
		out.ExpiresAt = timestamppb.New(*s.ExpiresAt)
	}
	if s.ResolvedAt != nil {
		out.ResolvedAt = timestamppb.New(*s.ResolvedAt)
	}
	return out
}
func browserCard(c interaction.Card) *gulv1.InteractionCard {
	out := &gulv1.InteractionCard{Summary: browserInteractionSummary(c.Summary), Actions: browserActionFlags(c.Actions.Flags), Blocker: gulv1.ActionBlocker(c.Actions.Blocker + 1)}
	for _, d := range c.Decisions {
		out.Decisions = append(out.Decisions, gulv1.InteractionCardDecision(d))
	}
	switch c.Summary.Kind {
	case interaction.CommandApproval:
		if c.Command != nil {
			v := c.Command
			out.Detail = &gulv1.InteractionCard_CommandApproval{CommandApproval: &gulv1.CommandApprovalCard{Title: v.Title, Message: v.Message, Reason: v.Reason, Command: v.Arguments, RelativeWorkingDirectory: v.WorkingDirectory}}
		}
	case interaction.FileApproval:
		if c.File != nil {
			v := c.File
			f := &gulv1.FileApprovalCard{Title: v.Title, Message: v.Message, Reason: v.Reason, VerifiedDiff: v.VerifiedDiff}
			for _, change := range v.Changes {
				f.Changes = append(f.Changes, &gulv1.FileChangeCard{RelativePath: change.Path, Kind: change.Kind, UnifiedDiff: change.Diff, RelativeMovePath: change.MovePath})
			}
			out.Detail = &gulv1.InteractionCard_FileApproval{FileApproval: f}
		}
	case interaction.UserInput:
		if c.Input != nil {
			input := &gulv1.UserInputCard{IsBlocking: c.Input.Blocking}
			for _, q := range c.Input.Questions {
				question := &gulv1.InteractionQuestionCard{QuestionId: q.ID, Header: q.Header, Question: q.Prompt, AllowsOther: q.AllowsOther, IsSecret: q.Secret}
				for _, o := range q.Options {
					question.Choices = append(question.Choices, &gulv1.InteractionChoice{Label: o.Label, Description: o.Description})
				}
				input.Questions = append(input.Questions, question)
			}
			out.Detail = &gulv1.InteractionCard_UserInput{UserInput: input}
		}
	case interaction.Unsupported:
		out.Detail = &gulv1.InteractionCard_Unsupported{Unsupported: &gulv1.UnsupportedInteractionCard{Blocker: c.UnsupportedReason}}
	}
	return out
}
func interactionError(err error) error {
	code := connect.CodeUnavailable
	detail := &gulv1.DomainError{Code: gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, Action: gulv1.ActionClass_ACTION_CLASS_REFETCH_INTERACTION}
	if errors.Is(err, interaction.ErrAuthority) {
		code = connect.CodePermissionDenied
		detail.Code = gulv1.ErrorCode_ERROR_CODE_CONTROLLER_MISMATCH
		detail.Action = gulv1.ActionClass_ACTION_CLASS_ABORT
	}
	if errors.Is(err, interaction.ErrInvalid) {
		code = connect.CodeInvalidArgument
		detail.Action = gulv1.ActionClass_ACTION_CLASS_ABORT
	}
	if errors.Is(err, interaction.ErrBlocked) {
		code = connect.CodeFailedPrecondition
	}
	if errors.Is(err, interaction.ErrPath) {
		code = connect.CodeFailedPrecondition
		detail.Code = gulv1.ErrorCode_ERROR_CODE_UNSUPPORTED_PATH_ENCODING
	}
	out := connect.NewError(code, errors.New("interaction request could not be completed"))
	if d, e := connect.NewErrorDetail(detail); e == nil {
		out.AddDetail(d)
	}
	return out
}

var _ gulv1connect.InteractionPresentationServiceHandler = (*InteractionHandler)(nil)
