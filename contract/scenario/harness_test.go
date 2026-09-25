package scenario

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"google.golang.org/protobuf/proto"
)

type fixture struct {
	h          *Harness
	workspace  *publicv1.WorkspaceRef
	controller *publicv1.ControllerCarrierRef
	run        *publicv1.RunRef
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	h := New(time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC))
	if err := h.RegisterController(ControllerSpec{ID: "controller-1", Generation: 1, CarrierPath: "/carrier/one", OrchestrationLaunch: true, PolicyName: "preprovisioned"}); err != nil {
		t.Fatal(err)
	}
	workspace, err := h.InspectWorkspace(context.Background(), &publicv1.InspectWorkspaceRequest{AbsolutePath: "/workspace/one"})
	if err != nil {
		t.Fatal(err)
	}
	w := &publicv1.WorkspaceRef{AbsolutePath: "/workspace/one", ExpectedWorkspaceId: workspace.GetWorkspaceId()}
	c := &publicv1.ControllerCarrierRef{AbsoluteFilePath: "/carrier/one", ExpectedControllerId: "controller-1", ExpectedControllerGeneration: 1}
	start, err := h.StartRun(context.Background(), &publicv1.StartRunRequest{Workspace: w, Controller: c,
		IdempotencyKey: "start-1", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{h: h, workspace: w, controller: c, run: &publicv1.RunRef{Workspace: w, RunId: start.GetRun().GetRunId()}}
}

func (f fixture) revision(t *testing.T) uint64 {
	t.Helper()
	response, err := f.h.GetRun(context.Background(), &publicv1.GetRunRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	return response.GetRun().GetStateRevision()
}

func requireProviderCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", code)
	}
	if actual := port.MapProviderError(err).Code; actual != code {
		t.Fatalf("provider error = %s, want %s: %v", actual, code, err)
	}
}

func TestProviderErrorsFollowPinnedPolicy(t *testing.T) {
	body, err := os.ReadFile("../upstream/dolgorae-grpc-error-mapping-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var pinned struct {
		DefaultStatus                   string              `json:"default_status"`
		DefaultRequiredAction           string              `json:"default_required_action"`
		DefaultRetryClassification      string              `json:"default_retry_classification"`
		DefaultRecoveryClassification   string              `json:"default_recovery_classification"`
		StatusOverrides                 map[string][]string `json:"status_overrides"`
		RequiredActionOverrides         map[string][]string `json:"required_action_overrides"`
		RetryClassificationOverrides    map[string][]string `json:"retry_classification_overrides"`
		RecoveryClassificationOverrides map[string][]string `json:"recovery_classification_overrides"`
		MethodOverrides                 map[string]map[string]struct {
			Status                 string   `json:"status"`
			RequiredAction         string   `json:"required_action"`
			RetryClassification    string   `json:"retry_classification"`
			RecoveryClassification string   `json:"recovery_classification"`
			RequiredFields         []string `json:"required_fields"`
		} `json:"method_overrides"`
	}
	if err := json.Unmarshal(body, &pinned); err != nil {
		t.Fatal(err)
	}
	lookup := func(fallback string, overrides map[string][]string, code string) string {
		for value, codes := range overrides {
			if slices.Contains(codes, code) {
				return value
			}
		}
		return fallback
	}
	for code := range scenarioErrorPolicies {
		var actual *connect.Error
		if !errors.As(providerError(code), &actual) || len(actual.Details()) != 1 {
			t.Fatalf("missing typed detail for %s", code)
		}
		value, detailErr := actual.Details()[0].Value()
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		detail := value.(*publicv1.DolgoraeErrorDetail)
		if strings.ToUpper(actual.Code().String()) != lookup(pinned.DefaultStatus, pinned.StatusOverrides, code) ||
			detail.GetAction().String() != lookup(pinned.DefaultRequiredAction, pinned.RequiredActionOverrides, code) ||
			detail.GetRetryClassification().String() != lookup(pinned.DefaultRetryClassification, pinned.RetryClassificationOverrides, code) ||
			detail.GetRecoveryClassification().String() != lookup(pinned.DefaultRecoveryClassification, pinned.RecoveryClassificationOverrides, code) {
			t.Fatalf("%s detail differs from pinned error policy: %v", code, detail)
		}
	}
	f := newFixture(t)
	_, err = f.h.VerifyController(context.Background(), &publicv1.VerifyControllerRequest{Run: f.run, Controller: &publicv1.ControllerCarrierRef{}})
	var mismatch *connect.Error
	if !errors.As(err, &mismatch) || len(mismatch.Details()) != 1 {
		t.Fatalf("VerifyController mismatch = %v", err)
	}
	value, err := mismatch.Details()[0].Value()
	if err != nil {
		t.Fatal(err)
	}
	assertMethodPolicy := func(method, code string, actual *connect.Error, detail *publicv1.DolgoraeErrorDetail) {
		t.Helper()
		policy, ok := pinned.MethodOverrides[method][code]
		if !ok {
			t.Fatalf("missing pinned %s/%s override", method, code)
		}
		status := policy.Status
		if status == "" {
			status = lookup(pinned.DefaultStatus, pinned.StatusOverrides, code)
		}
		action := policy.RequiredAction
		if action == "" {
			action = lookup(pinned.DefaultRequiredAction, pinned.RequiredActionOverrides, code)
		}
		retry := policy.RetryClassification
		if retry == "" {
			retry = lookup(pinned.DefaultRetryClassification, pinned.RetryClassificationOverrides, code)
		}
		recovery := policy.RecoveryClassification
		if recovery == "" {
			recovery = lookup(pinned.DefaultRecoveryClassification, pinned.RecoveryClassificationOverrides, code)
		}
		if strings.ToUpper(actual.Code().String()) != status || detail.GetAction().String() != action ||
			detail.GetRetryClassification().String() != retry || detail.GetRecoveryClassification().String() != recovery {
			t.Fatalf("%s/%s differs from pinned override: %v", method, code, detail)
		}
		for _, field := range policy.RequiredFields {
			switch field {
			case "run_id":
				if detail.GetRunId() == "" {
					t.Fatal("missing pinned run_id")
				}
			case "operation_id":
				if detail.GetOperationId() == "" {
					t.Fatal("missing pinned operation_id")
				}
			default:
				t.Fatalf("unhandled pinned required field %s", field)
			}
		}
	}
	assertMethodPolicy("ControllerService.VerifyController", "CONTROLLER_MISMATCH", mismatch, value.(*publicv1.DolgoraeErrorDetail))
	_, err = f.h.CloseRun(context.Background(), &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	var closeError *connect.Error
	if !errors.As(err, &closeError) || len(closeError.Details()) != 1 {
		t.Fatalf("CloseRun pending = %v", err)
	}
	closeValue, err := closeError.Details()[0].Value()
	if err != nil {
		t.Fatal(err)
	}
	assertMethodPolicy("RunService.CloseRun", "SESSION_CLOSE_IN_PROGRESS", closeError, closeValue.(*publicv1.DolgoraeErrorDetail))
}

func TestArtifactBounds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	capabilities, err := f.h.GetCapabilities(ctx, &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil || capabilities.GetArtifacts().GetMaximumArtifactSize() != maximumArtifactSize || capabilities.GetArtifacts().GetMaximumChunkSize() != maximumChunkSize {
		t.Fatalf("advertised artifact bounds = %v, %v", capabilities, err)
	}
	for _, length := range []uint32{0, maximumChunkSize + 1} {
		_, err = f.h.ReadArtifactChunk(ctx, &publicv1.ReadArtifactChunkRequest{Run: f.run, Controller: f.controller, ArtifactId: "missing", Length: length})
		requireProviderCode(t, err, "INVALID_REQUEST")
	}
	oversized := strings.Repeat("x", maximumArtifactSize+1)
	_, err = f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "too-large", Message: oversized, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "INVALID_REQUEST")
	if err := f.h.PublishResult(f.run.GetRunId(), &publicv1.OrchestratedSessionResult{ResultId: "too-large", TaskId: "too-large"}, []byte(oversized)); err == nil {
		t.Fatal("oversized result was accepted")
	}
}

