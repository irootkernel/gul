package scenario

import (
	"context"
	"strconv"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"google.golang.org/protobuf/proto"
)

// SPEC-006 and Projection revision authority at the pinned producer revision
// require one canonical decimal Run-ledger domain, including filtered gaps.
func TestLedgerCursorBoundaries(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	initial, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	head := initial.GetRun().GetStamp().GetCapturedHeadCursor()
	if head != strconv.FormatUint(initial.GetRun().GetStateRevision(), 10) {
		t.Fatalf("initial head=%q", head)
	}
	for _, after := range []string{"", "0", head} {
		stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: after})
		if err != nil {
			t.Fatalf("initial cursor %q: %v", after, err)
		}
		stream.Close()
		page, err := f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1, AfterCursor: after})
		if err != nil || len(page.GetItems()) != 0 || page.GetCapturedHeadCursor() != head {
			t.Fatalf("initial timeline=%v %v", page, err)
		}
	}
	for _, after := range []string{"01", "+1", "-1", " 1", "1.0", "event-1", "18446744073709551616", strconv.FormatUint(initial.GetRun().GetStateRevision()+1, 10)} {
		stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: after})
		if stream != nil {
			stream.Close()
			t.Fatalf("invalid stream cursor %q admitted", after)
		}
		requireProviderDetailCode(t, err, "EVENT_CURSOR_INVALID")
		_, err = f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1, AfterCursor: after})
		requireProviderDetailCode(t, err, "EVENT_CURSOR_INVALID")
	}
	// A mutation without a projected event creates a legal ledger gap.
	writer, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	gap := writer.GetStamp().GetCapturedHeadCursor()
	stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: gap})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	accepted, err := f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, IdempotencyKey: "ledger-input", Message: "original", ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	event, err := stream.Receive()
	if err != nil {
		t.Fatal(err)
	}
	want := strconv.FormatUint(accepted.GetRun().GetStateRevision(), 10)
	if event.GetDurableEvent().GetCursor() != want || accepted.GetAcceptedTurn().GetCursor() != want || !proto.Equal(event.GetDurableEvent().GetStamp(), accepted.GetRun().GetStamp()) {
		t.Fatalf("accepted boundaries diverge: %v %v", event, accepted)
	}
	page, err := f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: f.run, Controller: f.controller, TimelineVersion: 1, AfterCursor: gap})
	if err != nil || len(page.GetItems()) != 1 || page.GetItems()[0].GetCursor() != want || page.GetCapturedHeadCursor() != want {
		t.Fatalf("gap timeline=%v %v", page, err)
	}
	if err := f.h.ReplayLast(f.run.GetRunId()); err != nil {
		t.Fatal(err)
	}
	replay, err := stream.Receive()
	expected := copyOf(event.GetDurableEvent())
	expected.Replay = true
	if err != nil || !proto.Equal(replay.GetDurableEvent(), expected) {
		t.Fatalf("replay=%v %v", replay, err)
	}
	resumed, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: want})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if err := f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	next, err := resumed.Receive()
	if err != nil || next.GetDurableEvent().GetCursor() == want {
		t.Fatalf("exclusive resume replayed old boundary: %v %v", next, err)
	}
}

