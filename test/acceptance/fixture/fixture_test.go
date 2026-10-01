package fixture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/operation"
	"github.com/rootkernel/gul/internal/session"
)

type credentials struct{ cookie, csrf string }

func login(t *testing.T, f *Fixture) credentials {
	t.Helper()
	q := connect.NewRequest(&gulv1.LoginRequest{Password: []byte(Password)})
	q.Header().Set("Origin", f.Host.Origin())
	q.Header().Set(api.BrowserHeader, "1")
	r, err := gulv1connect.NewAuthServiceClient(f.HTTP, f.Host.Origin()).Login(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	cookie := strings.Split(r.Header().Get("Set-Cookie"), ";")[0]
	return credentials{cookie, r.Msg.Session.CsrfToken}
}
func authorize[T any](f *Fixture, c credentials, q *connect.Request[T]) *connect.Request[T] {
	q.Header().Set("Origin", f.Host.Origin())
	q.Header().Set(api.BrowserHeader, "1")
	q.Header().Set("Cookie", c.cookie)
	q.Header().Set(api.CSRFHeader, c.csrf)
	return q
}
func prepared(t *testing.T) *Fixture {
	t.Helper()
	dir := t.TempDir()
	f, err := New(filepath.Join(dir, "Gul"), filepath.Join(dir, "Workspace"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if err := f.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	if err = f.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return f
}
func synchronize(t *testing.T, f *Fixture) {
	t.Helper()
	if err := f.Runtime.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestAssembledAuthenticatedHistoryApprovalResultsCloseAndRestart(t *testing.T) {
	f := prepared(t)
	c := login(t, f)
	runtime := gulv1connect.NewRuntimeServiceClient(f.HTTP, f.Host.Origin())
	profiles, err := runtime.ListRuntimeProfiles(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListRuntimeProfilesRequest{})))
	if err != nil || len(profiles.Msg.Profiles) == 0 || len(profiles.Msg.Profiles[0].Models) == 0 {
		t.Fatalf("runtime choices: %v,%v", profiles, err)
	}
	profile := profiles.Msg.Profiles[0]
	model := profile.Models[0]
	choice, err := runtime.CheckCompatibility(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.CheckCompatibilityRequest{ProfileName: profile.Name, ModelId: model.ModelId, Effort: model.SupportedEfforts[0], Lane: gulv1.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_DEDICATED, RequiredAssurance: gulv1.LaunchAssurance_LAUNCH_ASSURANCE_BEST_EFFORT_PERSONAL_ALPHA, PolicyName: "preprovisioned"})))
	if err != nil || choice.Msg.Configuration == nil || choice.Msg.Configuration.ProfileName != profile.Name {
		t.Fatalf("runtime compatibility: %v,%v", choice, err)
	}
	direct := func() gulv1connect.DirectSessionServiceClient {
		return gulv1connect.NewDirectSessionServiceClient(f.HTTP, f.Host.Origin())
	}
	send := func(id, text string) *gulv1.SubmitResponse {
		t.Helper()
		r, e := direct().Submit(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: id, Text: text, WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ})))
		if e != nil {
			t.Fatal(e)
		}
		return r.Msg
	}
	texts := []string{"same\r\n한글", "same\r\n한글", strings.Repeat("긴 입력\r\n", 40000)}
	for i, text := range texts {
		synchronize(t, f)
		waitSubmitReady(t, f, c, gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ)
		result, err := submitAfterRefresh(t, f, c, direct(), &gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: string(rune('a' + i)), Text: text, WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ})
		if err != nil || result.Msg.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
			t.Fatalf("submit %d: %v, %v", i, result, err)
		}
		if result := send("busy", "never queued"); result.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_REJECTED {
			t.Fatalf("busy: %v", result)
		}
		if i == 0 {
			if e := f.Approval(); e != nil {
				t.Fatal(e)
			}
			synchronize(t, f)
			interactions := gulv1connect.NewInteractionPresentationServiceClient(f.HTTP, f.Host.Origin())
			card, e := interactions.GetCard(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetCardRequest{SessionId: f.Binding.ID, InteractionId: "approval"})))
			if e != nil || card.Msg.Card == nil {
				t.Fatalf("approval card: %v, %v", card, e)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				resolved, err := interactions.Resolve(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ResolveRequest{SessionId: f.Binding.ID, InteractionId: "approval", ResponseJson: []byte(`{"decision":"accept_once"}`)})))
				if err == nil {
					if resolved.Msg.Outcome != gulv1.InteractionResolutionOutcome_INTERACTION_RESOLUTION_OUTCOME_RESOLVED {
						t.Fatalf("approval outcome: %v", resolved.Msg)
					}
					break
				}
				var wire *connect.Error
				var domain *gulv1.DomainError
				if errors.As(err, &wire) {
					for _, detail := range wire.Details() {
						value, _ := detail.Value()
						if d, ok := value.(*gulv1.DomainError); ok {
							domain = d
						}
					}
				}
				// This typed refusal occurs before an Interaction attempt begins.
				if connect.CodeOf(err) != connect.CodeFailedPrecondition || domain == nil || domain.Code != gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED || domain.Action != gulv1.ActionClass_ACTION_CLASS_REFETCH_INTERACTION || time.Now().After(deadline) {
					t.Fatal(err)
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
		if e := f.Complete(); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.Results(); e != nil {
		t.Fatal(e)
	}
	synchronize(t, f)
	before, e := direct().ListPromptHistory(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: f.Binding.ID, PageSize: 2})))
	if e != nil {
		t.Fatal(e)
	}
	if len(before.Msg.Items) != 2 || before.Msg.NextPageToken == nil {
		t.Fatalf("first history page: %v", before.Msg)
	}
	second, e := direct().ListPromptHistory(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: f.Binding.ID, PageSize: 2, PageToken: before.Msg.NextPageToken})))
	if e != nil || len(second.Msg.Items) != 1 {
		t.Fatalf("second page: %v,%v", second, e)
	}
	ids := []string{before.Msg.Items[0].PromptItemId, before.Msg.Items[1].PromptItemId, second.Msg.Items[0].PromptItemId}
	if ids[0] == ids[1] {
		t.Fatal("duplicate text collapsed separate turns")
	}
	if e = f.Restart(t.Context()); e != nil {
		t.Fatal(e)
	}
	c = login(t, f)
	fresh, e := direct().ListPromptHistory(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: f.Binding.ID, PageSize: 50})))
	if e != nil || len(fresh.Msg.Items) != 3 {
		t.Fatalf("restart history: %v,%v", fresh, e)
	}
	for i := range ids {
		ids[i] = fresh.Msg.Items[i].PromptItemId
	}
	for i, id := range ids {
		original, e := direct().GetPromptHistoryItem(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetPromptHistoryItemRequest{SessionId: f.Binding.ID, PromptItemId: id})))
		if e != nil {
			t.Fatal(e)
		}
		body := original.Msg.Original.GetInlineUtf8()
		if ref := original.Msg.Original.GetArtifactRef(); ref != "" {
			body = artifact(t, f, c, ref)
		}
		if body != texts[i] {
			t.Fatalf("original %d changed after restart", i)
		}
	}
	results, e := direct().ListSpecialistResults(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListSpecialistResultsRequest{SessionId: f.Binding.ID, PageSize: 2})))
	if e != nil || len(results.Msg.Items) != 1 {
		t.Fatalf("results: %v,%v", results, e)
	}
	if body := artifact(t, f, c, results.Msg.Items[0].ArtifactRef); body != "Specialist original\r\n한글 <script>inert</script>" {
		t.Fatal("result original changed")
	}
	closed, e := direct().CloseRuntime(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.CloseRuntimeRequest{SessionId: f.Binding.ID, AttemptId: "close", Interrupt: false})))
	if e != nil || closed.Msg.Outcome.Status != gulv1.CloseStatus_CLOSE_STATUS_IN_PROGRESS {
		t.Fatalf("close: %v,%v", closed, e)
	}
	calls := f.Provider.CloseCalls()
	if len(calls) != 1 || calls[0] != f.Run.RunId {
		t.Fatalf("close targeted child: %v", calls)
	}
	if e = f.Provider.SetCloseProgress(f.Run.RunId, publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED); e != nil {
		t.Fatal(e)
	}
	synchronize(t, f)
	state, e := direct().GetExecutionState(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetExecutionStateRequest{SessionId: f.Binding.ID})))
	if e != nil || state.Msg.CloseProgress != gulv1.CloseProgress_CLOSE_PROGRESS_CONFIRMED {
		t.Fatalf("aggregate close: %v,%v", state, e)
	}
	after, e := direct().ListPromptHistory(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: f.Binding.ID, PageSize: 50})))
	if e != nil || len(after.Msg.Items) != 3 {
		t.Fatalf("post-close history: %v,%v", after, e)
	}
}
func artifact(t *testing.T, f *Fixture, c credentials, ref string) string {
	t.Helper()
	client := gulv1connect.NewArtifactPresentationServiceClient(f.HTTP, f.Host.Origin())
	m, e := client.GetMetadata(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetMetadataRequest{SessionId: f.Binding.ID, ArtifactRef: ref})))
	if e != nil {
		t.Fatal(e)
	}
	var body []byte
	for offset := uint64(0); offset < m.Msg.ByteLength; {
		length := min(uint64(gulv1.MaximumArtifactChunkBytes), m.Msg.ByteLength-offset)
		part, e := client.ReadChunk(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ReadChunkRequest{SessionId: f.Binding.ID, ArtifactRef: ref, Offset: offset, Length: uint32(length)})))
		if e != nil {
			t.Fatal(e)
		}
		if uint64(len(part.Msg.Data)) != length || part.Msg.Sha256 != m.Msg.Sha256 {
			t.Fatal("invalid artifact chunk")
		}
		body = append(body, part.Msg.Data...)
		offset += length
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != m.Msg.Sha256 {
		t.Fatal("artifact digest mismatch")
	}
	return string(body)
}

