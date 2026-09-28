package action

import (
	"reflect"
	"testing"

	"github.com/rootkernel/gul/internal/observation"
)

func readerInput() Input {
	stamp := observation.Stamp{Head: "4", Run: 4, Writer: 2, Interaction: 3}
	return Input{ControllerMatches: true, Checked: true, Freshness: Fresh, Run: RunFacts{Lifecycle: Idle, Variant: DedicatedReader, Control: Direct, Lane: Dedicated, Thread: Present, ActiveTurn: Missing, Access: Read, Verification: Verified, Authority: Unowned, Reconciliation: NoReconciliation, Requested: BestEffort, Achieved: BestEffort, Background: Absent, Server: ServerReady, Recovery: NoRecovery, RecoveryAction: NoRecoveryAction, Lineage: NoLineage, Stamp: stamp}, Writer: WriterProjection{Authority: Unowned, Access: UnknownAccess, Verification: Unverified, Owner: NoOwner, Reconciliation: NoReconciliation, Revision: 2, Stamp: observation.Stamp{Writer: 2}}, Profile: ProfileFacts{Compatibility: Compatible, Transition: Supported, BackgroundControl: Supported, MaximumAssurance: ProcessContained, SupportsLane: true}, Capabilities: Capabilities{Checked: true, Submit: true, Acquire: true, Release: true, Interrupt: true, Resolve: true, Recover: true, Reconcile: true, VerifyController: true, Pause: true, Resume: true, Close: true, ReaderWriter: true, DurableWriter: true, FirstWriteViaSubmit: true, Transition: Supported}, InteractionStamp: stamp, TimelineHead: "4", Aggregate: Aggregate{Directive: NoAggregateAction, Freshness: Fresh, Revision: 91, Lifecycle: SessionActive, CloseProgress: NoClose, Recovery: AggregateReady, CloseIntent: NoCloseIntent, NonretiredMembers: 1}, Local: LocalState{Ownership: OwnedSession, Credential: Healthy, Operation: NoOperation}, Request: Request{Intent: IntentRead, CloseIntent: CompleteSession}}
}
func writerInput() Input {
	in := readerInput()
	in.Run.Variant = DedicatedActive
	in.Run.Access = Write
	in.Run.Authority = Active
	in.Run.Generation = 8
	in.Writer = WriterProjection{Authority: Active, Generation: 8, Access: Write, Verification: Verified, Lane: Dedicated, Requested: BestEffort, Achieved: BestEffort, Owner: ThisSession, Reconciliation: NoReconciliation, Revision: 2, Stamp: in.Run.Stamp}
	in.Request.Intent = IntentWrite
	return in
}
func TestClosedFlagsAndWriterAdmission(t *testing.T) {
	if reflect.TypeFor[Flags]().NumField() != 19 {
		t.Fatal("first-release action set changed")
	}
	in := readerInput()
	got := Evaluate(in)
	if !got.Flags.CanSubmitRead || !got.Flags.CanAcquireWriter || got.Flags.CanSubmitWrite || got.Flags.CanReleaseWriter || !got.Flags.CanRequestSessionClose || got.Mode != WriterReadOnly {
		t.Fatal(got)
	}
	in = writerInput()
	got = Evaluate(in)
	if !got.Flags.CanSubmitWrite || !got.Flags.CanReleaseWriter || got.Flags.CanAcquireWriter || got.Mode != WriterWrite {
		t.Fatal(got)
	}
	in.Request.Intent = IntentRead
	if got := Evaluate(in); !got.Flags.CanSubmitRead || got.Mode != WriterWrite {
		t.Fatal(got)
	}
	in = readerInput()
	in.Run.Thread = Missing
	in.Run.Variant = DedicatedUnstarted
	in.Run.Access = UnknownAccess
	in.Run.Verification = Unverified
	in.Request.Intent = IntentWrite
	if got := Evaluate(in); got.Flags.CanAcquireWriter || !got.Flags.CanSubmitWrite {
		t.Fatal("threadless first write", got)
	}
	in.Capabilities.FirstWriteViaSubmit = false
	if Evaluate(in).Flags.CanSubmitWrite {
		t.Fatal("unadvertised first write")
	}
}
func TestActivePromptDraftDoesNotBlockCurrentAnswerAndRequiresInterruptConsent(t *testing.T) {
	in := writerInput()
	in.Run.ActiveTurn = Present
	in.Run.Lifecycle = WaitingInteraction
	in.Run.Pending = 1
	got := Evaluate(in)
	if got.Flags.CanSubmitRead || got.Flags.CanSubmitWrite || got.Flags.CanAcquireWriter || got.Flags.CanReleaseWriter || !got.Flags.CanResolveInteraction || got.Flags.CanInterrupt || !got.Flags.RequiresCloseConfirmation || got.Flags.CanRequestSessionClose || got.Blocker != ActiveTurnDraft {
		t.Fatal(got)
	}
	in.Request.InterruptConfirmed = true
	got = Evaluate(in)
	if !got.Flags.CanInterrupt || !got.Flags.CanRequestSessionClose || got.Flags.RequiresCloseConfirmation || !got.Flags.CanResolveInteraction {
		t.Fatal(got)
	}
}
func TestUnsupportedTransitionPreservesSourceAndExternalWriterHasNoTakeover(t *testing.T) {
	for _, support := range []Support{Unavailable, SupportUnverified, Unsafe} {
		in := readerInput()
		in.Profile.Transition = support
		in.Request.Intent = IntentWrite
		got := Evaluate(in)
		if got.Blocker != UnsupportedTransition || got.Flags.CanAcquireWriter || got.Flags.CanSubmitWrite {
			t.Fatal(got)
		}
		in.Request.Intent = IntentRead
		if !Evaluate(in).Flags.CanSubmitRead {
			t.Fatal("fixed read lost")
		}
		in = writerInput()
		in.Capabilities.Transition = support
		in.Request.Intent = IntentRead
		got = Evaluate(in)
		if got.Blocker != UnsupportedTransition || got.Flags.CanReleaseWriter || got.Flags.CanSubmitRead {
			t.Fatal(got)
		}
	}
	in := readerInput()
	in.Writer = writerInput().Writer
	in.Writer.Owner = ExternalOwner
	in.Writer.Stamp = observation.Stamp{Head: "99", Run: 99, Writer: 2, Interaction: 90}
	in.Request.Intent = IntentWrite
	got := Evaluate(in)
	if got.Blocker != WriterBusy || got.Flags.CanAcquireWriter || got.Flags.CanReleaseWriter || got.Flags.CanSubmitWrite || got.Flags.RequiresFreshSnapshot {
		t.Fatal("foreign Run domain or takeover", got)
	}
}
func TestFreshnessAndIndependentAggregateRevision(t *testing.T) {
	for _, change := range []func(*Input){
		func(in *Input) { in.Freshness = Stale }, func(in *Input) { in.Local.ProjectionsStale = true }, func(in *Input) { in.InteractionStamp.Run++ }, func(in *Input) { in.TimelineHead = "3" }, func(in *Input) { in.Writer.Revision++ }, func(in *Input) { in.Local.Floor = observation.Stamp{Head: "5", Run: 5, Writer: 2, Interaction: 3} },
	} {
		in := readerInput()
		change(&in)
		got := Evaluate(in)
		if !got.Flags.RequiresFreshSnapshot || got.Flags.CanAcquireWriter || got.Flags.CanSubmitRead {
			t.Fatal(got)
		}
	}
	in := readerInput()
	in.Aggregate.Revision = 999999
	if !Evaluate(in).Flags.CanRequestSessionClose {
		t.Fatal("aggregate revision compared to Run")
	}
	in.Aggregate.Freshness = Stale
	if got := Evaluate(in); got.Flags.CanRequestSessionClose || !got.Flags.RequiresFreshSnapshot {
		t.Fatal(got)
	}
}
func TestUnresolvedRecoveryCredentialAndBackgroundMatrix(t *testing.T) {
	for _, change := range []func(*Input){func(in *Input) { in.Local.Operation = OperationPending }, func(in *Input) { in.Local.Operation = OperationUnknown }, func(in *Input) { in.Run.Recovery = RecoveryUnknown }, func(in *Input) { in.Aggregate.UnknownOutcomeTasks = 1 }} {
		in := writerInput()
		change(&in)
		got := Evaluate(in)
		if !got.Flags.BlockedByOutcomeUnknown || got.Flags.CanSubmitWrite || got.Flags.CanReleaseWriter || got.Flags.CanRequestSessionClose {
			t.Fatal(got)
		}
	}
	in := readerInput()
	in.Run.Recovery = RecoveryRequired
	in.Run.RecoveryAction = Recover
	if got := Evaluate(in); !got.Flags.CanRecover || got.Flags.CanAcquireWriter {
		t.Fatal(got)
	}
	in.Aggregate.Freshness = Stale
	if Evaluate(in).Flags.CanRecover {
		t.Fatal("stale aggregate recovery")
	}
	in = readerInput()
	in.Run.Recovery = ReconcileRequired
	in.Run.RecoveryAction = Reconcile
	if !Evaluate(in).Flags.CanReconcile {
		t.Fatal("reconcile missing")
	}
	in = writerInput()
	in.Run.Background = BackgroundUnverified
	if got := Evaluate(in); !got.Flags.BlockedByBackgroundExecution || got.Flags.CanReleaseWriter || got.Flags.CanSubmitWrite || got.Flags.CanRequestSessionClose {
		t.Fatal(got)
	}
	in = readerInput()
	in.Local.Credential = CredentialMissing
	if got := Evaluate(in); !got.Flags.BlockedByCredentialState || got.Flags.CanAcquireWriter {
		t.Fatal(got)
	}
	in.Local.Credential = VerifiedAdoption
	if !Evaluate(in).Flags.CanAdoptController {
		t.Fatal("verified adoption missing")
	}
}
func TestMissingAndUnknownTypedInputsFailClosed(t *testing.T) {
	for _, change := range []func(*Input){
		func(in *Input) { in.Checked = false }, func(in *Input) { in.Capabilities.Checked = false }, func(in *Input) { in.Run.Lifecycle = 0 }, func(in *Input) { in.Run.Variant = 99 }, func(in *Input) { in.Run.Control = 0 }, func(in *Input) { in.Run.Lane = 0 }, func(in *Input) { in.Run.Thread = 0 }, func(in *Input) { in.Run.ActiveTurn = 0 }, func(in *Input) { in.Run.Access = 0 }, func(in *Input) { in.Run.Verification = 0 }, func(in *Input) { in.Run.Authority = 0 }, func(in *Input) { in.Run.Reconciliation = 0 }, func(in *Input) { in.Run.Background = 0 }, func(in *Input) { in.Run.Recovery = 0 }, func(in *Input) { in.Run.RecoveryAction = 0 }, func(in *Input) { in.Run.Lineage = 0 }, func(in *Input) { in.Run.Requested = 0 }, func(in *Input) { in.Writer.Authority = 0 }, func(in *Input) { in.Writer.Owner = 0 }, func(in *Input) { in.Profile.Compatibility = 0 }, func(in *Input) { in.Profile.Transition = 0 }, func(in *Input) { in.Capabilities.Transition = 0 }, func(in *Input) { in.Local.Credential = 0 }, func(in *Input) { in.Local.Operation = 0 }, func(in *Input) { in.Aggregate.CloseProgress = 0 },
	} {
		in := readerInput()
		change(&in)
		got := Evaluate(in)
		if !got.Flags.BlockedByProviderCompatibility || got.Flags.CanSubmitRead || got.Flags.CanAcquireWriter || got.Flags.CanRequestSessionClose {
			t.Fatal(got)
		}
	}
}

