// Package history reconstructs non-authoritative, Controller-safe presentation.
package history

import (
	"context"
	"errors"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
	"time"
)

const (
	MaximumArtifactBytes = 64 << 20
	MaximumChunkBytes    = 256 << 10
	MaximumInlineBytes   = 256 << 10
	MaximumProviderPages = 4
	MaximumObjects       = 4096
)

var (
	ErrInvalid     = errors.New("invalid history request")
	ErrAuthority   = errors.New("history authority unavailable")
	ErrUnavailable = errors.New("history source unavailable; refresh traversal")
	ErrBlocked     = errors.New("history source incompatible")
	ErrLimit       = errors.New("history presentation limit exceeded")
)

type Bound struct {
	Floor     observation.Stamp
	Binding   session.Binding
	Workspace workspace.Attachment
	Carrier   session.Carrier
}
type Repository interface {
	Binding(context.Context, string, string) (session.Binding, error)
}
type Kind uint8

const (
	Human Kind = iota + 1
	Assistant
	InteractionOpened
	InteractionResolved
	TurnTerminal
)

type Artifact struct {
	ID, MediaType, SHA256 string
	Length                uint64
	Kind, Visibility      uint32
}
type Content struct {
	Inline   *string
	Artifact *Artifact
}
type Image struct {
	Ordinal           uint32
	Detail, MediaType string
	Length            uint64
	SHA256            string
}
type SourceEntry struct {
	Cursor, ProviderID, TurnID, InteractionID         string
	Kind                                              Kind
	Status, InteractionKind, InteractionStatus, Title string
	At                                                time.Time
	Content                                           Content
	Images                                            []Image
}
type Snapshot struct {
	Stamp            observation.Stamp
	State            string
	Final            *Content
	FinalUnavailable bool
}
type Timeline struct {
	Items      []SourceEntry
	Head, Next string
	Stamp      observation.Stamp
}
type SourceResult struct {
	ID, TaskID, SpecialistID, Role string
	Order                          uint64
	At                             time.Time
	Artifact                       Artifact
}
type Results struct {
	Items          []SourceResult
	Head, Revision uint64
	CapturedAt     time.Time
	Next           string
}
type Provider interface {
	Snapshot(context.Context, Bound) (Snapshot, error)
	Timeline(context.Context, Bound, string, uint32) (Timeline, error)
	Results(context.Context, Bound, string, uint32) (Results, error)
	ReadArtifact(context.Context, Bound, Artifact) ([]byte, error)
}

// Browser records contain only Gul identities and allowlisted presentation.
type Entry struct {
	ID                string    `json:"entry_id"`
	PromptID          string    `json:"prompt_item_id,omitempty"`
	TurnRef           string    `json:"turn_ref"`
	Kind              Kind      `json:"kind"`
	Status            string    `json:"status"`
	At                time.Time `json:"occurred_at"`
	Ordinal           uint64    `json:"ordinal,omitempty"`
	Preview           string    `json:"preview"`
	Truncated         bool      `json:"preview_truncated"`
	HasOriginal       bool      `json:"has_original"`
	InteractionRef    string    `json:"interaction_ref,omitempty"`
	InteractionKind   string    `json:"interaction_kind,omitempty"`
	InteractionStatus string    `json:"interaction_status,omitempty"`
	Title             string    `json:"title,omitempty"`
	Images            []Image   `json:"images,omitempty"`
}
type Result struct {
	ID, ViewID, Role, ArtifactRef string
	Order                         uint64
	At                            time.Time
	Length                        uint64
	SHA256                        string
}
type Page[T any] struct {
	SnapshotID string    `json:"snapshot_id"`
	Items      []T       `json:"items"`
	Next       string    `json:"next_page_token,omitempty"`
	Complete   bool      `json:"traversal_complete"`
	ObservedAt time.Time `json:"observed_at"`
	RunState   string    `json:"run_state,omitempty"`
}
type Original struct {
	Entry       Entry
	Inline      *string
	ArtifactRef string
}
type Metadata struct {
	Ref, MediaType, SHA256 string
	Length                 uint64
}
type Chunk struct {
	Data   []byte
	Length uint64
	SHA256 string
}
