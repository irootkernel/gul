package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/workspace"
)

// PrincipalResolver is supplied by the authenticated host, never a browser field.
type PrincipalResolver func(context.Context) (app.Principal, error)

// WorkspaceHandler is available for explicit composition. E8 owns production
// authentication and listener registration; neither is installed by this type.
type WorkspaceHandler struct {
	Core         *app.Core
	Workspaces   *workspace.Service
	Presentation *presentation.Service
	Principal    PrincipalResolver
}

var _ gulv1connect.WorkspacePresentationServiceHandler = (*WorkspaceHandler)(nil)

func (h *WorkspaceHandler) access(ctx context.Context) (string, error) {
	if h == nil || h.Core == nil || h.Workspaces == nil || h.Principal == nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("product access unavailable"))
	}
	principal, err := h.Principal(ctx)
	if err != nil || principal.Subject == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if err := h.Core.RequireProductAccess(ctx, principal); err != nil {
		if errors.Is(err, app.ErrAccessDenied) {
			return "", connect.NewError(connect.CodePermissionDenied, errors.New("product access denied"))
		}
		return "", workspaceError(err)
	}
	return principal.Subject, nil
}

func (h *WorkspaceHandler) ListRegistrableRoots(ctx context.Context, _ *connect.Request[gulv1.ListRegistrableRootsRequest]) (*connect.Response[gulv1.ListRegistrableRootsResponse], error) {
	if _, err := h.access(ctx); err != nil {
		return nil, err
	}
	if err := h.Workspaces.RootConfigurationError(); err != nil {
		return nil, workspaceError(err)
	}
	result := &gulv1.ListRegistrableRootsResponse{}
	for _, root := range h.Workspaces.ListRegistrableRoots() {
		result.Roots = append(result.Roots, &gulv1.RegistrableRoot{RootId: root.ID, Label: root.Label})
	}
	return connect.NewResponse(result), nil
}

func (h *WorkspaceHandler) BrowseRegistrableRoot(ctx context.Context, request *connect.Request[gulv1.BrowseRegistrableRootRequest]) (*connect.Response[gulv1.BrowseRegistrableRootResponse], error) {
	if _, err := h.access(ctx); err != nil {
		return nil, err
	}
	directories, err := h.Workspaces.BrowseRegistrableRoot(request.Msg.GetRootId(), request.Msg.GetRelativePath())
	if err != nil {
		return nil, workspaceError(err)
	}
	result := &gulv1.BrowseRegistrableRootResponse{}
	for _, directory := range directories {
		result.Directories = append(result.Directories, &gulv1.RegistrableDirectory{Name: directory.Name, RelativePath: directory.RelativePath})
	}
	return connect.NewResponse(result), nil
}

