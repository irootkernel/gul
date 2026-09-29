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
	principal, err := h.filePrincipal(ctx)
	if err != nil {
		return nil, err
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

func (h *FileHandler) filePrincipal(ctx context.Context) (app.Principal, error) {
	if h == nil || h.Core == nil || h.Files == nil || h.Principal == nil {
		return app.Principal{}, accessError(connect.CodeUnauthenticated, "product access unavailable")
	}
	principal, err := h.Principal(ctx)
	if err != nil || principal.Subject == "" {
		return app.Principal{}, accessError(connect.CodeUnauthenticated, "authentication required")
	}
	if err := h.Core.RequireLocalAccess(ctx, principal); err != nil {
		if errors.Is(err, app.ErrAccessDenied) {
			return app.Principal{}, accessError(connect.CodePermissionDenied, "product access denied")
		}
		return app.Principal{}, accessError(connect.CodeUnavailable, "file service unavailable")
	}
	return principal, nil
}

func (h *FileHandler) ListDirectory(ctx context.Context, request *connect.Request[gulv1.ListDirectoryRequest]) (*connect.Response[gulv1.ListDirectoryResponse], error) {
	principal, err := h.filePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.Files.ListDirectory(ctx, principal.Subject, request.Msg.GetWorkspaceId(), request.Msg.GetRelativePath(), request.Msg.GetPageSize(), request.Msg.GetPageToken())
	if err != nil {
		return nil, fileError(err)
	}
	result := &gulv1.ListDirectoryResponse{NextPageToken: page.NextToken}
	for _, entry := range page.Entries {
		item := &gulv1.FileEntry{Name: entry.Name, ByteLength: uint64(entry.Size), ProviderManagedDenied: entry.ProviderManagedDenied}
		if entry.Directory {
			item.Kind = gulv1.FileNodeKind_FILE_NODE_KIND_DIRECTORY
		} else if !entry.ProviderManagedDenied {
			item.Kind = gulv1.FileNodeKind_FILE_NODE_KIND_REGULAR_FILE
		}
		result.Entries = append(result.Entries, item)
	}
	return connect.NewResponse(result), nil
}

func (h *FileHandler) ReadPreview(ctx context.Context, request *connect.Request[gulv1.ReadPreviewRequest]) (*connect.Response[gulv1.ReadPreviewResponse], error) {
	principal, err := h.filePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	preview, err := h.Files.ReadPreview(ctx, principal.Subject, request.Msg.GetWorkspaceId(), request.Msg.GetRelativePath())
	if err != nil {
		return nil, fileError(err)
	}
	result := &gulv1.ReadPreviewResponse{Text: preview.Text, Image: preview.Image, MimeType: preview.MIME, Language: preview.Language, Truncated: preview.Truncated}
	switch preview.Kind {
	case files.PreviewText:
		result.Kind = gulv1.FilePreviewKind_FILE_PREVIEW_KIND_TEXT
	case files.PreviewMarkdown:
		result.Kind = gulv1.FilePreviewKind_FILE_PREVIEW_KIND_MARKDOWN
	case files.PreviewRaster:
		result.Kind = gulv1.FilePreviewKind_FILE_PREVIEW_KIND_RASTER
	case files.PreviewSVGSource:
		result.Kind = gulv1.FilePreviewKind_FILE_PREVIEW_KIND_SVG_SOURCE
	default:
		result.Kind = gulv1.FilePreviewKind_FILE_PREVIEW_KIND_UNSUPPORTED
	}
	for _, asset := range preview.MarkdownImages {
		result.MarkdownImages = append(result.MarkdownImages, &gulv1.MarkdownImage{Reference: asset.Reference, MimeType: asset.MIME, Data: asset.Data})
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
