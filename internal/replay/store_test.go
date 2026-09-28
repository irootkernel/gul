package replay

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(now time.Time) StartRun {
	return StartRun{OperationID: "attempt_1", SubjectID: "owner", WorkspaceID: "workspace", ProviderWorkspaceID: "provider-workspace",
		CredentialKey: "controllers/destination", ControllerID: "controller", ControllerGeneration: 1, IdempotencyKey: "key",
		ProfileName: "profile", ControlMode: 1, ExecutionLane: 1, Purpose: 1, RequiredAssurance: 1,
		RequiredCapabilities: []string{"artifact_capability"}, CreatedAt: now}
}

func canonicalRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExactStartRunMaterialIsOwnerOnlyAndBoundToDigest(t *testing.T) {
	root := canonicalRoot(t)
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: root}
	request := fixture(time.Now().UTC())
	digest, err := store.Put(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, request.OperationID+".json")
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("replay mode = %v, %v", info, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/Users/owner/carrier", "capability-secret", "prompt-secret", "image-secret"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("replay material contains %q", forbidden)
		}
	}
	got, err := store.Get(request.OperationID, digest)
	if err != nil || got.IdempotencyKey != request.IdempotencyKey || got.CredentialKey != request.CredentialKey {
		t.Fatalf("reloaded material = %+v, %v", got, err)
	}
	if _, err := store.Put(request); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate material = %v", err)
	}
	if _, err := store.Get(request.OperationID, strings.Repeat("0", 64)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong digest = %v", err)
	}
	if err := store.Delete(request.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolved material remains: %v", err)
	}
}

func TestReplayRejectsUnsafeRootAndLogicalReference(t *testing.T) {
	root := canonicalRoot(t)
	request := fixture(time.Now().UTC())
	if _, err := (Store{Root: root}).Put(request); err == nil {
		t.Fatal("permissive root accepted")
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"/absolute", "../outside", "a/../outside", "."} {
		request.CredentialKey = key
		if _, err := (Store{Root: root}).Put(request); !errors.Is(err, ErrInvalid) {
			t.Fatalf("credential key %q = %v", key, err)
		}
	}
	request = fixture(time.Now().UTC())
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: link}).Put(request); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("symlink root = %v", err)
	}
	parentLink := filepath.Join(canonicalRoot(t), "linked-parent")
	if err := os.Symlink(filepath.Dir(root), parentLink); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: filepath.Join(parentLink, filepath.Base(root))}).Put(request); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("symlink parent = %v", err)
	}
}

func TestPurgeExpiresMaterialWithoutInferringOutcome(t *testing.T) {
	root := canonicalRoot(t)
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: root}
	now := time.Now().UTC()
	request := fixture(now.Add(-MaximumAge))
	if _, err := store.Put(request); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, request.OperationID+".json")
	if err := os.Chtimes(path, now.Add(-MaximumAge), now.Add(-MaximumAge)); err != nil {
		t.Fatal(err)
	}
	keys, err := store.PurgeExpired(now, MaximumAge)
	if err != nil || len(keys) != 1 || keys[0] != request.OperationID {
		t.Fatalf("expired keys = %v, %v", keys, err)
	}
	if _, err := store.Get(request.OperationID, strings.Repeat("0", 64)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired replay = %v", err)
	}
	if _, err := store.PurgeExpired(now, MaximumAge+time.Second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlong retention = %v", err)
	}
}

func TestPurgeBoundsOrphanByRequestAge(t *testing.T) {
	root := canonicalRoot(t)
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: root}
	now := time.Now().UTC()
	request := fixture(now.Add(-MaximumAge + time.Hour))
	if _, err := store.Put(request); err != nil {
		t.Fatal(err)
	}
	if keys, err := store.PurgeExpired(now, MaximumAge); err != nil || len(keys) != 0 {
		t.Fatalf("early purge = %v, %v", keys, err)
	}
	if keys, err := store.PurgeExpired(now.Add(time.Hour), MaximumAge); err != nil || len(keys) != 1 || keys[0] != request.OperationID {
		t.Fatalf("orphan retention = %v, %v", keys, err)
	}
}

func TestPurgeContinuesPastPartialCrashOrphan(t *testing.T) {
	root := canonicalRoot(t)
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: root}
	now := time.Now().UTC()
	partial := filepath.Join(root, "attempt_0.json")
	if err := os.WriteFile(partial, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	request := fixture(now.Add(-MaximumAge))
	if _, err := store.Put(request); err != nil {
		t.Fatal(err)
	}
	keys, err := store.PurgeExpired(now, MaximumAge)
	if err != nil || len(keys) != 1 || keys[0] != request.OperationID {
		t.Fatalf("partial orphan blocked expiry = %v, %v", keys, err)
	}
	if err := os.Chtimes(partial, now.Add(-MaximumAge), now.Add(-MaximumAge)); err != nil {
		t.Fatal(err)
	}
	keys, err = store.PurgeExpired(now, MaximumAge)
	if err != nil || len(keys) != 1 || keys[0] != "attempt_0" {
		t.Fatalf("partial orphan expiry = %v, %v", keys, err)
	}
}
