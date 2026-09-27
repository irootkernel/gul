// Package observation owns non-authoritative event invalidation and delivery.
package observation

import (
	"context"
	"errors"
	"strconv"
	"time"
)

var (
	ErrInvalid      = errors.New("invalid provider observation")
	ErrUnbound      = errors.New("observation binding unavailable")
	ErrSlowConsumer = errors.New("observation consumer requires snapshot")
	ErrRefresh      = errors.New("observation requires snapshot")
)

// Cursor retains the provider wire value. It is never a browser sequence.
type Cursor string
type Sequence int64

func (c Cursor) Valid() bool {
	n, err := strconv.ParseUint(string(c), 10, 64)
	return err == nil && strconv.FormatUint(n, 10) == string(c)
}
func (c Cursor) Compare(other Cursor) int {
	if len(c) < len(other) {
		return -1
	}
	if len(c) > len(other) {
		return 1
	}
	if c < other {
		return -1
	}
	if c > other {
		return 1
	}
	return 0
}

type Stamp struct {
	Head                     Cursor
	Run, Writer, Interaction uint64
}

func (s Stamp) Valid() bool {
	return s.Head.Valid() && s.Run > 0 && s.Head == Cursor(strconv.FormatUint(s.Run, 10)) && s.Interaction <= s.Run
}
func (s Stamp) Covers(f Stamp) bool {
	return s.Head.Compare(f.Head) >= 0 && s.Run >= f.Run && s.Writer >= f.Writer && s.Interaction >= f.Interaction
}

type Refresh uint16

const (
	Run Refresh = 1 << iota
	Writer
	Interaction
	Timeline
	Artifacts
	Files
	Profile
	Session
)
const AllAggregates = Run | Writer | Interaction | Timeline

// Event contains only validated identity and invalidation metadata. Sensitive
// interaction details, commands, paths and runtime diagnostics never cross it.
type Event struct {
	Kind                   string
	Cursor                 Cursor
	ID, RunID, WorkspaceID string
	Stamp                  Stamp
	At                     time.Time
	Replay                 bool
	Refresh                Refresh
	Priority               bool
}

type Envelope struct {
	Event     *Event
	RunID     string
	Head      Cursor
	Heartbeat time.Time
	End       string // terminal or shutdown; never SLOW_CONSUMER
}

type Binding struct {
	SubjectID, SessionID, ProviderID, RunID, WorkspaceID, AbsoluteRoot string
}

func (b Binding) Valid() bool {
	return b.SubjectID != "" && b.SessionID != "" && b.ProviderID != "" && b.RunID != "" && b.WorkspaceID != "" && b.AbsoluteRoot != ""
}

type Checkpoint struct {
	Validated, Committed Cursor
	Stamp                Stamp
}
type Invalidation struct {
	Binding       Binding
	Cursor        Cursor
	Floor         Stamp
	Refresh       Refresh
	CorrelationID string
	At            time.Time
}

// Notification is a browser allowlist; provider identities and cursors stay in
// the backend. It instructs clients to reread current authorized projections.
type Notification struct {
	Sequence      Sequence  `json:"delivery_sequence"`
	SessionID     string    `json:"session_id"`
	CorrelationID string    `json:"correlation_id"`
	Kind          string    `json:"kind"`
	CreatedAt     time.Time `json:"created_at"`
}

type Repository interface {
	Checkpoint(context.Context, Binding) (Checkpoint, error)
	Validate(context.Context, Binding, Cursor) error
	// Commit persists invalidations and the committed cursor before allocating
	// delivery in the same transaction. Failure must leave both unchanged.
	Commit(context.Context, Invalidation) (Notification, error)
}
type Stream interface {
	Receive() (Envelope, error)
	Close() error
}
type Provider interface {
	Watch(context.Context, Binding, Cursor) (Stream, error)
}

// Refresher performs mandatory authoritative reads without a database lock.
// It must never enable actions from an event alone or publish raw provider data.
type Refresher interface {
	Refresh(context.Context, Binding, Refresh, Stamp) error
}

type SubscriptionState struct {
	RunID                       string
	Validated, Committed        Cursor
	Generation                  uint64
	Connection                  string
	LastHeartbeat, LastSnapshot time.Time
	ReconnectAttempts           uint64
}

const MaximumReplay = 256

type DeliveryBatch struct {
	Events           []Notification
	Head             Sequence
	SnapshotRequired bool
}
type DeliveryReader interface {
	ReadDelivery(context.Context, string, string, Sequence) (DeliveryBatch, error)
}
