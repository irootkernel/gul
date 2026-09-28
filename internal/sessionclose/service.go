package sessionclose

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
)

type Service struct {
	Repository Repository
	Actions    *action.Service
	Sessions   *session.Service
	Provider   Provider
	Refresh    Refresher
	finishedMu sync.Mutex
	finished   map[string]finishedClose
}

type finishedClose struct {
	attempt           Attempt
	providerOperation string
}

func (s *Service) remember(a Attempt, providerOperation string) {
	s.finishedMu.Lock()
	defer s.finishedMu.Unlock()
	if s.finished == nil {
		s.finished = make(map[string]finishedClose)
	}
	s.finished[a.ID] = finishedClose{attempt: a, providerOperation: providerOperation}
}

func (s *Service) forget(id string) {
	s.finishedMu.Lock()
	delete(s.finished, id)
	s.finishedMu.Unlock()
}

func (s *Service) completed(id string) (finishedClose, bool) {
	s.finishedMu.Lock()
	defer s.finishedMu.Unlock()
	a, ok := s.finished[id]
	return a, ok
}

func (s *Service) flushCompleted(ctx context.Context, a Attempt) (Attempt, error) {
	finished, ok := s.completed(a.ID)
	if !ok || finished.attempt.SubjectID != a.SubjectID || finished.attempt.SessionID != a.SessionID || finished.attempt.RequestID != a.RequestID {
		return a, nil
	}
	if finished.providerOperation != "" && finished.attempt.Outcome.OperationRef == "" {
		binding, err := s.Repository.Binding(ctx, a.SubjectID, a.SessionID)
		if err != nil {
			return a, err
		}
		ref, err := s.Repository.OperationReference(ctx, binding, finished.providerOperation)
		if err != nil {
			return a, err
		}
		finished.attempt.Outcome.OperationRef = ref
	}
	if err := s.Repository.Save(ctx, finished.attempt); err != nil {
		return a, err
	}
	s.forget(a.ID)
	current, found, err := s.Repository.Find(ctx, a.SubjectID, a.SessionID, a.RequestID)
	if err != nil || !found {
		return a, errors.Join(err, ErrUnavailable)
	}
	return current, nil
}

