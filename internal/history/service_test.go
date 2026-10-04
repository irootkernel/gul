package history

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rootkernel/gul/internal/domain"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type repository struct {
	b   session.Binding
	err error
}

func (r *repository) Binding(_ context.Context, subject, id string) (session.Binding, error) {
	if r.err != nil {
		return session.Binding{}, r.err
	}
	if subject != r.b.SubjectID || id != r.b.ID {
		return session.Binding{}, ErrAuthority
	}
	return r.b, nil
}

type spaces struct {
	err     error
	foreign bool
}

func (s spaces) Revalidate(_ context.Context, subject, id string) (workspace.Attachment, error) {
	if s.err != nil {
		return workspace.Attachment{}, s.err
	}
	if s.foreign {
		subject = "other-subject"
	}
	return workspace.Attachment{SubjectID: subject, ID: id, ProviderID: "provider-workspace", CanonicalRoot: "/workspace"}, nil
}

type carriers struct{ err error }

func (c carriers) Resolve(context.Context, string, string) (session.Carrier, error) {
	if c.err != nil {
		return session.Carrier{}, c.err
	}
	return session.Carrier{ControllerID: "controller", AbsolutePath: "/private/carrier", Generation: 1}, nil
}

type readProvider struct {
	mu            sync.Mutex
	items         []SourceEntry
	timelineCalls int
	snapshotCalls int
	body          map[string][]byte
	state         string
}

func (p *readProvider) Snapshot(context.Context, Bound) (Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshotCalls++
	n := uint64(len(p.items) + 1)
	return Snapshot{Stamp: observation.Stamp{Head: observation.Cursor(strconv.FormatUint(n, 10)), Run: n}, State: p.state}, nil
}
func (p *readProvider) Timeline(ctx context.Context, _ Bound, after string, limit uint32) (Timeline, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Timeline{}, err
	}
	p.timelineCalls++
	n := uint64(len(p.items) + 1)
	v := Timeline{Head: strconv.FormatUint(n, 10), Stamp: observation.Stamp{Head: observation.Cursor(strconv.FormatUint(n, 10)), Run: n}}
	for _, e := range p.items {
		if after != "" && observation.Cursor(e.Cursor).Compare(observation.Cursor(after)) <= 0 {
			continue
		}
		if len(v.Items) == int(limit) {
			v.Next = v.Items[len(v.Items)-1].Cursor
			break
		}
		v.Items = append(v.Items, e)
	}
	return v, nil
}
func (p *readProvider) Results(context.Context, Bound, string, uint32) (Results, error) {
	return Results{Revision: 1, CapturedAt: time.Now()}, nil
}
func (p *readProvider) ReadArtifact(_ context.Context, _ Bound, a Artifact) ([]byte, error) {
	v, ok := p.body[a.ID]
	if !ok {
		return nil, ErrUnavailable
	}
	return append([]byte(nil), v...), nil
}
func (p *readProvider) add(kind Kind, text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := len(p.items) + 1
	status := map[Kind]string{Human: "accepted", Assistant: "final", InteractionOpened: "opened", InteractionResolved: "resolved", TurnTerminal: "interrupted"}[kind]
	e := SourceEntry{Cursor: strconv.Itoa(i + 1), ProviderID: fmt.Sprintf("private-%d", i), TurnID: "private-turn", Kind: kind, Status: status, At: time.Unix(int64(i), 0)}
	if kind == Human || kind == Assistant {
		e.Content.Inline = &text
	}
	p.items = append(p.items, e)
}
func fixture() (*Service, *readProvider, *repository) {
	r := &repository{b: session.Binding{SubjectID: "owner", ID: "session", RunID: "private-run", WorkspaceID: "workspace", ControllerBindingID: "binding"}}
	p := &readProvider{body: map[string][]byte{}, state: "idle"}
	return New(r, spaces{}, carriers{}, p), p, r
}