func TestAssembledGuardsOfflinePresentationAndUnknownRestart(t *testing.T) {
	f := prepared(t)
	c := login(t, f)
	services := gulv1.File_gul_v1_gul_proto.Services()
	for i := 0; i < services.Len(); i++ {
		service := services.Get(i)
		if service.Name() == "AuthService" {
			continue
		}
		methods := service.Methods()
		for j := 0; j < methods.Len(); j++ {
			path := "/" + string(service.FullName()) + "/" + string(methods.Get(j).Name())
			for _, guard := range []string{"auth", "csrf", "origin"} {
				q, e := http.NewRequestWithContext(t.Context(), http.MethodPost, f.Host.Origin()+path, bytes.NewBufferString("{}"))
				if e != nil {
					t.Fatal(e)
				}
				q.Header.Set("Content-Type", "application/json")
				q.Header.Set("Origin", f.Host.Origin())
				q.Header.Set(api.BrowserHeader, "1")
				q.Header.Set(api.CSRFHeader, c.csrf)
				if guard != "auth" {
					q.Header.Set("Cookie", c.cookie)
				}
				if guard == "csrf" {
					q.Header.Del(api.CSRFHeader)
				}
				if guard == "origin" {
					q.Header.Set("Origin", "https://attacker.invalid")
				}
				r, e := f.HTTP.Do(q)
				if e != nil {
					t.Fatal(e)
				}
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
				if r.StatusCode != http.StatusUnauthorized && r.StatusCode != http.StatusForbidden {
					t.Fatalf("%s %s guard: %d", path, guard, r.StatusCode)
				}
			}
		}
	}
	diagnostics := gulv1connect.NewDiagnosticsServiceClient(f.HTTP, f.Host.Origin())
	online, err := diagnostics.GetSummary(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetSummaryRequest{})))
	if err != nil || !online.Msg.ProviderReady || !online.Msg.PersistenceReady {
		t.Fatalf("online diagnostics: %v, %v", online, err)
	}
	f.Provider.Offline.Store(true)
	_ = f.Runtime.Synchronize(t.Context())
	w, e := gulv1connect.NewWorkspacePresentationServiceClient(f.HTTP, f.Host.Origin()).ListWorkspaces(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListWorkspacesRequest{})))
	if e != nil || len(w.Msg.Workspaces) != 1 {
		t.Fatalf("offline list: %v,%v", w, e)
	}
	d := gulv1connect.NewDiagnosticsServiceClient(f.HTTP, f.Host.Origin())
	status, e := d.GetSummary(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetSummaryRequest{})))
	if e != nil || status.Msg.ProviderReady || !status.Msg.PersistenceReady {
		t.Fatalf("offline diagnostics: %v,%v", status, e)
	}
	client := gulv1connect.NewDirectSessionServiceClient(f.HTTP, f.Host.Origin())
	offline, e := client.Submit(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: "offline", Text: "must not queue", WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ})))
	if e != nil || offline.Msg.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_REJECTED || len(f.Provider.SubmitCalls()) != 0 {
		t.Fatalf("offline submit admitted: %v, %v", offline, e)
	}
	key := sha256.Sum256([]byte(f.Binding.SubjectID + "\x00offline"))
	if _, err := f.Store.Attempts().Mutation(t.Context(), hex.EncodeToString(key[:])); !errors.Is(err, operation.ErrNotFound) {
		t.Fatalf("offline submit recorded an attempt: %v", err)
	}
	writer := gulv1connect.NewWriterActionServiceClient(f.HTTP, f.Host.Origin())
	if _, err := writer.AcquireWriter(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.AcquireWriterRequest{SessionId: f.Binding.ID}))); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("offline writer admitted: %v", err)
	}
	closed, err := client.CloseRuntime(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.CloseRuntimeRequest{SessionId: f.Binding.ID, AttemptId: "offline-close"})))
	if err != nil || closed.Msg.Outcome.Status != gulv1.CloseStatus_CLOSE_STATUS_REJECTED || closed.Msg.Outcome.CloseOperationRef != nil || len(f.Provider.CloseCalls()) != 0 {
		t.Fatalf("offline close admitted: %v, %v", closed, err)
	}
	f.Provider.Offline.Store(false)
	synchronize(t, f)
	if e = f.Provider.FaultNext("SubmitTurn", scenario.AfterCommit, io.ErrUnexpectedEOF); e != nil {
		t.Fatal(e)
	}
	q := &gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: "unknown", Text: "unknown original", WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ}
	result, e := submitAfterRefresh(t, f, c, client, q)
	if e != nil || result.Msg.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_UNKNOWN {
		t.Fatalf("unknown: %v,%v", result, e)
	}
	if len(f.Provider.SubmitCalls()) != 1 {
		t.Fatal("unknown submit did not dispatch exactly once")
	}
	if e = f.Restart(t.Context()); e != nil {
		t.Fatal(e)
	}
	c = login(t, f)
	client = gulv1connect.NewDirectSessionServiceClient(f.HTTP, f.Host.Origin())
	page, e := client.ListPromptHistory(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.ListPromptHistoryRequest{SessionId: f.Binding.ID, PageSize: 50})))
	if e != nil || len(page.Msg.Items) != 1 {
		t.Fatalf("unknown history lost after restart: %v,%v", page, e)
	}
	synchronize(t, f)
	if len(f.Provider.SubmitCalls()) != 1 {
		t.Fatal("unknown submit was redispatched across restart/convergence")
	}
}