func TestWriterScopeAndFreshLedgerHeads(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	status, err := f.h.GetWorkspaceWriterStatus(ctx, &publicv1.GetWorkspaceWriterStatusRequest{Workspace: f.workspace})
	if err != nil {
		t.Fatal(err)
	}
	checkOwnerlessWriter(t, status.GetWriter())
	acquired, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	original := copyOf(acquired)
	if acquired.GetExecutionLane() != publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED || acquired.GetEffectiveAccess() != publicv1.EffectiveAccess_EFFECTIVE_ACCESS_WRITE || acquired.GetPolicyVerification() != publicv1.PolicyVerification_POLICY_VERIFICATION_VERIFIED || acquired.GetRequestedAssurance() != publicv1.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA || acquired.GetAchievedAssurance() != publicv1.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA {
		t.Fatalf("owned policy=%v", acquired)
	}
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "ledger-question", Kind: publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT}}); err != nil {
		t.Fatal(err)
	}
	run, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	status, err = f.h.GetWorkspaceWriterStatus(ctx, &publicv1.GetWorkspaceWriterStatusRequest{Workspace: f.workspace})
	if err != nil || !proto.Equal(status.GetWriter().GetStamp(), run.GetRun().GetStamp()) || status.GetWriter().GetStateRevision() != acquired.GetStateRevision() {
		t.Fatalf("fresh owned writer=%v run=%v err=%v", status, run, err)
	}
	if run.GetRun().GetStamp().GetInteractionStateRevision() != run.GetRun().GetStateRevision() {
		t.Fatal("Interaction revision is not the changing ledger record")
	}
	if !proto.Equal(acquired, original) {
		t.Fatal("fresh reads mutated prior response")
	}
	released, err := f.h.ReleaseWriter(ctx, &publicv1.ReleaseWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	checkOwnerlessWriter(t, released)
	fresh, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || fresh.GetRun().GetStamp().GetCapturedHeadCursor() != strconv.FormatUint(fresh.GetRun().GetStateRevision(), 10) || fresh.GetRun().GetStamp().GetInteractionStateRevision() != run.GetRun().GetStamp().GetInteractionStateRevision() {
		t.Fatalf("writer-only change corrupted Run boundary: %v %v", fresh, err)
	}
}

func checkOwnerlessWriter(t *testing.T, w *publicv1.WriterState) {
	t.Helper()
	want := &publicv1.ProjectionStamp{WriterStateRevision: w.GetStateRevision()}
	if w.GetOwnerRunId() != "" || !proto.Equal(w.GetStamp(), want) || w.GetExecutionLane() != publicv1.ExecutionLane_EXECUTION_LANE_UNSPECIFIED || w.GetRequestedAssurance() != publicv1.AssuranceLevel_ASSURANCE_LEVEL_UNSPECIFIED || w.GetAchievedAssurance() != publicv1.AssuranceLevel_ASSURANCE_LEVEL_UNSPECIFIED || w.GetEffectiveAccess() != publicv1.EffectiveAccess_EFFECTIVE_ACCESS_UNKNOWN || w.GetPolicyVerification() != publicv1.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED || w.GetHandoffEligible() {
		t.Fatalf("ownerless projection borrowed owner facts: %v", w)
	}
}

func TestAdvisoryHeadsDoNotAdvanceLedger(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	before, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: before.GetRun().GetStamp().GetCapturedHeadCursor()})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for _, envelope := range []*publicv1.RunEventEnvelope{
		{Item: &publicv1.RunEventEnvelope_Heartbeat{Heartbeat: &publicv1.RunEventHeartbeat{}}},
		{Item: &publicv1.RunEventEnvelope_StreamEnd{StreamEnd: &publicv1.RunEventStreamEnd{Reason: publicv1.StreamEndReason_STREAM_END_REASON_SERVER_SHUTDOWN}}},
	} {
		if err := f.h.AppendEnvelope(f.run.GetRunId(), envelope); err != nil {
			t.Fatal(err)
		}
	}
	beat, err := stream.Receive()
	if err != nil || beat.GetHeartbeat().GetRunId() != f.run.GetRunId() || beat.GetHeartbeat().GetDurableHeadCursor() != before.GetRun().GetStamp().GetCapturedHeadCursor() || beat.GetHeartbeat().GetEmittedAt() == nil {
		t.Fatalf("heartbeat=%v %v", beat, err)
	}
	end, err := stream.Receive()
	if err != nil || end.GetStreamEnd().GetDurableHeadCursor() != before.GetRun().GetStamp().GetCapturedHeadCursor() {
		t.Fatalf("end=%v %v", end, err)
	}
	after, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || !proto.Equal(before, after) {
		t.Fatalf("advisory envelope advanced ledger: %v %v", after, err)
	}
	resumed, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: before.GetRun().GetStamp().GetCapturedHeadCursor()})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if err := f.h.AppendEvent(f.run.GetRunId(), &publicv1.DurableRunEvent{Event: &publicv1.DurableRunEvent_DiagnosticReported{DiagnosticReported: &publicv1.DiagnosticReported{SafeMessage: "new"}}}); err != nil {
		t.Fatal(err)
	}
	fresh, err := resumed.Receive()
	if err != nil || fresh.GetDurableEvent() == nil {
		t.Fatalf("new subscription replayed a historical advisory: %v %v", fresh, err)
	}
}

