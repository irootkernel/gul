package action

import "github.com/rootkernel/gul/internal/observation"

// These types keep provider decision inputs independent. Zero is unknown except
// for the explicitly ownerless Writer projection's inapplicable fields.
type Lifecycle uint8

const (
	Starting Lifecycle = iota + 1
	Idle
	Running
	WaitingInteraction
	ReconciliationRequired
	Paused
	Closed
	StartFailed
	OutcomeUnknown
)

type Variant uint8

const (
	SharedReadonly Variant = iota + 1
	DedicatedUnstarted
	DedicatedReader
	DedicatedReserved
	DedicatedActive
	DedicatedReleasing
	DedicatedBlocked
	DedicatedPaused
	DedicatedFailed
)

type Control uint8

const (
	Direct Control = iota + 1
	Managed
)

type Lane uint8

const (
	Shared Lane = iota + 1
	Dedicated
)

type Access uint8

const (
	Read Access = iota + 1
	Write
	Transitioning
	Unsupported
	UnknownAccess
)

type Verification uint8

const (
	Verified Verification = iota + 1
	Unverified
	Failed
)

type Authority uint8

const (
	Unowned Authority = iota + 1
	Reserved
	Active
	HandoffPrepared
	Releasing
	BlockedUnknown
)

type Reconciliation uint8

const (
	NoReconciliation Reconciliation = iota + 1
	RetryCensus
	ReconcileLane
	OperatorRepair
	ReverifyPolicy
)

type Background uint8

const (
	Absent Background = iota + 1
	BackgroundActive
	BackgroundUnverified
	NotApplicable
)

type ServerState uint8

const (
	ServerStarting ServerState = iota + 1
	ServerReady
	ServerStopping
	ServerAbsent
	ServerUnverified
	ServerFailed
)

type Recovery uint8

const (
	NoRecovery Recovery = iota + 1
	RecoveryRequired
	ReconcileRequired
	RecoveryUnknown
)

type RecoveryAction uint8

const (
	NoRecoveryAction RecoveryAction = iota + 1
	RefreshSnapshot
	Reconcile
	Recover
	Repair
)

type Support uint8

const (
	Supported Support = iota + 1
	Unavailable
	SupportUnverified
	Unsafe
)

type Compatibility uint8

const (
	Compatible Compatibility = iota + 1
	Incompatible
	CompatibilityUnverified
	CompatibilityUnavailable
)

type Assurance uint8

const (
	BestEffort Assurance = iota + 1
	ThreadScoped
	ProcessContained
)

type Presence uint8

const (
	Missing Presence = iota + 1
	Present
)

type Freshness uint8

const (
	Fresh Freshness = iota + 1
	Stale
	UnavailableSnapshot
)

type Credential uint8

const (
	Healthy Credential = iota + 1
	CredentialMissing
	CredentialUnhealthy
	VerifiedAdoption
)

type Ownership uint8

const (
	OwnedSession Ownership = iota + 1
	ObserverSession
)

type OperationState uint8

const (
	NoOperation OperationState = iota + 1
	OperationPending
	OperationUnknown
)

type Lineage uint8

const (
	NoLineage Lineage = iota + 1
	ValidLineage
	ContinuationRequired
)

type Owner uint8

const (
	NoOwner Owner = iota + 1
	ThisSession
	ExternalOwner
)

type SessionLifecycle uint8

const (
	SessionCreating SessionLifecycle = iota + 1
	SessionActive
	SessionDegraded
	SessionRecovering
	SessionCompleting
	SessionAborting
	SessionCompleted
	SessionAborted
)

type CloseProgress uint8

const (
	NoClose CloseProgress = iota + 1
	CloseSettling
	CloseCompleted
	CloseAborted
	CloseRecovery
	CloseUnknown
)

type AggregateRecovery uint8

const (
	AggregateReady AggregateRecovery = iota + 1
	AggregateSnapshotRequired
	AggregateReconcileRequired
	AggregateUnknown
)

type CloseIntent uint8

const (
	NoCloseIntent CloseIntent = iota + 1
	CompleteSession
	AbortSession
)

