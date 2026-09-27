package contractprovider

import (
	"connectrpc.com/connect"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/observation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func binding() observation.Binding {
	return observation.Binding{SubjectID: "owner", SessionID: "session", ProviderID: "dolgorae", RunID: "run", WorkspaceID: "workspace", AbsoluteRoot: "/workspace"}
}
func base() *publicv1.DurableRunEvent {
	return &publicv1.DurableRunEvent{Cursor: "4", EventId: "event", OccurredAt: timestamppb.Now(), WorkspaceId: "workspace", RunId: "run", ServerKey: "server", ServerEpoch: 1, Projection: publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL, ProjectionVersion: 1, Stamp: &publicv1.ProjectionStamp{CapturedHeadCursor: "4", RunStateRevision: 4}}
}
func TestEveryDescriptorVariantHasMandatoryInvalidation(t *testing.T) {
	masks := map[string]observation.Refresh{
		"RunStateChanged":        observation.Run | observation.Writer,
		"TurnStateChanged":       observation.Run | observation.Timeline,
		"FinalResponseAvailable": observation.Run | observation.Timeline,
		"InteractionOpened":      observation.Run | observation.Interaction,
		"InteractionResolved":    observation.Run | observation.Interaction | observation.Timeline,
		"WriterStateChanged":     observation.Run | observation.Writer,
		"RecoveryRequired":       observation.Run | observation.Writer,
		"RuntimeErrorOccurred":   observation.Run,
		"GenerationChanged":      observation.Run | observation.Writer | observation.Interaction,
		"WorkspaceChanges":       observation.Timeline, "CommandStarted": observation.Timeline,
		"CommandCompleted": observation.Timeline, "UsageReported": observation.Timeline,
		"DiagnosticReported": observation.Timeline, "ReasoningSuppressed": observation.Timeline,
	}
	data, err := os.ReadFile("../../../contract/generated/event-invalidation-map.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	// A changed policy invalidates this behavioral matrix even when its variant
	// inventory stays the same. Reconcile expected masks before updating the pin.
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "5349862f48c254bedeb457cfee9144116cec8f310dba8def4b697a1a9330b669" {
		t.Fatal("event policy changed; reconcile the behavioral matrix")
	}
	var policy struct {
		Events []struct {
			Variant string `json:"variant"`
		}
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	for _, row := range policy.Events[:15] {
		if _, ok := masks[row.Variant]; !ok {
			t.Fatalf("unclassified policy variant %s", row.Variant)
		}
	}
	if len(policy.Events) != len(masks)+5 {
		t.Fatal("policy added an unclassified envelope or transport rule")
	}
	fields := base().ProtoReflect().Descriptor().Oneofs().ByName("event").Fields()
	if fields.Len() != len(masks) {
		t.Fatal("descriptor added an unclassified event")
	}
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		e := base()
		v := e.ProtoReflect().Mutable(field).Message().Interface()
		switch v := v.(type) {
		case *publicv1.RunStateChanged:
			v.Current = publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE
		case *publicv1.TurnStateChanged:
			v.Current = publicv1.TurnStatus_TURN_STATUS_COMPLETED
			e.TurnId = proto.String("turn")
		case *publicv1.FinalResponseAvailable:
			v.Response = &publicv1.FinalResponse{Value: &publicv1.FinalResponse_InlineUtf8{InlineUtf8: "CANARY-PRIVATE"}}
			e.TurnId = proto.String("turn")
		case *publicv1.InteractionOpenedEvent:
			v.InteractionId = "interaction"
			v.Kind = publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT
		case *publicv1.InteractionResolvedEvent:
			v.InteractionId = "interaction"
			v.Outcome = publicv1.InteractionOutcome_INTERACTION_OUTCOME_ANSWERED
		case *publicv1.WriterStateChangedEvent:
			v.Current = publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE
		case *publicv1.RuntimeErrorOccurred:
			v.ErrorCode = "ERROR"
			v.SafeMessage = "CANARY-PRIVATE"
		case *publicv1.GenerationChanged:
			v.RunGeneration = 1
			v.ServerEpoch = 1
		case *publicv1.DiagnosticReported:
			v.SafeMessage = "CANARY-PRIVATE"
		case *publicv1.ReasoningSuppressed:
			v.SafeReason = "CANARY-PRIVATE"
		}
		// Exercise the generated wire decoder, not a hand-built JSON projection.
		wire, err := proto.Marshal(&publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_DurableEvent{DurableEvent: e}})
		if err != nil {
			t.Fatal(err)
		}
		decoded := &publicv1.RunEventEnvelope{}
		if err := proto.Unmarshal(wire, decoded); err != nil {
			t.Fatal(err)
		}
		got, err := Decode(decoded, binding())
		if err != nil {
			t.Fatalf("%s: %v", field.Name(), err)
		}
		mask, ok := masks[got.Event.Kind]
		if !ok || got.Event.Refresh&mask != mask {
			t.Fatalf("weakened %s: %+v", field.Name(), got)
		}
		encoded, _ := json.Marshal(got)
		if strings.Contains(string(encoded), "CANARY-PRIVATE") {
			t.Fatal("raw content crossed domain boundary")
		}
	}
}
func TestRejectMalformedIdentityStampAndUnknownValues(t *testing.T) {
	changes := map[string]func(*publicv1.DurableRunEvent){
		"foreign run":       func(e *publicv1.DurableRunEvent) { e.RunId = "foreign" },
		"foreign workspace": func(e *publicv1.DurableRunEvent) { e.WorkspaceId = "foreign" },
		"noncanonical":      func(e *publicv1.DurableRunEvent) { e.Cursor = "04" },
		"stamp":             func(e *publicv1.DurableRunEvent) { e.Stamp.RunStateRevision = 3 },
		"unknown enum":      func(e *publicv1.DurableRunEvent) { e.GetRunStateChanged().Current = 999 },
		"unknown field":     func(e *publicv1.DurableRunEvent) { e.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01}) },
		"absent variant":    func(e *publicv1.DurableRunEvent) { e.Event = nil },
		"size":              func(e *publicv1.DurableRunEvent) { e.ServerKey = strings.Repeat("x", 8*1024*1024) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			e := base()
			e.Event = &publicv1.DurableRunEvent_RunStateChanged{RunStateChanged: &publicv1.RunStateChanged{Current: publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE}}
			change(e)
			if _, err := Decode(&publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_DurableEvent{DurableEvent: e}}, binding()); err == nil {
				t.Fatal("accepted invalid event")
			}
		})
	}
}
func TestScenarioMinimalStream(t *testing.T) {
	h := scenario.New(time.Now())
	ws, err := h.InspectWorkspace(t.Context(), &publicv1.InspectWorkspaceRequest{AbsolutePath: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	carrier := &publicv1.ControllerCarrierRef{AbsoluteFilePath: "/carrier", ExpectedControllerId: "controller", ExpectedControllerGeneration: 1}
	if err := h.RegisterController(scenario.ControllerSpec{ID: "controller", Generation: 1, CarrierPath: "/carrier", OrchestrationLaunch: true, PolicyName: "preprovisioned"}); err != nil {
		t.Fatal(err)
	}
	started, err := h.StartRun(t.Context(), &publicv1.StartRunRequest{Workspace: &publicv1.WorkspaceRef{AbsolutePath: "/workspace", ExpectedWorkspaceId: ws.WorkspaceId}, Controller: carrier, IdempotencyKey: "start", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	b := binding()
	b.RunID = started.Run.RunId
	b.WorkspaceID = ws.WorkspaceId
	stream, err := (Provider{Port: h}).Watch(t.Context(), b, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	ref := &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: "/workspace", ExpectedWorkspaceId: b.WorkspaceID}, RunId: b.RunID}
	if _, err := h.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{Run: ref, Controller: carrier, IdempotencyKey: "submit", Message: "CANARY-PROMPT", ExpectedStateRevision: started.Run.StateRevision}); err != nil {
		t.Fatal(err)
	}
	event, err := stream.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if event.Event.Kind != "TurnStateChanged" || event.Event.Refresh != (observation.Run|observation.Timeline) {
		t.Fatalf("%+v", event)
	}
}

func TestAdvisoryAndTransportMatrix(t *testing.T) {
	heartbeat := &publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_Heartbeat{Heartbeat: &publicv1.RunEventHeartbeat{RunId: "run", DurableHeadCursor: "4", Lifecycle: publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE, EmittedAt: timestamppb.Now()}}}
	got, err := Decode(heartbeat, binding())
	if err != nil || got.Event != nil || got.Heartbeat.IsZero() || got.Head != "4" {
		t.Fatal(got, err)
	}
	for _, reason := range []publicv1.StreamEndReason{publicv1.StreamEndReason_STREAM_END_REASON_RUN_TERMINAL, publicv1.StreamEndReason_STREAM_END_REASON_SERVER_SHUTDOWN, 99} {
		envelope := &publicv1.RunEventEnvelope{Item: &publicv1.RunEventEnvelope_StreamEnd{StreamEnd: &publicv1.RunEventStreamEnd{RunId: "run", DurableHeadCursor: "4", Reason: reason}}}
		got, err := Decode(envelope, binding())
		if reason == 99 {
			if err == nil {
				t.Fatal("unknown end accepted")
			}
			continue
		}
		want := "terminal"
		if reason == publicv1.StreamEndReason_STREAM_END_REASON_SERVER_SHUTDOWN {
			want = "shutdown"
		}
		if err != nil || got.End != want {
			t.Fatal(got, err)
		}
	}
	typed := connect.NewError(connect.CodeResourceExhausted, errors.New("untrusted diagnostic"))
	detail, err := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: "SLOW_CONSUMER", Action: publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONNECT_FROM_COMMITTED_CURSOR, RetryClassification: publicv1.RetryClassification_RETRY_CLASSIFICATION_TOKENLESS_AFTER_SNAPSHOT, RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED})
	if err != nil {
		t.Fatal(err)
	}
	typed.AddDetail(detail)
	if !errors.Is(mapError(typed), observation.ErrSlowConsumer) {
		t.Fatal("typed slow consumer lost")
	}
	if !errors.Is(mapError(connect.NewError(connect.CodeResourceExhausted, errors.New("SLOW_CONSUMER"))), observation.ErrRefresh) {
		t.Fatal("diagnostic text treated as typed status")
	}
}
