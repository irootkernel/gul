package scenario

import (
	"context"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"google.golang.org/protobuf/proto"
)

func TestEventProjectionPerSubscriber(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	minimal, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run,
		Projection: publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL, ProjectionVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer minimal.Close()
	operational, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run,
		Projection: publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL, ProjectionVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer operational.Close()
	operationalDefault, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run,
		Projection: publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL})
	if err != nil {
		t.Fatal(err)
	}
	defer operationalDefault.Close()
	defaults, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	defer defaults.Close()
	request := &publicv1.SubmitTurnRequest{Run: f.run, Controller: f.controller,
		IdempotencyKey: "stamped-turn", Message: "test", ExpectedStateRevision: f.revision(t)}
	accepted, err := f.h.SubmitTurn(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	first, err := minimal.Receive()
	if err != nil {
		t.Fatal(err)
	}
	event := first.GetDurableEvent()
	if event.GetProjection() != publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL || event.GetProjectionVersion() != 1 {
		t.Fatalf("minimal event = %v", event)
	}
	if event.GetStamp() == nil || !proto.Equal(event.GetStamp(), accepted.GetRun().GetStamp()) || event.GetStamp().GetCapturedHeadCursor() != event.GetCursor() {
		t.Fatalf("event stamp does not match committed Run: event=%v run=%v", event, accepted.GetRun())
	}
	original := copyOf(event)
	// Consumers own their returned DTOs; changing one must not affect another.
	event.Stamp.RunStateRevision = 999
	other, err := operational.Receive()
	if err != nil {
		t.Fatal(err)
	}
	wantOperational := copyOf(original)
	wantOperational.Projection = publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL
	if !proto.Equal(other.GetDurableEvent(), wantOperational) {
		t.Fatalf("operational subscriber changed event identity or stamp: %v", other)
	}
	mixedDefaultEvent, err := operationalDefault.Receive()
	if err != nil || !proto.Equal(mixedDefaultEvent.GetDurableEvent(), wantOperational) {
		t.Fatalf("explicit profile with default version = %v, %v", mixedDefaultEvent, err)
	}
	defaultEvent, err := defaults.Receive()
	if err != nil || !proto.Equal(defaultEvent.GetDurableEvent(), original) {
		t.Fatalf("default projection = %v, %v", defaultEvent, err)
	}
	if err := f.h.ReplayLast(f.run.GetRunId()); err != nil {
		t.Fatal(err)
	}
	if err := f.h.CompleteTurn(f.run.GetRunId(), publicv1.TurnStatus_TURN_STATUS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	replay, err := minimal.Receive()
	wantReplay := copyOf(original)
	wantReplay.Replay = true
	if err != nil || !proto.Equal(replay.GetDurableEvent(), wantReplay) {
		t.Fatalf("replay did not retain original commit stamp: %v, %v", replay, err)
	}
	terminal, err := minimal.Receive()
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil || !proto.Equal(terminal.GetDurableEvent().GetStamp(), current.GetRun().GetStamp()) || current.GetRun().GetStateRevision() != accepted.GetRun().GetStateRevision()+1 {
		t.Fatalf("terminal event stamp = %v, snapshot=%v, err=%v", terminal, current, err)
	}
	resumed, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run, AfterCursor: original.GetCursor(),
		Projection: publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL, ProjectionVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	resumedEvent, err := resumed.Receive()
	wantTerminal := copyOf(terminal.GetDurableEvent())
	wantTerminal.Projection = publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL
	if err != nil || !proto.Equal(resumedEvent.GetDurableEvent(), wantTerminal) {
		t.Fatalf("resumed projection = %v, %v", resumedEvent, err)
	}
	_, err = f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	acceptedReplay, err := f.h.SubmitTurn(ctx, request)
	if err != nil || !proto.Equal(acceptedReplay, accepted) {
		t.Fatalf("writer mutation changed accepted replay: %v, %v", acceptedReplay, err)
	}
}

func TestEventProjectionRejectsUnsupportedRequests(t *testing.T) {
	f := newFixture(t)
	for _, request := range []*publicv1.WatchRunEventsRequest{
		{Run: f.run, Projection: publicv1.ProjectionProfile(999), ProjectionVersion: 1},
		{Run: f.run, Projection: publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL, ProjectionVersion: 2},
	} {
		stream, err := f.h.WatchRunEvents(context.Background(), request)
		if stream != nil {
			stream.Close()
			t.Fatal("unsupported projection opened a stream")
		}
		requireProviderCode(t, err, "INVALID_REQUEST")
	}
}

func TestWriterStampUsesStateRevision(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	initial, err := f.h.GetWorkspaceWriterStatus(ctx, &publicv1.GetWorkspaceWriterStatusRequest{Workspace: f.workspace})
	if err != nil {
		t.Fatal(err)
	}
	wantInitial := copyOf(initial.GetWriter())
	wantStamp := &publicv1.ProjectionStamp{WriterStateRevision: initial.GetWriter().GetStateRevision()}
	if !proto.Equal(initial.GetWriter().GetStamp(), wantStamp) {
		t.Fatalf("initial workspace writer stamp = %v, want %v", initial.GetWriter().GetStamp(), wantStamp)
	}
	previous := initial.GetWriter()
	for cycle := 0; cycle < 2; cycle++ {
		acquired, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
		if err != nil {
			t.Fatal(err)
		}
		assertWriterStamp(t, f, previous, acquired)
		released, err := f.h.ReleaseWriter(ctx, &publicv1.ReleaseWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
		if err != nil {
			t.Fatal(err)
		}
		assertWriterStamp(t, f, acquired, released)
		previous = released
	}
	if !proto.Equal(initial.GetWriter(), wantInitial) {
		t.Fatal("later mutations changed the initial returned stamp")
	}
}

func assertWriterStamp(t *testing.T, f fixture, previous, current *publicv1.WriterState) {
	t.Helper()
	if current.GetStateRevision() <= previous.GetStateRevision() || current.GetStamp().GetWriterStateRevision() != current.GetStateRevision() {
		t.Fatalf("writer stamp did not advance with writer state: previous=%v current=%v", previous, current)
	}
	if current.GetStateRevision() == current.GetWriterGeneration() {
		t.Fatal("fixture must keep state revision distinct from writer generation")
	}
	run, err := f.h.GetRun(context.Background(), &publicv1.GetRunRequest{Run: f.run})
	if err != nil || run.GetRun().GetStamp().GetWriterStateRevision() != current.GetStateRevision() {
		t.Fatalf("writer revision in Run stamp differs: run=%v writer=%v err=%v", run, current, err)
	}
	status, err := f.h.GetWorkspaceWriterStatus(context.Background(), &publicv1.GetWorkspaceWriterStatusRequest{Workspace: f.workspace})
	if err != nil || !proto.Equal(status.GetWriter(), current) {
		t.Fatalf("writer reread differs from committed response: %v, %v", status, err)
	}
}

func TestSharedWriterRevisionInFreshRunSnapshots(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other, err := f.h.StartRun(ctx, &publicv1.StartRunRequest{Workspace: f.workspace, Controller: f.controller,
		IdempotencyKey: "other-stamp", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	otherRef := &publicv1.RunRef{Workspace: f.workspace, RunId: other.GetRun().GetRunId()}
	writer, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: otherRef})
	if err != nil || fresh.GetRun().GetStamp().GetWriterStateRevision() != writer.GetStateRevision() || fresh.GetRun().GetStateRevision() != other.GetRun().GetStateRevision() {
		t.Fatalf("other Run snapshot = %v, %v", fresh, err)
	}
	if other.GetRun().GetStamp().GetWriterStateRevision() == writer.GetStateRevision() {
		t.Fatal("old StartRun response changed after writer mutation")
	}
	listed, err := f.h.ListRuns(ctx, &publicv1.ListRunsRequest{Workspace: f.workspace})
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range listed.GetItems() {
		if run.GetStamp().GetWriterStateRevision() != writer.GetStateRevision() {
			t.Fatalf("ListRuns returned an old writer revision: %v", run)
		}
	}
	pending, err := f.h.ListPendingInteractions(ctx, &publicv1.ListPendingInteractionsRequest{Run: otherRef})
	if err != nil || !proto.Equal(pending.GetStamp(), fresh.GetRun().GetStamp()) {
		t.Fatalf("interaction snapshot = %v, %v", pending, err)
	}
	timeline, err := f.h.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: otherRef, Controller: f.controller, TimelineVersion: 1})
	if err != nil || !proto.Equal(timeline.GetStamp(), fresh.GetRun().GetStamp()) {
		t.Fatalf("timeline snapshot stamp = %v, %v", timeline, err)
	}
	// The workspace-only writer query has no Run selector. It must not borrow
	// an arbitrary Run's cursor, Run revision, or Interaction revision.
	if writer.GetStamp().GetCapturedHeadCursor() != "" || writer.GetStamp().GetRunStateRevision() != 0 || writer.GetStamp().GetInteractionStateRevision() != 0 {
		t.Fatalf("workspace writer acquired unrelated Run context: %v", writer)
	}
}

