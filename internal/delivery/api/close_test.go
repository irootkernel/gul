package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/sessionclose"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/encoding/protojson"
)

type apiCloseProvider struct {
	accepted  atomic.Bool
	mutations atomic.Int32
	reads     atomic.Int32
	interrupt bool
	readErr   error
}

func (p *apiCloseProvider) Mutate(_ context.Context, b action.Bound, kind sessionclose.Kind, revision uint64, interrupt bool) (sessionclose.Mutation, error) {
	p.mutations.Add(1)
	p.interrupt = interrupt
	if b.Binding.RunID != "upstream-run" || kind != sessionclose.Close || revision != 4 {
		return sessionclose.Mutation{}, errors.New("private wrong root mutation")
	}
	p.accepted.Store(true)
	return sessionclose.Mutation{Pending: true, OperationID: "private-provider-close-operation"}, nil
}
func (p *apiCloseProvider) Snapshot(ctx context.Context, w workspace.Attachment, run string, c session.Carrier) (session.Snapshot, error) {
	p.reads.Add(1)
	if p.readErr != nil {
		return session.Snapshot{}, p.readErr
	}
	snapshot, err := (&sessionProvider{}).Snapshot(ctx, w, run, c)
	if p.accepted.Load() {
		snapshot.Lifecycle, snapshot.CloseProgress, snapshot.CloseOperationID = "completing", "settling", "private-provider-close-operation"
	}
	return snapshot, err
}

type apiCloseActions struct {
	apiActionProvider
	close *apiCloseProvider
}

func (p *apiCloseActions) Read(ctx context.Context, b action.Bound) (action.Input, error) {
	in, err := p.apiActionProvider.Read(ctx, b)
	in.Capabilities.Close = true
	in.Aggregate.Revision = 3
	if p.close.accepted.Load() {
		in.Aggregate.Lifecycle, in.Aggregate.CloseProgress, in.Aggregate.CloseIntent = action.SessionCompleting, action.CloseSettling, action.CompleteSession
	}
	return in, err
}

type apiCloseRefresh struct{}

func (apiCloseRefresh) Refresh(context.Context, action.Bound, action.RunFacts) error { return nil }

func TestCloseRuntimeAuthorizationPrivacyAndStableReadReference(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.Context(), filepath.Join(dir, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	entry := workspace.Attachment{SubjectID: "owner", ID: "workspace", CanonicalRoot: "/workspace", ProviderID: "upstream-workspace", FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}
	if err := store.Presentation().CreateAttachment(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	provider := &apiCloseProvider{}
	workspaces, carriers := &sessionWorkspace{entry: entry}, &sessionCarrier{}
	sessions := session.NewService(workspaces, store.Presentation(), provider, carriers)
	binding, err := sessions.BindPrimary(t.Context(), "owner", "workspace", "upstream-run", "binding")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Cache().PutTimelineHead(t.Context(), "owner", binding.ID, storage.UpstreamCursor{Value: "4"}); err != nil {
		t.Fatal(err)
	}
	actions := &action.Service{Repository: store.Actions("dolgorae"), Workspaces: workspaces, Carriers: carriers, Provider: &apiCloseActions{close: provider}, Gate: func(string, string) bool { return true }}
	closer := &sessionclose.Service{Repository: store.SessionClose(), Actions: actions, Sessions: sessions, Provider: provider, Refresh: apiCloseRefresh{}}
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	handler := &DirectPresentationHandler{Core: core, Presentation: presentation.NewService(store.Presentation()), Sessions: sessions, Close: closer,
		Principal: func(context.Context) (app.Principal, error) {
			return app.Principal{}, errors.New("private auth canary")
		}}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	for _, tc := range []struct {
		err  error
		want gulv1.ProviderState
	}{
		{nil, gulv1.ProviderState_PROVIDER_STATE_READY},
		{session.ErrUnavailable, gulv1.ProviderState_PROVIDER_STATE_DISCONNECTED},
		{session.ErrIncompatible, gulv1.ProviderState_PROVIDER_STATE_INCOMPATIBLE},
		{session.ErrBusy, gulv1.ProviderState_PROVIDER_STATE_BUSY},
		{session.ErrDegraded, gulv1.ProviderState_PROVIDER_STATE_DEGRADED},
	} {
		provider.readErr = tc.err
		response, err := handler.GetExecutionState(t.Context(), connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
		if err != nil || response.Msg.ProviderState != tc.want {
			t.Fatalf("provider state=%v error=%v", response, err)
		}
		if tc.err != nil && response.Msg.Freshness != gulv1.Freshness_FRESHNESS_STALE {
			t.Fatalf("failed read lost stale marker: %v", response.Msg)
		}
	}
	provider.readErr = nil
	if browserProviderState(session.ProviderState(255)) != gulv1.ProviderState_PROVIDER_STATE_UNSPECIFIED {
		t.Fatal("unknown provider state admitted")
	}
	handler.Principal = func(context.Context) (app.Principal, error) {
		return app.Principal{}, errors.New("private auth canary")
	}
	request := connect.NewRequest(&gulv1.CloseRuntimeRequest{SessionId: binding.ID, AttemptId: "browser-request", Interrupt: true})
	before := provider.reads.Load()
	for _, q := range []*connect.Request[gulv1.CloseRuntimeRequest]{nil, request} {
		result, err := handler.CloseRuntime(t.Context(), q)
		if result != nil || connect.CodeOf(err) != connect.CodeUnauthenticated || strings.Contains(err.Error(), "private") {
			t.Fatalf("unauthenticated close = %v, %v", result, err)
		}
	}
	if result, err := handler.GetExecutionState(t.Context(), connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID})); result != nil || connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated read = %v, %v", result, err)
	}
	if provider.reads.Load() != before || provider.mutations.Load() != 0 {
		t.Fatal("unauthenticated request reached provider")
	}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "other"}, nil }
	result, err := handler.CloseRuntime(t.Context(), request)
	if result != nil || connect.CodeOf(err) != connect.CodeNotFound || provider.reads.Load() != before || provider.mutations.Load() != 0 {
		t.Fatalf("foreign close = %v, %v", result, err)
	}
	handler.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	for _, q := range []*connect.Request[gulv1.CloseRuntimeRequest]{nil, connect.NewRequest(&gulv1.CloseRuntimeRequest{}), connect.NewRequest(&gulv1.CloseRuntimeRequest{SessionId: binding.ID, AttemptId: strings.Repeat("x", 257)})} {
		if _, err := handler.CloseRuntime(t.Context(), q); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("invalid request = %v", err)
		}
	}
	if provider.mutations.Load() != 0 {
		t.Fatal("invalid request dispatched")
	}
	if _, found, err := store.SessionClose().Find(t.Context(), "owner", binding.ID, "browser-request"); err != nil || found {
		t.Fatalf("rejected request persisted an attempt: found=%v, err=%v", found, err)
	}
	result, err = handler.CloseRuntime(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Msg.Outcome
	if out.Status != gulv1.CloseStatus_CLOSE_STATUS_IN_PROGRESS || out.CloseAttemptId == "" || out.GetCloseOperationRef() == "" || out.Rejection != nil || provider.mutations.Load() != 1 || !provider.interrupt {
		t.Fatalf("close = %v; calls %d", out, provider.mutations.Load())
	}
	state, err := handler.GetExecutionState(t.Context(), connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: binding.ID}))
	if err != nil || state.Msg.GetCloseOperationRef() != out.GetCloseOperationRef() || provider.mutations.Load() != 1 {
		t.Fatalf("read = %v, %v", state, err)
	}
	again, err := handler.CloseRuntime(t.Context(), request)
	if err != nil || again.Msg.Outcome.CloseAttemptId != out.CloseAttemptId || again.Msg.Outcome.GetCloseOperationRef() != out.GetCloseOperationRef() || provider.mutations.Load() != 1 {
		t.Fatalf("retry = %v, %v", again, err)
	}
	for _, message := range []string{protojson.Format(result.Msg), protojson.Format(state.Msg), protojson.Format(again.Msg)} {
		for _, private := range []string{"private-provider-close-operation", "upstream-run", "upstream-workspace", "upstream-session", "/trusted/carrier"} {
			if strings.Contains(message, private) {
				t.Fatalf("browser disclosed %q", private)
			}
		}
	}
}

