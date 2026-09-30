package storage

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/workspace"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "gul.sqlite")
	store, err := Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, filename
}

func TestAuthPresentationAndCache(t *testing.T) {
	store, filename := openTestStore(t)
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	if err := store.Auth().CreateAccount(t.Context(), "account-1", now); err != nil {
		t.Fatal(err)
	}
	tokenSHA256 := strings.Repeat("b", 64)
	if err := store.Auth().IssueSession(t.Context(), tokenSHA256, "account-1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if subject, err := store.Auth().SessionSubject(t.Context(), tokenSHA256, now); err != nil || subject != "account-1" {
		t.Fatalf("session subject = %q, %v", subject, err)
	}
	if _, err := store.Auth().SessionSubject(t.Context(), tokenSHA256, now.Add(time.Hour)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired session = %v", err)
	}
	shortToken := strings.Repeat("c", 64)
	if err := store.Auth().IssueSession(t.Context(), shortToken, "account-1", now.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Auth().SessionSubject(t.Context(), shortToken, now); err != nil {
		t.Fatalf("subsecond active session = %v", err)
	}
	if _, err := store.Auth().SessionSubject(t.Context(), shortToken, now.Add(100*time.Millisecond)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("subsecond expired session = %v", err)
	}
	if err := store.Auth().RevokeSession(t.Context(), tokenSHA256, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Auth().RevokeSession(t.Context(), tokenSHA256, now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("duplicate revocation = %v", err)
	}
	if _, err := store.Auth().SessionSubject(t.Context(), tokenSHA256, now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoked session = %v", err)
	}
	if err := store.Presentation().CreateAttachment(t.Context(), workspace.Attachment{SubjectID: "account-1", ID: "workspace-1", DisplayName: "Workspace", CanonicalRoot: filepath.Join(t.TempDir(), "workspace"), ProviderID: "provider-1", FileDevice: "1", FileInode: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Presentation().SetWorkspaceHidden(t.Context(), "account-1", "workspace-1", true); err != nil {
		t.Fatal(err)
	}
	entry, err := store.Presentation().Workspace(t.Context(), "account-1", "workspace-1")
	if err != nil || !entry.Hidden || entry.DisplayName != "Workspace" {
		t.Fatalf("workspace = %+v, %v", entry, err)
	}
	direct := DirectSessionPresentation{SubjectID: "account-1", SessionID: "direct-1", WorkspaceID: "workspace-1", DisplayName: "Session"}
	if err := store.Presentation().InsertDirectSessionIfAbsent(t.Context(), direct); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Presentation().DirectSession(t.Context(), "account-1", "direct-1"); err != nil || got != direct {
		t.Fatalf("Direct Session = %+v, %v", got, err)
	}
	for _, key := range []string{"/absolute", "../outside", "a/../outside", "."} {
		if err := store.Presentation().PutBinding(t.Context(), BindingReference{BindingID: "binding", SubjectID: "account-1", CredentialKey: key, ExpectedControllerID: "controller", Health: "unknown"}); err == nil {
			t.Errorf("accepted credential key %q", key)
		}
	}
	if err := store.Presentation().PutBinding(t.Context(), BindingReference{BindingID: "binding", SubjectID: "account-1", CredentialKey: "logical/controller", ExpectedControllerID: "controller", Health: "unknown"}); err != nil {
		t.Fatal(err)
	}
	stamp := ProjectionCacheEntry{SubjectID: "account-1", SessionID: "direct-1", AggregateKind: "run", Stamp: ProjectionStamp{CapturedHeadCursor: "stamp-1", RunStateRevision: 7, WriterStateRevision: 4, InteractionStateRevision: 2}, Freshness: "fresh", InvalidationFloor: ProjectionStamp{CapturedHeadCursor: "floor-1", RunStateRevision: 6}}
	if err := store.Cache().PutProjectionStamp(t.Context(), stamp); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"writer", "interaction"} {
		other := stamp
		other.AggregateKind = kind
		if err := store.Cache().PutProjectionStamp(t.Context(), other); err != nil {
			t.Fatal(err)
		}
	}
	providerCursor := UpstreamCursor{Value: "provider-cursor-3"}
	checkpoint := ObservationCheckpoint{ProviderID: "dolgorae", RuntimeObjectID: "runtime-1", StreamKind: "run", LastValidatedCursor: "cursor-2", LastCommittedCursor: "cursor-1"}
	if err := store.Cache().PutCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := store.Cache().PutTimelineHead(t.Context(), "account-1", "direct-1", providerCursor); err != nil {
		t.Fatal(err)
	}
	store.Close()
	reopened, err := Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Cache().ProjectionStamp(t.Context(), "account-1", "direct-1", "run")
	if err != nil || got.Freshness != "stale" || got.Stamp != stamp.Stamp || got.InvalidationFloor != stamp.InvalidationFloor {
		t.Fatalf("startup cache = %+v, %v", got, err)
	}
	for _, kind := range []string{"writer", "interaction"} {
		if other, err := reopened.Cache().ProjectionStamp(t.Context(), "account-1", "direct-1", kind); err != nil || other.Freshness != "stale" || other.Stamp != stamp.Stamp {
			t.Fatalf("retained %s cache = %+v, %v", kind, other, err)
		}
	}
	if got, err := reopened.Cache().Checkpoint(t.Context(), "dolgorae", "runtime-1", "run"); err != nil || got != checkpoint {
		t.Fatalf("retained checkpoint = %+v, %v", got, err)
	}
	if cursor, err := reopened.Cache().TimelineHead(t.Context(), "account-1", "direct-1"); err != nil || cursor != providerCursor {
		t.Fatalf("timeline cursor = %v, %v", cursor, err)
	}
}

func TestAuthRejectsNonDigestTokens(t *testing.T) {
	store, _ := openTestStore(t)
	for _, token := range []string{"plain bearer token", strings.Repeat("x", 64), strings.Repeat("A", 64)} {
		if err := store.Auth().IssueSession(t.Context(), token, "account-1", time.Now().Add(time.Hour)); err == nil {
			t.Errorf("issued session for non-digest token %q", token)
		}
		if _, err := store.Auth().SessionSubject(t.Context(), token, time.Now()); err == nil {
			t.Errorf("looked up non-digest token %q", token)
		}
		if err := store.Auth().RevokeSession(t.Context(), token, time.Now()); err == nil {
			t.Errorf("revoked non-digest token %q", token)
		}
	}
}

func TestPresentationWritesPreserveSessionMetadata(t *testing.T) {
	store, _ := openTestStore(t)
	ctx := t.Context()
	if err := store.Auth().CreateAccount(ctx, "subject", time.Now()); err != nil {
		t.Fatal(err)
	}
	workspace := WorkspaceEntry{SubjectID: "subject", WorkspaceID: "workspace", DisplayName: "Before"}
	if err := store.Presentation().RenameWorkspace(ctx, workspace.SubjectID, workspace.WorkspaceID, workspace.DisplayName); !errors.Is(err, presentation.ErrNotFound) {
		t.Fatalf("unattached presentation update = %v", err)
	}
	if err := store.Presentation().CreateAttachment(ctx, workspaceAttachment(workspace, filepath.Join(t.TempDir(), "workspace"))); err != nil {
		t.Fatal(err)
	}
	workspace.DisplayName, workspace.Hidden = "After", true
	if err := store.Presentation().RenameWorkspace(ctx, workspace.SubjectID, workspace.WorkspaceID, workspace.DisplayName); err != nil {
		t.Fatal(err)
	}
	if err := store.Presentation().SetWorkspaceHidden(ctx, workspace.SubjectID, workspace.WorkspaceID, true); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Presentation().Workspace(ctx, "subject", "workspace"); err != nil || got != workspace {
		t.Fatalf("updated workspace = %+v, %v", got, err)
	}
	direct := DirectSessionPresentation{SubjectID: "subject", SessionID: "session", WorkspaceID: "workspace", DisplayName: "Before"}
	if err := store.Presentation().InsertDirectSessionIfAbsent(ctx, direct); err != nil {
		t.Fatal(err)
	}
	direct.WorkspaceID, direct.DisplayName, direct.Archived = "workspace-updated", "After", true
	if err := store.Presentation().InsertDirectSessionIfAbsent(ctx, direct); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Presentation().DirectSession(ctx, "subject", "session"); err != nil || got != (DirectSessionPresentation{SubjectID: "subject", SessionID: "session", WorkspaceID: "workspace", DisplayName: "Before"}) {
		t.Fatalf("existing Direct Session presentation replaced = %+v, %v", got, err)
	}
	binding := BindingReference{BindingID: "binding", SubjectID: "subject", CredentialKey: "controller/first", ExpectedControllerID: "controller-1", Health: "unknown"}
	if err := store.Presentation().PutBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}
	binding.CredentialKey, binding.ExpectedControllerID, binding.Health = "controller/second", "controller-2", "healthy"
	if err := store.Presentation().PutBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}
	var key, controller, health string
	if err := store.reader.QueryRowContext(ctx, "SELECT credential_key, expected_controller_id, health FROM controller_binding_references WHERE binding_id = ?", binding.BindingID).Scan(&key, &controller, &health); err != nil || key != binding.CredentialKey || controller != binding.ExpectedControllerID || health != binding.Health {
		t.Fatalf("updated binding = %q, %q, %q, %v", key, controller, health, err)
	}
	seedSubjectFixture(t, store, "foreign")
	binding.SubjectID = "foreign"
	if err := store.Presentation().PutBinding(ctx, binding); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("binding reassignment = %v", err)
	}
	stamp := ProjectionCacheEntry{SubjectID: "subject", SessionID: "session", AggregateKind: "run", Stamp: ProjectionStamp{CapturedHeadCursor: "first", RunStateRevision: 1}, InvalidationFloor: ProjectionStamp{CapturedHeadCursor: "first"}, Freshness: "stale"}
	if err := store.Cache().PutProjectionStamp(ctx, stamp); err != nil {
		t.Fatal(err)
	}
	stamp.Stamp.CapturedHeadCursor, stamp.Stamp.RunStateRevision, stamp.Freshness = "second", 2, "fresh"
	stamp.InvalidationFloor = ProjectionStamp{CapturedHeadCursor: "second", RunStateRevision: 2}
	if err := store.Cache().PutProjectionStamp(ctx, stamp); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Cache().ProjectionStamp(ctx, "subject", "session", "run"); err != nil || got != stamp {
		t.Fatalf("updated stamp = %+v, %v", got, err)
	}
	if err := store.Cache().PutTimelineHead(ctx, "subject", "session", UpstreamCursor{Value: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Cache().PutTimelineHead(ctx, "subject", "session", UpstreamCursor{Value: "second"}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Cache().TimelineHead(ctx, "subject", "session"); err != nil || got.Value != "second" {
		t.Fatalf("updated timeline head = %+v, %v", got, err)
	}
	checkpoint := ObservationCheckpoint{ProviderID: "provider", RuntimeObjectID: "object", StreamKind: "run", LastValidatedCursor: "first", LastCommittedCursor: "first"}
	if err := store.Cache().PutCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	checkpoint.LastValidatedCursor, checkpoint.LastCommittedCursor = "third", "second"
	if err := store.Cache().PutCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Cache().Checkpoint(ctx, "provider", "object", "run"); err != nil || got != checkpoint {
		t.Fatalf("updated checkpoint = %+v, %v", got, err)
	}
}

func workspaceAttachment(entry WorkspaceEntry, root string) workspace.Attachment {
	return workspace.Attachment{SubjectID: entry.SubjectID, ID: entry.WorkspaceID, DisplayName: entry.DisplayName, CanonicalRoot: root, ProviderID: "provider-1", FileDevice: "1", FileInode: "1"}
}

func TestAttemptsRejectUnsafeReferencesAndDeliveryAllocatesAtomically(t *testing.T) {
	store, filename := openTestStore(t)
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	if err := store.Auth().CreateAccount(t.Context(), "account-1", now); err != nil {
		t.Fatal(err)
	}
	attempt := OperationAttempt{OperationID: "op-1", SubjectID: "account-1", Kind: "StartRun", RequestSHA256: strings.Repeat("a", 64), ReplayKey: "replay/op-1", ReplayAvailable: true, ControllerReferences: []ControllerReference{{Role: "source", CredentialKey: "controller/key-1", ExpectedControllerID: "controller-1"}}, State: "pending", CreatedAt: now}
	if err := store.Attempts().Record(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	if recorded, err := store.Attempts().Get(t.Context(), attempt.OperationID); err != nil || len(recorded.ControllerReferences) != 1 || recorded.ControllerReferences[0] != attempt.ControllerReferences[0] || !recorded.ReplayAvailable {
		t.Fatalf("recorded Controller references = %+v, %v", recorded, err)
	}
	if err := store.Attempts().MarkOutcomeUnknown(t.Context(), "op-1"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Attempts().Get(t.Context(), "op-1")
	if err != nil || got.State != "outcome_unknown" || got.ReplayKey != "replay/op-1" {
		t.Fatalf("attempt = %+v, %v", got, err)
	}
	if err := store.Attempts().MarkResolved(t.Context(), "op-1"); err != nil {
		t.Fatal(err)
	}
	if resolved, err := store.Attempts().Get(t.Context(), "op-1"); err != nil || resolved.State != "resolved" || resolved.ReplayKey != "" || resolved.ReplayAvailable {
		t.Fatalf("resolved attempt = %+v, %v", resolved, err)
	}
	if err := store.Attempts().MarkResolved(t.Context(), "op-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("duplicate resolution = %v", err)
	}
	attempt.OperationID = "op-expired"
	if err := store.Attempts().Record(t.Context(), attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.Attempts().MarkOutcomeUnknown(t.Context(), attempt.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := store.Attempts().ExpireReplay(t.Context(), attempt.OperationID); err != nil {
		t.Fatal(err)
	}
	if expired, err := store.Attempts().Get(t.Context(), attempt.OperationID); err != nil || expired.ReplayAvailable || expired.ReplayKey != "" || expired.State != "outcome_unknown" {
		t.Fatalf("expired replay = %+v, %v", expired, err)
	}
	if err := store.Attempts().ExpireReplay(t.Context(), attempt.OperationID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("duplicate replay expiry = %v", err)
	}
	attempt.OperationID = "op-2"
	attempt.Kind = "SubmitTurn"
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("non-StartRun replay accepted")
	}
	attempt.Kind = "StartRun"
	attempt.ControllerReferences[0].CredentialKey = "/private/carrier"
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("absolute credential path accepted")
	}
	attempt.ControllerReferences[0].CredentialKey = ""
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("replay without logical Controller reference accepted")
	}
	attempt.ControllerReferences[0].CredentialKey = "controller/key-1"
	attempt.ControllerReferences[0].Role = "unclassified"
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("unclassified Controller role accepted")
	}
	attempt.ControllerReferences[0].Role = "source"
	attempt.ControllerReferences = append(attempt.ControllerReferences, attempt.ControllerReferences[0])
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("duplicate Controller role accepted")
	}
	attempt.ControllerReferences = attempt.ControllerReferences[:1]
	attempt.ControllerReferences[0].BindingID = "binding-1"
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("ambiguous Controller reference accepted")
	}
	attempt.ControllerReferences[0].BindingID = ""
	attempt.ReplayAvailable = false
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("replay key without replay availability accepted")
	}
	attempt.ReplayAvailable = true
	attempt.Kind = "prompt-canary"
	if err := store.Attempts().Record(t.Context(), attempt); err == nil {
		t.Fatal("unrecognized operation kind accepted")
	}
	if _, err := store.Delivery().Append(t.Context(), "foreign-account", "changed", now); err == nil {
		t.Fatal("foreign-key violating delivery accepted")
	}
	const count = 20
	results := make(chan int64, count)
	errors := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			sequence, err := store.Delivery().Append(t.Context(), "account-1", "changed", now)
			results <- sequence
			errors <- err
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	seen := map[int64]bool{}
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for sequence := range results {
		if sequence < 1 || sequence > count || seen[sequence] {
			t.Fatalf("invalid sequence %d", sequence)
		}
		seen[sequence] = true
	}
	if len(seen) != count {
		t.Fatalf("allocated %d sequences", len(seen))
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Rejected carrier paths and unrecognized operation kinds never reach SQLite.
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/private/carrier", "prompt-canary"} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("database contains forbidden canary %q", forbidden)
		}
	}
}