func TestPinnedMethodsAndExplicitLaunch(t *testing.T) {
	var provider port.PublicContractPort = New(time.Unix(0, 0))
	methods := reflect.TypeOf((*port.PublicContractPort)(nil)).Elem()
	if methods.NumMethod() != len(requiredMethods) {
		t.Fatalf("methods = %d, listed = %d", methods.NumMethod(), len(requiredMethods))
	}
	capabilities, err := provider.GetCapabilities(context.Background(), &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(capabilities.GetSupportedMethods(), requiredMethods) {
		t.Fatalf("advertised methods differ: %v", capabilities.GetSupportedMethods())
	}
	_, err = provider.GetCapabilities(context.Background(), &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 2, MaximumProtocolVersion: 2})
	requireProviderCode(t, err, "INVALID_REQUEST")
	profileBytes, err := os.ReadFile("../upstream/dolgorae-gul-consumer-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var contractProfile struct {
		RequiredMethods []string `json:"required_methods"`
		LaterMethods    []string `json:"unavailable_until_later_tasks"`
	}
	if err := json.Unmarshal(profileBytes, &contractProfile); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(contractProfile.RequiredMethods, requiredMethods) {
		t.Fatalf("scenario method inventory differs from pinned contract: %v", requiredMethods)
	}
	if !slices.Equal(contractProfile.LaterMethods, laterMethods) {
		t.Fatal("scenario later-method inventory differs from pinned contract")
	}
	mutationBytes, err := os.ReadFile("../upstream/dolgorae-rpc-mutation-policy-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var mutationPolicy struct {
		Mutations []struct {
			RPC string `json:"rpc"`
		} `json:"mutations"`
	}
	if err := json.Unmarshal(mutationBytes, &mutationPolicy); err != nil {
		t.Fatal(err)
	}
	pinnedMutations := make(map[string]bool)
	for _, entry := range mutationPolicy.Mutations {
		pinnedMutations[entry.RPC] = true
	}
	for _, method := range requiredMethods {
		name := method[strings.LastIndexByte(method, '.')+1:]
		if mutationMethod(name) != pinnedMutations[method] {
			t.Fatalf("%s fault phase differs from pinned mutation policy", method)
		}
	}
	if err := provider.(*Harness).AdvertiseLaterMethods("RunService.DeleteRun"); err != nil {
		t.Fatal(err)
	}
	withLater, err := provider.GetCapabilities(context.Background(), &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil || !slices.Equal(withLater.GetSupportedMethods(), append(slices.Clone(requiredMethods), "RunService.DeleteRun")) {
		t.Fatalf("later method advertisement = %v, %v", withLater, err)
	}
	if err := provider.(*Harness).AdvertiseLaterMethods("RunService.Unknown"); err == nil {
		t.Fatal("unknown later method advertised")
	}
	provider.(*Harness).Reset()
	withoutLater, err := provider.GetCapabilities(context.Background(), &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil || !slices.Equal(withoutLater.GetSupportedMethods(), requiredMethods) {
		t.Fatal("Reset retained later-method advertisement")
	}
	for path, advertised := range map[string]string{
		"../upstream/dolgorae-public-v1.descriptor.pb":              capabilities.GetDescriptorSha256(),
		"../upstream/dolgorae-controller-credential-v1.schema.json": capabilities.GetControllerCarrier().GetSchemaSha256(),
	} {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != advertised {
			t.Fatalf("advertised digest differs from %s", path)
		}
	}
	for _, method := range requiredMethods {
		if strings.Contains(method, "DeleteRun") || strings.Contains(method, "Continuation") {
			t.Fatalf("optional action advertised: %s", method)
		}
	}
	f := newFixture(t)
	profiles, err := f.h.ListProfiles(context.Background(), &publicv1.ListProfilesRequest{})
	if err != nil || len(profiles.GetItems()) != 1 {
		t.Fatalf("profiles = %v, %v", profiles, err)
	}
	profile, err := f.h.GetProfile(context.Background(), &publicv1.GetProfileRequest{ProfileName: "default"})
	if err != nil || profile.GetProfile().GetName() != "default" {
		t.Fatalf("profile = %v, %v", profile, err)
	}
	ws, err := f.h.InspectWorkspace(context.Background(), &publicv1.InspectWorkspaceRequest{AbsolutePath: "/workspace/one", ExpectedWorkspaceId: pointer(f.workspace.GetExpectedWorkspaceId())})
	if err != nil || ws.GetWorkspaceId() != f.workspace.GetExpectedWorkspaceId() {
		t.Fatalf("workspace identity changed: %v, %v", ws, err)
	}
	startRequest := &publicv1.StartRunRequest{Workspace: f.workspace, Controller: f.controller,
		IdempotencyKey: "start-1", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE}
	invalidLaunch := proto.Clone(startRequest).(*publicv1.StartRunRequest)
	invalidLaunch.IdempotencyKey = "invalid-mode"
	invalidLaunch.ControlMode = publicv1.ControlMode_CONTROL_MODE_UNSPECIFIED
	_, err = f.h.StartRun(context.Background(), invalidLaunch)
	requireProviderCode(t, err, "INVALID_REQUEST")
	invalidLaunch.ControlMode = publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE
	invalidLaunch.ExecutionLane = publicv1.ExecutionLane_EXECUTION_LANE_UNSPECIFIED
	_, err = f.h.StartRun(context.Background(), invalidLaunch)
	requireProviderCode(t, err, "INVALID_REQUEST")
	invalidLaunch.ExecutionLane = publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED
	invalidLaunch.Purpose = publicv1.PurposeKind_PURPOSE_KIND_REVIEW
	_, err = f.h.StartRun(context.Background(), invalidLaunch)
	requireProviderCode(t, err, "INVALID_REQUEST")
	replay, err := f.h.StartRun(context.Background(), startRequest)
	if err != nil || !replay.GetExactReplay() || replay.GetRun().GetRunId() != f.run.GetRunId() {
		t.Fatalf("start replay = %v, %v", replay, err)
	}
	changed := proto.Clone(startRequest).(*publicv1.StartRunRequest)
	changed.Purpose = publicv1.PurposeKind_PURPOSE_KIND_REVIEW
	_, err = f.h.StartRun(context.Background(), changed)
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	listed, err := f.h.ListRuns(context.Background(), &publicv1.ListRunsRequest{Workspace: f.workspace})
	if err != nil || len(listed.GetItems()) != 1 {
		t.Fatalf("runs = %v, %v", listed, err)
	}
	verification, err := f.h.VerifyController(context.Background(), &publicv1.VerifyControllerRequest{Run: f.run, Controller: f.controller})
	if err != nil || verification.GetController().GetControllerId() != "controller-1" {
		t.Fatalf("controller = %v, %v", verification, err)
	}
	wrong := proto.Clone(f.controller).(*publicv1.ControllerCarrierRef)
	wrong.ExpectedControllerId = "foreign"
	_, err = f.h.GetOrchestratedSession(context.Background(), &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: wrong})
	requireProviderCode(t, err, "CONTROLLER_MISMATCH")

	if err := f.h.RegisterController(ControllerSpec{ID: "controller-2", Generation: 1, CarrierPath: "/carrier/two"}); err != nil {
		t.Fatal(err)
	}
	plain, err := f.h.StartRun(context.Background(), &publicv1.StartRunRequest{Workspace: f.workspace,
		Controller:     &publicv1.ControllerCarrierRef{AbsoluteFilePath: "/carrier/two", ExpectedControllerId: "controller-2", ExpectedControllerGeneration: 1},
		IdempotencyKey: "plain", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.GetOrchestratedSession(context.Background(), &publicv1.GetOrchestratedSessionRequest{RootRun: &publicv1.RunRef{Workspace: f.workspace, RunId: plain.GetRun().GetRunId()}, Controller: &publicv1.ControllerCarrierRef{AbsoluteFilePath: "/carrier/two", ExpectedControllerId: "controller-2", ExpectedControllerGeneration: 1}})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
}

func TestAcceptedHistoryCapturedPagesAndProtectedArtifact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	message := "첫 줄\r\n둘째 줄\n" + strings.Repeat("가", 4200)
	request := &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "turn-1", Message: message,
		WriteIntent: publicv1.WriteIntent_WRITE_INTENT_READ, ExpectedStateRevision: f.revision(t)}
	if err := f.h.FaultNext("SubmitTurn", AfterCommit, connect.NewError(connect.CodeUnavailable, errors.New("reply lost"))); err != nil {
		t.Fatal(err)
	}
	_, err := f.h.SubmitTurn(ctx, request)
	if err == nil {
		t.Fatal("lost response did not surface")
	}
	replay, err := f.h.SubmitTurn(ctx, request)
	if err != nil || replay.GetAcceptedTurn().GetTurnId() == "" {
		t.Fatalf("accepted replay = %v, %v", replay, err)
	}
	_, err = f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller,
		IdempotencyKey: "turn-2", Message: message, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	if err := f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_FAILED); err != nil {
		t.Fatal(err)
	}
	second, err := f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller,
		IdempotencyKey: "turn-2", Message: message, ExpectedStateRevision: f.revision(t)})
	if err != nil || second.GetAcceptedTurn().GetTurnId() == replay.GetAcceptedTurn().GetTurnId() {
		t.Fatalf("separate same-text submission = %v, %v", second, err)
	}
	firstPage, err := f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1, Limit: 1})
	if err != nil || len(firstPage.GetItems()) != 1 || firstPage.NextAfterCursor == nil {
		t.Fatalf("first page = %v, %v", firstPage, err)
	}
	artifact := firstPage.GetItems()[0].GetArtifact()
	if artifact == nil {
		t.Fatal("long accepted input lost artifact")
	}
	meta, err := f.h.GetArtifact(ctx, &publicv1.GetArtifactRequest{Run: f.run, ArtifactId: artifact.GetArtifactId(), Controller: f.controller})
	if err != nil || meta.GetArtifact().GetSha256() != digest([]byte(message)) || meta.GetArtifact().GetKind() != publicv1.ArtifactKind_ARTIFACT_KIND_USER_INPUT || meta.GetMaximumChunkSize() != maximumChunkSize {
		t.Fatalf("artifact metadata = %v, %v", meta, err)
	}
	var body []byte
	for offset := uint64(0); offset < uint64(len(message)); {
		chunk, readErr := f.h.ReadArtifactChunk(ctx, &publicv1.ReadArtifactChunkRequest{Run: f.run, ArtifactId: artifact.GetArtifactId(), Controller: f.controller, Offset: offset, Length: 1024})
		if readErr != nil {
			t.Fatal(readErr)
		}
		body = append(body, chunk.GetData()...)
		offset += uint64(chunk.GetLength())
	}
	if string(body) != message {
		t.Fatal("original accepted text changed")
	}
	_, err = f.h.GetArtifact(ctx, &publicv1.GetArtifactRequest{Run: f.run, ArtifactId: artifact.GetArtifactId()})
	requireProviderCode(t, err, "CONTROLLER_MISMATCH")
	if err := f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	page, err := f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1, Limit: 20, AfterCursor: firstPage.GetNextAfterCursor()})
	if err != nil || len(page.GetItems()) != 2 {
		t.Fatalf("captured-head page must exclude later append: %v, %v", page, err)
	}
	foreign := pageCursor("timeline", "other-run", 1, 0)
	_, err = f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1, AfterCursor: foreign})
	requireProviderCode(t, err, "INVALID_REQUEST")
}

