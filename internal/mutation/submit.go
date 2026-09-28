package mutation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/rootkernel/gul/internal/operation"
)

const MaximumSubmitBytes = 8 << 20

type SubmitRequest struct {
	OperationID, SubjectID, RunID, BindingID, ControllerID, IdempotencyKey string
	Canonical                                                              []byte // normalized provider request, including prompt and images
	CreatedAt                                                              time.Time
}

type SubmitReference struct {
	OperationID, SubjectID, RunID, BindingID, ControllerID string
}

type SubmitEvidence struct {
	// Accepted and Rejected require exact provider evidence for this attempt.
	// A missing timeline page or matching prompt text is always Unknown.
	Status string // accepted, rejected, unknown
	TurnID string
}

type SubmitProvider interface {
	SubmitTurn(context.Context, SubmitRequest) (string, error)
	ReconcileTurn(context.Context, SubmitReference) (SubmitEvidence, error)
}

type SubmitAttempts interface {
	Mutation(context.Context, string) (operation.MutationAttempt, error)
	BeginMutation(context.Context, operation.MutationAttempt) (operation.MutationAttempt, bool, error)
	MarkOutcomeUnknown(context.Context, string) error
	ResolveMutation(context.Context, string, string) error
}

type SubmitService struct {
	Attempts SubmitAttempts
	Provider SubmitProvider
	Gate     func(string, string) bool // subject, Run; compatibility and convergence
	mu       sync.Mutex
	live     map[string]SubmitRequest
}

func submitDigest(request SubmitRequest) (string, error) {
	if request.OperationID == "" || request.SubjectID == "" || request.RunID == "" || request.BindingID == "" || request.ControllerID == "" ||
		request.IdempotencyKey == "" || request.CreatedAt.IsZero() || len(request.Canonical) == 0 || len(request.Canonical) > MaximumSubmitBytes {
		return "", ErrInvalid
	}
	b, err := json.Marshal(struct {
		OperationID, SubjectID, RunID, BindingID, ControllerID, IdempotencyKey string
		Canonical                                                              []byte
	}{request.OperationID, request.SubjectID, request.RunID, request.BindingID, request.ControllerID, request.IdempotencyKey, request.Canonical})
	if err != nil {
		return "", ErrInvalid
	}
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:]), nil
}

func (s *SubmitService) Submit(ctx context.Context, request SubmitRequest) (operation.MutationAttempt, error) {
	if s == nil || s.Attempts == nil || s.Provider == nil || s.Gate == nil {
		return operation.MutationAttempt{}, ErrInvalid
	}
	digest, err := submitDigest(request)
	if err != nil {
		return operation.MutationAttempt{}, err
	}
	if !s.Gate(request.SubjectID, request.RunID) {
		return operation.MutationAttempt{}, ErrBlocked
	}
	a := operation.MutationAttempt{OperationAttempt: operation.OperationAttempt{OperationID: request.OperationID, SubjectID: request.SubjectID,
		Kind: "SubmitTurn", RequestSHA256: digest, State: "pending", CreatedAt: request.CreatedAt,
		ControllerReferences: []operation.ControllerReference{{Role: "source", BindingID: request.BindingID, ExpectedControllerID: request.ControllerID}}},
		TargetRef: request.RunID, DeadlineAt: request.CreatedAt.Add(20 * time.Second), ReconciliationRoute: "run_timeline"}
	stored, dispatch, err := s.Attempts.BeginMutation(ctx, a)
	if err != nil {
		return operation.MutationAttempt{}, err
	}
	if !dispatch {
		return stored, nil
	}
	s.mu.Lock()
	if s.live == nil {
		s.live = make(map[string]SubmitRequest)
	}
	request.Canonical = append([]byte(nil), request.Canonical...)
	retained := request
	retained.Canonical = append([]byte(nil), request.Canonical...)
	s.live[request.OperationID] = retained
	s.mu.Unlock()
	defer clear(request.Canonical)
	return s.dispatchSubmit(ctx, stored, request)
}

