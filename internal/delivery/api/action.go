package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/app"
)

// WriterHandler is unmounted until authenticated product assembly. Only writer
// Acquire/Release execute here; lifecycle coordination belongs to E5/E2.
type WriterHandler struct {
	Core      *app.Core
	Principal PrincipalResolver
	Actions   *action.Service
}

func (h *WriterHandler) access(ctx context.Context) (string, error) {
	if h == nil || h.Core == nil || h.Principal == nil || h.Actions == nil {
		return "", accessError(connect.CodeUnauthenticated, "action access unavailable")
	}
	return localAccess(ctx, h.Core, h.Principal)
}
func (h *WriterHandler) GetActionState(ctx context.Context, request *connect.Request[gulv1.GetActionStateRequest]) (*connect.Response[gulv1.GetActionStateResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	q := request.Msg
	if q.WriteIntent != gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ && q.WriteIntent != gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE || q.CloseIntent < gulv1.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE || q.CloseIntent > gulv1.ActionCloseIntent_ACTION_CLOSE_INTENT_ABORT {
		return nil, actionError(action.ErrInvalid)
	}
	intent := action.IntentRead
	if q.WriteIntent == gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE {
		intent = action.IntentWrite
	}
	state, err := h.Actions.Evaluate(ctx, subject, q.SessionId, action.Request{Intent: intent, CloseIntent: action.CloseIntent(q.CloseIntent), InterruptConfirmed: q.InterruptConfirmed})
	if err != nil {
		return nil, actionError(err)
	}
	return connect.NewResponse(&gulv1.GetActionStateResponse{State: browserActionState(state)}), nil
}
func (h *WriterHandler) AcquireWriter(ctx context.Context, request *connect.Request[gulv1.AcquireWriterRequest]) (*connect.Response[gulv1.AcquireWriterResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Actions.Acquire(ctx, subject, request.Msg.SessionId)
	if err != nil {
		return nil, actionError(err)
	}
	return connect.NewResponse(&gulv1.AcquireWriterResponse{State: browserActionState(result.Evaluation)}), nil
}
func (h *WriterHandler) ReleaseWriter(ctx context.Context, request *connect.Request[gulv1.ReleaseWriterRequest]) (*connect.Response[gulv1.ReleaseWriterResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Actions.Release(ctx, subject, request.Msg.SessionId)
	if err != nil {
		return nil, actionError(err)
	}
	return connect.NewResponse(&gulv1.ReleaseWriterResponse{State: browserActionState(result.Evaluation)}), nil
}
func browserActionFlags(f action.Flags) *gulv1.ActionFlags {
	return &gulv1.ActionFlags{CanSubmitRead: f.CanSubmitRead, CanSubmitWrite: f.CanSubmitWrite, CanAcquireWriter: f.CanAcquireWriter, CanReleaseWriter: f.CanReleaseWriter, CanInterrupt: f.CanInterrupt, CanResolveInteraction: f.CanResolveInteraction, CanRecover: f.CanRecover, CanReconcile: f.CanReconcile, CanAdoptController: f.CanAdoptController, CanPausePrimary: f.CanPausePrimary, CanResumePrimary: f.CanResumePrimary, CanRequestSessionClose: f.CanRequestSessionClose, RequiresCloseConfirmation: f.RequiresCloseConfirmation, RequiresOperatorAction: f.RequiresOperatorAction, RequiresFreshSnapshot: f.RequiresFreshSnapshot, BlockedByOutcomeUnknown: f.BlockedByOutcomeUnknown, BlockedByCredentialState: f.BlockedByCredentialState, BlockedByBackgroundExecution: f.BlockedByBackgroundExecution, BlockedByProviderCompatibility: f.BlockedByProviderCompatibility}
}
func browserActionState(e action.Evaluation) *gulv1.ActionState {
	w := e.Writer
	lane := gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_UNSPECIFIED
	switch w.Lane {
	case action.Shared:
		lane = gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_SHARED_READONLY
	case action.Dedicated:
		lane = gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_DEDICATED
	}
	mode := gulv1.WriterAccessMode_WRITER_ACCESS_MODE_BLOCKED
	switch e.Mode {
	case action.WriterReadOnly:
		mode = gulv1.WriterAccessMode_WRITER_ACCESS_MODE_READ_ONLY
	case action.WriterWrite:
		mode = gulv1.WriterAccessMode_WRITER_ACCESS_MODE_WRITE
	}
	return &gulv1.ActionState{Flags: browserActionFlags(e.Flags), Blocker: gulv1.ActionBlocker(e.Blocker + 1), Mode: mode, Writer: &gulv1.WriterPresentation{Authority: gulv1.WriterAuthority(w.Authority), Generation: w.Generation, EffectiveAccess: gulv1.WriterEffectiveAccess(w.Access), PolicyVerification: gulv1.WriterPolicyVerification(w.Verification), Lane: lane, RequestedAssurance: gulv1.LaunchAssurance(w.Requested), AchievedAssurance: gulv1.LaunchAssurance(w.Achieved), Owner: gulv1.WriterOwner(w.Owner), BackgroundBlocked: w.BackgroundBlocked, RecoveryBlocked: w.RecoveryBlocked}}
}
func actionError(err error) error {
	blocker := action.ProviderIncompatible
	code := connect.CodeFailedPrecondition
	detail := &gulv1.DomainError{Code: gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, Action: gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT}
	switch {
	case errors.Is(err, action.ErrInvalid):
		code = connect.CodeInvalidArgument
		detail.Action = gulv1.ActionClass_ACTION_CLASS_FIX_REQUEST
	case errors.Is(err, action.ErrAuthority):
		blocker = action.CredentialBlocked
		code = connect.CodePermissionDenied
		detail.Code = gulv1.ErrorCode_ERROR_CODE_CONTROLLER_MISMATCH
		detail.Action = gulv1.ActionClass_ACTION_CLASS_VERIFY_CONTROLLER
	case errors.Is(err, action.ErrPersistence):
		blocker = action.FreshSnapshotRequired
		code = connect.CodeUnavailable
		detail.Code = gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE
		detail.Action = gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR
	case errors.Is(err, action.ErrWriterBusy):
		blocker = action.WriterBusy
		detail.Code = gulv1.ErrorCode_ERROR_CODE_WRITER_CONFLICT
		detail.Action = gulv1.ActionClass_ACTION_CLASS_WAIT
	case errors.Is(err, action.ErrUnsupportedTransition):
		blocker = action.UnsupportedTransition
		detail.Action = gulv1.ActionClass_ACTION_CLASS_ABORT
	case errors.Is(err, action.ErrOutcomeUnknown):
		blocker = action.UnresolvedOutcome
		code = connect.CodeUnavailable
		detail.Code = gulv1.ErrorCode_ERROR_CODE_OUTCOME_UNKNOWN
	case errors.Is(err, action.ErrUnavailable):
		blocker = action.ProviderFailure
		code = connect.CodeUnavailable
		detail.Code = gulv1.ErrorCode_ERROR_CODE_TRANSPORT_UNAVAILABLE
	}
	out := connect.NewError(code, errors.New("action could not be completed"))
	if d, e := connect.NewErrorDetail(&gulv1.ActionFailure{Blocker: gulv1.ActionBlocker(blocker + 1)}); e == nil {
		out.AddDetail(d)
	}
	if d, e := connect.NewErrorDetail(detail); e == nil {
		out.AddDetail(d)
	}
	return out
}

var _ gulv1connect.WriterActionServiceHandler = (*WriterHandler)(nil)
