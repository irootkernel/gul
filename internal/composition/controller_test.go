package composition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/controller"
	"github.com/rootkernel/gul/internal/reconnect"
	"github.com/rootkernel/gul/internal/storage"
	"google.golang.org/protobuf/proto"
)

type adoptionProvider struct {
	*scenario.Harness
	instance, subject, originalPath string
	failCard                        bool
	cardReads                       int
}

func (p *adoptionProvider) GetRun(ctx context.Context, q *pb.GetRunRequest) (*pb.GetRunResponse, error) {
	r, err := p.Harness.GetRun(ctx, q)
	if err == nil {
		r.Run.Controller.InstanceId = p.instance
		r.Run.Controller.SubjectId = proto.String(p.subject)
	}
	return r, err
}
func (p *adoptionProvider) VerifyController(ctx context.Context, q *pb.VerifyControllerRequest) (*pb.VerifyControllerResponse, error) {
	q = proto.Clone(q).(*pb.VerifyControllerRequest)
	q.Controller.AbsoluteFilePath = p.originalPath // Scenario uses path equality instead of secret verification.
	r, err := p.Harness.VerifyController(ctx, q)
	if err == nil {
		r.Controller.InstanceId = p.instance
		r.Controller.SubjectId = proto.String(p.subject)
	}
	return r, err
}
func (p *adoptionProvider) GetControllerInteraction(ctx context.Context, q *pb.GetControllerInteractionRequest) (*pb.GetControllerInteractionResponse, error) {
	p.cardReads++
	if p.failCard {
		return nil, errors.New("injected Controller Interaction read failure")
	}
	q = proto.Clone(q).(*pb.GetControllerInteractionRequest)
	q.Controller.AbsoluteFilePath = p.originalPath
	return p.Harness.GetControllerInteraction(ctx, q)
}
func (p *adoptionProvider) GetOrchestratedSession(ctx context.Context, q *pb.GetOrchestratedSessionRequest) (*pb.GetOrchestratedSessionResponse, error) {
	q = proto.Clone(q).(*pb.GetOrchestratedSessionRequest)
	q.Controller.AbsoluteFilePath = p.originalPath
	return p.Harness.GetOrchestratedSession(ctx, q)
}
func (p *adoptionProvider) ListRunTimelineItems(ctx context.Context, q *pb.ListRunTimelineItemsRequest) (*pb.ListRunTimelineItemsResponse, error) {
	q = proto.Clone(q).(*pb.ListRunTimelineItemsRequest)
	q.Controller.AbsoluteFilePath = p.originalPath
	return p.Harness.ListRunTimelineItems(ctx, q)
}

// Observation acknowledgement is isolated here so this test measures the
// actual binding, checked aggregate providers and reconnect readiness gate.
type adoptionObserver struct{}

func (adoptionObserver) Resume(_ context.Context, runs []reconnect.ReadyRun) (map[string]string, error) {
	modes := map[string]string{}
	for _, run := range runs {
		modes[run.Bound.Binding.RunID] = "connected"
	}
	return modes, nil
}

