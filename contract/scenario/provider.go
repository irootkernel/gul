package scenario

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"google.golang.org/protobuf/proto"
)

var requiredMethods = []string{
	"ArtifactService.GetArtifact", "ArtifactService.ReadArtifactChunk",
	"ControllerService.VerifyController",
	"InteractionService.GetControllerInteraction", "InteractionService.ListPendingInteractions", "InteractionService.ResolveInteraction",
	"ObservationService.ListRunTimelineItems", "ObservationService.WatchRunEvents",
	"OrchestrationService.GetOrchestratedSession", "OrchestrationService.ListOrchestratedSessionResults",
	"RunService.CloseRun", "RunService.GetRun", "RunService.InterruptTurn", "RunService.ListRuns",
	"RunService.PauseRun", "RunService.ReconcileRun", "RunService.RecoverRun", "RunService.ResumeRun",
	"RunService.StartRun", "RunService.SubmitTurn",
	"RuntimeService.GetCapabilities", "RuntimeService.GetProfile", "RuntimeService.InspectWorkspace", "RuntimeService.ListProfiles",
	"WriterService.AcquireWriter", "WriterService.GetWorkspaceWriterStatus", "WriterService.ReleaseWriter",
}

var laterMethods = []string{
	"RunService.CreateWriteContinuation", "RunService.DeleteRun", "RunService.ForkRun",
	"RunService.SetDefaultEffort", "RunService.VerifyRun", "RuntimeService.ListProfileDiagnostics",
	"WriterService.CancelWriterHandoff", "WriterService.CommitWriterHandoff", "WriterService.PrepareWriterHandoff",
}

const pinnedDescriptorSHA256 = "28b132842bbeb48123c2b7cc529de689e6e6286b0783cbde8551d17ba4921ed5"
const maximumArtifactSize = 64 << 20
const maximumChunkSize = 65536
const maximumPageLimit = 500

func pointer[T any](value T) *T { return &value }

func (h *Harness) GetCapabilities(_ context.Context, request *publicv1.GetCapabilitiesRequest) (*publicv1.GetCapabilitiesResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("GetCapabilities"); err != nil {
		return nil, err
	}
	if request == nil || request.GetMinimumProtocolVersion() > 1 || request.GetMaximumProtocolVersion() < 1 {
		return nil, invalid()
	}
	return &publicv1.GetCapabilitiesResponse{
		Context: h.response(), DolgoraeVersion: "0.1.3",
		Protocol: &publicv1.ProtocolCapabilities{RpcProtocolVersion: 1, MinimumClientProtocolVersion: 1, MaximumClientProtocolVersion: 1,
			EventProtocolVersion: 1, TimelineProtocolVersion: 1, EventProjectionVersion: 1, GrpcErrorDetailVersion: 1,
			ProjectionProfiles: []publicv1.ProjectionProfile{publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL, publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL}},
		ControllerCarrier: &publicv1.CredentialCarrierCapabilities{SchemaId: "dolgorae.controller-credential/v1", SchemaVersion: 1,
			SchemaSha256:            "6e888023fa6f12964afbd2867832944307dc626cad3fe6c6bc517768ab98b84f",
			CarrierRootLocator:      "~/.dolgorae/controller-carriers/gul/<installation-id>",
			AcceptedControllerKinds: []publicv1.ControllerKind{publicv1.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT},
			SameUidRequired:         true, RegularFileRequired: true, SymlinksForbidden: true,
			CarrierRootPolicy: publicv1.ControllerCarrierRootPolicy_CONTROLLER_CARRIER_ROOT_POLICY_DOLGORAE_OWNED_HOME},
		Features: &publicv1.RuntimeFeatureCapabilities{PersistentRuns: true, ReaderWriterAccess: true,
			ControllerTimeline: true, DurableWriterAuthority: true, EventReplay: true,
			ArtifactRetrieval: true, ControllerBinding: true, SafeClientProjection: true,
			PublicLocalSocket: true, ControlModes: true, BrokeredIndependentSubagentRuns: true},
		Artifacts: &publicv1.ArtifactCapabilities{MaximumArtifactSize: maximumArtifactSize, MaximumChunkSize: maximumChunkSize,
			DigestVerificationRequired: true, ExactByteLengthReported: true,
			VisibilityClasses: []publicv1.ArtifactVisibility{publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY}},
		SupportedMethods:      append(slices.Clone(requiredMethods), h.laterMethods...),
		DescriptorSha256:      pinnedDescriptorSHA256,
		SupportedControlModes: []publicv1.ControlMode{publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE},
		SupportedTransports:   []publicv1.PublicTransport{publicv1.PublicTransport_PUBLIC_TRANSPORT_LOCAL_GRPC},
		ProfileLaunchMode:     publicv1.ProfileLaunchMode_PROFILE_LAUNCH_MODE_DOLGORAE_OWNED_DIRECT_EXECUTABLE,
	}, nil
}

