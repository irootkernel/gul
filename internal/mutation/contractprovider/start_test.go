package contractprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/replay"
)

type startWire struct {
	request *publicv1.StartRunRequest
	list    *publicv1.ListRunsRequest
	badKey  bool
}

func (p *startWire) StartRun(_ context.Context, request *publicv1.StartRunRequest) (*publicv1.StartRunResponse, error) {
	p.request = request
	key := request.GetIdempotencyKey()
	if p.badKey {
		key = "different"
	}
	return &publicv1.StartRunResponse{IdempotencyKey: key, Run: validRun()}, nil
}
func (p *startWire) ListRuns(_ context.Context, request *publicv1.ListRunsRequest) (*publicv1.ListRunsResponse, error) {
	p.list = request
	return &publicv1.ListRunsResponse{Items: []*publicv1.RunProjection{validRun()}}, nil
}

func validRun() *publicv1.RunProjection {
	return &publicv1.RunProjection{RunId: "run", WorkspaceId: "provider-workspace", StateRevision: 1,
		Controller:    &publicv1.ControllerProjection{ControllerId: "controller", Generation: 1},
		ControlMode:   publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED,
		Configuration: &publicv1.RunConfigurationProjection{ProfileName: "profile"}}
}

func TestStartAdapterReconstructsCarrierAndChecksExactResponse(t *testing.T) {
	port := &startWire{}
	request := replay.StartRun{OperationID: "attempt", SubjectID: "owner", WorkspaceID: "workspace", ProviderWorkspaceID: "provider-workspace",
		CredentialKey: "controllers/key", ControllerID: "controller", ControllerGeneration: 1, IdempotencyKey: "same-key",
		ProfileName: "profile", ControlMode: int32(publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE),
		ExecutionLane: int32(publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED), Purpose: int32(publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE),
		RequiredAssurance: int32(publicv1.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA), CreatedAt: time.Now()}
	provider := StartProvider{Port: port}
	result, err := provider.StartRun(t.Context(), request, "/trusted/workspace", "/trusted/carrier")
	if err != nil || result.RunID != "run" || port.request.GetWorkspace().GetAbsolutePath() != "/trusted/workspace" ||
		port.request.GetController().GetAbsoluteFilePath() != "/trusted/carrier" || port.request.GetIdempotencyKey() != "same-key" {
		t.Fatalf("reconstructed request = %+v, %+v, %v", port.request, result, err)
	}
	runs, err := provider.ListRunsByController(t.Context(), request, "/trusted/workspace")
	if err != nil || len(runs) != 1 || port.list.GetControllerId() != "controller" {
		t.Fatalf("secondary run lookup = %+v, %v", runs, err)
	}
	port.badKey = true
	if _, err := provider.StartRun(t.Context(), request, "/trusted/workspace", "/trusted/carrier"); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("mismatched key = %v", err)
	}
}
