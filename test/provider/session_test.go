package provider_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	gv "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	contract "github.com/rootkernel/gul/contract"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1/dolgoraev1connect"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/host"
)

// No Assemble/Features/Provider override: every route uses ordinary production
// host assembly, the published gateway and real files/public gRPC. Protocol
// behavior behind Dolgorae is the isolated upstream native fake, never live Codex.
func TestPublishedAuthenticatedSessionCreation(t *testing.T) {
	binary, archive, source := os.Getenv("GUL_E2_DOLGORAE_EXECUTABLE"), os.Getenv("GUL_E2_DOLGORAE_ARCHIVE"), os.Getenv("GUL_E2_DOLGORAE_SOURCE")
	if binary == "" {
		t.Skip("published-provider session qualification is opt-in")
	}
	root, err := os.MkdirTemp(providerTemporaryBase(), "gs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	release := contract.QualifiedRelease()
	binary, err = qualifyReleaseArtifacts(archive, binary, release.Archive.SHA256, release.Executable.SHA256, root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 360*time.Second)
	defer cancel()
	prepare := exec.CommandContext(ctx, "python3", "fixtures/prepare_session.py", binary, root, source)
	prepare.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "TMPDIR="+filepath.Join(root, "tmp"))
	if b, err := prepare.CombinedOutput(); err != nil {
		t.Fatalf("isolated upstream fake bootstrap: %v (%s)", err, b)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	data := filepath.Join(root, "Gul")
	h := host.New(host.Config{DataDirectory: data, Port: port, DolgoraeExecutable: binary, ProviderHome: filepath.Join(root, "home"), WorkspaceRoots: []string{filepath.Join(root, "workspace")}, Policies: []string{"carrier-test"}})
	if err = h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if err := h.Stop(stop); err != nil {
			t.Error(err)
		}
	})
	cert, err := x509.ParseCertificate(h.CertificateDER())
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 40 * time.Second}
	auth := gulv1connect.NewAuthServiceClient(client, h.Origin())
	attachment, err := host.Attach(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	setup := connect.NewRequest(&gv.FirstRunSetupRequest{Password: []byte("published session fixture password")})
	setup.Header().Set("Origin", h.Origin())
	setup.Header().Set(api.BrowserHeader, "1")
	setup.Header().Set(api.SetupHeader, attachment.SetupCredential)
	if _, err = auth.FirstRunSetup(ctx, setup); err != nil {
		t.Fatal(err)
	}
	login := connect.NewRequest(&gv.LoginRequest{Password: []byte("published session fixture password")})
	login.Header().Set("Origin", h.Origin())
	login.Header().Set(api.BrowserHeader, "1")
	logged, err := auth.Login(ctx, login)
	if err != nil {
		t.Fatal(err)
	}
	decorate := func(q connect.AnyRequest) {
		q.Header().Set("Origin", h.Origin())
		q.Header().Set(api.BrowserHeader, "1")
		q.Header().Set(api.CSRFHeader, logged.Msg.Session.CsrfToken)
		q.Header().Set("Cookie", strings.Split(logged.Header().Get("Set-Cookie"), ";")[0])
	}
	diagnosticsQ := connect.NewRequest(&gv.GetSummaryRequest{})
	decorate(diagnosticsQ)
	diagnostics, err := gulv1connect.NewDiagnosticsServiceClient(client, h.Origin()).GetSummary(ctx, diagnosticsQ)
	if err != nil || !diagnostics.Msg.GetProviderChannelReady() {
		t.Fatal("published provider startup", diagnostics, err)
	}
	workspaces := gulv1connect.NewWorkspacePresentationServiceClient(client, h.Origin())
	register := connect.NewRequest(&gv.RegisterFromAllowlistPathRequest{RootId: "root-1", RelativePath: "."})
	decorate(register)
	registered, err := workspaces.RegisterFromAllowlistPath(ctx, register)
	if err != nil {
		t.Fatal(err)
	}
	ws := registered.Msg.Workspace.WorkspaceId
	direct := gulv1connect.NewDirectSessionServiceClient(client, h.Origin())
	choice := &gv.CheckCompatibilityRequest{ProfileName: "default", ModelId: "gpt-5.6", Effort: "medium", Lane: gv.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_DEDICATED, RequiredAssurance: gv.LaunchAssurance_LAUNCH_ASSURANCE_BEST_EFFORT_PERSONAL_ALPHA, PolicyName: "carrier-test"}
	msg := &gv.CreateSessionRequest{WorkspaceId: ws, AttemptId: uuid.NewString(), Choice: choice}
	unauth := connect.NewRequest(msg)
	unauth.Header().Set("Origin", h.Origin())
	unauth.Header().Set(api.BrowserHeader, "1")
	if _, err = direct.CreateSession(ctx, unauth); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("unprotected session creation", err)
	}
	q := connect.NewRequest(msg)
	decorate(q)
	created, err := direct.CreateSession(ctx, q)
	if err != nil || created.Msg.OutcomeUnknown || created.Msg.SessionId == "" {
		t.Fatal("published authenticated creation", created, err)
	}
	q = connect.NewRequest(msg)
	decorate(q)
	retry, err := direct.CreateSession(ctx, q)
	if err != nil || retry.Msg.SessionId != created.Msg.SessionId || retry.Msg.OutcomeUnknown {
		t.Fatal("creation retry duplicated or lost its session", retry, err)
	}
	stateQ := connect.NewRequest(&gv.GetExecutionStateRequest{SessionId: created.Msg.SessionId})
	decorate(stateQ)
	state, err := direct.GetExecutionState(ctx, stateQ)
	if err != nil || state.Msg.SpecialistPolicyName != "carrier-test" || state.Msg.Composition == gv.SessionComposition_SESSION_COMPOSITION_UNSPECIFIED || state.Msg.Freshness != gv.Freshness_FRESHNESS_FRESH {
		t.Fatal("required public session aggregate", state, err)
	}
	listQ := connect.NewRequest(&gv.ListDirectSessionsRequest{WorkspaceId: ws})
	decorate(listQ)
	listed, err := direct.ListDirectSessions(ctx, listQ)
	if err != nil || len(listed.Msg.Sessions) != 1 {
		t.Fatal("browser-visible session inventory", listed, err)
	}
	if len(listed.Msg.PendingCreationAttemptIds) != 0 {
		t.Fatal("completed creation remained pending")
	}
	accepted := false
	for range 40 {
		input := connect.NewRequest(&gv.SubmitRequest{SessionId: created.Msg.SessionId, AttemptId: uuid.NewString(), Text: "published first WRITE", WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE})
		decorate(input)
		response, err := direct.Submit(ctx, input)
		if err != nil {
			t.Fatal("first WRITE", err)
		}
		if response.Msg.Outcome == gv.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
			accepted = true
			break
		}
		if response.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED || response.Msg.State.GetBlocker() != gv.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED {
			t.Fatal("first WRITE refused", response.Msg)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !accepted {
		t.Fatal("first WRITE did not converge")
	}
	if body, err := os.ReadFile(filepath.Join(root, "workspace/provider-write.txt")); err != nil || string(body) != "upstream native fake WRITE effect\n" {
		t.Fatal("native fake did not execute workspace WRITE", err)
	}
	writer := gulv1connect.NewWriterActionServiceClient(client, h.Origin())
	var action *gv.ActionState
	publicRun := publishedHostRun(t, ctx, root, created.Msg.SessionId)
	if publicRun.GetEffectivePolicy().GetAccess() != pb.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN || publicRun.GetEffectivePolicy().GetVerification() != pb.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED || publicRun.GetWriterAuthority().GetState() != pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE {
		t.Fatal("published public policy facts changed", publicRun)
	}
	for range 40 {
		q := connect.NewRequest(&gv.GetActionStateRequest{SessionId: created.Msg.SessionId, WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_READ, CloseIntent: gv.ActionCloseIntent_ACTION_CLOSE_INTENT_COMPLETE})
		decorate(q)
		r, err := writer.GetActionState(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		action = r.Msg.State
		if action.GetWriter().GetAuthority() == gv.WriterAuthority_WRITER_AUTHORITY_ACTIVE && action.GetFlags().GetCanPausePrimary() {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !action.GetFlags().GetCanSubmitRead() || !action.GetFlags().GetCanSubmitWrite() || !action.GetFlags().GetCanReleaseWriter() || action.GetFlags().GetCanAcquireWriter() || action.GetMode() != gv.WriterAccessMode_WRITER_ACCESS_MODE_UNVERIFIED {
		t.Fatal("known unverified held writer eligibility", action)
	}
	originalThread, generation := publicRun.GetThread().GetThreadId(), publicRun.GetWriterAuthority().GetWriterGeneration()
	prompts := []string{"published first WRITE", "same text is a new request\r\n원문", "same text is a new request\r\n원문"}
	for index, intent := range []gv.ActionWriteIntent{gv.ActionWriteIntent_ACTION_WRITE_INTENT_READ, gv.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE} {
		for tries := 0; ; tries++ {
			input := connect.NewRequest(&gv.SubmitRequest{SessionId: created.Msg.SessionId, AttemptId: uuid.NewString(), Text: prompts[index+1], WriteIntent: intent})
			decorate(input)
			response, err := direct.Submit(ctx, input)
			if err != nil {
				t.Fatal("sequential request", err)
			}
			if response.Msg.Outcome == gv.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
				break
			}
			if tries >= 12 || response.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED || response.Msg.State.GetBlocker() != gv.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED && response.Msg.State.GetBlocker() != gv.ActionBlocker_ACTION_BLOCKER_ACTIVE_TURN_DRAFT {
				t.Fatal("sequential request refused", response.Msg)
			}
			time.Sleep(200 * time.Millisecond)
		}
		next := publishedHostRun(t, ctx, root, created.Msg.SessionId)
		if next.GetRunId() != publicRun.GetRunId() || next.GetThread().GetThreadId() != originalThread || next.GetWriterAuthority().GetWriterGeneration() != generation || next.GetWriterAuthority().GetState() != pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE || next.GetEffectivePolicy().GetAccess() != pb.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN || next.GetEffectivePolicy().GetVerification() != pb.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED {
			t.Fatal("sequential request altered provider facts", next)
		}
	}
	inputs, err := os.ReadFile(filepath.Join(root, "native-inputs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(inputs)), "\n")
	if len(lines) != len(prompts) {
		t.Fatal("duplicated or missing native dispatch", len(lines))
	}
	for i, line := range lines {
		var native struct {
			Sandbox string `json:"sandbox"`
			Thread  string `json:"thread"`
			Input   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"input"`
		}
		if json.Unmarshal([]byte(line), &native) != nil || native.Sandbox != "workspaceWrite" || native.Thread != originalThread || len(native.Input) != 1 || native.Input[0].Text != prompts[i] {
			t.Fatal("native request differs", native)
		}
	}
	var token *string
	var snapshot string
	var ids []string
	for {
		historyQ := connect.NewRequest(&gv.ListPromptHistoryRequest{SessionId: created.Msg.SessionId, PageSize: 2, PageToken: token})
		decorate(historyQ)
		page, err := direct.ListPromptHistory(ctx, historyQ)
		if err != nil || page.Msg.SnapshotId == "" {
			t.Fatal("paged history", page, err)
		}
		if snapshot != "" && snapshot != page.Msg.SnapshotId {
			t.Fatal("history snapshot changed")
		}
		snapshot = page.Msg.SnapshotId
		for _, item := range page.Msg.Items {
			ids = append(ids, item.PromptItemId)
			detailQ := connect.NewRequest(&gv.GetPromptHistoryItemRequest{SessionId: created.Msg.SessionId, PromptItemId: item.PromptItemId})
			decorate(detailQ)
			detail, err := direct.GetPromptHistoryItem(ctx, detailQ)
			if err != nil || int(item.Ordinal) != len(ids) || detail.Msg.GetOriginal().GetInlineUtf8() != prompts[len(ids)-1] {
				t.Fatal("exact original history", detail, err)
			}
		}
		if page.Msg.TraversalComplete {
			break
		}
		token = page.Msg.NextPageToken
		if token == nil || len(ids) >= len(prompts) {
			t.Fatal("history did not complete")
		}
	}
	if len(ids) != len(prompts) || ids[0] == ids[1] || ids[1] == ids[2] {
		t.Fatal("history duplicates or omissions", ids)
	}
	// Exercise a real waiting Interaction without browser request fan-out.
	for tries := 0; ; tries++ {
		q := connect.NewRequest(&gv.SubmitRequest{SessionId: created.Msg.SessionId, AttemptId: uuid.NewString(), Text: "wait-for-approval", WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_READ})
		decorate(q)
		response, err := direct.Submit(ctx, q)
		if err != nil {
			t.Fatal("waiting request", err)
		}
		if response.Msg.Outcome == gv.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
			break
		}
		if tries >= 40 || response.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED {
			t.Fatal("waiting request refused", response.Msg)
		}
		time.Sleep(500 * time.Millisecond)
	}
	interactions := gulv1connect.NewInteractionPresentationServiceClient(client, h.Origin())
	var card *gv.InteractionCard
	for tries := 0; tries < 40; tries++ {
		q := connect.NewRequest(&gv.ListPendingRequest{SessionId: created.Msg.SessionId})
		decorate(q)
		pending, err := interactions.ListPending(ctx, q)
		if err != nil {
			t.Fatal("pending Interaction", err)
		}
		if len(pending.Msg.Summaries) == 1 {
			q := connect.NewRequest(&gv.GetCardRequest{SessionId: created.Msg.SessionId, InteractionId: pending.Msg.Summaries[0].InteractionId})
			decorate(q)
			detail, err := interactions.GetCard(ctx, q)
			if err != nil {
				t.Fatal("Controller card", err)
			}
			card = detail.Msg.Card
			if card.GetActions().GetCanResolveInteraction() {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if card == nil || !card.GetActions().GetCanResolveInteraction() {
		t.Fatal("waiting card did not become actionable", card)
	}
	for tries := 0; ; tries++ {
		resolveQ := connect.NewRequest(&gv.ResolveRequest{SessionId: created.Msg.SessionId, InteractionId: card.Summary.InteractionId, ResponseJson: []byte(`{"decision":"accept_once"}`)})
		decorate(resolveQ)
		resolved, err := interactions.Resolve(ctx, resolveQ)
		if err == nil && resolved.Msg.Outcome == gv.InteractionResolutionOutcome_INTERACTION_RESOLUTION_OUTCOME_RESOLVED {
			break
		}
		var rpc *connect.Error
		safeRefetch := false
		if errors.As(err, &rpc) && rpc.Code() == connect.CodeFailedPrecondition {
			for _, detail := range rpc.Details() {
				value, decodeErr := detail.Value()
				domain, ok := value.(*gv.DomainError)
				if decodeErr == nil && ok && domain.Code == gv.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED && domain.Action == gv.ActionClass_ACTION_CLASS_REFETCH_INTERACTION {
					safeRefetch = true
				}
			}
		}
		if !safeRefetch || tries >= 20 {
			t.Fatal("waiting reply", resolved, err)
		}
		time.Sleep(500 * time.Millisecond)
		q := connect.NewRequest(&gv.GetCardRequest{SessionId: created.Msg.SessionId, InteractionId: card.Summary.InteractionId})
		decorate(q)
		if _, err = interactions.GetCard(ctx, q); err != nil {
			t.Fatal("refetched Interaction", err)
		}
	}
	picture, err := os.Create(filepath.Join(root, "workspace/picture.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(picture, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	picture.Close()
	for _, text := range []string{"accepted image", "after image"} {
		accepted := false
		for tries := 0; tries < 40; tries++ {
			msg := &gv.SubmitRequest{SessionId: created.Msg.SessionId, AttemptId: uuid.NewString(), Text: text, WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_READ}
			if text == "accepted image" {
				msg.Images = []*gv.SubmitImage{{WorkspaceId: ws, RelativePath: "picture.png", Detail: "auto"}}
			}
			q := connect.NewRequest(msg)
			decorate(q)
			response, err := direct.Submit(ctx, q)
			if err != nil {
				t.Fatal("image sequence", text, err)
			}
			if response.Msg.Outcome == gv.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
				accepted = true
				break
			}
			if response.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED {
				t.Fatal("image sequence uncertainty", text, response.Msg)
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !accepted {
			current := publishedHostRun(t, ctx, root, created.Msg.SessionId)
			t.Fatal("image sequence did not converge", text, current.GetLifecycle(), current.GetRecovery(), current.GetActiveTurn(), current.GetStamp())
		}
	}
	if err = os.WriteFile(filepath.Join(root, "workspace/.dolgorae/private.png"), []byte("private canary"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(".dolgorae", filepath.Join(root, "workspace/private-alias")); err != nil {
		t.Fatal(err)
	}
	unsupportedEffort := "unsupported-effort"
	for _, invalid := range []*gv.SubmitRequest{
		{Images: []*gv.SubmitImage{{WorkspaceId: ws, RelativePath: ".dolgorae/private.png", Detail: "auto"}}},
		{Images: []*gv.SubmitImage{{WorkspaceId: ws, RelativePath: "private-alias/private.png", Detail: "auto"}}},
		{Images: []*gv.SubmitImage{{WorkspaceId: uuid.NewString(), RelativePath: "picture.png", Detail: "auto"}}},
		{Effort: &unsupportedEffort},
	} {
		invalid.SessionId, invalid.Text, invalid.WriteIntent = created.Msg.SessionId, "invalid input must not dispatch", gv.ActionWriteIntent_ACTION_WRITE_INTENT_READ
		for tries := 0; ; tries++ {
			invalid.AttemptId = uuid.NewString()
			q := connect.NewRequest(invalid)
			decorate(q)
			response, err := direct.Submit(ctx, q)
			if connect.CodeOf(err) == connect.CodeInvalidArgument {
				break
			}
			if err != nil || tries >= 40 || response.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED || response.Msg.State.GetBlocker() != gv.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED {
				t.Fatal("guarded invalid input", response, err)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	inputLog, err := os.ReadFile(filepath.Join(root, "native-inputs.jsonl"))
	if err != nil || len(strings.Split(strings.TrimSpace(string(inputLog)), "\n")) != 6 {
		t.Fatal("invalid inputs dispatched native work", err)
	}
	for tries := 0; ; tries++ {
		q := connect.NewRequest(&gv.ReleaseWriterRequest{SessionId: created.Msg.SessionId})
		decorate(q)
		released, err := writer.ReleaseWriter(ctx, q)
		if err == nil {
			if released.Msg.State.GetWriter().GetAuthority() == gv.WriterAuthority_WRITER_AUTHORITY_ACTIVE {
				t.Fatal("Release retained writer")
			}
			break
		}
		if tries >= 40 || connect.CodeOf(err) != connect.CodeFailedPrecondition {
			stateQ := connect.NewRequest(&gv.GetActionStateRequest{SessionId: created.Msg.SessionId, WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_READ, CloseIntent: gv.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE})
			decorate(stateQ)
			state, stateErr := writer.GetActionState(ctx, stateQ)
			current := publishedHostRun(t, ctx, root, created.Msg.SessionId)
			t.Fatal("unverified writer Release", err, state, stateErr, current.GetLifecycle(), current.GetRecovery(), current.GetWriterAuthority(), current.GetEffectivePolicy(), current.GetStamp())
		}
		time.Sleep(500 * time.Millisecond)
	}
	publicReleased := publishedHostRun(t, ctx, root, created.Msg.SessionId)
	if publicReleased.GetWriterAuthority().GetState() != pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE {
		t.Fatal("provider did not release writer", publicReleased.GetWriterAuthority())
	}
	// Lose Gul's allocation receipt durably after the published provider accepted.
	// The isolated database fault does not alter the provider or inject runtime ports.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(data, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `CREATE TRIGGER lose_start_receipt BEFORE UPDATE OF outcome_ref ON mutation_attempt_details WHEN (SELECT operation_kind FROM provider_operation_attempts WHERE operation_id=NEW.operation_id)='StartRun' BEGIN SELECT RAISE(ABORT,'fixture receipt loss'); END`); err != nil {
		t.Fatal(err)
	}
	competitorAttempt := uuid.NewString()
	q = connect.NewRequest(&gv.CreateSessionRequest{WorkspaceId: ws, AttemptId: competitorAttempt, Choice: choice})
	decorate(q)
	if response, createErr := direct.CreateSession(ctx, q); createErr == nil && !response.Msg.OutcomeUnknown {
		t.Fatal("receipt loss was hidden", response.Msg)
	}
	if _, err = db.ExecContext(ctx, `DROP TRIGGER lose_start_receipt`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	recoverQ := connect.NewRequest(&gv.RecoverCreationRequest{WorkspaceId: ws, AttemptId: competitorAttempt})
	decorate(recoverQ)
	competitor, err := direct.RecoverCreation(ctx, recoverQ)
	if err != nil || competitor.Msg.OutcomeUnknown || competitor.Msg.SessionId == "" || competitor.Msg.SessionId == created.Msg.SessionId {
		t.Fatal("exact StartRun recovery", competitor, err)
	}
	recoverQ = connect.NewRequest(&gv.RecoverCreationRequest{WorkspaceId: ws, AttemptId: competitorAttempt})
	decorate(recoverQ)
	recovered, err := direct.RecoverCreation(ctx, recoverQ)
	if err != nil || recovered.Msg.SessionId != competitor.Msg.SessionId {
		t.Fatal("recovery duplicated allocation", recovered, err)
	}
	competitorRun := publishedHostRun(t, ctx, root, competitor.Msg.SessionId)
	if competitorRun.GetRunId() == publicRun.GetRunId() {
		t.Fatal("unrelated Run identity collapsed")
	}
	for tries := 0; ; tries++ {
		q := connect.NewRequest(&gv.SubmitRequest{SessionId: competitor.Msg.SessionId, AttemptId: uuid.NewString(), Text: "unrelated first WRITE", WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE})
		decorate(q)
		r, err := direct.Submit(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if r.Msg.Outcome == gv.SubmitOutcome_SUBMIT_OUTCOME_ACCEPTED {
			break
		}
		if tries >= 40 || r.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED || r.Msg.State.GetBlocker() != gv.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED {
			t.Fatal("unrelated first WRITE", r.Msg)
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Acceptance precedes native Turn settlement. Observe the competitor's
	// quiescent owned Writer through Gul before checking another Run's refusal.
	for tries := 0; ; tries++ {
		q := connect.NewRequest(&gv.GetActionStateRequest{SessionId: competitor.Msg.SessionId, WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_READ, CloseIntent: gv.ActionCloseIntent_ACTION_CLOSE_INTENT_NONE})
		decorate(q)
		state, readErr := writer.GetActionState(ctx, q)
		if readErr == nil && state.Msg.State.GetFlags().GetCanPausePrimary() && state.Msg.State.GetWriter().GetAuthority() == gv.WriterAuthority_WRITER_AUTHORITY_ACTIVE {
			break
		}
		if tries >= 40 {
			t.Fatal("competitor Writer did not settle", state, readErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
	for tries := 0; ; tries++ {
		q := connect.NewRequest(&gv.SubmitRequest{SessionId: created.Msg.SessionId, AttemptId: uuid.NewString(), Text: "competing WRITE must not dispatch", WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE})
		decorate(q)
		response, err := direct.Submit(ctx, q)
		if err != nil {
			t.Fatal("competing WRITE read", err, publishedHostRun(t, ctx, root, created.Msg.SessionId, true))
		}
		if response.Msg.Outcome == gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED && response.Msg.State.GetBlocker() == gv.ActionBlocker_ACTION_BLOCKER_WRITER_BUSY {
			break
		}
		if tries >= 40 || response.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED || response.Msg.State.GetBlocker() != gv.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED {
			t.Fatal("competing writer refusal", response.Msg)
		}
		time.Sleep(500 * time.Millisecond)
	}
	closeMsg := &gv.CloseRuntimeRequest{SessionId: created.Msg.SessionId, AttemptId: uuid.NewString()}
	for tries := 0; ; tries++ {
		closeQ := connect.NewRequest(closeMsg)
		decorate(closeQ)
		closed, err := direct.CloseRuntime(ctx, closeQ)
		if err != nil {
			t.Fatal("owned aggregate close", err)
		}
		if closed.Msg.Outcome.Status != gv.CloseStatus_CLOSE_STATUS_REJECTED {
			break
		}
		if tries >= 40 || closed.Msg.Outcome.GetRejection().GetAction() != gv.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT {
			t.Fatal("owned aggregate close", closed.Msg)
		}
		time.Sleep(500 * time.Millisecond)
	}
	for tries := 0; ; tries++ {
		q := connect.NewRequest(&gv.GetExecutionStateRequest{SessionId: created.Msg.SessionId})
		decorate(q)
		r, err := direct.GetExecutionState(ctx, q)
		if err == nil && r.Msg.CloseProgress == gv.CloseProgress_CLOSE_PROGRESS_CONFIRMED {
			break
		}
		if tries >= 40 {
			t.Fatal("aggregate close settlement", r, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err = h.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err = h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for tries := 0; ; tries++ {
		historyQ := connect.NewRequest(&gv.ListPromptHistoryRequest{SessionId: created.Msg.SessionId, PageSize: 20})
		decorate(historyQ)
		history, readErr := direct.ListPromptHistory(ctx, historyQ)
		if readErr == nil && len(history.Msg.Items) == 6 {
			break
		}
		if tries >= 40 {
			t.Fatal("closed history retention", history, readErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
	unrelated := publishedHostRun(t, ctx, root, competitor.Msg.SessionId)
	if unrelated.GetLifecycle() == pb.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		t.Fatal("root close affected unrelated session")
	}
	sharedChoice := &gv.CheckCompatibilityRequest{ProfileName: "default", ModelId: "gpt-5.6", Effort: "medium", Lane: gv.LaunchExecutionLane_LAUNCH_EXECUTION_LANE_SHARED_READONLY, RequiredAssurance: gv.LaunchAssurance_LAUNCH_ASSURANCE_BEST_EFFORT_PERSONAL_ALPHA, PolicyName: "carrier-test", AcknowledgeSharedReadonly: true}
	q = connect.NewRequest(&gv.CreateSessionRequest{WorkspaceId: ws, AttemptId: uuid.NewString(), Choice: sharedChoice})
	decorate(q)
	shared, err := direct.CreateSession(ctx, q)
	if err != nil || shared.Msg.OutcomeUnknown {
		t.Fatal("shared session", shared, err)
	}
	for tries := 0; ; tries++ {
		q := connect.NewRequest(&gv.SubmitRequest{SessionId: shared.Msg.SessionId, AttemptId: uuid.NewString(), Text: "shared WRITE must not dispatch", WriteIntent: gv.ActionWriteIntent_ACTION_WRITE_INTENT_WRITE})
		decorate(q)
		response, err := direct.Submit(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if response.Msg.Outcome == gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED && response.Msg.State.GetBlocker() != gv.ActionBlocker_ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED {
			break
		}
		if tries >= 40 || response.Msg.Outcome != gv.SubmitOutcome_SUBMIT_OUTCOME_REJECTED {
			t.Fatal("shared WRITE refusal", response.Msg, publishedHostRun(t, ctx, root, shared.Msg.SessionId))
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// This additional read uses the ordinary host's owned socket and public Run
// identity. It is a diagnostic oracle, not a production fallback or a substitute
// for the authenticated Gul/browser acceptance above.
func publishedHostRun(t *testing.T, ctx context.Context, root, sessionID string, diagnostic ...bool) *pb.RunProjection {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(root, "Gul/gul.sqlite")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var runID, providerWorkspace string
	if err := db.QueryRowContext(ctx, `SELECT b.run_id,a.provider_workspace_id FROM primary_session_bindings b JOIN workspace_attachments a USING(subject_id,workspace_id) WHERE b.session_id=?`, sessionID).Scan(&runID, &providerWorkspace); err != nil {
		t.Fatal(err)
	}
	sockets, err := filepath.Glob(filepath.Join(root, "home/Library/Caches/Gul/runtime/g-*/rpc.sock"))
	if err != nil || len(sockets) != 1 {
		t.Fatal("owned host socket", sockets, err)
	}
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sockets[0])
	}}
	defer transport.CloseIdleConnections()
	client := dolgoraev1connect.NewRunServiceClient(&http.Client{Transport: transport}, "http://localhost", connect.WithGRPC())
	r, err := client.GetRun(ctx, connect.NewRequest(&pb.GetRunRequest{Context: &pb.RequestContext{ProtocolVersion: 1, ClientRequestId: uuid.NewString(), ClientInstanceId: uuid.NewString()}, Run: &pb.RunRef{RunId: runID, Workspace: &pb.WorkspaceRef{AbsolutePath: filepath.Join(root, "workspace"), ExpectedWorkspaceId: providerWorkspace}}}))
	if err != nil {
		t.Fatal("published diagnostic GetRun", err)
	}
	if len(diagnostic) > 0 && diagnostic[0] {
		writer := dolgoraev1connect.NewWriterServiceClient(&http.Client{Transport: transport}, "http://localhost", connect.WithGRPC())
		response, readErr := writer.GetWorkspaceWriterStatus(ctx, connect.NewRequest(&pb.GetWorkspaceWriterStatusRequest{Context: &pb.RequestContext{ProtocolVersion: 1, ClientRequestId: uuid.NewString(), ClientInstanceId: uuid.NewString()}, Workspace: &pb.WorkspaceRef{AbsolutePath: filepath.Join(root, "workspace"), ExpectedWorkspaceId: providerWorkspace}}))
		t.Log("public writer diagnostic", response, readErr)
	}
	controllerID := r.Msg.Run.GetController().GetControllerId()
	runs, err := client.ListRuns(ctx, connect.NewRequest(&pb.ListRunsRequest{Context: &pb.RequestContext{ProtocolVersion: 1, ClientRequestId: uuid.NewString(), ClientInstanceId: uuid.NewString()}, Workspace: &pb.WorkspaceRef{AbsolutePath: filepath.Join(root, "workspace"), ExpectedWorkspaceId: providerWorkspace}, ControllerId: &controllerID}))
	if err != nil || len(runs.Msg.Items) != 1 || runs.Msg.Items[0].RunId != runID {
		t.Fatal("provider allocation count for exact root Controller", runs, err)
	}
	return r.Msg.Run
}

func providerTemporaryBase() string {
	mounted, err := exec.Command("mount").Output()
	if err == nil && strings.Contains(string(mounted), " on /Volumes/RootKernel (") && exec.Command("test", "-w", "/Volumes/RootKernel").Run() == nil {
		return "/Volumes/RootKernel/tmp"
	}
	return os.TempDir()
}
