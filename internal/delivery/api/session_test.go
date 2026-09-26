package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type sessionWorkspace struct {
	entry workspace.Attachment
	err   error
}

func (w *sessionWorkspace) Revalidate(_ context.Context, subject, id string) (workspace.Attachment, error) {
	if w.err != nil {
		return workspace.Attachment{}, w.err
	}
	if subject != w.entry.SubjectID || id != w.entry.ID {
		return workspace.Attachment{}, workspace.ErrAttachmentNotFound
	}
	return w.entry, nil
}

type sessionCarrier struct{ err error }

func (c *sessionCarrier) Resolve(_ context.Context, subject, bindingID string) (session.Carrier, error) {
	if c.err != nil {
		return session.Carrier{}, c.err
	}
	if subject != "owner" || bindingID != "binding" {
		return session.Carrier{}, session.ErrNotFound
	}
	return session.Carrier{AbsolutePath: "/trusted/carrier", ControllerID: "controller", Generation: 1}, nil
}

type sessionProvider struct {
	offline bool
	calls   int
}

func (p *sessionProvider) Snapshot(_ context.Context, attachment workspace.Attachment, runID string, _ session.Carrier) (session.Snapshot, error) {
	p.calls++
	if p.offline {
		return session.Snapshot{}, errors.New("offline")
	}
	return session.Snapshot{ProviderSessionID: "upstream-session", PrimaryRunID: runID,
		ProviderWorkspaceID: attachment.ProviderID, Configuration: session.Configuration{ProfileName: "provider-profile"},
		Lifecycle: "active", Composition: "standalone_primary", ApprovalPolicy: "user_approval_required",
		SpecialistPolicyName: "policy", Counts: session.Counts{NonretiredMembers: 1}, CloseProgress: "none",
		Recovery: "none", AggregateRevision: 3, RunRevision: 4, ObservedAt: time.Now().UTC(),
		Members: []session.Member{{RunID: "private-specialist-id", Lifecycle: "running"}}}, nil
}

