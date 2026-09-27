package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/encoding/protojson"
)

type historyRepository struct{}

func (historyRepository) Binding(_ context.Context, subject, id string) (session.Binding, error) {
	if subject != "owner" || id != "session" {
		return session.Binding{}, history.ErrAuthority
	}
	return session.Binding{SubjectID: subject, ID: id, RunID: "private-run", WorkspaceID: "workspace", ControllerBindingID: "binding"}, nil
}

type historyProvider struct{ body string }

func (p historyProvider) Snapshot(context.Context, history.Bound) (history.Snapshot, error) {
	return history.Snapshot{Stamp: observation.Stamp{Head: "2", Run: 2}, State: "closed"}, nil
}
func (p historyProvider) Timeline(_ context.Context, _ history.Bound, _ string, _ uint32) (history.Timeline, error) {
	return history.Timeline{Head: "2", Stamp: observation.Stamp{Head: "2", Run: 2}, Items: []history.SourceEntry{{Cursor: "2", ProviderID: "private-item", TurnID: "private-turn", Kind: history.Human, Status: "accepted", At: time.Unix(1, 0), Content: history.Content{Inline: &p.body}}}}, nil
}
func (p historyProvider) Results(context.Context, history.Bound, string, uint32) (history.Results, error) {
	return history.Results{Revision: 1, CapturedAt: time.Unix(1, 0)}, nil
}
func (p historyProvider) ReadArtifact(context.Context, history.Bound, history.Artifact) ([]byte, error) {
	return nil, history.ErrUnavailable
}
func TestHistoryAndArtifactHandlersRecheckAuthorityAndKeepPrivateReferencesOut(t *testing.T) {
	ctx := t.Context()
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err := core.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	ws := &sessionWorkspace{entry: workspace.Attachment{SubjectID: "owner", ID: "workspace", ProviderID: "private-workspace", CanonicalRoot: "/private/root"}}
	body := strings.Repeat("원문\r\n", 40000)
	svc := history.New(historyRepository{}, ws, &sessionCarrier{}, historyProvider{body: body})
	principal := func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }
	h := &DirectPresentationHandler{Core: core, Principal: principal, Presentation: presentation.NewService(nil), History: svc}
	a := &ArtifactHandler{Core: core, Principal: principal, History: svc}
	response, err := h.ListPromptHistory(ctx, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: "session"}))
	if err != nil || len(response.Msg.Items) != 1 {
		t.Fatal(response, err)
	}
	encoded, err := protojson.Marshal(response.Msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-run", "private-item", "private-turn", "/private/root", "controller"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private metadata escaped", private)
		}
	}
	item := response.Msg.Items[0]
	original, err := h.GetPromptHistoryItem(ctx, connect.NewRequest(&gulv1.GetPromptHistoryItemRequest{SessionId: "session", PromptItemId: item.PromptItemId}))
	if err != nil || original.Msg.Original.GetArtifactRef() == "" {
		t.Fatal(original, err)
	}
	ref := original.Msg.Original.GetArtifactRef()
	meta, err := a.GetMetadata(ctx, connect.NewRequest(&gulv1.GetMetadataRequest{SessionId: "session", ArtifactRef: ref}))
	if err != nil || meta.Msg.ByteLength != uint64(len(body)) {
		t.Fatal(meta, err)
	}
	chunk, err := a.ReadChunk(ctx, connect.NewRequest(&gulv1.ReadChunkRequest{SessionId: "session", ArtifactRef: ref, Length: 6}))
	if err != nil || string(chunk.Msg.Data) != body[:6] {
		t.Fatal(chunk, err)
	}
	conversation, err := h.ListConversation(ctx, connect.NewRequest(&gulv1.ListConversationRequest{SessionId: "session"}))
	if err != nil || conversation.Msg.Items[0].EntryId != item.ConversationEntryId || conversation.Msg.Items[0].Kind != gulv1.ConversationKind_CONVERSATION_KIND_HUMAN {
		t.Fatal(conversation, err)
	}
	if _, err = h.GetConversationEntry(ctx, connect.NewRequest(&gulv1.GetConversationEntryRequest{SessionId: "session", EntryId: item.ConversationEntryId})); err != nil {
		t.Fatal(err)
	}
	if _, err = h.ListSpecialistResults(ctx, connect.NewRequest(&gulv1.ListSpecialistResultsRequest{SessionId: "session"})); err != nil {
		t.Fatal(err)
	}
	h.Principal = func(context.Context) (app.Principal, error) { return app.Principal{Subject: "foreign"}, nil }
	a.Principal = h.Principal
	for _, call := range []func() error{
		func() error {
			_, e := h.ListPromptHistory(ctx, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: "session"}))
			return e
		},
		func() error {
			_, e := h.GetPromptHistoryItem(ctx, connect.NewRequest(&gulv1.GetPromptHistoryItemRequest{SessionId: "session", PromptItemId: item.PromptItemId}))
			return e
		},
		func() error {
			_, e := h.ListConversation(ctx, connect.NewRequest(&gulv1.ListConversationRequest{SessionId: "session"}))
			return e
		},
		func() error {
			_, e := h.GetConversationEntry(ctx, connect.NewRequest(&gulv1.GetConversationEntryRequest{SessionId: "session", EntryId: item.ConversationEntryId}))
			return e
		},
		func() error {
			_, e := h.ListSpecialistResults(ctx, connect.NewRequest(&gulv1.ListSpecialistResultsRequest{SessionId: "session"}))
			return e
		},
		func() error {
			_, e := a.GetMetadata(ctx, connect.NewRequest(&gulv1.GetMetadataRequest{SessionId: "session", ArtifactRef: ref}))
			return e
		},
		func() error {
			_, e := a.ReadChunk(ctx, connect.NewRequest(&gulv1.ReadChunkRequest{SessionId: "session", ArtifactRef: ref, Length: 1}))
			return e
		},
	} {
		assertWorkspaceError(t, call(), connect.CodePermissionDenied, gulv1.ErrorCode_ERROR_CODE_UNAUTHORIZED, gulv1.ActionClass_ACTION_CLASS_ABORT)
	}
	h.Principal = principal
	a.Principal = nil
	if _, err = a.GetMetadata(ctx, connect.NewRequest(&gulv1.GetMetadataRequest{SessionId: "session", ArtifactRef: ref})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal(err)
	}
	_, err = h.ListPromptHistory(ctx, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: "session", PageToken: ptrString("bad")}))
	assertWorkspaceError(t, err, connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT)
}
func ptrString(v string) *string { return &v }