func waitSubmitReady(t *testing.T, f *Fixture, c credentials, intent gulv1.ActionWriteIntent) {
	t.Helper()
	client := gulv1connect.NewWriterActionServiceClient(f.HTTP, f.Host.Origin())
	deadline := time.Now().Add(5 * time.Second)
	var last *gulv1.GetActionStateResponse
	for time.Now().Before(deadline) {
		r, e := client.GetActionState(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetActionStateRequest{SessionId: f.Binding.ID, WriteIntent: intent, CloseIntent: gulv1.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE})))
		if e != nil {
			t.Fatal(e)
		}
		last = r.Msg
		if intent == gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ && r.Msg.State.Flags.CanSubmitRead || intent == gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE && r.Msg.State.Flags.CanSubmitWrite {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("submit readiness never converged: %v", last)
}

// Readiness can change while the observer refreshes dependent projections.
// Only a typed freshness rejection permits a new explicit attempt; an unknown
// effect or any other response is returned without retrying it.
func submitAfterRefresh(t *testing.T, f *Fixture, c credentials, client gulv1connect.DirectSessionServiceClient, q *gulv1.SubmitRequest) (*connect.Response[gulv1.SubmitResponse], error) {
	t.Helper()
	calls := len(f.Provider.SubmitCalls())
	deadline := time.Now().Add(5 * time.Second)
	for attempt := 0; ; attempt++ {
		request := &gulv1.SubmitRequest{SessionId: q.SessionId, AttemptId: fmt.Sprintf("%s-%d", q.AttemptId, attempt), Text: q.Text, WriteIntent: q.WriteIntent}
		result, err := client.Submit(t.Context(), authorize(f, c, connect.NewRequest(request)))
		if err != nil || result.Msg.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_REJECTED || result.Msg.State.GetBlocker() != gulv1.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED {
			return result, err
		}
		if len(f.Provider.SubmitCalls()) != calls || !result.Msg.State.GetFlags().GetRequiresFreshSnapshot() {
			t.Fatalf("freshness rejection dispatched or lost its reason: %v", result.Msg)
		}
		if time.Now().After(deadline) {
			t.Fatalf("submit did not converge after freshness rejection: %v", result.Msg)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestAssembledFirstWriteAndAdmissionLimits(t *testing.T) {
	f := prepared(t)
	c := login(t, f)
	client := gulv1connect.NewDirectSessionServiceClient(f.HTTP, f.Host.Origin())
	for _, text := range []string{"", strings.Repeat("a", gulv1.MaximumSubmitTextBytes+1)} {
		_, e := client.Submit(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: "oversized", Text: text, WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE})))
		if connect.CodeOf(e) != connect.CodeInvalidArgument {
			t.Fatalf("invalid prompt admission: %v", e)
		}
	}
	for i := 0; i < 2; i++ {
		synchronize(t, f)
		waitSubmitReady(t, f, c, gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE)
		r, e := submitAfterRefresh(t, f, c, client, &gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: string(rune('x' + i)), Text: "write prompt", WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE})
		if e != nil || r.Msg.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
			t.Fatalf("write %d: %v,%v", i, r, e)
		}
		calls := f.Provider.SubmitCalls()
		if len(calls) != i+1 || calls[i] != publicv1.WriteIntent_WRITE_INTENT_WRITE {
			t.Fatalf("WRITE intent downgraded: %v", calls)
		}
		synchronize(t, f)
		writer := gulv1connect.NewWriterActionServiceClient(f.HTTP, f.Host.Origin())
		state, err := writer.GetActionState(t.Context(), authorize(f, c, connect.NewRequest(&gulv1.GetActionStateRequest{SessionId: f.Binding.ID, WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE, CloseIntent: gulv1.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE})))
		if err != nil || state.Msg.State.Writer.Authority != gulv1.WriterAuthority_WRITER_AUTHORITY_ACTIVE || state.Msg.State.Writer.EffectiveAccess != gulv1.WriterEffectiveAccess_WRITER_EFFECTIVE_ACCESS_WRITE || state.Msg.State.Writer.PolicyVerification != gulv1.WriterPolicyVerification_WRITER_POLICY_VERIFICATION_VERIFIED {
			t.Fatalf("first write did not publish verified writer: %v, %v", state, err)
		}
		if e = f.Complete(); e != nil {
			t.Fatal(e)
		}
	}
}