func (h *Harness) InspectWorkspace(_ context.Context, request *publicv1.InspectWorkspaceRequest) (*publicv1.InspectWorkspaceResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("InspectWorkspace"); err != nil {
		return nil, err
	}
	if request == nil || !filepath.IsAbs(request.GetAbsolutePath()) || filepath.Clean(request.GetAbsolutePath()) != request.GetAbsolutePath() {
		return nil, invalid()
	}
	w := h.workspaces[request.GetAbsolutePath()]
	if request.ExpectedWorkspaceId != nil && (w == nil || request.GetExpectedWorkspaceId() != w.id) {
		return nil, invalid()
	}
	if w == nil {
		w = &workspace{id: h.next("workspace"), path: request.GetAbsolutePath(), runs: make(map[string]*run)}
		w.writer = &publicv1.WriterState{Context: h.response(), WorkspaceId: w.id,
			AuthorityState: publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE, StateRevision: 1}
		h.workspaces[w.path] = w
	}
	return &publicv1.InspectWorkspaceResponse{Context: h.response(), WorkspaceId: w.id,
		CanonicalPath: &publicv1.PathProjection{Value: &publicv1.PathProjection_Utf8Path{Utf8Path: w.path}},
		Mode:          publicv1.WorkspaceMode_WORKSPACE_MODE_GIT,
		Status:        publicv1.WorkspaceInspectionStatus_WORKSPACE_INSPECTION_STATUS_COMPATIBLE}, nil
}

func (h *Harness) ListProfiles(_ context.Context, _ *publicv1.ListProfilesRequest) (*publicv1.ListProfilesResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ListProfiles"); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(h.profiles))
	for name := range h.profiles {
		names = append(names, name)
	}
	slices.Sort(names)
	response := &publicv1.ListProfilesResponse{Context: h.response()}
	for _, name := range names {
		response.Items = append(response.Items, copyOf(h.profiles[name]))
	}
	return response, nil
}

func (h *Harness) GetProfile(_ context.Context, request *publicv1.GetProfileRequest) (*publicv1.GetProfileResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("GetProfile"); err != nil {
		return nil, err
	}
	if request == nil || h.profiles[request.GetProfileName()] == nil {
		return nil, invalid()
	}
	return &publicv1.GetProfileResponse{Context: h.response(), Profile: copyOf(h.profiles[request.GetProfileName()])}, nil
}

