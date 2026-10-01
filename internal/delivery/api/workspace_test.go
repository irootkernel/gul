package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
	"github.com/rootkernel/gul/internal/workspace/contractprovider"
)

type ready struct{}

func (ready) Ready(context.Context) error { return nil }

type unavailable struct{}

func (unavailable) Ready(context.Context) error { return errors.New("offline") }

type allow struct{}

func (allow) Authorize(context.Context, app.Principal) error { return nil }

type deny struct{}

func (deny) Authorize(context.Context, app.Principal) error { return errors.New("denied") }

func TestWorkspaceHandlerRequiresTrustedPrincipalAndKeepsProviderIdentityPrivate(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "project"), 0700); err != nil {
		t.Fatal(err)
	}
	database := t.TempDir()
	if err := os.Chmod(database, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.Context(), filepath.Join(database, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	handler := &WorkspaceHandler{Core: core,
		Workspaces: workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, store.Presentation(), nil, []string{root})}
	request := connect.NewRequest(&gulv1.RegisterFromAllowlistPathRequest{RootId: "root-1", RelativePath: "project"})
	if _, err := handler.RegisterFromAllowlistPath(t.Context(), request); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("untrusted registration = %v", err)
	}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	registered, err := handler.RegisterFromAllowlistPath(t.Context(), request)
	if err != nil || registered.Msg.GetWorkspace().GetWorkspaceId() == "" {
		t.Fatalf("registration = %+v, %v", registered, err)
	}
	entryID := registered.Msg.GetWorkspace().GetWorkspaceId()
	if err := store.Presentation().SetWorkspaceFavorite(t.Context(), "owner", entryID, true); err != nil {
		t.Fatal(err)
	}
	listed, err := handler.ListWorkspaces(t.Context(), connect.NewRequest(&gulv1.ListWorkspacesRequest{}))
	if err != nil || len(listed.Msg.GetWorkspaces()) != 1 || listed.Msg.GetWorkspaces()[0].GetWorkspaceId() != entryID || !listed.Msg.GetWorkspaces()[0].GetFavorite() {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	if _, err := handler.RevalidateWorkspace(t.Context(), connect.NewRequest(&gulv1.RevalidateWorkspaceRequest{WorkspaceId: entryID})); err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	_, err = handler.BrowseRegistrableRoot(t.Context(), connect.NewRequest(&gulv1.BrowseRegistrableRootRequest{RootId: "root-1", RelativePath: "../outside"}))
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeInvalidArgument {
		t.Fatalf("outside browse = %v", err)
	}
	foundTyped := false
	for _, detail := range connectErr.Details() {
		value, err := detail.Value()
		if err == nil {
			if typed, ok := value.(*gulv1.DomainError); ok && typed.GetCode() == gulv1.ErrorCode_ERROR_CODE_WORKSPACE_SELECTION_UNAVAILABLE {
				foundTyped = true
			}
		}
	}
	if !foundTyped {
		t.Fatalf("missing typed outside-root rejection: %+v", connectErr.Details())
	}
	_, err = handler.RevalidateWorkspace(t.Context(), connect.NewRequest(&gulv1.RevalidateWorkspaceRequest{WorkspaceId: "missing"}))
	assertWorkspaceError(t, err, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_WORKSPACE_IDENTITY_MISMATCH, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT)
	for _, test := range []struct {
		blocker workspace.Blocker
		code    gulv1.ErrorCode
	}{
		{workspace.BlockerUninitialized, gulv1.ErrorCode_ERROR_CODE_WORKSPACE_NOT_PROVISIONED},
		{workspace.BlockerProfileMissing, gulv1.ErrorCode_ERROR_CODE_PROFILE_MISSING},
		{workspace.BlockerProfileServerUnavailable, gulv1.ErrorCode_ERROR_CODE_PROFILE_SERVER_UNAVAILABLE},
		{workspace.BlockerUnknown, gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED},
	} {
		handler.Workspaces = workspace.NewService(blockedProvider{test.blocker}, store.Presentation(), nil, []string{root})
		_, err := handler.RegisterFromAllowlistPath(t.Context(), request)
		assertWorkspaceError(t, err, connect.CodeFailedPrecondition, test.code, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	}
	handler.Workspaces = workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, store.Presentation(), cancelledPicker{}, nil)
	cancelled, err := handler.RegisterFromHostSelection(t.Context(), connect.NewRequest(&gulv1.RegisterFromHostSelectionRequest{}))
	if err != nil || !cancelled.Msg.GetCancelled() || cancelled.Msg.GetWorkspace() != nil {
		t.Fatalf("cancelled host selection = %+v, %v", cancelled, err)
	}
	handler.Workspaces = workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, store.Presentation(), contextCanceledPicker{}, nil)
	_, err = handler.RegisterFromHostSelection(t.Context(), connect.NewRequest(&gulv1.RegisterFromHostSelectionRequest{}))
	if connect.CodeOf(err) != connect.CodeCanceled {
		t.Fatalf("request cancellation = %v", err)
	}
	handler.Workspaces = workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, store.Presentation(), deadlinePicker{}, nil)
	_, err = handler.RegisterFromHostSelection(t.Context(), connect.NewRequest(&gulv1.RegisterFromHostSelectionRequest{}))
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Fatalf("request deadline = %v", err)
	}
	handler.Principal = func(context.Context) (app.Principal, error) {
		return app.Principal{}, errors.New("resolver unavailable")
	}
	_, err = handler.ListWorkspaces(t.Context(), connect.NewRequest(&gulv1.ListWorkspacesRequest{}))
	assertWorkspaceError(t, err, connect.CodeUnauthenticated, gulv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
		gulv1.ActionClass_ACTION_CLASS_ABORT)
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	deniedCore := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: deny{}})
	if err := deniedCore.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer deniedCore.Stop(context.Background())
	handler.Core = deniedCore
	_, err = handler.ListWorkspaces(t.Context(), connect.NewRequest(&gulv1.ListWorkspacesRequest{}))
	assertWorkspaceError(t, err, connect.CodePermissionDenied, gulv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
		gulv1.ActionClass_ACTION_CLASS_ABORT)
	handler.Core = core
	for _, test := range []struct {
		name string
		core *app.Core
		code gulv1.ErrorCode
	}{

		{"persistence", app.NewCore(app.Dependencies{Provider: ready{}, Persistence: unavailable{}, Authorization: allow{}}), gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE},
	} {
		t.Run(test.name+" access outage", func(t *testing.T) {
			if err := test.core.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			defer test.core.Stop(context.Background())
			handler.Core = test.core
			_, err := handler.ListWorkspaces(t.Context(), connect.NewRequest(&gulv1.ListWorkspacesRequest{}))
			assertWorkspaceError(t, err, connect.CodeUnavailable, test.code, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
		})
	}
	handler.Core = core
	handler.Workspaces = workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, store.Presentation(), nil, []string{filepath.Join(root, "missing")})
	_, err = handler.ListRegistrableRoots(t.Context(), connect.NewRequest(&gulv1.ListRegistrableRootsRequest{}))
	assertWorkspaceError(t, err, connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_WORKSPACE_SELECTION_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	handler.Workspaces = workspace.NewService(contractprovider.ContractProvider{Port: scenario.New(time.Now())}, store.Presentation(), nil, nil)
	_, err = handler.RegisterFromHostSelection(t.Context(), connect.NewRequest(&gulv1.RegisterFromHostSelectionRequest{}))
	assertWorkspaceError(t, err, connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_WORKSPACE_SELECTION_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = handler.ListWorkspaces(t.Context(), connect.NewRequest(&gulv1.ListWorkspacesRequest{}))
	assertWorkspaceError(t, err, connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
}

type blockedProvider struct{ blocker workspace.Blocker }

type cancelledPicker struct{}

type contextCanceledPicker struct{}

type deadlinePicker struct{}

func (cancelledPicker) PickDirectory(context.Context) (string, error) {
	return "", workspace.ErrSelectionCancelled
}

func (contextCanceledPicker) PickDirectory(context.Context) (string, error) {
	return "", context.Canceled
}

func (deadlinePicker) PickDirectory(context.Context) (string, error) {
	return "", context.DeadlineExceeded
}

func (p blockedProvider) InspectWorkspace(context.Context, string, *string) (workspace.Inspection, error) {
	return workspace.Inspection{Blocker: p.blocker}, nil
}

func TestUnconfiguredHandlersReturnTypedAccessErrors(t *testing.T) {
	_, workspaceErr := (*WorkspaceHandler)(nil).ListWorkspaces(t.Context(), connect.NewRequest(&gulv1.ListWorkspacesRequest{}))
	_, localErr := (*WorkspaceHandler)(nil).RenameWorkspace(t.Context(), connect.NewRequest(&gulv1.RenameWorkspaceRequest{}))
	_, sessionErr := (*DirectPresentationHandler)(nil).GetDirectSessionPresentation(t.Context(), connect.NewRequest(&gulv1.GetDirectSessionPresentationRequest{}))
	_, runtimeErr := (*RuntimeHandler)(nil).ListRuntimeProfiles(t.Context(), connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{}))
	for name, err := range map[string]error{
		"workspace": workspaceErr, "local workspace": localErr, "session": sessionErr, "runtime": runtimeErr,
	} {
		t.Run(name, func(t *testing.T) {
			assertWorkspaceError(t, err, connect.CodeUnauthenticated, gulv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
				gulv1.ActionClass_ACTION_CLASS_ABORT)
		})
	}
}

func assertWorkspaceError(t *testing.T, err error, code connect.Code, domainCode gulv1.ErrorCode, action gulv1.ActionClass) {
	t.Helper()
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != code {
		t.Fatalf("connect error = %v; want %s", err, code)
	}
	for _, detail := range connectErr.Details() {
		value, decodeErr := detail.Value()
		if decodeErr == nil {
			if typed, ok := value.(*gulv1.DomainError); ok && typed.GetCode() == domainCode && typed.GetAction() == action {
				return
			}
		}
	}
	t.Fatalf("missing typed error %s/%s in %+v", domainCode, action, connectErr.Details())
}
