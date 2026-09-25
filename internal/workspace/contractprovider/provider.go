// Package contractprovider adapts the pinned public inspection port to Gul's
// contract-free Workspace service.
package contractprovider

import (
	"context"
	"fmt"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/workspace"
)

// ContractProvider maps only the pinned public workspace inspection contract.
// Supplying a test scenario port is explicit; this adapter is not a CLI fallback.
type ContractProvider struct {
	Port port.RuntimePort
}

func (p ContractProvider) InspectWorkspace(ctx context.Context, absolutePath string, expectedID *string) (workspace.Inspection, error) {
	if p.Port == nil {
		return workspace.Inspection{}, workspace.ErrWorkspaceBlocked
	}
	request := &publicv1.InspectWorkspaceRequest{AbsolutePath: absolutePath}
	if expectedID != nil {
		request.ExpectedWorkspaceId = expectedID
	}
	response, err := p.Port.InspectWorkspace(ctx, request)
	if err != nil {
		mapped := port.MapProviderError(err)
		if expectedID != nil && mapped.Code == "INVALID_REQUEST" {
			return workspace.Inspection{}, workspace.ErrReattachRequired
		}
		return workspace.Inspection{}, fmt.Errorf("%w: %s", workspace.ErrWorkspaceBlocked, mapped.Code)
	}
	if response == nil || response.GetCanonicalPath() == nil || response.GetCanonicalPath().GetUtf8Path() == "" {
		return workspace.Inspection{}, workspace.ErrWorkspaceBlocked
	}
	result := workspace.Inspection{
		CanonicalRoot: response.GetCanonicalPath().GetUtf8Path(),
		ProviderID:    response.GetWorkspaceId(),
		Compatible:    response.GetStatus() == publicv1.WorkspaceInspectionStatus_WORKSPACE_INSPECTION_STATUS_COMPATIBLE,
	}
	for _, blocker := range response.GetBlockers() {
		mapped := workspace.BlockerUnknown
		switch blocker.GetCode() {
		case publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_WORKSPACE_NOT_INITIALIZED:
			mapped = workspace.BlockerUninitialized
		case publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_PROFILE_CONFIG_INVALID,
			publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_PROFILE_RUNTIME_INCOMPATIBLE,
			publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_PROFILE_MIGRATION_REQUIRED,
			publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_PROFILE_MEMBERSHIP_INCOMPLETE:
			mapped = workspace.BlockerProfileMissing
		case publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_PROFILE_SERVER_UNAVAILABLE:
			mapped = workspace.BlockerProfileServerUnavailable
		}
		if blockerPriority(mapped) > blockerPriority(result.Blocker) {
			result.Blocker = mapped
		}
	}
	if response.GetStatus() == publicv1.WorkspaceInspectionStatus_WORKSPACE_INSPECTION_STATUS_UNINITIALIZED {
		result.Blocker = workspace.BlockerUninitialized
	}
	return result, nil
}

func blockerPriority(value workspace.Blocker) int {
	switch value {
	case workspace.BlockerUninitialized:
		return 3
	case workspace.BlockerProfileServerUnavailable:
		return 2
	case workspace.BlockerProfileMissing:
		return 1
	default:
		return 0
	}
}
