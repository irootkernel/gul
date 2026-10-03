package contractprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This wire fixture models the pinned producer's ownerless Writer projection;
// its absent Run stamp domains must not be replaced with the current Run's.
type wireActions struct {
	run                                     *pb.RunProjection
	writer                                  *pb.WriterState
	profile                                 *pb.ProfileProjection
	aggregate                               *pb.OrchestratedSessionProjection
	caps                                    *pb.GetCapabilitiesResponse
	sensitive, acquire, release, interrupts int
	requestRevision                         uint64
	mutationErr                             error
}

func fixture() (*wireActions, action.Bound) {
	b := action.Bound{Binding: session.Binding{SubjectID: "owner", ID: "session", RunID: "run", ProviderSessionID: "aggregate"}, Workspace: workspace.Attachment{ProviderID: "workspace", CanonicalRoot: "/workspace"}, Carrier: session.Carrier{ControllerID: "controller", Generation: 1, AbsolutePath: "/private/carrier"}}
	s := &pb.ProjectionStamp{CapturedHeadCursor: "4", RunStateRevision: 4, WriterStateRevision: 2, InteractionStateRevision: 3}
	f := &wireActions{
		run:       &pb.RunProjection{WorkspaceId: "workspace", RunId: "run", Lifecycle: pb.RunLifecycle_RUN_LIFECYCLE_IDLE, ControlMode: pb.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: pb.ExecutionLane_EXECUTION_LANE_DEDICATED, Controller: &pb.ControllerProjection{ControllerId: "controller", Generation: 1, Kind: pb.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT}, Thread: &pb.ThreadProjection{ThreadId: "thread", ThreadGeneration: 1}, EventCursor: "4", Recovery: &pb.RecoveryProjection{State: pb.RecoveryState_RECOVERY_STATE_NOT_REQUIRED, RequiredAction: pb.RecoveryAction_RECOVERY_ACTION_NONE}, EffectivePolicy: &pb.EffectivePolicyProjection{Access: pb.EffectiveAccess_EFFECTIVE_ACCESS_READ, Verification: pb.PolicyVerification_POLICY_VERIFICATION_VERIFIED}, WriterAuthority: &pb.WriterAuthorityProjection{State: pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE, ReconciliationAction: pb.ReconciliationAction_RECONCILIATION_ACTION_NONE}, StateRevision: 4, StateVariant: pb.RunStateVariant_RUN_STATE_VARIANT_DEDICATED_READER, ServerLane: &pb.ServerLaneProjection{Kind: pb.ExecutionLane_EXECUTION_LANE_DEDICATED, State: pb.ServerLaneState_SERVER_LANE_STATE_READY}, BackgroundExecution: &pb.BackgroundExecutionProjection{State: pb.BackgroundExecutionState_BACKGROUND_EXECUTION_STATE_VERIFIED_ABSENT}, RequestedAssurance: pb.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA, AchievedAssurance: pb.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA, Configuration: &pb.RunConfigurationProjection{ProfileName: "profile"}, Stamp: s},
		writer:    &pb.WriterState{WorkspaceId: "workspace", AuthorityState: pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE, WriterGeneration: 7, EffectiveAccess: pb.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN, PolicyVerification: pb.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED, ReconciliationAction: pb.ReconciliationAction_RECONCILIATION_ACTION_NONE, StateRevision: 2, Stamp: &pb.ProjectionStamp{WriterStateRevision: 2}},
		profile:   &pb.ProfileProjection{Name: "profile", Compatibility: pb.ProfileCompatibility_PROFILE_COMPATIBILITY_COMPATIBLE, SupportedExecutionLanes: []pb.ExecutionLane{pb.ExecutionLane_EXECUTION_LANE_DEDICATED}, MaximumAssurance: pb.AssuranceLevel_ASSURANCE_LEVEL_STRONG_PROCESS_CONTAINMENT, AccessPolicyTransition: pb.SupportState_SUPPORT_STATE_SUPPORTED, BackgroundExecution: &pb.BackgroundExecutionCapabilities{Support: pb.SupportState_SUPPORT_STATE_SUPPORTED}},
		aggregate: &pb.OrchestratedSessionProjection{SessionId: "aggregate", PrimaryRun: ref(b), AggregateRevision: 91, Lifecycle: pb.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ACTIVE, NonretiredMemberCount: 1, CloseIntent: pb.SessionCloseIntent_SESSION_CLOSE_INTENT_NONE, CloseProgress: pb.SessionCloseProgress_SESSION_CLOSE_PROGRESS_NONE, RecoveryClassification: pb.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE, RequiredAction: pb.RequiredClientAction_REQUIRED_CLIENT_ACTION_NONE, CapturedAt: timestamppb.Now(), SourceRevision: 4, Availability: pb.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_AVAILABLE},
		caps:      &pb.GetCapabilitiesResponse{Lanes: &pb.LaneCapabilities{Items: []*pb.LaneCapability{{Lane: pb.ExecutionLane_EXECUTION_LANE_DEDICATED, WriterSupport: true}}}, Protocol: &pb.ProtocolCapabilities{TimelineProtocolVersion: 1}, Features: &pb.RuntimeFeatureCapabilities{PersistentRuns: true, ControllerBinding: true, SafeClientProjection: true, ControlModes: true, ReaderWriterAccess: true, DurableWriterAuthority: true, FirstWriteViaSubmitTurn: true}, AccessPolicyTransition: pb.SupportState_SUPPORT_STATE_SUPPORTED, SupportedMethods: []string{"RunService.SubmitTurn", "WriterService.AcquireWriter", "WriterService.ReleaseWriter", "RunService.InterruptTurn", "InteractionService.ResolveInteraction", "RunService.RecoverRun", "RunService.ReconcileRun", "ControllerService.VerifyController", "RunService.PauseRun", "RunService.ResumeRun", "RunService.CloseRun"}},
	}
	return f, b
}
func (f *wireActions) GetRun(_ context.Context, q *pb.GetRunRequest) (*pb.GetRunResponse, error) {
	if q.GetRun().GetRunId() != "run" {
		return nil, errors.New("wrong run")
	}
	return &pb.GetRunResponse{Run: proto.Clone(f.run).(*pb.RunProjection)}, nil
}
func (f *wireActions) GetWorkspaceWriterStatus(_ context.Context, q *pb.GetWorkspaceWriterStatusRequest) (*pb.GetWorkspaceWriterStatusResponse, error) {
	if q.GetWorkspace().GetExpectedWorkspaceId() != "workspace" {
		return nil, errors.New("wrong workspace")
	}
	return &pb.GetWorkspaceWriterStatusResponse{Writer: proto.Clone(f.writer).(*pb.WriterState)}, nil
}
func (f *wireActions) GetProfile(_ context.Context, q *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	if q.ProfileName != "profile" {
		return nil, errors.New("wrong profile")
	}
	return &pb.GetProfileResponse{Profile: proto.Clone(f.profile).(*pb.ProfileProjection)}, nil
}
func (f *wireActions) ListPendingInteractions(context.Context, *pb.ListPendingInteractionsRequest) (*pb.ListPendingInteractionsResponse, error) {
	return &pb.ListPendingInteractionsResponse{Stamp: proto.Clone(f.run.Stamp).(*pb.ProjectionStamp)}, nil
}
func (f *wireActions) ListRunTimelineItems(_ context.Context, q *pb.ListRunTimelineItemsRequest) (*pb.ListRunTimelineItemsResponse, error) {
	f.sensitive++
	if q.GetController().GetAbsoluteFilePath() != "/private/carrier" || q.AfterCursor != "4" || q.Limit != 1 || q.TimelineVersion != 1 {
		return nil, errors.New("wrong bounded authorized timeline request")
	}
	return &pb.ListRunTimelineItemsResponse{CapturedHeadCursor: "4", Stamp: proto.Clone(f.run.Stamp).(*pb.ProjectionStamp)}, nil
}
func (f *wireActions) GetOrchestratedSession(_ context.Context, q *pb.GetOrchestratedSessionRequest) (*pb.GetOrchestratedSessionResponse, error) {
	f.sensitive++
	if q.GetController().GetExpectedControllerId() != "controller" || q.GetRootRun().GetRunId() != "run" {
		return nil, errors.New("wrong authorized aggregate")
	}
	return &pb.GetOrchestratedSessionResponse{Session: proto.Clone(f.aggregate).(*pb.OrchestratedSessionProjection)}, nil
}
func (f *wireActions) AcquireWriter(_ context.Context, q *pb.AcquireWriterRequest) (*pb.WriterState, error) {
	f.acquire++
	f.requestRevision = q.ExpectedStateRevision
	if q.GetController().GetExpectedControllerGeneration() != 1 {
		return nil, errors.New("wrong carrier")
	}
	if f.mutationErr != nil {
		return nil, f.mutationErr
	}
	return proto.Clone(f.writer).(*pb.WriterState), nil
}
func (f *wireActions) ReleaseWriter(_ context.Context, q *pb.ReleaseWriterRequest) (*pb.WriterState, error) {
	f.release++
	f.requestRevision = q.ExpectedStateRevision
	released := proto.Clone(f.writer).(*pb.WriterState)
	released.OwnerRunId = nil
	released.AuthorityState = pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE
	released.EffectiveAccess = pb.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN
	released.PolicyVerification = pb.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED
	released.ExecutionLane = 0
	released.RequestedAssurance = 0
	released.AchievedAssurance = 0
	released.Stamp = &pb.ProjectionStamp{WriterStateRevision: f.writer.StateRevision}
	return released, f.mutationErr
}
func evaluateWire(t *testing.T, f *wireActions, b action.Bound) action.Evaluation {
	t.Helper()
	p, err := New(f, f.caps)
	if err != nil {
		t.Fatal(err)
	}
	in, err := p.Read(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	in.Local = action.LocalState{Ownership: action.OwnedSession, Credential: action.Healthy, Operation: action.NoOperation}
	in.Request = action.Request{Intent: action.IntentRead, CloseIntent: action.CompleteSession}
	return action.Evaluate(in)
}
func TestWireActionReadsPreserveOwnerlessAndAggregateDomains(t *testing.T) {
	f, b := fixture()
	got := evaluateWire(t, f, b)
	if !got.Flags.CanAcquireWriter || !got.Flags.CanSubmitRead || !got.Flags.CanRequestSessionClose || got.Writer.Owner != action.NoOwner || got.Writer.Generation != 7 || f.sensitive != 2 {
		t.Fatal(got, f.sensitive)
	}
	f.aggregate.AggregateRevision = 9999
	if !evaluateWire(t, f, b).Flags.CanRequestSessionClose {
		t.Fatal("aggregate revision treated as Run revision")
	}
	f.aggregate.CapturedAt = timestamppb.New(time.Now().Add(-time.Minute))
	if got := evaluateWire(t, f, b); got.Flags.CanRequestSessionClose || !got.Flags.RequiresFreshSnapshot {
		t.Fatal(got)
	}
}
func TestWireMissingCredentialKeepsWriterVisibleWithoutSensitiveReads(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		f, b := fixture()
		if mismatch {
			b.Carrier.Generation++
		} else {
			b.Carrier = session.Carrier{}
		}
		p, _ := New(f, f.caps)
		in, err := p.Read(t.Context(), b)
		if err != nil || in.Writer.Generation != 7 || in.ControllerMatches || in.Checked || f.sensitive != 0 {
			t.Fatal(in, err, f.sensitive)
		}
	}
}
func TestWireMalformedIndependentFactsNeverEnableActions(t *testing.T) {
	for _, change := range []func(*wireActions){
		func(f *wireActions) { f.run.StateRevision++ }, func(f *wireActions) { f.run.EffectivePolicy = nil }, func(f *wireActions) { f.run.Lifecycle = 99 }, func(f *wireActions) { f.run.ServerLane.State = 99 }, func(f *wireActions) { f.run.ProtoReflect().SetUnknown([]byte{0xf8, 0x7, 0x1}) }, func(f *wireActions) { f.writer.StateRevision++ }, func(f *wireActions) { f.writer.Stamp.RunStateRevision = 4 }, func(f *wireActions) { f.profile.AccessPolicyTransition = 0 }, func(f *wireActions) { f.aggregate.PrimaryRun.RunId = "foreign" }, func(f *wireActions) { f.aggregate.RequiredAction = 0 },
	} {
		f, b := fixture()
		change(f)
		p, err := New(f, f.caps)
		if err != nil {
			t.Fatal(err)
		}
		in, err := p.Read(t.Context(), b)
		if err != nil {
			continue
		}
		in.Local = action.LocalState{Ownership: action.OwnedSession, Credential: action.Healthy, Operation: action.NoOperation}
		in.Request = action.Request{Intent: action.IntentRead, CloseIntent: action.NoCloseIntent}
		got := action.Evaluate(in)
		if got.Flags.CanAcquireWriter || got.Flags.CanSubmitRead || got.Flags.CanRequestSessionClose {
			t.Fatal("malformed input admitted", got)
		}
	}
}
func TestWireMutationsForwardRevisionOnceAndClassifyTypedFailures(t *testing.T) {
	f, b := fixture()
	p, _ := New(f, f.caps)
	if _, err := p.Release(t.Context(), b, 37); err != nil || f.release != 1 || f.requestRevision != 37 {
		t.Fatal(err)
	}
	for _, code := range []string{"WRITER_BUSY", "ACCESS_TRANSITION_UNSUPPORTED"} {
		rpc := connect.NewError(connect.CodeFailedPrecondition, errors.New("private-provider-canary"))
		required := pb.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT
		if code == "ACCESS_TRANSITION_UNSUPPORTED" {
			required = pb.RequiredClientAction_REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION
		}
		detail, _ := connect.NewErrorDetail(&pb.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: code, Action: required, RetryClassification: pb.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, RecoveryClassification: pb.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE})
		rpc.AddDetail(detail)
		f.mutationErr = rpc
		before := f.acquire
		_, err := p.Acquire(t.Context(), b, 41)
		expected := action.ErrWriterBusy
		if code == "ACCESS_TRANSITION_UNSUPPORTED" {
			expected = action.ErrUnsupportedTransition
		}
		if err != expected || f.acquire != before+1 || f.requestRevision != 41 {
			t.Fatal(err, f.acquire)
		}
	}
	if safeError(errors.New("WRITER_BUSY")) != action.ErrUnavailable {
		t.Fatal("parsed diagnostic string")
	}
}

