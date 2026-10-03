package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/interrupt"
	"github.com/rootkernel/gul/internal/launch"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/sessionclose"
	"github.com/rootkernel/gul/internal/submit"
)

func localAccess(ctx context.Context, core *app.Core, resolve PrincipalResolver) (string, error) {
	if core == nil || resolve == nil {
		return "", accessError(connect.CodeUnauthenticated, "product access unavailable")
	}
	principal, err := resolve(ctx)
	if err != nil || principal.Subject == "" {
		return "", accessError(connect.CodeUnauthenticated, "authentication required")
	}
	if err := core.RequireLocalAccess(ctx, principal); err != nil {
		if errors.Is(err, app.ErrAccessDenied) {
			return "", accessError(connect.CodePermissionDenied, "product access denied")
		}
		return "", presentationError(err)
	}
	return principal.Subject, nil
}

func (h *WorkspaceHandler) presentationAccess(ctx context.Context) (string, error) {
	if h == nil || h.Presentation == nil {
		return "", accessError(connect.CodeUnauthenticated, "product access unavailable")
	}
	return localAccess(ctx, h.Core, h.Principal)
}

func browserPresentation(entry presentation.Workspace) *gulv1.WorkspaceEntry {
	return &gulv1.WorkspaceEntry{WorkspaceId: entry.WorkspaceID, DisplayName: entry.DisplayName, Hidden: entry.Hidden, Favorite: entry.Favorite}
}

func (h *WorkspaceHandler) RenameWorkspace(ctx context.Context, request *connect.Request[gulv1.RenameWorkspaceRequest]) (*connect.Response[gulv1.WorkspacePresentationResponse], error) {
	subject, err := h.presentationAccess(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Presentation.RenameWorkspace(ctx, subject, request.Msg.GetWorkspaceId(), request.Msg.GetDisplayName())
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.WorkspacePresentationResponse{Workspace: browserPresentation(entry)}), nil
}

func (h *WorkspaceHandler) SetWorkspaceFavorite(ctx context.Context, request *connect.Request[gulv1.SetWorkspaceFavoriteRequest]) (*connect.Response[gulv1.WorkspacePresentationResponse], error) {
	subject, err := h.presentationAccess(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Presentation.SetWorkspaceFavorite(ctx, subject, request.Msg.GetWorkspaceId(), request.Msg.GetFavorite())
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.WorkspacePresentationResponse{Workspace: browserPresentation(entry)}), nil
}

func (h *WorkspaceHandler) SetWorkspaceHidden(ctx context.Context, request *connect.Request[gulv1.SetWorkspaceHiddenRequest]) (*connect.Response[gulv1.WorkspacePresentationResponse], error) {
	subject, err := h.presentationAccess(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Presentation.SetWorkspaceHidden(ctx, subject, request.Msg.GetWorkspaceId(), request.Msg.GetHidden())
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.WorkspacePresentationResponse{Workspace: browserPresentation(entry)}), nil
}

func (h *WorkspaceHandler) RemoveWorkspaceEntry(ctx context.Context, request *connect.Request[gulv1.RemoveWorkspaceEntryRequest]) (*connect.Response[gulv1.RemoveWorkspaceEntryResponse], error) {
	subject, err := h.presentationAccess(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.Presentation.RemoveWorkspace(ctx, subject, request.Msg.GetWorkspaceId()); err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.RemoveWorkspaceEntryResponse{}), nil
}

func (h *WorkspaceHandler) GetNavigation(ctx context.Context, _ *connect.Request[gulv1.GetNavigationRequest]) (*connect.Response[gulv1.NavigationResponse], error) {
	subject, err := h.presentationAccess(ctx)
	if err != nil {
		return nil, err
	}
	value, err := h.Presentation.Navigation(ctx, subject)
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.NavigationResponse{WorkspaceId: value.WorkspaceID, SessionId: value.SessionID}), nil
}

func (h *WorkspaceHandler) SetNavigation(ctx context.Context, request *connect.Request[gulv1.SetNavigationRequest]) (*connect.Response[gulv1.NavigationResponse], error) {
	subject, err := h.presentationAccess(ctx)
	if err != nil {
		return nil, err
	}
	value, err := h.Presentation.SetNavigation(ctx, subject, presentation.Navigation{WorkspaceID: request.Msg.GetWorkspaceId(), SessionID: request.Msg.GetSessionId()})
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.NavigationResponse{WorkspaceId: value.WorkspaceID, SessionId: value.SessionID}), nil
}

