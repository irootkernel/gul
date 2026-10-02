package composition

import (
	"context"
	"time"

	"github.com/google/uuid"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
)

// AdoptController is a trusted host hook, with no browser route or absolute-path
// input. Verification performs no provider mutation and never repairs/reset a
// Controller; externally provisioned credentials must already authorize this Run.
func (r *Runtime) AdoptController(ctx context.Context, subject, sessionID, key string) error {
	if r == nil || r.Controllers == nil || subject == "" || sessionID == "" {
		return session.ErrCarrierUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	previous, err := r.store.Presentation().Binding(ctx, subject, sessionID)
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	attachment, err := r.Workspaces.Revalidate(ctx, subject, previous.WorkspaceID)
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	ref := &pb.RunRef{Workspace: &pb.WorkspaceRef{AbsolutePath: attachment.CanonicalRoot, ExpectedWorkspaceId: attachment.ProviderID}, RunId: previous.RunID}
	response, err := r.config.Port.GetRun(ctx, &pb.GetRunRequest{Run: ref})
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	run := response.GetRun()
	projection := run.GetController()
	if run.GetRunId() != previous.RunID || run.GetWorkspaceId() != attachment.ProviderID || run.GetConfiguration() == nil || run.GetConfiguration().GetParent() != nil || run.GetControlMode() != pb.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE || projection.GetKind() != pb.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT || projection.GetGeneration() == 0 {
		return session.ErrCarrierUnavailable
	}
	meta, carrier, err := r.Controllers.Validate(ctx, subject, key, projection.GetControllerId(), projection.GetGeneration())
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	if projection.GetInstanceId() != meta.InstanceID || projection.SubjectId == nil || projection.GetSubjectId() != subject {
		return session.ErrCarrierUnavailable
	}
	verified, err := r.config.Port.VerifyController(ctx, &pb.VerifyControllerRequest{Run: ref, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: carrier.Generation}})
	if err != nil || verified.GetRunId() != previous.RunID || verified.GetController().GetControllerId() != carrier.ControllerID || verified.GetController().GetGeneration() != carrier.Generation || verified.GetController().GetKind() != pb.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT || verified.GetController().GetInstanceId() != meta.InstanceID || verified.GetController().SubjectId == nil || verified.GetController().GetSubjectId() != subject || verified.GetVerifiedAt() == nil || !verified.GetVerifiedAt().IsValid() || verified.GetVerifiedAt().AsTime().After(time.Now().Add(time.Minute)) || time.Since(verified.GetVerifiedAt().AsTime()) > time.Minute {
		return session.ErrCarrierUnavailable
	}
	current, _, err := r.Controllers.Validate(ctx, subject, key, carrier.ControllerID, carrier.Generation)
	if err != nil || current.FileIdentity != meta.FileIdentity {
		return session.ErrCarrierUnavailable
	}
	oldCarrier, _ := r.config.Carriers.Resolve(ctx, subject, previous.ControllerBindingID)
	if err = r.reconnect.Disconnect(ctx, []action.Bound{{Binding: previous, Workspace: attachment, Carrier: oldCarrier}}); err != nil {
		return session.ErrCarrierUnavailable
	}
	id, err := uuid.NewV7()
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	meta.BindingID = id.String()
	meta.AllocationStarted = true
	binding, err := r.store.Presentation().AdoptCredential(ctx, previous, meta)
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	bound := action.Bound{Binding: binding, Workspace: attachment, Carrier: carrier}
	input, err := r.raw.Read(ctx, bound)
	if err != nil || !input.ControllerMatches {
		return session.ErrCarrierUnavailable
	}
	if err = r.refresh.Refresh(ctx, bound, input.Run); err != nil {
		return session.ErrCarrierUnavailable
	}
	// The normal reconnect loop restores readiness only after all dependent
	// projections, event subscription and public compatibility converge.
	return nil
}
