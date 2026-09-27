// Package sessionclose coordinates root-only session closure and recovery.
package sessionclose

import (
	"context"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
)

var (
	ErrInvalid     = errors.New("invalid close request")
	ErrConflict    = errors.New("conflicting session operation")
	ErrUnavailable = errors.New("session operation unavailable")
)

type Status string

const (
	Rejected         Status = "rejected"
	InProgress       Status = "in_progress"
	Confirmed        Status = "confirmed"
	OutcomeUnknown   Status = "outcome_unknown"
	RecoveryRequired Status = "recovery_required"
)

type Kind string

// Mutation deadlines apply to both coordination and checked provider dispatch.
const (
	CloseTimeout    = 10 * time.Second
	RecoveryTimeout = 60 * time.Second
)

const (
	Close     Kind = "CloseRun"
	Recover   Kind = "RecoverRun"
	Reconcile Kind = "ReconcileRun"
)

// Outcome contains only Gul-owned browser references and closed error codes.
type Outcome struct {
	Status                  Status
	AttemptID, OperationRef string
	Code, NextAction        string
}

// Attempt is non-secret coordination metadata, never provider outcome authority.
type Attempt struct {
	ID, SubjectID, SessionID, RequestID, RequestSHA256 string
	Kind                                               Kind
	Interrupt                                          bool
	DispatchFinished                                   bool
	CreatedAt                                          time.Time
	Outcome                                            Outcome
}

type Repository interface {
	Binding(context.Context, string, string) (session.Binding, error)
	Find(context.Context, string, string, string) (Attempt, bool, error)
	Pending(context.Context, string, string) ([]Attempt, error)
	// Begin atomically rejects conflicting pending/unknown operations, records
	// the attempt and its generic operation blocker before any provider call.
	Begin(context.Context, action.Bound, Attempt) (Attempt, bool, error)
	Save(context.Context, Attempt) error
	OperationReference(context.Context, session.Binding, string) (string, error)
}

// Mutation carries a validated immediate Run only. Other projections must be
// read independently before any final outcome or action can be enabled.
type Mutation struct {
	Run                       action.RunFacts
	OperationID               string // Backend-only provider correlation.
	Pending                   bool
	RejectionCode, NextAction string
}

type Provider interface {
	Mutate(context.Context, action.Bound, Kind, uint64, bool) (Mutation, error)
}

type Refresher interface {
	// Refresh invalidates affected caches first, then reads Session, Writer,
	// Interaction, timeline and any newly referenced artifacts independently.
	Refresh(context.Context, action.Bound, action.RunFacts) error
}
