package contractprovider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/sessionclose"
	"google.golang.org/protobuf/proto"
)

func TestSessionControlErrorsMatchPinnedProviderPolicy(t *testing.T) {
	body, err := os.ReadFile("../../../contract/upstream/dolgorae-grpc-error-mapping-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var pinned struct {
		DefaultStatus   string              `json:"default_status"`
		DefaultAction   string              `json:"default_required_action"`
		DefaultRetry    string              `json:"default_retry_classification"`
		DefaultRecovery string              `json:"default_recovery_classification"`
		Statuses        map[string][]string `json:"status_overrides"`
		Actions         map[string][]string `json:"required_action_overrides"`
		Retries         map[string][]string `json:"retry_classification_overrides"`
		Recoveries      map[string][]string `json:"recovery_classification_overrides"`
		Methods         map[string]map[string]struct {
			Status   string   `json:"status"`
			Action   string   `json:"required_action"`
			Retry    string   `json:"retry_classification"`
			Recovery string   `json:"recovery_classification"`
			Fields   []string `json:"required_fields"`
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
	statusValue := func(name string) connect.Code {
		t.Helper()
		for value := connect.CodeCanceled; value <= connect.CodeUnauthenticated; value++ {
			if strings.ToUpper(value.String()) == name {
				return value
			}
		}
		t.Fatalf("unknown pinned status %q", name)
		return 0
	}
	for _, code := range []string{"INVALID_ARGUMENT", "CONTROLLER_MISMATCH", "OBSERVER_MUTATION_FORBIDDEN", "CONTROL_MODE_CONTROLLER_MISMATCH", "RUN_STATE_CONFLICT", "RUN_BUSY", "WRITER_BUSY", "RUN_NOT_FOUND", "WORKSPACE_NOT_INITIALIZED"} {
		t.Run(code, func(t *testing.T) {
			status := lookup(pinned.DefaultStatus, pinned.Statuses, code)
			action := lookup(pinned.DefaultAction, pinned.Actions, code)
			retry := lookup(pinned.DefaultRetry, pinned.Retries, code)
			recovery := lookup(pinned.DefaultRecovery, pinned.Recoveries, code)
			actualStatus, actualAction, actualRecovery := rejectionPolicy(code)
			if strings.ToUpper(actualStatus.String()) != status || actualAction.String() != action || actualRecovery.String() != recovery {
				t.Fatalf("rejection differs from pinned policy: %v, %v, %v", actualStatus, actualAction, actualRecovery)
			}
			d := &pb.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: code, Action: pb.RequiredClientAction(pb.RequiredClientAction_value[action]), RetryClassification: pb.RetryClassification(pb.RetryClassification_value[retry]), RecoveryClassification: pb.RecoveryClassification(pb.RecoveryClassification_value[recovery])}
			for _, kind := range []sessionclose.Kind{sessionclose.Close, sessionclose.Recover, sessionclose.Reconcile} {
				out, err := sessionControlError(controlError(t, statusValue(status), d), "run", kind)
				if err != nil || out.RejectionCode == "" || out.NextAction == "" || out.Pending {
					t.Fatalf("pinned %s rejection not recognized: %+v, %v", kind, out, err)
				}
			}
		})
	}
	policy, ok := pinned.Methods["RunService.CloseRun"]["SESSION_CLOSE_IN_PROGRESS"]
	if !ok || !slices.Contains(policy.Fields, "run_id") || !slices.Contains(policy.Fields, "operation_id") {
		t.Fatal("pinned close pending correlation contract missing")
	}
	detail := &pb.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: "SESSION_CLOSE_IN_PROGRESS", RunId: proto.String("run"), OperationId: proto.String("operation"), Action: pb.RequiredClientAction(pb.RequiredClientAction_value[policy.Action]), RetryClassification: pb.RetryClassification(pb.RetryClassification_value[policy.Retry]), RecoveryClassification: pb.RecoveryClassification(pb.RecoveryClassification_value[policy.Recovery])}
	out, err := sessionControlError(controlError(t, statusValue(policy.Status), detail), "run", sessionclose.Close)
	if err != nil || !out.Pending || out.OperationID != "operation" || out.RejectionCode != "" {
		t.Fatalf("pinned close pending policy not recognized: %+v, %v", out, err)
	}
}