func TestAggregateDirectionsAndCloseWorkMatrix(t *testing.T) {
	for _, directive := range []AggregateDirective{0, RefreshAggregate, ReconcileAggregate, RecoverAggregate, RepairAggregate, BlockedAggregate, ContinueWrite, 99} {
		in := readerInput()
		in.Aggregate.Directive = directive
		got := Evaluate(in)
		if got.Flags.CanSubmitRead || got.Flags.CanAcquireWriter || got.Flags.CanRequestSessionClose {
			t.Fatal(directive, got)
		}
		if directive == ReconcileAggregate && !got.Flags.CanReconcile || directive == RecoverAggregate && !got.Flags.CanRecover || directive == RepairAggregate && !got.Flags.RequiresOperatorAction || directive == ContinueWrite && got.Blocker != UnsupportedTransition {
			t.Fatal(directive, got)
		}
		in.Aggregate.Freshness = Stale
		got = Evaluate(in)
		if got.Flags.CanReconcile || got.Flags.CanRecover {
			t.Fatal("stale session action", got)
		}
	}
	for _, change := range []func(*Input){
		func(in *Input) { in.Run.Background = BackgroundActive },
		func(in *Input) { in.Aggregate.NonretiredMembers = 2 },
		func(in *Input) { in.Aggregate.NonterminalSpawns = 1 },
		func(in *Input) { in.Aggregate.PendingApprovals = 1 },
		func(in *Input) { in.Aggregate.AcceptedUnfinishedTasks = 1 },
	} {
		in := readerInput()
		change(&in)
		if got := Evaluate(in); got.Flags.CanRequestSessionClose || !got.Flags.RequiresCloseConfirmation {
			t.Fatal(got)
		}
		in.Request.InterruptConfirmed = true
		if got := Evaluate(in); !got.Flags.CanRequestSessionClose || got.Flags.RequiresCloseConfirmation {
			t.Fatal(got)
		}
	}
}

