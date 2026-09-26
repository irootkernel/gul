package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/launch"
	"google.golang.org/protobuf/proto"
)

// RuntimeHandler exposes prospective choices only. Route registration and
// actual StartRun remain with later owners.
type RuntimeHandler struct {
	Core      *app.Core
	Launch    *launch.Service
	Principal PrincipalResolver
}

var _ gulv1connect.RuntimeServiceHandler = (*RuntimeHandler)(nil)

func (h *RuntimeHandler) access(ctx context.Context) error {
	if h == nil || h.Core == nil || h.Launch == nil || h.Principal == nil {
		return accessError(connect.CodeUnauthenticated, "product access unavailable")
	}
	principal, err := h.Principal(ctx)
	if err != nil || principal.Subject == "" {
		return accessError(connect.CodeUnauthenticated, "authentication required")
	}
	if err := h.Core.RequireProductAccess(ctx, principal); err != nil {
		if errors.Is(err, app.ErrAccessDenied) {
			return accessError(connect.CodePermissionDenied, "product access denied")
		}
		return launchError(err)
	}
	return nil
}

func (h *RuntimeHandler) ListRuntimeProfiles(ctx context.Context, _ *connect.Request[gulv1.ListRuntimeProfilesRequest]) (*connect.Response[gulv1.ListRuntimeProfilesResponse], error) {
	if err := h.access(ctx); err != nil {
		return nil, err
	}
	profiles, policies, err := h.Launch.List(ctx)
	if err != nil {
		return nil, launchError(err)
	}
	response := &gulv1.ListRuntimeProfilesResponse{PreprovisionedPolicyNames: policies,
		SharedReadonlyWarning: launch.SharedReadOnlyWarning}
	for _, profile := range profiles {
		item := &gulv1.RuntimeProfileChoice{Name: profile.Name, Compatibility: browserProfileCompatibility(profile.Compatibility),
			RuntimeVersion: profile.RuntimeVersion, MaximumAssurance: browserAssurance(profile.MaximumAssurance),
			FeatureFlags: profile.FeatureFlags, SupportedInteractions: profile.SupportedInteractions,
			AccessPolicyTransition: profile.AccessPolicyTransition, BackgroundExecution: profile.BackgroundExecution,
			NativeSubagentsEnabled: profile.NativeSubagentsEnabled}
		for _, model := range profile.Models {
			item.Models = append(item.Models, &gulv1.RuntimeModelChoice{ModelId: model.ID, IsDefault: model.Default, SupportedEfforts: model.SupportedEfforts})
		}
		for _, lane := range profile.SupportedLanes {
			item.SupportedLanes = append(item.SupportedLanes, browserLane(lane))
		}
		response.Profiles = append(response.Profiles, item)
	}
	if len(response.Profiles) > 100 || len(response.PreprovisionedPolicyNames) > 100 ||
		proto.Size(response) > gulv1.MaximumPageMetadataBytes {
		return nil, launchError(launch.ErrInvalidProjection)
	}
	return connect.NewResponse(response), nil
}

func (h *RuntimeHandler) CheckCompatibility(ctx context.Context, request *connect.Request[gulv1.CheckCompatibilityRequest]) (*connect.Response[gulv1.CheckCompatibilityResponse], error) {
	if err := h.access(ctx); err != nil {
		return nil, err
	}
	choice := launch.Choice{ProfileName: request.Msg.GetProfileName(), ModelID: request.Msg.GetModelId(), Effort: request.Msg.GetEffort(),
		Lane: domainLane(request.Msg.GetLane()), RequiredAssurance: domainAssurance(request.Msg.GetRequiredAssurance()),
		PolicyName: request.Msg.GetPolicyName(), AcknowledgeSharedReadOnly: request.Msg.GetAcknowledgeSharedReadonly()}
	configuration, err := h.Launch.Check(ctx, choice)
	if err != nil {
		return nil, launchError(err)
	}
	return connect.NewResponse(&gulv1.CheckCompatibilityResponse{Configuration: &gulv1.ProspectiveLaunchConfiguration{
		ProfileName: configuration.ProfileName, ModelId: configuration.ModelID, Effort: configuration.Effort,
		Lane: browserLane(configuration.Lane), RequiredAssurance: browserAssurance(configuration.RequiredAssurance),
		PolicyName: configuration.PolicyName, ControlMode: configuration.ControlMode, Purpose: configuration.Purpose,
		ControllerKind: configuration.ControllerKind, OrchestrationUseCase: configuration.OrchestrationUseCase,
		SharedReadonlyWarning: configuration.SharedReadOnlyWarning}}), nil
}