func TestAcceptedInputAndChunkBoundary(t *testing.T) {
	for _, size := range []int{4096, 4097} {
		f := newFixture(t)
		ctx := context.Background()
		message := strings.Repeat("x", size)
		_, err := f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller,
			IdempotencyKey: "boundary", Message: message, ExpectedStateRevision: f.revision(t)})
		if err != nil {
			t.Fatal(err)
		}
		page, err := f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1})
		if err != nil || len(page.GetItems()) == 0 {
			t.Fatalf("accepted input timeline = %v, %v", page, err)
		}
		item := page.GetItems()[0]
		if size == 4096 {
			if item.GetInlineText() != message || item.GetArtifact() != nil {
				t.Fatal("4096-byte input did not stay inline")
			}
			continue
		}
		artifact := item.GetArtifact()
		if artifact == nil || artifact.GetByteLength() != uint64(size) {
			t.Fatal("4097-byte input did not spill to artifact")
		}
		chunk, err := f.h.ReadArtifactChunk(ctx, &publicv1.ReadArtifactChunkRequest{Run: f.run, Controller: f.controller,
			ArtifactId: artifact.GetArtifactId(), Length: maximumChunkSize})
		if err != nil || string(chunk.GetData()) != message || !chunk.GetEof() || chunk.GetTotalByteLength() != uint64(size) {
			t.Fatalf("full artifact chunk = %v, %v", chunk, err)
		}
		tail, err := f.h.ReadArtifactChunk(ctx, &publicv1.ReadArtifactChunkRequest{Run: f.run, Controller: f.controller,
			ArtifactId: artifact.GetArtifactId(), Offset: uint64(size), Length: 1})
		if err != nil || !tail.GetEof() || tail.GetLength() != 0 {
			t.Fatalf("end-of-artifact chunk = %v, %v", tail, err)
		}
		_, err = f.h.ReadArtifactChunk(ctx, &publicv1.ReadArtifactChunkRequest{Run: f.run, Controller: f.controller,
			ArtifactId: artifact.GetArtifactId(), Offset: uint64(size + 1), Length: 1})
		requireProviderCode(t, err, "INVALID_REQUEST")
	}
}

