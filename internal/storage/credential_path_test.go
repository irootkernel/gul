package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type foreignOwnerInfo struct{ os.FileInfo }

func (info foreignOwnerInfo) Sys() any {
	stat := *info.FileInfo.Sys().(*syscall.Stat_t)
	stat.Uid = uint32(os.Getuid() + 1)
	return &stat
}

func TestResolveCredentialPathRevalidatesEveryComponent(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "binding")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	credential := filepath.Join(nested, "key")
	if err := os.WriteFile(credential, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if path, err := ResolveCredentialPath(root, "binding/key"); err != nil || path != credential {
		t.Fatalf("resolved path = %q, %v", path, err)
	}
	if _, err := ResolveCredentialPath("relative-root", "binding/key"); err == nil {
		t.Fatal("accepted relative credential root")
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCredentialPath(root, "binding/key"); err == nil {
		t.Fatal("accepted alternate unsafe root")
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	rootAlias := root + "-alias"
	if err := os.Symlink(root, rootAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCredentialPath(rootAlias, "binding/key"); err == nil {
		t.Fatal("accepted symlinked alternate root")
	}
	info, err := os.Lstat(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOwnerOnlyInfo(credential, foreignOwnerInfo{info}, false, os.Getuid()); err == nil {
		t.Fatal("accepted foreign-owned credential metadata")
	}
	if err := os.Mkdir(filepath.Join(nested, "directory-key"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCredentialPath(root, "binding/directory-key"); err == nil {
		t.Fatal("accepted directory in place of credential file")
	}
	if err := os.WriteFile(filepath.Join(root, "file-parent"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCredentialPath(root, "file-parent/key"); err == nil {
		t.Fatal("accepted file in place of credential directory")
	}
	for _, key := range []string{"/absolute", "../outside", "binding/../key", "binding\\key", "binding//key"} {
		if _, err := ResolveCredentialPath(root, key); err == nil {
			t.Errorf("accepted unsafe key %q", key)
		}
	}
	if err := os.Chmod(credential, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCredentialPath(root, "binding/key"); err == nil {
		t.Fatal("accepted world-readable credential")
	}
	if err := os.Chmod(credential, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(nested, nested+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(nested+"-old", nested); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCredentialPath(root, "binding/key"); err == nil {
		t.Fatal("accepted replacement symlink")
	}
}
