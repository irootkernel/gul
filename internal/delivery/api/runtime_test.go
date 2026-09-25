package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/launch"
)

type runtimeProfiles struct{ profile launch.Profile }

func (p runtimeProfiles) ListProfiles(context.Context) ([]launch.Profile, error) {
	return []launch.Profile{p.profile}, nil
}
func (p runtimeProfiles) GetProfile(context.Context, string) (launch.Profile, error) {
	return p.profile, nil
}

func TestRuntimeProfileBrowserBoundaryRequiresExplicitSharedChoice(t *testing.T) {
	ctx := t.Context()
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	profile := launch.Profile{Name: "profile", Compatibility: "compatible", RuntimeVersion: "runtime-1",
		Models:         []launch.Model{{ID: "model", Default: true, SupportedEfforts: []string{"medium"}}},
		SupportedLanes: []string{"dedicated", "shared_readonly"}, MaximumAssurance: "best_effort_personal_alpha",
		FeatureFlags: []string{"safe_client_projection"}, AccessPolicyTransition: "unverified", BackgroundExecution: "unavailable"}
	handler := &RuntimeHandler{Core: core, Launch: launch.NewService(runtimeProfiles{profile: profile}, []string{"named-policy"}),
		Principal: func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }}
	listed, err := handler.ListRuntimeProfiles(ctx, connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{}))
	if err != nil || len(listed.Msg.GetProfiles()) != 1 || listed.Msg.GetProfiles()[0].GetRuntimeVersion() != "runtime-1" ||
		listed.Msg.GetProfiles()[0].GetMaximumAssurance() != gulv1.LaunchAssurance_LAUNCH_ASSURANCE_BEST_EFFORT_PERSONAL_ALPHA ||
		len(listed.Msg.GetProfiles()[0].GetModels()) != 1 || len(listed.Msg.GetProfiles()[0].GetSupportedLanes()) != 2 ||
		listed.Msg.GetPreprovisionedPolicyNames()[0] != "named-policy" ||
		listed.Msg.GetSharedReadonlyWarning() != "Shared read-only access is permanent for this session. It cannot be changed to dedicated access." {
		t.Fatalf("browser profile catalog = %+v, %v", listed, err)
	}
	request := &gulv1.CheckCompatibilityRequest{ProfileName: "profile", ModelId: "model", Effort: "medium",
		Lane:              gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_SHARED_READONLY,
		RequiredAssurance: gulv1.LaunchAssurance_LAUNCH_ASSURANCE_BEST_EFFORT_PERSONAL_ALPHA, PolicyName: "named-policy"}
	if _, err := handler.CheckCompatibility(ctx, connect.NewRequest(request)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unacknowledged browser shared choice = %v", err)
	}
	request.AcknowledgeSharedReadonly = true
	checked, err := handler.CheckCompatibility(ctx, connect.NewRequest(request))
	if err != nil || checked.Msg.GetConfiguration().GetSharedReadonlyWarning() != launch.SharedReadOnlyWarning ||
		checked.Msg.GetConfiguration().GetOrchestrationUseCase() != "dolgorae_orchestrated_session" ||
		checked.Msg.GetConfiguration().GetControllerKind() != "interactive_client" {
		t.Fatalf("acknowledged browser configuration = %+v, %v", checked, err)
	}
	request.ModelId = "unsupported"
	_, err = handler.CheckCompatibility(ctx, connect.NewRequest(request))
	assertWorkspaceError(t, err, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED,
		gulv1.ActionClass_ACTION_CLASS_USE_SUPPORTED_PROFILE)
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{}, nil }
	if _, err := handler.ListRuntimeProfiles(ctx, connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated profile catalog = %v", err)
	}
}

func TestLaunchErrorMapping(t *testing.T) {
	for _, test := range []struct {
		err    error
		status connect.Code
		code   gulv1.ErrorCode
		action gulv1.ActionClass
	}{
		{launch.ErrInvalidChoice, connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, gulv1.ActionClass_ACTION_CLASS_FIX_REQUEST},
		{launch.ErrUnsupported, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, gulv1.ActionClass_ACTION_CLASS_USE_SUPPORTED_PROFILE},
		{launch.ErrInvalidProjection, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR},
		{launch.ErrProviderUnavailable, connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT},
	} {
		assertWorkspaceError(t, launchError(test.err), test.status, test.code, test.action)
	}
	if connect.CodeOf(launchError(context.Canceled)) != connect.CodeCanceled ||
		connect.CodeOf(launchError(context.DeadlineExceeded)) != connect.CodeDeadlineExceeded {
		t.Fatal("context error lost")
	}
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: deny{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	handler := &RuntimeHandler{Core: core, Launch: launch.NewService(runtimeProfiles{}, nil),
		Principal: func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }}
	if _, err := handler.ListRuntimeProfiles(t.Context(), connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("access denial = %v", err)
	}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{}, errors.New("unavailable") }
	if _, err := handler.ListRuntimeProfiles(t.Context(), connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("principal error = %v", err)
	}
}

func TestRuntimeCatalogRejectsOversizeLocalPolicyList(t *testing.T) {
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	policies := make([]string, 101)
	for index := range policies {
		policies[index] = fmt.Sprintf("policy-%d", index)
	}
	handler := &RuntimeHandler{Core: core, Launch: launch.NewService(runtimeProfiles{}, policies),
		Principal: func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }}
	_, err := handler.ListRuntimeProfiles(t.Context(), connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{}))
	assertWorkspaceError(t, err, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED,
		gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
}