type wireSessionControl struct {
	response   *pb.RunMutationResponse
	err        error
	calls      int
	kind       sessionclose.Kind
	run        *pb.RunRef
	controller *pb.ControllerCarrierRef
	revision   uint64
	interrupt  bool
	deadline   time.Time
}

func (f *wireSessionControl) result(ctx context.Context, kind sessionclose.Kind, run *pb.RunRef, controller *pb.ControllerCarrierRef, revision uint64) (*pb.RunMutationResponse, error) {
	f.calls++
	f.kind, f.run, f.controller, f.revision = kind, run, controller, revision
	f.deadline, _ = ctx.Deadline()
	return f.response, f.err
}
func (f *wireSessionControl) CloseRun(ctx context.Context, q *pb.CloseRunRequest) (*pb.RunMutationResponse, error) {
	f.interrupt = q.Interrupt
	return f.result(ctx, sessionclose.Close, q.Run, q.Controller, q.ExpectedStateRevision)
}
func (f *wireSessionControl) RecoverRun(ctx context.Context, q *pb.RecoverRunRequest) (*pb.RunMutationResponse, error) {
	return f.result(ctx, sessionclose.Recover, q.Run, q.Controller, q.ExpectedStateRevision)
}
func (f *wireSessionControl) ReconcileRun(ctx context.Context, q *pb.ReconcileRunRequest) (*pb.RunMutationResponse, error) {
	return f.result(ctx, sessionclose.Reconcile, q.Run, q.Controller, q.ExpectedStateRevision)
}
func sessionControlFixture(t *testing.T) (*SessionControlProvider, *wireSessionControl, action.Bound) {
	t.Helper()
	wire, b := fixture()
	f := &wireSessionControl{response: &pb.RunMutationResponse{Context: &pb.ResponseContext{ProtocolVersion: 1, ServerInstanceId: "server", OperationId: proto.String("operation")}, Run: wire.run}}
	p, err := NewSessionControl(f)
	if err != nil {
		t.Fatal(err)
	}
	return p, f, b
}

func TestSessionControlDispatchUsesRootCarrierRevisionAndBoundedDeadline(t *testing.T) {
	for _, kind := range []sessionclose.Kind{sessionclose.Close, sessionclose.Recover, sessionclose.Reconcile} {
		t.Run(string(kind), func(t *testing.T) {
			p, f, b := sessionControlFixture(t)
			before := time.Now()
			got, err := p.Mutate(context.Background(), b, kind, 4, true)
			if err != nil || got.OperationID != "operation" || got.Run.Stamp.Run != 4 || got.Pending || got.RejectionCode != "" {
				t.Fatalf("mutation = %+v, %v", got, err)
			}
			if f.calls != 1 || f.kind != kind || !proto.Equal(f.run, ref(b)) || !proto.Equal(f.controller, carrier(b)) || f.revision != 4 || (kind == sessionclose.Close && !f.interrupt) {
				t.Fatalf("dispatch = %+v", f)
			}
			maximum := 60 * time.Second
			if kind == sessionclose.Close {
				maximum = 10 * time.Second
			}
			if f.deadline.Before(before.Add(maximum-time.Second)) || f.deadline.After(time.Now().Add(maximum)) {
				t.Fatalf("deadline = %v", f.deadline)
			}
		})
	}
	p, f, b := sessionControlFixture(t)
	deadline := time.Now().Add(time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	_, _ = p.Mutate(ctx, b, sessionclose.Close, 4, false)
	if !f.deadline.Equal(deadline) {
		t.Fatal("adapter extended caller deadline")
	}
}

func TestSessionControlRejectsIncompleteOrForeignSuccess(t *testing.T) {
	for name, change := range map[string]func(*pb.RunMutationResponse){
		"missing run":           func(r *pb.RunMutationResponse) { r.Run = nil },
		"missing context":       func(r *pb.RunMutationResponse) { r.Context = nil },
		"protocol":              func(r *pb.RunMutationResponse) { r.Context.ProtocolVersion = 2 },
		"empty operation":       func(r *pb.RunMutationResponse) { r.Context.OperationId = proto.String("") },
		"foreign run":           func(r *pb.RunMutationResponse) { r.Run.RunId = "other" },
		"foreign workspace":     func(r *pb.RunMutationResponse) { r.Run.WorkspaceId = "other" },
		"foreign controller":    func(r *pb.RunMutationResponse) { r.Run.Controller.ControllerId = "other" },
		"controller generation": func(r *pb.RunMutationResponse) { r.Run.Controller.Generation++ },
		"missing controller":    func(r *pb.RunMutationResponse) { r.Run.Controller = nil },
		"missing configuration": func(r *pb.RunMutationResponse) { r.Run.Configuration = nil },
		"missing profile":       func(r *pb.RunMutationResponse) { r.Run.Configuration.ProfileName = "" },
		"child run": func(r *pb.RunMutationResponse) {
			r.Run.Configuration.Parent = &pb.ParentRefProjection{Namespace: "provider", Kind: "run", Id: "root"}
		},
		"missing stamp":         func(r *pb.RunMutationResponse) { r.Run.Stamp = nil },
		"incomplete cursor":     func(r *pb.RunMutationResponse) { r.Run.EventCursor = "" },
		"old revision":          func(r *pb.RunMutationResponse) { r.Run.StateRevision = 3 },
		"missing policy":        func(r *pb.RunMutationResponse) { r.Run.EffectivePolicy = nil },
		"unspecified lifecycle": func(r *pb.RunMutationResponse) { r.Run.Lifecycle = 0 },
		"unknown decisive enum": func(r *pb.RunMutationResponse) { r.Run.Recovery.State = 999 },
		"unknown field":         func(r *pb.RunMutationResponse) { r.Run.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01}) },
	} {
		t.Run(name, func(t *testing.T) {
			p, f, b := sessionControlFixture(t)
			change(f.response)
			got, err := p.Mutate(context.Background(), b, sessionclose.Close, 4, false)
			if !errors.Is(err, sessionclose.ErrUnavailable) || got != (sessionclose.Mutation{}) || f.calls != 1 {
				t.Fatalf("mutation = %+v, %v; calls %d", got, err, f.calls)
			}
		})
	}
}

