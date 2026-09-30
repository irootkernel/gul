package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotConfigured      = errors.New("Gul password account is not configured")
	ErrAlreadyConfigured  = errors.New("Gul password account is already configured")
	ErrSetupDenied        = errors.New("Gul local setup permission is unavailable")
	ErrInvalidPassword    = errors.New(fmt.Sprintf("Gul password must contain at least %d characters and at most %d UTF-8 bytes", MinPasswordCharacters, MaxPasswordBytes))
	ErrInvalidCredentials = errors.New("Gul password is incorrect")
	ErrUnavailable        = errors.New("Gul authentication is unavailable")
)

type PasswordAccount struct {
	SubjectID string
	Hash      string
}

// AccountRepository carries encoded hashes, never passwords. State errors use
// package sentinels; implementations may return private dependency errors.
// Service owns sanitization before an error reaches delivery.
type AccountRepository interface {
	PasswordAccount(context.Context) (PasswordAccount, error)
	InitializePasswordAccount(context.Context, PasswordAccount, time.Time) (string, error)
}

// LocalSetupGrant is an in-process capability for the trusted local shell.
// Zero values, grants from another service, expired grants and reuse fail closed.
// It is never a request field, URL, password, Controller capability or session.
type LocalSetupGrant struct {
	owner     *Service
	expiresAt time.Time
	used      bool
}

type Service struct {
	repository AccountRepository
	gate       chan struct{}
	now        func() time.Time
}

func NewService(repository AccountRepository) *Service {
	return &Service{repository: repository, gate: make(chan struct{}, 1), now: time.Now}
}

func (s *Service) enter(ctx context.Context) error {
	if s == nil || s.repository == nil {
		return ErrUnavailable
	}
	select {
	case s.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			s.leave()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) leave() { <-s.gate }

// GrantLocalSetup is called only by trusted host composition after identifying
// the local shell. An HTTP request, Origin or loopback address is not authority
// to call it. E8-T2/T3 own the protected transport and native bootstrap exchange.
func (s *Service) GrantLocalSetup(ctx context.Context) (*LocalSetupGrant, error) {
	if err := s.enter(ctx); err != nil {
		return nil, err
	}
	defer s.leave()
	_, err := s.repository.PasswordAccount(ctx)
	if !errors.Is(err, ErrNotConfigured) {
		if err == nil {
			return nil, ErrAlreadyConfigured
		}
		return nil, ErrUnavailable
	}
	return &LocalSetupGrant{owner: s, expiresAt: s.now().Add(10 * time.Minute)}, nil
}

// FirstRunSetup commits the subject and hash together. The caller retains its
// password buffer and must clear it; no plaintext reaches the repository.
func (s *Service) FirstRunSetup(ctx context.Context, grant *LocalSetupGrant, password []byte) (string, error) {
	if err := s.enter(ctx); err != nil {
		return "", err
	}
	defer s.leave()
	if grant == nil || grant.owner != s || grant.used || !s.now().Before(grant.expiresAt) {
		return "", ErrSetupDenied
	}
	_, err := s.repository.PasswordAccount(ctx)
	if err == nil {
		return "", ErrAlreadyConfigured
	}
	if !errors.Is(err, ErrNotConfigured) {
		return "", ErrUnavailable
	}
	if !ValidPassword(password) {
		return "", ErrInvalidPassword
	}
	hash := hashPassword(password)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !s.now().Before(grant.expiresAt) {
		return "", ErrSetupDenied
	}
	subject, err := s.repository.InitializePasswordAccount(ctx, PasswordAccount{SubjectID: uuid.NewString(), Hash: hash}, s.now())
	if errors.Is(err, ErrAlreadyConfigured) {
		return "", ErrAlreadyConfigured
	}
	if err != nil {
		return "", ErrUnavailable
	}
	grant.used = true
	return subject, nil
}

// VerifyPassword returns the saved subject, never a browser-selected identity.
// E8-T2 adds rate limiting and server-side sessions around this bounded check.
func (s *Service) VerifyPassword(ctx context.Context, password []byte) (string, error) {
	if err := s.enter(ctx); err != nil {
		return "", err
	}
	defer s.leave()
	account, err := s.repository.PasswordAccount(ctx)
	if errors.Is(err, ErrNotConfigured) {
		return "", ErrInvalidCredentials
	}
	if err != nil || account.SubjectID == "" || !ValidPasswordHash(account.Hash) {
		return "", ErrUnavailable
	}
	if !verifyPassword(password, account.Hash) {
		return "", ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return account.SubjectID, nil
}

// AccountConfigured reports only the saved setup state, with no credential data.
func (s *Service) AccountConfigured(ctx context.Context) (bool, error) {
	if err := s.enter(ctx); err != nil {
		return false, err
	}
	defer s.leave()
	account, err := s.repository.PasswordAccount(ctx)
	if errors.Is(err, ErrNotConfigured) {
		return false, nil
	}
	if err != nil || account.SubjectID == "" || !ValidPasswordHash(account.Hash) {
		return false, ErrUnavailable
	}
	return true, nil
}