func TestInteractionWriterAndRecovery(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: 0})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	writer, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil || writer.GetOwnerRunId() != f.run.GetRunId() {
		t.Fatalf("writer = %v, %v", writer, err)
	}
	_, err = f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "WRITER_CONFLICT")
	status, err := f.h.GetWorkspaceWriterStatus(ctx, &publicv1.GetWorkspaceWriterStatusRequest{Workspace: f.workspace})
	if err != nil || status.GetWriter().GetWriterGeneration() == 0 {
		t.Fatalf("writer status = %v, %v", status, err)
	}
	_, err = f.h.ReleaseWriter(ctx, &publicv1.ReleaseWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller,
		IdempotencyKey: "turn", Message: "answer while waiting", ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "question-1", Kind: publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT}}); err != nil {
		t.Fatal(err)
	}
	pending, err := f.h.ListPendingInteractions(ctx, &publicv1.ListPendingInteractionsRequest{Run: f.run})
	if err != nil || len(pending.GetItems()) != 1 {
		t.Fatalf("pending = %v, %v", pending, err)
	}
	interaction, err := f.h.GetControllerInteraction(ctx, &publicv1.GetControllerInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "question-1"})
	if err != nil || interaction.GetInteraction().GetSummary().GetStatus() != publicv1.InteractionStatus_INTERACTION_STATUS_PENDING {
		t.Fatalf("interaction = %v, %v", interaction, err)
	}
	answer := &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "question-1", IdempotencyKey: "answer-1", ResponseJson: []byte(`{"answer":"yes"}`)}
	resolved, err := f.h.ResolveInteraction(ctx, answer)
	if err != nil || resolved.GetResolutionReceipt() == "" {
		t.Fatalf("resolution = %v, %v", resolved, err)
	}
	again, err := f.h.ResolveInteraction(ctx, answer)
	if err != nil || again.GetResolutionReceipt() != resolved.GetResolutionReceipt() {
		t.Fatalf("resolution replay = %v, %v", again, err)
	}
	pending, err = f.h.ListPendingInteractions(ctx, &publicv1.ListPendingInteractionsRequest{Run: f.run})
	if err != nil || len(pending.GetItems()) != 0 {
		t.Fatalf("pending after resolution = %v, %v", pending, err)
	}
	_, err = f.h.InterruptTurn(ctx, &publicv1.InterruptTurnRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.PauseRun(ctx, &publicv1.PauseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.ResumeRun(ctx, &publicv1.ResumeRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClosedInteractionReplayAndRejectedWorkspace(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "approval"}}); err != nil {
		t.Fatal(err)
	}
	request := &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "approval", IdempotencyKey: "response", ResponseJson: []byte(`{"approved":true}`)}
	if err := f.h.FaultNext("ResolveInteraction", AfterCommit, connect.NewError(connect.CodeUnavailable, errors.New("lost resolution reply"))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.ResolveInteraction(ctx, request); err == nil {
		t.Fatal("lost resolution reply did not surface")
	}
	first, err := f.h.ResolveInteraction(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	if err := f.h.SetCloseProgress(f.run.GetRunId(), publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	replay, err := f.h.ResolveInteraction(ctx, request)
	if err != nil || replay.GetResolutionReceipt() != first.GetResolutionReceipt() {
		t.Fatalf("closed resolution replay = %v, %v", replay, err)
	}
	changed := proto.Clone(request).(*publicv1.ResolveInteractionRequest)
	changed.ResponseJson = []byte(`{"approved":false}`)
	_, err = f.h.ResolveInteraction(ctx, changed)
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")

	h := New(time.Unix(0, 0))
	_, err = h.InspectWorkspace(ctx, &publicv1.InspectWorkspaceRequest{AbsolutePath: "/new", ExpectedWorkspaceId: pointer("wrong")})
	requireProviderCode(t, err, "INVALID_REQUEST")
	workspace, err := h.InspectWorkspace(ctx, &publicv1.InspectWorkspaceRequest{AbsolutePath: "/new"})
	if err != nil || workspace.GetWorkspaceId() != "workspace-000001" {
		t.Fatalf("rejected inspection consumed workspace identity: %v, %v", workspace, err)
	}
}

func TestPausedTurnCompletionRequiresResume(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "turn", Message: "work", ExpectedStateRevision: f.revision(t)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.PauseRun(ctx, &publicv1.PauseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)}); err != nil {
		t.Fatal(err)
	}
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "paused-question"}}); err != nil {
		t.Fatal(err)
	}
	withInteraction, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || withInteraction.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED || withInteraction.GetRun().GetPendingInteractionCount() != 1 {
		t.Fatalf("interaction resumed paused Run: %v, %v", withInteraction, err)
	}
	if _, err := f.h.ResolveInteraction(ctx, &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "paused-question", IdempotencyKey: "answer", ResponseJson: []byte(`{"answer":true}`)}); err != nil {
		t.Fatal(err)
	}
	afterResolution, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || afterResolution.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED || afterResolution.GetRun().GetPendingInteractionCount() != 0 {
		t.Fatalf("resolution resumed paused Run: %v, %v", afterResolution, err)
	}
	if err := f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	paused, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || paused.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED || paused.GetRun().GetActiveTurn() != nil {
		t.Fatalf("completed Turn resumed paused Run: %v, %v", paused, err)
	}
	resumed, err := f.h.ResumeRun(ctx, &publicv1.ResumeRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil || resumed.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE {
		t.Fatalf("resume after terminal Turn = %v, %v", resumed, err)
	}
}

