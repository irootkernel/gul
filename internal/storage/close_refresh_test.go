package storage

import (
	"errors"
	"testing"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/sessionclose"
	"github.com/rootkernel/gul/internal/workspace"
)

func closeRefreshFixture(t *testing.T) (*Store, action.Bound) {
	t.Helper()
	s, _ := openTestStore(t)
	b := observationBinding(t, s)
	binding, err := s.Presentation().Binding(t.Context(), b.SubjectID, b.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	return s, action.Bound{Binding: binding, Workspace: workspace.Attachment{ProviderID: b.WorkspaceID, CanonicalRoot: b.AbsoluteRoot}}
}
func refreshMask(t *testing.T, s *Store, b action.Bound) observation.Refresh {
	t.Helper()
	var mask observation.Refresh
	if err := s.reader.QueryRowContext(t.Context(), `SELECT refresh_mask FROM observation_refreshes WHERE subject_id=? AND session_id=?`, b.Binding.SubjectID, b.Binding.ID).Scan(&mask); err != nil {
		t.Fatal(err)
	}
	return mask
}
func TestCloseRefreshAtomicallyInvalidatesAndCompletesOnlyReadAggregate(t *testing.T) {
	s, b := closeRefreshFixture(t)
	ctx := t.Context()
	stamp := observation.Stamp{Head: "10", Run: 10, Writer: 3, Interaction: 4}
	if err := s.Cache().PutTimelineHead(ctx, b.Binding.SubjectID, b.Binding.ID, UpstreamCursor{Value: "2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.ExecContext(ctx, `CREATE TRIGGER refresh_fault BEFORE INSERT ON observation_refreshes BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseRefresh().BeginRefresh(ctx, b, stamp); err == nil {
		t.Fatal("fault ignored")
	}
	var count int
	if err := s.reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_projection_cache`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial invalidation %d %v", count, err)
	}
	if _, err := s.writer.ExecContext(ctx, `DROP TRIGGER refresh_fault`); err != nil {
		t.Fatal(err)
	}
	after, err := s.CloseRefresh().BeginRefresh(ctx, b, stamp)
	if err != nil || after != "2" {
		t.Fatal(after, err)
	}
	for _, kind := range []string{"run", "writer", "interaction"} {
		entry, err := s.Cache().ProjectionStamp(ctx, b.Binding.SubjectID, b.Binding.ID, kind)
		want := "stale"
		if kind == "run" {
			want = "fresh"
		}
		if err != nil || entry.Freshness != want {
			t.Fatal(kind, entry, err)
		}
	}
	if err := s.CloseRefresh().CompleteRefresh(ctx, b, observation.Writer, stamp); err != nil {
		t.Fatal(err)
	}
	mask := refreshMask(t, s, b)
	if mask&observation.Writer != 0 || mask&(observation.Interaction|observation.Timeline|observation.Artifacts|observation.Session) != (observation.Interaction|observation.Timeline|observation.Artifacts|observation.Session) {
		t.Fatalf("mask=%v", mask)
	}
	for _, kind := range []observation.Refresh{observation.Interaction, observation.Timeline, observation.Artifacts, observation.Session} {
		if err := s.CloseRefresh().CompleteRefresh(ctx, b, kind, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if refreshMask(t, s, b) != 0 {
		t.Fatal("completed reads remain pending")
	}
	head, err := s.Cache().TimelineHead(ctx, b.Binding.SubjectID, b.Binding.ID)
	if err != nil || head.Value != "10" {
		t.Fatal(head, err)
	}
}
func TestCloseRefreshCannotClearNewerInvalidationOrForeignBinding(t *testing.T) {
	s, b := closeRefreshFixture(t)
	ctx := t.Context()
	old := observation.Stamp{Head: "10", Run: 10, Writer: 3, Interaction: 4}
	newer := observation.Stamp{Head: "11", Run: 11, Writer: 4, Interaction: 4}
	if _, err := s.CloseRefresh().BeginRefresh(ctx, b, newer); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseRefresh().BeginRefresh(ctx, b, old); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []observation.Refresh{observation.Run, observation.Writer, observation.Interaction, observation.Timeline, observation.Artifacts, observation.Session} {
		if err := s.CloseRefresh().CompleteRefresh(ctx, b, kind, old); !errors.Is(err, sessionclose.ErrUnavailable) {
			t.Fatalf("cleared newer invalidation kind=%v err=%v", kind, err)
		}
	}
	entry, err := s.Cache().ProjectionStamp(ctx, b.Binding.SubjectID, b.Binding.ID, "run")
	if err != nil || entry.Freshness != "stale" || observationStamp(entry.InvalidationFloor) != newer {
		t.Fatal(entry, err)
	}
	foreign := b
	foreign.Binding.SubjectID = "foreign"
	if _, err := s.CloseRefresh().BeginRefresh(ctx, foreign, newer); !errors.Is(err, sessionclose.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := s.CloseRefresh().CompleteRefresh(ctx, foreign, observation.Writer, newer); !errors.Is(err, sessionclose.ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestCloseRefreshRetainsOwnerlessAndForeignOwnerWriterStamps(t *testing.T) {
	for _, writer := range []observation.Stamp{{Writer: 3}, {Head: "1", Run: 1, Writer: 3, Interaction: 1}} {
		s, b := closeRefreshFixture(t)
		stamp := observation.Stamp{Head: "10", Run: 10, Writer: 3, Interaction: 4}
		if _, err := s.CloseRefresh().BeginRefresh(t.Context(), b, stamp); err != nil {
			t.Fatal(err)
		}
		if err := s.CloseRefresh().CompleteRefresh(t.Context(), b, observation.Writer, writer); err != nil {
			t.Fatal(err)
		}
		entry, err := s.Cache().ProjectionStamp(t.Context(), b.Binding.SubjectID, b.Binding.ID, "writer")
		if err != nil || entry.Freshness != "fresh" || observationStamp(entry.Stamp) != writer {
			t.Fatalf("invented writer stamp: %+v %v", entry, err)
		}
		writer.Writer--
		if err := s.CloseRefresh().CompleteRefresh(t.Context(), b, observation.Writer, writer); !errors.Is(err, sessionclose.ErrUnavailable) {
			t.Fatal("accepted old writer revision", err)
		}
	}
}
