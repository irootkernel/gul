package sessionclose

import (
	"context"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
)

type RefreshCache interface {
	// BeginRefresh atomically invalidates every dependent aggregate and accepts
	// only the immediate Run stamp. It returns the prior verified timeline head.
	BeginRefresh(context.Context, action.Bound, observation.Stamp) (observation.Cursor, error)
	CompleteRefresh(context.Context, action.Bound, observation.Refresh, observation.Stamp) error
}
type WriterReader interface {
	ReadWriter(context.Context, action.Bound) (action.WriterProjection, error)
}
type PendingReader interface {
	Pending(context.Context, interaction.Bound) (interaction.PendingState, error)
}
type SessionReader interface {
	GetExecutionState(context.Context, string, string) (session.ExecutionState, error)
}

// AggregateRefresher keeps each independent projection stale until its checked
// read succeeds. No response from RecoverRun or ReconcileRun supplies the other
// aggregates. The existing checked providers own wire validation and artifact
// digest/length verification; this coordinator owns ordering and bounded work.
type AggregateRefresher struct {
	Cache        RefreshCache
	Sessions     SessionReader
	Writer       WriterReader
	Interactions PendingReader
	History      history.Provider
}

func (r *AggregateRefresher) Refresh(ctx context.Context, b action.Bound, run action.RunFacts) error {
	if r == nil || r.Cache == nil || r.Sessions == nil || r.Writer == nil || r.Interactions == nil || r.History == nil || !run.Stamp.Valid() {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	after, err := r.Cache.BeginRefresh(ctx, b, run.Stamp)
	if err != nil {
		return err
	}
	hb := history.Bound{Binding: b.Binding, Workspace: b.Workspace, Carrier: b.Carrier, Floor: run.Stamp}
	var failures []error
	record := func(err error) {
		if err != nil {
			failures = append(failures, err)
		}
	}
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	snapshot, snapshotErr := r.History.Snapshot(readCtx, hb)
	readCancel()
	stamp := run.Stamp
	if snapshotErr == nil && (!snapshot.Stamp.Valid() || !snapshot.Stamp.Covers(run.Stamp)) {
		snapshotErr = ErrUnavailable
	}
	if snapshotErr == nil {
		stamp = snapshot.Stamp
		record(r.Cache.CompleteRefresh(ctx, b, observation.Run, stamp))
	}
	record(snapshotErr)

	readCtx, readCancel = context.WithTimeout(ctx, 5*time.Second)
	state, sessionErr := r.Sessions.GetExecutionState(readCtx, b.Binding.SubjectID, b.Binding.ID)
	readCancel()
	if sessionErr == nil && (state.Freshness != "fresh" || state.Snapshot == nil || state.Snapshot.RunRevision != stamp.Run) {
		sessionErr = ErrUnavailable
	}
	if sessionErr == nil {
		sessionErr = r.Cache.CompleteRefresh(ctx, b, observation.Session, stamp)
	}
	record(sessionErr)

	readCtx, readCancel = context.WithTimeout(ctx, 5*time.Second)
	writer, writerErr := r.Writer.ReadWriter(readCtx, b)
	readCancel()
	if writerErr == nil {
		writerErr = r.Cache.CompleteRefresh(ctx, b, observation.Writer, writer.Stamp)
	}
	record(writerErr)
	readCtx, readCancel = context.WithTimeout(ctx, 5*time.Second)
	pending, pendingErr := r.Interactions.Pending(readCtx, interaction.Bound{Binding: b.Binding, Workspace: b.Workspace, Carrier: b.Carrier})
	readCancel()
	if pendingErr == nil {
		pendingErr = r.Cache.CompleteRefresh(ctx, b, observation.Interaction, pending.Stamp)
	}
	record(pendingErr)

	// Timeline and new artifact references are bounded together. Partial pages
	// never advance the cached head or declare artifact checks complete.
	artifacts := map[string]history.Artifact{}
	if snapshotErr == nil && snapshot.Final != nil && snapshot.Final.Artifact != nil {
		artifacts[snapshot.Final.Artifact.ID] = *snapshot.Final.Artifact
	}
	var timelineStamp observation.Stamp
	complete := false
	for page := 0; page < history.MaximumProviderPages; page++ {
		readCtx, readCancel = context.WithTimeout(ctx, 5*time.Second)
		timeline, timelineErr := r.History.Timeline(readCtx, hb, string(after), 100)
		readCancel()
		if timelineErr == nil && (!timeline.Stamp.Valid() || !timeline.Stamp.Covers(stamp) || timeline.Head != string(timeline.Stamp.Head) || timelineStamp.Valid() && timeline.Stamp != timelineStamp) {
			timelineErr = ErrUnavailable
		}
		if timelineErr != nil {
			record(timelineErr)
			break
		}
		timelineStamp = timeline.Stamp
		for _, item := range timeline.Items {
			if item.Content.Artifact != nil {
				a := *item.Content.Artifact
				if previous, found := artifacts[a.ID]; found && previous != a {
					return ErrUnavailable
				}
				artifacts[a.ID] = a
			}
		}
		if timeline.Next == "" {
			complete = true
			break
		}
		next := observation.Cursor(timeline.Next)
		if !next.Valid() || next.Compare(after) <= 0 {
			record(ErrUnavailable)
			break
		}
		after = next
	}
	if !complete {
		record(history.ErrLimit)
	}
	if complete {
		var bytes uint64
		artifactsOK := snapshotErr == nil && !snapshot.FinalUnavailable
		if snapshot.FinalUnavailable {
			record(history.ErrUnavailable)
		}
		for _, a := range artifacts {
			if a.Length > history.MaximumArtifactBytes || bytes > history.MaximumArtifactBytes-a.Length {
				record(history.ErrLimit)
				artifactsOK = false
				break
			}
			bytes += a.Length
			readCtx, readCancel = context.WithTimeout(ctx, 5*time.Second)
			data, artifactErr := r.History.ReadArtifact(readCtx, hb, a)
			readCancel()
			clear(data)
			if artifactErr != nil {
				record(artifactErr)
				artifactsOK = false
			}
		}
		if artifactsOK {
			// Keep the prior head until every discovered reference is verified;
			// otherwise a retry would skip an unread artifact's timeline entry.
			if err := r.Cache.CompleteRefresh(ctx, b, observation.Artifacts, timelineStamp); err != nil {
				record(err)
			} else {
				record(r.Cache.CompleteRefresh(ctx, b, observation.Timeline, timelineStamp))
			}
		}
	}
	return errors.Join(failures...)
}

var _ Refresher = (*AggregateRefresher)(nil)
