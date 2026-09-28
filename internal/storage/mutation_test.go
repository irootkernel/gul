package storage

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/session"
)

func TestTokenlessWriterAttemptSurvivesRestartAndBlocksConflict(t *testing.T) {
	s, filename := openTestStore(t)
	if err := s.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	b := action.Bound{Binding: session.Binding{SubjectID: "owner", RunID: "run", ControllerBindingID: "binding"}, Carrier: session.Carrier{ControllerID: "controller"}}
	id, err := s.WriterAttempts().BeginWriter(t.Context(), b, true, 4)
	if err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "pending" || a.ReplayAvailable || a.ReconciliationRoute != "run_writer" {
		t.Fatalf("pretransmission attempt = %+v, %v", a, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(t.Context(), filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("restart attempt = %+v, %v", a, err)
	}
	if _, err := s.WriterAttempts().BeginWriter(t.Context(), b, false, 5); !errors.Is(err, action.ErrOutcomeUnknown) {
		t.Fatalf("conflicting writer mutation = %v", err)
	}
	if err := s.Attempts().ResolveMutation(t.Context(), id, "writer-snapshot-confirmed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriterAttempts().BeginWriter(t.Context(), b, false, 5); err != nil {
		t.Fatalf("resolved writer blocked: %v", err)
	}
}

func TestProtectedInteractionAttemptStoresOnlyMetadata(t *testing.T) {
	s, filename := openTestStore(t)
	if err := s.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	b := interaction.Bound{Binding: session.Binding{SubjectID: "owner", RunID: "run", ControllerBindingID: "binding"}, Carrier: session.Carrier{ControllerID: "controller"}}
	id, err := s.InteractionAttempts().BeginInteraction(t.Context(), b, "interaction", "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filename, filename + "-wal"} {
		data, err := os.ReadFile(path)
		if err == nil && (strings.Contains(string(data), "protected-canary") || strings.Contains(string(data), "stable-key")) {
			t.Fatalf("protected material in %s", path)
		}
	}
	if err := s.InteractionAttempts().FinishInteraction(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InteractionAttempts().BeginInteraction(t.Context(), b, "interaction", "another-key"); !errors.Is(err, interaction.ErrAttemptConflict) {
		t.Fatalf("unknown protected response allowed duplicate: %v", err)
	}
	if err := s.Attempts().ResolveMutation(t.Context(), id, "resolved-by-fresh-card"); err != nil {
		t.Fatal(err)
	}
}

func TestLaterAuthoritativeInteractionCardSettlesUnknownAttempt(t *testing.T) {
	s, _ := openTestStore(t)
	if err := s.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	b := interaction.Bound{Binding: session.Binding{SubjectID: "owner", RunID: "run", ControllerBindingID: "binding"}, Carrier: session.Carrier{ControllerID: "controller"}}
	id, err := s.InteractionAttempts().BeginInteraction(t.Context(), b, "interaction", "stable-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InteractionAttempts().FinishInteraction(t.Context(), id, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.InteractionAttempts().ReconcileInteraction(t.Context(), b, "interaction", interaction.Pending); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "outcome_unknown" {
		t.Fatalf("pending card cleared uncertainty = %+v, %v", a, err)
	}
	if err := s.InteractionAttempts().ReconcileInteraction(t.Context(), b, "interaction", interaction.Resolved); err != nil {
		t.Fatal(err)
	}
	if a, err := s.Attempts().Mutation(t.Context(), id); err != nil || a.State != "resolved" || a.OutcomeRef != "resolved" {
		t.Fatalf("resolved card did not settle attempt = %+v, %v", a, err)
	}
}