func (h *Harness) StartRun(_ context.Context, request *publicv1.StartRunRequest) (*publicv1.StartRunResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("StartRun"); err != nil {
		return nil, err
	}
	if request == nil || request.GetIdempotencyKey() == "" || request.GetProfileName() == "" ||
		request.GetControlMode() != publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE ||
		request.GetExecutionLane() != publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED {
		return nil, invalid()
	}
	w, err := h.workspaceFor(request.GetWorkspace())
	if err != nil {
		return nil, err
	}
	controller := request.GetController()
	spec, ok := h.controllers[controller.GetAbsoluteFilePath()]
	if !ok || spec.ID != controller.GetExpectedControllerId() || spec.Generation != controller.GetExpectedControllerGeneration() {
		return nil, controllerMismatch()
	}
	if h.profiles[request.GetProfileName()] == nil {
		return nil, invalid()
	}
	key := startRequestKey{w.id, spec.ID, request.GetIdempotencyKey()}
	body := copyOf(request)
	body.Context = nil
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
	if err != nil {
		return nil, invalid()
	}
	if prior := h.startKeys[key]; prior != nil {
		if h.startBody[key] != digest(encoded) {
			return nil, conflict()
		}
		response := copyOf(prior)
		response.ExactReplay = true
		return response, nil
	}
	if request.GetParent() != nil || (spec.OrchestrationLaunch && request.GetPurpose() != publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE) {
		return nil, invalid()
	}
	id := h.next("run")
	r := newRun(w, controller, "")
	r.projection = &publicv1.RunProjection{
		WorkspaceId: w.id, RunId: id, Lifecycle: publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE,
		ControlMode: request.GetControlMode(), ExecutionLane: request.GetExecutionLane(),
		Controller: &publicv1.ControllerProjection{ControllerId: spec.ID, Generation: spec.Generation,
			Kind: publicv1.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT, InstanceId: "scenario-client"},
		StateRevision: 1, StateVariant: publicv1.RunStateVariant_RUN_STATE_VARIANT_DEDICATED_READER,
		Recovery: &publicv1.RecoveryProjection{State: publicv1.RecoveryState_RECOVERY_STATE_NOT_REQUIRED, RequiredAction: publicv1.RecoveryAction_RECOVERY_ACTION_NONE},
		EffectivePolicy: &publicv1.EffectivePolicyProjection{Access: publicv1.EffectiveAccess_EFFECTIVE_ACCESS_READ,
			Verification: publicv1.PolicyVerification_POLICY_VERIFICATION_VERIFIED},
		WriterAuthority:     &publicv1.WriterAuthorityProjection{State: publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE},
		ServerLane:          &publicv1.ServerLaneProjection{Kind: request.GetExecutionLane(), State: publicv1.ServerLaneState_SERVER_LANE_STATE_READY},
		BackgroundExecution: &publicv1.BackgroundExecutionProjection{State: publicv1.BackgroundExecutionState_BACKGROUND_EXECUTION_STATE_VERIFIED_ABSENT},
		RequestedAssurance:  publicv1.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA,
		AchievedAssurance:   publicv1.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA,
		Configuration:       &publicv1.RunConfigurationProjection{ProfileName: request.GetProfileName(), Purpose: request.GetPurpose()},
	}
	r.ledgerLifecycle = r.projection.GetLifecycle()
	if spec.OrchestrationLaunch {
		r.session = &publicv1.OrchestratedSessionProjection{SessionId: h.next("session"),
			PrimaryRun: &publicv1.RunRef{Workspace: copyOf(request.GetWorkspace()), RunId: id}, AggregateRevision: 1, SourceRevision: 1,
			Lifecycle:              publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ACTIVE,
			Composition:            publicv1.OrchestratedSessionComposition_ORCHESTRATED_SESSION_COMPOSITION_STANDALONE_PRIMARY,
			ApprovalPolicy:         publicv1.OrchestratedSessionApprovalPolicy_ORCHESTRATED_SESSION_APPROVAL_POLICY_USER_APPROVAL_REQUIRED,
			SpecialistPolicyName:   spec.PolicyName,
			CloseIntent:            publicv1.SessionCloseIntent_SESSION_CLOSE_INTENT_NONE,
			CloseProgress:          publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_NONE,
			RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE,
			Availability:           publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_AVAILABLE,
			CapturedAt:             h.timestamp()}
	}
	w.runs[id] = r
	response := &publicv1.StartRunResponse{Context: h.response(), Run: r.snapshot(), IdempotencyKey: request.GetIdempotencyKey()}
	h.startKeys[key] = copyOf(response)
	h.startBody[key] = digest(encoded)
	if err := h.after("StartRun"); err != nil {
		return nil, err
	}
	return response, nil
}

func (h *Harness) ListRuns(_ context.Context, request *publicv1.ListRunsRequest) (*publicv1.ListRunsResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ListRuns"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	w, err := h.workspaceFor(request.GetWorkspace())
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(w.runs))
	for id := range w.runs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	response := &publicv1.ListRunsResponse{Context: h.response()}
	for _, id := range ids {
		r := w.runs[id]
		if request.ControllerId == nil || request.GetControllerId() == r.projection.GetController().GetControllerId() {
			response.Items = append(response.Items, r.snapshot())
		}
	}
	return response, nil
}