func closePendingDetail() *pb.DolgoraeErrorDetail {
	return &pb.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: "SESSION_CLOSE_IN_PROGRESS", RunId: proto.String("run"), OperationId: proto.String("operation"), Action: pb.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT, RetryClassification: pb.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, RecoveryClassification: pb.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED}
}
func controlError(t *testing.T, code connect.Code, details ...*pb.DolgoraeErrorDetail) error {
	t.Helper()
	err := connect.NewError(code, errors.New("private provider text"))
	for _, d := range details {
		value, e := connect.NewErrorDetail(d)
		if e != nil {
			t.Fatal(e)
		}
		err.AddDetail(value)
	}
	return err
}

func TestSessionControlPendingRequiresExactCorrelatedPolicy(t *testing.T) {
	p, f, b := sessionControlFixture(t)
	f.response, f.err = nil, controlError(t, connect.CodeFailedPrecondition, closePendingDetail())
	got, err := p.Mutate(context.Background(), b, sessionclose.Close, 4, false)
	if err != nil || !got.Pending || got.OperationID != "operation" || got.Run != (action.RunFacts{}) || got.RejectionCode != "" || f.calls != 1 {
		t.Fatalf("pending = %+v, %v", got, err)
	}
	for name, change := range map[string]func(*pb.DolgoraeErrorDetail){
		"foreign run":          func(d *pb.DolgoraeErrorDetail) { d.RunId = proto.String("other") },
		"missing run":          func(d *pb.DolgoraeErrorDetail) { d.RunId = nil },
		"missing operation":    func(d *pb.DolgoraeErrorDetail) { d.OperationId = nil },
		"empty operation":      func(d *pb.DolgoraeErrorDetail) { d.OperationId = proto.String("") },
		"wrong action":         func(d *pb.DolgoraeErrorDetail) { d.Action = pb.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT },
		"unspecified recovery": func(d *pb.DolgoraeErrorDetail) { d.RecoveryClassification = 0 },
		"unknown retry":        func(d *pb.DolgoraeErrorDetail) { d.RetryClassification = 999 },
		"unknown version":      func(d *pb.DolgoraeErrorDetail) { d.DetailVersion++ },
		"foreign turn":         func(d *pb.DolgoraeErrorDetail) { d.TurnId = proto.String("turn") },
	} {
		t.Run(name, func(t *testing.T) {
			d := closePendingDetail()
			change(d)
			got, err := sessionControlError(controlError(t, connect.CodeFailedPrecondition, d), "run", sessionclose.Close)
			if !errors.Is(err, sessionclose.ErrUnavailable) || got != (sessionclose.Mutation{}) {
				t.Fatalf("mutation = %+v, %v", got, err)
			}
		})
	}
	for _, kind := range []sessionclose.Kind{sessionclose.Recover, sessionclose.Reconcile} {
		if _, err := sessionControlError(f.err, "run", kind); !errors.Is(err, sessionclose.ErrUnavailable) {
			t.Fatal("close pending accepted for recovery")
		}
	}
}