func TestWireAcceptedWriterProjectionPreservesProviderAuthority(t *testing.T) {
	f, b := fixture()
	owner := "run"
	f.writer = &pb.WriterState{WorkspaceId: "workspace", OwnerRunId: &owner, AuthorityState: pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE, WriterGeneration: 8, EffectiveAccess: pb.EffectiveAccess_EFFECTIVE_ACCESS_WRITE, PolicyVerification: pb.PolicyVerification_POLICY_VERIFICATION_VERIFIED, ExecutionLane: pb.ExecutionLane_EXECUTION_LANE_DEDICATED, RequestedAssurance: pb.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA, AchievedAssurance: pb.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA, ReconciliationAction: pb.ReconciliationAction_RECONCILIATION_ACTION_NONE, StateRevision: 3, Stamp: &pb.ProjectionStamp{CapturedHeadCursor: "5", RunStateRevision: 5, WriterStateRevision: 3, InteractionStateRevision: 3}}
	p, _ := New(f, f.caps)
	got, err := p.Acquire(t.Context(), b, 4)
	if err != nil || got.Owner != action.ThisSession || got.Generation != 8 || got.Authority != action.Active || got.Access != action.Write || got.Revision != 3 || f.acquire != 1 || f.requestRevision != 4 {
		t.Fatal(got, err)
	}
	f.writer.Stamp = nil
	if _, err := p.Acquire(t.Context(), b, 5); err != action.ErrBlocked {
		t.Fatal("missing accepted projection stamp", err)
	}
}