func (h *Harness) GetRun(_ context.Context, request *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("GetRun"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	return &publicv1.GetRunResponse{Context: h.response(), Run: r.snapshot()}, nil
}

func (h *Harness) SubmitTurn(_ context.Context, request *publicv1.SubmitTurnRequest) (*publicv1.SubmitTurnAccepted, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("SubmitTurn"); err != nil {
		return nil, err
	}
	if request == nil || request.GetIdempotencyKey() == "" || request.GetMessage() == "" || len(request.GetMessage()) > maximumArtifactSize {
		return nil, invalid()
	}
	w, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	key := request.GetIdempotencyKey()
	body := copyOf(request)
	body.Context = nil
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
	if err != nil {
		return nil, invalid()
	}
	if prior := r.turnKeys[key]; prior != nil {
		if r.turnBodies[key] != digest(encoded) {
			return nil, conflict()
		}
		return copyOf(prior), nil
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if r.projection.GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE || r.projection.GetActiveTurn() != nil || r.closeChoice != nil {
		return nil, conflict()
	}
	turnID := h.next("turn")
	turn := &publicv1.TurnProjection{RunId: r.projection.GetRunId(), ThreadId: h.next("thread"), TurnId: turnID,
		Status: publicv1.TurnStatus_TURN_STATUS_RUNNING}
	r.projection.ActiveTurn = turn
	r.projection.Lifecycle = publicv1.RunLifecycle_RUN_LIFECYCLE_RUNNING
	item := &publicv1.TimelineItem{Type: publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_USER_INPUT_ACCEPTED,
		RunId: r.projection.GetRunId(), TurnId: turnID, OccurredAt: h.timestamp(),
		ProviderItemId: pointer(h.next("item")), ProviderOrder: pointer(uint64(len(r.timeline) + 1)),
		Status: pointer(publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_ACCEPTED)}
	if len(request.GetMessage()) > 4096 {
		id := h.next("input-artifact")
		body := []byte(request.GetMessage())
		r.artifacts[id] = append([]byte(nil), body...)
		item.Content = &publicv1.TimelineItem_Artifact{Artifact: &publicv1.ArtifactRef{ArtifactId: id,
			Kind:       publicv1.ArtifactKind_ARTIFACT_KIND_USER_INPUT,
			Visibility: publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY,
			MediaType:  "text/plain; charset=utf-8", ByteLength: uint64(len(body)), Sha256: digest(body)}}
		r.artifactRefs[id] = copyOf(item.GetArtifact())
	} else {
		item.Content = &publicv1.TimelineItem_InlineText{InlineText: request.GetMessage()}
	}
	h.emit(r, &publicv1.DurableRunEvent{TurnId: pointer(turnID), Event: &publicv1.DurableRunEvent_TurnStateChanged{
		TurnStateChanged: &publicv1.TurnStateChanged{Current: publicv1.TurnStatus_TURN_STATUS_RUNNING}}})
	turn.Cursor = r.head()
	item.Cursor = r.head()
	r.timeline = append(r.timeline, item)
	response := &publicv1.SubmitTurnAccepted{Context: h.response(), AcceptedTurn: copyOf(turn), Run: r.snapshot(),
		Writer: w.snapshot(), IdempotencyKey: key, CorrelationId: h.next("correlation")}
	r.turnKeys[key] = copyOf(response)
	r.turnBodies[key] = digest(encoded)
	if err := h.after("SubmitTurn"); err != nil {
		return nil, err
	}
	return response, nil
}

func (h *Harness) InterruptTurn(_ context.Context, request *publicv1.InterruptTurnRequest) (*publicv1.RunMutationResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("InterruptTurn"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if r.projection.GetActiveTurn() == nil {
		return nil, conflict()
	}
	h.completeTurn(r, publicv1.TurnStatus_TURN_STATUS_INTERRUPTED)
	if err := h.after("InterruptTurn"); err != nil {
		return nil, err
	}
	return &publicv1.RunMutationResponse{Context: h.response(), Run: r.snapshot()}, nil
}

func (h *Harness) PauseRun(_ context.Context, request *publicv1.PauseRunRequest) (*publicv1.RunMutationResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("PauseRun"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return nil, conflict()
	}
	if request.GetInterrupt() && r.projection.GetActiveTurn() != nil {
		h.completeTurn(r, publicv1.TurnStatus_TURN_STATUS_INTERRUPTED)
	}
	r.projection.Lifecycle = publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED
	h.changed(r)
	if err := h.after("PauseRun"); err != nil {
		return nil, err
	}
	return &publicv1.RunMutationResponse{Context: h.response(), Run: r.snapshot()}, nil
}

func (h *Harness) ResumeRun(_ context.Context, request *publicv1.ResumeRunRequest) (*publicv1.RunMutationResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ResumeRun"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if r.projection.GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED {
		return nil, conflict()
	}
	r.projection.Lifecycle = lifecycleFromWork(r)
	h.changed(r)
	if err := h.after("ResumeRun"); err != nil {
		return nil, err
	}
	return &publicv1.RunMutationResponse{Context: h.response(), Run: r.snapshot()}, nil
}

func (h *Harness) CloseRun(_ context.Context, request *publicv1.CloseRunRequest) (*publicv1.RunMutationResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("CloseRun"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if r.session == nil {
		return nil, conflict()
	}
	if r.closeChoice != nil {
		if *r.closeChoice != request.GetInterrupt() {
			return nil, conflict()
		}
		if r.projection.GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
			return nil, closePending(r)
		}
		return &publicv1.RunMutationResponse{Context: &publicv1.ResponseContext{ProtocolVersion: 1, ServerInstanceId: "scenario-server", OperationId: pointer(r.session.GetCloseOperationId())}, Run: r.snapshot()}, nil
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if (r.projection.GetActiveTurn() != nil || r.session.GetNonterminalSpawnCount() != 0 || r.session.GetPendingApprovalCount() != 0) && !request.GetInterrupt() {
		return nil, conflict()
	}
	choice := request.GetInterrupt()
	r.closeChoice = &choice
	r.session.CloseOperationId = pointer(h.next("close-operation"))
	r.session.CloseProgress = publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING
	if choice {
		r.session.CloseIntent = publicv1.SessionCloseIntent_SESSION_CLOSE_INTENT_ABORT
		r.session.Lifecycle = publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ABORTING
	} else {
		r.session.CloseIntent = publicv1.SessionCloseIntent_SESSION_CLOSE_INTENT_COMPLETE
		r.session.Lifecycle = publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_COMPLETING
	}
	h.changed(r)
	if err := h.after("CloseRun"); err != nil {
		return nil, err
	}
	return nil, closePending(r)
}

func closePending(r *run) error {
	err := connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session close in progress"))
	detail, _ := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{DetailVersion: 1,
		DolgoraeErrorCode: "SESSION_CLOSE_IN_PROGRESS", Action: publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT,
		RetryClassification:    publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN,
		RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED,
		RunId:                  pointer(r.projection.GetRunId()), OperationId: pointer(r.session.GetCloseOperationId())})
	err.AddDetail(detail)
	return err
}

func (h *Harness) RecoverRun(_ context.Context, request *publicv1.RecoverRunRequest) (*publicv1.RunMutationResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("RecoverRun"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if r.session == nil || (r.session.GetCloseProgress() != publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_RECOVERY_REQUIRED &&
		r.session.GetCloseProgress() != publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_OUTCOME_UNKNOWN) {
		return nil, conflict()
	}
	r.session.CloseProgress = publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING
	r.session.UnknownOutcomeTaskCount = 0
	if *r.closeChoice {
		r.session.Lifecycle = publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ABORTING
	} else {
		r.session.Lifecycle = publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_COMPLETING
	}
	r.session.Availability = publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_AVAILABLE
	h.changed(r)
	if err := h.after("RecoverRun"); err != nil {
		return nil, err
	}
	return &publicv1.RunMutationResponse{Context: h.response(), Run: r.snapshot()}, nil
}

func (h *Harness) ReconcileRun(_ context.Context, request *publicv1.ReconcileRunRequest) (*publicv1.RunMutationResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ReconcileRun"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if r.session == nil || r.session.GetCloseProgress() != publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_OUTCOME_UNKNOWN {
		return nil, conflict()
	}
	r.session.CloseProgress = publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_RECOVERY_REQUIRED
	h.changed(r)
	if err := h.after("ReconcileRun"); err != nil {
		return nil, err
	}
	return &publicv1.RunMutationResponse{Context: h.response(), Run: r.snapshot()}, nil
}

func pageCursor(kind, runID string, head, offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s:%d:%d", kind, runID, head, offset)))
}

func parseCursor(value, kind, runID string, maximum int) (head, offset int, err error) {
	decoded, decodeErr := base64.RawURLEncoding.DecodeString(value)
	if decodeErr != nil {
		return 0, 0, invalidCursor()
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 4 || parts[0] != kind || parts[1] != runID {
		return 0, 0, invalidCursor()
	}
	head, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, invalidCursor()
	}
	offset, err = strconv.Atoi(parts[3])
	if err != nil || head < 0 || offset < 0 || offset > head || head > maximum {
		return 0, 0, invalidCursor()
	}
	return head, offset, nil
}

// Ledger cursors are decimal strings, including zero and filtered gaps.
// Session-result pagination retains its independent opaque token domain.
func ledgerCursor(value string, head uint64) (uint64, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || strconv.FormatUint(n, 10) != value || n > head {
		return 0, invalidCursor()
	}
	return n, nil
}

func boundedLimit(value uint32) int {
	if value == 0 {
		return 100
	}
	return int(value)
}

func (h *Harness) ListRunTimelineItems(_ context.Context, request *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ListRunTimelineItems"); err != nil {
		return nil, err
	}
	if request == nil || request.GetTimelineVersion() != 1 || request.GetLimit() > maximumPageLimit {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	after, err := ledgerCursor(request.GetAfterCursor(), r.projection.GetStateRevision())
	if err != nil {
		return nil, err
	}
	response := &publicv1.ListRunTimelineItemsResponse{Context: h.response(), CapturedHeadCursor: r.head(), Stamp: r.stamp()}
	limit := boundedLimit(request.GetLimit())
	for _, item := range r.timeline {
		sequence, _ := strconv.ParseUint(item.GetCursor(), 10, 64)
		if sequence <= after {
			continue
		}
		if len(response.Items) == limit {
			response.NextAfterCursor = pointer(response.Items[len(response.Items)-1].GetCursor())
			break
		}
		response.Items = append(response.Items, copyOf(item))
	}
	return response, nil
}

type eventStream struct {
	ctx           context.Context
	h             *Harness
	r             *run
	index         int
	closed        bool
	projection    publicv1.ProjectionProfile
	after         uint64
	advisoryStart int
}

func (s *eventStream) Receive() (*publicv1.RunEventEnvelope, error) {
	for {
		s.h.mu.Lock()
		if s.closed || s.r.retired {
			s.h.mu.Unlock()
			return nil, io.EOF
		}
		if queue := s.h.streamFaults[s.r.projection.GetRunId()]; len(queue) > 0 {
			s.h.streamFaults[s.r.projection.GetRunId()] = queue[1:]
			s.h.mu.Unlock()
			return nil, queue[0]
		}
		if err := s.h.before("WatchRunEvents.Receive"); err != nil {
			s.h.mu.Unlock()
			return nil, err
		}
		if s.index < len(s.r.events) {
			item := copyOf(s.r.events[s.index])
			s.index++
			if event := item.GetDurableEvent(); event != nil {
				sequence, _ := strconv.ParseUint(event.GetCursor(), 10, 64)
				if sequence <= s.after {
					s.h.mu.Unlock()
					continue
				}
				event.Projection = s.projection
				event.ProjectionVersion = 1
			} else if s.index <= s.advisoryStart {
				// Heartbeats and stream ends belong to the original subscription;
				// they are not durable replay records for a newly opened stream.
				s.h.mu.Unlock()
				continue
			}
			if item.GetStreamEnd() != nil {
				s.closed = true
			}
			s.h.mu.Unlock()
			return item, nil
		}
		notify := s.r.notify
		s.h.mu.Unlock()
		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		case <-notify:
		}
	}
}

func (s *eventStream) Close() error {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	s.closed = true
	s.h.signal(s.r)
	return nil
}

func (h *Harness) WatchRunEvents(ctx context.Context, request *publicv1.WatchRunEventsRequest) (port.EventStream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("WatchRunEvents"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	// Omitted settings are a fixture convenience: minimal, version 1.
	// Explicit unsupported values must not silently select another contract.
	projection := request.GetProjection()
	if projection == publicv1.ProjectionProfile_PROJECTION_PROFILE_UNSPECIFIED {
		projection = publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL
	}
	if (projection != publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL &&
		projection != publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL) || request.GetProjectionVersion() > 1 {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	after, err := ledgerCursor(request.GetAfterCursor(), r.projection.GetStateRevision())
	if err != nil {
		return nil, err
	}
	return &eventStream{ctx: ctx, h: h, r: r, projection: projection, after: after, advisoryStart: len(r.events)}, nil
}
