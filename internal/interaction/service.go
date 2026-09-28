package interaction

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
)

type ActionEvaluator interface {
	InteractionActions(context.Context, string, string) (action.Evaluation, error)
}
type MutationAttempts interface {
	BeginInteraction(context.Context, Bound, string, string) (string, error)
	FinishInteraction(context.Context, string, string) error // empty outcome means unknown
	ReconcileInteraction(context.Context, Bound, string, Status) error
}

type Service struct {
	Actions    ActionEvaluator
	Repository Repository
	Workspaces session.Workspace
	Carriers   session.CarrierResolver
	Provider   Provider
	Attempts   MutationAttempts
}

func (s *Service) bound(ctx context.Context, subject, id string, controller bool) (Bound, error) {
	if s == nil || s.Repository == nil || s.Workspaces == nil || s.Provider == nil || subject == "" || id == "" || len(id) > 256 {
		return Bound{}, ErrInvalid
	}
	b, err := s.Repository.Binding(ctx, subject, id)
	if err != nil || b.SubjectID != subject || b.ID != id || b.RunID == "" {
		return Bound{}, ErrAuthority
	}
	w, err := s.Workspaces.Revalidate(ctx, subject, b.WorkspaceID)
	if err != nil {
		return Bound{}, ErrAuthority
	}
	result := Bound{Binding: b, Workspace: w}
	if controller {
		if s.Carriers == nil || b.ControllerBindingID == "" {
			return Bound{}, ErrAuthority
		}
		result.Carrier, err = s.Carriers.Resolve(ctx, subject, b.ControllerBindingID)
		if err != nil || result.Carrier.ControllerID == "" || result.Carrier.AbsolutePath == "" || result.Carrier.Generation == 0 {
			return Bound{}, ErrAuthority
		}
	}
	return result, nil
}
func (s *Service) List(ctx context.Context, subject, id string) (PendingState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := s.bound(ctx, subject, id, false)
	if err != nil {
		return PendingState{}, err
	}
	return s.Provider.Pending(ctx, b)
}
func (s *Service) Get(ctx context.Context, subject, id, interactionID string) (Card, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if interactionID == "" || len(interactionID) > 256 {
		return Card{}, ErrInvalid
	}
	b, err := s.bound(ctx, subject, id, true)
	if err != nil {
		return Card{}, err
	}
	card, err := s.Provider.Card(ctx, b, interactionID)
	if err != nil {
		return Card{}, err
	}
	if s.Attempts != nil && (card.Summary.Status == Resolved || card.Summary.Status == Stale) {
		if err := s.Attempts.ReconcileInteraction(ctx, b, interactionID, card.Summary.Status); err != nil {
			return Card{}, ErrUnavailable
		}
	}
	card.Actions = action.Evaluation{Flags: action.Flags{BlockedByProviderCompatibility: true}, Blocker: action.ProviderFailure}
	if s.Actions != nil {
		evaluated, evaluationErr := s.Actions.InteractionActions(ctx, subject, id)
		if evaluationErr == nil {
			card.Actions = evaluated
		}
	}
	return card, nil
}

