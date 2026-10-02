package sessionclose_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/sessionclose"
)

type refreshCache struct {
	fresh    map[observation.Refresh]bool
	stamp    observation.Stamp
	beginErr error
}

func (c *refreshCache) BeginRefresh(_ context.Context, _ action.Bound, s observation.Stamp) (observation.Cursor, error) {
	if c.beginErr != nil {
		return "", c.beginErr
	}
	c.fresh = map[observation.Refresh]bool{observation.Run: true}
	c.stamp = s
	return "2", nil
}
func (c *refreshCache) CompleteRefresh(_ context.Context, _ action.Bound, kind observation.Refresh, s observation.Stamp) error {
	if !s.Valid() || !s.Covers(c.stamp) {
		return sessionclose.ErrUnavailable
	}
	c.fresh[kind] = true
	return nil
}

type refreshReads struct {
	t                   *testing.T
	cache               *refreshCache
	fail                string
	stamp               observation.Stamp
	final               *history.Content
	timelineArtifacts   []history.Artifact
	unavailable         bool
	pages, artifacts    int
	alwaysMore, advance bool
	nextOverride        string
	pending             []interaction.Summary
	cardReads           int
}

func (r *refreshReads) check(kind string) error {
	if r.cache.fresh == nil || !r.cache.fresh[observation.Run] {
		r.t.Fatal("provider read before atomic invalidation")
	}
	if kind == r.fail {
		return errors.New("injected read failure")
	}
	return nil
}
func (r *refreshReads) Snapshot(context.Context, history.Bound) (history.Snapshot, error) {
	return history.Snapshot{Stamp: r.stamp, Final: r.final, FinalUnavailable: r.unavailable}, r.check("run")
}
func (r *refreshReads) GetExecutionState(context.Context, string, string) (session.ExecutionState, error) {
	return session.ExecutionState{Freshness: "fresh", Snapshot: &session.Snapshot{RunRevision: r.stamp.Run}}, r.check("session")
}
func (r *refreshReads) ReadWriter(context.Context, action.Bound) (action.WriterProjection, error) {
	return action.WriterProjection{Stamp: r.stamp}, r.check("writer")
}
func (r *refreshReads) Pending(context.Context, interaction.Bound) (interaction.PendingState, error) {
	return interaction.PendingState{Items: r.pending, Stamp: r.stamp}, r.check("interaction")
}
func (r *refreshReads) Card(_ context.Context, _ interaction.Bound, id string) (interaction.Card, error) {
	r.cardReads++
	for _, item := range r.pending {
		if item.ID == id {
			return interaction.Card{Summary: item}, r.check("card")
		}
	}
	return interaction.Card{}, sessionclose.ErrUnavailable
}
func (r *refreshReads) Timeline(_ context.Context, _ history.Bound, after string, _ uint32) (history.Timeline, error) {
	r.pages++
	if r.pages == 1 && after != "2" {
		r.t.Fatalf("lost verified timeline head: %q", after)
	}
	p := history.Timeline{Head: string(r.stamp.Head), Stamp: r.stamp}
	for _, artifact := range r.timelineArtifacts {
		p.Items = append(p.Items, history.SourceEntry{Content: history.Content{Artifact: &artifact}})
	}
	if r.alwaysMore {
		p.Next = fmt.Sprint(r.pages + 2)
	}
	if r.advance && r.pages == 2 {
		p.Stamp.Run++
		p.Stamp.Head = observation.Cursor(fmt.Sprint(p.Stamp.Run))
		p.Head = string(p.Stamp.Head)
	}
	if r.nextOverride != "" {
		p.Next = r.nextOverride
	}
	return p, r.check("timeline")
}
func (r *refreshReads) Results(context.Context, history.Bound, string, uint32) (history.Results, error) {
	panic("unexpected results read")
}
func (r *refreshReads) ReadArtifact(context.Context, history.Bound, history.Artifact) ([]byte, error) {
	r.artifacts++
	return []byte("secret final"), r.check("artifact")
}
func refreshFixture(t *testing.T) (*sessionclose.AggregateRefresher, *refreshReads, *refreshCache) {
	t.Helper()
	c := &refreshCache{}
	r := &refreshReads{t: t, cache: c, stamp: observation.Stamp{Head: "10", Run: 10, Writer: 3, Interaction: 4}, final: &history.Content{Artifact: &history.Artifact{ID: "final", Length: 12}}}
	return &sessionclose.AggregateRefresher{Cache: c, Sessions: r, Writer: r, Interactions: r, History: r}, r, c
}
func TestRecoveryRefreshReadsEachAggregateAndFinalArtifactIndependently(t *testing.T) {
	for _, fail := range []string{"", "run", "session", "writer", "interaction", "timeline", "artifact"} {
		t.Run(fail, func(t *testing.T) {
			ref, r, c := refreshFixture(t)
			r.fail = fail
			err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp})
			if (err != nil) != (fail != "") {
				t.Fatalf("failure %s returned %v", fail, err)
			}
			for name, kind := range map[string]observation.Refresh{"session": observation.Session, "writer": observation.Writer, "interaction": observation.Interaction, "timeline": observation.Timeline, "artifact": observation.Artifacts} {
				want := name != fail
				if (name == "artifact" || name == "timeline") && (fail == "run" || fail == "timeline" || fail == "artifact") {
					want = false
				}
				if c.fresh[kind] != want {
					t.Fatalf("%s freshness=%v want %v", name, c.fresh[kind], want)
				}
			}
			if !c.fresh[observation.Run] {
				t.Fatal("immediate mutation Run discarded")
			}
		})
	}
}
func TestRecoveryRefreshKeepsIncompleteTimelineStale(t *testing.T) {
	for _, advance := range []bool{false, true} {
		t.Run(fmt.Sprint(advance), func(t *testing.T) {
			ref, r, c := refreshFixture(t)
			r.alwaysMore = true
			r.advance = advance
			if err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp}); err == nil {
				t.Fatal("accepted incomplete timeline")
			}
			wantPages := history.MaximumProviderPages
			if advance {
				wantPages = 2
			}
			if r.pages != wantPages || c.fresh[observation.Timeline] || c.fresh[observation.Artifacts] || r.artifacts != 0 {
				t.Fatalf("pages=%d fresh=%v artifacts=%d", r.pages, c.fresh, r.artifacts)
			}
		})
	}
}

