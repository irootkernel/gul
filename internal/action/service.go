package action

import (
	"context"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

var (
	ErrOutcomeUnknown        = errors.New("action outcome unknown")
	ErrInvalid               = errors.New("invalid action request")
	ErrAuthority             = errors.New("action authority unavailable")
	ErrUnavailable           = errors.New("action provider unavailable")
	ErrBlocked               = errors.New("action blocked by current eligibility")
	ErrWriterBusy            = errors.New("writer_busy")
	ErrUnsupportedTransition = errors.New("unsupported access transition")
)

type Bound struct {
	Binding   session.Binding
	Workspace workspace.Attachment
	Carrier   session.Carrier
}
type Repository interface {
	Binding(context.Context, string, string) (session.Binding, error)
	LocalState(context.Context, Bound) (LocalState, error)
}
type Provider interface {
	Read(context.Context, Bound) (Input, error)
	Acquire(context.Context, Bound, uint64) (WriterProjection, error)
	Release(context.Context, Bound, uint64) (WriterProjection, error)
}
type Service struct {
	Repository Repository
	Workspaces session.Workspace
	Carriers   session.CarrierResolver
	Provider   Provider
}

func (s *Service) state(ctx context.Context, subject, id string, request Request) (Bound, Input, error) {
	if s == nil || s.Repository == nil || s.Workspaces == nil || s.Provider == nil || subject == "" || id == "" || len(id) > 256 {
		return Bound{}, Input{}, ErrInvalid
	}
	b, err := s.Repository.Binding(ctx, subject, id)
	if err != nil || b.SubjectID != subject || b.ID != id || b.RunID == "" || b.ProviderSessionID == "" {
		return Bound{}, Input{}, ErrAuthority
	}
	w, err := s.Workspaces.Revalidate(ctx, subject, b.WorkspaceID)
	if err != nil {
		return Bound{}, Input{}, ErrAuthority
	}
	bound := Bound{Binding: b, Workspace: w}
	health := CredentialMissing
	if s.Carriers != nil && b.ControllerBindingID != "" {
		carrier, err := s.Carriers.Resolve(ctx, subject, b.ControllerBindingID)
		health = CredentialUnhealthy
		if err == nil && carrier.ControllerID != "" && carrier.AbsolutePath != "" && carrier.Generation > 0 {
			bound.Carrier = carrier
			health = Healthy
		}
	}
	in, err := s.Provider.Read(ctx, bound)
	if err != nil {
		return bound, Input{}, err
	}
	local, err := s.Repository.LocalState(ctx, bound)
	if err != nil {
		return bound, Input{}, ErrAuthority
	}
	// Only the backend credential store and binding repository establish these.
	if health == Healthy && !in.ControllerMatches {
		health = CredentialUnhealthy
	}
	local.Credential = health
	in.Local = local
	in.Request = request
	return bound, in, nil
}
func (s *Service) Evaluate(ctx context.Context, subject, id string, request Request) (Evaluation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, in, err := s.state(ctx, subject, id, request)
	if err != nil {
		return Evaluation{Flags: Flags{BlockedByProviderCompatibility: true}, Blocker: ProviderFailure, Mode: WriterBlocked}, err
	}
	return Evaluate(in), nil
}
func (s *Service) InteractionActions(ctx context.Context, subject, id string) (Evaluation, error) {
	return s.Evaluate(ctx, subject, id, Request{Intent: IntentRead, CloseIntent: NoCloseIntent})
}
func (s *Service) RequireSubmit(ctx context.Context, subject, id string, intent WriteIntent) (Evaluation, error) {
	result, err := s.Evaluate(ctx, subject, id, Request{Intent: intent, CloseIntent: NoCloseIntent})
	if err != nil {
		return result, err
	}
	if intent == IntentRead && result.Flags.CanSubmitRead || intent == IntentWrite && result.Flags.CanSubmitWrite {
		return result, nil
	}
	return result, blockedError(result.Blocker)
}

type MutationResult struct {
	Accepted WriterProjection
	// An accepted writer response alone cannot establish compatible Run and
	// Interaction projections. A subsequent explicit read derives fresh flags.
	Evaluation Evaluation
}

func (s *Service) Acquire(ctx context.Context, subject, id string) (MutationResult, error) {
	return s.mutate(ctx, subject, id, true)
}
func (s *Service) Release(ctx context.Context, subject, id string) (MutationResult, error) {
	return s.mutate(ctx, subject, id, false)
}
func (s *Service) mutate(ctx context.Context, subject, id string, acquire bool) (MutationResult, error) {
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	b, in, err := s.state(readCtx, subject, id, Request{Intent: IntentWrite, CloseIntent: NoCloseIntent})
	cancel()
	if err != nil {
		return MutationResult{}, err
	}
	evaluation := Evaluate(in)
	if acquire && !evaluation.Flags.CanAcquireWriter || !acquire && !evaluation.Flags.CanReleaseWriter {
		return MutationResult{Evaluation: evaluation}, blockedError(evaluation.Blocker)
	}
	// Each tokenless mutation is invoked once with the fresh Run revision. The
	// provider rechecks Controller/revision and owns all authority transitions.
	callCtx, callCancel := context.WithTimeout(ctx, 20*time.Second)
	defer callCancel()
	var accepted WriterProjection
	if acquire {
		accepted, err = s.Provider.Acquire(callCtx, b, in.Run.Stamp.Run)
	} else {
		accepted, err = s.Provider.Release(callCtx, b, in.Run.Stamp.Run)
	}
	if err != nil {
		if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrBlocked) {
			err = ErrOutcomeUnknown
		}
		return MutationResult{}, err
	}
	return MutationResult{Accepted: accepted, Evaluation: Evaluation{Writer: accepted, Mode: WriterBlocked, Flags: Flags{RequiresFreshSnapshot: true}, Blocker: FreshSnapshotRequired}}, nil
}
func blockedError(b Blocker) error {
	switch b {
	case WriterBusy:
		return ErrWriterBusy
	case UnsupportedTransition:
		return ErrUnsupportedTransition
	default:
		return ErrBlocked
	}
}