func (s *SubmitService) dispatchSubmit(ctx context.Context, a operation.MutationAttempt, request SubmitRequest) (operation.MutationAttempt, error) {
	if !s.Gate(request.SubjectID, request.RunID) {
		_ = s.markSubmitUnknown(ctx, a.OperationID)
		return a, ErrBlocked
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	turnID, err := s.Provider.SubmitTurn(callCtx, request)
	cancel()
	if err != nil || turnID == "" {
		if markErr := s.markSubmitUnknown(ctx, a.OperationID); markErr != nil {
			return a, markErr
		}
		a.State = "outcome_unknown"
		return a, ErrUnknown
	}
	if err := s.Attempts.ResolveMutation(context.WithoutCancel(ctx), a.OperationID, turnID); err != nil {
		return a, err
	}
	s.forget(a.OperationID)
	a.State, a.OutcomeRef = "resolved", turnID
	return a, nil
}

func (s *SubmitService) markSubmitUnknown(ctx context.Context, id string) error {
	err := s.Attempts.MarkOutcomeUnknown(context.WithoutCancel(ctx), id)
	if err != nil {
		a, getErr := s.Attempts.Mutation(context.WithoutCancel(ctx), id)
		if getErr == nil && a.State == "outcome_unknown" {
			return nil
		}
	}
	return err
}

func (s *SubmitService) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request, ok := s.live[id]; ok {
		clear(request.Canonical)
		delete(s.live, id)
	}
}

// RecoverTurn always reads authoritative Run/timeline evidence first. Only a
// still-live exact request can be resent; a new process has no such bytes.
func (s *SubmitService) RecoverTurn(ctx context.Context, id string) (operation.MutationAttempt, error) {
	if s == nil || s.Attempts == nil || s.Provider == nil || s.Gate == nil || id == "" {
		return operation.MutationAttempt{}, ErrInvalid
	}
	a, err := s.Attempts.Mutation(ctx, id)
	if err != nil || a.Kind != "SubmitTurn" || len(a.ControllerReferences) != 1 {
		return operation.MutationAttempt{}, ErrInvalid
	}
	if a.State == "resolved" {
		return a, nil
	}
	if a.State == "pending" {
		return a, ErrBlocked
	}
	if !s.Gate(a.SubjectID, a.TargetRef) {
		return a, ErrBlocked
	}
	ref := SubmitReference{OperationID: a.OperationID, SubjectID: a.SubjectID, RunID: a.TargetRef,
		BindingID: a.ControllerReferences[0].BindingID, ControllerID: a.ControllerReferences[0].ExpectedControllerID}
	evidence, err := s.Provider.ReconcileTurn(ctx, ref)
	if err != nil {
		return a, ErrUnknown
	}
	switch evidence.Status {
	case "accepted":
		if evidence.TurnID == "" {
			return a, ErrUnknown
		}
		if err := s.Attempts.ResolveMutation(context.WithoutCancel(ctx), id, evidence.TurnID); err != nil {
			return a, err
		}
		s.forget(id)
		a.State, a.OutcomeRef = "resolved", evidence.TurnID
		return a, nil
	case "rejected":
		if err := s.Attempts.ResolveMutation(context.WithoutCancel(ctx), id, "rejected"); err != nil {
			return a, err
		}
		s.forget(id)
		a.State, a.OutcomeRef = "resolved", "rejected"
		return a, nil
	case "unknown":
	default:
		return a, ErrUnknown
	}
	s.mu.Lock()
	request, live := s.live[id]
	if live {
		request.Canonical = append([]byte(nil), request.Canonical...)
	}
	s.mu.Unlock()
	if !live {
		return a, ErrUnknown
	}
	defer clear(request.Canonical)
	digest, err := submitDigest(request)
	if err != nil || digest != a.RequestSHA256 {
		return a, ErrBlocked
	}
	return s.dispatchSubmit(ctx, a, request)
}

func (s *SubmitService) DropProcessMemory() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, request := range s.live {
		clear(request.Canonical)
		delete(s.live, id)
	}
}
