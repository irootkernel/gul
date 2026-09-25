package storage

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/workspace"
)

func TestLocalPresentationPersistsAndIsSubjectScoped(t *testing.T) {
	store, filename := openTestStore(t)
	for _, subject := range []string{"owner", "other"} {
		if err := store.Auth().CreateAccount(t.Context(), subject, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := store.Presentation().CreateAttachment(t.Context(), workspace.Attachment{
			SubjectID: subject, ID: "workspace", CanonicalRoot: filepath.Join(t.TempDir(), "workspace"),
			ProviderID: "provider-workspace", FileDevice: "1", FileInode: "2", DisplayName: "Original",
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.Presentation().InsertDirectSessionIfAbsent(t.Context(), DirectSessionPresentation{
			SubjectID: subject, WorkspaceID: "workspace", SessionID: "session", DisplayName: "Original session",
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc := presentation.NewService(store.Presentation())
	ctx := t.Context()
	if _, err := svc.RenameWorkspace(ctx, "owner", "workspace", "My workspace"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetWorkspaceFavorite(ctx, "owner", "workspace", true); err != nil {
		t.Fatal(err)
	}
	if attachment, err := store.Presentation().Attachment(ctx, "owner", "workspace"); err != nil || !attachment.Favorite {
		t.Fatalf("attachment favorite = %+v, %v", attachment, err)
	}
	if entries, err := store.Presentation().ListAttachments(ctx, "owner"); err != nil || len(entries) != 1 || !entries[0].Favorite {
		t.Fatalf("listed favorite = %+v, %v", entries, err)
	}
	if err := store.Presentation().CreateAttachment(ctx, workspace.Attachment{
		SubjectID: "owner", ID: "other-workspace", CanonicalRoot: filepath.Join(t.TempDir(), "other-workspace"),
		ProviderID: "other-provider-workspace", FileDevice: "3", FileInode: "4", DisplayName: "Other workspace",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetNavigation(ctx, "owner", presentation.Navigation{WorkspaceID: "other-workspace", SessionID: "session"}); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("cross-workspace session navigation = %v", err)
	}
	if _, err := svc.RenameDirectSession(ctx, "owner", "session", "My session"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDirectSessionFavorite(ctx, "owner", "session", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetNavigation(ctx, "owner", presentation.Navigation{WorkspaceID: "workspace", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetWorkspaceHidden(ctx, "owner", "workspace", true); err != nil {
		t.Fatal(err)
	}
	if navigation, err := svc.Navigation(ctx, "owner"); err != nil || navigation != (presentation.Navigation{}) {
		t.Fatalf("hidden workspace navigation = %+v, %v", navigation, err)
	}
	if _, err := svc.SetNavigation(ctx, "owner", presentation.Navigation{WorkspaceID: "workspace"}); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("hidden workspace remained navigable: %v", err)
	}
	if _, err := svc.SetWorkspaceHidden(ctx, "owner", "workspace", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetNavigation(ctx, "owner", presentation.Navigation{WorkspaceID: "workspace", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDirectSessionArchived(ctx, "owner", "session", true); err != nil {
		t.Fatal(err)
	}
	if navigation, err := svc.Navigation(ctx, "owner"); err != nil || navigation != (presentation.Navigation{WorkspaceID: "workspace"}) {
		t.Fatalf("archived session navigation = %+v, %v", navigation, err)
	}
	if _, err := svc.SetNavigation(ctx, "owner", presentation.Navigation{WorkspaceID: "workspace", SessionID: "session"}); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("archived session remained navigable: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	svc = presentation.NewService(store.Presentation())
	if navigation, err := svc.Navigation(ctx, "owner"); err != nil || navigation != (presentation.Navigation{WorkspaceID: "workspace"}) {
		t.Fatalf("reopened navigation = %+v, %v", navigation, err)
	}
	owner, err := svc.Workspace(ctx, "owner", "workspace")
	if err != nil || owner.DisplayName != "My workspace" || !owner.Favorite || owner.Hidden {
		t.Fatalf("reopened owner workspace = %+v, %v", owner, err)
	}
	other, err := svc.Workspace(ctx, "other", "workspace")
	if err != nil || other.DisplayName != "Original" || other.Favorite || other.Hidden {
		t.Fatalf("other workspace changed = %+v, %v", other, err)
	}
	direct, err := svc.DirectSession(ctx, "owner", "session")
	if err != nil || direct.DisplayName != "My session" || !direct.Favorite || !direct.Archived {
		t.Fatalf("reopened owner session = %+v, %v", direct, err)
	}
	otherDirect, err := svc.DirectSession(ctx, "other", "session")
	if err != nil || otherDirect.DisplayName != "Original session" || otherDirect.Favorite || otherDirect.Archived {
		t.Fatalf("other session changed = %+v, %v", otherDirect, err)
	}
	if _, err := svc.RenameWorkspace(ctx, "other", "missing", "foreign"); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("missing workspace = %v", err)
	}
	if _, err := svc.RenameDirectSession(ctx, "other", "missing", "foreign"); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("missing session = %v", err)
	}
	if err := store.Presentation().InsertDirectSessionIfAbsent(ctx, DirectSessionPresentation{
		SubjectID: "owner", SessionID: "bad-name", WorkspaceID: "workspace", DisplayName: "line\nbreak",
	}); !errors.Is(err, presentation.ErrInvalid) {
		t.Fatalf("accepted invalid discovery name: %v", err)
	}
	if _, err := svc.SetWorkspaceFavorite(ctx, "owner", "missing", true); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("missing workspace favorite = %v", err)
	}
	if _, err := svc.SetDirectSessionFavorite(ctx, "owner", "missing", true); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("missing session favorite = %v", err)
	}
	for _, name := range []string{"", " padded ", "line\nbreak", string([]byte{0xff}), strings.Repeat("a", 121)} {
		if _, err := svc.RenameWorkspace(ctx, "owner", "workspace", name); !errors.Is(err, presentation.ErrInvalid) {
			t.Fatalf("accepted invalid name %q: %v", name, err)
		}
	}
	if _, err := svc.RenameWorkspace(ctx, "owner", "workspace", strings.Repeat("가", 120)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetNavigation(ctx, "owner", presentation.Navigation{SessionID: "session"}); !errors.Is(err, presentation.ErrInvalid) {
		t.Fatalf("session-only navigation = %v", err)
	}
	if _, err := svc.SetDirectSessionArchived(ctx, "owner", "session", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDirectSessionArchived(ctx, "owner", "session", false); err != nil {
		t.Fatalf("idempotent unarchive: %v", err)
	}
	if _, err := svc.SetWorkspaceHidden(ctx, "owner", "workspace", false); err != nil {
		t.Fatalf("idempotent unhide: %v", err)
	}
	if _, err := svc.SetWorkspaceFavorite(ctx, "owner", "workspace", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDirectSessionFavorite(ctx, "owner", "session", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetNavigation(ctx, "owner", presentation.Navigation{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if owner, err := store.Presentation().Workspace(ctx, "owner", "workspace"); err != nil || owner.Favorite {
		t.Fatalf("cleared workspace favorite = %+v, %v", owner, err)
	}
	if direct, err := store.Presentation().DirectSession(ctx, "owner", "session"); err != nil || direct.Favorite || direct.Archived {
		t.Fatalf("cleared session flags = %+v, %v", direct, err)
	}
	if navigation, err := store.Presentation().Navigation(ctx, "owner"); err != nil || navigation != (presentation.Navigation{}) {
		t.Fatalf("cleared navigation = %+v, %v", navigation, err)
	}
}