func TestIndependentPolicyVariantAndCapabilityGuards(t *testing.T) {
	for _, change := range []func(*Input){
		func(in *Input) { in.Run.Variant = DedicatedActive },
		func(in *Input) { in.Run.Generation = 1 },
		func(in *Input) { in.Profile.SupportsLane = false },
		func(in *Input) { in.Run.Achieved = ThreadScoped; in.Profile.MaximumAssurance = BestEffort },
		func(in *Input) { in.Run.Requested = ThreadScoped },
	} {
		in := readerInput()
		change(&in)
		if got := Evaluate(in); !got.Flags.BlockedByProviderCompatibility || got.Flags.CanSubmitRead {
			t.Fatal(got)
		}
	}
	for _, state := range []Reconciliation{RetryCensus, ReconcileLane, OperatorRepair, ReverifyPolicy} {
		in := readerInput()
		in.Run.Reconciliation = state
		if got := Evaluate(in); got.Flags.CanAcquireWriter || got.Flags.CanSubmitRead {
			t.Fatal(got)
		}
	}
	in := readerInput()
	in.Capabilities = Capabilities{Checked: true, Transition: Supported}
	if got := Evaluate(in); got.Flags.CanAcquireWriter || got.Flags.CanSubmitRead || got.Flags.CanRequestSessionClose || got.Flags.CanPausePrimary {
		t.Fatal(got)
	}
	in = readerInput()
	in.Run.Lifecycle = Paused
	in.Run.Variant = DedicatedPaused
	if got := Evaluate(in); !got.Flags.CanResumePrimary || got.Flags.CanPausePrimary {
		t.Fatal(got)
	}
}