func (h *WorkspaceHandler) RegisterFromHostSelection(ctx context.Context, _ *connect.Request[gulv1.RegisterFromHostSelectionRequest]) (*connect.Response[gulv1.RegisterWorkspaceResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Workspaces.RegisterFromHostSelection(ctx, subject)
	if errors.Is(err, workspace.ErrSelectionCancelled) {
		return connect.NewResponse(&gulv1.RegisterWorkspaceResponse{Cancelled: true}), nil
	}
	if err != nil {
		return nil, workspaceError(err)
	}
	return connect.NewResponse(&gulv1.RegisterWorkspaceResponse{Workspace: browserWorkspace(entry)}), nil
}

func (h *WorkspaceHandler) RegisterFromAllowlistPath(ctx context.Context, request *connect.Request[gulv1.RegisterFromAllowlistPathRequest]) (*connect.Response[gulv1.RegisterWorkspaceResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Workspaces.RegisterFromAllowlistPath(ctx, subject, request.Msg.GetRootId(), request.Msg.GetRelativePath())
	if err != nil {
		return nil, workspaceError(err)
	}
	return connect.NewResponse(&gulv1.RegisterWorkspaceResponse{Workspace: browserWorkspace(entry)}), nil
}

func (h *WorkspaceHandler) RevalidateWorkspace(ctx context.Context, request *connect.Request[gulv1.RevalidateWorkspaceRequest]) (*connect.Response[gulv1.RevalidateWorkspaceResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Workspaces.Revalidate(ctx, subject, request.Msg.GetWorkspaceId())
	if err != nil {
		return nil, workspaceError(err)
	}
	return connect.NewResponse(&gulv1.RevalidateWorkspaceResponse{Workspace: browserWorkspace(entry)}), nil
}

func (h *WorkspaceHandler) ListWorkspaces(ctx context.Context, _ *connect.Request[gulv1.ListWorkspacesRequest]) (*connect.Response[gulv1.ListWorkspacesResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := h.Workspaces.List(ctx, subject)
	if err != nil {
		return nil, workspaceError(err)
	}
	result := &gulv1.ListWorkspacesResponse{}
	for _, entry := range entries {
		result.Workspaces = append(result.Workspaces, browserWorkspace(entry))
	}
	return connect.NewResponse(result), nil
}

func browserWorkspace(entry workspace.Attachment) *gulv1.WorkspaceEntry {
	return &gulv1.WorkspaceEntry{WorkspaceId: entry.ID, DisplayName: entry.DisplayName, Hidden: entry.Hidden, Favorite: entry.Favorite}
}

func workspaceError(err error) error {
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, errors.New("workspace request cancelled"))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("workspace request timed out"))
	}
	code := gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED
	action := gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR
	connectCode := connect.CodeFailedPrecondition
	message := "workspace provider blocked"
	switch {
	case errors.Is(err, workspace.ErrProviderUnavailable):
		code, action, connectCode = gulv1.ErrorCode_ERROR_CODE_TRANSPORT_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT, connect.CodeUnavailable
		message = "workspace provider unavailable"
	case errors.Is(err, workspace.ErrHostSelectionUnavailable):
		code, action, connectCode = gulv1.ErrorCode_ERROR_CODE_WORKSPACE_SELECTION_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR, connect.CodeUnavailable
		message = "host workspace selection unavailable"
	case errors.Is(err, workspace.ErrSelectionUnavailable):
		code, action, connectCode = gulv1.ErrorCode_ERROR_CODE_WORKSPACE_SELECTION_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_FIX_REQUEST, connect.CodeInvalidArgument
		message = "workspace selection unavailable"
	case errors.Is(err, workspace.ErrWorkspaceUninitialized):
		code = gulv1.ErrorCode_ERROR_CODE_WORKSPACE_NOT_PROVISIONED
		message = "workspace is not initialized"
	case errors.Is(err, workspace.ErrProfileMissing):
		code = gulv1.ErrorCode_ERROR_CODE_PROFILE_MISSING
		message = "workspace profile is unavailable"
	case errors.Is(err, workspace.ErrProfileServerUnavailable):
		code = gulv1.ErrorCode_ERROR_CODE_PROFILE_SERVER_UNAVAILABLE
		message = "profile server unavailable"
	case errors.Is(err, workspace.ErrIdentityMismatch), errors.Is(err, workspace.ErrReattachRequired):
		code = gulv1.ErrorCode_ERROR_CODE_WORKSPACE_IDENTITY_MISMATCH
		action = gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT
		message = "workspace reattachment required"
	case errors.Is(err, workspace.ErrPersistenceUnavailable), errors.Is(err, app.ErrPersistenceUnavailable):
		code = gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE
		connectCode = connect.CodeUnavailable
		message = "workspace storage unavailable"
	case errors.Is(err, app.ErrCoreNotRunning):
		code = gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE
		connectCode = connect.CodeUnavailable
		message = "workspace service unavailable"
	case errors.Is(err, app.ErrProviderUnavailable):
		connectCode = connect.CodeUnavailable
		message = "workspace service unavailable"
	}
	result := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&gulv1.DomainError{Code: code, Action: action})
	if detailErr == nil {
		result.AddDetail(detail)
	}
	return result
}