func TestSessionControlPreacceptanceRejectionsAndAmbiguity(t *testing.T) {
	for _, code := range []string{"INVALID_ARGUMENT", "CONTROLLER_MISMATCH", "RUN_STATE_CONFLICT", "RUN_BUSY", "RUN_NOT_FOUND"} {
		t.Run(code, func(t *testing.T) {
			status, required, recovery := rejectionPolicy(code)
			d := &pb.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: code, Action: required, RetryClassification: pb.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, RecoveryClassification: recovery}
			got, err := sessionControlError(controlError(t, status, d), "run", sessionclose.Close)
			if err != nil || got.RejectionCode == "" || got.NextAction == "" || got.OperationID != "" || got.Pending {
				t.Fatalf("rejected = %+v, %v", got, err)
			}
			d.OperationId = proto.String("operation")
			if _, err = sessionControlError(controlError(t, status, d), "run", sessionclose.Close); !errors.Is(err, sessionclose.ErrUnavailable) {
				t.Fatal("claimed rejection despite operation evidence")
			}
		})
	}
	for _, err := range []error{
		context.Canceled, context.DeadlineExceeded, errors.New("lost reply"),
		connect.NewError(connect.CodeUnavailable, errors.New("transport")),
		controlError(t, connect.CodeAborted, closePendingDetail()),
		controlError(t, connect.CodeFailedPrecondition, closePendingDetail(), closePendingDetail()),
		controlError(t, connect.CodeFailedPrecondition, &pb.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: "OUTCOME_UNKNOWN", Action: pb.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONCILE_RUN, RetryClassification: pb.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, RecoveryClassification: pb.RecoveryClassification_RECOVERY_CLASSIFICATION_OUTCOME_UNKNOWN}),
	} {
		p, f, b := sessionControlFixture(t)
		f.response, f.err = nil, err
		got, actual := p.Mutate(context.Background(), b, sessionclose.Close, 4, false)
		if !errors.Is(actual, sessionclose.ErrUnavailable) || got != (sessionclose.Mutation{}) || f.calls != 1 {
			t.Fatalf("ambiguous = %+v, %v; calls %d", got, actual, f.calls)
		}
	}
	p, f, b := sessionControlFixture(t)
	f.err = controlError(t, connect.CodeFailedPrecondition, closePendingDetail())
	if _, err := p.Mutate(context.Background(), b, sessionclose.Close, 4, false); !errors.Is(err, sessionclose.ErrUnavailable) {
		t.Fatal("partial response accepted")
	}
}

func TestSessionControlRecoveryDoesNotInventOperationOrOtherProjections(t *testing.T) {
	p, f, b := sessionControlFixture(t)
	f.response.Context.OperationId = nil
	for _, kind := range []sessionclose.Kind{sessionclose.Recover, sessionclose.Reconcile} {
		got, err := p.Mutate(context.Background(), b, kind, 4, false)
		if err != nil || got.OperationID != "" || got.Pending || got.Run.Stamp.Run != 4 {
			t.Fatalf("recovery = %+v, %v", got, err)
		}
	}
}

func TestSessionControlInvalidDispatchDoesNotReachProvider(t *testing.T) {
	if _, err := NewSessionControl(nil); !errors.Is(err, sessionclose.ErrInvalid) {
		t.Fatal("nil port accepted")
	}
	p, f, b := sessionControlFixture(t)
	for _, kind := range []sessionclose.Kind{"", "DeleteRun"} {
		if _, err := p.Mutate(context.Background(), b, kind, 4, false); !errors.Is(err, sessionclose.ErrInvalid) {
			t.Fatalf("kind %q accepted", kind)
		}
	}
	if _, err := p.Mutate(context.Background(), b, sessionclose.Close, 0, false); !errors.Is(err, sessionclose.ErrInvalid) {
		t.Fatal("missing revision accepted")
	}
	b.Carrier.ControllerID = ""
	if _, err := p.Mutate(context.Background(), b, sessionclose.Close, 4, false); !errors.Is(err, sessionclose.ErrInvalid) {
		t.Fatal("missing Controller accepted")
	}
	if f.calls != 0 {
		t.Fatalf("invalid dispatch called provider %d times", f.calls)
	}
}

