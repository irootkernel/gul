package history

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rootkernel/gul/internal/domain"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
)

type object struct {
	result   *SourceResult
	source   string
	before   string
	item     SourceEntry
	entry    Entry
	artifact *Artifact
	digest   string
	inline   bool
}
type Service struct {
	Repository Repository
	Workspaces session.Workspace
	Carriers   session.CarrierResolver
	Provider   Provider
	tokens     *domain.PageTokens
	slots      chan struct{}
	mu         sync.Mutex
	objects    map[string]object
	order      []string
}

func New(repo Repository, workspaces session.Workspace, carriers session.CarrierResolver, provider Provider) *Service {
	return &Service{Repository: repo, Workspaces: workspaces, Carriers: carriers, Provider: provider, tokens: domain.NewPageTokens(), slots: make(chan struct{}, 2), objects: map[string]object{}}
}
func (s *Service) begin(ctx context.Context, subject, id string) (context.Context, context.CancelFunc, Bound, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if s == nil || s.Repository == nil || s.Workspaces == nil || s.Carriers == nil || s.Provider == nil || s.tokens == nil || subject == "" || id == "" || len(id) > 256 {
		cancel()
		return ctx, func() {}, Bound{}, ErrInvalid
	}
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		cancel()
		return ctx, func() {}, Bound{}, ctx.Err()
	}
	done := func() { <-s.slots; cancel() }
	b, err := s.Repository.Binding(ctx, subject, id)
	if err != nil || b.ID != id || b.SubjectID != subject || b.RunID == "" || b.ControllerBindingID == "" {
		done()
		return ctx, func() {}, Bound{}, ErrAuthority
	}
	w, err := s.Workspaces.Revalidate(ctx, subject, b.WorkspaceID)
	if err != nil || w.SubjectID != subject || w.ID != b.WorkspaceID || w.ProviderID == "" || w.CanonicalRoot == "" {
		done()
		return ctx, func() {}, Bound{}, ErrAuthority
	}
	c, err := s.Carriers.Resolve(ctx, subject, b.ControllerBindingID)
	if err != nil || c.ControllerID == "" || c.AbsolutePath == "" || c.Generation == 0 {
		done()
		return ctx, func() {}, Bound{}, ErrAuthority
	}
	bound := Bound{Binding: b, Workspace: w, Carrier: c}
	snapshot, err := s.Provider.Snapshot(ctx, bound)
	if err != nil {
		done()
		return ctx, func() {}, Bound{}, err
	}
	bound.Floor = snapshot.Stamp
	return ctx, done, bound, nil
}
func identity(parts ...string) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func sourceID(b Bound) string {
	return identity(b.Binding.SubjectID, b.Binding.ID, b.Binding.RunID, b.Workspace.ProviderID, b.Workspace.CanonicalRoot, b.Carrier.ControllerID, strconv.FormatUint(b.Carrier.Generation, 10))
}

// Stable item identity excludes temporary Controller generation and content.
func itemKey(b Bound, kind, id string) string {
	return identity(b.Binding.SubjectID, b.Binding.ID, b.Workspace.ProviderID, b.Binding.RunID, kind, id)
}

