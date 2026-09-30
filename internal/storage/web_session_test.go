package storage

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/auth"
)

func TestDurableBrowserSessionUsesOnlyDigestAndSavedSubject(t *testing.T) {
	store, filename := openTestStore(t)
	subject := provisionPassword(t, store, []byte("saved browser password"))
	sessions := auth.NewSessions(store.Auth())
	credentials, err := sessions.Issue(t.Context(), subject)
	if err != nil {
		t.Fatal(err)
	}
	var digest string
	if err := store.reader.QueryRow("SELECT token_sha256 FROM web_sessions").Scan(&digest); err != nil || digest != auth.SecretDigest(credentials.Token) {
		t.Fatal("session secret persisted", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	other := auth.NewSessions(reopened.Auth())
	if row, err := other.Inspect(t.Context(), credentials.Token); err != nil || row.SubjectID != subject || !row.ExpiresAt.Equal(credentials.Session.ExpiresAt) {
		t.Fatal("session lost on restart", err)
	}
	if _, err := reopened.Auth().LookupSession(t.Context(), digest, credentials.Session.ExpiresAt); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("expiry boundary accepted", err)
	}
	if err := other.Revoke(t.Context(), credentials.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Inspect(t.Context(), credentials.Token); err == nil {
		t.Fatal("closed store authenticated")
	}
	if _, err := other.Inspect(t.Context(), credentials.Token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("durable revocation lost", err)
	}
	if err := other.Revoke(t.Context(), credentials.Token); err != nil {
		t.Fatal("logout not idempotent", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || bytes.Contains(data, []byte(credentials.Token)) || bytes.Contains(data, []byte(credentials.CSRF)) {
		t.Fatal("session bearer/CSRF persisted", err)
	}
}

func TestBrowserSessionRejectsForeignOrDamagedAccount(t *testing.T) {
	for _, kind := range []string{"foreign", "ambiguous", "damaged"} {
		t.Run(kind, func(t *testing.T) {
			store, _ := openTestStore(t)
			subject := provisionPassword(t, store, []byte("saved browser password"))
			token := auth.NewSecret()
			if kind == "foreign" {
				seedSubjectFixture(t, store, "foreign")
				subject = "foreign"
			}
			if err := store.Auth().IssueSession(t.Context(), auth.SecretDigest(token), subject, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if kind == "ambiguous" {
				seedSubjectFixture(t, store, "foreign")
			}
			if kind == "damaged" {
				if _, err := store.writer.Exec("UPDATE password_account SET password_hash='damaged'"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := auth.NewSessions(store.Auth()).Inspect(t.Context(), token); err == nil {
				t.Fatal("unsafe account authenticated")
			}
		})
	}
}
