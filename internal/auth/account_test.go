package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type accountMemory struct {
	account PasswordAccount
	failure error
}

func (r *accountMemory) PasswordAccount(context.Context) (PasswordAccount, error) {
	if r.failure != nil {
		return PasswordAccount{}, r.failure
	}
	if r.account.SubjectID == "" {
		return PasswordAccount{}, ErrNotConfigured
	}
	return r.account, nil
}

func (r *accountMemory) InitializePasswordAccount(_ context.Context, account PasswordAccount, _ time.Time) (string, error) {
	if r.failure != nil {
		return "", r.failure
	}
	if r.account.SubjectID != "" {
		return "", ErrAlreadyConfigured
	}
	r.account = account
	return account.SubjectID, nil
}

func setupGrant(t *testing.T, service *Service) *LocalSetupGrant {
	t.Helper()
	grant, err := service.GrantLocalSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func TestFirstRunSetupAndSavedIdentity(t *testing.T) {
	repository := &accountMemory{}
	service := NewService(repository)
	grant := setupGrant(t, service)
	password := []byte("한글 암호 😀 exact  spaces")
	defer clear(password)
	subject, err := service.FirstRunSetup(t.Context(), grant, password)
	if err != nil || subject == "" || !ValidPasswordHash(repository.account.Hash) || strings.Contains(repository.account.Hash, string(password)) {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := service.FirstRunSetup(t.Context(), grant, password); !errors.Is(err, ErrSetupDenied) {
		t.Fatalf("grant reuse: %v", err)
	}
	if _, err := service.GrantLocalSetup(t.Context()); !errors.Is(err, ErrAlreadyConfigured) {
		t.Fatalf("second setup grant: %v", err)
	}
	reopened := NewService(repository)
	if got, err := reopened.VerifyPassword(t.Context(), password); err != nil || got != subject {
		t.Fatalf("saved identity: %q %v", got, err)
	}
	if _, err := reopened.VerifyPassword(t.Context(), []byte("한글 암호 😀 exact spaces")); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := NewService(&accountMemory{}).VerifyPassword(t.Context(), password); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("missing account: %v", err)
	}
}

func TestSetupPermissionAndPasswordFailuresDoNotProvision(t *testing.T) {
	for _, kind := range []string{"nil", "zero", "foreign", "expired", "short", "oversized", "invalid-utf8", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			repository := &accountMemory{}
			service := NewService(repository)
			grant := setupGrant(t, service)
			password := []byte("correct long password")
			defer clear(password)
			ctx := t.Context()
			want := ErrSetupDenied
			switch kind {
			case "nil":
				grant = nil
			case "zero":
				grant = &LocalSetupGrant{}
			case "foreign":
				grant = setupGrant(t, NewService(&accountMemory{}))
			case "expired":
				service.now = func() time.Time { return grant.expiresAt }
			case "short":
				password, want = []byte("short"), ErrInvalidPassword
			case "oversized":
				password, want = []byte(strings.Repeat("a", MaxPasswordBytes+1)), ErrInvalidPassword
			case "invalid-utf8":
				password, want = append(password, 0xff), ErrInvalidPassword
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx, want = cancelled, context.Canceled
			}
			if _, err := service.FirstRunSetup(ctx, grant, password); !errors.Is(err, want) {
				t.Fatalf("invalid setup: %v want %v", err, want)
			}
			if repository.account.SubjectID != "" {
				t.Fatal("failed setup persisted an account")
			}
			// A cancelled admission must release its gate when it won the select.
			live, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			service.now = time.Now
			if _, err := service.GrantLocalSetup(live); err != nil {
				t.Fatalf("service stuck after rejection: %v", err)
			}
		})
	}
}

func TestConcurrentSetupConsumesOnlyOneGrant(t *testing.T) {
	repository := &accountMemory{}
	service := NewService(repository)
	grant := setupGrant(t, service)
	results := make(chan error, 8)
	var group sync.WaitGroup
	for range cap(results) {
		group.Go(func() {
			_, err := service.FirstRunSetup(t.Context(), grant, []byte("concurrent setup password"))
			results <- err
		})
	}
	group.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrSetupDenied) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful setup count: %d", succeeded)
	}
}

func TestSetupGrantExpiresDuringHashWork(t *testing.T) {
	repository := &accountMemory{}
	service := NewService(repository)
	grant := setupGrant(t, service)
	reads := 0
	service.now = func() time.Time {
		reads++
		if reads == 1 {
			return grant.expiresAt.Add(-time.Nanosecond)
		}
		return grant.expiresAt
	}
	password := []byte("correct long password")
	defer clear(password)
	if _, err := service.FirstRunSetup(t.Context(), grant, password); err != ErrSetupDenied {
		t.Fatalf("setup after hash-time expiry: %v", err)
	}
	if reads != 2 || grant.used || repository.account.SubjectID != "" {
		t.Fatal("post-hash expiry was not checked before provisioning")
	}
}

func TestDependencyFailuresAreSanitized(t *testing.T) {
	repository := &accountMemory{}
	service := NewService(repository)
	grant := setupGrant(t, service)
	repository.failure = errors.New("private dependency detail")
	if permission, err := service.GrantLocalSetup(t.Context()); permission != nil || err != ErrUnavailable {
		t.Fatalf("dependency error escaped during grant issuance: %v", err)
	}
	if _, err := service.FirstRunSetup(t.Context(), grant, []byte("sensitive long password")); err != ErrUnavailable {
		t.Fatalf("dependency error escaped: %v", err)
	}
	if _, err := service.VerifyPassword(t.Context(), []byte("sensitive long password")); err != ErrUnavailable {
		t.Fatalf("dependency error escaped during verification: %v", err)
	}
	repository.failure = nil
	if _, err := service.FirstRunSetup(t.Context(), grant, []byte("sensitive long password")); err != nil {
		t.Fatalf("failed setup consumed permission: %v", err)
	}
	if _, err := NewService(nil).GrantLocalSetup(t.Context()); err != ErrUnavailable {
		t.Fatalf("missing repository: %v", err)
	}
}
