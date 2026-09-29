package domain

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
)

const (
	DefaultPageSize               = gulv1.DefaultPageSize
	MaximumPageSize               = gulv1.MaximumPageSize
	MaximumTokenBytes             = gulv1.MaximumTokenBytes
	MaximumPageMetadataBytes      = gulv1.MaximumPageMetadataBytes
	MaximumPreviewBytes           = gulv1.MaximumPreviewBytes
	MaximumFileTextBytes          = gulv1.MaximumFileTextBytes
	MaximumFileTextLines          = gulv1.MaximumFileTextLines
	MaximumFileImageBytes         = gulv1.MaximumFileImageBytes
	MaximumFileImagePixels        = gulv1.MaximumFileImagePixels
	MaximumFileMarkdownImages     = gulv1.MaximumFileMarkdownImages
	MaximumFileMarkdownImageBytes = gulv1.MaximumFileMarkdownImageBytes
	MaximumLivePageTokens         = 4096
)

var (
	ErrInvalidPageToken  = errors.New("invalid Gul page token")
	ErrPageTokenExpired  = errors.New("Gul page token expired")
	ErrInvalidPageSize   = errors.New("invalid Gul page size")
	ErrPageTokenCapacity = errors.New("Gul page token capacity reached")
)

type QueryKind uint8

const (
	PromptHistoryQuery QueryKind = iota + 1
	SpecialistResultsQuery
	ConversationQuery
)

// PageScope is the authorization and projection context of one traversal.
// Provider cursors remain private in the associated PagePosition.
type PageScope struct {
	PageBinding
	SnapshotID string
}

type PageBinding struct {
	SourceIdentity    string
	AccountID         string
	SessionID         string
	Query             QueryKind
	ProjectionVersion uint64
}

type PagePosition struct {
	SourceRevision uint64
	CapturedAt     time.Time
	ProviderCursor string
	CapturedHead   string
	ScannedCount   uint64
}

type ResolvedPage struct {
	SnapshotID string
	Position   PagePosition
}

type pageMapping struct {
	scope     PageScope
	position  PagePosition
	expiresAt time.Time
}

// PageTokens keeps opaque browser handles separate from provider cursors.
// An empty store after restart intentionally expires prior handles.
type PageTokens struct {
	mu       sync.Mutex
	mappings map[string]pageMapping
}

func NewPageTokens() *PageTokens {
	return &PageTokens{mappings: make(map[string]pageMapping)}
}

func BoundPageSize(requested uint32) (uint32, error) {
	if requested == 0 {
		return DefaultPageSize, nil
	}
	if requested > MaximumPageSize {
		return 0, ErrInvalidPageSize
	}
	return requested, nil
}

func (p *PageTokens) Issue(scope PageScope, position PagePosition, now, expiresAt time.Time) (string, error) {
	if !validScope(scope) || !now.Before(expiresAt) {
		return "", ErrInvalidPageToken
	}
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes[:])
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.mappings) >= MaximumLivePageTokens {
		for candidate, mapping := range p.mappings {
			if !now.Before(mapping.expiresAt) {
				delete(p.mappings, candidate)
			}
		}
	}
	if len(p.mappings) >= MaximumLivePageTokens {
		return "", ErrPageTokenCapacity
	}
	p.mappings[token] = pageMapping{scope: scope, position: position, expiresAt: expiresAt}
	return token, nil
}

func (p *PageTokens) Resolve(binding PageBinding, token string, now time.Time) (ResolvedPage, error) {
	if !validBinding(binding) || len(token) > MaximumTokenBytes || len(token) != 43 {
		return ResolvedPage{}, ErrInvalidPageToken
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return ResolvedPage{}, ErrInvalidPageToken
	}
	p.mu.Lock()
	mapping, found := p.mappings[token]
	if found && !now.Before(mapping.expiresAt) {
		delete(p.mappings, token)
	}
	p.mu.Unlock()
	if !found || !now.Before(mapping.expiresAt) {
		return ResolvedPage{}, ErrPageTokenExpired
	}
	if mapping.scope.PageBinding != binding {
		return ResolvedPage{}, ErrInvalidPageToken
	}
	return ResolvedPage{SnapshotID: mapping.scope.SnapshotID, Position: mapping.position}, nil
}

func validScope(scope PageScope) bool {
	return scope.SnapshotID != "" && validBinding(scope.PageBinding)
}

func validBinding(binding PageBinding) bool {
	return binding.AccountID != "" && binding.SessionID != "" && binding.ProjectionVersion != 0 &&
		(binding.Query == PromptHistoryQuery || binding.Query == SpecialistResultsQuery || binding.Query == ConversationQuery)
}
