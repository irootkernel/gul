// Package interaction owns authorized cards and one-shot user responses.
package interaction

import (
	"context"
	"errors"
	"github.com/rootkernel/gul/internal/action"
	"time"

	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

const GUL_MAX_SAFE_INTERACTION_PAYLOAD_BYTES = 8 * 1024 * 1024
const MaximumResponseBytes = 64 * 1024

var (
	ErrInvalid     = errors.New("invalid interaction request")
	ErrBlocked     = errors.New("interaction provider contract blocked")
	ErrAuthority   = errors.New("interaction controller binding unavailable")
	ErrUnavailable = errors.New("interaction unavailable")
	ErrPath        = errors.New("interaction path cannot be presented safely")
)

type Kind uint8

const (
	CommandApproval Kind = iota + 1
	FileApproval
	UserInput
	Unsupported
)

type Status uint8

const (
	Pending Status = iota + 1
	Resolved
	Stale
)

type Decision uint8

const (
	AcceptOnce Decision = iota + 1
	Decline
	Cancel
)

type Summary struct {
	ID                      string
	Kind                    Kind
	Status                  Status
	CreatedAt               time.Time
	ExpiresAt, ResolvedAt   *time.Time
	Protected, RequiresUser bool
}
type Command struct {
	Title, Message, Reason, WorkingDirectory string
	Arguments                                []string
}
type Change struct{ Path, Kind, Diff, MovePath string }
type File struct {
	Title, Message, Reason string
	Changes                []Change
	VerifiedDiff           string
}
type Option struct{ Label, Description string }
type Question struct {
	ID, Header, Prompt  string
	AllowsOther, Secret bool
	Options             []Option
}
type Input struct {
	Blocking  bool
	Questions []Question
}
type Card struct {
	Actions           action.Evaluation
	Summary           Summary
	Decisions         []Decision
	Command           *Command
	File              *File
	Input             *Input
	UnsupportedReason string
}

// Bound is backend-only; no browser supplies a Run, Workspace path or carrier.
type Bound struct {
	Binding   session.Binding
	Workspace workspace.Attachment
	Carrier   session.Carrier
}
type PendingState struct {
	Items []Summary
	Stamp observation.Stamp
}
type Resolution struct {
	Status  Status
	Receipt string
}
type Outcome uint8

const (
	OutcomeUnknown Outcome = iota
	OutcomeResolved
	OutcomeReenter
	OutcomeStale
)

type Result struct {
	Outcome Outcome
	Receipt string
	Card    *Card
}

type Provider interface {
	ResponseLimit() int
	Pending(context.Context, Bound) (PendingState, error)
	Card(context.Context, Bound, string) (Card, error)
	// Resolve is exactly one invocation, with no transport retry or hedging.
	Resolve(context.Context, Bound, string, string, []byte) (Resolution, error)
	Observe(context.Context, Bound) (PendingState, error)
}
type Repository interface {
	Binding(context.Context, string, string) (session.Binding, error)
}
type Notifier interface {
	InteractionChanged(context.Context, Bound, observation.Stamp) error
}