func TestCloseOutcomeStatusesAndClosedErrorVocabulary(t *testing.T) {
	for state, expected := range map[sessionclose.Status]gulv1.CloseStatus{
		sessionclose.Rejected:         gulv1.CloseStatus_CLOSE_STATUS_REJECTED,
		sessionclose.InProgress:       gulv1.CloseStatus_CLOSE_STATUS_IN_PROGRESS,
		sessionclose.Confirmed:        gulv1.CloseStatus_CLOSE_STATUS_CONFIRMED,
		sessionclose.OutcomeUnknown:   gulv1.CloseStatus_CLOSE_STATUS_OUTCOME_UNKNOWN,
		sessionclose.RecoveryRequired: gulv1.CloseStatus_CLOSE_STATUS_RECOVERY_REQUIRED,
	} {
		out, err := browserCloseOutcome(sessionclose.Outcome{Status: state, AttemptID: "gul-attempt", OperationRef: "gul-operation", Code: "RUN_STATE_CONFLICT", NextAction: "REFRESH_SNAPSHOT"})
		if err != nil || out.Status != expected || out.GetCloseOperationRef() != "gul-operation" || out.NextAction != gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT {
			t.Fatalf("%s = %v, %v", state, out, err)
		}
		if state == sessionclose.Rejected {
			if out.Rejection == nil || out.Rejection.Code != gulv1.ErrorCode_ERROR_CODE_RUN_STATE_CONFLICT || out.Rejection.Action != out.NextAction {
				t.Fatal(out)
			}
		} else if out.Rejection != nil {
			t.Fatal("non-rejection carried rejection")
		}
	}
	for _, bad := range []sessionclose.Outcome{
		{Status: "future"},
		{Status: sessionclose.Confirmed},
		{Status: sessionclose.Rejected, Code: "private-error-canary", NextAction: "REFRESH_SNAPSHOT"},
		{Status: sessionclose.Rejected, Code: "PROVIDER_BLOCKED", NextAction: "private-action-canary"},
	} {
		if _, err := browserCloseOutcome(bad); connect.CodeOf(err) != connect.CodeFailedPrecondition || strings.Contains(err.Error(), "canary") {
			t.Fatalf("invalid outcome = %v", err)
		}
	}
	assertWorkspaceError(t, closeError(sessionclose.ErrConflict), connect.CodeAborted, gulv1.ErrorCode_ERROR_CODE_RUN_STATE_CONFLICT, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT)
	assertWorkspaceError(t, closeError(errors.Join(session.ErrPersistenceUnavailable, errors.New("private-error-canary"))), connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR)
	if err := closeError(errors.New("private-error-canary")); strings.Contains(err.Error(), "canary") {
		t.Fatal("raw provider failure exposed")
	}
}
