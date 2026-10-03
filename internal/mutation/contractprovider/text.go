package contractprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/files"
	"github.com/rootkernel/gul/internal/launch"
	"github.com/rootkernel/gul/internal/mutation"
	"github.com/rootkernel/gul/internal/operation"
	"github.com/rootkernel/gul/internal/submit"
	"google.golang.org/protobuf/proto"
)

// TextDispatcher constructs normalized provider bytes only from backend authority.
// The carrier and workspace paths are reconstructed again at dispatch time.
type TextDispatcher struct {
	Service  *mutation.SubmitService
	Images   *files.SubmitImages
	Profiles launch.Provider
	Port     SubmitPort
}

func (d TextDispatcher) Submit(ctx context.Context, b action.Bound, in action.Input, attempt, text string, intent action.WriteIntent) error {
	return d.SubmitInput(ctx, b, in, attempt, text, intent, submit.Options{})
}
func (d TextDispatcher) SubmitInput(ctx context.Context, b action.Bound, in action.Input, attempt, text string, intent action.WriteIntent, options submit.Options) error {
	digest := sha256.Sum256([]byte(b.Binding.SubjectID + "\x00" + attempt))
	key := hex.EncodeToString(digest[:])
	write := publicv1.WriteIntent_WRITE_INTENT_READ
	if intent == action.IntentWrite {
		write = publicv1.WriteIntent_WRITE_INTENT_WRITE
	}
	message := &publicv1.SubmitTurnRequest{Run: &publicv1.RunRef{RunId: b.Binding.RunID, Workspace: &publicv1.WorkspaceRef{ExpectedWorkspaceId: b.Workspace.ProviderID}}, Controller: &publicv1.ControllerCarrierRef{ExpectedControllerId: b.Carrier.ControllerID, ExpectedControllerGeneration: b.Carrier.Generation}, IdempotencyKey: key, Message: text, WriteIntent: write, ExpectedStateRevision: in.Run.Stamp.Run}
	if options.Effort != nil {
		if d.Port == nil || d.Profiles == nil || *options.Effort == "" || len(*options.Effort) > 256 {
			return action.ErrInvalid
		}
		r, err := d.Port.GetRun(ctx, &publicv1.GetRunRequest{Run: &publicv1.RunRef{RunId: b.Binding.RunID, Workspace: &publicv1.WorkspaceRef{AbsolutePath: b.Workspace.CanonicalRoot, ExpectedWorkspaceId: b.Workspace.ProviderID}}})
		if err != nil {
			return action.ErrBlocked
		}
		if r.GetRun().GetRunId() != b.Binding.RunID || r.GetRun().GetController().GetControllerId() != b.Carrier.ControllerID || r.GetRun().GetWorkspaceId() != b.Workspace.ProviderID || r.GetRun().GetConfiguration() == nil {
			return action.ErrInvalid
		}
		profile, err := d.Profiles.GetProfile(ctx, r.GetRun().GetConfiguration().GetProfileName())
		if err != nil {
			return action.ErrBlocked
		}
		valid := false
		for _, m := range profile.Models {
			if m.ID == r.GetRun().GetConfiguration().GetModelId() {
				for _, e := range m.SupportedEfforts {
					valid = valid || e == *options.Effort
				}
			}
		}
		if !valid {
			return action.ErrInvalid
		}
		message.Effort = options.Effort
	}
	if len(options.Images) > 0 {
		images, err := d.Images.Stage(ctx, b.Binding.SubjectID, b.Binding.WorkspaceID, b.Binding.RunID, key, options.Images)
		if err != nil {
			return action.ErrInvalid
		}
		for _, image := range images {
			detail := publicv1.ImageDetail_IMAGE_DETAIL_AUTO
			if image.Detail == "low" {
				detail = publicv1.ImageDetail_IMAGE_DETAIL_LOW
			}
			if image.Detail == "high" {
				detail = publicv1.ImageDetail_IMAGE_DETAIL_HIGH
			}
			message.Images = append(message.Images, &publicv1.ImageInput{AbsoluteFilePath: image.Path, Detail: detail})
		}
	}
	canonical, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		return action.ErrInvalid
	}
	defer clear(canonical)
	result, err := d.Service.Submit(ctx, mutation.SubmitRequest{OperationID: key, SubjectID: b.Binding.SubjectID, RunID: b.Binding.RunID, BindingID: b.Binding.ControllerBindingID, ControllerID: b.Carrier.ControllerID, IdempotencyKey: key, Canonical: canonical, CreatedAt: time.Now()})
	if result.State == "resolved" && result.OutcomeRef == "rejected" {
		d.Images.Rejected(key)
		return action.ErrBlocked
	}
	if err != nil {
		if result.OperationID == "" {
			d.Images.Rejected(key)
		}
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
	d.Images.Accepted(key)
	return nil
}