func TestResultsAndCloseOutcomes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	specialist, err := f.h.SpawnSpecialist(f.run.GetRunId(), "review")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"result-1", "result-2"} {
		if err := f.h.PublishResult(f.run.GetRunId(), &publicv1.OrchestratedSessionResult{ResultId: id, TaskId: id, SpecialistRole: "review", SpecialistRun: specialist}, []byte("public "+id)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.h.ListOrchestratedSessionResults(ctx, &publicv1.ListOrchestratedSessionResultsRequest{RootRun: f.run, Controller: f.controller, ProjectionVersion: 1, Limit: 1})
	if err != nil || page.GetCapturedPublicationHead() != 2 || page.NextPageCursor == nil {
		t.Fatalf("result page = %v, %v", page, err)
	}
	if err := f.h.PublishResult(f.run.GetRunId(), &publicv1.OrchestratedSessionResult{ResultId: "result-3", TaskId: "task-3", SpecialistRun: specialist}, []byte("later")); err != nil {
		t.Fatal(err)
	}
	second, err := f.h.ListOrchestratedSessionResults(ctx, &publicv1.ListOrchestratedSessionResultsRequest{RootRun: f.run, Controller: f.controller, ProjectionVersion: 1, PageCursor: page.NextPageCursor})
	if err != nil || len(second.GetItems()) != 1 || second.GetItems()[0].GetResultId() != "result-2" {
		t.Fatalf("captured result page = %v, %v", second, err)
	}
	if second.GetSourceRevision() != page.GetSourceRevision() || !proto.Equal(second.GetCapturedAt(), page.GetCapturedAt()) {
		t.Fatal("continuation changed the captured result snapshot")
	}
	result := second.GetItems()[0]
	if result.GetArtifactOwner().GetRunId() != f.run.GetRunId() {
		t.Fatal("result artifact owner must be Primary")
	}
	meta, err := f.h.GetArtifact(ctx, &publicv1.GetArtifactRequest{Run: f.run, Controller: f.controller, ArtifactId: result.GetArtifact().GetArtifactId()})
	if err != nil || meta.GetArtifact().GetKind() != publicv1.ArtifactKind_ARTIFACT_KIND_FINAL_RESPONSE {
		t.Fatalf("published result artifact metadata = %v, %v", meta, err)
	}
	_, err = f.h.GetArtifact(ctx, &publicv1.GetArtifactRequest{Run: f.run, ArtifactId: "missing", Controller: &publicv1.ControllerCarrierRef{}})
	requireProviderCode(t, err, "CONTROLLER_MISMATCH")
	_, err = f.h.GetArtifact(ctx, &publicv1.GetArtifactRequest{Run: f.run, ArtifactId: "missing", Controller: f.controller})
	requireProviderCode(t, err, "ARTIFACT_UNAVAILABLE")
	_, err = f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	if err := f.h.CompleteSpecialist(specialist.GetRunId()); err != nil {
		t.Fatal(err)
	}
	if err := f.h.FaultNext("CloseRun", AfterCommit, connect.NewError(connect.CodeUnavailable, errors.New("lost close reply"))); err != nil {
		t.Fatal(err)
	}
	_, err = f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err == nil {
		t.Fatal("expected ambiguous close response")
	}
	state, err := f.h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: f.controller})
	if err != nil || state.GetSession().GetCloseProgress() != publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING || state.GetSession().GetCloseOperationId() == "" {
		t.Fatalf("retained close intent = %v, %v", state, err)
	}
	closeOperationID := state.GetSession().GetCloseOperationId()
	_, err = f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	var pending *connect.Error
	if !errors.As(err, &pending) || len(pending.Details()) == 0 {
		t.Fatalf("pending close detail = %v", err)
	}
	value, detailErr := pending.Details()[0].Value()
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	if detail, ok := value.(*publicv1.DolgoraeErrorDetail); !ok || detail.GetDolgoraeErrorCode() != "SESSION_CLOSE_IN_PROGRESS" || detail.GetOperationId() != closeOperationID {
		t.Fatalf("pending close detail = %v", value)
	}
	_, err = f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, Interrupt: true, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	if err := f.h.SetCloseProgress(f.run.GetRunId(), publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_OUTCOME_UNKNOWN); err != nil {
		t.Fatal(err)
	}
	_, err = f.h.ReconcileRun(ctx, &publicv1.ReconcileRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	state, err = f.h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: f.controller})
	if err != nil || state.GetSession().GetCloseProgress() != publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_RECOVERY_REQUIRED {
		t.Fatalf("reconcile state = %v, %v", state, err)
	}
	if err := f.h.SetCloseProgress(f.run.GetRunId(), publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_OUTCOME_UNKNOWN); err != nil {
		t.Fatal(err)
	}
	_, err = f.h.RecoverRun(ctx, &publicv1.RecoverRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	state, err = f.h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: f.controller})
	if err != nil || state.GetSession().GetCloseProgress() != publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING {
		t.Fatalf("recovery state = %v, %v", state, err)
	}
	if err := f.h.SetCloseProgress(f.run.GetRunId(), publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	closed, err := f.h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: f.controller})
	if err != nil || closed.GetSession().GetLifecycle() != publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_COMPLETED || closed.GetSession().GetPublishedResultCount() != 3 {
		t.Fatalf("closed aggregate = %v, %v", closed, err)
	}
	repeated, err := f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil || repeated.GetContext().GetOperationId() != closeOperationID {
		t.Fatalf("repeat close lost retained identity: %v, %v", repeated, err)
	}
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "late"}}); err == nil {
		t.Fatal("closed Run accepted an Interaction")
	}
	_, err = f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	if err := f.h.SetCloseProgress(f.run.GetRunId(), publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING); err == nil {
		t.Fatal("terminal close regressed")
	}
	if err := f.h.PublishResult(f.run.GetRunId(), &publicv1.OrchestratedSessionResult{ResultId: "late", TaskId: "late", SpecialistRun: specialist}, []byte("late")); err == nil {
		t.Fatal("terminal close accepted a new result")
	}
	if err := f.h.SetRunLifecycle(f.run.GetRunId(), publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE); err == nil {
		t.Fatal("terminal close accepted lifecycle reset")
	}
}