func TestHistoryAuthorityFailuresAndRetryableSources(t *testing.T) {
	for _, test := range []struct {
		name       string
		repository error
		workspace  spaces
		carrier    error
		want       error
	}{
		{name: "missing binding", repository: session.ErrNotFound, want: ErrAuthority},
		{name: "binding storage", repository: errors.New("private database unavailable"), want: ErrUnavailable},
		{name: "workspace transport", workspace: spaces{err: workspace.ErrProviderUnavailable}, want: ErrUnavailable},
		{name: "profile server", workspace: spaces{err: workspace.ErrProfileServerUnavailable}, want: ErrUnavailable},
		{name: "workspace storage", workspace: spaces{err: workspace.ErrPersistenceUnavailable}, want: ErrUnavailable},
		{name: "workspace identity", workspace: spaces{err: workspace.ErrReattachRequired}, want: ErrAuthority},
		{name: "foreign workspace", workspace: spaces{foreign: true}, want: ErrAuthority},
		{name: "unsupported workspace", workspace: spaces{err: workspace.ErrWorkspaceBlocked}, want: ErrBlocked},
		{name: "canceled binding", repository: context.Canceled, want: context.Canceled},
		{name: "workspace deadline", workspace: spaces{err: context.DeadlineExceeded}, want: context.DeadlineExceeded},
		{name: "unsafe carrier", carrier: session.ErrCarrierUnavailable, want: ErrAuthority},
		{name: "carrier deadline", carrier: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, p, r := fixture()
			p.add(Human, "retained original\r\n한글")
			r.err, s.Workspaces, s.Carriers = test.repository, test.workspace, carriers{err: test.carrier}
			if _, err := s.ListHistory(t.Context(), "owner", "session", "", 10); !errors.Is(err, test.want) {
				t.Fatal("wrong failure classification", err)
			}
			if p.snapshotCalls != 0 || p.timelineCalls != 0 || len(s.slots) != 0 {
				t.Fatal("failed authority dispatched provider reads or retained capacity")
			}
			r.err, s.Workspaces, s.Carriers = nil, spaces{}, carriers{}
			history, err := s.ListHistory(t.Context(), "owner", "session", "", 10)
			if err != nil || len(history.Items) != 1 || history.Items[0].Ordinal != 1 || p.snapshotCalls == 0 {
				t.Fatal("restored source did not recover", history, err)
			}
		})
	}
}
func TestHistoryFixedScopeIdentityEmptyPageAndOriginal(t *testing.T) {
	s, p, _ := fixture()
	original := "같은 입력\r\nline two\n"
	p.add(Human, original)
	for range 8 {
		p.add(TurnTerminal, "")
	}
	p.add(Human, original)
	first, err := s.ListHistory(t.Context(), "owner", "session", "", 1)
	if err != nil || len(first.Items) != 1 || first.Complete || first.Items[0].Ordinal != 1 {
		t.Fatal(first, err)
	}
	p.add(Human, "appended")
	before := p.timelineCalls
	empty, err := s.ListHistory(t.Context(), "owner", "session", first.Next, 1)
	if err != nil || len(empty.Items) != 0 || empty.Complete || empty.Next == "" || p.timelineCalls-before != 4 {
		t.Fatal("empty continuable", empty, err, p.timelineCalls-before)
	}
	var found []Entry
	next := empty.Next
	for next != "" {
		v, err := s.ListHistory(t.Context(), "owner", "session", next, 1)
		if err != nil {
			t.Fatal(err)
		}
		if v.SnapshotID != first.SnapshotID {
			t.Fatal("scope changed")
		}
		found = append(found, v.Items...)
		next = v.Next
	}
	if len(found) != 1 || found[0].Ordinal != 2 || found[0].PromptID == first.Items[0].PromptID {
		t.Fatal("identity or append scope", found)
	}
	for _, e := range []Entry{first.Items[0], found[0]} {
		v, err := s.GetOriginal(t.Context(), "owner", "session", e.PromptID, true)
		if err != nil || v.Inline == nil || *v.Inline != original {
			t.Fatal(v, err)
		}
	}
	fresh, err := s.ListHistory(t.Context(), "owner", "session", "", 100)
	if err != nil || len(fresh.Items) != 3 || fresh.Items[0].ID != first.Items[0].ID {
		t.Fatal(fresh, err)
	}
	all, err := s.ListConversation(t.Context(), "owner", "session", "", 100)
	if err != nil || len(all.Items) != 11 || all.Items[0].ID != first.Items[0].ID || all.RunState != "idle" {
		t.Fatal(all, err)
	}
}
func TestRestartTokensScopeAuthorizationAndConcurrentReconstruction(t *testing.T) {
	s, p, r := fixture()
	p.add(Human, "same")
	p.add(Human, "same")
	first, err := s.ListHistory(t.Context(), "owner", "session", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ListConversation(t.Context(), "owner", "session", first.Next, 1); !errors.Is(err, domain.ErrInvalidPageToken) {
		t.Fatal("wrong query", err)
	}
	if _, err = s.ListHistory(t.Context(), "other", "session", first.Next, 1); !errors.Is(err, ErrAuthority) {
		t.Fatal(err)
	}
	r.b.RunID = "rebound"
	if _, err = s.ListHistory(t.Context(), "owner", "session", first.Next, 1); !errors.Is(err, domain.ErrInvalidPageToken) {
		t.Fatal("rebound source", err)
	}
	r.b.RunID = "private-run"
	rebuilt := New(r, spaces{}, carriers{}, p)
	if _, err = rebuilt.ListHistory(t.Context(), "owner", "session", first.Next, 1); !errors.Is(err, domain.ErrPageTokenExpired) {
		t.Fatal("restart token", err)
	}
	if _, err = rebuilt.GetOriginal(t.Context(), "owner", "session", first.Items[0].PromptID, true); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing mapping reassigned", err)
	}
	p.state = "closed"
	var group sync.WaitGroup
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			v, err := rebuilt.ListHistory(t.Context(), "owner", "session", "", 100)
			if err != nil || len(v.Items) != 2 || v.Items[0].ID != first.Items[0].ID || v.Items[0].Ordinal != 1 || v.Items[1].Ordinal != 2 {
				t.Errorf("rebuild %+v %v", v, err)
			}
		}()
	}
	group.Wait()
	v, err := rebuilt.GetOriginal(t.Context(), "owner", "session", first.Items[0].PromptID, true)
	if err != nil || v.Inline == nil || *v.Inline != "same" {
		t.Fatal(v, err)
	}
	if _, err = rebuilt.ListHistory(t.Context(), "owner", "session", "bad", 1); !errors.Is(err, domain.ErrInvalidPageToken) {
		t.Fatal(err)
	}
	if _, err = rebuilt.ListHistory(t.Context(), "owner", "session", "", 101); !errors.Is(err, domain.ErrInvalidPageSize) {
		t.Fatal(err)
	}
}
func TestLargeOriginalUTF8BoundaryAndArtifactPresentation(t *testing.T) {
	s, p, _ := fixture()
	body := strings.Repeat("한글\r\n", 40000)
	p.add(Human, body)
	v, err := s.ListHistory(t.Context(), "owner", "session", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	e := v.Items[0]
	if !e.Truncated || len(e.Preview) > 1024 || !utf8.ValidString(e.Preview) || !strings.HasPrefix(body, e.Preview) {
		t.Fatal(e)
	}
	original, err := s.GetOriginal(t.Context(), "owner", "session", e.PromptID, true)
	if err != nil || original.Inline != nil || original.ArtifactRef == "" {
		t.Fatal(original, err)
	}
	meta, err := s.GetMetadata(t.Context(), "owner", "session", original.ArtifactRef)
	if err != nil || meta.Length != uint64(len(body)) || meta.SHA256 != digest([]byte(body)) {
		t.Fatal(meta, err)
	}
	var got []byte
	for offset := 0; offset < len(body); {
		length := min(MaximumChunkBytes, len(body)-offset)
		c, err := s.ReadChunk(t.Context(), "owner", "session", original.ArtifactRef, uint64(offset), uint32(length))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, c.Data...)
		offset += length
	}
	if string(got) != body {
		t.Fatal("original changed")
	}
	changed := "corrupt"
	p.items[0].Content.Inline = &changed
	if _, err := s.ListHistory(t.Context(), "owner", "session", "", 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("changed content reassigned to stable identity", err)
	}
	if _, err = s.ReadChunk(t.Context(), "owner", "session", original.ArtifactRef, 0, 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("changed source", err)
	}
	if _, err = s.ReadChunk(t.Context(), "owner", "session", "/etc/passwd", 0, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("path-shaped reference", err)
	}
}
func TestPageMetadataLimitAndCanceledRead(t *testing.T) {
	s, p, _ := fixture()
	for range 100 {
		p.add(Human, strings.Repeat("<", 1024))
	}
	if _, err := s.ListHistory(t.Context(), "owner", "session", "", 100); !errors.Is(err, ErrLimit) {
		t.Fatal("encoded bound", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.ListHistory(ctx, "owner", "session", "", 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type staleTimeline struct{ *readProvider }

func (p staleTimeline) Snapshot(context.Context, Bound) (Snapshot, error) {
	return Snapshot{Stamp: observation.Stamp{Head: "100", Run: 100}, State: "idle"}, nil
}
func TestTimelineBehindAuthorizedRunCannotClaimFreshHistory(t *testing.T) {
	s, p, _ := fixture()
	p.add(Human, "retained")
	s.Provider = staleTimeline{p}
	if _, err := s.ListHistory(t.Context(), "owner", "session", "", 1); !errors.Is(err, ErrUnavailable) {
		t.Fatal("stale timeline admitted", err)
	}
}

type resultProvider struct {
	*readProvider
	values Results
}

func (p *resultProvider) Results(context.Context, Bound, string, uint32) (Results, error) {
	return p.values, nil
}
func TestPublishedResultIdentityCannotBeReassigned(t *testing.T) {
	s, p, _ := fixture()
	results := &resultProvider{readProvider: p, values: Results{Head: 2, Revision: 2, CapturedAt: time.Unix(1, 0), Items: []SourceResult{{ID: "result", TaskID: "task", SpecialistID: "child", Role: "reviewer", Order: 1, At: time.Unix(1, 0), Artifact: Artifact{ID: "opaque", Length: 1, SHA256: digest([]byte("x"))}}}}}
	s.Provider = results
	if _, err := s.ListResults(t.Context(), "owner", "session", "", 1); err != nil {
		t.Fatal(err)
	}
	results.values.Items[0].TaskID = "different-task"
	if _, err := s.ListResults(t.Context(), "owner", "session", "", 1); !errors.Is(err, ErrBlocked) {
		t.Fatal("result identity reassigned", err)
	}
}

func TestTimelineImmutableMetadataCannotBeReassigned(t *testing.T) {
	for name, change := range map[string]func(*SourceEntry){
		"interaction-id":    func(e *SourceEntry) { e.InteractionID = "other" },
		"interaction-kind":  func(e *SourceEntry) { e.InteractionKind = "other" },
		"interaction-state": func(e *SourceEntry) { e.InteractionStatus = "resolved" },
		"title":             func(e *SourceEntry) { e.Title = "other" },
		"image":             func(e *SourceEntry) { e.Images[0].SHA256 = digest([]byte("other")) },
		"artifact":          func(e *SourceEntry) { e.Content.Artifact.ID = "other" },
		"content-form":      func(e *SourceEntry) { e.Content.Artifact = nil; body := "body"; e.Content.Inline = &body },
	} {
		t.Run(name, func(t *testing.T) {
			s, p, _ := fixture()
			p.add(Human, "body")
			e := &p.items[0]
			if strings.HasPrefix(name, "interaction") || name == "title" {
				e.Kind = InteractionOpened
				e.Status = "opened"
				e.Content = Content{}
				e.InteractionID = "original"
				e.InteractionKind = "user_input"
				e.InteractionStatus = "pending"
				e.Title = "Approval"
			} else if name == "image" {
				e.Images = []Image{{Ordinal: 1, MediaType: "image/png", SHA256: digest([]byte("image"))}}
			} else {
				e.Content = Content{Artifact: &Artifact{ID: "original", MediaType: "text/plain", Length: 4, SHA256: digest([]byte("body"))}}
				p.body["original"] = []byte("body")
				p.body["other"] = []byte("body")
			}
			first, err := s.ListConversation(t.Context(), "owner", "session", "", 10)
			if err != nil {
				t.Fatal(err)
			}
			original := *e
			original.Images = append([]Image(nil), e.Images...)
			if e.Content.Artifact != nil {
				a := *e.Content.Artifact
				original.Content.Artifact = &a
			}
			change(e)
			if _, err = s.ListConversation(t.Context(), "owner", "session", "", 10); !errors.Is(err, ErrBlocked) {
				t.Fatal("source reassigned", err)
			}
			// A rejected reconstruction leaves the first mapping intact.
			*e = original
			again, err := s.ListConversation(t.Context(), "owner", "session", "", 10)
			if err != nil || again.Items[0].ID != first.Items[0].ID || again.Items[0].InteractionRef != first.Items[0].InteractionRef {
				t.Fatal(again, err)
			}
			if e.Kind == Human {
				if _, err := s.GetOriginal(t.Context(), "owner", "session", first.Items[0].ID, false); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
