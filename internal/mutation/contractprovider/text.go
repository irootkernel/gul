package contractprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/mutation"
	"github.com/rootkernel/gul/internal/operation"
	"google.golang.org/protobuf/proto"
)

// TextDispatcher constructs normalized provider bytes only from backend authority.
// The carrier and workspace paths are reconstructed again at dispatch time.
type TextDispatcher struct{ Service *mutation.SubmitService }

func (d TextDispatcher) Submit(ctx context.Context, b action.Bound, in action.Input, attempt, text string, intent action.WriteIntent) error {
	digest := sha256.Sum256([]byte(b.Binding.SubjectID + "\x00" + attempt))
	key := hex.EncodeToString(digest[:])
	write := publicv1.WriteIntent_WRITE_INTENT_READ
	if intent == action.IntentWrite {
		write = publicv1.WriteIntent_WRITE_INTENT_WRITE
	}
	message := &publicv1.SubmitTurnRequest{Run: &publicv1.RunRef{RunId: b.Binding.RunID, Workspace: &publicv1.WorkspaceRef{ExpectedWorkspaceId: b.Workspace.ProviderID}}, Controller: &publicv1.ControllerCarrierRef{ExpectedControllerId: b.Carrier.ControllerID, ExpectedControllerGeneration: b.Carrier.Generation}, IdempotencyKey: key, Message: text, WriteIntent: write, ExpectedStateRevision: in.Run.Stamp.Run}
	canonical, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		return action.ErrInvalid
	}
	defer clear(canonical)
	result, err := d.Service.Submit(ctx, mutation.SubmitRequest{OperationID: key, SubjectID: b.Binding.SubjectID, RunID: b.Binding.RunID, BindingID: b.Binding.ControllerBindingID, ControllerID: b.Carrier.ControllerID, IdempotencyKey: key, Canonical: canonical, CreatedAt: time.Now()})
	if result.State == "resolved" && result.OutcomeRef == "rejected" {
		return action.ErrBlocked
	}
	if err != nil {
		if errors.Is(err, mutation.ErrSubmitPersistence) {
			return action.ErrPersistence
		}
		// An empty attempt means the gate/storage rejected before dispatch.
		if result.OperationID == "" && errors.Is(err, mutation.ErrBlocked) {
			return action.ErrBlocked
		}
		if result.OperationID == "" && errors.Is(err, operation.ErrConflict) {
			return action.ErrInvalid
		}
		if result.OperationID == "" {
			return action.ErrPersistence
		}
		return err
	}
	if result.State != "resolved" {
		return mutation.ErrUnknown
	}
	return nil
}
