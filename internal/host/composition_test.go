package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"connectrpc.com/connect"
	"github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/workspace"
)

func TestOfflineCompositionThroughAuthenticatedTLSHost(t *testing.T) {
	c := config(t)
	h := started(t, c)
	httpClient := client(t, h)
	authClient := gulv1connect.NewAuthServiceClient(httpClient, h.Origin())
	attachment, err := Attach(t.Context(), c.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	password := []byte("offline composition password")
	setup := connect.NewRequest(&gulv1.FirstRunSetupRequest{Password: password})
	setup.Header().Set("Origin", h.Origin())
	setup.Header().Set(api.BrowserHeader, "1")
	setup.Header().Set(api.SetupHeader, attachment.SetupCredential)
	if _, err := authClient.FirstRunSetup(t.Context(), setup); err != nil {
		t.Fatal(err)
	}
	login := connect.NewRequest(&gulv1.LoginRequest{Password: password})
	login.Header().Set("Origin", h.Origin())
	login.Header().Set(api.BrowserHeader, "1")
	loggedIn, err := authClient.Login(t.Context(), login)
	if err != nil {
		t.Fatal(err)
	}
	cookie := strings.Split(loggedIn.Header().Get("Set-Cookie"), ";")[0]
	csrf := loggedIn.Msg.Session.CsrfToken
	account, err := h.store.Auth().PasswordAccount(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	withSession := func(headers connect.AnyRequest) {
		headers.Header().Set("Origin", h.Origin())
		headers.Header().Set(api.BrowserHeader, "1")
		headers.Header().Set(api.CSRFHeader, csrf)
		headers.Header().Set("Cookie", cookie)
	}

	workspaceClient := gulv1connect.NewWorkspacePresentationServiceClient(httpClient, h.Origin())
	unauthenticated := connect.NewRequest(&gulv1.GetNavigationRequest{})
	unauthenticated.Header().Set("Origin", h.Origin())
	unauthenticated.Header().Set(api.BrowserHeader, "1")
	if _, err := workspaceClient.GetNavigation(t.Context(), unauthenticated); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unprotected navigation: %v", err)
	}
	get := connect.NewRequest(&gulv1.GetNavigationRequest{})
	withSession(get)
	if result, err := workspaceClient.GetNavigation(t.Context(), get); err != nil || result.Msg.WorkspaceId != "" {
		t.Fatalf("offline navigation: %v, %v", result, err)
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("offline file"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := info.Sys().(*syscall.Stat_t)
	if err := h.store.Presentation().CreateAttachment(t.Context(), workspace.Attachment{
		SubjectID: account.SubjectID, ID: "saved", CanonicalRoot: root, ProviderID: "provider-workspace",
		FileDevice: fmt.Sprint(identity.Dev), FileInode: fmt.Sprint(identity.Ino), DisplayName: "Saved",
	}); err != nil {
		t.Fatal(err)
	}
	set := connect.NewRequest(&gulv1.SetNavigationRequest{WorkspaceId: "saved"})
	withSession(set)
	if result, err := workspaceClient.SetNavigation(t.Context(), set); err != nil || result.Msg.WorkspaceId != "saved" {
		t.Fatalf("saved navigation: %v, %v", result, err)
	}
	get = connect.NewRequest(&gulv1.GetNavigationRequest{})
	withSession(get)
	if result, err := workspaceClient.GetNavigation(t.Context(), get); err != nil || result.Msg.WorkspaceId != "saved" {
		t.Fatalf("restored navigation: %v, %v", result, err)
	}
	fileClient := gulv1connect.NewFileServiceClient(httpClient, h.Origin())
	inspect := connect.NewRequest(&gulv1.InspectPathRequest{WorkspaceId: "saved", RelativePath: "note.txt"})
	withSession(inspect)
	if result, err := fileClient.InspectPath(t.Context(), inspect); err != nil || result.Msg.ByteLength != uint64(len("offline file")) {
		t.Fatalf("saved file: %v, %v", result, err)
	}
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	private := connect.NewRequest(&gulv1.InspectPathRequest{WorkspaceId: "saved", RelativePath: ".dolgorae"})
	withSession(private)
	if _, err := fileClient.InspectPath(t.Context(), private); err == nil {
		t.Fatal("provider private path was exposed")
	}
	directClient := gulv1connect.NewDirectSessionServiceClient(httpClient, h.Origin())
	direct := connect.NewRequest(&gulv1.ListDirectSessionsRequest{WorkspaceId: "saved"})
	withSession(direct)
	if result, err := directClient.ListDirectSessions(t.Context(), direct); err != nil || len(result.Msg.Sessions) != 0 {
		t.Fatalf("offline direct sessions: %v, %v", result, err)
	}
	actionClient := gulv1connect.NewWriterActionServiceClient(httpClient, h.Origin())
	state := connect.NewRequest(&gulv1.GetActionStateRequest{SessionId: "missing", WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ, CloseIntent: gulv1.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE})
	withSession(state)
	if result, err := actionClient.GetActionState(t.Context(), state); err != nil || result.Msg.State.Flags.CanSubmitRead || result.Msg.State.Flags.CanAcquireWriter {
		t.Fatalf("offline actions: %v, %v", result, err)
	}

	artifactClient := gulv1connect.NewArtifactPresentationServiceClient(httpClient, h.Origin())
	artifact := connect.NewRequest(&gulv1.GetMetadataRequest{SessionId: "missing", ArtifactRef: "missing"})
	withSession(artifact)
	if _, err := artifactClient.GetMetadata(t.Context(), artifact); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("offline artifact handler: %v", err)
	}
	interactionClient := gulv1connect.NewInteractionPresentationServiceClient(httpClient, h.Origin())
	pending := connect.NewRequest(&gulv1.ListPendingRequest{SessionId: "missing"})
	withSession(pending)
	if _, err := interactionClient.ListPending(t.Context(), pending); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("offline interaction handler: %v", err)
	}
	eventClient := gulv1connect.NewClientEventServiceClient(httpClient, h.Origin())
	watch := connect.NewRequest(&gulv1.WatchClientEventsRequest{SessionId: "missing"})
	withSession(watch)
	stream, err := eventClient.WatchClientEvents(t.Context(), watch)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if stream.Receive() || connect.CodeOf(stream.Err()) != connect.CodeNotFound {
		t.Fatalf("offline event handler: %v", stream.Err())
	}

	runtimeClient := gulv1connect.NewRuntimeServiceClient(httpClient, h.Origin())
	profiles := connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{})
	withSession(profiles)
	if _, err := runtimeClient.ListRuntimeProfiles(t.Context(), profiles); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("missing runtime: %v", err)
	}
	list := connect.NewRequest(&gulv1.ListWorkspacesRequest{})
	withSession(list)
	if _, err := workspaceClient.ListWorkspaces(t.Context(), list); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("provider workspace listing: %v", err)
	}
}
