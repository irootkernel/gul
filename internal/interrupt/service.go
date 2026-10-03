// Package interrupt coordinates a tokenless Primary Turn request. Dolgorae
// owns Turn lifecycle; Gul records attempts and observes, without replaying.
package interrupt

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
)

type Outcome uint8

const (
	Accepted Outcome = iota + 1
	Rejected
	Unknown
	StateObserved
)

type Attempt struct {
	ID, TurnID, Head, State, Outcome string
	Revision                         uint64
	DispatchFinished                 bool
}
type Repository interface {
	Find(context.Context, action.Bound, string) (Attempt, bool, error)
	Begin(context.Context, action.Bound, string, string, observation.Stamp) (Attempt, bool, error)
	Finish(context.Context, string, string) error
	Pending(context.Context, action.Bound) ([]Attempt, error)
}
type Provider interface {
	Interrupt(context.Context, action.Bound, uint64) error
}
type TimelineReader interface {
	Timeline(context.Context, history.Bound, string, uint32) (history.Timeline, error)
}
type Service struct {
	Actions    *action.Service
	Repository Repository
	Provider   Provider
	Timeline   TimelineReader
	mu         sync.Mutex
	finished   map[string]string
}

func freshState() action.Evaluation {
	return action.Evaluation{Mode: action.WriterBlocked, Blocker: action.FreshSnapshotRequired, Flags: action.Flags{RequiresFreshSnapshot: true}}
}
func recorded(a Attempt) Outcome {
	if a.State != "resolved" {
		return Unknown
	}
	if a.Outcome == "accepted" {
		return Accepted
	}
	if a.Outcome == "state_observed" {
		return StateObserved
	}
	return Rejected
}
func (s *Service) flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, outcome := range s.finished {
		if err := s.Repository.Finish(ctx, id, outcome); err != nil {
			return err
		}
		delete(s.finished, id)
	}
	return nil
}
func (s *Service) finish(ctx context.Context, id, outcome string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished == nil {
		s.finished = map[string]string{}
	}
	s.finished[id] = outcome
	if err := s.Repository.Finish(ctx, id, outcome); err != nil {
		return err
	}
	delete(s.finished, id)
	return nil
}
func (s *Service) Send(ctx context.Context, subject, id, requestID string, consent bool) (Outcome, action.Evaluation, error) {
	if s == nil || s.Actions == nil || s.Repository == nil || s.Provider == nil || subject == "" || !session.ValidID(id) || !session.ValidID(requestID) {
		return 0, freshState(), action.ErrInvalid
	}
	b, in, err := s.Actions.ReadState(ctx, subject, id, action.Request{Intent: action.IntentRead, CloseIntent: action.NoCloseIntent, InterruptConfirmed: consent})
	if err != nil {
		return 0, freshState(), err
	}
	if err := s.flush(ctx); err != nil {
		return Unknown, freshState(), action.ErrPersistence
	}
	if !consent {
		return Rejected, action.Evaluate(in), nil
	}
	prior, found, err := s.Repository.Find(ctx, b, requestID)
	if err != nil {
		return 0, freshState(), action.ErrPersistence
	}
	// A repeated browser request reads its durable receipt. It cannot transmit
	// again, even after the captured Turn ends or the process restarts.
	if found {
		return recorded(prior), freshState(), nil
	}
	evaluation := action.Evaluate(in)
	if !evaluation.Flags.CanInterrupt || in.Run.ActiveTurnID == "" {
		return Rejected, evaluation, nil
	}
	a, dispatch, err := s.Repository.Begin(ctx, b, requestID, in.Run.ActiveTurnID, in.Run.Stamp)
	if err != nil {
		if errors.Is(err, action.ErrOutcomeUnknown) {
			return Unknown, freshState(), nil
		}
		return 0, freshState(), action.ErrPersistence
	}
	if !dispatch {
		return recorded(a), freshState(), nil
	}
	outcome := ""
	if s.Actions.Gate != nil && s.Actions.Gate(subject, id) {
		callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = s.Provider.Interrupt(callCtx, b, in.Run.Stamp.Run)
		cancel()
		if err == nil {
			outcome = "accepted"
		}
	} else {
		outcome = "rejected"
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.finish(saveCtx, a.ID, outcome); err != nil {
		return Unknown, freshState(), nil
	}
	if outcome == "accepted" {
		return Accepted, freshState(), nil
	}
	if outcome == "rejected" {
		return Rejected, freshState(), nil
	}
	return Unknown, freshState(), nil
}

// ObservePending uses the exact captured Turn's terminal timeline record and
// fresh matching projections. Settlement means current state was observed,
// never that a lost Interrupt receipt proved success. No mutation occurs here.
func (s *Service) ObservePending(ctx context.Context, subject, id string) error {
	if s == nil || s.Actions == nil || s.Actions.Repository == nil || s.Actions.Carriers == nil || s.Repository == nil || s.Timeline == nil || subject == "" || !session.ValidID(id) {
		return action.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	binding, err := s.Actions.Repository.Binding(ctx, subject, id)
	if err != nil || binding.SubjectID != subject || binding.ID != id {
		return action.ErrAuthority
	}
	carrier, err := s.Actions.Carriers.Resolve(ctx, subject, binding.ControllerBindingID)
	if err != nil {
		return action.ErrAuthority
	}
	if err := s.flush(ctx); err != nil {
		return action.ErrPersistence
	}
	pending, err := s.Repository.Pending(ctx, action.Bound{Binding: binding, Carrier: carrier})
	if err != nil || len(pending) == 0 {
		return err
	}
	b, in, err := s.Actions.ReadState(ctx, subject, id, action.Request{Intent: action.IntentRead, CloseIntent: action.NoCloseIntent})
	if err != nil {
		return err
	}
	if b.Binding.RunID != binding.RunID || b.Binding.ControllerBindingID != binding.ControllerBindingID || b.Carrier.ControllerID != carrier.ControllerID || b.Carrier.Generation != carrier.Generation {
		return action.ErrAuthority
	}
	if !in.Checked || !in.ControllerMatches || in.Freshness != action.Fresh || in.Local.Credential != action.Healthy || in.Local.ProjectionsStale ||
		in.Aggregate.Freshness != action.Fresh || in.Aggregate.Recovery != action.AggregateReady || in.Aggregate.Directive != action.NoAggregateAction ||
		in.Run.Recovery != action.NoRecovery || in.Run.Reconciliation != action.NoReconciliation || !in.Run.Stamp.Valid() || in.InteractionStamp != in.Run.Stamp || in.TimelineHead != in.Run.Stamp.Head {
		return nil
	}
	for _, a := range pending {
		if !a.DispatchFinished || a.State != "outcome_unknown" || in.Run.Stamp.Run <= a.Revision || in.Run.ActiveTurnID == a.TurnID {
			continue
		}
		after := a.Head
		bound := history.Bound{Binding: b.Binding, Workspace: b.Workspace, Carrier: b.Carrier, Floor: in.Run.Stamp}
		for page := 0; page < history.MaximumProviderPages; page++ {
			timeline, err := s.Timeline.Timeline(ctx, bound, after, 100)
			if err != nil {
				return err
			}
			if timeline.Stamp != in.Run.Stamp || timeline.Head != string(in.Run.Stamp.Head) {
				break
			}
			settled := false
			for _, entry := range timeline.Items {
				if entry.TurnID == a.TurnID && entry.Kind == history.TurnTerminal && (entry.Status == "completed" || entry.Status == "failed" || entry.Status == "interrupted") {
					settled = true
					break
				}
			}
			if settled {
				if err := s.Repository.Finish(ctx, a.ID, "state_observed"); err != nil {
					return err
				}
				break
			}
			if timeline.Next == "" || timeline.Next == after {
				break
			}
			after = timeline.Next
		}
	}
	return nil
}
