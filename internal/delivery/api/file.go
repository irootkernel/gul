package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/files"
)

// FileHandler is explicitly composed by a trusted host. E8 owns production
// authentication and listener registration.
type FileHandler struct {
	Core      *app.Core
	Files     *files.Service
	Principal PrincipalResolver
}

var _ gulv1connect.FileServiceHandler = (*FileHandler)(nil)

func (h *FileHandler) InspectPath(ctx context.Context, request *connect.Request[gulv1.InspectPathRequest]) (*connect.Response[gulv1.InspectPathResponse], error) {
	if h == nil || h.Core == nil || h.Files == nil || h.Principal == nil {
		return nil, accessError(connect.CodeUnauthenticated, "product access unavailable")
	}
	principal, err := h.Principal(ctx)
	if err != nil || principal.Subject == "" {
		return nil, accessError(connect.CodeUnauthenticated, "authentication required")
	}
	if err := h.Core.RequireLocalAccess(ctx, principal); err != nil {
		if errors.Is(err, app.ErrAccessDenied) {
			return nil, accessError(connect.CodePermissionDenied, "product access denied")
		}
		return nil, accessError(connect.CodeUnavailable, "file service unavailable")
	}
	node, err := h.Files.Inspect(ctx, principal.Subject, request.Msg.GetWorkspaceId(), request.Msg.GetRelativePath())
	if err != nil {
		return nil, fileError(err)
	}
	result := &gulv1.InspectPathResponse{ByteLength: uint64(node.Size)}
	if node.Directory {
		result.Kind = gulv1.FileNodeKind_FILE_NODE_KIND_DIRECTORY
	} else {
		result.Kind = gulv1.FileNodeKind_FILE_NODE_KIND_REGULAR_FILE
	}
	return connect.NewResponse(result), nil
}

func fileError(err error) error {
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, errors.New("file request cancelled"))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("file request timed out"))
	}
	code := gulv1.ErrorCode_ERROR_CODE_INVALID_REQUEST
	action := gulv1.ActionClass_ACTION_CLASS_FIX_REQUEST
	status := connect.CodeInvalidArgument
	message := "file unavailable"
	switch {
	case errors.Is(err, files.ErrUnsupportedPathEncoding):
		code = gulv1.ErrorCode_ERROR_CODE_UNSUPPORTED_PATH_ENCODING
	case errors.Is(err, files.ErrReattachRequired):
		code = gulv1.ErrorCode_ERROR_CODE_WORKSPACE_IDENTITY_MISMATCH
		action = gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT
		status = connect.CodeFailedPrecondition
		message = "workspace reattachment required"
	}
	result := connect.NewError(status, errors.New(message))
	if detail, detailErr := connect.NewErrorDetail(&gulv1.DomainError{Code: code, Action: action}); detailErr == nil {
		result.AddDetail(detail)
	}
	return result
}