func TestEventsFaultsClockAndReset(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	events := []*publicv1.DurableRunEvent{
		{Event: &publicv1.DurableRunEvent_RunStateChanged{RunStateChanged: &publicv1.RunStateChanged{Current: publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE}}},
		{Event: &publicv1.DurableRunEvent_TurnStateChanged{TurnStateChanged: &publicv1.TurnStateChanged{Current: publicv1.TurnStatus_TURN_STATUS_RUNNING}}},
		{Event: &publicv1.DurableRunEvent_FinalResponseAvailable{FinalResponseAvailable: &publicv1.FinalResponseAvailable{}}},
		{Event: &publicv1.DurableRunEvent_InteractionOpened{InteractionOpened: &publicv1.InteractionOpenedEvent{InteractionId: "i"}}},
		{Event: &publicv1.DurableRunEvent_InteractionResolved{InteractionResolved: &publicv1.InteractionResolvedEvent{InteractionId: "i"}}},
		{Event: &publicv1.DurableRunEvent_RuntimeErrorOccurred{RuntimeErrorOccurred: &publicv1.RuntimeErrorOccurred{ErrorCode: "safe"}}},
		{Event: &publicv1.DurableRunEvent_UsageReported{UsageReported: &publicv1.UsageReported{InputTokens: 1}}},
		{Event: &publicv1.DurableRunEvent_WorkspaceChanges{WorkspaceChanges: &publicv1.WorkspaceChanges{}}},
		{Event: &publicv1.DurableRunEvent_WriterStateChanged{WriterStateChanged: &publicv1.WriterStateChangedEvent{}}},
		{Event: &publicv1.DurableRunEvent_RecoveryRequired{RecoveryRequired: &publicv1.RecoveryRequiredEvent{SafeReason: "fault"}}},
		{Event: &publicv1.DurableRunEvent_CommandStarted{CommandStarted: &publicv1.CommandStarted{}}},
		{Event: &publicv1.DurableRunEvent_CommandCompleted{CommandCompleted: &publicv1.CommandCompleted{}}},
		{Event: &publicv1.DurableRunEvent_DiagnosticReported{DiagnosticReported: &publicv1.DiagnosticReported{}}},
		{Event: &publicv1.DurableRunEvent_GenerationChanged{GenerationChanged: &publicv1.GenerationChanged{ServerEpoch: 2}}},
		{Event: &publicv1.DurableRunEvent_ReasoningSuppressed{ReasoningSuppressed: &publicv1.ReasoningSuppressed{Method: "safe"}}},
	}
	for _, event := range events {
		if err := f.h.AppendEvent(f.run.GetRunId(), event); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.h.ReplayLast(f.run.GetRunId()); err != nil {
		t.Fatal(err)
	}
	var lastCursor, lastEventID string
	for i := 0; i < len(events); i++ {
		item, receiveErr := stream.Receive()
		if receiveErr != nil || item.GetDurableEvent().GetEvent() == nil {
			t.Fatalf("event %d = %v, %v", i, item, receiveErr)
		}
		lastCursor, lastEventID = item.GetDurableEvent().GetCursor(), item.GetDurableEvent().GetEventId()
	}
	replay, err := stream.Receive()
	if err != nil || !replay.GetDurableEvent().GetReplay() || replay.GetDurableEvent().GetCursor() != lastCursor || replay.GetDurableEvent().GetEventId() != lastEventID {
		t.Fatalf("duplicate event = %v, %v", replay, err)
	}
	other, err := f.h.StartRun(ctx, &publicv1.StartRunRequest{Workspace: f.workspace, Controller: f.controller,
		IdempotencyKey: "other-run", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	otherRef := &publicv1.RunRef{Workspace: f.workspace, RunId: other.GetRun().GetRunId()}
	if err := f.h.AppendEvent(otherRef.GetRunId(), events[0]); err != nil {
		t.Fatal(err)
	}
	otherStream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: otherRef})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.FailStream(f.run.GetRunId(), providerError("SLOW_CONSUMER")); err != nil {
		t.Fatal(err)
	}
	_, err = stream.Receive()
	requireProviderCode(t, err, "SLOW_CONSUMER")
	_, err = f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: "foreign-cursor"})
	requireProviderCode(t, err, "INVALID_REQUEST")
	if err := f.h.AppendEvent(f.run.GetRunId(), events[0]); err != nil {
		t.Fatal(err)
	}
	resumed, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: lastCursor})
	if err != nil {
		t.Fatal(err)
	}
	if item, receiveErr := resumed.Receive(); receiveErr != nil || item.GetDurableEvent().GetCursor() == lastCursor {
		t.Fatalf("event resume did not advance: %v, %v", item, receiveErr)
	}
	_ = resumed.Close()
	if item, err := otherStream.Receive(); err != nil || item.GetDurableEvent() == nil {
		t.Fatalf("slow stream affected another Run: %v, %v", item, err)
	}
	_, err = f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: otherRef, Controller: f.controller, ExpectedStateRevision: other.GetRun().GetStateRevision() + 1})
	if err != nil {
		t.Fatalf("slow stream blocked unary mutation: %v", err)
	}
	if err := f.h.AppendEnvelope(f.run.GetRunId(), &publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_Heartbeat{Heartbeat: &publicv1.RunEventHeartbeat{RunId: f.run.GetRunId()}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.h.AppendEnvelope(f.run.GetRunId(), &publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_StreamEnd{StreamEnd: &publicv1.RunEventStreamEnd{RunId: f.run.GetRunId(), Reason: publicv1.StreamEndReason_STREAM_END_REASON_SERVER_SHUTDOWN}}}); err != nil {
		t.Fatal(err)
	}
	if item, err := stream.Receive(); err != nil || item.GetDurableEvent() == nil {
		t.Fatalf("post-reconnect durable event = %v, %v", item, err)
	}
	if item, err := stream.Receive(); err != nil || item.GetHeartbeat() == nil {
		t.Fatalf("heartbeat = %v, %v", item, err)
	}
	if item, err := stream.Receive(); err != nil || item.GetStreamEnd() == nil {
		t.Fatalf("stream end = %v, %v", item, err)
	}
	if _, err := stream.Receive(); !errors.Is(err, io.EOF) {
		t.Fatalf("post-end receive = %v", err)
	}
	before, err := f.h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: f.controller})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.Advance(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err := f.h.SetRunLifecycle(f.run.GetRunId(), publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE); err != nil {
		t.Fatal(err)
	}
	after, err := f.h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: f.controller})
	if err != nil || !after.GetSession().GetCapturedAt().AsTime().After(before.GetSession().GetCapturedAt().AsTime()) {
		t.Fatalf("clock advancement invisible: %v, %v", after, err)
	}
	if err := f.h.Advance(-time.Second); err == nil {
		t.Fatal("clock moved backward")
	}
	if err := f.h.SetRunLifecycle(f.run.GetRunId(), publicv1.RunLifecycle(999)); err != nil {
		t.Fatal(err)
	}
	unknown, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	if _, recognized := publicv1.RunLifecycle_name[int32(unknown.GetRun().GetLifecycle())]; recognized {
		t.Fatal("unknown required lifecycle was treated as recognized")
	}
	f.h.Reset()
	_, err = f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	requireProviderCode(t, err, "INVALID_REQUEST")
}