func TestPinnedThreadlessPolicyAllowsFirstWriteOnlyThroughSubmit(t *testing.T) {
	f, b := fixture()
	f.run.Thread = nil
	f.run.StateVariant = pb.RunStateVariant_RUN_STATE_VARIANT_DEDICATED_UNSTARTED
	f.run.ServerLane.State = pb.ServerLaneState_SERVER_LANE_STATE_ABSENT
	f.run.EffectivePolicy.Access = pb.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN
	f.run.EffectivePolicy.Verification = pb.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED
	got := evaluateWire(t, f, b)
	if !got.Flags.CanSubmitWrite || got.Flags.CanAcquireWriter || got.Mode == action.WriterWrite {
		t.Fatal(got)
	}
	f.run.Thread = &pb.ThreadProjection{ThreadId: "thread", ThreadGeneration: 1}
	f.run.StateVariant = pb.RunStateVariant_RUN_STATE_VARIANT_DEDICATED_READER
	if got := evaluateWire(t, f, b); got.Flags.CanSubmitWrite || !got.Flags.CanSubmitRead || got.Mode != action.WriterUnverified {
		t.Fatal("existing unknown policy admitted")
	}
}

func TestReleasedWriterResponsePreservesOwnerlessWorkspaceProjection(t *testing.T) {
	f, b := fixture()
	p, _ := New(f, f.caps)
	got, err := p.Release(t.Context(), b, 4)
	if err != nil || got.Owner != action.NoOwner || got.Access != action.UnknownAccess || got.Verification != action.Unverified || got.Lane != 0 || got.Requested != 0 || got.Generation != 7 || got.Stamp.Run != 0 || f.release != 1 {
		t.Fatal(got, err)
	}
	// Run-scoped facts in an ownerless Workspace projection are malformed.
	released, _ := f.ReleaseWriter(t.Context(), &pb.ReleaseWriterRequest{})
	released.ExecutionLane = pb.ExecutionLane_EXECUTION_LANE_DEDICATED
	f.writer = released
	if _, err := p.Read(t.Context(), b); err != action.ErrBlocked {
		t.Fatal("workspace-only read contract weakened", err)
	}
}