func TestRecoveryRefreshKeepsInteractionStaleUntilEveryCardSucceeds(t *testing.T) {
	ref, r, c := refreshFixture(t)
	r.pending = []interaction.Summary{{ID: "approval", Kind: interaction.CommandApproval, Status: interaction.Pending}}
	r.fail = "card"
	for attempt := 0; attempt < 3; attempt++ {
		if err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp}); err == nil || c.fresh[observation.Interaction] {
			t.Fatal("failed card made Interaction fresh", err, c.fresh)
		}
	}
	r.fail = ""
	if err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp}); err != nil || !c.fresh[observation.Interaction] || r.cardReads != 4 {
		t.Fatal("card recovery did not complete Interaction", err, c.fresh, r.cardReads)
	}
}
func TestRecoveryRefreshUnavailableFinalBlocksArtifactCompletion(t *testing.T) {
	ref, r, c := refreshFixture(t)
	r.unavailable = true
	r.final = nil
	if err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp}); !errors.Is(err, history.ErrUnavailable) || c.fresh[observation.Artifacts] {
		t.Fatalf("fresh=%v err=%v", c.fresh, err)
	}
}
func TestRecoveryRefreshDoesNotReadBeforeInvalidationCommits(t *testing.T) {
	ref, r, c := refreshFixture(t)
	c.beginErr = errors.New("transaction failure")
	if err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp}); err != c.beginErr || r.pages != 0 || r.artifacts != 0 {
		t.Fatal(err)
	}
}

func TestRecoveryRefreshBoundsIndividualAndCumulativeArtifactBytes(t *testing.T) {
	for _, cumulative := range []bool{false, true} {
		t.Run(fmt.Sprint(cumulative), func(t *testing.T) {
			ref, r, c := refreshFixture(t)
			wantReads := 0
			r.final.Artifact.Length = history.MaximumArtifactBytes + 1
			if cumulative {
				r.final.Artifact.Length = history.MaximumArtifactBytes/2 + 1
				r.timelineArtifacts = []history.Artifact{{ID: "other", Length: history.MaximumArtifactBytes/2 + 1}}
				wantReads = 1
			}
			if err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp}); !errors.Is(err, history.ErrLimit) {
				t.Fatal("accepted excessive artifact bytes", err)
			}
			if r.artifacts != wantReads || c.fresh[observation.Artifacts] || c.fresh[observation.Timeline] {
				t.Fatalf("reads=%d freshness=%v", r.artifacts, c.fresh)
			}
		})
	}
}

func TestRecoveryRefreshRejectsNonadvancingCursorAndConflictingArtifact(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			ref, r, c := refreshFixture(t)
			if conflict {
				r.timelineArtifacts = []history.Artifact{{ID: "final", Length: 13}}
			} else {
				r.nextOverride = "2"
			}
			if err := ref.Refresh(t.Context(), action.Bound{}, action.RunFacts{Stamp: r.stamp}); !errors.Is(err, sessionclose.ErrUnavailable) {
				t.Fatal(err)
			}
			if c.fresh[observation.Timeline] || c.fresh[observation.Artifacts] || r.artifacts != 0 {
				t.Fatal(c.fresh, r.artifacts)
			}
		})
	}
}
