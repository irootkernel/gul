package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	contract "github.com/rootkernel/gul/contract"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/gateway"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/proto"
)

func TestOfflineCompositionThroughAuthenticatedTLSHost(t *testing.T) {
	t.Run("missing", func(t *testing.T) { offlineHost(t, config(t), false) })
	t.Run("incompatible", func(t *testing.T) {
		c := config(t)
		if err := os.WriteFile(c.DolgoraeExecutable, []byte("unqualified executable"), 0700); err != nil {
			t.Fatal(err)
		}
		offlineHost(t, c, false)
	})
}

func TestPublishedProductionHostChoicesAndTransportLoss(t *testing.T) {
	binary := os.Getenv("GUL_E2_DOLGORAE_EXECUTABLE")
	if binary == "" {
		t.Skip("published artifact qualification is opt-in")
	}
	archive, err := os.ReadFile(os.Getenv("GUL_E2_DOLGORAE_ARCHIVE"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	if hex.EncodeToString(digest[:]) != contract.QualifiedRelease().Archive.SHA256 {
		t.Fatal("published archive identity")
	}
	c := publishedHostConfig(t, binary)
	offlineHost(t, c, true)
}

func publishedHostConfig(t *testing.T, binary string) Config {
	t.Helper()
	c := config(t)
	fixtureRoot, err := os.MkdirTemp("/private/tmp", "gul-host-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(fixtureRoot) })
	c.DolgoraeExecutable, err = copyPublishedHostExecutable(binary, filepath.Join(fixtureRoot, "dolgorae"))
	if err != nil {
		t.Fatal("published executable identity", err)
	}
	c.ProviderHome = filepath.Join(fixtureRoot, "home")
	work := filepath.Join(filepath.Dir(c.ProviderHome), "workspace")
	for _, path := range []string{c.ProviderHome, work} {
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	init := exec.CommandContext(t.Context(), c.DolgoraeExecutable, "init", work, "--non-git")
	init.Env, init.Dir, init.Stdout, init.Stderr = []string{"HOME=" + c.ProviderHome, "PATH=/usr/bin:/bin", "TMPDIR=" + filepath.Dir(work)}, work, io.Discard, io.Discard
	if err = init.Run(); err != nil {
		t.Fatal("isolated fixture bootstrap", err)
	}
	c.WorkspaceRoots = []string{work}
	return c
}

func copyPublishedHostExecutable(source, destination string) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Size() > 256<<20 {
		return "", errors.New("invalid published executable")
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, (256<<20)+1))
	if err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != contract.QualifiedRelease().Executable.SHA256 {
		return "", errors.New("published executable digest mismatch")
	}
	return destination, nil
}

func TestPublishedHostBootstrapRejectsUnqualifiedExecutable(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "selected")
	if err := os.WriteFile(source, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	qualified, err := copyPublishedHostExecutable(source, filepath.Join(root, "dolgorae"))
	if err == nil || qualified != "" {
		t.Fatal("unqualified bootstrap executable admitted", qualified, err)
	}
}

func TestPublishedProductionAssemblyPreservesLocalHostAfterHandshakeLoss(t *testing.T) {
	binary := os.Getenv("GUL_E2_DOLGORAE_EXECUTABLE")
	if binary == "" {
		t.Skip("published artifact qualification is opt-in")
	}
	archive, err := os.ReadFile(os.Getenv("GUL_E2_DOLGORAE_ARCHIVE"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	if hex.EncodeToString(digest[:]) != contract.QualifiedRelease().Archive.SHA256 {
		t.Fatal("published archive identity")
	}
	c := publishedHostConfig(t, binary)
	// Exercise the production assembly boundary deterministically, after its
	// accepted handshake but before construction, using the actual released child.
	c.Assemble = func(store *storage.Store) (Assembly, error) {
		g := gateway.New(gateway.Config{Executable: c.DolgoraeExecutable, Home: c.ProviderHome, WorkspaceRoots: c.WorkspaceRoots})
		if err := g.Start(t.Context()); err != nil {
			return Assembly{}, err
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := g.Stop(ctx); err != nil {
				t.Error(err)
			}
		})
		caps := g.NegotiatedCapabilities()
		if caps == nil {
			t.Fatal("missing accepted handshake", g.Status())
		}
		altered := g.NegotiatedCapabilities()
		altered.DescriptorSha256 = "caller mutation"
		if !proto.Equal(caps, g.NegotiatedCapabilities()) {
			t.Fatal("shared mutable handshake")
		}
		if err := g.Stop(t.Context()); err != nil {
			return Assembly{}, err
		}
		assembled, err := assembleGateway(store, c, g)
		if err == nil && (assembled.Features == nil || g.Status().ChannelReady) {
			t.Fatal("lost provider discarded production handlers or opened gate")
		}
		return assembled, err
	}
	offlineHost(t, c, false)
}

func offlineHost(t *testing.T, c Config, published bool) {
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

	if published {
		runtimeClient := gulv1connect.NewRuntimeServiceClient(httpClient, h.Origin())
		profiles := connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{})
		withSession(profiles)
		result, err := runtimeClient.ListRuntimeProfiles(t.Context(), profiles)
		if err != nil || result == nil {
			t.Fatal("ordinary production profiles", err, h.Core.ProviderStatus())
		}
		diagnostic := connect.NewRequest(&gulv1.GetSummaryRequest{})
		withSession(diagnostic)
		summary, err := gulv1connect.NewDiagnosticsServiceClient(httpClient, h.Origin()).GetSummary(t.Context(), diagnostic)
		if err != nil || !summary.Msg.ProviderReady || summary.Msg.ProviderVersion != "0.1.3" || summary.Msg.ProviderProtocol != 1 || summary.Msg.ProviderCapabilities.ReaderWriterAccess || !summary.Msg.ProviderCapabilities.DedicatedWriterSupport {
			t.Fatal("production diagnostics", summary, err)
		}
		// Stop only the host's owned provider; its authenticated local core remains.
		stop, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		if err = h.gateway.Stop(stop); err != nil {
			t.Fatal(err)
		}
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
	submit := connect.NewRequest(&gulv1.SubmitRequest{SessionId: "missing", AttemptId: "offline", Text: "must not dispatch", WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ})
	withSession(submit)
	if result, err := directClient.Submit(t.Context(), submit); connect.CodeOf(err) != connect.CodeUnavailable && (err != nil || result == nil || result.Msg.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_REJECTED) {
		t.Fatalf("production Submit did not fail closed: %v, %v", result, err)
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
	if result, err := workspaceClient.ListWorkspaces(t.Context(), list); err != nil || len(result.Msg.Workspaces) != 1 {
		t.Fatalf("offline local workspace listing: %v, %v", result, err)
	}
}
