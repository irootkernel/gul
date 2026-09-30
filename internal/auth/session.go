package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const SessionLifetime = 7 * 24 * time.Hour

var ErrInvalidSession = errors.New("Gul session is unavailable")

type Session struct {
	SubjectID string
	ExpiresAt time.Time
}

// SessionRepository persists only digests. Missing, expired and revoked rows
// return ErrInvalidSession; lookup must require the sole saved password account
// and its valid bounded hash. Private dependency errors are sanitized here.
type SessionRepository interface {
	IssueSession(context.Context, string, string, time.Time) error
	LookupSession(context.Context, string, time.Time) (Session, error)
	RevokeSession(context.Context, string, time.Time) error
}

type Credentials struct {
	Token   string
	CSRF    string
	Session Session
}

type sessionBinding struct {
	owner   *Sessions
	digest  string
	session Session
}

type sessionContextKey struct{}

// Sessions owns bearer creation and request cancellation, never browser subject
// selection. Hosts share one instance with all handlers and authorization ports.
type Sessions struct {
	repository SessionRepository
	now        func() time.Time
	mu         sync.Mutex
	active     map[*sessionBinding]context.CancelFunc
}

func NewSessions(repository SessionRepository) *Sessions {
	return &Sessions{repository: repository, now: time.Now, active: make(map[*sessionBinding]context.CancelFunc)}
}

func NewSecret() string {
	value := make([]byte, 32)
	rand.Read(value)
	defer clear(value)
	return base64.RawURLEncoding.EncodeToString(value)
}

func ValidSecret(token string) bool {
	if len(token) != 43 {
		return false
	}
	value, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(value) == 32 && base64.RawURLEncoding.EncodeToString(value) == token
}

func SecretDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// CSRFToken is domain-separated from the bearer and cannot authenticate a call.
// Only a same-origin session read exposes it to JavaScript; it is not persisted.
func CSRFToken(token string) string {
	digest := sha256.Sum256([]byte("gul.csrf/v1\x00" + token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (s *Sessions) Issue(ctx context.Context, subject string) (Credentials, error) {
	if s == nil || s.repository == nil || subject == "" {
		return Credentials{}, ErrUnavailable
	}
	result := Credentials{Token: NewSecret(), Session: Session{SubjectID: subject, ExpiresAt: s.now().Add(SessionLifetime)}}
	result.CSRF = CSRFToken(result.Token)
	if err := s.repository.IssueSession(ctx, SecretDigest(result.Token), subject, result.Session.ExpiresAt); err != nil {
		return Credentials{}, ErrUnavailable
	}
	return result, nil
}

func (s *Sessions) lookup(ctx context.Context, digest string) (Session, error) {
	if s == nil || s.repository == nil {
		return Session{}, ErrUnavailable
	}
	result, err := s.repository.LookupSession(ctx, digest, s.now())
	if errors.Is(err, ErrInvalidSession) {
		return Session{}, ErrInvalidSession
	}
	if err != nil || result.SubjectID == "" || !s.now().Before(result.ExpiresAt) {
		return Session{}, ErrUnavailable
	}
	return result, nil
}

func (s *Sessions) Inspect(ctx context.Context, token string) (Session, error) {
	if !ValidSecret(token) {
		return Session{}, ErrInvalidSession
	}
	return s.lookup(ctx, SecretDigest(token))
}

// Bind bounds all calls by session expiry. Streams also observe durable
// revocation every 250ms, including revocation through another repository.
func (s *Sessions) Bind(ctx context.Context, token string, stream bool) (context.Context, func(), error) {
	session, err := s.Inspect(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	binding := &sessionBinding{owner: s, digest: SecretDigest(token), session: session}
	bound, cancel := context.WithTimeout(ctx, session.ExpiresAt.Sub(s.now()))
	s.mu.Lock()
	if len(s.active) >= 128 {
		s.mu.Unlock()
		cancel()
		return nil, nil, ErrUnavailable
	}
	s.active[binding] = cancel
	s.mu.Unlock()
	cleanup := func() { cancel(); s.mu.Lock(); delete(s.active, binding); s.mu.Unlock() }
	bound = context.WithValue(bound, sessionContextKey{}, binding)
	if _, err := s.Current(bound); err != nil {
		cleanup()
		return nil, nil, err
	}
	if stream {
		go func() {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-bound.Done():
					return
				case <-ticker.C:
					if _, err := s.Current(bound); err != nil {
						cancel()
						return
					}
				}
			}
		}()
	}
	return bound, cleanup, nil
}

func (s *Sessions) Current(ctx context.Context) (Session, error) {
	if ctx.Err() != nil {
		return Session{}, ErrInvalidSession
	}
	binding, ok := ctx.Value(sessionContextKey{}).(*sessionBinding)
	if !ok || binding.owner != s {
		return Session{}, ErrInvalidSession
	}
	current, err := s.lookup(ctx, binding.digest)
	if err != nil {
		return Session{}, err
	}
	if current != binding.session {
		return Session{}, ErrInvalidSession
	}
	return current, nil
}

func (s *Sessions) Revoke(ctx context.Context, token string) error {
	if s == nil || s.repository == nil {
		return ErrUnavailable
	}
	if !ValidSecret(token) {
		return ErrInvalidSession
	}
	digest := SecretDigest(token)
	if err := s.repository.RevokeSession(ctx, digest, s.now()); err != nil && !errors.Is(err, ErrInvalidSession) {
		return ErrUnavailable
	}
	s.mu.Lock()
	for binding, cancel := range s.active {
		if binding.digest == digest {
			cancel()
		}
	}
	s.mu.Unlock()
	return nil
}
