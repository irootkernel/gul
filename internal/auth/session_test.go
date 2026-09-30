package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type sessionMemory struct {
	mu      sync.Mutex
	rows    map[string]Session
	failure error
}

func (r *sessionMemory) IssueSession(_ context.Context, digest, subject string, expires time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		return r.failure
	}
	r.rows[digest] = Session{subject, expires}
	return nil
}
func (r *sessionMemory) LookupSession(_ context.Context, digest string, now time.Time) (Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		return Session{}, r.failure
	}
	row, ok := r.rows[digest]
	if !ok || !now.Before(row.ExpiresAt) {
		return Session{}, ErrInvalidSession
	}
	return row, nil
}
func (r *sessionMemory) RevokeSession(_ context.Context, digest string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		return r.failure
	}
	if _, ok := r.rows[digest]; !ok {
		return ErrInvalidSession
	}
	delete(r.rows, digest)
	return nil
}

func TestSessionCreationExpiryAndContextAuthority(t *testing.T) {
	r := &sessionMemory{rows: make(map[string]Session)}
	s := NewSessions(r)
	at := time.Now()
	s.now = func() time.Time { return at }
	first, err := s.Issue(t.Context(), "saved-owner")
	if err != nil || !ValidSecret(first.Token) || first.CSRF == first.Token || first.Session.ExpiresAt.Sub(at) != SessionLifetime {
		t.Fatal("invalid session issuance", err)
	}
	second, err := s.Issue(t.Context(), "saved-owner")
	if err != nil || first.Token == second.Token {
		t.Fatal("session tokens reused", err)
	}
	if _, ok := r.rows[first.Token]; ok {
		t.Fatal("bearer persisted")
	}
	if row := r.rows[SecretDigest(first.Token)]; row != first.Session {
		t.Fatal("digest storage differs")
	}
	bound, release, err := s.Bind(t.Context(), first.Token, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if current, err := s.Current(bound); err != nil || current.SubjectID != "saved-owner" {
		t.Fatal("context subject", err)
	}
	if _, err := NewSessions(r).Current(bound); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("foreign manager trusted", err)
	}
	if _, err := s.Current(t.Context()); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("unbound context trusted", err)
	}
	at = at.Add(SessionLifetime)
	if _, err := s.Inspect(t.Context(), first.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("expired session trusted", err)
	}
	if _, err := s.Inspect(t.Context(), first.CSRF); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("CSRF authenticated", err)
	}
}

func TestSessionRevocationCancelsAndObservesDurableChanges(t *testing.T) {
	for _, kind := range []string{"same-manager", "other-manager", "expiry", "dependency-failure"} {
		t.Run(kind, func(t *testing.T) {
			r := &sessionMemory{rows: make(map[string]Session)}
			s := NewSessions(r)
			credentials, err := s.Issue(t.Context(), "owner")
			if err != nil {
				t.Fatal(err)
			}
			bound, release, err := s.Bind(t.Context(), credentials.Token, true)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			switch kind {
			case "same-manager":
				err = s.Revoke(t.Context(), credentials.Token)
			case "other-manager":
				err = NewSessions(r).Revoke(t.Context(), credentials.Token)
			case "expiry":
				r.mu.Lock()
				row := r.rows[SecretDigest(credentials.Token)]
				row.ExpiresAt = time.Now().Add(-time.Second)
				r.rows[SecretDigest(credentials.Token)] = row
				r.mu.Unlock()
			case "dependency-failure":
				r.mu.Lock()
				r.failure = errors.New("private detail")
				r.mu.Unlock()
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-bound.Done():
			case <-time.After(time.Second):
				t.Fatal("live request survived revocation")
			}
			if _, err := s.Current(bound); err == nil {
				t.Fatal("cancelled principal survived")
			}
		})
	}
}

func TestSessionFailuresAndBoundedRequests(t *testing.T) {
	r := &sessionMemory{rows: make(map[string]Session)}
	s := NewSessions(r)
	for _, token := range []string{"", NewSecret() + "=", "invalid", NewSecret()} {
		if _, err := s.Inspect(t.Context(), token); !errors.Is(err, ErrInvalidSession) {
			t.Fatal(err)
		}
	}
	credentials, err := s.Issue(t.Context(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for range 128 {
		_, release, err := s.Bind(t.Context(), credentials.Token, false)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	if _, _, err := s.Bind(t.Context(), credentials.Token, false); err != ErrUnavailable {
		t.Fatal("unbounded active calls", err)
	}
	r.failure = errors.New("private dependency")
	if _, err := s.Issue(t.Context(), "owner"); err != ErrUnavailable {
		t.Fatal("issue error escaped", err)
	}
	if _, err := s.Inspect(t.Context(), credentials.Token); err != ErrUnavailable {
		t.Fatal("lookup error escaped", err)
	}
	if err := s.Revoke(t.Context(), credentials.Token); err != ErrUnavailable {
		t.Fatal("revoke error escaped", err)
	}
}