func TestExecutionStateHandlerKeepsProviderIdentityPrivateAndUnavailableCountsAbsent(t *testing.T) {
	assertWorkspaceError(t, sessionError(session.ErrPersistenceUnavailable), connect.CodeUnavailable,
		gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	assertWorkspaceError(t, sessionError(session.ErrInvalidProjection), connect.CodeFailedPrecondition,
		gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	assertWorkspaceError(t, workspaceError(workspace.ErrProviderUnavailable), connect.CodeUnavailable,
		gulv1.ErrorCode_ERROR_CODE_TRANSPORT_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT)
	ctx := t.Context()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(ctx, filepath.Join(dir, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Auth().CreateAccount(ctx, "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	entry := workspace.Attachment{SubjectID: "owner", ID: "workspace", CanonicalRoot: "/workspace",
		ProviderID: "upstream-workspace", FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}
	if err := store.Presentation().CreateAttachment(ctx, entry); err != nil {
		t.Fatal(err)
	}
	provider := &sessionProvider{}
	workspacePort := &sessionWorkspace{entry: entry}
	carrierPort := &sessionCarrier{}
	svc := session.NewService(workspacePort, store.Presentation(), provider, carrierPort)
	binding, err := svc.BindPrimary(ctx, "owner", "workspace", "upstream-run", "binding")
	if err != nil {
		t.Fatal(err)
	}
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	handler := &DirectPresentationHandler{Core: core, Presentation: presentation.NewService(store.Presentation()), Sessions: svc,
		Principal: func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }}
	listed, err := handler.ListDirectSessions(ctx, connect.NewRequest(&gulv1.ListDirectSessionsRequest{WorkspaceId: "workspace"}))
	if err != nil || len(listed.Msg.GetSessions()) != 1 || listed.Msg.GetSessions()[0].GetSessionId() != binding.ID {
		t.Fatalf("listed sessions = %+v, %v", listed, err)
	}
	result, err := handler.GetExecutionState(ctx, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
	if err != nil || result.Msg.GetFreshness() != gulv1.Freshness_FRESHNESS_FRESH ||
		result.Msg.GetComposition() != gulv1.SessionComposition_SESSION_COMPOSITION_STANDALONE_PRIMARY ||
		result.Msg.GetCounts().GetNonretiredMembers() != 1 || len(result.Msg.GetObservedMembers()) != 1 ||
		result.Msg.GetObservedMembers()[0].GetLifecycle() != gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_RUNNING ||
		result.Msg.GetObservedMembers()[0].GetObservedRef() == "private-specialist-id" || result.Msg.CloseOperationRef != nil {
		t.Fatalf("execution state = %+v, %v", result, err)
	}
	workspacePort.err = workspace.ErrProviderUnavailable
	stale, err := handler.GetExecutionState(ctx, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
	if err != nil || stale.Msg.GetFreshness() != gulv1.Freshness_FRESHNESS_STALE || stale.Msg.Counts == nil {
		t.Fatalf("workspace transport stale state = %+v, %v", stale, err)
	}
	workspacePort.err = workspace.ErrReattachRequired
	_, err = handler.GetExecutionState(ctx, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
	assertWorkspaceError(t, err, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_WORKSPACE_IDENTITY_MISMATCH, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT)
	workspacePort.err = nil
	carrierPort.err = context.DeadlineExceeded
	stale, err = handler.GetExecutionState(ctx, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
	if err != nil || stale.Msg.GetFreshness() != gulv1.Freshness_FRESHNESS_STALE {
		t.Fatalf("carrier timeout stale state = %+v, %v", stale, err)
	}
	carrierPort.err = errors.New("carrier unavailable")
	_, err = handler.GetExecutionState(ctx, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
	assertWorkspaceError(t, err, connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_CONTROLLER_CARRIER_INVALID, gulv1.ActionClass_ACTION_CLASS_VERIFY_CONTROLLER)
	carrierPort.err = nil
	provider.offline = true
	handler.Sessions = session.NewService(workspacePort, store.Presentation(), provider, carrierPort)
	unavailable, err := handler.GetExecutionState(ctx, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
	if err != nil || unavailable.Msg.GetFreshness() != gulv1.Freshness_FRESHNESS_UNAVAILABLE || unavailable.Msg.Counts != nil {
		t.Fatalf("unavailable execution state = %+v, %v", unavailable, err)
	}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "other"}, nil }
	if _, err := handler.GetExecutionState(ctx, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("cross-subject state = %v", err)
	}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	if _, err := handler.Presentation.SetNavigation(ctx, "owner", presentation.Navigation{WorkspaceID: "workspace", SessionID: binding.ID}); err != nil {
		t.Fatal(err)
	}
	before := provider.calls
	workspaceHandler := &WorkspaceHandler{Core: core, Presentation: handler.Presentation, Principal: handler.Principal}
	if _, err := workspaceHandler.RemoveWorkspaceEntry(ctx, connect.NewRequest(&gulv1.RemoveWorkspaceEntryRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty workspace removal = %v", err)
	}
	if _, err := workspaceHandler.RemoveWorkspaceEntry(ctx, connect.NewRequest(&gulv1.RemoveWorkspaceEntryRequest{WorkspaceId: "missing"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing workspace removal = %v", err)
	}
	if _, err := store.Presentation().Binding(ctx, "owner", binding.ID); err != nil {
		t.Fatalf("failed removal changed binding = %v", err)
	}
	if _, err := workspaceHandler.RemoveWorkspaceEntry(ctx, connect.NewRequest(&gulv1.RemoveWorkspaceEntryRequest{WorkspaceId: "workspace"})); err != nil {
		t.Fatal(err)
	}
	if provider.calls != before {
		t.Fatalf("local removal invoked provider %d times", provider.calls-before)
	}
	if _, err := store.Presentation().Binding(ctx, "owner", binding.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("removed binding = %v", err)
	}
	if navigation, err := handler.Presentation.Navigation(ctx, "owner"); err != nil || navigation != (presentation.Navigation{}) {
		t.Fatalf("removed workspace navigation = %+v, %v", navigation, err)
	}
}

func TestExecutionStateBrowserEnumMappings(t *testing.T) {
	if browserMemberLifecycle("waiting_interaction") != gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_WAITING_INTERACTION ||
		browserMemberLifecycle("start_failed") != gulv1.ObservedMemberLifecycle_OBSERVED_MEMBER_LIFECYCLE_START_FAILED ||
		browserComposition("brokered_hierarchy") != gulv1.SessionComposition_SESSION_COMPOSITION_BROKERED_HIERARCHY ||
		browserLifecycle("recovering") != gulv1.SessionLifecycle_SESSION_LIFECYCLE_RECOVERING ||
		browserApproval("fully_delegated") != gulv1.ApprovalPolicy_APPROVAL_POLICY_FULLY_DELEGATED ||
		browserCloseProgress("outcome_unknown") != gulv1.CloseProgress_CLOSE_PROGRESS_OUTCOME_UNKNOWN ||
		browserRecovery("reconcile_required") != gulv1.RecoveryClass_RECOVERY_CLASS_RECONCILE_REQUIRED {
		t.Fatal("provider execution-state vocabulary mapped incorrectly")
	}
}
