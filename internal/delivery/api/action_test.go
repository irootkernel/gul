package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type apiActionProvider struct {
	calls      atomic.Int32
	threadless atomic.Bool
}

func (p *apiActionProvider) Read(context.Context, action.Bound) (action.Input, error) {
	stamp := observation.Stamp{Head: "4", Run: 4, Writer: 2, Interaction: 3}
	in := action.Input{ControllerMatches: true, Checked: true, Freshness: action.Fresh, Run: action.RunFacts{Lifecycle: action.Idle, Variant: action.DedicatedReader, Control: action.Direct, Lane: action.Dedicated, Thread: action.Present, ActiveTurn: action.Missing, Access: action.Read, Verification: action.Verified, Authority: action.Unowned, Reconciliation: action.NoReconciliation, Requested: action.BestEffort, Achieved: action.BestEffort, Background: action.Absent, Server: action.ServerReady, Recovery: action.NoRecovery, RecoveryAction: action.NoRecoveryAction, Lineage: action.NoLineage, Stamp: stamp}, Writer: action.WriterProjection{Authority: action.Unowned, Access: action.UnknownAccess, Verification: action.Unverified, Owner: action.NoOwner, Reconciliation: action.NoReconciliation, Revision: 2, Stamp: observation.Stamp{Writer: 2}}, Profile: action.ProfileFacts{Compatibility: action.Compatible, Transition: action.Supported, BackgroundControl: action.Supported, MaximumAssurance: action.ProcessContained, SupportsLane: true}, Capabilities: action.Capabilities{Checked: true, Submit: true, Acquire: true, Release: true, ReaderWriter: true, DurableWriter: true, FirstWriteViaSubmit: true, Transition: action.Supported}, InteractionStamp: stamp, TimelineHead: "4", Aggregate: action.Aggregate{Directive: action.NoAggregateAction, Freshness: action.Fresh, Revision: 91, Lifecycle: action.SessionActive, CloseProgress: action.NoClose, Recovery: action.AggregateReady, CloseIntent: action.NoCloseIntent, NonretiredMembers: 1}}
	if p.threadless.Load() {
		in.Run.Thread = action.Missing
		in.Run.Variant = action.DedicatedUnstarted
	}
	return in, nil
}
func (p *apiActionProvider) Acquire(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	p.calls.Add(1)
	return action.WriterProjection{}, action.ErrWriterBusy
}
func (p *apiActionProvider) Release(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	p.calls.Add(1)
	return action.WriterProjection{}, action.ErrWriterBusy
}
func TestWriterRPCUsesFreshSharedGateAndTypedConflict(t *testing.T) {
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
	if err := store.Presentation().CreateAttachment(t.Context(), workspace.Attachment{SubjectID: "owner", ID: "ws", CanonicalRoot: "/workspace", ProviderID: "provider", FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Presentation().InsertBinding(t.Context(), session.Binding{SubjectID: "owner", ID: "session", WorkspaceID: "ws", RunID: "run", ControllerBindingID: "controller", ProviderSessionID: "provider-session"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Cache().PutTimelineHead(t.Context(), "owner", "session", storage.UpstreamCursor{Value: "4"}); err != nil {
		t.Fatal(err)
	}
	provider := &apiActionProvider{}
	service := &action.Service{Repository: store.Actions("dolgorae"), Workspaces: interactionWorkspace{}, Carriers: interactionCarrier{}, Provider: provider, Gate: func(string, string) bool { return true }}
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	handler := &WriterHandler{Core: core, Principal: func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }, Actions: service}
	_, rpc := gulv1connect.NewWriterActionServiceHandler(handler)
	server := httptest.NewServer(rpc)
	defer server.Close()
	client := gulv1connect.NewWriterActionServiceClient(server.Client(), server.URL)
	state, err := client.GetActionState(t.Context(), connect.NewRequest(&gulv1.GetActionStateRequest{SessionId: "session", WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ, CloseIntent: gulv1.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE}))
	if err != nil || !state.Msg.State.Flags.CanAcquireWriter {
		t.Fatal(state, err)
	}
	_, err = client.AcquireWriter(t.Context(), connect.NewRequest(&gulv1.AcquireWriterRequest{SessionId: "session"}))
	var conflict *connect.Error
	if !errors.As(err, &conflict) || conflict.Code() != connect.CodeFailedPrecondition || provider.calls.Load() != 1 {
		t.Fatal(err, provider.calls.Load())
	}
	found := false
	for _, d := range conflict.Details() {
		value, _ := d.Value()
		if typed, ok := value.(*gulv1.ActionFailure); ok {
			found = typed.Blocker == gulv1.ActionBlocker_ACTION_BLOCKER_WRITER_BUSY
		}
	}
	if !found {
		t.Fatal("writer_busy not preserved")
	}
	provider.threadless.Store(true)
	if _, err = client.AcquireWriter(t.Context(), connect.NewRequest(&gulv1.AcquireWriterRequest{SessionId: "session"})); err == nil || provider.calls.Load() != 1 {
		t.Fatal("threadless RPC invoked", err)
	}
	if _, err = client.ReleaseWriter(t.Context(), connect.NewRequest(&gulv1.ReleaseWriterRequest{SessionId: "session"})); err == nil || provider.calls.Load() != 1 {
		t.Fatal("unowned release invoked", err)
	}
	if _, err = client.AcquireWriter(t.Context(), connect.NewRequest(&gulv1.AcquireWriterRequest{SessionId: "foreign"})); connect.CodeOf(err) != connect.CodePermissionDenied || provider.calls.Load() != 1 {
		t.Fatal("foreign session", err)
	}
	if _, err = client.GetActionState(t.Context(), connect.NewRequest(&gulv1.GetActionStateRequest{SessionId: "session", WriteIntent: 99, CloseIntent: gulv1.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatal("unknown intent", err)
	}
}

func TestActionPersistenceErrorUsesLocalRepairCode(t *testing.T) {
	err := actionError(action.ErrPersistence)
	var rpc *connect.Error
	if !errors.As(err, &rpc) || rpc.Code() != connect.CodeUnavailable {
		t.Fatal(err)
	}
	var blocker, code bool
	for _, detail := range rpc.Details() {
		value, _ := detail.Value()
		switch typed := value.(type) {
		case *gulv1.ActionFailure:
			blocker = typed.Blocker == gulv1.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED
		case *gulv1.DomainError:
			code = typed.Code == gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE && typed.Action == gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR
		}
	}
	if !blocker || !code {
		t.Fatal(rpc.Details())
	}
}
