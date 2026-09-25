package api

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *DirectPresentationHandler) ListDirectSessions(ctx context.Context, request *connect.Request[gulv1.ListDirectSessionsRequest]) (*connect.Response[gulv1.ListDirectSessionsResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	if h.Sessions == nil {
		return nil, sessionError(session.ErrUnavailable)
	}
	bindings, err := h.Sessions.List(ctx, subject, request.Msg.GetWorkspaceId())
	if err != nil {
		return nil, sessionError(err)
	}
	response := &gulv1.ListDirectSessionsResponse{}
	for _, binding := range bindings {
		entry, err := h.Presentation.DirectSession(ctx, subject, binding.ID)
		if err != nil {
			return nil, presentationError(err)
		}
		response.Sessions = append(response.Sessions, browserDirect(entry))
	}
	return connect.NewResponse(response), nil
}

func (h *DirectPresentationHandler) GetExecutionState(ctx context.Context, request *connect.Request[gulv1.GetExecutionStateRequest]) (*connect.Response[gulv1.GetExecutionStateResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	if h.Sessions == nil {
		return nil, sessionError(session.ErrUnavailable)
	}
	state, err := h.Sessions.GetExecutionState(ctx, subject, request.Msg.GetSessionId())
	if err != nil {
		return nil, sessionError(err)
	}
	response := &gulv1.GetExecutionStateResponse{SessionId: state.SessionID, StateVersion: state.StateVersion}
	switch state.Freshness {
	case "fresh":
		response.Freshness = gulv1.Freshness_FRESHNESS_FRESH
	case "stale":
		response.Freshness = gulv1.Freshness_FRESHNESS_STALE
	default:
		response.Freshness = gulv1.Freshness_FRESHNESS_UNAVAILABLE
	}
	if !state.ObservedAt.IsZero() {
		response.ObservedAt = timestamppb.New(state.ObservedAt)
	}
	if snapshot := state.Snapshot; snapshot != nil {
		response.Lifecycle = browserLifecycle(snapshot.Lifecycle)
		response.Composition = browserComposition(snapshot.Composition)
		response.ApprovalPolicy = browserApproval(snapshot.ApprovalPolicy)
		response.SpecialistPolicyName = snapshot.SpecialistPolicyName
		response.CloseProgress = browserCloseProgress(snapshot.CloseProgress)
		response.Recovery = browserRecovery(snapshot.Recovery)
		response.ObservedMembersTruncated = snapshot.MembersTruncated
		response.Counts = &gulv1.ExecutionCounts{
			NonretiredMembers: &snapshot.Counts.NonretiredMembers, NonterminalSpawns: &snapshot.Counts.NonterminalSpawns,
			PendingApprovals: &snapshot.Counts.PendingApprovals, AcceptedUnfinishedTasks: &snapshot.Counts.AcceptedUnfinishedTasks,
			UnknownOutcomeTasks: &snapshot.Counts.UnknownOutcomeTasks, PublishedResults: &snapshot.Counts.PublishedResults}
		for index, member := range snapshot.Members {
			response.ObservedMembers = append(response.ObservedMembers, &gulv1.ObservedMember{ObservedRef: fmt.Sprintf("observed-%d", index+1), Lifecycle: browserMemberLifecycle(member.Lifecycle)})
		}
	}
	return connect.NewResponse(response), nil
}

func browserMemberLifecycle(value string) gulv1.ObservedMemberLifecycle {
	switch value {
	case "starting":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_STARTING
	case "idle":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_IDLE
	case "running":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_RUNNING
	case "waiting_interaction":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_WAITING_INTERACTION
	case "reconciliation_required":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_RECONCILIATION_REQUIRED
	case "paused":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_PAUSED
	case "closed":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_CLOSED
	case "start_failed":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_START_FAILED
	case "outcome_unknown":
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_OUTCOME_UNKNOWN
	default:
		return gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_UNSPECIFIED
	}
}

func browserLifecycle(value string) gulv1.SessionLifecycle {
	switch value {
	case "creating":
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_CREATING
	case "active":
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_ACTIVE
	case "degraded":
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_DEGRADED
	case "recovering":
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_RECOVERING
	case "completing", "aborting":
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_CLOSING
	case "completed":
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_CLOSED
	case "aborted":
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_ABORTED
	default:
		return gulv1.SessionLifecycle_SESSION_LIFECYCLE_UNSPECIFIED
	}
}

func browserComposition(value string) gulv1.SessionComposition {
	switch value {
	case "standalone_primary":
		return gulv1.SessionComposition_SESSION_COMPOSITION_STANDALONE_PRIMARY
	case "brokered_hierarchy":
		return gulv1.SessionComposition_SESSION_COMPOSITION_BROKERED_HIERARCHY
	default:
		return gulv1.SessionComposition_SESSION_COMPOSITION_UNSPECIFIED
	}
}

func browserApproval(value string) gulv1.ApprovalPolicy {
	switch value {
	case "user_approval_required":
		return gulv1.ApprovalPolicy_APPROVAL_POLICY_USER_APPROVAL_REQUIRED
	case "fully_delegated":
		return gulv1.ApprovalPolicy_APPROVAL_POLICY_FULLY_DELEGATED
	default:
		return gulv1.ApprovalPolicy_APPROVAL_POLICY_UNSPECIFIED
	}
}

func browserCloseProgress(value string) gulv1.CloseProgress {
	switch value {
	case "none":
		return gulv1.CloseProgress_CLOSE_PROGRESS_NONE
	case "settling":
		return gulv1.CloseProgress_CLOSE_PROGRESS_SETTLING
	case "confirmed":
		return gulv1.CloseProgress_CLOSE_PROGRESS_CONFIRMED
	case "outcome_unknown":
		return gulv1.CloseProgress_CLOSE_PROGRESS_OUTCOME_UNKNOWN
	case "recovery_required":
		return gulv1.CloseProgress_CLOSE_PROGRESS_RECOVERY_REQUIRED
	default:
		return gulv1.CloseProgress_CLOSE_PROGRESS_UNSPECIFIED
	}
}

func browserRecovery(value string) gulv1.RecoveryClass {
	switch value {
	case "none":
		return gulv1.RecoveryClass_RECOVERY_CLASS_NONE
	case "snapshot_required":
		return gulv1.RecoveryClass_RECOVERY_CLASS_SNAPSHOT_REQUIRED
	case "reconcile_required":
		return gulv1.RecoveryClass_RECOVERY_CLASS_RECONCILE_REQUIRED
	case "outcome_unknown":
		return gulv1.RecoveryClass_RECOVERY_CLASS_OUTCOME_UNKNOWN
	default:
		return gulv1.RecoveryClass_RECOVERY_CLASS_UNSPECIFIED
	}
}

func sessionError(err error) error {
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, errors.New("session request cancelled"))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("session request timed out"))
	}
	code, action, status, message := gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT, connect.CodeUnavailable, "session unavailable"
	switch {
	case errors.Is(err, session.ErrInvalid):
		status, message = connect.CodeInvalidArgument, "invalid session request"
	case errors.Is(err, session.ErrNotFound), errors.Is(err, presentation.ErrNotFound):
		status, message = connect.CodeNotFound, "session not found"
	case errors.Is(err, session.ErrInvalidProjection):
		code, action, status, message = gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR, connect.CodeFailedPrecondition, "session projection invalid"
	case errors.Is(err, session.ErrCarrierUnavailable):
		code, action, status, message = gulv1.ErrorCode_ERROR_CODE_CONTROLLER_CARRIER_INVALID, gulv1.ActionClass_ACTION_CLASS_VERIFY_CONTROLLER, connect.CodeFailedPrecondition, "session controller carrier unavailable"
	case errors.Is(err, session.ErrPersistenceUnavailable):
		code, action, message = gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR, "session persistence unavailable"
	case errors.Is(err, workspace.ErrReattachRequired), errors.Is(err, workspace.ErrIdentityMismatch),
		errors.Is(err, workspace.ErrWorkspaceUninitialized), errors.Is(err, workspace.ErrProfileMissing),
		errors.Is(err, workspace.ErrWorkspaceBlocked), errors.Is(err, workspace.ErrPersistenceUnavailable):
		return workspaceError(err)
	}
	result := connect.NewError(status, errors.New(message))
	if detail, detailErr := connect.NewErrorDetail(&gulv1.DomainError{Code: code, Action: action}); detailErr == nil {
		result.AddDetail(detail)
	}
	return result
}
