package contractprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/mutation"
	"google.golang.org/protobuf/proto"
)

type submitBindings struct{}

func (submitBindings) Resolve(_ context.Context, subject, run, binding, controller string) (string, string, string, uint64, error) {
	if subject != "owner" || run != "run" || binding != "binding" || controller != "controller" {
		return "", "", "", 0, mutation.ErrBlocked
	}
	return "/trusted/workspace", "provider-workspace", "/trusted/carrier", 1, nil
}

type submitWire struct {
	submission *publicv1.SubmitTurnRequest
	reads      int
	badKey     bool
}

func (p *submitWire) SubmitTurn(_ context.Context, request *publicv1.SubmitTurnRequest) (*publicv1.SubmitTurnAccepted, error) {
	p.submission = request
	key := request.GetIdempotencyKey()
	if p.badKey {
		key = "other"
	}
	run := validRun()
	run.RunId = "run"
	return &publicv1.SubmitTurnAccepted{IdempotencyKey: key, AcceptedTurn: &publicv1.TurnProjection{RunId: "run", TurnId: "turn"}, Run: run}, nil
}
func (p *submitWire) GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	p.reads++
	run := validRun()
	run.RunId = "run"
	return &publicv1.GetRunResponse{Run: run}, nil
}
func (p *submitWire) ListRunTimelineItems(context.Context, *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error) {
	p.reads++
	return &publicv1.ListRunTimelineItemsResponse{Items: []*publicv1.TimelineItem{{RunId: "run", TurnId: "unrelated-turn"}}}, nil
}

func TestSubmitAdapterUsesTrustedPathsAndDoesNotInferAcceptanceFromHistory(t *testing.T) {
	message := &publicv1.SubmitTurnRequest{Run: &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: "/browser/untrusted",
		ExpectedWorkspaceId: "provider-workspace"}, RunId: "run"},
		Controller:     &publicv1.ControllerCarrierRef{AbsoluteFilePath: "/browser/untrusted-carrier", ExpectedControllerId: "controller", ExpectedControllerGeneration: 1},
		IdempotencyKey: "same-key", Message: "prompt-canary", ExpectedStateRevision: 4}
	b, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	port := &submitWire{}
	provider := SubmitProvider{Port: port, Bindings: submitBindings{}}
	request := mutation.SubmitRequest{OperationID: "attempt", SubjectID: "owner", RunID: "run", BindingID: "binding", ControllerID: "controller",
		IdempotencyKey: "same-key", Canonical: b, CreatedAt: time.Now()}
	turn, err := provider.SubmitTurn(t.Context(), request)
	if err != nil || turn != "turn" || port.submission.GetRun().GetWorkspace().GetAbsolutePath() != "/trusted/workspace" ||
		port.submission.GetController().GetAbsoluteFilePath() != "/trusted/carrier" {
		t.Fatalf("submit path reconstruction = %q, %+v, %v", turn, port.submission, err)
	}
	proof, err := provider.ReconcileTurn(t.Context(), mutation.SubmitReference{OperationID: "attempt", SubjectID: "owner", RunID: "run", BindingID: "binding", ControllerID: "controller"})
	if err != nil || proof.Status != "unknown" || port.reads != 2 {
		t.Fatalf("history inferred acceptance = %+v, %v, reads=%d", proof, err, port.reads)
	}
	port.badKey = true
	if _, err := provider.SubmitTurn(t.Context(), request); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("mismatched accepted key = %v", err)
	}
}
