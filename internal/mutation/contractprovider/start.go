// Package contractprovider binds mutation coordination to the checked public
// Dolgorae gRPC messages. The Machine CLI is not a production fallback.
package contractprovider

import (
	"context"
	"errors"
	"slices"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/mutation"
	"github.com/rootkernel/gul/internal/replay"
)

var ErrInvalidProjection = errors.New("invalid provider mutation projection")

type StartPort interface {
	StartRun(context.Context, *publicv1.StartRunRequest) (*publicv1.StartRunResponse, error)
	ListRuns(context.Context, *publicv1.ListRunsRequest) (*publicv1.ListRunsResponse, error)
}

type StartProvider struct{ Port StartPort }

func (p StartProvider) StartRun(ctx context.Context, request replay.StartRun, root, carrier string) (mutation.StartResult, error) {
	if p.Port == nil || root == "" || carrier == "" || request.ControllerID == "" || request.ControllerGeneration == 0 {
		return mutation.StartResult{}, mutation.ErrInvalid
	}
	message := &publicv1.StartRunRequest{Workspace: &publicv1.WorkspaceRef{AbsolutePath: root, ExpectedWorkspaceId: request.ProviderWorkspaceID},
		Controller: &publicv1.ControllerCarrierRef{AbsoluteFilePath: carrier, ExpectedControllerId: request.ControllerID,
			ExpectedControllerGeneration: request.ControllerGeneration}, IdempotencyKey: request.IdempotencyKey,
		ProfileName: request.ProfileName, ControlMode: publicv1.ControlMode(request.ControlMode), ExecutionLane: publicv1.ExecutionLane(request.ExecutionLane),
		Purpose: publicv1.PurposeKind(request.Purpose), PurposeLabel: request.PurposeLabel, Model: request.Model, Effort: request.Effort,
		RequiredAssurance: publicv1.AssuranceLevel(request.RequiredAssurance), RequiredCapabilities: append([]string(nil), request.RequiredCapabilities...),
		Instructions: request.Instructions}
	if message.ControlMode != publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE || message.Purpose == publicv1.PurposeKind_PURPOSE_KIND_UNSPECIFIED ||
		message.ExecutionLane == publicv1.ExecutionLane_EXECUTION_LANE_UNSPECIFIED || message.RequiredAssurance == publicv1.AssuranceLevel_ASSURANCE_LEVEL_UNSPECIFIED {
		return mutation.StartResult{}, mutation.ErrInvalid
	}
	response, err := p.Port.StartRun(ctx, message)
	if err != nil {
		return mutation.StartResult{}, err
	}
	if response == nil || response.GetIdempotencyKey() != request.IdempotencyKey {
		return mutation.StartResult{}, ErrInvalidProjection
	}
	run := response.GetRun()
	if !matchesRun(run, request) || run.GetConfiguration().GetProfileName() != request.ProfileName ||
		run.GetControlMode() != message.ControlMode || run.GetExecutionLane() != message.ExecutionLane ||
		run.GetRequestedAssurance() != message.RequiredAssurance || run.GetConfiguration().GetPurpose() != message.Purpose ||
		run.GetConfiguration().GetPurposeLabel() != message.GetPurposeLabel() || !slices.Equal(run.GetConfiguration().GetRequiredCapabilities(), message.RequiredCapabilities) ||
		(request.Model != nil && run.GetConfiguration().GetModelId() != *request.Model) || (request.Effort != nil && run.GetConfiguration().GetDefaultEffort() != *request.Effort) {
		return mutation.StartResult{}, ErrInvalidProjection
	}
	return mutation.StartResult{RunID: run.GetRunId(), WorkspaceID: run.GetWorkspaceId(), ControllerID: run.GetController().GetControllerId()}, nil
}

func (p StartProvider) ListRunsByController(ctx context.Context, request replay.StartRun, root string) ([]mutation.StartResult, error) {
	if p.Port == nil || root == "" || request.ProviderWorkspaceID == "" || request.ControllerID == "" {
		return nil, mutation.ErrInvalid
	}
	response, err := p.Port.ListRuns(ctx, &publicv1.ListRunsRequest{Workspace: &publicv1.WorkspaceRef{AbsolutePath: root,
		ExpectedWorkspaceId: request.ProviderWorkspaceID}, ControllerId: &request.ControllerID})
	if err != nil {
		return nil, err
	}
	if response == nil || len(response.GetItems()) > 256 {
		return nil, ErrInvalidProjection
	}
	var runs []mutation.StartResult
	for _, run := range response.GetItems() {
		if !matchesRun(run, request) {
			return nil, ErrInvalidProjection
		}
		runs = append(runs, mutation.StartResult{RunID: run.GetRunId(), WorkspaceID: run.GetWorkspaceId(), ControllerID: run.GetController().GetControllerId()})
	}
	return runs, nil
}

func matchesRun(run *publicv1.RunProjection, request replay.StartRun) bool {
	return run != nil && run.GetRunId() != "" && run.GetWorkspaceId() == request.ProviderWorkspaceID &&
		run.GetController().GetControllerId() == request.ControllerID &&
		run.GetController().GetGeneration() == request.ControllerGeneration && run.GetConfiguration() != nil &&
		run.GetController().GetKind() == publicv1.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT &&
		run.GetConfiguration().GetParent() == nil && run.GetStateRevision() > 0
}

var _ mutation.StartProvider = StartProvider{}