func TestAssembledBindingLookupIsSubjectScopedAndCancellable(t *testing.T) {
	f := prepared(t)
	b, err := f.Store.Presentation().BindingByRun(t.Context(), f.Binding.SubjectID, f.Binding.RunID)
	if err != nil || b.ID != f.Binding.ID {
		t.Fatalf("bound Run lookup: %v, %v", b, err)
	}
	if _, err := f.Store.Presentation().BindingByRun(t.Context(), "another-subject", f.Binding.RunID); err == nil {
		t.Fatal("lookup crossed subject boundary")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.Store.Presentation().BindingByRun(ctx, f.Binding.SubjectID, f.Binding.RunID); !errors.Is(err, context.Canceled) {
		t.Fatalf("lookup ignored cancellation: %v", err)
	}
	attachment, err := f.Store.Presentation().Attachment(t.Context(), f.Binding.SubjectID, f.Binding.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	attachment.ID = "other-workspace"
	attachment.CanonicalRoot = t.TempDir()
	attachment.ProviderID = "other-provider-workspace"
	if err := f.Store.Presentation().CreateAttachment(t.Context(), attachment); err != nil {
		t.Fatal(err)
	}
	other := f.Binding
	other.ID, other.WorkspaceID = "ambiguous-session", attachment.ID
	if _, err := f.Store.Presentation().InsertBinding(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.Presentation().BindingByRun(t.Context(), f.Binding.SubjectID, f.Binding.RunID); !errors.Is(err, session.ErrBindingConflict) {
		t.Fatalf("ambiguous Run selected a binding: %v", err)
	}
}

type persistenceFailureDispatcher struct{}

func (persistenceFailureDispatcher) Submit(context.Context, action.Bound, action.Input, string, string, action.WriteIntent) error {
	return action.ErrPersistence
}

func TestAssembledPredispatchPersistenceFailureIsTypedAndDoesNotDispatch(t *testing.T) {
	f := prepared(t)
	c := login(t, f)
	waitSubmitReady(t, f, c, gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ)
	service := f.Runtime.Features(f.Host.Core).Direct.Submissions
	dispatcher := service.Dispatcher
	service.Dispatcher = persistenceFailureDispatcher{}
	defer func() { service.Dispatcher = dispatcher }()
	client := gulv1connect.NewDirectSessionServiceClient(f.HTTP, f.Host.Origin())
	_, err := submitAfterRefresh(t, f, c, client, &gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: "persistence", Text: "retained draft", WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ})
	if connect.CodeOf(err) != connect.CodeUnavailable || len(f.Provider.SubmitCalls()) != 0 {
		t.Fatalf("persistence failure changed effect taxonomy: %v", err)
	}
	var domain *gulv1.DomainError
	var wire *connect.Error
	if errors.As(err, &wire) {
		for _, detail := range wire.Details() {
			value, _ := detail.Value()
			if d, ok := value.(*gulv1.DomainError); ok {
				domain = d
			}
		}
	}
	if domain == nil || domain.Code != gulv1.ErrorCode_ERROR_CODE_PERSISTENCE_UNAVAILABLE {
		t.Fatalf("persistence failure was not typed: %v", err)
	}
}

func TestFakeSubmitAuthorityAndThreadAcrossSequentialTurns(t *testing.T) {
	f := prepared(t)
	initial, err := f.Provider.GetRun(t.Context(), &publicv1.GetRunRequest{Run: f.Run})
	if err != nil {
		t.Fatal(err)
	}
	if initial.GetRun().GetThread() != nil || initial.GetRun().GetEffectivePolicy().GetAccess() != publicv1.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN || initial.GetRun().GetEffectivePolicy().GetVerification() != publicv1.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED {
		t.Fatalf("unstarted fake policy was qualified: %v", initial)
	}
	controller := &publicv1.ControllerCarrierRef{AbsoluteFilePath: f.carrier.path, ExpectedControllerId: controllerID, ExpectedControllerGeneration: 1}
	start := &publicv1.StartRunRequest{Workspace: f.Run.Workspace, Controller: controller, IdempotencyKey: "other", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_SHARED_READONLY, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE}
	if _, err := f.Provider.StartRun(t.Context(), start); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("fake admitted an unsupported lane: %v", err)
	}
	start.ExecutionLane = publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED
	other, err := f.Provider.StartRun(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	var thread string
	for i, intent := range []publicv1.WriteIntent{publicv1.WriteIntent_WRITE_INTENT_WRITE, publicv1.WriteIntent_WRITE_INTENT_READ} {
		current, err := f.Provider.GetRun(t.Context(), &publicv1.GetRunRequest{Run: f.Run})
		if err != nil {
			t.Fatal(err)
		}
		accepted, err := f.Provider.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{Run: f.Run, Controller: controller, IdempotencyKey: []string{"write", "read"}[i], Message: "prompt", WriteIntent: intent, ExpectedStateRevision: current.GetRun().GetStateRevision()})
		if err != nil {
			t.Fatal(err)
		}
		run := accepted.GetRun()
		if run.GetEffectivePolicy().GetAccess() != publicv1.EffectiveAccess_EFFECTIVE_ACCESS_WRITE || run.GetEffectivePolicy().GetVerification() != publicv1.PolicyVerification_POLICY_VERIFICATION_VERIFIED || run.GetWriterAuthority().GetState() != publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE || accepted.GetWriter().GetOwnerRunId() != f.Run.RunId {
			t.Fatalf("write authority lost on intent %v: %v", intent, accepted)
		}
		if i == 0 {
			thread = accepted.GetAcceptedTurn().GetThreadId()
		}
		if thread == "" || accepted.GetAcceptedTurn().GetThreadId() != thread || run.GetThread().GetThreadId() != thread || run.GetThread().GetThreadGeneration() != 1 {
			t.Fatalf("sequential submit replaced thread: %v", accepted)
		}
		if err := f.Complete(); err != nil {
			t.Fatal(err)
		}
	}
	ref := &publicv1.RunRef{Workspace: f.Run.Workspace, RunId: other.GetRun().GetRunId()}
	_, err = f.Provider.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{Run: ref, Controller: controller, IdempotencyKey: "forbidden-write", Message: "prompt", WriteIntent: publicv1.WriteIntent_WRITE_INTENT_WRITE, ExpectedStateRevision: other.GetRun().GetStateRevision()})
	var wire *connect.Error
	var busy bool
	if errors.As(err, &wire) {
		for _, detail := range wire.Details() {
			value, _ := detail.Value()
			if d, ok := value.(*publicv1.DolgoraeErrorDetail); ok && d.DolgoraeErrorCode == "WRITER_BUSY" {
				busy = true
			}
		}
	}
	if !busy {
		t.Fatalf("another Run stole writer: %v", err)
	}
	current, err := f.Provider.GetRun(t.Context(), &publicv1.GetRunRequest{Run: ref})
	if err != nil || current.GetRun().GetActiveTurn() != nil || current.GetRun().GetThread() != nil || current.GetRun().GetStateRevision() != other.GetRun().GetStateRevision() {
		t.Fatalf("rejected write changed Run: %v, %v", current, err)
	}
}

