package files

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", root}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, output, err)
	}
}

func initGit(t *testing.T, root string) {
	t.Helper()
	gitCommand(t, root, "init", "-q")
	gitCommand(t, root, "config", "user.name", "Fixture")
	gitCommand(t, root, "config", "user.email", "fixture@example.invalid")
}

func rasterFixture(t *testing.T, value uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: value, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestGitReviewFixedRevisionsAndPrivateFiltering(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	for name, body := range map[string][]byte{
		"text.txt": []byte("base\n"), "gone.txt": []byte("gone\n"), "old.txt": []byte("old\n"),
		"doc.md": []byte("# Image\n![local](img.png)\n"), "img.png": rasterFixture(t, 10),
		"[a].txt": []byte("literal base\n"), "a.txt": []byte("other base\n"),
	} {
		writePreviewFile(t, root, name, body)
	}
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret.txt", []byte("base secret"))
	gitCommand(t, root, "add", "-A")
	gitCommand(t, root, "commit", "-qm", "base")
	writePreviewFile(t, root, "text.txt", []byte("staged\n"))
	gitCommand(t, root, "add", "text.txt")
	writePreviewFile(t, root, "text.txt", []byte("working\n"))
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "old.txt"), filepath.Join(root, "new.txt")); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, root, "add", "-A", "old.txt", "new.txt")
	writePreviewFile(t, root, "untracked.txt", []byte("new"))
	writePreviewFile(t, root, "img.png", rasterFixture(t, 200))
	writePreviewFile(t, root, "[a].txt", []byte("literal working\n"))
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret.txt", []byte("changed secret"))
	if err := os.Symlink(".dolgorae/secret.txt", filepath.Join(root, "private-alias")); err != nil {
		t.Fatal(err)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for _, tc := range []struct {
		path             string
		direct           ChangeKind
		staged, unstaged bool
	}{
		{"text.txt", ChangeModified, true, true}, {"gone.txt", ChangeDeleted, false, true},
		{"new.txt", ChangeRenamed, true, false}, {"untracked.txt", ChangeUntracked, false, true},
	} {
		status, err := service.GitStatus(t.Context(), "owner", "entry", tc.path)
		if err != nil || status.State != GitAvailable || status.Direct != tc.direct || status.Staged != tc.staged || status.Unstaged != tc.unstaged {
			t.Fatalf("status %s = %+v, %v", tc.path, status, err)
		}
	}
	rootStatus, err := service.GitStatus(t.Context(), "owner", "entry", ".")
	if err != nil || rootStatus.Aggregate != ChangeMixed {
		t.Fatalf("root status = %+v, %v", rootStatus, err)
	}
	for _, unsafe := range []string{".dolgorae/secret.txt", "private-alias"} {
		if _, err := service.GitStatus(t.Context(), "owner", "entry", unsafe); err == nil {
			t.Fatalf("private Git path %q accepted", unsafe)
		}
	}
	changed, err := service.Compare(t.Context(), "owner", "entry", "text.txt")
	if err != nil || changed.State != GitAvailable || changed.Head.Text != "base\n" || changed.Working.Text != "working\n" {
		t.Fatalf("text compare = %+v, %v", changed, err)
	}
	deleted, err := service.Compare(t.Context(), "owner", "entry", "gone.txt")
	if err != nil || deleted.HeadMissing || !deleted.WorkingMissing || deleted.Head.Text != "gone\n" {
		t.Fatalf("deleted compare = %+v, %v", deleted, err)
	}
	renamed, err := service.Compare(t.Context(), "owner", "entry", "new.txt")
	if err != nil || renamed.PreviousPath != "old.txt" || renamed.Head.Text != "old\n" || renamed.Working.Text != "old\n" {
		t.Fatalf("renamed compare = %+v, %v", renamed, err)
	}
	markdown, err := service.Compare(t.Context(), "owner", "entry", "doc.md")
	if err != nil || len(markdown.Head.MarkdownImages) != 1 || len(markdown.Working.MarkdownImages) != 1 || bytes.Equal(markdown.Head.MarkdownImages[0].Data, markdown.Working.MarkdownImages[0].Data) {
		t.Fatalf("Markdown revision assets = %+v, %v", markdown, err)
	}
	literal, err := service.Compare(t.Context(), "owner", "entry", "[a].txt")
	if err != nil || literal.Head.Text != "literal base\n" || literal.Working.Text != "literal working\n" {
		t.Fatalf("literal filename compare = %+v, %v", literal, err)
	}
}

func TestGitDegradationKeepsWorkingPreview(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "current.txt", []byte("current"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for _, want := range []GitState{GitNotRepository, GitUnavailable, GitUnbornHead} {
		if want == GitUnavailable {
			service.gitBinary = filepath.Join(root, "missing-git")
		}
		if want == GitUnbornHead {
			service.gitBinary = "git"
			initGit(t, root)
		}
		comparison, err := service.Compare(context.Background(), "owner", "entry", "current.txt")
		if err != nil || comparison.State != want || comparison.Working.Text != "current" {
			t.Fatalf("degraded %s = %+v, %v", want, comparison, err)
		}
	}
	parent := t.TempDir()
	initGit(t, parent)
	workspace := filepath.Join(parent, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, workspace, "file.txt", []byte("contained"))
	outside := NewService(attachmentStore{attachedRoot(t, workspace)})
	comparison, err := outside.Compare(t.Context(), "owner", "entry", "file.txt")
	if err != nil || comparison.State != GitNotRepository || comparison.Working.Text != "contained" {
		t.Fatalf("outside Git root = %+v, %v", comparison, err)
	}
}
