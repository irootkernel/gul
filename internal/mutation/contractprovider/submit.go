package contractprovider

import (
	"context"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/mutation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type SubmitPort interface {
	SubmitTurn(context.Context, *publicv1.SubmitTurnRequest) (*publicv1.SubmitTurnAccepted, error)
	GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error)
	ListRunTimelineItems(context.Context, *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error)
}

// SubmitBindings revalidates logical IDs through trusted local repositories.
// No browser-selected path or carrier in the in-memory request is authoritative.
type SubmitBindings interface {
	Resolve(context.Context, string, string, string, string) (workspaceRoot, providerWorkspaceID, carrierPath string, generation uint64, err error)
}

type SubmitProvider struct {
	Port     SubmitPort
	Bindings SubmitBindings
}

func (p SubmitProvider) SubmitTurn(ctx context.Context, request mutation.SubmitRequest) (string, error) {
	if p.Port == nil || p.Bindings == nil || len(request.Canonical) == 0 || len(request.Canonical) > mutation.MaximumSubmitBytes {
		return "", mutation.ErrInvalid
	}
	root, workspaceID, carrier, generation, err := p.Bindings.Resolve(ctx, request.SubjectID, request.RunID, request.BindingID, request.ControllerID)
	if err != nil || root == "" || workspaceID == "" || carrier == "" || generation == 0 {
		return "", mutation.ErrBlocked
	}
	var message publicv1.SubmitTurnRequest
	if proto.Unmarshal(request.Canonical, &message) != nil || hasUnknown(message.ProtoReflect()) ||
		message.GetIdempotencyKey() != request.IdempotencyKey || message.GetRun().GetRunId() != request.RunID ||
		message.GetController().GetExpectedControllerId() != request.ControllerID ||
		message.GetController().GetExpectedControllerGeneration() != generation ||
		message.GetRun().GetWorkspace().GetExpectedWorkspaceId() != workspaceID || message.GetExpectedStateRevision() == 0 {
		return "", mutation.ErrInvalid
	}
	message.Run.Workspace.AbsolutePath = root
	message.Controller.AbsoluteFilePath = carrier
	response, err := p.Port.SubmitTurn(ctx, &message)
	if err != nil {
		return "", err
	}
	if response == nil || response.GetIdempotencyKey() != request.IdempotencyKey || response.GetAcceptedTurn().GetRunId() != request.RunID ||
		response.GetAcceptedTurn().GetTurnId() == "" || response.GetRun().GetRunId() != request.RunID ||
		response.GetRun().GetWorkspaceId() != workspaceID || response.GetRun().GetController().GetControllerId() != request.ControllerID {
		return "", ErrInvalidProjection
	}
	return response.GetAcceptedTurn().GetTurnId(), nil
}

func (p SubmitProvider) ReconcileTurn(ctx context.Context, ref mutation.SubmitReference) (mutation.SubmitEvidence, error) {
	if p.Port == nil || p.Bindings == nil || ref.OperationID == "" {
		return mutation.SubmitEvidence{}, mutation.ErrInvalid
	}
	root, workspaceID, carrier, generation, err := p.Bindings.Resolve(ctx, ref.SubjectID, ref.RunID, ref.BindingID, ref.ControllerID)
	if err != nil || root == "" || workspaceID == "" || carrier == "" || generation == 0 {
		return mutation.SubmitEvidence{}, mutation.ErrBlocked
	}
	run := &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: root, ExpectedWorkspaceId: workspaceID}, RunId: ref.RunID}
	snapshot, err := p.Port.GetRun(ctx, &publicv1.GetRunRequest{Run: run})
	if err != nil {
		return mutation.SubmitEvidence{}, err
	}
	if snapshot == nil || snapshot.GetRun().GetRunId() != ref.RunID || snapshot.GetRun().GetWorkspaceId() != workspaceID ||
		snapshot.GetRun().GetController().GetControllerId() != ref.ControllerID {
		return mutation.SubmitEvidence{}, ErrInvalidProjection
	}
	page, err := p.Port.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: run,
		Controller: &publicv1.ControllerCarrierRef{AbsoluteFilePath: carrier, ExpectedControllerId: ref.ControllerID,
			ExpectedControllerGeneration: generation}, Limit: 100, TimelineVersion: 1})
	if err != nil {
		return mutation.SubmitEvidence{}, err
	}
	if page == nil {
		return mutation.SubmitEvidence{}, ErrInvalidProjection
	}
	// Neither the Run nor timeline item carries the SubmitTurn idempotency key.
	// A missing page or matching text cannot identify this exact attempt.
	return mutation.SubmitEvidence{Status: "unknown"}, nil
}

var _ mutation.SubmitProvider = SubmitProvider{}

func hasUnknown(message protoreflect.Message) bool {
	if len(message.GetUnknown()) != 0 {
		return true
	}
	unknown := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				unknown = unknown || hasUnknown(list.Get(i).Message())
			}
		} else if field.IsMap() {
			value.Map().Range(func(_ protoreflect.MapKey, entry protoreflect.Value) bool {
				unknown = unknown || hasUnknown(entry.Message())
				return !unknown
			})
		} else {
			unknown = hasUnknown(value.Message())
		}
		return !unknown
	})
	return unknown
}