func TestStaleAggregateDisablesEveryAction(t *testing.T) {
	for _, change := range []func(*Input){func(in *Input) { in.Aggregate.Freshness = Stale }, func(in *Input) { in.Aggregate.Freshness = UnavailableSnapshot }, func(in *Input) { in.Aggregate.Revision = 0 }} {
		in := readerInput()
		change(&in)
		got := Evaluate(in)
		if !got.Flags.RequiresFreshSnapshot || got.Blocker != FreshSnapshotRequired {
			t.Fatal(got)
		}
		flags := reflect.ValueOf(got.Flags)
		for i := 0; i < 12; i++ {
			if flags.Field(i).Bool() {
				t.Fatalf("stale aggregate enabled %s", flags.Type().Field(i).Name)
			}
		}
	}
}

func TestExplicitUnknownOutcomeReconciliationAndFailedRunRecovery(t *testing.T) {
	in := readerInput()
	in.Run.Lifecycle = OutcomeUnknown
	in.Run.Recovery = RecoveryUnknown
	in.Run.RecoveryAction = Reconcile
	got := Evaluate(in)
	if !got.Flags.BlockedByOutcomeUnknown || !got.Flags.CanReconcile || got.Flags.CanSubmitRead || got.Flags.CanAcquireWriter || got.Flags.CanRequestSessionClose {
		t.Fatal(got)
	}
	in.Capabilities.Reconcile = false
	if Evaluate(in).Flags.CanReconcile {
		t.Fatal("unadvertised reconciliation")
	}
	in.Capabilities.Reconcile = true
	in.Run.ActiveTurn = Present
	if Evaluate(in).Flags.CanReconcile {
		t.Fatal("implicit interruption consent")
	}
	in.Request.InterruptConfirmed = true
	if !Evaluate(in).Flags.CanReconcile {
		t.Fatal("confirmed reconciliation blocked")
	}
	in.Local.Operation = OperationPending
	if Evaluate(in).Flags.CanReconcile {
		t.Fatal("concurrent unresolved dispatch")
	}
	in = readerInput()
	in.Aggregate.Recovery = AggregateUnknown
	in.Aggregate.Directive = ReconcileAggregate
	if got := Evaluate(in); !got.Flags.CanReconcile || !got.Flags.BlockedByOutcomeUnknown {
		t.Fatal(got)
	}
	in = readerInput()
	in.Run.Server = ServerFailed
	in.Run.Variant = DedicatedFailed
	in.Run.Lifecycle = StartFailed
	in.Run.Recovery = RecoveryRequired
	in.Run.RecoveryAction = Recover
	if got := Evaluate(in); !got.Flags.CanRecover || got.Flags.CanSubmitRead {
		t.Fatal(got)
	}
}

