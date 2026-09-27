// Package contractprovider translates generated event envelopes into bounded,
// payload-free Gul invalidations. It never implements a production transport.
package contractprovider

import (
	"context"
	"errors"
	"io"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/observation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Provider struct{ Port port.ObservationPort }

func (p Provider) Watch(ctx context.Context, b observation.Binding, after observation.Cursor) (observation.Stream, error) {
	if p.Port == nil || !b.Valid() || !after.Valid() {
		return nil, observation.ErrInvalid
	}
	stream, err := p.Port.WatchRunEvents(ctx, &publicv1.WatchRunEventsRequest{Run: &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: b.AbsoluteRoot, ExpectedWorkspaceId: b.WorkspaceID}, RunId: b.RunID}, AfterCursor: string(after), Projection: publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL, ProjectionVersion: 1})
	if err != nil {
		return nil, mapError(err)
	}
	if stream == nil {
		return nil, observation.ErrInvalid
	}
	return &eventStream{stream: stream, binding: b}, nil
}

type eventStream struct {
	stream  port.EventStream
	binding observation.Binding
}

func (s *eventStream) Close() error { return s.stream.Close() }
func (s *eventStream) Receive() (observation.Envelope, error) {
	message, err := s.stream.Receive()
	if err != nil {
		return observation.Envelope{}, mapError(err)
	}
	if event := message.GetDurableEvent(); event != nil && event.GetProjection() != publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL {
		return observation.Envelope{}, observation.ErrInvalid
	}
	return Decode(message, s.binding)
}
func mapError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	mapped := port.MapProviderError(err)
	if mapped.Code == "SLOW_CONSUMER" {
		return observation.ErrSlowConsumer
	}
	return observation.ErrRefresh
}

