package contractprovider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProviderStatusUsesCheckedErrorsWithoutProviderText(t *testing.T) {
	for _, tc := range []struct {
		code   string
		action publicv1.RequiredClientAction
		want   error
	}{
		{"PROTOCOL_VERSION_UNSUPPORTED", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_CAPABILITIES, session.ErrIncompatible},
		{"CAPABILITY_UNSUPPORTED", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_CAPABILITIES, session.ErrIncompatible},
		{"PROFILE_SERVER_BUSY", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT, session.ErrBusy},
		{"RUN_BUSY", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT, session.ErrBusy},
		{"RECOVERY_REQUIRED", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECOVER_RUN, session.ErrDegraded},
		{"OUTCOME_UNKNOWN", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONCILE_RUN, session.ErrDegraded},
		{"CONTROLLER_MISMATCH", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_VERIFY_CONTROLLER, session.ErrInvalidProjection},
		{"TRANSPORT_FAILURE", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT, session.ErrUnavailable},
	} {
		t.Run(tc.code, func(t *testing.T) {
			err := connect.NewError(connect.CodeFailedPrecondition, errors.New("private provider diagnostic"))
			detail, detailErr := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{
				DetailVersion: 1, DolgoraeErrorCode: tc.code, Action: tc.action,
				RetryClassification:    publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN,
				RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE,
			})
			if detailErr != nil {
				t.Fatal(detailErr)
			}
			err.AddDetail(detail)
			if got := readError(err); got != tc.want {
				t.Fatalf("mapped error = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProviderErrorTextCannotInventBusyStateOrHideAuthorizationFailure(t *testing.T) {
	if got := readError(connect.NewError(connect.CodeUnavailable, errors.New("PROFILE_SERVER_BUSY"))); got != session.ErrUnavailable {
		t.Fatalf("untyped status = %v", got)
	}
	for _, code := range []connect.Code{connect.CodeUnauthenticated, connect.CodePermissionDenied} {
		if got := readError(connect.NewError(code, errors.New("PROFILE_SERVER_BUSY"))); got != session.ErrInvalidProjection {
			t.Fatalf("authorization error = %v", got)
		}
	}
}

type operationIdentityPort struct {
	operation *string
	lists     int
}

func (p *operationIdentityPort) GetRun(_ context.Context, q *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	return &publicv1.GetRunResponse{Run: &publicv1.RunProjection{
		RunId: q.Run.RunId, WorkspaceId: q.Run.Workspace.ExpectedWorkspaceId,
		Controller:    &publicv1.ControllerProjection{ControllerId: "controller"},
		ControlMode:   publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		Configuration: &publicv1.RunConfigurationProjection{ProfileName: "profile"}, StateRevision: 1,
	}}, nil
}
func (p *operationIdentityPort) GetOrchestratedSession(_ context.Context, q *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error) {
	return &publicv1.GetOrchestratedSessionResponse{Session: &publicv1.OrchestratedSessionProjection{
		SessionId: "session", PrimaryRun: q.RootRun, AggregateRevision: 1,
		CapturedAt: timestamppb.New(time.Now()), CloseOperationId: p.operation,
		Availability:           publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_AVAILABLE,
		Lifecycle:              publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_COMPLETING,
		Composition:            publicv1.OrchestratedSessionComposition_ORCHESTRATED_SESSION_COMPOSITION_STANDALONE_PRIMARY,
		ApprovalPolicy:         publicv1.OrchestratedSessionApprovalPolicy_ORCHESTRATED_SESSION_APPROVAL_POLICY_USER_APPROVAL_REQUIRED,
		CloseProgress:          publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING,
		RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE,
	}}, nil
}
func (p *operationIdentityPort) ListRuns(context.Context, *publicv1.ListRunsRequest) (*publicv1.ListRunsResponse, error) {
	p.lists++
	return &publicv1.ListRunsResponse{}, nil
}

func TestSessionProjectionValidatesSuppliedCloseOperationIdentity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *string
		valid bool
	}{
		{"absent", nil, true},
		{"normal", proto.String("operation"), true},
		{"maximum bytes", proto.String(strings.Repeat("x", 1024)), true},
		{"maximum UTF8 bytes", proto.String(strings.Repeat("é", 512)), true},
		{"supplied empty", proto.String(""), false},
		{"oversized", proto.String(strings.Repeat("x", 1025)), false},
		{"oversized UTF8", proto.String(strings.Repeat("é", 513)), false},
		{"invalid UTF8", proto.String(string([]byte{0xff})), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := &operationIdentityPort{operation: tc.value}
			out, err := (Provider{Port: port}).Snapshot(t.Context(), workspace.Attachment{CanonicalRoot: "/workspace", ProviderID: "workspace"}, "root", session.Carrier{AbsolutePath: "/private/carrier", ControllerID: "controller", Generation: 1})
			if tc.valid {
				expected := ""
				if tc.value != nil {
					expected = *tc.value
				}
				if err != nil || out.CloseOperationID != expected || port.lists != 1 {
					t.Fatalf("valid identity: %+v, %v", out, err)
				}
			} else if !errors.Is(err, session.ErrInvalidProjection) || out.CloseOperationID != "" || port.lists != 0 {
				t.Fatalf("invalid identity escaped boundary: %+v, %v", out, err)
			}
		})
	}
}
