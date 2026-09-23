package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPageSizeBounds(t *testing.T) {
	for _, test := range []struct{ requested, want uint32 }{{0, 50}, {1, 1}, {100, 100}} {
		actual, err := BoundPageSize(test.requested)
		if err != nil || actual != test.want {
			t.Fatalf("page size %d = %d, %v", test.requested, actual, err)
		}
	}
	if _, err := BoundPageSize(101); !errors.Is(err, ErrInvalidPageSize) {
		t.Fatalf("page size 101 error = %v", err)
	}
}

func TestPageTokensBindAccountSessionQueryAndSnapshot(t *testing.T) {
	store := NewPageTokens()
	now := time.Unix(1_000, 0)
	binding := PageBinding{AccountID: "account-1", SessionID: "session-1", Query: PromptHistoryQuery, ProjectionVersion: 2}
	scope := PageScope{PageBinding: binding, SnapshotID: "snapshot-1"}
	position := PagePosition{ProviderCursor: "private-provider-cursor", CapturedHead: "private-provider-head", ScannedCount: 4}
	token, err := store.Issue(scope, position, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, position.ProviderCursor) || strings.Contains(token, position.CapturedHead) || len(token) > MaximumTokenBytes {
		t.Fatalf("browser token leaks provider state: %q", token)
	}
	actual, err := store.Resolve(binding, token, now)
	if err != nil || actual != (ResolvedPage{SnapshotID: "snapshot-1", Position: position}) {
		t.Fatalf("resolved position = %+v, %v", actual, err)
	}
	for _, mutate := range []func(*PageBinding){
		func(s *PageBinding) { s.AccountID = "other" },
		func(s *PageBinding) { s.SessionID = "other" },
		func(s *PageBinding) { s.Query = SpecialistResultsQuery },
		func(s *PageBinding) { s.ProjectionVersion++ },
	} {
		other := binding
		mutate(&other)
		if _, err := store.Resolve(other, token, now); !errors.Is(err, ErrInvalidPageToken) {
			t.Fatalf("foreign scope error = %v", err)
		}
	}
	if _, err := store.Resolve(binding, token, now.Add(time.Minute)); !errors.Is(err, ErrPageTokenExpired) {
		t.Fatalf("expired token error = %v", err)
	}
	if _, err := NewPageTokens().Resolve(binding, token, now); !errors.Is(err, ErrPageTokenExpired) {
		t.Fatalf("restart-invalidated token error = %v", err)
	}
	if _, err := store.Resolve(binding, "private-provider-cursor", now); !errors.Is(err, ErrInvalidPageToken) {
		t.Fatalf("raw provider cursor error = %v", err)
	}
}

func TestPageTokenCapacityAndExpirySweep(t *testing.T) {
	store := NewPageTokens()
	now := time.Unix(1_000, 0)
	scope := PageScope{PageBinding: PageBinding{AccountID: "account", SessionID: "session", Query: PromptHistoryQuery, ProjectionVersion: 1}, SnapshotID: "snapshot"}
	if _, err := store.Issue(scope, PagePosition{}, now, now); !errors.Is(err, ErrInvalidPageToken) {
		t.Fatalf("nonfuture expiry = %v", err)
	}
	for range MaximumLivePageTokens {
		if _, err := store.Issue(scope, PagePosition{}, now, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Issue(scope, PagePosition{}, now, now.Add(time.Second)); !errors.Is(err, ErrPageTokenCapacity) {
		t.Fatalf("capacity error = %v", err)
	}
	if _, err := store.Issue(scope, PagePosition{}, now.Add(time.Second), now.Add(time.Minute)); err != nil {
		t.Fatalf("expired handles were not swept: %v", err)
	}
}

func TestPageTokensRejectInvalidScope(t *testing.T) {
	valid := PageScope{PageBinding: PageBinding{AccountID: "account", SessionID: "session", Query: PromptHistoryQuery, ProjectionVersion: 1}, SnapshotID: "snapshot"}
	for _, test := range []struct {
		name   string
		mutate func(*PageScope)
	}{
		{"account", func(scope *PageScope) { scope.AccountID = "" }},
		{"session", func(scope *PageScope) { scope.SessionID = "" }},
		{"query", func(scope *PageScope) { scope.Query = 0 }},
		{"projection", func(scope *PageScope) { scope.ProjectionVersion = 0 }},
		{"snapshot", func(scope *PageScope) { scope.SnapshotID = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope := valid
			test.mutate(&scope)
			token, err := NewPageTokens().Issue(scope, PagePosition{}, time.Unix(1_000, 0), time.Unix(1_060, 0))
			if token != "" || !errors.Is(err, ErrInvalidPageToken) {
				t.Fatalf("invalid scope issued token %q: %v", token, err)
			}
		})
	}
}