type Capabilities struct {
	Checked                                                                                                  bool
	Submit, Acquire, Release, Interrupt, Resolve, Recover, Reconcile, VerifyController, Pause, Resume, Close bool
	ReaderWriter, DedicatedWriter, DurableWriter, ThreadlessAcquire, FirstWriteViaSubmit                     bool
	Transition                                                                                               Support
}
type RunFacts struct {
	Lifecycle           Lifecycle
	Variant             Variant
	Control             Control
	Lane                Lane
	Thread, ActiveTurn  Presence
	Pending             uint32
	Access              Access
	Verification        Verification
	Authority           Authority
	Generation          uint64
	Reconciliation      Reconciliation
	Requested, Achieved Assurance
	Background          Background
	Server              ServerState
	Recovery            Recovery
	RecoveryAction      RecoveryAction
	Lineage             Lineage
	Stamp               observation.Stamp
}
type WriterProjection struct {
	Authority                          Authority
	Generation                         uint64
	Access                             Access
	Verification                       Verification
	Lane                               Lane
	Requested, Achieved                Assurance
	Owner                              Owner
	Reconciliation                     Reconciliation
	BackgroundBlocked, RecoveryBlocked bool
	Revision                           uint64
	Stamp                              observation.Stamp
}
type ProfileFacts struct {
	Compatibility                 Compatibility
	Transition, BackgroundControl Support
	MaximumAssurance              Assurance
	SupportsLane                  bool
}
type AggregateDirective uint8

const (
	NoAggregateAction AggregateDirective = iota + 1
	RefreshAggregate
	ReconcileAggregate
	RecoverAggregate
	RepairAggregate
	BlockedAggregate
	ContinueWrite
)

type Aggregate struct {
	Directive                                                                                            AggregateDirective
	Freshness                                                                                            Freshness
	Revision                                                                                             uint64
	Lifecycle                                                                                            SessionLifecycle
	CloseProgress                                                                                        CloseProgress
	Recovery                                                                                             AggregateRecovery
	CloseIntent                                                                                          CloseIntent
	NonretiredMembers, NonterminalSpawns, PendingApprovals, AcceptedUnfinishedTasks, UnknownOutcomeTasks uint64
}
type LocalState struct {
	Ownership        Ownership
	Credential       Credential
	Operation        OperationState
	Floor            observation.Stamp
	ProjectionsStale bool
}
type Request struct {
	Intent             WriteIntent
	CloseIntent        CloseIntent
	InterruptConfirmed bool
}
type Input struct {
	ControllerMatches bool
	// Checked means the adapter verified identities, complete field presence,
	// typed enum values, negotiated capabilities and the root Controller binding.
	Checked          bool
	Freshness        Freshness
	Run              RunFacts
	Writer           WriterProjection
	Profile          ProfileFacts
	Capabilities     Capabilities
	InteractionStamp observation.Stamp
	TimelineHead     observation.Cursor
	Aggregate        Aggregate
	Local            LocalState
	Request          Request
}

// Flags is the complete first-release action/blocker set. These decisions do
// not grant provider authority and never assert that a session is closed.
type Flags struct {
	CanSubmitRead, CanSubmitWrite, CanAcquireWriter, CanReleaseWriter                                               bool
	CanInterrupt, CanResolveInteraction, CanRecover, CanReconcile, CanAdoptController                               bool
	CanPausePrimary, CanResumePrimary, CanRequestSessionClose                                                       bool
	RequiresCloseConfirmation, RequiresOperatorAction, RequiresFreshSnapshot                                        bool
	BlockedByOutcomeUnknown, BlockedByCredentialState, BlockedByBackgroundExecution, BlockedByProviderCompatibility bool
}
type Blocker uint8

const (
	NoBlocker Blocker = iota
	ProviderIncompatible
	FreshSnapshotRequired
	CredentialBlocked
	UnresolvedOutcome
	BackgroundBlocked
	WriterBusy
	UnsupportedTransition
	ActiveTurnDraft
	InterruptConfirmationRequired
	RecoveryBlocked
	ProviderFailure
)

type Evaluation struct {
	Flags   Flags
	Blocker Blocker
	Writer  WriterProjection
	Mode    WriterMode
}