func TestPinnedUnknownActiveTurnKeepsExplicitReconciliation(t *testing.T) {
	f, b := fixture()
	f.run.Lifecycle = pb.RunLifecycle_RUN_LIFECYCLE_OUTCOME_UNKNOWN
	f.run.Recovery.State = pb.RecoveryState_RECOVERY_STATE_OUTCOME_UNKNOWN
	f.run.Recovery.RequiredAction = pb.RecoveryAction_RECOVERY_ACTION_RECONCILE_RUN
	f.run.ActiveTurn = &pb.TurnProjection{RunId: "run", ThreadId: "thread", TurnId: "turn", Status: pb.TurnStatus_TURN_STATUS_OUTCOME_UNKNOWN}
	p, _ := New(f, f.caps)
	in, err := p.Read(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	in.Local = action.LocalState{Ownership: action.OwnedSession, Credential: action.Healthy, Operation: action.NoOperation}
	in.Request = action.Request{Intent: action.IntentRead, CloseIntent: action.NoCloseIntent, InterruptConfirmed: true}
	got := action.Evaluate(in)
	if !got.Flags.CanReconcile || !got.Flags.BlockedByOutcomeUnknown || got.Flags.CanSubmitRead || got.Flags.CanAcquireWriter {
		t.Fatal(got)
	}
}

func TestPinnedDeclaredUnverifiedHeldWriterAndRelease(t *testing.T) {
	f, b := fixture()
	f.run.EffectivePolicy.Access = pb.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN
	f.run.EffectivePolicy.Verification = pb.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED
	f.run.WriterAuthority.State = pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE
	f.run.WriterAuthority.WriterGeneration = 7
	f.run.StateVariant = pb.RunStateVariant_RUN_STATE_VARIANT_DEDICATED_WRITER_ACTIVE
	f.profile.AccessPolicyTransition = pb.SupportState_SUPPORT_STATE_UNVERIFIED
	f.writer.OwnerRunId = &f.run.RunId
	f.writer.AuthorityState = pb.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE
	f.writer.ExecutionLane = f.run.ExecutionLane
	f.writer.RequestedAssurance, f.writer.AchievedAssurance = f.run.RequestedAssurance, f.run.AchievedAssurance
	f.writer.Stamp = proto.Clone(f.run.Stamp).(*pb.ProjectionStamp)
	got := evaluateWire(t, f, b)
	if !got.Flags.CanSubmitRead || !got.Flags.CanSubmitWrite || !got.Flags.CanReleaseWriter || got.Flags.CanAcquireWriter || got.Mode != action.WriterUnverified {
		t.Fatal(got)
	}
	p, _ := New(f, f.caps)
	released, err := p.Release(t.Context(), b, 4)
	if err != nil || released.Owner != action.NoOwner || released.Authority != action.Unowned || released.Generation != 7 || released.Access != action.UnknownAccess || released.Verification != action.Unverified || f.release != 1 || f.requestRevision != 4 {
		t.Fatal(released, err)
	}
}

func (f *wireActions) InterruptTurn(_ context.Context, q *pb.InterruptTurnRequest) (*pb.RunMutationResponse, error) {
	f.interrupts++
	if q.GetRun().GetRunId() != "run" || q.GetController().GetExpectedControllerId() != "controller" || q.GetController().GetAbsoluteFilePath() != "/private/carrier" || q.GetController().GetExpectedControllerGeneration() != 1 {
		return nil, errors.New("wrong Interrupt authority")
	}
	f.requestRevision = q.ExpectedStateRevision
	if f.mutationErr != nil {
		return nil, f.mutationErr
	}
	return &pb.RunMutationResponse{Run: proto.Clone(f.run).(*pb.RunProjection)}, nil
}

func TestInterruptUsesFreshRevisionAndRejectsUntrustedReceipt(t *testing.T) {
	f, b := fixture()
	p, err := New(f, f.caps)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Interrupt(t.Context(), b, 4); err != nil || f.interrupts != 1 || f.requestRevision != 4 {
		t.Fatalf("Interrupt delegation: %v calls%d revision%d", err, f.interrupts, f.requestRevision)
	}
	for name, change := range map[string]func(*pb.RunProjection){
		"Run":        func(r *pb.RunProjection) { r.RunId = "foreign" },
		"Controller": func(r *pb.RunProjection) { r.Controller.ControllerId = "foreign" },
		"generation": func(r *pb.RunProjection) { r.Controller.Generation = 2 },
		"revision":   func(r *pb.RunProjection) { r.StateRevision = 3 },
		"malformed":  func(r *pb.RunProjection) { r.Recovery = nil },
	} {
		t.Run(name, func(t *testing.T) {
			f, b := fixture()
			p, _ := New(f, f.caps)
			change(f.run)
			if err := p.Interrupt(t.Context(), b, 4); !errors.Is(err, action.ErrOutcomeUnknown) || f.interrupts != 1 {
				t.Fatalf("untrusted Interrupt receipt: %v calls%d", err, f.interrupts)
			}
		})
	}
}
