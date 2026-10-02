package action

import "github.com/rootkernel/gul/internal/observation"

// Evaluate is shared by browser presentation and backend mutation admission.
// Callers supply fresh facts; no browser-provided flag can authorize an RPC.
func Evaluate(in Input) Evaluation {
	out := Evaluation{Writer: in.Writer, Mode: WriterBlocked}
	f := &out.Flags
	r, w, c, p, a, l := in.Run, in.Writer, in.Capabilities, in.Profile, in.Aggregate, in.Local
	if !validInput(in) || p.Compatibility != Compatible || !p.SupportsLane || r.Achieved > p.MaximumAssurance || r.Achieved < r.Requested {
		f.BlockedByProviderCompatibility = true
		f.BlockedByCredentialState = l.Credential >= CredentialMissing && l.Credential <= VerifiedAdoption
		out.Blocker = ProviderIncompatible
		if f.BlockedByCredentialState {
			out.Blocker = CredentialBlocked
		}
		return out
	}
	if in.Freshness != Fresh || l.ProjectionsStale || !compatibleStamps(in) {
		f.RequiresFreshSnapshot = true
		out.Blocker = FreshSnapshotRequired
		return out
	}
	out.Mode = EvaluateWriter(WriterFacts{Fresh: true, ActiveAuthority: w.Owner == ThisSession && w.Authority == Active, VerifiedPolicy: r.Verification == Verified && (w.Owner != ThisSession || w.Verification == Verified), EffectiveRead: r.Access == Read, EffectiveWrite: r.Access == Write}, in.Request.Intent).Mode
	if a.Freshness != Fresh || a.Revision == 0 {
		f.RequiresFreshSnapshot = true
		out.Blocker = FreshSnapshotRequired
		return out
	}
	if l.Credential != Healthy || l.Ownership != OwnedSession || r.Control != Direct {
		f.BlockedByCredentialState = true
		f.CanAdoptController = l.Credential == VerifiedAdoption && l.Ownership == OwnedSession && c.VerifyController
		out.Blocker = CredentialBlocked
		return out
	}
	active := r.ActiveTurn == Present
	ownedWork := active || r.Background == BackgroundActive || a.NonretiredMembers > 1 || a.NonterminalSpawns > 0 || a.PendingApprovals > 0 || a.AcceptedUnfinishedTasks > 0
	recoveryConsent := !ownedWork || in.Request.InterruptConfirmed
	if r.RecoveryAction == Repair || r.Reconciliation == OperatorRepair || w.Reconciliation == OperatorRepair {
		f.RequiresOperatorAction = true
		out.Blocker = RecoveryBlocked
		return out
	}
	if l.Operation != NoOperation || r.Lifecycle == OutcomeUnknown || r.Recovery == RecoveryUnknown || r.Authority == BlockedUnknown || w.Authority == BlockedUnknown || a.CloseProgress == CloseUnknown || a.Recovery == AggregateUnknown || a.UnknownOutcomeTasks > 0 {
		f.BlockedByOutcomeUnknown = true
		out.Blocker = UnresolvedOutcome
		// Uncertainty blocks ordinary mutations, not the provider's explicit
		// resolution instruction. An in-flight local call still owns admission.
		if l.Operation != OperationPending && (a.Directive == NoAggregateAction || a.Directive == ReconcileAggregate || a.Directive == RecoverAggregate) && a.Recovery != AggregateSnapshotRequired {
			f.CanReconcile = c.Reconcile && recoveryConsent && (r.RecoveryAction == Reconcile || a.Directive == ReconcileAggregate)
			f.CanRecover = c.Recover && recoveryConsent && (r.RecoveryAction == Recover || a.Directive == RecoverAggregate)
		}
		return out
	}
	if a.Directive != NoAggregateAction {
		out.Blocker = RecoveryBlocked
		switch a.Directive {
		case RefreshAggregate:
			f.RequiresFreshSnapshot = true
		case ReconcileAggregate:
			f.CanReconcile = c.Reconcile && recoveryConsent && a.Freshness == Fresh && a.Revision > 0
		case RecoverAggregate:
			f.CanRecover = c.Recover && recoveryConsent && a.Freshness == Fresh && a.Revision > 0
		case RepairAggregate:
			f.RequiresOperatorAction = true
		case ContinueWrite:
			out.Blocker = UnsupportedTransition
		default:
			f.BlockedByProviderCompatibility = true
		}
		return out
	}
	if r.Reconciliation != NoReconciliation || w.Reconciliation != NoReconciliation {
		f.RequiresFreshSnapshot = true
		out.Blocker = RecoveryBlocked
		f.CanReconcile = c.Reconcile && recoveryConsent && a.Freshness == Fresh && a.Revision > 0 && r.RecoveryAction == Reconcile
		return out
	}
	if r.Recovery != NoRecovery || a.Recovery != AggregateReady || a.CloseProgress == CloseRecovery {
		out.Blocker = RecoveryBlocked
		if a.Freshness == Fresh && a.Revision > 0 {
			f.CanRecover = c.Recover && recoveryConsent && r.RecoveryAction == Recover
			f.CanReconcile = c.Reconcile && recoveryConsent && (r.RecoveryAction == Reconcile || a.Recovery == AggregateReconcileRequired)
		}
		if r.RecoveryAction == RefreshSnapshot || a.Recovery == AggregateSnapshotRequired {
			f.RequiresFreshSnapshot = true
		}
		return out
	}
	if r.Server == ServerFailed {
		f.RequiresOperatorAction = true
		out.Blocker = ProviderIncompatible
		return out
	}
	if r.Server == ServerUnverified {
		f.BlockedByProviderCompatibility = true
		out.Blocker = ProviderIncompatible
		return out
	}
	if r.Lifecycle == Closed || r.Lifecycle == StartFailed || a.Lifecycle == SessionCompleted || a.Lifecycle == SessionAborted || a.CloseProgress != NoClose {
		return out
	}
	backgroundSafe := r.Background == Absent || r.Background == NotApplicable
	if !backgroundSafe || w.BackgroundBlocked || w.RecoveryBlocked {
		f.BlockedByBackgroundExecution = true
		out.Blocker = BackgroundBlocked
	}
	// Current Interaction answers remain possible while ordinary prompts are drafts.
	f.CanResolveInteraction = c.Resolve && r.Pending > 0 && (r.Lifecycle == Running || r.Lifecycle == WaitingInteraction || r.Lifecycle == Idle)
	f.CanInterrupt = c.Interrupt && active && in.Request.InterruptConfirmed
	if active && !in.Request.InterruptConfirmed {
		out.Blocker = InterruptConfirmationRequired
	}
	transitionBusy := r.Authority == HandoffPrepared || r.Authority == Releasing || r.Authority == Reserved && r.Thread == Present
	if transitionBusy {
		f.RequiresFreshSnapshot = true
		out.Blocker = FreshSnapshotRequired
	}
	policyReady := r.Verification == Verified && (r.Access == Read || r.Access == Write)
	quiescent := !transitionBusy && (r.Server == ServerReady || r.Server == ServerAbsent) && !active && r.Pending == 0 && backgroundSafe && !w.BackgroundBlocked && !w.RecoveryBlocked
	f.CanPausePrimary = c.Pause && quiescent && r.Lifecycle == Idle
	f.CanResumePrimary = c.Resume && quiescent && r.Lifecycle == Paused
	transition := p.Transition == Supported
	fixed := r.Thread == Present && !transition
	writeFeatures := c.DedicatedWriter && c.DurableWriter && r.Lane == Dedicated
	writerAvailable := w.Owner == NoOwner || w.Owner == ThisSession
	initialPolicy := r.Thread == Missing && r.Access == UnknownAccess && r.Verification == Unverified && r.Authority == Unowned && w.Owner == NoOwner
	verifiedWriter := w.Owner == ThisSession && w.Authority == Active && r.Authority == Active && r.Access == Write && w.Access == Write && w.Verification == Verified
	if quiescent && r.Lifecycle == Idle {
		f.CanSubmitRead = c.Submit && (initialPolicy || policyReady && (r.Access == Read || verifiedWriter))
		firstWrite := initialPolicy && c.FirstWriteViaSubmit && r.Variant == DedicatedUnstarted
		f.CanSubmitWrite = c.Submit && writeFeatures && writerAvailable && (firstWrite || policyReady && verifiedWriter)
	}
	if quiescent && policyReady && (r.Lifecycle == Idle || r.Lifecycle == Paused) && writeFeatures {
		f.CanAcquireWriter = c.Acquire && w.Owner == NoOwner && (r.Thread == Present && transition)
		f.CanReleaseWriter = c.Release && w.Owner == ThisSession && w.Authority == Active && r.Authority == Active && r.Verification == Verified && w.Verification == Verified
	}
	if w.Owner == ExternalOwner && in.Request.Intent == IntentWrite {
		out.Blocker = WriterBusy
	}
	if r.Lineage == ContinuationRequired || fixed && in.Request.Intent == IntentWrite && r.Access != Write {
		out.Blocker = UnsupportedTransition
		f.CanSubmitRead = false
		f.CanSubmitWrite = false
		f.CanAcquireWriter = false
		f.CanReleaseWriter = false
	}
	if active {
		out.Blocker = ActiveTurnDraft
	}
	if a.Freshness == Fresh && a.Revision > 0 && (a.Lifecycle == SessionActive || a.Lifecycle == SessionDegraded) && c.Close && r.Background != BackgroundUnverified {
		f.RequiresCloseConfirmation = ownedWork && !in.Request.InterruptConfirmed
		f.CanRequestSessionClose = (in.Request.CloseIntent == CompleteSession || in.Request.CloseIntent == AbortSession) && (!ownedWork || in.Request.InterruptConfirmed)
	}
	return out
}

