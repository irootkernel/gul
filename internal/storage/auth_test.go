package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/auth"
)

// Subject-isolation tests deliberately seed adversarial legacy rows without
// using the one-account provisioning port. This is never product composition.
func seedSubjectFixture(t *testing.T, store *Store, subject string) {
	t.Helper()
	if _, err := store.writer.ExecContext(t.Context(), "INSERT INTO app_account(subject_id,created_at) VALUES (?,?)", subject, timestamp(time.Now())); err != nil {
		t.Fatal(err)
	}
}

func provisionPassword(t *testing.T, store *Store, password []byte) string {
	t.Helper()
	service := auth.NewService(store.Auth())
	grant, err := service.GrantLocalSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	subject, err := service.FirstRunSetup(t.Context(), grant, password)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func TestPasswordAccountReopenAndBackup(t *testing.T) {
	store, filename := openTestStore(t)
	password := []byte("unique password 한글 😀")
	defer clear(password)
	subject := provisionPassword(t, store, password)
	if err := store.Auth().CreateAccount(t.Context(), "second", time.Now()); !errors.Is(err, auth.ErrAlreadyConfigured) {
		t.Fatalf("second account provisioned: %v", err)
	}
	backup := filepath.Join(filepath.Dir(filename), "backup.sqlite")
	if err := store.Backup(t.Context(), backup); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filename, backup} {
		reopened, err := Open(t.Context(), name)
		if err != nil {
			t.Fatal(err)
		}
		service := auth.NewService(reopened.Auth())
		if got, err := service.VerifyPassword(t.Context(), password); err != nil || got != subject {
			t.Fatalf("restored account: %q %v", got, err)
		}
		if _, err := service.GrantLocalSetup(t.Context()); !errors.Is(err, auth.ErrAlreadyConfigured) {
			t.Fatalf("restored account reopened setup: %v", err)
		}
		reopened.Close()
		data, err := os.ReadFile(name)
		if err != nil || bytes.Contains(data, password) {
			t.Fatalf("plaintext password persisted: %v", err)
		}
	}
}

func TestPasswordSetupAtomicRollbackAndLegacyIdentity(t *testing.T) {
	store, _ := openTestStore(t)
	service := auth.NewService(store.Auth())
	grant, err := service.GrantLocalSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Force failure after subject insertion but before the credential write.
	if _, err := store.writer.Exec(`CREATE TRIGGER reject_password BEFORE INSERT ON password_account BEGIN SELECT RAISE(ABORT,'private database detail'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.FirstRunSetup(t.Context(), grant, []byte("atomic setup password")); err != auth.ErrUnavailable {
		t.Fatalf("write failure escaped: %v", err)
	}
	var subjects, credentials int
	if err := store.reader.QueryRow("SELECT COUNT(*) FROM app_account").Scan(&subjects); err != nil {
		t.Fatal(err)
	}
	if err := store.reader.QueryRow("SELECT COUNT(*) FROM password_account").Scan(&credentials); err != nil || subjects != 0 || credentials != 0 {
		t.Fatalf("partial setup: %d %d %v", subjects, credentials, err)
	}
	if _, err := store.writer.Exec("DROP TRIGGER reject_password"); err != nil {
		t.Fatal(err)
	}
	if err := store.Auth().CreateAccount(t.Context(), "stable-foundation-subject", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := service.FirstRunSetup(t.Context(), grant, []byte("atomic setup password")); err != nil || got != "stable-foundation-subject" {
		t.Fatalf("foundation identity changed: %q %v", got, err)
	}
}

func TestConcurrentPasswordSetupAcrossRepositories(t *testing.T) {
	store, filename := openTestStore(t)
	other, err := Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	first, second := auth.NewService(store.Auth()), auth.NewService(other.Auth())
	firstGrant, err := first.GrantLocalSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	secondGrant, err := second.GrantLocalSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, attempt := range []struct {
		service *auth.Service
		grant   *auth.LocalSetupGrant
	}{{first, firstGrant}, {second, secondGrant}} {
		group.Go(func() {
			_, err := attempt.service.FirstRunSetup(t.Context(), attempt.grant, []byte("concurrent saved password"))
			results <- err
		})
	}
	group.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, auth.ErrAlreadyConfigured) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent successful setups: %d", succeeded)
	}
}

func TestPasswordAccountRejectsAmbiguousOrDamagedData(t *testing.T) {
	for _, kind := range []string{"multiple-legacy-subjects", "damaged-hash", "second-subject-after-setup"} {
		t.Run(kind, func(t *testing.T) {
			store, _ := openTestStore(t)
			if kind == "multiple-legacy-subjects" {
				seedSubjectFixture(t, store, "one")
				seedSubjectFixture(t, store, "two")
				service := auth.NewService(store.Auth())
				grant, err := service.GrantLocalSetup(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := service.FirstRunSetup(t.Context(), grant, []byte("ambiguous setup password")); err != auth.ErrUnavailable {
					t.Fatalf("ambiguous data provisioned: %v", err)
				}
				return
			}
			provisionPassword(t, store, []byte("saved account password"))
			if kind == "damaged-hash" {
				if _, err := store.writer.Exec("UPDATE password_account SET password_hash='malformed'"); err != nil {
					t.Fatal(err)
				}
			} else {
				seedSubjectFixture(t, store, "second")
			}
			if _, err := auth.NewService(store.Auth()).VerifyPassword(t.Context(), []byte("saved account password")); err != auth.ErrUnavailable {
				t.Fatalf("damaged account allowed: %v", err)
			}
		})
	}
}