func (s *Service) flushSession(ctx context.Context, subject, id string) error {
	s.finishedMu.Lock()
	var entries []Attempt
	for _, a := range s.finished {
		if a.attempt.SubjectID == subject && a.attempt.SessionID == id {
			entries = append(entries, a.attempt)
		}
	}
	s.finishedMu.Unlock()
	for _, a := range entries {
		if _, err := s.flushCompleted(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Close(ctx context.Context, subject, id, requestID string, interrupt bool) (Outcome, error) {
	return s.execute(ctx, subject, id, requestID, Close, interrupt)
}

// Recover and Reconcile are explicit backend actions. They are never scheduled
// by a failed Close call or by browser reconnection.
func (s *Service) Recover(ctx context.Context, subject, id, requestID string, interrupt bool) (Outcome, error) {
	return s.execute(ctx, subject, id, requestID, Recover, interrupt)
}
func (s *Service) Reconcile(ctx context.Context, subject, id, requestID string, interrupt bool) (Outcome, error) {
	return s.execute(ctx, subject, id, requestID, Reconcile, interrupt)
}

func (s *Service) execute(ctx context.Context, subject, id, requestID string, kind Kind, interrupt bool) (Outcome, error) {
	if s == nil || s.Repository == nil || s.Actions == nil || s.Sessions == nil || s.Provider == nil || s.Refresh == nil || subject == "" || !session.ValidID(id) || !session.ValidID(requestID) {
		return Outcome{}, ErrInvalid
	}
	// Session authorization precedes attempt lookup and all provider reads.
	binding, err := s.Repository.Binding(ctx, subject, id)
	if err != nil {
		return Outcome{}, bindingError(err)
	}
	if binding.SubjectID != subject || binding.ID != id {
		return Outcome{}, session.ErrNotFound
	}
	digest := requestDigest(id, kind, interrupt)
	previous, found, err := s.Repository.Find(ctx, subject, id, requestID)
	if err != nil {
		return Outcome{}, err
	}
	if found {
		if previous.RequestSHA256 != digest {
			return Outcome{}, ErrConflict
		}
		if _, ok := s.completed(previous.ID); ok {
			previous, err = s.flushCompleted(ctx, previous)
			if err != nil {
				return Outcome{}, err
			}
		}
		if !previous.DispatchFinished || previous.Outcome.Status == Rejected || previous.Outcome.Status == Confirmed {
			return previous.Outcome, nil
		}
		return s.observe(ctx, previous)
	}
	key, err := newID()
	if err != nil {
		return Outcome{}, err
	}
	request := action.Request{Intent: action.IntentRead, CloseIntent: action.CompleteSession, InterruptConfirmed: interrupt}
	if interrupt {
		request.CloseIntent = action.AbortSession
	}
	bound, input, readErr := s.Actions.ReadState(ctx, subject, id, request)
	if readErr != nil {
		out := rejectionForRead(readErr)
		out.AttemptID = key
		return out, nil
	}
	eligible := action.Evaluate(input)
	allowed := kind == Close && eligible.Flags.CanRequestSessionClose || kind == Recover && eligible.Flags.CanRecover || kind == Reconcile && eligible.Flags.CanReconcile
	if !allowed {
		out := rejectionFor(eligible)
		out.AttemptID = key
		return out, nil
	}
	attempt := Attempt{ID: key, SubjectID: subject, SessionID: id, RequestID: requestID, Kind: kind, Interrupt: interrupt, RequestSHA256: digest, CreatedAt: time.Now().UTC(), Outcome: Outcome{Status: InProgress, AttemptID: key, NextAction: "WAIT"}}
	attempt, dispatch, err := s.Repository.Begin(ctx, bound, attempt)
	if err != nil {
		return Outcome{}, err
	}
	if !dispatch {
		return attempt.Outcome, nil
	}
	deadline := CloseTimeout
	if kind != Close {
		deadline = RecoveryTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, deadline)
	mutation, callErr := s.Provider.Mutate(callCtx, bound, kind, input.Run.Stamp.Run, interrupt)
	cancel()
	attempt.DispatchFinished = true
	attempt.Outcome.Status = OutcomeUnknown
	attempt.Outcome.NextAction = "REFRESH_SNAPSHOT"
	// The browser may have disconnected after transmission. Persist uncertainty
	// independently, with a finite deadline, before attempting observation.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	if callErr == nil {
		if mutation.OperationID != "" {
			attempt.Outcome.OperationRef, err = s.Repository.OperationReference(saveCtx, bound.Binding, mutation.OperationID)
			if err != nil {
				s.remember(attempt, mutation.OperationID)
				_ = s.Repository.Save(saveCtx, attempt)
				return Outcome{}, err
			}
		}
		if mutation.RejectionCode != "" {
			attempt.Outcome.Status, attempt.Outcome.Code, attempt.Outcome.NextAction = Rejected, mutation.RejectionCode, mutation.NextAction
		} else if mutation.Pending {
			attempt.Outcome.Status = InProgress
		}
	}
	s.remember(attempt, "")
	if err := s.Repository.Save(saveCtx, attempt); err != nil {
		return Outcome{}, err
	}
	s.forget(attempt.ID)
	if attempt.Outcome.Status == Rejected || ctx.Err() != nil {
		return attempt.Outcome, nil
	}
	if callErr == nil && mutation.Run.Stamp.Valid() {
		if err := s.Refresh.Refresh(ctx, bound, mutation.Run); err != nil {
			return attempt.Outcome, nil
		}
	}
	return s.observe(ctx, attempt)
}

// ObservePending updates retained outcomes only from authoritative reads. It
// never transmits a mutation, retries a call or starts recovery.
func (s *Service) ObservePending(ctx context.Context, subject, id string) error {
	if s == nil || s.Repository == nil || s.Actions == nil || s.Sessions == nil || s.Refresh == nil || subject == "" || !session.ValidID(id) {
		return ErrInvalid
	}
	b, err := s.Repository.Binding(ctx, subject, id)
	if err != nil {
		return bindingError(err)
	}
	if b.SubjectID != subject || b.ID != id {
		return session.ErrNotFound
	}
	if err := s.flushSession(ctx, subject, id); err != nil {
		return err
	}
	attempts, err := s.Repository.Pending(ctx, subject, id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, attempt := range attempts {
		if _, err := s.observe(ctx, attempt); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) observe(ctx context.Context, attempt Attempt) (Outcome, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	state, err := s.Sessions.GetExecutionState(ctx, attempt.SubjectID, attempt.SessionID)
	if err != nil || state.Freshness != "fresh" || state.Snapshot == nil {
		return attempt.Outcome, nil
	}
	request := action.Request{Intent: action.IntentRead, CloseIntent: action.CompleteSession, InterruptConfirmed: attempt.Interrupt}
	bound, input, err := s.Actions.ReadState(ctx, attempt.SubjectID, attempt.SessionID, request)
	if err != nil {
		return attempt.Outcome, nil
	}
	if input.Local.ProjectionsStale {
		if err := s.Refresh.Refresh(ctx, bound, input.Run); err != nil {
			return attempt.Outcome, nil
		}
		bound, input, err = s.Actions.ReadState(ctx, attempt.SubjectID, attempt.SessionID, request)
		if err != nil {
			return attempt.Outcome, nil
		}
	}
	// The operation itself blocks conflicting mutations. It does not prevent
	// checking whether independent authoritative reads have converged.
	input.Local.Operation = action.NoOperation
	evaluation := action.Evaluate(input)
	if (evaluation.Flags.RequiresFreshSnapshot && !evaluation.Flags.CanRecover && !evaluation.Flags.CanReconcile) || evaluation.Flags.BlockedByProviderCompatibility || evaluation.Flags.BlockedByCredentialState || state.Snapshot.AggregateRevision != input.Aggregate.Revision || state.Snapshot.RunRevision != input.Run.Stamp.Run {
		return attempt.Outcome, nil
	}
	matchedClose := false
	if operation := state.Snapshot.CloseOperationID; attempt.Kind == Close && operation != "" && attempt.Outcome.OperationRef != "" {
		ref, err := s.Repository.OperationReference(ctx, bound.Binding, operation)
		if err != nil {
			return Outcome{}, err
		}
		if attempt.Outcome.OperationRef != ref {
			return attempt.Outcome, nil
		}
		matchedClose = true
	}
	aggregate := input.Aggregate
	attempt.Outcome.Code = ""
	attempt.Outcome.NextAction = "REFRESH_SNAPSHOT"
	switch {
	case evaluation.Flags.CanReconcile:
		attempt.Outcome.Status, attempt.Outcome.NextAction = RecoveryRequired, "RECONCILE_RUN"
	case evaluation.Flags.CanRecover:
		attempt.Outcome.Status, attempt.Outcome.NextAction = RecoveryRequired, "RECOVER_RUN"
	case evaluation.Flags.RequiresOperatorAction:
		attempt.Outcome.Status, attempt.Outcome.NextAction = RecoveryRequired, "OPERATOR_REPAIR"
	case evaluation.Flags.BlockedByOutcomeUnknown || aggregate.UnknownOutcomeTasks != 0 || input.Run.Background == action.BackgroundUnverified:
		attempt.Outcome.Status = OutcomeUnknown
	case aggregate.CloseProgress == action.CloseRecovery || aggregate.Recovery != action.AggregateReady:
		attempt.Outcome.Status = RecoveryRequired
	case attempt.Kind != Close && aggregate.CloseProgress == action.NoClose && (aggregate.Lifecycle == action.SessionActive || aggregate.Lifecycle == action.SessionDegraded) && input.Run.Recovery == action.NoRecovery && input.Run.Reconciliation == action.NoReconciliation && input.Writer.Reconciliation == action.NoReconciliation && (input.Run.Lifecycle == action.Idle || input.Run.Lifecycle == action.Paused) && !evaluation.Flags.BlockedByBackgroundExecution:
		// Confirmation here resolves this explicit recovery request; only Close
		// is exposed as a browser CloseOutcome. It does not close the session.
		if err := s.Refresh.Refresh(ctx, bound, input.Run); err != nil {
			return attempt.Outcome, nil
		}
		attempt.Outcome.Status, attempt.Outcome.NextAction = Confirmed, "ABORT"
	case input.Run.Lifecycle == action.Closed && (aggregate.CloseProgress == action.CloseCompleted || aggregate.CloseProgress == action.CloseAborted) && (aggregate.Lifecycle == action.SessionCompleted || aggregate.Lifecycle == action.SessionAborted) && aggregate.NonterminalSpawns == 0 && aggregate.PendingApprovals == 0 && aggregate.AcceptedUnfinishedTasks == 0 && input.Run.ActiveTurn == action.Missing && input.Run.Pending == 0 && (input.Run.Background == action.Absent || input.Run.Background == action.NotApplicable) && input.Run.Authority == action.Unowned && input.Writer.Owner != action.ThisSession && !input.Writer.BackgroundBlocked && !input.Writer.RecoveryBlocked:
		if attempt.Kind == Close && (!matchedClose || !closeIntentMatches(attempt.Interrupt, aggregate)) {
			attempt.Outcome.Status = OutcomeUnknown
			break
		}
		if !terminalSnapshot(*state.Snapshot, aggregate) {
			return attempt.Outcome, nil
		}
		if err := s.Refresh.Refresh(ctx, bound, input.Run); err != nil {
			return attempt.Outcome, nil
		}
		attempt.Outcome.Status, attempt.Outcome.NextAction = Confirmed, "ABORT"
	case aggregate.CloseProgress == action.CloseSettling:
		attempt.Outcome.Status = InProgress
	default:
		attempt.Outcome.Status = OutcomeUnknown
	}
	if err := s.Repository.Save(ctx, attempt); err != nil {
		return Outcome{}, err
	}
	return attempt.Outcome, nil
}

func closeIntentMatches(interrupt bool, aggregate action.Aggregate) bool {
	if interrupt {
		return aggregate.CloseIntent == action.AbortSession && aggregate.CloseProgress == action.CloseAborted && aggregate.Lifecycle == action.SessionAborted
	}
	return aggregate.CloseIntent == action.CompleteSession && aggregate.CloseProgress == action.CloseCompleted && aggregate.Lifecycle == action.SessionCompleted
}

func terminalSnapshot(s session.Snapshot, a action.Aggregate) bool {
	return ((s.Lifecycle == "completed" && a.Lifecycle == action.SessionCompleted) || (s.Lifecycle == "aborted" && a.Lifecycle == action.SessionAborted)) &&
		s.CloseProgress == "confirmed" &&
		s.Recovery == "none" && s.Counts.NonterminalSpawns == 0 && s.Counts.PendingApprovals == 0 && s.Counts.AcceptedUnfinishedTasks == 0 && s.Counts.UnknownOutcomeTasks == 0
}

func bindingError(err error) error {
	if errors.Is(err, session.ErrNotFound) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %w", session.ErrPersistenceUnavailable, err)
}
func requestDigest(id string, kind Kind, interrupt bool) string {
	data, _ := json.Marshal(struct {
		Session   string
		Kind      Kind
		Interrupt bool
	}{id, kind, interrupt})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func newID() (string, error) {
	var key [24]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	return "close_" + hex.EncodeToString(key[:]), nil
}
func rejectionForRead(err error) Outcome {
	if errors.Is(err, action.ErrAuthority) {
		return Outcome{Status: Rejected, Code: "CONTROLLER_CARRIER_INVALID", NextAction: "VERIFY_CONTROLLER"}
	}
	return Outcome{Status: Rejected, Code: "SOURCE_UNAVAILABLE", NextAction: "REFRESH_SNAPSHOT"}
}
func rejectionFor(e action.Evaluation) Outcome {
	out := Outcome{Status: Rejected, Code: "PROVIDER_BLOCKED", NextAction: "REFRESH_SNAPSHOT"}
	switch {
	case e.Flags.BlockedByCredentialState:
		out.Code, out.NextAction = "CONTROLLER_CARRIER_INVALID", "VERIFY_CONTROLLER"
	case e.Flags.RequiresOperatorAction || e.Flags.BlockedByProviderCompatibility:
		out.NextAction = "OPERATOR_REPAIR"
	case e.Flags.BlockedByOutcomeUnknown:
		out.Code = "OUTCOME_UNKNOWN"
	case e.Flags.RequiresCloseConfirmation:
		out.Code, out.NextAction = "RUN_STATE_CONFLICT", "FIX_REQUEST"
	}
	return out
}