// Decode covers every descriptor variant even though v0.1 requests minimal.
// Broader event payloads are discarded rather than forwarded to browsers.
func Decode(message *publicv1.RunEventEnvelope, b observation.Binding) (observation.Envelope, error) {
	bad := func() (observation.Envelope, error) { return observation.Envelope{}, observation.ErrInvalid }
	if message == nil || !b.Valid() || proto.Size(message) > 8*1024*1024 || !known(message.ProtoReflect()) {
		return bad()
	}
	switch item := message.Item.(type) {
	case *publicv1.RunEventEnvelope_DurableEvent:
		e := item.DurableEvent
		if e == nil || e.RunId != b.RunID || e.WorkspaceId != b.WorkspaceID || e.EventId == "" || len(e.EventId) > 256 || e.ServerKey == "" || e.ServerEpoch == 0 || (e.Projection != publicv1.ProjectionProfile_PROJECTION_PROFILE_MINIMAL && e.Projection != publicv1.ProjectionProfile_PROJECTION_PROFILE_OPERATIONAL) || e.ProjectionVersion != 1 || e.OccurredAt == nil || !e.OccurredAt.IsValid() || e.Stamp == nil {
			return bad()
		}
		stamp := observation.Stamp{Head: observation.Cursor(e.Stamp.CapturedHeadCursor), Run: e.Stamp.RunStateRevision, Writer: e.Stamp.WriterStateRevision, Interaction: e.Stamp.InteractionStateRevision}
		if !stamp.Valid() || string(stamp.Head) != e.Cursor {
			return bad()
		}
		result := &observation.Event{Cursor: observation.Cursor(e.Cursor), ID: e.EventId, RunID: e.RunId, WorkspaceID: e.WorkspaceId, Stamp: stamp, At: e.OccurredAt.AsTime(), Replay: e.Replay}
		// All Run lifecycle changes conservatively refresh Writer as an additive
		// authority check; no lifecycle hint can turn an action on.
		switch v := e.Event.(type) {
		case *publicv1.DurableRunEvent_RunStateChanged:
			if v.RunStateChanged == nil || v.RunStateChanged.Current == 0 {
				return bad()
			}
			result.Kind = "RunStateChanged"
			result.Refresh = observation.Run | observation.Writer
		case *publicv1.DurableRunEvent_TurnStateChanged:
			if v.TurnStateChanged == nil || v.TurnStateChanged.Current == 0 || e.GetTurnId() == "" {
				return bad()
			}
			result.Kind = "TurnStateChanged"
			result.Refresh = observation.Run | observation.Timeline
		case *publicv1.DurableRunEvent_FinalResponseAvailable:
			if v.FinalResponseAvailable == nil || e.GetTurnId() == "" || v.FinalResponseAvailable.GetResponse().GetValue() == nil {
				return bad()
			}
			result.Kind = "FinalResponseAvailable"
			result.Refresh = observation.Run | observation.Timeline | observation.Artifacts
		case *publicv1.DurableRunEvent_InteractionOpened:
			if v.InteractionOpened == nil || v.InteractionOpened.InteractionId == "" || v.InteractionOpened.Kind == 0 {
				return bad()
			}
			result.Kind = "InteractionOpened"
			result.Refresh = observation.Run | observation.Interaction
			result.Priority = true
		case *publicv1.DurableRunEvent_InteractionResolved:
			if v.InteractionResolved == nil || v.InteractionResolved.InteractionId == "" || v.InteractionResolved.Outcome == 0 {
				return bad()
			}
			result.Kind = "InteractionResolved"
			result.Refresh = observation.Run | observation.Interaction | observation.Timeline
		case *publicv1.DurableRunEvent_WriterStateChanged:
			if v.WriterStateChanged == nil || v.WriterStateChanged.Current == 0 {
				return bad()
			}
			result.Kind = "WriterStateChanged"
			result.Refresh = observation.Run | observation.Writer
		case *publicv1.DurableRunEvent_RecoveryRequired:
			if v.RecoveryRequired == nil {
				return bad()
			}
			result.Kind = "RecoveryRequired"
			result.Refresh = observation.Run | observation.Writer
			result.Priority = true
		case *publicv1.DurableRunEvent_RuntimeErrorOccurred:
			if v.RuntimeErrorOccurred == nil || v.RuntimeErrorOccurred.ErrorCode == "" {
				return bad()
			}
			result.Kind = "RuntimeErrorOccurred"
			result.Refresh = observation.Run
		case *publicv1.DurableRunEvent_GenerationChanged:
			if v.GenerationChanged == nil || v.GenerationChanged.RunGeneration == 0 || v.GenerationChanged.ServerEpoch == 0 {
				return bad()
			}
			result.Kind = "GenerationChanged"
			result.Refresh = observation.Run | observation.Writer | observation.Interaction | observation.Profile
		case *publicv1.DurableRunEvent_WorkspaceChanges:
			if v.WorkspaceChanges == nil {
				return bad()
			}
			result.Kind = "WorkspaceChanges"
			result.Refresh = observation.Timeline | observation.Files
		case *publicv1.DurableRunEvent_CommandStarted:
			if v.CommandStarted == nil {
				return bad()
			}
			result.Kind = "CommandStarted"
			result.Refresh = observation.Timeline
		case *publicv1.DurableRunEvent_CommandCompleted:
			if v.CommandCompleted == nil {
				return bad()
			}
			result.Kind = "CommandCompleted"
			result.Refresh = observation.Timeline
		case *publicv1.DurableRunEvent_UsageReported:
			if v.UsageReported == nil {
				return bad()
			}
			result.Kind = "UsageReported"
			result.Refresh = observation.Timeline
		case *publicv1.DurableRunEvent_DiagnosticReported:
			if v.DiagnosticReported == nil {
				return bad()
			}
			result.Kind = "DiagnosticReported"
			result.Refresh = observation.Timeline
		case *publicv1.DurableRunEvent_ReasoningSuppressed:
			if v.ReasoningSuppressed == nil {
				return bad()
			}
			result.Kind = "ReasoningSuppressed"
			result.Refresh = observation.Timeline
		default:
			return bad()
		}
		return observation.Envelope{Event: result}, nil
	case *publicv1.RunEventEnvelope_Heartbeat:
		h := item.Heartbeat
		if h == nil || h.RunId != b.RunID || !observation.Cursor(h.DurableHeadCursor).Valid() || h.Lifecycle == 0 || h.EmittedAt == nil || !h.EmittedAt.IsValid() {
			return bad()
		}
		return observation.Envelope{RunID: h.RunId, Head: observation.Cursor(h.DurableHeadCursor), Heartbeat: h.EmittedAt.AsTime()}, nil
	case *publicv1.RunEventEnvelope_StreamEnd:
		end := item.StreamEnd
		if end == nil || end.RunId != b.RunID || !observation.Cursor(end.DurableHeadCursor).Valid() {
			return bad()
		}
		reason := ""
		switch end.Reason {
		case publicv1.StreamEndReason_STREAM_END_REASON_RUN_TERMINAL:
			reason = "terminal"
		case publicv1.StreamEndReason_STREAM_END_REASON_SERVER_SHUTDOWN:
			reason = "shutdown"
		default:
			return bad()
		}
		return observation.Envelope{RunID: end.RunId, Head: observation.Cursor(end.DurableHeadCursor), End: reason}, nil
	default:
		return bad()
	}
}

func known(m protoreflect.Message) bool {
	if !m.IsValid() || len(m.GetUnknown()) != 0 {
		return false
	}
	ok := true
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		check := func(v protoreflect.Value) bool {
			if f.Kind() == protoreflect.EnumKind {
				return f.Enum().Values().ByNumber(v.Enum()) != nil
			}
			if f.Kind() == protoreflect.MessageKind {
				return known(v.Message())
			}
			return true
		}
		if f.IsList() {
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				if !check(list.Get(i)) {
					ok = false
					break
				}
			}
		} else {
			ok = check(v)
		}
		return ok
	})
	return ok
}