// DirectPresentationHandler supplies shared authenticated session routes.
type DirectPresentationHandler struct {
	gulv1connect.UnimplementedDirectSessionServiceHandler
	Core         *app.Core
	Presentation *presentation.Service
	Sessions     *session.Service
	Close        *sessionclose.Service
	History      *history.Service
	Submissions  *submit.Service
	Interrupts   *interrupt.Service
	Creator      SessionCreator
	Principal    PrincipalResolver
}

type SessionCreator interface {
	PendingCreations(context.Context, string, string) ([]string, error)
	CreateSession(context.Context, string, string, string, launch.Choice) (session.CreationOutcome, error)
	RecoverCreation(context.Context, string, string, string) (session.CreationOutcome, error)
}

var _ gulv1connect.DirectSessionServiceHandler = (*DirectPresentationHandler)(nil)

func (h *DirectPresentationHandler) access(ctx context.Context) (string, error) {
	if h == nil || h.Presentation == nil {
		return "", accessError(connect.CodeUnauthenticated, "product access unavailable")
	}
	return localAccess(ctx, h.Core, h.Principal)
}

func browserDirect(entry presentation.DirectSession) *gulv1.DirectSessionPresentation {
	return &gulv1.DirectSessionPresentation{SessionId: entry.SessionID, WorkspaceId: entry.WorkspaceID, DisplayName: entry.DisplayName, Favorite: entry.Favorite, Archived: entry.Archived}
}

func (h *DirectPresentationHandler) GetDirectSessionPresentation(ctx context.Context, request *connect.Request[gulv1.GetDirectSessionPresentationRequest]) (*connect.Response[gulv1.DirectSessionPresentationResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Presentation.DirectSession(ctx, subject, request.Msg.GetSessionId())
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.DirectSessionPresentationResponse{Session: browserDirect(entry)}), nil
}

func (h *DirectPresentationHandler) RenameDirectSession(ctx context.Context, request *connect.Request[gulv1.RenameDirectSessionRequest]) (*connect.Response[gulv1.DirectSessionPresentationResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Presentation.RenameDirectSession(ctx, subject, request.Msg.GetSessionId(), request.Msg.GetDisplayName())
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.DirectSessionPresentationResponse{Session: browserDirect(entry)}), nil
}

func (h *DirectPresentationHandler) SetDirectSessionFavorite(ctx context.Context, request *connect.Request[gulv1.SetDirectSessionFavoriteRequest]) (*connect.Response[gulv1.DirectSessionPresentationResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Presentation.SetDirectSessionFavorite(ctx, subject, request.Msg.GetSessionId(), request.Msg.GetFavorite())
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.DirectSessionPresentationResponse{Session: browserDirect(entry)}), nil
}

func (h *DirectPresentationHandler) SetDirectSessionArchived(ctx context.Context, request *connect.Request[gulv1.SetDirectSessionArchivedRequest]) (*connect.Response[gulv1.DirectSessionPresentationResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.Presentation.SetDirectSessionArchived(ctx, subject, request.Msg.GetSessionId(), request.Msg.GetArchived())
	if err != nil {
		return nil, presentationError(err)
	}
	return connect.NewResponse(&gulv1.DirectSessionPresentationResponse{Session: browserDirect(entry)}), nil
}

func presentationError(err error) error {
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, errors.New("presentation request cancelled"))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("presentation request timed out"))
	}
	code := gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE
	action := gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR
	connectCode := connect.CodeUnavailable
	message := "presentation unavailable"
	switch {
	case errors.Is(err, app.ErrCoreNotRunning):
		code, message = gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, "presentation service unavailable"
	case errors.Is(err, presentation.ErrInvalid):
		code, action, connectCode, message = gulv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, gulv1.ActionClass_ACTION_CLASS_FIX_REQUEST, connect.CodeInvalidArgument, "invalid presentation request"
	case errors.Is(err, presentation.ErrNotFound):
		code, action, connectCode, message = gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT, connect.CodeNotFound, "presentation not found"
	}
	result := connect.NewError(connectCode, errors.New(message))
	if detail, detailErr := connect.NewErrorDetail(&gulv1.DomainError{Code: code, Action: action}); detailErr == nil {
		result.AddDetail(detail)
	}
	return result
}