func browserProfileCompatibility(value string) gulv1.RuntimeProfileCompatibility {
	switch value {
	case "compatible":
		return gulv1.RuntimeProfileCompatibility_RUNTIME_PROFILE_COMPATIBILITY_COMPATIBLE
	case "incompatible":
		return gulv1.RuntimeProfileCompatibility_RUNTIME_PROFILE_COMPATIBILITY_INCOMPATIBLE
	case "unverified":
		return gulv1.RuntimeProfileCompatibility_RUNTIME_PROFILE_COMPATIBILITY_UNVERIFIED
	case "unavailable":
		return gulv1.RuntimeProfileCompatibility_RUNTIME_PROFILE_COMPATIBILITY_UNAVAILABLE
	default:
		return gulv1.RuntimeProfileCompatibility_RUNTIME_PROFILE_COMPATIBILITY_UNSPECIFIED
	}
}

func browserLane(value string) gulv1.LaunchExecutionLane {
	switch value {
	case "dedicated":
		return gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_DEDICATED
	case "shared_readonly":
		return gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_SHARED_READONLY
	default:
		return gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_UNSPECIFIED
	}
}

func domainLane(value gulv1.LaunchExecutionLane) string {
	switch value {
	case gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_DEDICATED:
		return "dedicated"
	case gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_SHARED_READONLY:
		return "shared_readonly"
	default:
		return ""
	}
}

func browserAssurance(value string) gulv1.LaunchAssurance {
	switch value {
	case "best_effort_personal_alpha":
		return gulv1.LaunchAssurance_LAUNCH_ASSURANCE_BEST_EFFORT_PERSONAL_ALPHA
	case "verified_thread_scoped_control":
		return gulv1.LaunchAssurance_LAUNCH_ASSURANCE_VERIFIED_THREAD_SCOPED_CONTROL
	case "strong_process_containment":
		return gulv1.LaunchAssurance_LAUNCH_ASSURANCE_STRONG_PROCESS_CONTAINMENT
	default:
		return gulv1.LaunchAssurance_LAUNCH_ASSURANCE_UNSPECIFIED
	}
}

func domainAssurance(value gulv1.LaunchAssurance) string {
	switch value {
	case gulv1.LaunchAssurance_LAUNCH_ASSURANCE_BEST_EFFORT_PERSONAL_ALPHA:
		return "best_effort_personal_alpha"
	case gulv1.LaunchAssurance_LAUNCH_ASSURANCE_VERIFIED_THREAD_SCOPED_CONTROL:
		return "verified_thread_scoped_control"
	case gulv1.LaunchAssurance_LAUNCH_ASSURANCE_STRONG_PROCESS_CONTAINMENT:
		return "strong_process_containment"
	default:
		return ""
	}
}

func launchError(err error) error {
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, errors.New("launch choice cancelled"))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("launch choice timed out"))
	}
	code, action, status, message := gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT, connect.CodeUnavailable, "runtime profile unavailable"
	switch {
	case errors.Is(err, launch.ErrInvalidChoice):
		code, action, status, message = gulv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, gulv1.ActionClass_ACTION_CLASS_FIX_REQUEST, connect.CodeInvalidArgument, "invalid launch choice"
	case errors.Is(err, launch.ErrUnsupported):
		code, action, status, message = gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, gulv1.ActionClass_ACTION_CLASS_USE_SUPPORTED_PROFILE, connect.CodeFailedPrecondition, "launch choice unsupported"
	case errors.Is(err, launch.ErrInvalidProjection):
		code, action, status, message = gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR, connect.CodeFailedPrecondition, "runtime profile projection invalid"
	}
	result := connect.NewError(status, errors.New(message))
	if detail, detailErr := connect.NewErrorDetail(&gulv1.DomainError{Code: code, Action: action}); detailErr == nil {
		result.AddDetail(detail)
	}
	return result
}
