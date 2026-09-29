package files

import (
	"bytes"
	"context"
	"fmt"
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
	newFile, err := service.Compare(t.Context(), "owner", "entry", "untracked.txt")
	if err != nil || newFile.State != GitAvailable || newFile.Change != ChangeUntracked || !newFile.HeadMissing || newFile.WorkingMissing || newFile.Working.Text != "new" {
		t.Fatalf("untracked compare = %+v, %v", newFile, err)
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
	raster, err := service.Compare(t.Context(), "owner", "entry", "img.png")
	if err != nil || raster.State != GitAvailable || raster.Head.Kind != PreviewRaster || raster.Working.Kind != PreviewRaster || !bytes.Equal(raster.Head.Image, rasterFixture(t, 10)) || !bytes.Equal(raster.Working.Image, rasterFixture(t, 200)) {
		t.Fatalf("raster revisions = %+v, %v", raster, err)
	}
	literal, err := service.Compare(t.Context(), "owner", "entry", "[a].txt")
	if err != nil || literal.Head.Text != "literal base\n" || literal.Working.Text != "literal working\n" {
		t.Fatalf("literal filename compare = %+v, %v", literal, err)
	}
}

func TestGitAddedAndNestedDirectoryStatus(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	for _, dir := range []string{"nested", "sibling"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
		writePreviewFile(t, filepath.Join(root, dir), "base.txt", []byte("base"))
	}
	gitCommand(t, root, "add", "-A")
	gitCommand(t, root, "commit", "-qm", "base")
	writePreviewFile(t, filepath.Join(root, "nested"), "added.txt", []byte("added"))
	gitCommand(t, root, "add", "nested/added.txt")
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for _, tc := range []struct {
		path              string
		direct, aggregate ChangeKind
		staged, unstaged  bool
	}{
		{"nested/added.txt", ChangeAdded, ChangeAdded, true, false},
		{"nested", ChangeClean, ChangeAdded, false, false},
		{"sibling", ChangeClean, ChangeClean, false, false},
	} {
		status, err := service.GitStatus(t.Context(), "owner", "entry", tc.path)
		if err != nil || status.State != GitAvailable || status.Direct != tc.direct || status.Aggregate != tc.aggregate || status.Staged != tc.staged || status.Unstaged != tc.unstaged {
			t.Fatalf("status %s = %+v, %v", tc.path, status, err)
		}
	}
	added, err := service.Compare(t.Context(), "owner", "entry", "nested/added.txt")
	if err != nil || added.State != GitAvailable || added.Change != ChangeAdded || !added.HeadMissing || added.WorkingMissing || added.Working.Text != "added" {
		t.Fatalf("added compare = %+v, %v", added, err)
	}
}

func TestGitConflictStatus(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	writePreviewFile(t, root, "conflict.txt", []byte("base\n"))
	gitCommand(t, root, "add", "conflict.txt")
	gitCommand(t, root, "commit", "-qm", "base")
	gitCommand(t, root, "checkout", "-qb", "side")
	writePreviewFile(t, root, "conflict.txt", []byte("side\n"))
	gitCommand(t, root, "commit", "-qam", "side")
	gitCommand(t, root, "checkout", "-q", "-")
	writePreviewFile(t, root, "conflict.txt", []byte("main\n"))
	gitCommand(t, root, "commit", "-qam", "main")
	merge := exec.CommandContext(t.Context(), "git", "-C", root, "merge", "side")
	if output, err := merge.CombinedOutput(); err == nil {
		t.Fatalf("expected merge conflict: %s", output)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	status, err := service.GitStatus(t.Context(), "owner", "entry", "conflict.txt")
	if err != nil || status.State != GitAvailable || status.Direct != ChangeConflicted || status.Aggregate != ChangeConflicted || !status.Staged || !status.Unstaged {
		t.Fatalf("conflict status = %+v, %v", status, err)
	}
}

func TestGitLimitExceededKeepsWorkingPreview(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	writePreviewFile(t, root, "current.txt", []byte("current"))
	gitCommand(t, root, "add", "current.txt")
	gitCommand(t, root, "commit", "-qm", "base")
	for i := 0; i <= MaxGitEntries; i++ {
		writePreviewFile(t, root, fmt.Sprintf("untracked-%04d", i), []byte("x"))
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	status, err := service.GitStatus(t.Context(), "owner", "entry", "current.txt")
	if err != nil || status.State != GitLimitExceeded {
		t.Fatalf("limit status = %+v, %v", status, err)
	}
	comparison, err := service.Compare(t.Context(), "owner", "entry", "current.txt")
	if err != nil || comparison.State != GitLimitExceeded || comparison.Working.Text != "current" {
		t.Fatalf("limit comparison = %+v, %v", comparison, err)
	}
}

func TestGitCommandBoundsKeepWorkingPreview(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	writePreviewFile(t, root, "current.txt", []byte("current"))
	gitCommand(t, root, "add", "current.txt")
	gitCommand(t, root, "commit", "-qm", "base")
	for _, tc := range []struct {
		name, command string
		want          GitState
	}{
		{"output-cap", "exec dd if=/dev/zero bs=1048577 count=1 2>/dev/null", GitLimitExceeded},
		{"timeout", "exec sleep 10", GitUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := filepath.Join(t.TempDir(), "git-wrapper")
			script := "#!/bin/sh\ncase \" $* \" in\n  *\" status \"*) " + tc.command + ";;\n  *) exec git \"$@\";;\nesac\n"
			if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			service := NewService(attachmentStore{attachedRoot(t, root)})
			service.gitBinary = wrapper
			comparison, err := service.Compare(t.Context(), "owner", "entry", "current.txt")
			if err != nil || comparison.State != tc.want || comparison.Working.Text != "current" {
				t.Fatalf("bounded Git comparison = %+v, %v", comparison, err)
			}
		})
	}
}

func TestOversizeHeadRasterRetainsWorkingRaster(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	writePreviewFile(t, root, "img.png", bytes.Repeat([]byte{'x'}, MaxImageBytes+1))
	gitCommand(t, root, "add", "img.png")
	gitCommand(t, root, "commit", "-qm", "base")
	working := rasterFixture(t, 42)
	writePreviewFile(t, root, "img.png", working)
	service := NewService(attachmentStore{attachedRoot(t, root)})
	comparison, err := service.Compare(t.Context(), "owner", "entry", "img.png")
	if err != nil || comparison.State != GitAvailable || comparison.Head.Kind != PreviewUnsupported || comparison.Working.Kind != PreviewRaster || !bytes.Equal(comparison.Working.Image, working) {
		t.Fatalf("oversize HEAD raster = %+v, %v", comparison, err)
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
