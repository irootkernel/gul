// Package scenario provides an explicitly constructed, deterministic Dolgorae
// consumer for tests. It is not a production provider adapter.
package scenario

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ port.PublicContractPort = (*Harness)(nil)

// FaultPhase distinguishes a rejected call from an accepted call whose reply
// was lost. The latter is available only for mutations.
type FaultPhase uint8

const (
	BeforeCommit FaultPhase = iota + 1
	AfterCommit
)

type fault struct {
	phase FaultPhase
	err   error
}

type workspace struct {
	id     string
	path   string
	runs   map[string]*run
	writer *publicv1.WriterState
}

type startRequestKey struct {
	workspaceID, controllerID, idempotencyKey string
}

type interactionRequestKey struct {
	interactionID, idempotencyKey string
}

type run struct {
	projection          *publicv1.RunProjection
	controller          *publicv1.ControllerCarrierRef
	timeline            []*publicv1.TimelineItem
	events              []*publicv1.RunEventEnvelope
	interactions        map[string]*publicv1.ControllerInteraction
	turnKeys            map[string]*publicv1.SubmitTurnAccepted
	turnBodies          map[string]string
	interactionKeys     map[interactionRequestKey]*publicv1.ResolveInteractionResponse
	interactionBodies   map[interactionRequestKey]string
	interactionRevision uint64
	session             *publicv1.OrchestratedSessionProjection
	results             []*publicv1.OrchestratedSessionResult
	resultRevisions     []uint64
	closeChoice         *bool
	artifacts           map[string][]byte
	artifactRefs        map[string]*publicv1.ArtifactRef
	notify              chan struct{}
	parentID            string
	retired             bool
}

// ControllerSpec stands in for a preprovisioned carrier. Launch intent is
// separate from a low-level direct Run, as it is in the producer contract.
type ControllerSpec struct {
	ID                  string
	Generation          uint64
	CarrierPath         string
	OrchestrationLaunch bool
	PolicyName          string
}

// Harness owns isolated provider state. Every response is copied before it is
// returned, so a driver cannot silently mutate the provider by editing a DTO.
type Harness struct {
	mu           sync.Mutex
	initial      time.Time
	now          time.Time
	sequence     uint64
	workspaces   map[string]*workspace
	profiles     map[string]*publicv1.ProfileProjection
	controllers  map[string]ControllerSpec
	startKeys    map[startRequestKey]*publicv1.StartRunResponse
	startBody    map[startRequestKey]string
	faults       map[string][]fault
	streamFaults map[string][]error
	laterMethods []string
}

func newRun(controller *publicv1.ControllerCarrierRef, parentID string) *run {
	return &run{controller: copyOf(controller), parentID: parentID,
		interactions: make(map[string]*publicv1.ControllerInteraction),
		turnKeys:     make(map[string]*publicv1.SubmitTurnAccepted), turnBodies: make(map[string]string),
		interactionKeys: make(map[interactionRequestKey]*publicv1.ResolveInteractionResponse), interactionBodies: make(map[interactionRequestKey]string),
		artifacts: make(map[string][]byte), artifactRefs: make(map[string]*publicv1.ArtifactRef), notify: make(chan struct{})}
}

func New(start time.Time) *Harness {
	h := &Harness{initial: start.UTC()}
	h.Reset()
	return h
}

func (h *Harness) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, w := range h.workspaces {
		for _, r := range w.runs {
			r.retired = true
			h.signal(r)
		}
	}
	h.now = h.initial
	h.sequence = 0
	h.workspaces = make(map[string]*workspace)
	h.startKeys = make(map[startRequestKey]*publicv1.StartRunResponse)
	h.startBody = make(map[startRequestKey]string)
	h.faults = make(map[string][]fault)
	h.streamFaults = make(map[string][]error)
	h.laterMethods = nil
	h.profiles = map[string]*publicv1.ProfileProjection{
		"default": {
			Name: "default", ServerKey: "scenario-server",
			Compatibility:           publicv1.ProfileCompatibility_PROFILE_COMPATIBILITY_COMPATIBLE,
			Models:                  []*publicv1.ModelCapability{{ModelId: "scenario-model", IsDefault: true, SupportedEfforts: []string{"low", "medium", "high"}}},
			SupportedExecutionLanes: []publicv1.ExecutionLane{publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED},
		},
	}
	h.controllers = make(map[string]ControllerSpec)
}

