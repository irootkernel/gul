package contractprovider

import (
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
)

func runFacts(r *publicv1.RunProjection) (action.RunFacts, error) {
	if r.EffectivePolicy == nil || r.WriterAuthority == nil || r.Recovery == nil || r.BackgroundExecution == nil || r.ServerLane == nil || r.ServerLane.Kind != r.ExecutionLane || r.ServerLane.State == 0 {
		return action.RunFacts{}, action.ErrBlocked
	}
	thread, active, lineage := action.Missing, action.Missing, action.NoLineage
	if r.Thread != nil {
		if r.Thread.ThreadId == "" || r.Thread.ThreadGeneration == 0 {
			return action.RunFacts{}, action.ErrBlocked
		}
		thread = action.Present
	}
	if r.ActiveTurn != nil {
		if r.ActiveTurn.RunId != r.RunId || r.ActiveTurn.TurnId == "" || r.ActiveTurn.ThreadId != r.GetThread().GetThreadId() || r.ActiveTurn.Status < publicv1.TurnStatus_TURN_STATUS_RESERVED || (r.ActiveTurn.Status > publicv1.TurnStatus_TURN_STATUS_INTERRUPTING && !(r.ActiveTurn.Status == publicv1.TurnStatus_TURN_STATUS_OUTCOME_UNKNOWN && r.Lifecycle == publicv1.RunLifecycle_RUN_LIFECYCLE_OUTCOME_UNKNOWN && r.Recovery.State == publicv1.RecoveryState_RECOVERY_STATE_OUTCOME_UNKNOWN)) {
			return action.RunFacts{}, action.ErrBlocked
		}
		active = action.Present
	}
	if r.Lineage != nil {
		if r.Lineage.SourceRunId == "" || r.Lineage.SourceThreadId == "" || r.Lineage.SourceTurnId == "" || !r.Lineage.Immutable || r.Lineage.CreationReason == 0 || r.Lineage.SourceControllerKind == 0 || r.Lineage.DestinationControllerKind == 0 {
			return action.RunFacts{}, action.ErrBlocked
		}
		lineage = action.ValidLineage
	}
	return action.RunFacts{Lifecycle: action.Lifecycle(r.Lifecycle), Variant: action.Variant(r.StateVariant), Control: action.Control(r.ControlMode), Lane: action.Lane(r.ExecutionLane), Thread: thread, ActiveTurn: active, Pending: r.PendingInteractionCount, Access: action.Access(r.EffectivePolicy.Access), Verification: action.Verification(r.EffectivePolicy.Verification), Authority: action.Authority(r.WriterAuthority.State), Generation: r.WriterAuthority.WriterGeneration, Reconciliation: action.Reconciliation(r.WriterAuthority.ReconciliationAction), Requested: action.Assurance(r.RequestedAssurance), Achieved: action.Assurance(r.AchievedAssurance), Background: action.Background(r.BackgroundExecution.State), Server: action.ServerState(r.ServerLane.State), Recovery: action.Recovery(r.Recovery.State), RecoveryAction: action.RecoveryAction(r.Recovery.RequiredAction), Lineage: lineage, Stamp: stamp(r.Stamp)}, nil
}
func writerProjection(w *publicv1.WriterState, b action.Bound, released bool) (action.WriterProjection, error) {
	if w == nil || !known(w.ProtoReflect()) || w.WorkspaceId != b.Workspace.ProviderID || w.Stamp == nil || w.StateRevision != w.Stamp.WriterStateRevision || w.AuthorityState == 0 || w.EffectiveAccess == 0 || w.PolicyVerification == 0 || w.ReconciliationAction == 0 {
		return action.WriterProjection{}, action.ErrBlocked
	}
	owner := action.ExternalOwner
	if w.GetOwnerRunId() == "" {
		owner = action.NoOwner
	} else if w.GetOwnerRunId() == b.Binding.RunID {
		owner = action.ThisSession
	}
	if owner == action.NoOwner {
		if w.AuthorityState != publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE {
			return action.WriterProjection{}, action.ErrBlocked
		}
		if released {
			// ReleaseWriter returns the released Run's policy and complete stamp.
			if w.ExecutionLane == 0 || w.RequestedAssurance == 0 || w.AchievedAssurance == 0 || !stamp(w.Stamp).Valid() {
				return action.WriterProjection{}, action.ErrBlocked
			}
		} else if w.ExecutionLane != 0 || w.RequestedAssurance != 0 || w.AchievedAssurance != 0 || w.EffectiveAccess != publicv1.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN || w.PolicyVerification != publicv1.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED || w.Stamp.CapturedHeadCursor != "" || w.Stamp.RunStateRevision != 0 || w.Stamp.InteractionStateRevision != 0 {
			return action.WriterProjection{}, action.ErrBlocked
		}
	} else if w.AuthorityState == publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE || w.WriterGeneration == 0 || w.ExecutionLane == 0 || w.RequestedAssurance == 0 || w.AchievedAssurance == 0 || !stamp(w.Stamp).Valid() {
		return action.WriterProjection{}, action.ErrBlocked
	}
	return action.WriterProjection{Authority: action.Authority(w.AuthorityState), Generation: w.WriterGeneration, Access: action.Access(w.EffectiveAccess), Verification: action.Verification(w.PolicyVerification), Lane: action.Lane(w.ExecutionLane), Requested: action.Assurance(w.RequestedAssurance), Achieved: action.Assurance(w.AchievedAssurance), Owner: owner, Reconciliation: action.Reconciliation(w.ReconciliationAction), BackgroundBlocked: w.BackgroundExecutionBlocker != nil, RecoveryBlocked: w.RecoveryBlocker != nil, Revision: w.StateRevision, Stamp: stamp(w.Stamp)}, nil
}
