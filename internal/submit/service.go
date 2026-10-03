// Package submit admits text submissions against the current trusted action state.
package submit

import (
	"context"
	"errors"
	"unicode/utf8"

	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/files"
	"github.com/rootkernel/gul/internal/session"
)

const MaximumTextBytes = gulv1.MaximumSubmitTextBytes

type Dispatcher interface {
	SubmitInput(context.Context, action.Bound, action.Input, string, string, action.WriteIntent, Options) error
}
type Options struct {
	Images []files.ImageReference
	Effort *string
}
type Service struct {
	Actions    *action.Service
	Dispatcher Dispatcher
}

// Send does not queue a blocked request, retain a draft, or retry an unknown effect.
func (s *Service) Send(ctx context.Context, subject, id, attempt, text string, intent action.WriteIntent) (action.Evaluation, error) {
	return s.SendInput(ctx, subject, id, attempt, text, intent, Options{})
}
func (s *Service) SendInput(ctx context.Context, subject, id, attempt, text string, intent action.WriteIntent, options Options) (action.Evaluation, error) {
	if s == nil || s.Actions == nil || s.Dispatcher == nil || !session.ValidID(attempt) || len(text) == 0 || len(text) > MaximumTextBytes || !utf8.ValidString(text) || intent != action.IntentRead && intent != action.IntentWrite {
		return action.Evaluation{}, action.ErrInvalid
	}
	b, in, err := s.Actions.ReadState(ctx, subject, id, action.Request{Intent: intent, CloseIntent: action.NoCloseIntent})
	if err != nil {
		if errors.Is(err, action.ErrBlocked) {
			return action.Evaluation{Flags: action.Flags{RequiresFreshSnapshot: true}, Blocker: action.FreshSnapshotRequired, Mode: action.WriterBlocked}, err
		}
		return action.Evaluation{}, err
	}
	state := action.Evaluate(in)
	if intent == action.IntentRead && !state.Flags.CanSubmitRead || intent == action.IntentWrite && !state.Flags.CanSubmitWrite {
		return state, action.ErrBlocked
	}
	err = s.Dispatcher.SubmitInput(ctx, b, in, attempt, text, intent, options)
	if err != nil {
		if errors.Is(err, action.ErrInvalid) || errors.Is(err, action.ErrPersistence) {
			return state, err
		}
		if errors.Is(err, action.ErrBlocked) {
			return action.Evaluation{Flags: action.Flags{RequiresFreshSnapshot: true}, Blocker: action.FreshSnapshotRequired, Mode: action.WriterBlocked}, err
		}
		return action.Evaluation{Flags: action.Flags{RequiresFreshSnapshot: true, BlockedByOutcomeUnknown: true}, Blocker: action.UnresolvedOutcome, Mode: action.WriterBlocked}, errors.Join(action.ErrOutcomeUnknown, err)
	}
	return action.Evaluation{Flags: action.Flags{RequiresFreshSnapshot: true}, Blocker: action.FreshSnapshotRequired, Mode: action.WriterBlocked}, nil
}