func TestUnknownPolicyOnlyAdmitsTheUnstartedFirstTurn(t *testing.T) {
	in := readerInput()
	in.Run.Thread = Missing
	in.Run.Variant = DedicatedUnstarted
	in.Run.Access = UnknownAccess
	in.Run.Verification = Unverified
	in.Request.Intent = IntentWrite
	if got := Evaluate(in); !got.Flags.CanSubmitWrite || !got.Flags.CanSubmitRead || got.Flags.CanAcquireWriter || got.Mode == WriterWrite {
		t.Fatal(got)
	}
	in.Run.Thread = Present
	in.Run.Variant = DedicatedReader
	if got := Evaluate(in); got.Flags.CanSubmitRead || got.Flags.CanSubmitWrite {
		t.Fatal("unknown existing-thread policy admitted", got)
	}
}

func TestRecoveryInstructionsRequireConsentForActiveOwnedWork(t *testing.T) {
	work := map[string]func(*Input){
		"primary":    func(in *Input) { in.Run.ActiveTurn = Present },
		"background": func(in *Input) { in.Run.Background = BackgroundActive },
		"members":    func(in *Input) { in.Aggregate.NonretiredMembers = 2 },
		"spawns":     func(in *Input) { in.Aggregate.NonterminalSpawns = 1 },
		"approvals":  func(in *Input) { in.Aggregate.PendingApprovals = 1 },
		"tasks":      func(in *Input) { in.Aggregate.AcceptedUnfinishedTasks = 1 },
	}
	instructions := map[string]func(*Input){
		"run-recover":         func(in *Input) { in.Run.Recovery = RecoveryRequired; in.Run.RecoveryAction = Recover },
		"run-reconcile":       func(in *Input) { in.Run.Recovery = ReconcileRequired; in.Run.RecoveryAction = Reconcile },
		"aggregate-reconcile": func(in *Input) { in.Aggregate.Directive = ReconcileAggregate },
		"aggregate-recover":   func(in *Input) { in.Aggregate.Directive = RecoverAggregate },
		"unknown-reconcile":   func(in *Input) { in.Local.Operation = OperationUnknown; in.Run.RecoveryAction = Reconcile },
		"unknown-recover":     func(in *Input) { in.Aggregate.Recovery = AggregateUnknown; in.Aggregate.Directive = RecoverAggregate },
	}
	for name, change := range work {
		for instruction, recovery := range instructions {
			t.Run(name+"/"+instruction, func(t *testing.T) {
				in := readerInput()
				change(&in)
				recovery(&in)
				if got := Evaluate(in); got.Flags.CanReconcile || got.Flags.CanRecover {
					t.Fatal("missing consent admitted", got)
				}
				in.Request.InterruptConfirmed = true
				if got := Evaluate(in); !got.Flags.CanReconcile && !got.Flags.CanRecover {
					t.Fatal("confirmed recovery blocked", got)
				}
			})
		}
	}
}

func TestStaleAggregateCannotEnableControllerAdoption(t *testing.T) {
	in := readerInput()
	in.Local.Credential = VerifiedAdoption
	in.Aggregate.Freshness = Stale
	if got := Evaluate(in); got.Flags.CanAdoptController || !got.Flags.RequiresFreshSnapshot {
		t.Fatal(got)
	}
}