func TestInteractionEventsRetainCommitStamps(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := f.h.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := f.h.OpenInteraction(f.run.GetRunId(), &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{
		InteractionId: "stamped-question", Kind: publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT}}); err != nil {
		t.Fatal(err)
	}
	opened, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := f.h.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.h.ResolveInteraction(ctx, &publicv1.ResolveInteractionRequest{Run: f.run, Controller: f.controller,
		InteractionId: "stamped-question", IdempotencyKey: "resolve-stamp", ResponseJson: []byte(`{"answer":"ok"}`)})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := f.h.GetRun(ctx, &publicv1.GetRunRequest{Run: f.run})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GetRun().GetStamp().GetWriterStateRevision() != writer.GetStateRevision() || opened.GetRun().GetStamp().GetWriterStateRevision() >= writer.GetStateRevision() {
		t.Fatalf("interaction snapshots missed writer transition: opened=%v resolved=%v writer=%v", opened, resolved, writer)
	}
	released, err := f.h.ReleaseWriter(ctx, &publicv1.ReleaseWriterRequest{Run: f.run, Controller: f.controller, ExpectedStateRevision: f.revision(t)})
	if err != nil || released.GetStateRevision() <= writer.GetStateRevision() {
		t.Fatalf("writer release did not advance revision: %v, %v", released, err)
	}
	for i, want := range []*publicv1.RunProjection{opened.GetRun(), resolved.GetRun()} {
		envelope, err := stream.Receive()
		stamp := envelope.GetDurableEvent().GetStamp()
		if err != nil || !proto.Equal(stamp, want.GetStamp()) || stamp.GetInteractionStateRevision() != uint64(i+1) {
			t.Fatalf("interaction event %d = %v, %v; want stamp %v", i, envelope, err, want.GetStamp())
		}
	}
	if err := f.h.ReplayLast(f.run.GetRunId()); err != nil {
		t.Fatal(err)
	}
	replay, err := stream.Receive()
	if err != nil || !replay.GetDurableEvent().GetReplay() || !proto.Equal(replay.GetDurableEvent().GetStamp(), resolved.GetRun().GetStamp()) {
		t.Fatalf("writer mutation changed replayed interaction stamp: %v, %v", replay, err)
	}
}