func TestSessionControlBoundsSuppliedOperationIdentity(t *testing.T) {
	for _, id := range []string{strings.Repeat("x", 1024), strings.Repeat("é", 512)} {
		p, f, b := sessionControlFixture(t)
		f.response.Context.OperationId = proto.String(id)
		out, err := p.Mutate(context.Background(), b, sessionclose.Close, 4, false)
		if err != nil || out.OperationID != id {
			t.Fatalf("valid maximum operation rejected: %v", err)
		}
		d := closePendingDetail()
		d.OperationId = proto.String(id)
		out, err = sessionControlError(controlError(t, connect.CodeFailedPrecondition, d), "run", sessionclose.Close)
		if err != nil || !out.Pending || out.OperationID != id {
			t.Fatalf("valid maximum pending operation rejected: %v", err)
		}
	}
	for _, id := range []string{strings.Repeat("x", 1025), strings.Repeat("é", 513), string([]byte{0xff})} {
		p, f, b := sessionControlFixture(t)
		f.response.Context.OperationId = proto.String(id)
		out, err := p.Mutate(context.Background(), b, sessionclose.Close, 4, false)
		if !errors.Is(err, sessionclose.ErrUnavailable) || out != (sessionclose.Mutation{}) || f.calls != 1 {
			t.Fatalf("invalid success identity accepted: %+v, %v", out, err)
		}
		d := closePendingDetail()
		rpc := controlError(t, connect.CodeFailedPrecondition, d)
		// An in-process checked port can return a typed message without passing
		// through protobuf's wire UTF-8 validation. Validate that boundary too.
		d.OperationId = proto.String(id)
		out, err = sessionControlError(rpc, "run", sessionclose.Close)
		if !errors.Is(err, sessionclose.ErrUnavailable) || out != (sessionclose.Mutation{}) {
			t.Fatalf("invalid pending identity accepted: %+v, %v", out, err)
		}
	}
}

func TestSessionControlCapabilitiesRequireEachAdvertisedMethod(t *testing.T) {
	for _, method := range []string{"RunService.CloseRun", "RunService.RecoverRun", "RunService.ReconcileRun"} {
		t.Run(method, func(t *testing.T) {
			for _, advertised := range []bool{true, false} {
				f, bound := fixture()
				if !advertised {
					f.caps.SupportedMethods = slices.DeleteFunc(f.caps.SupportedMethods, func(value string) bool { return value == method })
				}
				provider, err := New(f, f.caps)
				if err != nil {
					t.Fatal(err)
				}
				in, err := provider.Read(t.Context(), bound)
				if err != nil {
					t.Fatal(err)
				}
				in.Local = action.LocalState{Ownership: action.OwnedSession, Credential: action.Healthy, Operation: action.NoOperation}
				in.Request = action.Request{Intent: action.IntentRead, CloseIntent: action.CompleteSession}
				if in.Capabilities.Close != (advertised || method != "RunService.CloseRun") || in.Capabilities.Recover != (advertised || method != "RunService.RecoverRun") || in.Capabilities.Reconcile != (advertised || method != "RunService.ReconcileRun") {
					t.Fatalf("advertised=%v capabilities=%+v", advertised, in.Capabilities)
				}
				var enabled bool
				switch method {
				case "RunService.CloseRun":
					enabled = action.Evaluate(in).Flags.CanRequestSessionClose
				case "RunService.RecoverRun":
					in.Aggregate.Directive = action.RecoverAggregate
					enabled = action.Evaluate(in).Flags.CanRecover
				case "RunService.ReconcileRun":
					in.Aggregate.Directive = action.ReconcileAggregate
					enabled = action.Evaluate(in).Flags.CanReconcile
				}
				if enabled != advertised {
					t.Fatalf("advertised=%v enabled=%v", advertised, enabled)
				}
			}
		})
	}
}
