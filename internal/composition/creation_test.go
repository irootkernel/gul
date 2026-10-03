package composition

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type unavailableWorkspace struct{ calls int }

func (p *unavailableWorkspace) InspectWorkspace(context.Context, string, *string) (workspace.Inspection, error) {
	p.calls++
	return workspace.Inspection{}, workspace.ErrProviderUnavailable
}

func TestPendingCreationsRemainSubjectScopedWithoutProvider(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(t.Context(), filepath.Join(root, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const subject = "local-owner"
	if err := db.Auth().CreateAccount(t.Context(), subject, time.Now()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := info.Sys().(*syscall.Stat_t)
	if err := db.Presentation().CreateAttachment(t.Context(), workspace.Attachment{
		SubjectID: subject, ID: "saved", CanonicalRoot: root, ProviderID: "provider-workspace",
		FileDevice: fmt.Sprint(identity.Dev), FileInode: fmt.Sprint(identity.Ino), DisplayName: "Saved",
	}); err != nil {
		t.Fatal(err)
	}
	credential := storage.ControllerCredential{
		BindingReference: storage.BindingReference{BindingID: "binding", SubjectID: subject, CredentialKey: "local.json", ExpectedControllerID: "controller", Health: "healthy"},
		Generation:       1, InstanceID: "installation", FileIdentity: storage.FileIdentity{Inode: 1, Size: 1},
	}
	if err := db.Presentation().RegisterCredential(t.Context(), credential); err != nil {
		t.Fatal(err)
	}
	if err := db.Presentation().ReserveCreation(t.Context(), storage.Creation{
		OperationID: "pending", SubjectID: subject, WorkspaceID: "saved", AttemptID: "public-label",
		ChoiceJSON: "{}", ControllerBindingID: "binding", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	p := &unavailableWorkspace{}
	r := &Runtime{store: db, Workspaces: workspace.NewService(p, db.Presentation(), nil, nil)}
	labels, err := r.PendingCreations(t.Context(), subject, "saved")
	if err != nil || !slices.Equal(labels, []string{"public-label"}) || p.calls != 0 {
		t.Fatal("local pending labels required provider access", labels, err, p.calls)
	}
	for _, owner := range []string{"foreign-owner", ""} {
		if labels, err := r.PendingCreations(t.Context(), owner, "saved"); !errors.Is(err, workspace.ErrAttachmentNotFound) || len(labels) != 0 {
			t.Fatal("foreign subject read pending labels", labels, err)
		}
	}
	if labels, err := r.PendingCreations(t.Context(), subject, "missing"); !errors.Is(err, workspace.ErrAttachmentNotFound) || len(labels) != 0 {
		t.Fatal("unknown Workspace returned pending labels", labels, err)
	}
	if p.calls != 0 {
		t.Fatal("local ownership checks invoked the provider", p.calls)
	}
}