func TestAssembledOfflineStartupReleasesListenerAndOwnership(t *testing.T) {
	dir := t.TempDir()
	f, err := New(filepath.Join(dir, "Gul"), filepath.Join(dir, "Workspace"), 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Provider.Offline.Store(true)
	if err = f.Start(t.Context()); err == nil {
		t.Fatal("offline checked provider admitted assembled startup")
	}
	origin, err := url.Parse(f.Host.Origin())
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", origin.Host, time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("failed startup retained listener")
	}
	if _, err = os.Stat(filepath.Join(f.Data, "core.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed startup published owner: %v", err)
	}
	f.Provider.Offline.Store(false)
	if err = f.Start(t.Context()); err != nil {
		t.Fatalf("failed startup retained lock/store: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := f.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	if err = f.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	login(t, f)
}

func TestDirectJSONEnvelopeAcceptsMaximumEscapedTextAndRejectsOversizeWire(t *testing.T) {
	f := prepared(t)
	c := login(t, f)
	text := strings.Repeat("\x01", gulv1.MaximumSubmitTextBytes)
	client := gulv1connect.NewDirectSessionServiceClient(f.HTTP, f.Host.Origin(), connect.WithProtoJSON())
	result, err := submitAfterRefresh(t, f, c, client, &gulv1.SubmitRequest{SessionId: f.Binding.ID, AttemptId: "maximum-escaped", Text: text, WriteIntent: gulv1.ActionWriteIntent_ACTION_WRITE_INTENT_READ})
	if err != nil || result.Msg.Outcome != gulv1.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
		t.Fatalf("maximum escaped JSON rejected: %v, %v", result, err)
	}
	calls := f.Provider.SubmitCalls()
	if len(calls) != 1 {
		t.Fatalf("maximum escaped JSON dispatch count: %d", len(calls))
	}
	request, err := http.NewRequestWithContext(t.Context(), "POST", f.Host.Origin()+gulv1connect.DirectSessionServiceSubmitProcedure, strings.NewReader(strings.Repeat(" ", 6*gulv1.MaximumSubmitTextBytes+4097)+"{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("Origin", f.Host.Origin())
	request.Header.Set(api.BrowserHeader, "1")
	request.Header.Set("Cookie", c.cookie)
	request.Header.Set(api.CSRFHeader, c.csrf)
	response, err := f.HTTP.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusTooManyRequests || !bytes.Contains(body, []byte("resource_exhausted")) || len(f.Provider.SubmitCalls()) != 1 {
		t.Fatalf("oversize wire admitted: HTTP%d %s; read=%v", response.StatusCode, body, err)
	}
}