// FailStream affects only one Run's next Receive call.
func (h *Harness) FailStream(runID string, err error) error {
	if err == nil {
		return errors.New("nil stream fault")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.findRun(runID) == nil {
		return invalid()
	}
	h.streamFaults[runID] = append(h.streamFaults[runID], err)
	return nil
}

func (h *Harness) RegisterController(spec ControllerSpec) error {
	if spec.ID == "" || spec.Generation == 0 || spec.CarrierPath == "" ||
		(spec.OrchestrationLaunch && spec.PolicyName == "") {
		return errors.New("invalid scenario controller")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.controllers[spec.CarrierPath] = spec
	return nil
}

func (h *Harness) Advance(d time.Duration) error {
	if d < 0 {
		return errors.New("scenario clock cannot move backward")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.now = h.now.Add(d)
	return nil
}

// AdvertiseLaterMethods models known future capabilities without creating Gul
// port methods or first-release actions for them.
func (h *Harness) AdvertiseLaterMethods(methods ...string) error {
	seen := make(map[string]bool)
	for _, method := range methods {
		if !slices.Contains(laterMethods, method) || seen[method] {
			return errors.New("unknown or repeated later method")
		}
		seen[method] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.laterMethods = slices.Clone(methods)
	return nil
}

// FaultNext queues one method-named fault. AfterCommit can be used to prove
// that a lost response is not evidence that the mutation was rejected.
func (h *Harness) FaultNext(method string, phase FaultPhase, err error) error {
	if method == "" || (phase != BeforeCommit && phase != AfterCommit) || err == nil {
		return errors.New("invalid scenario fault")
	}
	known := method == "WatchRunEvents.Receive"
	for _, qualified := range requiredMethods {
		if strings.HasSuffix(qualified, "."+method) {
			known = true
			break
		}
	}
	if !known {
		return errors.New("unknown scenario method")
	}
	if phase == AfterCommit && !mutationMethod(method) {
		return errors.New("post-commit fault requires a mutation method")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.faults[method] = append(h.faults[method], fault{phase: phase, err: err})
	return nil
}

func mutationMethod(method string) bool {
	switch method {
	case "StartRun", "SubmitTurn", "InterruptTurn", "PauseRun", "ResumeRun", "CloseRun", "RecoverRun", "ReconcileRun", "ResolveInteraction", "AcquireWriter", "ReleaseWriter":
		return true
	default:
		return false
	}
}

func (h *Harness) before(method string) error {
	queue := h.faults[method]
	if len(queue) == 0 || queue[0].phase != BeforeCommit {
		return nil
	}
	h.faults[method] = queue[1:]
	return queue[0].err
}

func (h *Harness) after(method string) error {
	queue := h.faults[method]
	if len(queue) == 0 || queue[0].phase != AfterCommit {
		return nil
	}
	h.faults[method] = queue[1:]
	return queue[0].err
}

func (h *Harness) next(prefix string) string {
	h.sequence++
	return fmt.Sprintf("%s-%06d", prefix, h.sequence)
}

func (h *Harness) response() *publicv1.ResponseContext {
	return &publicv1.ResponseContext{ProtocolVersion: 1, ServerInstanceId: "scenario-server"}
}

func (h *Harness) timestamp() *timestamppb.Timestamp { return timestamppb.New(h.now) }

func copyOf[T proto.Message](value T) T {
	if any(value) == nil {
		return value
	}
	return proto.Clone(value).(T)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type providerErrorPolicy struct {
	code     connect.Code
	action   publicv1.RequiredClientAction
	retry    publicv1.RetryClassification
	recovery publicv1.RecoveryClassification
}

var scenarioErrorPolicies = map[string]providerErrorPolicy{
	"INVALID_ARGUMENT": {connect.CodeInvalidArgument, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_REQUEST,
		publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE},
	"EVENT_CURSOR_INVALID": {connect.CodeInvalidArgument, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_REQUEST,
		publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE},
	"RUN_STATE_CONFLICT": {connect.CodeAborted, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT,
		publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED},
	"CONTROLLER_MISMATCH": {connect.CodePermissionDenied, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_VERIFY_CONTROLLER,
		publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE},
	"ARTIFACT_NOT_FOUND": {connect.CodeNotFound, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_ABORT,
		publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE},
	"WRITER_BUSY": {connect.CodeAborted, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT,
		publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE},
	"SLOW_CONSUMER": {connect.CodeResourceExhausted, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONNECT_FROM_COMMITTED_CURSOR,
		publicv1.RetryClassification_RETRY_CLASSIFICATION_TOKENLESS_AFTER_SNAPSHOT, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED},
}

func providerError(name string) error {
	policy, ok := scenarioErrorPolicies[name]
	if !ok {
		panic("unknown scenario error code: " + name)
	}
	return errorWithPolicy(name, policy)
}

func errorWithPolicy(name string, policy providerErrorPolicy) error {
	err := connect.NewError(policy.code, errors.New("scenario provider rejected request"))
	detail, detailErr := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{
		DetailVersion:          1,
		DolgoraeErrorCode:      name,
		Action:                 policy.action,
		RetryClassification:    policy.retry,
		RecoveryClassification: policy.recovery,
	})
	if detailErr != nil {
		panic(detailErr)
	}
	err.AddDetail(detail)
	return err
}

func invalid() error {
	return providerError("INVALID_ARGUMENT")
}

func invalidCursor() error {
	return providerError("EVENT_CURSOR_INVALID")
}

func conflict() error {
	return providerError("RUN_STATE_CONFLICT")
}

func controllerMismatch() error {
	return providerError("CONTROLLER_MISMATCH")
}

func (h *Harness) workspaceFor(ref *publicv1.WorkspaceRef) (*workspace, error) {
	if ref == nil || ref.GetAbsolutePath() == "" {
		return nil, invalid()
	}
	w := h.workspaces[ref.GetAbsolutePath()]
	if w == nil || w.id != ref.GetExpectedWorkspaceId() {
		return nil, invalid()
	}
	return w, nil
}

func (h *Harness) runFor(ref *publicv1.RunRef) (*workspace, *run, error) {
	if ref == nil || ref.GetRunId() == "" {
		return nil, nil, invalid()
	}
	w, err := h.workspaceFor(ref.GetWorkspace())
	if err != nil {
		return nil, nil, err
	}
	r := w.runs[ref.GetRunId()]
	if r == nil {
		return nil, nil, invalid()
	}
	return w, r, nil
}

func checkController(r *run, ref *publicv1.ControllerCarrierRef) error {
	if ref == nil || ref.GetExpectedControllerId() == "" ||
		ref.GetExpectedControllerId() != r.controller.GetExpectedControllerId() ||
		ref.GetExpectedControllerGeneration() != r.controller.GetExpectedControllerGeneration() ||
		ref.GetAbsoluteFilePath() != r.controller.GetAbsoluteFilePath() {
		return controllerMismatch()
	}
	return nil
}

func checkRevision(r *run, revision uint64) error {
	if revision != r.projection.GetStateRevision() {
		return conflict()
	}
	return nil
}

func (h *Harness) changed(r *run) {
	r.projection.StateRevision++
	r.projection.Stamp = &publicv1.ProjectionStamp{
		CapturedHeadCursor:       r.projection.GetEventCursor(),
		RunStateRevision:         r.projection.GetStateRevision(),
		WriterStateRevision:      r.projection.GetWriterAuthority().GetWriterGeneration(),
		InteractionStateRevision: r.interactionRevision,
	}
	if r.session != nil {
		r.session.AggregateRevision++
		r.session.SourceRevision = r.session.AggregateRevision
		r.session.CapturedAt = h.timestamp()
	}
}

func (h *Harness) emit(r *run, event *publicv1.DurableRunEvent) {
	event = copyOf(event)
	event.Cursor = h.next("event")
	event.EventId = h.next("event-id")
	event.OccurredAt = h.timestamp()
	event.WorkspaceId = r.projection.GetWorkspaceId()
	event.RunId = r.projection.GetRunId()
	event.ServerKey = "scenario-server"
	event.ServerEpoch = 1
	event.Projection = publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL
	event.ProjectionVersion = 1
	r.projection.EventCursor = event.Cursor
	r.events = append(r.events, &publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_DurableEvent{DurableEvent: event}})
	h.signal(r)
}

func (h *Harness) signal(r *run) {
	close(r.notify)
	r.notify = make(chan struct{})
}

// AppendEvent permits a driver to exercise every pinned durable event variant.
func (h *Harness) AppendEvent(runID string, event *publicv1.DurableRunEvent) error {
	if event == nil || event.GetEvent() == nil {
		return invalid()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(runID)
	if r == nil {
		return invalid()
	}
	h.emit(r, event)
	h.changed(r)
	return nil
}

// AppendEnvelope supplies a heartbeat or stream end at a controlled boundary.
func (h *Harness) AppendEnvelope(runID string, envelope *publicv1.RunEventEnvelope) error {
	if envelope == nil || (envelope.GetHeartbeat() == nil && envelope.GetStreamEnd() == nil) {
		return invalid()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(runID)
	if r == nil {
		return invalid()
	}
	r.events = append(r.events, copyOf(envelope))
	h.signal(r)
	return nil
}

// ReplayLast repeats the last durable event with its original identity.
func (h *Harness) ReplayLast(runID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(runID)
	if r == nil || len(r.events) == 0 {
		return conflict()
	}
	last := r.events[len(r.events)-1].GetDurableEvent()
	if last == nil {
		return conflict()
	}
	copy := copyOf(last)
	copy.Replay = true
	r.events = append(r.events, &publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_DurableEvent{DurableEvent: copy}})
	h.signal(r)
	return nil
}

func (h *Harness) findRun(id string) *run {
	for _, w := range h.workspaces {
		if r := w.runs[id]; r != nil {
			return r
		}
	}
	return nil
}

// CompleteTurn records an authoritative terminal Turn without involving Gul's
// action evaluator. Accepted history remains after failure or interruption.
func (h *Harness) CompleteTurn(runID string, status publicv1.TurnStatus) error {
	if status != publicv1.TurnStatus_TURN_STATUS_COMPLETED &&
		status != publicv1.TurnStatus_TURN_STATUS_FAILED &&
		status != publicv1.TurnStatus_TURN_STATUS_INTERRUPTED {
		return invalid()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(runID)
	if r == nil || r.projection.GetActiveTurn() == nil {
		return conflict()
	}
	h.completeTurn(r, status)
	return nil
}

func (h *Harness) completeTurn(r *run, status publicv1.TurnStatus) {
	runID := r.projection.GetRunId()
	turn := r.projection.ActiveTurn
	turn.Status = status
	r.timeline = append(r.timeline, &publicv1.TimelineItem{
		Type:   publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_TURN_TERMINAL,
		Cursor: h.next("timeline"), RunId: runID, TurnId: turn.GetTurnId(),
		OccurredAt: h.timestamp(), Status: &[]publicv1.TimelineItemStatus{terminalItemStatus(status)}[0],
	})
	r.projection.ActiveTurn = nil
	if r.projection.GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED {
		if r.projection.GetPendingInteractionCount() != 0 {
			r.projection.Lifecycle = publicv1.RunLifecycle_RUN_LIFECYCLE_WAITING_INTERACTION
		} else {
			r.projection.Lifecycle = publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE
		}
	}
	h.emit(r, &publicv1.DurableRunEvent{Event: &publicv1.DurableRunEvent_TurnStateChanged{TurnStateChanged: &publicv1.TurnStateChanged{Current: status}}})
	h.changed(r)
}

func terminalItemStatus(status publicv1.TurnStatus) publicv1.TimelineItemStatus {
	switch status {
	case publicv1.TurnStatus_TURN_STATUS_COMPLETED:
		return publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_COMPLETED
	case publicv1.TurnStatus_TURN_STATUS_FAILED:
		return publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_FAILED
	default:
		return publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_INTERRUPTED
	}
}

// OpenInteraction simulates a provider-owned pending Controller request.
func (h *Harness) OpenInteraction(runID string, interaction *publicv1.ControllerInteraction) error {
	if interaction == nil || interaction.GetSummary().GetInteractionId() == "" {
		return invalid()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(runID)
	if r == nil || r.closeChoice != nil || r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED ||
		r.interactions[interaction.GetSummary().GetInteractionId()] != nil {
		return conflict()
	}
	item := copyOf(interaction)
	item.Summary.RunId = runID
	item.Summary.Status = publicv1.InteractionStatus_INTERACTION_STATUS_PENDING
	item.Summary.CreatedAt = h.timestamp()
	r.interactions[item.Summary.GetInteractionId()] = item
	r.projection.PendingInteractionCount++
	r.interactionRevision++
	if r.session != nil {
		r.session.PendingApprovalCount++
	}
	if r.projection.GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED {
		r.projection.Lifecycle = publicv1.RunLifecycle_RUN_LIFECYCLE_WAITING_INTERACTION
	}
	h.emit(r, &publicv1.DurableRunEvent{Event: &publicv1.DurableRunEvent_InteractionOpened{InteractionOpened: &publicv1.InteractionOpenedEvent{InteractionId: item.Summary.GetInteractionId(), Kind: item.Summary.GetKind()}}})
	h.changed(r)
	return nil
}

// SpawnSpecialist creates a provider-owned observer-only member. Its carrier
// is never exposed through the consumer's Primary Controller.
func (h *Harness) SpawnSpecialist(rootID, role string) (*publicv1.RunRef, error) {
	if role == "" {
		return nil, invalid()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	root := h.findRun(rootID)
	if root == nil || root.session == nil || root.closeChoice != nil {
		return nil, conflict()
	}
	w := h.workspaces[h.workspacePath(root.projection.GetWorkspaceId())]
	id := h.next("specialist")
	ref := &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: w.path, ExpectedWorkspaceId: w.id}, RunId: id}
	child := newRun(&publicv1.ControllerCarrierRef{}, rootID)
	child.projection = &publicv1.RunProjection{WorkspaceId: w.id, RunId: id,
		Lifecycle:     publicv1.RunLifecycle_RUN_LIFECYCLE_RUNNING,
		ControlMode:   publicv1.ControlMode_CONTROL_MODE_MANAGED_AGENT,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED,
		StateRevision: 1, Stamp: &publicv1.ProjectionStamp{RunStateRevision: 1},
		Configuration: &publicv1.RunConfigurationProjection{Purpose: publicv1.PurposeKind_PURPOSE_KIND_WORKFLOW_STAGE,
			PurposeLabel: pointer(role), Parent: &publicv1.ParentRefProjection{Namespace: "run", Kind: "primary", Id: rootID}}}
	w.runs[id] = child
	root.session.NonretiredMemberCount++
	root.session.NonterminalSpawnCount++
	h.changed(root)
	return ref, nil
}

func (h *Harness) CompleteSpecialist(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	child := h.findRun(id)
	if child == nil || child.parentID == "" || child.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return conflict()
	}
	root := h.findRun(child.parentID)
	child.projection.Lifecycle = publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED
	h.changed(child)
	root.session.NonterminalSpawnCount--
	h.changed(root)
	return nil
}

// SetRunLifecycle can inject an unknown required enum value so a consumer's
// compatibility gate can prove it fails closed.
func (h *Harness) SetRunLifecycle(runID string, lifecycle publicv1.RunLifecycle) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(runID)
	if r == nil {
		return invalid()
	}
	if r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return conflict()
	}
	r.projection.Lifecycle = lifecycle
	h.changed(r)
	return nil
}

// PublishResult exposes a public result and its Primary-owned artifact.
func (h *Harness) PublishResult(rootID string, item *publicv1.OrchestratedSessionResult, body []byte) error {
	if item == nil || item.GetResultId() == "" || item.GetTaskId() == "" || len(body) > maximumArtifactSize || !utf8.Valid(body) {
		return invalid()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(rootID)
	if r == nil || r.session == nil || r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return conflict()
	}
	if item.GetSpecialistRun() == nil || h.findRun(item.GetSpecialistRun().GetRunId()) == nil ||
		h.findRun(item.GetSpecialistRun().GetRunId()).parentID != rootID {
		return conflict()
	}
	for _, existing := range r.results {
		if existing.GetResultId() == item.GetResultId() {
			return conflict()
		}
	}
	result := copyOf(item)
	result.PublicationOrder = uint64(len(r.results) + 1)
	result.PublishedAt = h.timestamp()
	result.ByteLength = uint64(len(body))
	result.Sha256 = digest(body)
	result.Format = publicv1.OrchestratedResultFormat_ORCHESTRATED_RESULT_FORMAT_UTF8_TEXT
	artifactID := h.next("result-artifact")
	result.Artifact = &publicv1.ArtifactRef{ArtifactId: artifactID, Kind: publicv1.ArtifactKind_ARTIFACT_KIND_FINAL_RESPONSE,
		Visibility: publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY, MediaType: "text/plain; charset=utf-8",
		ByteLength: uint64(len(body)), Sha256: result.Sha256}
	result.ArtifactOwner = &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: h.workspacePath(r.projection.GetWorkspaceId()), ExpectedWorkspaceId: r.projection.GetWorkspaceId()}, RunId: rootID}
	r.artifacts[artifactID] = append([]byte(nil), body...)
	r.artifactRefs[artifactID] = copyOf(result.Artifact)
	r.results = append(r.results, result)
	r.session.PublishedResultCount = uint64(len(r.results))
	h.changed(r)
	r.resultRevisions = append(r.resultRevisions, r.session.GetSourceRevision())
	return nil
}

func (h *Harness) workspacePath(id string) string {
	for _, w := range h.workspaces {
		if w.id == id {
			return w.path
		}
	}
	return ""
}

// SetCloseProgress advances a retained close intent through pending, unknown,
// recovery-required, and confirmed states without pretending one RPC proves it.
func (h *Harness) SetCloseProgress(rootID string, progress publicv1.SessionCloseProgress) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.findRun(rootID)
	if r == nil || r.session == nil || r.closeChoice == nil {
		return conflict()
	}
	if r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return conflict()
	}
	switch progress {
	case publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING,
		publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_OUTCOME_UNKNOWN,
		publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_RECOVERY_REQUIRED,
		publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED,
		publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED:
	default:
		return invalid()
	}
	if (*r.closeChoice && progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED) ||
		(!*r.closeChoice && progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED) {
		return conflict()
	}
	if (progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED || progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED) &&
		(r.projection.GetActiveTurn() != nil || r.session.GetNonterminalSpawnCount() != 0 ||
			r.session.GetPendingApprovalCount() != 0 || r.session.GetUnknownOutcomeTaskCount() != 0) {
		return conflict()
	}
	r.session.CloseProgress = progress
	if progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED || progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED {
		r.projection.Lifecycle = publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED
		if progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED {
			r.session.Lifecycle = publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_COMPLETED
		} else {
			r.session.Lifecycle = publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ABORTED
		}
		r.session.Availability = publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_AVAILABLE
	} else if progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_RECOVERY_REQUIRED {
		r.session.Lifecycle = publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_RECOVERING
		r.session.Availability = publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_RECOVERY_REQUIRED
	} else if progress == publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_OUTCOME_UNKNOWN {
		r.session.UnknownOutcomeTaskCount = 1
		r.session.Availability = publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_RECOVERY_REQUIRED
	}
	h.changed(r)
	return nil
}