func validInput(in Input) bool {
	r, w, p, c, a, l := in.Run, in.Writer, in.Profile, in.Capabilities, in.Aggregate, in.Local
	if !in.Checked || !in.ControllerMatches || !c.Checked || in.Request.Intent > IntentWrite || in.Request.CloseIntent < NoCloseIntent || in.Request.CloseIntent > AbortSession || in.Freshness < Fresh || in.Freshness > UnavailableSnapshot {
		return false
	}
	if r.Server < ServerStarting || r.Server > ServerFailed ||
		r.Lifecycle < Starting || r.Lifecycle > OutcomeUnknown ||
		r.Variant < SharedReadonly || r.Variant > DedicatedFailed ||
		r.Control < Direct || r.Control > Managed ||
		r.Lane < Shared || r.Lane > Dedicated ||
		r.Thread < Missing || r.Thread > Present ||
		r.ActiveTurn < Missing || r.ActiveTurn > Present ||
		r.Access < Read || r.Access > UnknownAccess ||
		r.Verification < Verified || r.Verification > Failed ||
		r.Authority < Unowned || r.Authority > BlockedUnknown ||
		r.Reconciliation < NoReconciliation || r.Reconciliation > ReverifyPolicy ||
		r.Requested < BestEffort || r.Requested > ProcessContained ||
		r.Achieved < BestEffort || r.Achieved > ProcessContained ||
		r.Background < Absent || r.Background > NotApplicable ||
		r.Recovery < NoRecovery || r.Recovery > RecoveryUnknown ||
		r.RecoveryAction < NoRecoveryAction || r.RecoveryAction > Repair ||
		r.Lineage < NoLineage || r.Lineage > ContinuationRequired {
		return false
	}
	if w.Authority < Unowned || w.Authority > BlockedUnknown ||
		w.Owner < NoOwner || w.Owner > ExternalOwner ||
		w.Access < Read || w.Access > UnknownAccess ||
		w.Verification < Verified || w.Verification > Failed ||
		w.Reconciliation < NoReconciliation || w.Reconciliation > ReverifyPolicy {
		return false
	}
	if !variantMatches(r) || w.Owner != ThisSession && (r.Authority != Unowned || r.Generation != 0) {
		return false
	}
	if w.Owner == NoOwner {
		if w.Authority != Unowned || w.Lane != 0 || w.Requested != 0 || w.Achieved != 0 || w.Access != UnknownAccess || w.Verification != Unverified {
			return false
		}
	} else if w.Authority == Unowned || w.Generation == 0 ||
		w.Lane < Shared || w.Lane > Dedicated ||
		w.Requested < BestEffort || w.Requested > ProcessContained ||
		w.Achieved < BestEffort || w.Achieved > ProcessContained {
		return false
	}
	if p.Compatibility < Compatible || p.Compatibility > CompatibilityUnavailable ||
		p.Transition < Supported || p.Transition > Unsafe ||
		c.Transition < Supported || c.Transition > Unsafe ||
		p.BackgroundControl < Supported || p.BackgroundControl > Unsafe ||
		p.MaximumAssurance < BestEffort || p.MaximumAssurance > ProcessContained {
		return false
	}
	if l.Ownership < OwnedSession || l.Ownership > ObserverSession ||
		l.Credential < Healthy || l.Credential > VerifiedAdoption ||
		l.Operation < NoOperation || l.Operation > OperationUnknown {
		return false
	}
	return a.Directive >= NoAggregateAction && a.Directive <= ContinueWrite && a.Freshness >= Fresh && a.Freshness <= UnavailableSnapshot && a.Lifecycle >= SessionCreating && a.Lifecycle <= SessionAborted && a.CloseProgress >= NoClose && a.CloseProgress <= CloseUnknown && a.Recovery >= AggregateReady && a.Recovery <= AggregateUnknown && a.CloseIntent >= NoCloseIntent && a.CloseIntent <= AbortSession
}
func compatibleStamps(in Input) bool {
	r, w := in.Run, in.Writer
	if !r.Stamp.Valid() || !in.InteractionStamp.Valid() || r.Stamp != in.InteractionStamp || in.TimelineHead != r.Stamp.Head || w.Revision != r.Stamp.Writer || w.Stamp.Writer != w.Revision {
		return false
	}
	if in.Local.Floor != (observation.Stamp{}) && (!in.Local.Floor.Valid() || !r.Stamp.Covers(in.Local.Floor)) {
		return false
	}
	if w.Owner == NoOwner {
		return w.Stamp.Head == "" && w.Stamp.Run == 0 && w.Stamp.Interaction == 0
	}
	if !w.Stamp.Valid() {
		return false
	}
	if w.Owner == ThisSession {
		return w.Stamp == r.Stamp && w.Generation == r.Generation && w.Authority == r.Authority && w.Access == r.Access && w.Verification == r.Verification
	}
	// Other owner Run/Interaction revisions belong to another Run domain.
	return true
}

// Validate the independently supplied variant against its typed source facts;
// never fill a missing variant by deriving it.
func variantMatches(r RunFacts) bool {
	if r.Lane == Shared {
		return r.Variant == SharedReadonly
	}
	if r.Server == ServerFailed {
		return r.Variant == DedicatedFailed
	}
	if r.Thread == Missing {
		return r.Variant == DedicatedUnstarted
	}
	switch r.Authority {
	case Active:
		return r.Variant == DedicatedActive
	case Reserved:
		return r.Variant == DedicatedReserved
	case HandoffPrepared, Releasing:
		return r.Variant == DedicatedReleasing
	case BlockedUnknown:
		return r.Variant == DedicatedBlocked
	case Unowned:
		if r.Lifecycle == Paused {
			return r.Variant == DedicatedPaused
		}
		return r.Variant == DedicatedReader
	default:
		return false
	}
}
