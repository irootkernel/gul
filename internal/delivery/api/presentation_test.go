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
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type unavailableCounter struct{ calls int }

func (p *unavailableCounter) Ready(context.Context) error {
	p.calls++
	return errors.New("provider offline")
}

func TestPresentationHandlersUseOnlyLocalState(t *testing.T) {
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
	if err := store.Presentation().CreateAttachment(t.Context(), workspace.Attachment{
		SubjectID: "owner", ID: "workspace", CanonicalRoot: filepath.Join(t.TempDir(), "workspace"),
		ProviderID: "provider-id", FileDevice: "1", FileInode: "2", DisplayName: "Original",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Presentation().InsertDirectSessionIfAbsent(t.Context(), storage.DirectSessionPresentation{
		SubjectID: "owner", WorkspaceID: "workspace", SessionID: "session", DisplayName: "Original session",
	}); err != nil {
		t.Fatal(err)
	}
	provider := &unavailableCounter{}
	core := app.NewCore(app.Dependencies{Provider: provider, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	service := presentation.NewService(store.Presentation())
	principal := func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	workspaceHandler := &WorkspaceHandler{Core: core, Presentation: service, Principal: principal}
	directHandler := &DirectPresentationHandler{Core: core, Presentation: service, Principal: principal}
	renamed, err := workspaceHandler.RenameWorkspace(t.Context(), connect.NewRequest(&gulv1.RenameWorkspaceRequest{WorkspaceId: "workspace", DisplayName: "Custom"}))
	if err != nil || renamed.Msg.GetWorkspace().GetDisplayName() != "Custom" {
		t.Fatalf("rename workspace = %+v, %v", renamed, err)
	}
	favorite, err := workspaceHandler.SetWorkspaceFavorite(t.Context(), connect.NewRequest(&gulv1.SetWorkspaceFavoriteRequest{WorkspaceId: "workspace", Favorite: true}))
	if err != nil || !favorite.Msg.GetWorkspace().GetFavorite() {
		t.Fatalf("favorite workspace = %+v, %v", favorite, err)
	}
	direct, err := directHandler.RenameDirectSession(t.Context(), connect.NewRequest(&gulv1.RenameDirectSessionRequest{SessionId: "session", DisplayName: "Custom session"}))
	if err != nil || direct.Msg.GetSession().GetDisplayName() != "Custom session" {
		t.Fatalf("rename session = %+v, %v", direct, err)
	}
	directFavorite, err := directHandler.SetDirectSessionFavorite(t.Context(), connect.NewRequest(&gulv1.SetDirectSessionFavoriteRequest{SessionId: "session", Favorite: true}))
	if err != nil || !directFavorite.Msg.GetSession().GetFavorite() || directFavorite.Msg.GetSession().GetArchived() {
		t.Fatalf("favorite session = %+v, %v", directFavorite, err)
	}
	if _, err := workspaceHandler.SetNavigation(t.Context(), connect.NewRequest(&gulv1.SetNavigationRequest{WorkspaceId: "workspace", SessionId: "session"})); err != nil {
		t.Fatal(err)
	}
	directArchived, err := directHandler.SetDirectSessionArchived(t.Context(), connect.NewRequest(&gulv1.SetDirectSessionArchivedRequest{SessionId: "session", Archived: true}))
	if err != nil || !directArchived.Msg.GetSession().GetFavorite() || !directArchived.Msg.GetSession().GetArchived() {
		t.Fatalf("archived session = %+v, %v", directArchived, err)
	}
	directRead, err := directHandler.GetDirectSessionPresentation(t.Context(), connect.NewRequest(&gulv1.GetDirectSessionPresentationRequest{SessionId: "session"}))
	if err != nil || !directRead.Msg.GetSession().GetFavorite() || !directRead.Msg.GetSession().GetArchived() {
		t.Fatalf("read session = %+v, %v", directRead, err)
	}
	navigation, err := workspaceHandler.GetNavigation(t.Context(), connect.NewRequest(&gulv1.GetNavigationRequest{}))
	if err != nil || navigation.Msg.GetWorkspaceId() != "workspace" || navigation.Msg.GetSessionId() != "" {
		t.Fatalf("navigation = %+v, %v", navigation, err)
	}
	if provider.calls != 0 {
		t.Fatalf("local presentation called provider readiness %d times", provider.calls)
	}
	if _, err := workspaceHandler.RenameWorkspace(t.Context(), connect.NewRequest(&gulv1.RenameWorkspaceRequest{WorkspaceId: "missing", DisplayName: "X"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing workspace = %v", err)
	}
	if _, err := directHandler.RenameDirectSession(t.Context(), connect.NewRequest(&gulv1.RenameDirectSessionRequest{SessionId: "session", DisplayName: ""})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid name = %v", err)
	}
	directHandler.Principal = nil
	_, err = directHandler.GetDirectSessionPresentation(t.Context(), connect.NewRequest(&gulv1.GetDirectSessionPresentationRequest{SessionId: "session"}))
	assertWorkspaceError(t, err, connect.CodeUnauthenticated, gulv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
		gulv1.ActionClass_ACTION_CLASS_ABORT)
	workspaceHandler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "other"}, nil }
	if _, err := workspaceHandler.SetWorkspaceHidden(t.Context(), connect.NewRequest(&gulv1.SetWorkspaceHiddenRequest{WorkspaceId: "workspace", Hidden: true})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("foreign workspace = %v", err)
	}
	workspaceHandler.Principal = principal
	deniedCore := app.NewCore(app.Dependencies{Provider: provider, Persistence: ready{}, Authorization: deny{}})
	if err := deniedCore.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	workspaceHandler.Core = deniedCore
	_, err = workspaceHandler.SetWorkspaceFavorite(t.Context(), connect.NewRequest(&gulv1.SetWorkspaceFavoriteRequest{WorkspaceId: "workspace", Favorite: false}))
	assertWorkspaceError(t, err, connect.CodePermissionDenied, gulv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
		gulv1.ActionClass_ACTION_CLASS_ABORT)
	if err := deniedCore.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	workspaceHandler.Core = core
	persistenceCore := app.NewCore(app.Dependencies{Provider: provider, Persistence: unavailable{}, Authorization: allow{}})
	if err := persistenceCore.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	workspaceHandler.Core = persistenceCore
	_, err = workspaceHandler.SetWorkspaceFavorite(t.Context(), connect.NewRequest(&gulv1.SetWorkspaceFavoriteRequest{WorkspaceId: "workspace", Favorite: false}))
	assertWorkspaceError(t, err, connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	if err := persistenceCore.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	workspaceHandler.Core = core
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = workspaceHandler.RenameWorkspace(cancelled, connect.NewRequest(&gulv1.RenameWorkspaceRequest{WorkspaceId: "workspace", DisplayName: "Cancelled"}))
	if connect.CodeOf(err) != connect.CodeCanceled {
		t.Fatalf("cancelled local write = %v", err)
	}
	expired, deadlineCancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	_, err = workspaceHandler.RenameWorkspace(expired, connect.NewRequest(&gulv1.RenameWorkspaceRequest{WorkspaceId: "workspace", DisplayName: "Expired"}))
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Fatalf("expired local write = %v", err)
	}
	hidden, err := workspaceHandler.SetWorkspaceHidden(t.Context(), connect.NewRequest(&gulv1.SetWorkspaceHiddenRequest{WorkspaceId: "workspace", Hidden: true}))
	if err != nil || !hidden.Msg.GetWorkspace().GetHidden() {
		t.Fatalf("hidden workspace = %+v, %v", hidden, err)
	}
	if err := core.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = workspaceHandler.RenameWorkspace(t.Context(), connect.NewRequest(&gulv1.RenameWorkspaceRequest{WorkspaceId: "workspace", DisplayName: "Stopped"}))
	assertWorkspaceError(t, err, connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	workspaceHandler.Workspaces = workspace.NewService(nil, store.Presentation(), nil, nil)
	_, err = workspaceHandler.ListWorkspaces(t.Context(), connect.NewRequest(&gulv1.ListWorkspacesRequest{}))
	assertWorkspaceError(t, err, connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
}