// Resolve consumes its input buffer. It retains neither the body nor any
// content-derived digest, and never retries even if the response is lost.
func (s *Service) Resolve(ctx context.Context, subject, id, interactionID string, body []byte) (Result, error) {
	defer clear(body)
	if s == nil || s.Provider == nil || len(body) == 0 || len(body) > MaximumResponseBytes || len(body) > s.Provider.ResponseLimit() {
		return Result{}, ErrInvalid
	}
	card, err := s.Get(ctx, subject, id, interactionID)
	if err != nil {
		return Result{}, err
	}
	if card.Summary.Status == Resolved {
		return Result{Outcome: OutcomeResolved}, nil
	}
	if card.Summary.Status == Stale {
		return Result{Outcome: OutcomeStale}, nil
	}
	if !card.Actions.Flags.CanResolveInteraction {
		return Result{}, ErrBlocked
	}

	normalized, err := normalize(body, card)
	if err != nil {
		return Result{}, err
	}
	defer clear(normalized)
	if len(normalized) > s.Provider.ResponseLimit() || len(normalized) > MaximumResponseBytes {
		return Result{}, ErrInvalid
	}
	authorityCtx, authorityCancel := context.WithTimeout(ctx, 5*time.Second)
	b, err := s.bound(authorityCtx, subject, id, true)
	authorityCancel()
	if err != nil {
		return Result{}, err
	}
	var key [16]byte
	if _, err = rand.Read(key[:]); err != nil {
		return Result{}, ErrUnavailable
	}
	if s.Attempts == nil {
		return Result{}, ErrUnavailable
	}
	keyText := hex.EncodeToString(key[:])
	attemptID, err := s.Attempts.BeginInteraction(ctx, b, interactionID, keyText)
	if err != nil {
		if errors.Is(err, ErrAttemptConflict) {
			return s.observeCompetingResolution(ctx, subject, id, interactionID)
		}
		return Result{}, ErrUnavailable
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	receipt, callErr := s.Provider.Resolve(callCtx, b, interactionID, keyText, normalized)
	cancel()
	clear(normalized)
	clear(body)
	// A transport failure may follow durable acceptance. Always use fresh reads;
	// never interpret the receipt as a complete refreshed Interaction.
	refreshCtx, refreshCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer refreshCancel()
	fresh, refreshErr := s.Get(refreshCtx, subject, id, interactionID)
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	result := Result{Outcome: OutcomeUnknown}
	if callErr == nil {
		result.Receipt = receipt.Receipt
	}
	if refreshErr != nil {
		if err := s.Attempts.FinishInteraction(finishCtx, attemptID, ""); err != nil {
			return result, ErrUnavailable
		}
		return result, nil
	}
	outcomeRef := ""
	switch fresh.Summary.Status {
	case Resolved:
		result.Outcome = OutcomeResolved
		outcomeRef = "resolved"
	case Stale:
		result.Outcome = OutcomeStale
		outcomeRef = "stale"
	case Pending:
		result.Outcome = OutcomeReenter
		result.Card = &fresh
		outcomeRef = "reenter"
	}
	if err := s.Attempts.FinishInteraction(finishCtx, attemptID, outcomeRef); err != nil {
		return Result{Outcome: OutcomeUnknown}, ErrUnavailable
	}
	return result, nil
}

func (s *Service) observeCompetingResolution(ctx context.Context, subject, id, interactionID string) (Result, error) {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		card, err := s.Get(readCtx, subject, id, interactionID)
		if err != nil {
			return Result{Outcome: OutcomeUnknown}, nil
		}
		switch card.Summary.Status {
		case Resolved:
			return Result{Outcome: OutcomeResolved}, nil
		case Stale:
			return Result{Outcome: OutcomeStale}, nil
		}
		select {
		case <-readCtx.Done():
			return Result{Outcome: OutcomeUnknown}, nil
		case <-ticker.C:
		}
	}
}

func decode(data []byte, target any) error {
	if !utf8.Valid(data) {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return ErrInvalid
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func normalize(body []byte, card Card) ([]byte, error) {
	switch card.Summary.Kind {
	case CommandApproval, FileApproval:
		var response struct {
			Decision string `json:"decision"`
		}
		if decode(body, &response) != nil {
			return nil, ErrInvalid
		}
		d := map[string]Decision{"accept_once": AcceptOnce, "decline": Decline, "cancel": Cancel}[response.Decision]
		found := false
		for _, allowed := range card.Decisions {
			found = found || d == allowed
		}
		if d == 0 || !found {
			return nil, ErrInvalid
		}
		return json.Marshal(response)
	case UserInput:
		var response struct {
			Answers map[string]struct {
				Answers []string `json:"answers"`
			} `json:"answers"`
		}
		if card.Input == nil || decode(body, &response) != nil || len(response.Answers) == 0 {
			return nil, ErrInvalid
		}
		questions := map[string]Question{}
		for _, q := range card.Input.Questions {
			questions[q.ID] = q
		}
		if len(response.Answers) != len(questions) {
			return nil, ErrInvalid
		}
		for id, answer := range response.Answers {
			q, ok := questions[id]
			if !ok || len(answer.Answers) == 0 {
				return nil, ErrInvalid
			}
			for _, value := range answer.Answers {
				if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
					return nil, ErrInvalid
				}
				if len(q.Options) > 0 && !q.AllowsOther {
					found := false
					for _, o := range q.Options {
						found = found || value == o.Label
					}
					if !found {
						return nil, ErrInvalid
					}
				}
			}
		}
		return json.Marshal(response)
	default:
		return nil, ErrBlocked
	}
}