func TestAdoptionCardFailureKeepsReconnectBlockedUntilRefreshSucceeds(t *testing.T) {
	base, err := os.MkdirTemp("/private/tmp", "gul-adoption-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	home, work := filepath.Join(base, "home"), filepath.Join(base, "workspace")
	for _, dir := range []string{home, work} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := storage.Open(t.Context(), filepath.Join(base, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	const subject = "adoption-subject"
	if err = db.Auth().CreateAccount(t.Context(), subject, time.Now()); err != nil {
		t.Fatal(err)
	}
	p := &adoptionProvider{Harness: scenario.NewRealtime(), subject: subject}
	caps, err := p.GetCapabilities(t.Context(), &pb.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	carriers, err := controller.New(t.Context(), db, home, []string{work}, caps)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := carriers.Create(t.Context(), subject, "preprovisioned")
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := carriers.Resolve(t.Context(), subject, meta.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	p.instance, p.originalPath = meta.InstanceID, carrier.AbsolutePath
	if err = p.RegisterController(scenario.ControllerSpec{ID: carrier.ControllerID, Generation: 1, CarrierPath: carrier.AbsolutePath, OrchestrationLaunch: true, PolicyName: "preprovisioned"}); err != nil {
		t.Fatal(err)
	}
	r, err := NewQualified(db, Config{Port: p, Carriers: carriers, Roots: []string{work}}, caps)
	if err != nil {
		t.Fatal(err)
	}
	r.reconnect.Observer = adoptionObserver{}
	w, err := r.Workspaces.RegisterFromAllowlistPath(t.Context(), subject, "root-1", ".")
	if err != nil {
		t.Fatal(err)
	}
	started, err := p.StartRun(t.Context(), &pb.StartRunRequest{Workspace: &pb.WorkspaceRef{AbsolutePath: work, ExpectedWorkspaceId: w.ProviderID}, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: 1}, IdempotencyKey: "adoption-start", ProfileName: "default", ControlMode: pb.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: pb.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: pb.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Sessions.BindPrimary(t.Context(), subject, w.ID, started.Run.RunId, meta.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Synchronize(t.Context()); err != nil || !r.reconnect.Ready(subject, b.ID) {
		t.Fatal("initial reconnect failed", err, r.reconnect.State(subject, b.ID))
	}
	if err = p.OpenInteraction(b.RunID, &pb.ControllerInteraction{Summary: &pb.InteractionSummary{InteractionId: "approval", Kind: pb.InteractionKind_INTERACTION_KIND_COMMAND_EXECUTION_APPROVAL}, ResponseSchemaId: "dolgorae.interaction.command-approval/v1", Decisions: []pb.InteractionDecision{pb.InteractionDecision_INTERACTION_DECISION_ACCEPT_ONCE, pb.InteractionDecision_INTERACTION_DECISION_DECLINE, pb.InteractionDecision_INTERACTION_DECISION_CANCEL}, Payload: &pb.ControllerInteraction_CommandApproval{CommandApproval: &pb.CommandApprovalInteraction{Title: "Approval", Message: "Allow this command?", Command: []string{"printf", "fixture"}, Cwd: &pb.PathProjection{Value: &pb.PathProjection_Utf8Path{Utf8Path: work}}}}}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(carrier.AbsolutePath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(body)
	const key = "adopted.json"
	if err = os.WriteFile(filepath.Join(filepath.Dir(carrier.AbsolutePath), key), body, 0600); err != nil {
		t.Fatal(err)
	}
	p.failCard = true
	if err = r.AdoptController(t.Context(), subject, b.ID, key); err == nil || r.reconnect.Ready(subject, b.ID) {
		t.Fatal("failed adoption refresh opened readiness", err)
	}
	adopted, err := db.Presentation().Binding(t.Context(), subject, b.ID)
	if err != nil || adopted.ControllerBindingID == b.ControllerBindingID || p.cardReads != 1 {
		t.Fatal("failure did not occur after exact binding replacement and Card read", err, p.cardReads)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err = r.Synchronize(t.Context()); err == nil || r.reconnect.Ready(subject, b.ID) {
			t.Fatal("reconnect forgot failed Controller Interaction refresh", err)
		}
	}
	// Reassembly models a host restart: the obligation comes from the common
	// refresh path, rather than an ephemeral adoption-only flag.
	r, err = NewQualified(db, Config{Port: p, Carriers: carriers, Roots: []string{work}}, caps)
	if err != nil {
		t.Fatal(err)
	}
	r.reconnect.Observer = adoptionObserver{}
	if err = r.Synchronize(t.Context()); err == nil || r.reconnect.Ready(subject, b.ID) {
		t.Fatal("reassembly forgot Card failure", err)
	}
	p.failCard = false
	if err = r.Synchronize(t.Context()); err != nil || !r.reconnect.Ready(subject, b.ID) || p.cardReads != 6 {
		t.Fatal("successful Card read did not restore readiness", err, p.cardReads)
	}
}
