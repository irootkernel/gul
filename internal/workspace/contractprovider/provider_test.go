package contractprovider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/workspace"
	"github.com/rootkernel/gul/internal/workspace/contractprovider"
)

func TestExpectedIdentityRejectedByPinnedPortRequiresReattachment(t *testing.T) {
	provider := contractprovider.ContractProvider{Port: scenario.New(time.Now())}
	path := t.TempDir()
	initial, err := provider.InspectWorkspace(t.Context(), path, nil)
	if err != nil || initial.ProviderID == "" {
		t.Fatalf("initial inspection = %+v, %v", initial, err)
	}
	wrong := "different"
	if _, err := provider.InspectWorkspace(t.Context(), path, &wrong); !errors.Is(err, workspace.ErrReattachRequired) {
		t.Fatalf("provider-rejected expected identity = %v", err)
	}
}

type inspectionPort struct {
	port.RuntimePort
	blockers []*publicv1.CapabilityBlocker
}

func (p inspectionPort) InspectWorkspace(context.Context, *publicv1.InspectWorkspaceRequest) (*publicv1.InspectWorkspaceResponse, error) {
	return &publicv1.InspectWorkspaceResponse{
		WorkspaceId: "workspace", Status: publicv1.WorkspaceInspectionStatus_WORKSPACE_INSPECTION_STATUS_BLOCKED,
		CanonicalPath: &publicv1.PathProjection{Value: &publicv1.PathProjection_Utf8Path{Utf8Path: "/workspace"}},
		Blockers:      p.blockers,
	}, nil
}

func TestInspectionScansAllTypedBlockers(t *testing.T) {
	provider := contractprovider.ContractProvider{Port: inspectionPort{blockers: []*publicv1.CapabilityBlocker{
		{Code: publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_EXECUTION_LANE_UNSUPPORTED},
		{Code: publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_PROFILE_MIGRATION_REQUIRED},
		{Code: publicv1.CapabilityBlockerCode_CAPABILITY_BLOCKER_CODE_PROFILE_SERVER_UNAVAILABLE},
	}}}
	result, err := provider.InspectWorkspace(t.Context(), "/workspace", nil)
	if err != nil || result.Blocker != workspace.BlockerProfileServerUnavailable {
		t.Fatalf("ordered blockers = %+v, %v", result, err)
	}
}
