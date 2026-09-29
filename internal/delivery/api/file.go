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
	return connect.NewResponse(filePreviewProto(preview)), nil
}

func filePreviewProto(preview files.Preview) *gulv1.ReadPreviewResponse {
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
	return result
}

func (h *FileHandler) RefreshFiles(ctx context.Context, request *connect.Request[gulv1.RefreshFilesRequest]) (*connect.Response[gulv1.RefreshFilesResponse], error) {
	principal, err := h.filePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	revision, err := h.Files.Refresh(ctx, principal.Subject, request.Msg.GetWorkspaceId())
	if err != nil {
		return nil, fileError(err)
	}
	return connect.NewResponse(&gulv1.RefreshFilesResponse{Revision: revision}), nil
}

func (h *FileHandler) GetGitStatus(ctx context.Context, request *connect.Request[gulv1.GetGitStatusRequest]) (*connect.Response[gulv1.GetGitStatusResponse], error) {
	principal, err := h.filePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	status, err := h.Files.GitStatus(ctx, principal.Subject, request.Msg.GetWorkspaceId(), request.Msg.GetRelativePath())
	if err != nil {
		return nil, fileError(err)
	}
	return connect.NewResponse(&gulv1.GetGitStatusResponse{State: fileGitState(status.State), Direct: fileChangeKind(status.Direct), Aggregate: fileChangeKind(status.Aggregate), Staged: status.Staged, Unstaged: status.Unstaged}), nil
}

func (h *FileHandler) CompareFixedRevisions(ctx context.Context, request *connect.Request[gulv1.CompareFixedRevisionsRequest]) (*connect.Response[gulv1.CompareFixedRevisionsResponse], error) {
	principal, err := h.filePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	comparison, err := h.Files.Compare(ctx, principal.Subject, request.Msg.GetWorkspaceId(), request.Msg.GetRelativePath())
	if err != nil {
		return nil, fileError(err)
	}
	result := &gulv1.CompareFixedRevisionsResponse{State: fileGitState(comparison.State), Change: fileChangeKind(comparison.Change), HeadMissing: comparison.HeadMissing, WorkingMissing: comparison.WorkingMissing, PreviousRelativePath: comparison.PreviousPath}
	if !comparison.HeadMissing && comparison.State == files.GitAvailable {
		result.Head = filePreviewProto(comparison.Head)
	}
	if !comparison.WorkingMissing {
		result.Working = filePreviewProto(comparison.Working)
	}
	return connect.NewResponse(result), nil
}

func (h *FileHandler) WatchFileChanges(ctx context.Context, request *connect.Request[gulv1.WatchFileChangesRequest], stream *connect.ServerStream[gulv1.FileChange]) error {
	principal, err := h.filePrincipal(ctx)
	if err != nil {
		return err
	}
	changes, err := h.Files.Watch(ctx, principal.Subject, request.Msg.GetWorkspaceId())
	if err != nil {
		return fileError(err)
	}
	for change := range changes {
		if change.Err != nil {
			return fileError(change.Err)
		}
		current, err := h.filePrincipal(ctx)
		if err != nil {
			return err
		}
		if current.Subject != principal.Subject {
			return accessError(connect.CodePermissionDenied, "file principal changed")
		}
		if err := stream.Send(&gulv1.FileChange{Revision: change.Revision, RelativePath: change.RelativePath}); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func fileGitState(state files.GitState) gulv1.FileGitState {
	switch state {
	case files.GitAvailable:
		return gulv1.FileGitState_FILE_GIT_STATE_AVAILABLE
	case files.GitNotRepository:
		return gulv1.FileGitState_FILE_GIT_STATE_NOT_REPOSITORY
	case files.GitUnbornHead:
		return gulv1.FileGitState_FILE_GIT_STATE_UNBORN_HEAD
	case files.GitLimitExceeded:
		return gulv1.FileGitState_FILE_GIT_STATE_LIMIT_EXCEEDED
	default:
		return gulv1.FileGitState_FILE_GIT_STATE_UNAVAILABLE
	}
}

func fileChangeKind(kind files.ChangeKind) gulv1.FileChangeKind {
	switch kind {
	case files.ChangeClean:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_CLEAN
	case files.ChangeModified:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_MODIFIED
	case files.ChangeAdded:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_ADDED
	case files.ChangeUntracked:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_UNTRACKED
	case files.ChangeDeleted:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_DELETED
	case files.ChangeRenamed:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_RENAMED
	case files.ChangeConflicted:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_CONFLICTED
	case files.ChangeMixed:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_MIXED
	default:
		return gulv1.FileChangeKind_FILE_CHANGE_KIND_UNSPECIFIED
	}
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
	case errors.Is(err, files.ErrInvalidPageToken):
		code = gulv1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
		message = "invalid file page token"
	case errors.Is(err, files.ErrPageTokenExpired):
		code = gulv1.ErrorCode_ERROR_CODE_PAGE_TOKEN_EXPIRED
		action = gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT
		message = "file page token expired; restart listing"
	case errors.Is(err, files.ErrReattachRequired):
		code = gulv1.ErrorCode_ERROR_CODE_WORKSPACE_IDENTITY_MISMATCH
		action = gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT
		status = connect.CodeFailedPrecondition
		message = "workspace reattachment required"
	case errors.Is(err, files.ErrWatchLimit):
		code = gulv1.ErrorCode_ERROR_CODE_LIMIT_EXCEEDED
		status = connect.CodeResourceExhausted
		message = "file watcher limit reached"
	case errors.Is(err, files.ErrWatchUnavailable):
		code = gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE
		status = connect.CodeUnavailable
		message = "file watcher unavailable"
	}
	result := connect.NewError(status, errors.New(message))
	if detail, detailErr := connect.NewErrorDetail(&gulv1.DomainError{Code: code, Action: action}); detailErr == nil {
		result.AddDetail(detail)
	}
	return result
}
