package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/rootkernel/gul/internal/workspace"
	"golang.org/x/sys/unix"
)

type attachmentStore struct {
	entry workspace.Attachment
}

func (s attachmentStore) Attachment(_ context.Context, subject, id string) (workspace.Attachment, error) {
	if subject != s.entry.SubjectID || id != s.entry.ID {
		return workspace.Attachment{}, workspace.ErrAttachmentNotFound
	}
	return s.entry, nil
}

func attachedRoot(t *testing.T, root string) workspace.Attachment {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := info.Sys().(*syscall.Stat_t)
	return workspace.Attachment{
		SubjectID: "owner", ID: "entry", CanonicalRoot: canonical,
		ProviderID: "provider-workspace",
		FileDevice: fmt.Sprint(identity.Dev),
		FileInode:  fmt.Sprint(identity.Ino),
	}
}

func TestGuardedOpenConfinesEveryPathToVerifiedRoot(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, ".dolgorae")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "secret"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{
		"inside":               "safe.txt",
		"nested/inside":        "../safe.txt",
		"private-alias":        ".dolgorae",
		"nested/private-alias": "../.dolgorae",
		"outside-alias":        outside,
		"nested/escape":        "../../outside",
	} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for _, relative := range []string{"safe.txt", "inside", "nested/inside"} {
		file, _, err := service.Open(t.Context(), "owner", "entry", relative)
		if err != nil {
			t.Fatalf("open %q: %v", relative, err)
		}
		body, err := io.ReadAll(file)
		file.Close()
		if err != nil || string(body) != "safe" {
			t.Fatalf("read %q = %q, %v", relative, body, err)
		}
	}
	for _, relative := range []string{
		"../outside", "/etc/passwd", "nested/../../outside", ".dolgorae",
		".dolgorae/secret", "private-alias/secret", "nested/private-alias/secret",
		"outside-alias/outside", "nested/escape", "missing", "safe.txt/child",
	} {
		file, _, err := service.Open(t.Context(), "owner", "entry", relative)
		if file != nil {
			file.Close()
		}
		if !errors.Is(err, ErrPathUnavailable) {
			t.Fatalf("unsafe %q = %v", relative, err)
		}
	}
	if _, _, err := service.Open(t.Context(), "owner", "entry", string([]byte{'x', 0xff})); !errors.Is(err, ErrUnsupportedPathEncoding) {
		t.Fatalf("non-UTF-8 path = %v", err)
	}
	if _, _, err := service.Open(t.Context(), "other", "entry", "safe.txt"); !errors.Is(err, ErrReattachRequired) {
		t.Fatalf("foreign attachment = %v", err)
	}
}

func TestReplacedWorkspaceRootFailsClosed(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	if err := os.Rename(root, filepath.Join(parent, "old-workspace")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Open(t.Context(), "owner", "entry", "."); !errors.Is(err, ErrReattachRequired) {
		t.Fatalf("replaced root = %v", err)
	}
}

func TestMovedWorkspaceRootThroughAncestorAliasFailsClosed(t *testing.T) {
	parent := t.TempDir()
	ancestor := filepath.Join(parent, "original")
	root := filepath.Join(ancestor, "workspace")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	if err := os.Rename(ancestor, filepath.Join(parent, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("moved", ancestor); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Open(t.Context(), "owner", "entry", "."); !errors.Is(err, ErrReattachRequired) {
		t.Fatalf("moved root through ancestor alias = %v", err)
	}
}

func TestSymlinkReplacementCannotReadPrivateBytes(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".dolgorae", "secret"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink("safe", alias); err != nil {
		t.Fatal(err)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for i := 0; i < 200; i++ {
		target := "safe"
		if i%2 == 0 {
			target = ".dolgorae/secret"
		}
		next := filepath.Join(root, "next")
		if err := os.Symlink(target, next); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(next, alias); err != nil {
			t.Fatal(err)
		}
		file, _, err := service.Open(t.Context(), "owner", "entry", "alias")
		if err != nil {
			if !errors.Is(err, ErrPathUnavailable) {
				t.Fatal(err)
			}
			continue
		}
		body, readErr := io.ReadAll(file)
		file.Close()
		if readErr != nil || string(body) != "safe" {
			t.Fatalf("replacement read = %q, %v", body, readErr)
		}
	}
	changes := make(chan error, 1)
	go func() {
		for i := 0; i < 300; i++ {
			target := "safe"
			if i%2 == 0 {
				target = ".dolgorae/secret"
			}
			next := filepath.Join(root, "next")
			if err := os.Symlink(target, next); err != nil {
				changes <- err
				return
			}
			if err := os.Rename(next, alias); err != nil {
				changes <- err
				return
			}
		}
		changes <- nil
	}()
	for i := 0; i < 300; i++ {
		file, _, err := service.Open(t.Context(), "owner", "entry", "alias")
		if err != nil {
			if !errors.Is(err, ErrPathUnavailable) {
				t.Fatal(err)
			}
			continue
		}
		body, readErr := io.ReadAll(file)
		file.Close()
		if readErr != nil || string(body) != "safe" {
			t.Fatalf("concurrent replacement read = %q, %v", body, readErr)
		}
	}
	if err := <-changes; err != nil {
		t.Fatal(err)
	}
}

func TestMovedOpenDirectoryCannotSupplyAFile(t *testing.T) {
	for _, destination := range []string{"outside", "private"} {
		t.Run(destination, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "nested", "secret"), []byte("secret"), 0600); err != nil {
				t.Fatal(err)
			}
			dir, err := os.Open(filepath.Join(root, "nested"))
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()
			var moved string
			if destination == "private" {
				if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
					t.Fatal(err)
				}
				moved = filepath.Join(root, ".dolgorae", "nested")
			} else {
				moved = filepath.Join(t.TempDir(), "nested")
			}
			if err := os.Rename(filepath.Join(root, "nested"), moved); err != nil {
				t.Fatal(err)
			}
			fd, err := unix.Openat(int(dir.Fd()), "secret", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			file := os.NewFile(uintptr(fd), filepath.Join(root, "nested", "secret"))
			defer file.Close()
			if _, err := verifiedOpenPath(file, root); !errors.Is(err, ErrPathUnavailable) {
				t.Fatalf("moved directory yielded accessible file: %v", err)
			}
		})
	}
}