func TestInteractionLedgerRevisionAcrossLifecycle(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)}); err != nil {
		t.Fatal(err)
	}
	pause := func() error {
		_, err := f.h.PauseRun(ctx, &publicv1.PauseRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
		return err
	}
	resume := func() error {
		_, err := f.h.ResumeRun(ctx, &publicv1.ResumeRunRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
		return err
	}
	for _, change := range []func() error{pause, resume} {
		if err := change(); err != nil {
			t.Fatal(err)
		}
		current, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
		if err != nil || current.GetRun().GetStamp().GetInteractionStateRevision() != 0 {
			t.Fatalf("lifecycle before any Interaction created a revision: %v %v", current, err)
		}
	}
	stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: strconv.FormatUint(f.revision(t), 10)})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "lifecycle-question", Kind: publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT}}); err != nil {
		t.Fatal(err)
	}
	event, err := stream.Receive()
	if err != nil {
		t.Fatal(err)
	}
	historicalEvent := copyOf(event)
	pending, err := f.h.ListPendingInteractions(ctx, &publicv1.ListPendingInteractionsRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	historicalPending := copyOf(pending)
	steps := []struct {
		name   string
		change func() error
	}{
		{"pause", pause},
		{"resume", resume},
		{"resolve", func() error {
			_, err := f.h.ResolveInteraction(ctx, &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller, InteractionId: "lifecycle-question", IdempotencyKey: "lifecycle-answer", ResponseJson: []byte(`{"answer":"yes"}`)})
			return err
		}},
		{"submit after resolved Interaction", func() error {
			_, err := f.h.SubmitTurn(ctx, &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t), IdempotencyKey: "lifecycle-turn", Message: "continue"})
			return err
		}},
		{"complete turn", func() error { return f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_COMPLETED) }},
		{"close lifecycle", func() error { return f.h.SetRunLifecycle(f.run.GetRunId(), publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED) }},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			before := f.revision(t)
			if err := step.change(); err != nil {
				t.Fatal(err)
			}
			current, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
			if err != nil || current.GetRun().GetStateRevision() <= before || current.GetRun().GetStamp().GetInteractionStateRevision() != current.GetRun().GetStateRevision() {
				t.Fatalf("lifecycle did not advance Interaction boundary: %v %v", current, err)
			}
			freshPending, err := f.h.ListPendingInteractions(ctx, &publicv1.ListPendingInteractionsRequest{Run: f.run})
			if err != nil || !proto.Equal(freshPending.GetStamp(), current.GetRun().GetStamp()) {
				t.Fatalf("Interaction observation disagrees: %v %v", freshPending, err)
			}
			writer, err := f.h.GetWorkspaceWriterStatus(ctx, &publicv1.GetWorkspaceWriterStatusRequest{Workspace: f.workspace})
			if err != nil || !proto.Equal(writer.GetWriter().GetStamp(), current.GetRun().GetStamp()) {
				t.Fatalf("owned Writer observation disagrees: %v %v", writer, err)
			}
			if !proto.Equal(event, historicalEvent) || !proto.Equal(pending, historicalPending) {
				t.Fatal("lifecycle mutation changed historical observations")
			}
		})
	}
}
