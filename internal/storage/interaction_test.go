package storage

import (
	"testing"

	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

func TestInteractionPollingJournalRollbackAndCursorIndependence(t *testing.T) {
	s, _ := openTestStore(t)
	b := observationBinding(t, s)
	ctx := t.Context()
	bound := interaction.Bound{Binding: session.Binding{ID: b.SessionID, SubjectID: b.SubjectID, RunID: b.RunID}, Workspace: workspace.Attachment{CanonicalRoot: b.AbsoluteRoot, ProviderID: b.WorkspaceID}}
	before, err := s.Observation().Checkpoint(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.writer.ExecContext(ctx, `CREATE TRIGGER notification_fault BEFORE INSERT ON client_projection_notifications BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	stamp := observation.Stamp{Head: "2", Run: 2, Interaction: 2}
	if err = s.Observation().InteractionChanged(ctx, bound, stamp); err == nil {
		t.Fatal("missing failure")
	}
	batch, err := s.Observation().ReadDelivery(ctx, b.SubjectID, b.SessionID, 0)
	if err != nil || batch.Head != 0 || len(batch.Events) != 0 {
		t.Fatal("partial commit", batch, err)
	}
	if _, err = s.writer.ExecContext(ctx, "DROP TRIGGER notification_fault"); err != nil {
		t.Fatal(err)
	}
	if err = s.Observation().InteractionChanged(ctx, bound, stamp); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Observation().Checkpoint(ctx, b)
	if before != after {
		t.Fatal("poll moved event cursor")
	}
	if err = s.Observation().InteractionChanged(ctx, bound, observation.Stamp{}); err != nil {
		t.Fatal("failure invalidation", err)
	}
	afterFailure, _ := s.Observation().Checkpoint(ctx, b)
	if afterFailure != before {
		t.Fatal("failure notification moved cursor")
	}
	bound.Binding.SubjectID = "foreign"
	if err = s.Observation().InteractionChanged(ctx, bound, stamp); err != interaction.ErrAuthority {
		t.Fatal("foreign notification", err)
	}
}
