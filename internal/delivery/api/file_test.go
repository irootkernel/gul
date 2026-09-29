package api

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/files"
	"github.com/rootkernel/gul/internal/workspace"
)

type fileAttachments struct{ entry workspace.Attachment }

func (s fileAttachments) Attachment(_ context.Context, subject, id string) (workspace.Attachment, error) {
	if subject != s.entry.SubjectID || id != s.entry.ID {
		return workspace.Attachment{}, workspace.ErrAttachmentNotFound
	}
	return s.entry, nil
}

func TestFileWatchStreamRechecksPrincipalOnInvalidation(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "public.txt")
	if err := os.WriteFile(name, []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := info.Sys().(*syscall.Stat_t)
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := workspace.Attachment{SubjectID: "owner", ID: "entry", ProviderID: "provider-workspace", CanonicalRoot: canonical,
		FileDevice: fmt.Sprint(identity.Dev), FileInode: fmt.Sprint(identity.Ino)}
	core := app.NewCore(app.Dependencies{Provider: &unavailableCounter{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	var changed atomic.Bool
	handler := &FileHandler{Core: core, Files: files.NewService(fileAttachments{entry}), Principal: func(context.Context) (app.Principal, error) {
		if changed.Load() {
			return app.Principal{Subject: "other"}, nil
		}
		return app.Principal{Subject: "owner"}, nil
	}}
	_, route := gulv1connect.NewFileServiceHandler(handler)
	server := httptest.NewServer(route)
	defer server.Close()
	client := gulv1connect.NewFileServiceClient(server.Client(), server.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	writerCtx, stopWriter := context.WithCancel(ctx)
	defer stopWriter()
	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-writerCtx.Done():
				return
			case <-ticker.C:
				_ = os.WriteFile(name, []byte(fmt.Sprint(i)), 0600)
			}
		}
	}()
	stream, err := client.WatchFileChanges(ctx, connect.NewRequest(&gulv1.WatchFileChangesRequest{WorkspaceId: "entry"}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if !stream.Receive() || stream.Msg().GetRevision() == 0 {
		t.Fatalf("first invalidation = %v", stream.Err())
	}
	changed.Store(true)
	if stream.Receive() || connect.CodeOf(stream.Err()) != connect.CodePermissionDenied {
		t.Fatalf("changed principal stream = %v", stream.Err())
	}
}

func TestFileHandlerUsesTrustedSubjectAndHidesPrivatePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "public.txt"), []byte("public"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".dolgorae", "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := info.Sys().(*syscall.Stat_t)
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := workspace.Attachment{SubjectID: "owner", ID: "entry", ProviderID: "provider-workspace", CanonicalRoot: canonical,
		FileDevice: fmt.Sprint(identity.Dev), FileInode: fmt.Sprint(identity.Ino)}
	provider := &unavailableCounter{}
	core := app.NewCore(app.Dependencies{Provider: provider, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	handler := &FileHandler{Core: core, Files: files.NewService(fileAttachments{entry})}
	request := connect.NewRequest(&gulv1.InspectPathRequest{WorkspaceId: "entry", RelativePath: "public.txt"})
	if _, err := handler.InspectPath(t.Context(), request); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("untrusted read = %v", err)
	}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	result, err := handler.InspectPath(t.Context(), request)
	if err != nil || result.Msg.GetKind() != gulv1.FileNodeKind_FILE_NODE_KIND_REGULAR_FILE || result.Msg.GetByteLength() != 6 {
		t.Fatalf("public node = %+v, %v", result, err)
	}
	if provider.calls != 0 {
		t.Fatalf("file inspection called the offline provider %d times", provider.calls)
	}
	page, err := handler.ListDirectory(t.Context(), connect.NewRequest(&gulv1.ListDirectoryRequest{WorkspaceId: "entry", RelativePath: ".", PageSize: 10}))
	if err != nil || len(page.Msg.GetEntries()) != 2 {
		t.Fatalf("directory = %+v, %v", page, err)
	}
	preview, err := handler.ReadPreview(t.Context(), connect.NewRequest(&gulv1.ReadPreviewRequest{WorkspaceId: "entry", RelativePath: "public.txt"}))
	if err != nil || preview.Msg.GetKind() != gulv1.FilePreviewKind_FILE_PREVIEW_KIND_TEXT || preview.Msg.GetText() != "public" {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	if provider.calls != 0 {
		t.Fatalf("file preview called offline provider %d times", provider.calls)
	}
	gitStatus, err := handler.GetGitStatus(t.Context(), connect.NewRequest(&gulv1.GetGitStatusRequest{WorkspaceId: "entry", RelativePath: "public.txt"}))
	if err != nil || gitStatus.Msg.GetState() != gulv1.FileGitState_FILE_GIT_STATE_NOT_REPOSITORY {
		t.Fatalf("non-Git status = %+v, %v", gitStatus, err)
	}
	comparison, err := handler.CompareFixedRevisions(t.Context(), connect.NewRequest(&gulv1.CompareFixedRevisionsRequest{WorkspaceId: "entry", RelativePath: "public.txt"}))
	if err != nil || comparison.Msg.GetState() != gulv1.FileGitState_FILE_GIT_STATE_NOT_REPOSITORY || comparison.Msg.GetWorking().GetText() != "public" {
		t.Fatalf("non-Git compare = %+v, %v", comparison, err)
	}
	refresh, err := handler.RefreshFiles(t.Context(), connect.NewRequest(&gulv1.RefreshFilesRequest{WorkspaceId: "entry"}))
	if err != nil || refresh.Msg.GetRevision() != 1 {
		t.Fatalf("offline refresh = %+v, %v", refresh, err)
	}
	page, err = handler.ListDirectory(t.Context(), connect.NewRequest(&gulv1.ListDirectoryRequest{WorkspaceId: "entry", RelativePath: ".", PageSize: 1}))
	if err != nil || page.Msg.GetNextPageToken() == "" {
		t.Fatalf("paged directory = %+v, %v", page, err)
	}
	_, err = handler.ListDirectory(t.Context(), connect.NewRequest(&gulv1.ListDirectoryRequest{WorkspaceId: "entry", RelativePath: ".", PageSize: 1, PageToken: "bad"}))
	assertFileCode(t, err, connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN)
	_, err = handler.RefreshFiles(t.Context(), connect.NewRequest(&gulv1.RefreshFilesRequest{WorkspaceId: "entry"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.ListDirectory(t.Context(), connect.NewRequest(&gulv1.ListDirectoryRequest{WorkspaceId: "entry", RelativePath: ".", PageSize: 1, PageToken: page.Msg.GetNextPageToken()}))
	assertFileCode(t, err, connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_PAGE_TOKEN_EXPIRED)
	if provider.calls != 0 {
		t.Fatalf("file review or refresh called offline provider %d times", provider.calls)
	}
	_, err = handler.ReadPreview(t.Context(), connect.NewRequest(&gulv1.ReadPreviewRequest{WorkspaceId: "entry", RelativePath: ".dolgorae/secret"}))
	assertFileCode(t, err, connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_INVALID_REQUEST)
	for _, relative := range []string{".dolgorae/secret", "../outside", "missing"} {
		_, err := handler.InspectPath(t.Context(), connect.NewRequest(&gulv1.InspectPathRequest{WorkspaceId: "entry", RelativePath: relative}))
		assertFileCode(t, err, connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_INVALID_REQUEST)
	}
	_, err = handler.InspectPath(t.Context(), connect.NewRequest(&gulv1.InspectPathRequest{WorkspaceId: "entry", RelativePath: string([]byte{'x', 0xff})}))
	assertFileCode(t, err, connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_UNSUPPORTED_PATH_ENCODING)
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "other"}, nil }
	_, err = handler.InspectPath(t.Context(), request)
	assertFileCode(t, err, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_WORKSPACE_IDENTITY_MISMATCH)
}

func assertFileCode(t *testing.T, err error, status connect.Code, code gulv1.ErrorCode) {
	t.Helper()
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != status {
		t.Fatalf("file error = %v, want %v", err, status)
	}
	for _, detail := range connectErr.Details() {
		value, readErr := detail.Value()
		if readErr == nil {
			if typed, ok := value.(*gulv1.DomainError); ok && typed.GetCode() == code {
				return
			}
		}
	}
	t.Fatalf("file error lacks %v: %v", code, err)
}