func TestBeforeCommitAndStateBoundaries(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	initialRevision := f.revision(t)
	request := &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "turn", Message: "first", ExpectedStateRevision: initialRevision}
	if err := f.h.FaultNext("SubmitTurn ", BeforeCommit, errors.New("typo")); err == nil {
		t.Fatal("unknown fault method was accepted")
	}
	if err := f.h.FaultNext("SubmitTurn", BeforeCommit, connect.NewError(connect.CodeUnavailable, errors.New("not accepted"))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.SubmitTurn(ctx, request); err == nil {
		t.Fatal("before-commit fault did not reject")
	}
	if f.revision(t) != initialRevision {
		t.Fatal("rejected turn changed Run state")
	}
	page, err := f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1})
	if err != nil || len(page.GetItems()) != 0 {
		t.Fatalf("rejected turn entered timeline: %v, %v", page, err)
	}
	first, err := f.h.SubmitTurn(ctx, request)
	if err != nil || first.GetAcceptedTurn().GetTurnId() == "" {
		t.Fatalf("fresh retry was not accepted: %v, %v", first, err)
	}
	_, err = f.h.PauseRun(ctx, &publicv1.PauseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.ResumeRun(ctx, &publicv1.ResumeRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || state.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_RUNNING || state.GetRun().GetActiveTurn().GetTurnId() != first.GetAcceptedTurn().GetTurnId() {
		t.Fatalf("resume lost running Turn: %v, %v", state, err)
	}
	_, err = f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "next", Message: "second", ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	for _, id := range []string{"one", "two"} {
		if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: id}}); err != nil {
			t.Fatal(err)
		}
	}
	before := f.h.findRun(f.run.GetRunId()).projection.GetStamp().GetInteractionStateRevision()
	_, err = f.h.ResolveInteraction(ctx, &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "one", IdempotencyKey: "answer-one", ResponseJson: []byte(`{"answer":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	state, err = f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || state.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_WAITING_INTERACTION || state.GetRun().GetStamp().GetInteractionStateRevision() <= before {
		t.Fatalf("remaining interaction or stamp lost: %v, %v", state, err)
	}
	if err := f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	state, err = f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || state.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_WAITING_INTERACTION {
		t.Fatalf("terminal Turn cleared pending interaction: %v, %v", state, err)
	}
	_, err = f.h.ResolveInteraction(ctx, &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "two", IdempotencyKey: "answer-two", ResponseJson: []byte(`{"answer":2}`)})
	if err != nil {
		t.Fatal(err)
	}
	state, err = f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || state.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE {
		t.Fatalf("resolved Run did not return idle: %v, %v", state, err)
	}
}

func TestResetReleasesBlockedStream(t *testing.T) {
	f := newFixture(t)
	stream, err := f.h.WatchRunEvents(context.Background(), &publicv1.WatchRunEventsRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, receiveErr := stream.Receive()
		finished <- receiveErr
	}()
	f.h.Reset()
	select {
	case err := <-finished:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("retired stream = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Reset left stream blocked")
	}
}

func TestStreamCancellationAndExplicitClose(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, receiveErr := stream.Receive()
		finished <- receiveErr
	}()
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled stream = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled stream remained blocked")
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Receive(); !errors.Is(err, io.EOF) {
		t.Fatalf("closed stream = %v", err)
	}
}

func TestInterruptPauseAndAbortClose(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.h.FaultNext("GetRun", AfterCommit, errors.New("invalid phase")); err == nil {
		t.Fatal("read method accepted an after-commit fault")
	}
	_, err := f.h.ReconcileRun(ctx, &publicv1.ReconcileRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	_, err = f.h.RecoverRun(ctx, &publicv1.RecoverRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	requireProviderCode(t, err, "RUN_STATE_CONFLICT")
	_, err = f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "before-pause", Message: "pause", ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.PauseRun(ctx, &publicv1.PauseRunRequest{Run: f.run, Controller: f.controller, Interrupt: true, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || state.GetRun().GetActiveTurn() != nil || state.GetRun().GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED {
		t.Fatalf("interrupt pause retained active work: %v, %v", state, err)
	}
	_, err = f.h.ResumeRun(ctx, &publicv1.ResumeRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "abort-turn", Message: "abort", ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "before-abort"}}); err != nil {
		t.Fatal(err)
	}
	_, err = f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, Interrupt: true, ExpectedStateRevision: f.revision(t)})
	var pending *connect.Error
	if !errors.As(err, &pending) || len(pending.Details()) != 1 {
		t.Fatalf("abort close did not enter pending state: %v", err)
	}
	if err := f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_INTERRUPTED); err != nil {
		t.Fatal(err)
	}
	if err := f.h.SetCloseProgress(f.run.GetRunId(), publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED); err == nil {
		t.Fatal("abort close ignored pending approval")
	}
	_, err = f.h.ResolveInteraction(ctx, &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "before-abort", IdempotencyKey: "resolve-abort", ResponseJson: []byte(`{"deny":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.SetCloseProgress(f.run.GetRunId(), publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED); err != nil {
		t.Fatal(err)
	}
	closed, err := f.h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: f.run, Controller: f.controller})
	if err != nil || closed.GetSession().GetCloseIntent() != publicv1.SessionCloseIntent_SESSION_CLOSE_INTENT_ABORT || closed.GetSession().GetLifecycle() != publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ABORTED {
		t.Fatalf("abort close state = %v, %v", closed, err)
	}
	retained, err := f.h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: f.run, Controller: f.controller, Interrupt: true, ExpectedStateRevision: f.revision(t)})
	if err != nil || retained.GetContext().GetOperationId() != closed.GetSession().GetCloseOperationId() {
		t.Fatalf("abort replay identity = %v, %v", retained, err)
	}
}
