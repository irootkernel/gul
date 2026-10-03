package files

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func imageFixture(t *testing.T) (*SubmitImages, string, []byte) {
	t.Helper()
	root := t.TempDir()
	var body bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&body, picture); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "picture.png"), body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	scratch, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	return &SubmitImages{Root: scratch, Files: NewService(attachmentStore{attachedRoot(t, root)})}, root, body.Bytes()
}

func TestSubmitImageCopiesGuardedBytesAndSettlesOnlyAfterFreshObservation(t *testing.T) {
	s, root, original := imageFixture(t)
	refs := []ImageReference{{"entry", "picture.png", "high"}}
	images, err := s.Stage(t.Context(), "owner", "entry", "run", "key", refs)
	if err != nil || len(images) != 1 {
		t.Fatal(images, err)
	}
	copyPath := images[0].Path
	if filepath.Dir(copyPath) == root || images[0].Detail != "high" {
		t.Fatal("original path reached provider")
	}
	info, err := os.Stat(copyPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unprotected image", info, err)
	}
	parent, err := os.Stat(filepath.Dir(copyPath))
	if err != nil || parent.Mode().Perm() != 0700 {
		t.Fatal("unprotected parent", parent, err)
	}
	retry, err := s.Stage(t.Context(), "owner", "entry", "run", "key", refs)
	if err != nil || retry[0].Path != copyPath {
		t.Fatal("same input lost its exact private snapshot", retry, err)
	}
	if err := os.WriteFile(filepath.Join(root, "picture.png"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(copyPath)
	if err != nil || !bytes.Equal(body, original) {
		t.Fatal("workspace edit changed staged bytes", err)
	}
	beforeAcceptance := time.Now()
	s.Accepted("key")
	s.ReleaseRun("run", false, beforeAcceptance)
	if _, err := os.Stat(copyPath); err != nil {
		t.Fatal("a read begun before acceptance removed the input", err)
	}
	s.ReleaseRun("run", false, time.Now())
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Fatal("settled input retained", err)
	}
}

func TestSubmitImageRejectsProviderPrivatePathsAndAliases(t *testing.T) {
	s, root, body := imageFixture(t)
	if err := os.MkdirAll(filepath.Join(root, ".dolgorae", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".dolgorae", "nested", "secret.png"), body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".dolgorae", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("alias/nested/secret.png", filepath.Join(root, "chain.png")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".dolgorae/nested/secret.png", "alias/nested/secret.png", "chain.png", "../picture.png", filepath.Join(root, "picture.png")} {
		if images, err := s.Stage(t.Context(), "owner", "entry", "run", "key", []ImageReference{{"entry", path, "auto"}}); err == nil || images != nil {
			t.Fatalf("unsafe path %q accepted", path)
		}
	}
	for _, ref := range []ImageReference{{"foreign", "picture.png", "auto"}, {"entry", "picture.png", "unknown"}} {
		if _, err := s.Stage(t.Context(), "owner", "entry", "run", "key", []ImageReference{ref}); err == nil {
			t.Fatal("untrusted image reference accepted")
		}
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil || len(entries) != 0 {
		t.Fatal("rejected input left scratch files", entries, err)
	}
}

func TestUnknownSubmitImageSurvivesIdleAndRestartUntilItsOwnAggregateCloses(t *testing.T) {
	s, _, _ := imageFixture(t)
	refs := []ImageReference{{"entry", "picture.png", "auto"}}
	first, err := s.Stage(t.Context(), "owner", "entry", "first", "key-1", refs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Stage(t.Context(), "owner", "entry", "second", "key-2", refs)
	if err != nil {
		t.Fatal(err)
	}
	s.ReleaseRun("first", false, time.Now())
	if _, err := os.Stat(first[0].Path); err != nil {
		t.Fatal("unknown input removed on idle", err)
	}
	restarted := &SubmitImages{Root: s.Root, Files: s.Files}
	restarted.ReleaseRun("first", true, time.Now())
	if _, err := os.Stat(first[0].Path); !os.IsNotExist(err) {
		t.Fatal("closed input retained across restart", err)
	}
	if _, err := os.Stat(second[0].Path); err != nil {
		t.Fatal("unrelated input removed", err)
	}
	s.Rejected("key-2")
	if _, err := os.Stat(second[0].Path); !os.IsNotExist(err) {
		t.Fatal("known rejection retained input", err)
	}
}