// Timeline records are append-only; a stable cursor cannot change its source.
// Inline bodies are compared separately by digest so the cache retains no body.
func sameSourceEntry(a, b SourceEntry) bool {
	return a.Cursor == b.Cursor && a.ProviderID == b.ProviderID && a.TurnID == b.TurnID && a.InteractionID == b.InteractionID && a.Kind == b.Kind && a.Status == b.Status && a.InteractionKind == b.InteractionKind && a.InteractionStatus == b.InteractionStatus && a.Title == b.Title && a.At.Equal(b.At) && slices.Equal(a.Images, b.Images) && (a.Content.Artifact == nil) == (b.Content.Artifact == nil) && (a.Content.Artifact == nil || *a.Content.Artifact == *b.Content.Artifact)
}
func (s *Service) put(key string, o object) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.objects[key]; ok {
		if (old.result == nil) != (o.result == nil) || old.result != nil && *old.result != *o.result {
			return ErrBlocked
		}
		if !sameSourceEntry(old.item, o.item) || old.inline != o.inline || old.entry.Ordinal != o.entry.Ordinal || old.digest != o.digest || (old.artifact == nil) != (o.artifact == nil) || old.artifact != nil && *old.artifact != *o.artifact {
			return ErrBlocked
		}
	} else {
		if len(s.order) == MaximumObjects {
			delete(s.objects, s.order[0])
			s.order = s.order[1:]
		}
		s.order = append(s.order, key)
	}
	s.objects[key] = o
	return nil
}
func (s *Service) get(b Bound, ref, prefix string) (object, error) {
	if !strings.HasPrefix(ref, prefix) || len(ref) != len(prefix)+64 {
		return object{}, ErrInvalid
	}
	key := strings.TrimPrefix(ref, prefix)
	if _, err := hex.DecodeString(key); err != nil {
		return object{}, ErrInvalid
	}
	s.mu.Lock()
	o, ok := s.objects[key]
	s.mu.Unlock()
	if !ok {
		return object{}, ErrUnavailable
	}
	if o.source != sourceID(b) {
		return object{}, ErrAuthority
	}
	return o, nil
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func preview(data []byte) (string, bool) {
	n := min(len(data), domain.MaximumPreviewBytes)
	for n > 0 && !utf8.Valid(data[:n]) {
		n--
	}
	return string(data[:n]), n < len(data)
}
func snapshotID() (string, error) {
	var v [16]byte
	if _, err := rand.Read(v[:]); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(v[:]), nil
}
func binding(b Bound, q domain.QueryKind) domain.PageBinding {
	return domain.PageBinding{SourceIdentity: sourceID(b), AccountID: b.Binding.SubjectID, SessionID: b.Binding.ID, Query: q, ProjectionVersion: 1}
}
func (s *Service) position(b Bound, q domain.QueryKind, token string) (string, domain.PagePosition, error) {
	if token != "" {
		v, err := s.tokens.Resolve(binding(b, q), token, time.Now())
		return v.SnapshotID, v.Position, err
	}
	id, err := snapshotID()
	return id, domain.PagePosition{}, err
}
func (s *Service) next(b Bound, q domain.QueryKind, snapshot string, pos domain.PagePosition) (string, error) {
	now := time.Now()
	return s.tokens.Issue(domain.PageScope{PageBinding: binding(b, q), SnapshotID: snapshot}, pos, now, now.Add(15*time.Minute))
}
func boundedPage[T any](p Page[T]) error {
	data, err := json.Marshal(p)
	if err != nil {
		return ErrBlocked
	}
	if len(data) > domain.MaximumPageMetadataBytes {
		return ErrLimit
	}
	return nil
}
func (s *Service) content(ctx context.Context, b Bound, c Content) ([]byte, error) {
	if c.Inline != nil {
		if len(*c.Inline) > MaximumArtifactBytes || !utf8.ValidString(*c.Inline) {
			return nil, ErrLimit
		}
		return []byte(*c.Inline), nil
	}
	if c.Artifact != nil {
		return s.Provider.ReadArtifact(ctx, b, *c.Artifact)
	}
	return nil, nil
}
func (s *Service) ListHistory(ctx context.Context, subject, id, token string, size uint32) (Page[Entry], error) {
	return s.list(ctx, subject, id, token, size, domain.PromptHistoryQuery)
}
func (s *Service) ListConversation(ctx context.Context, subject, id, token string, size uint32) (Page[Entry], error) {
	return s.list(ctx, subject, id, token, size, domain.ConversationQuery)
}
func (s *Service) list(ctx context.Context, subject, id, token string, size uint32, q domain.QueryKind) (Page[Entry], error) {
	ctx, done, b, err := s.begin(ctx, subject, id)
	if err != nil {
		return Page[Entry]{}, err
	}
	defer done()
	limit, err := domain.BoundPageSize(size)
	if err != nil {
		return Page[Entry]{}, err
	}
	scope, pos, err := s.position(b, q, token)
	if err != nil {
		return Page[Entry]{}, err
	}
	out := Page[Entry]{SnapshotID: scope, Items: []Entry{}, ObservedAt: time.Now().UTC()}
	var latest observation.Stamp
	for reads := 0; reads < MaximumProviderPages && len(out.Items) < int(limit); reads++ {
		if err := ctx.Err(); err != nil {
			return Page[Entry]{}, err
		}
		page, err := s.Provider.Timeline(ctx, b, pos.ProviderCursor, limit-uint32(len(out.Items)))
		if err != nil {
			return Page[Entry]{}, err
		}
		if !page.Stamp.Covers(b.Floor) {
			return Page[Entry]{}, ErrUnavailable
		}
		if pos.CapturedHead == "" {
			pos.CapturedHead = page.Head
		}
		if !observation.Cursor(pos.CapturedHead).Valid() || observation.Cursor(page.Head).Compare(observation.Cursor(pos.CapturedHead)) < 0 {
			return Page[Entry]{}, ErrBlocked
		}
		latest = page.Stamp
		for _, item := range page.Items {
			if observation.Cursor(item.Cursor).Compare(observation.Cursor(pos.CapturedHead)) > 0 {
				out.Complete = true
				break
			}
			before := pos.ProviderCursor
			pos.ProviderCursor = item.Cursor
			if item.Kind == Human {
				if pos.ScannedCount == ^uint64(0) {
					return Page[Entry]{}, ErrLimit
				}
				pos.ScannedCount++
			}
			if q == domain.PromptHistoryQuery && item.Kind != Human {
				continue
			}
			key := itemKey(b, "timeline", item.Cursor)
			e := Entry{ID: "entry_" + key, TurnRef: "turn_" + itemKey(b, "turn", item.TurnID), Kind: item.Kind, Status: item.Status, At: item.At, Title: item.Title, InteractionKind: item.InteractionKind, InteractionStatus: item.InteractionStatus, Images: item.Images}
			if item.Kind == Human {
				e.PromptID = "prompt_" + key
				e.Ordinal = pos.ScannedCount
			}
			if item.InteractionID != "" {
				e.InteractionRef = "interaction_" + itemKey(b, "interaction", item.InteractionID)
			}
			data, err := s.content(ctx, b, item.Content)
			if err != nil {
				return Page[Entry]{}, err
			}
			e.HasOriginal = item.Content.Inline != nil || item.Content.Artifact != nil
			e.Preview, e.Truncated = preview(data)
			o := object{source: sourceID(b), before: before, item: item, entry: e, digest: digest(data), inline: item.Content.Inline != nil}
			o.item.Content.Inline = nil // Cache metadata only; originals are always reread.
			o.item.Images = slices.Clone(item.Images)
			o.entry.Images = slices.Clone(item.Images)
			if item.Content.Artifact != nil {
				artifact := *item.Content.Artifact
				o.item.Content.Artifact = &artifact
			}
			clear(data)
			if err := s.put(key, o); err != nil {
				return Page[Entry]{}, err
			}
			out.Items = append(out.Items, e)
		}
		if out.Complete || page.Next == "" || observation.Cursor(pos.ProviderCursor).Compare(observation.Cursor(pos.CapturedHead)) >= 0 {
			out.Complete = true
			break
		}
		if pos.ProviderCursor != page.Next {
			return Page[Entry]{}, ErrBlocked
		}
	}
	fresh, err := s.Provider.Snapshot(ctx, b)
	if err != nil {
		return Page[Entry]{}, err
	}
	if !fresh.Stamp.Covers(latest) {
		return Page[Entry]{}, ErrUnavailable
	}
	out.RunState = fresh.State
	if !out.Complete {
		out.Next, err = s.next(b, q, scope, pos)
		if err != nil {
			return Page[Entry]{}, err
		}
	}
	if err = boundedPage(out); err != nil {
		return Page[Entry]{}, err
	}
	return out, nil
}
func (s *Service) original(ctx context.Context, b Bound, o object) ([]byte, error) {
	page, err := s.Provider.Timeline(ctx, b, o.before, 1)
	if err != nil {
		return nil, err
	}
	if !page.Stamp.Covers(b.Floor) {
		return nil, ErrUnavailable
	}
	if len(page.Items) != 1 {
		return nil, ErrUnavailable
	}
	item := page.Items[0]
	if (item.Content.Inline != nil) != o.inline || (item.Content.Artifact == nil) != (o.item.Content.Artifact == nil) || item.Content.Artifact != nil && *item.Content.Artifact != *o.item.Content.Artifact {
		return nil, ErrBlocked
	}
	if !sameSourceEntry(item, o.item) {
		return nil, ErrBlocked
	}
	data, err := s.content(ctx, b, item.Content)
	if err != nil {
		return nil, err
	}
	if digest(data) != o.digest {
		clear(data)
		return nil, ErrBlocked
	}
	return data, nil
}
func (s *Service) GetOriginal(ctx context.Context, subject, id, ref string, prompt bool) (Original, error) {
	ctx, done, b, err := s.begin(ctx, subject, id)
	if err != nil {
		return Original{}, err
	}
	defer done()
	prefix := "entry_"
	if prompt {
		prefix = "prompt_"
	}
	o, err := s.get(b, ref, prefix)
	if err != nil {
		return Original{}, err
	}
	if !o.entry.HasOriginal || prompt && o.entry.Kind != Human {
		return Original{}, ErrInvalid
	}
	data, err := s.original(ctx, b, o)
	if err != nil {
		return Original{}, err
	}
	defer clear(data)
	out := Original{Entry: o.entry}
	if len(data) <= MaximumInlineBytes {
		v := string(data)
		out.Inline = &v
	} else {
		out.ArtifactRef = "artifact_" + strings.TrimPrefix(ref, prefix)
	}
	return out, nil
}
func (s *Service) artifactData(ctx context.Context, b Bound, ref string) ([]byte, Metadata, error) {
	o, err := s.get(b, ref, "artifact_")
	if err != nil {
		return nil, Metadata{}, err
	}
	var data []byte
	media := "text/plain; charset=utf-8"
	if o.artifact != nil {
		data, err = s.Provider.ReadArtifact(ctx, b, *o.artifact)
		media = o.artifact.MediaType
	} else if o.entry.HasOriginal {
		data, err = s.original(ctx, b, o)
		if o.item.Content.Artifact != nil {
			media = o.item.Content.Artifact.MediaType
		}
	} else {
		return nil, Metadata{}, ErrInvalid
	}
	if err != nil {
		return nil, Metadata{}, err
	}
	return data, Metadata{Ref: ref, MediaType: media, Length: uint64(len(data)), SHA256: digest(data)}, nil
}
func (s *Service) GetMetadata(ctx context.Context, subject, id, ref string) (Metadata, error) {
	ctx, done, b, err := s.begin(ctx, subject, id)
	if err != nil {
		return Metadata{}, err
	}
	defer done()
	data, m, err := s.artifactData(ctx, b, ref)
	clear(data)
	return m, err
}
func (s *Service) ReadChunk(ctx context.Context, subject, id, ref string, offset uint64, length uint32) (Chunk, error) {
	if length == 0 || length > MaximumChunkBytes {
		return Chunk{}, ErrInvalid
	}
	ctx, done, b, err := s.begin(ctx, subject, id)
	if err != nil {
		return Chunk{}, err
	}
	defer done()
	data, m, err := s.artifactData(ctx, b, ref)
	if err != nil {
		return Chunk{}, err
	}
	defer clear(data)
	if offset > uint64(len(data)) || uint64(length) > uint64(len(data))-offset {
		return Chunk{}, ErrInvalid
	}
	return Chunk{Data: append([]byte(nil), data[offset:offset+uint64(length)]...), Length: m.Length, SHA256: m.SHA256}, nil
}
func (s *Service) ListResults(ctx context.Context, subject, id, token string, size uint32) (Page[Result], error) {
	ctx, done, b, err := s.begin(ctx, subject, id)
	if err != nil {
		return Page[Result]{}, err
	}
	defer done()
	limit, err := domain.BoundPageSize(size)
	if err != nil {
		return Page[Result]{}, err
	}
	scope, pos, err := s.position(b, domain.SpecialistResultsQuery, token)
	if err != nil {
		return Page[Result]{}, err
	}
	page, err := s.Provider.Results(ctx, b, pos.ProviderCursor, limit)
	if err != nil {
		return Page[Result]{}, err
	}
	head := strconv.FormatUint(page.Head, 10)
	if pos.CapturedHead != "" && (pos.CapturedHead != head || pos.SourceRevision != page.Revision || !pos.CapturedAt.Equal(page.CapturedAt)) {
		return Page[Result]{}, ErrBlocked
	}
	out := Page[Result]{SnapshotID: scope, Items: []Result{}, ObservedAt: time.Now().UTC(), Complete: page.Next == ""}
	for _, item := range page.Items {
		if item.Order <= pos.ScannedCount || item.Order > page.Head {
			return Page[Result]{}, ErrBlocked
		}
		pos.ScannedCount = item.Order
		key := itemKey(b, "result", item.ID)
		artifact := item.Artifact
		if err := s.put(key, object{source: sourceID(b), artifact: &artifact, result: &item}); err != nil {
			return Page[Result]{}, err
		}
		out.Items = append(out.Items, Result{ID: "result_" + key, ViewID: "specialist_" + itemKey(b, "specialist", item.SpecialistID), Role: item.Role, Order: item.Order, At: item.At, Length: artifact.Length, SHA256: artifact.SHA256, ArtifactRef: "artifact_" + key})
	}
	if !out.Complete {
		if page.Next == pos.ProviderCursor {
			return Page[Result]{}, ErrBlocked
		}
		pos.ProviderCursor = page.Next
		pos.CapturedHead = head
		pos.SourceRevision = page.Revision
		pos.CapturedAt = page.CapturedAt
		out.Next, err = s.next(b, domain.SpecialistResultsQuery, scope, pos)
		if err != nil {
			return Page[Result]{}, err
		}
	}
	if err = boundedPage(out); err != nil {
		return Page[Result]{}, err
	}
	return out, nil
}
