// Package mutation coordinates provider operations without deriving resend
// authority from browser delivery or cached history.
package mutation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/rootkernel/gul/internal/operation"
	"github.com/rootkernel/gul/internal/replay"
)

var ErrInvalid = errors.New("invalid mutation request")
var ErrBlocked = errors.New("mutation blocked by unresolved outcome")
var ErrUnknown = errors.New("mutation outcome unknown")

type StartResult struct {
	RunID, WorkspaceID, ControllerID string
}

// StartProvider is an application-level replay port. Its implementation must
// disable transport retries and check the returned Run's identity.
type StartProvider interface {
	StartRun(context.Context, replay.StartRun, string, string) (StartResult, error)
	ListRunsByController(context.Context, replay.StartRun, string) ([]StartResult, error)
}

type StartResolver interface {
	Workspace(context.Context, string, string) (absoluteRoot, providerWorkspaceID string, err error)
	Carrier(context.Context, string, string) (absolutePath, controllerID string, generation uint64, err error)
}

type StartAttempts interface {
	Mutation(context.Context, string) (operation.MutationAttempt, error)
	BeginMutation(context.Context, operation.MutationAttempt) (operation.MutationAttempt, bool, error)
	MarkOutcomeUnknown(context.Context, string) error
	ExpireReplay(context.Context, string) error
	ResolveMutation(context.Context, string, string) error
	Replayable(context.Context) ([]operation.MutationAttempt, error)
}

type StartService struct {
	Attempts         StartAttempts
	Replay           replay.Store
	Resolver         StartResolver
	Provider         StartProvider
	Gate             func(string, string) bool // subject, Workspace; compatibility and startup admission
	MaxAge           time.Duration             // zero selects the fixed 72-hour maximum
	Now              func() time.Time
	ReportPurgeError func(error) // required when running Maintain
}

const PurgeInterval = 6 * time.Hour
const purgeRetryInterval = time.Minute

func (s *StartService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *StartService) age() time.Duration {
	if s.MaxAge == 0 {
		return replay.MaximumAge
	}
	return s.MaxAge
}

func (s *StartService) valid() bool {
	return s != nil && s.Attempts != nil && s.Resolver != nil && s.Provider != nil && s.age() > 0 && s.age() <= replay.MaximumAge
}

func (s *StartService) paths(ctx context.Context, request replay.StartRun) (string, string, error) {
	root, workspaceID, err := s.Resolver.Workspace(ctx, request.SubjectID, request.WorkspaceID)
	if err != nil || root == "" || workspaceID != request.ProviderWorkspaceID {
		return "", "", ErrBlocked
	}
	if overlaps(root, s.Replay.Root) {
		return "", "", ErrBlocked
	}
	carrier, controllerID, generation, err := s.Resolver.Carrier(ctx, request.SubjectID, request.CredentialKey)
	if err != nil || carrier == "" || controllerID != request.ControllerID || generation != request.ControllerGeneration {
		return "", "", ErrBlocked
	}
	return root, carrier, nil
}

func overlaps(a, b string) bool {
	if !filepath.IsAbs(a) || !filepath.IsAbs(b) {
		return true
	}
	within := func(parent, child string) bool {
		owner, err := os.Stat(parent)
		if err != nil {
			return true
		}
		for current := child; ; current = filepath.Dir(current) {
			info, err := os.Stat(current)
			if err != nil {
				return true
			}
			if os.SameFile(owner, info) {
				return true
			}
			if filepath.Dir(current) == current {
				return false
			}
		}
	}
	return within(a, b) || within(b, a)
}

func startAttempt(request replay.StartRun, digest string) operation.MutationAttempt {
	return operation.MutationAttempt{OperationAttempt: operation.OperationAttempt{
		OperationID: request.OperationID, SubjectID: request.SubjectID, Kind: "StartRun", RequestSHA256: digest,
		ReplayKey: request.OperationID, ReplayAvailable: true, State: "pending", CreatedAt: request.CreatedAt,
		ControllerReferences: []operation.ControllerReference{{Role: "destination", CredentialKey: request.CredentialKey, ExpectedControllerID: request.ControllerID}},
	}, TargetRef: request.WorkspaceID, DeadlineAt: request.CreatedAt.Add(30 * time.Second), ReconciliationRoute: "exact_start_then_controller_list"}
}

func (s *StartService) Start(ctx context.Context, request replay.StartRun) (operation.MutationAttempt, error) {
	if !s.valid() || request.CreatedAt.IsZero() || request.CreatedAt.After(s.now().Add(time.Minute)) || s.now().Sub(request.CreatedAt) >= s.age() {
		return operation.MutationAttempt{}, ErrInvalid
	}
	_, digest, err := replay.Canonical(request)
	if err != nil {
		return operation.MutationAttempt{}, ErrInvalid
	}
	if s.Gate == nil || !s.Gate(request.SubjectID, request.WorkspaceID) {
		return operation.MutationAttempt{}, ErrBlocked
	}
	old, err := s.Attempts.Mutation(ctx, request.OperationID)
	if err == nil {
		if old.SubjectID != request.SubjectID || old.Kind != "StartRun" || old.RequestSHA256 != digest {
			return operation.MutationAttempt{}, ErrBlocked
		}
		return old, nil // Browser retry only reads the existing outcome.
	}
	if !errors.Is(err, operation.ErrNotFound) {
		return operation.MutationAttempt{}, err
	}
	root, carrier, err := s.paths(ctx, request)
	if err != nil {
		return operation.MutationAttempt{}, err
	}
	created := false
	if _, err := s.Replay.Put(request); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return operation.MutationAttempt{}, err
		}
		// A crash may leave the exact file before BeginMutation commits. Reuse
		// only matching canonical material; no provider call preceded Begin.
		if _, err := s.Replay.Get(request.OperationID, digest); err != nil {
			return operation.MutationAttempt{}, err
		}
	} else {
		created = true
	}
	attempt, dispatch, err := s.Attempts.BeginMutation(ctx, startAttempt(request, digest))
	if err != nil {
		if created && errors.Is(err, operation.ErrConflict) {
			if removeErr := s.Replay.Delete(request.OperationID); removeErr != nil {
				return operation.MutationAttempt{}, errors.Join(err, removeErr)
			}
		}
		return operation.MutationAttempt{}, err
	}
	if !dispatch {
		return attempt, nil
	}
	return s.dispatch(ctx, attempt, request, root, carrier)
}

func (s *StartService) dispatch(ctx context.Context, attempt operation.MutationAttempt, request replay.StartRun, root, carrier string) (operation.MutationAttempt, error) {
	if s.Gate == nil || !s.Gate(request.SubjectID, request.WorkspaceID) {
		_ = s.markUnknown(ctx, attempt.OperationID)
		return attempt, ErrBlocked
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	result, callErr := s.Provider.StartRun(callCtx, request, root, carrier)
	cancel()
	if callErr != nil || result.RunID == "" || result.WorkspaceID != request.ProviderWorkspaceID || result.ControllerID != request.ControllerID {
		if err := s.markUnknown(ctx, attempt.OperationID); err != nil {
			return attempt, err
		}
		attempt.State = "outcome_unknown"
		return attempt, ErrUnknown
	}
	if err := s.resolve(ctx, attempt.OperationID, result.RunID); err != nil {
		_ = s.markUnknown(ctx, attempt.OperationID)
		return attempt, err
	}
	attempt.State, attempt.OutcomeRef, attempt.ReplayAvailable, attempt.ReplayKey = "resolved", result.RunID, false, ""
	return attempt, nil
}

func (s *StartService) markUnknown(ctx context.Context, id string) error {
	err := s.Attempts.MarkOutcomeUnknown(context.WithoutCancel(ctx), id)
	if err != nil {
		old, getErr := s.Attempts.Mutation(context.WithoutCancel(ctx), id)
		if getErr == nil && old.State == "outcome_unknown" {
			return nil
		}
	}
	return err
}

func (s *StartService) resolve(ctx context.Context, id, runID string) error {
	if err := s.Replay.Delete(id); err != nil {
		return err
	}
	return s.Attempts.ResolveMutation(context.WithoutCancel(ctx), id, runID)
}

// Recover uses exact replay only while the original protected request remains.
// Missing or expired material permits only Controller-matched reconciliation.
func (s *StartService) Recover(ctx context.Context, id string) (operation.MutationAttempt, error) {
	if !s.valid() || id == "" {
		return operation.MutationAttempt{}, ErrInvalid
	}
	a, err := s.Attempts.Mutation(ctx, id)
	if err != nil || a.Kind != "StartRun" {
		return operation.MutationAttempt{}, ErrInvalid
	}
	if a.State == "resolved" {
		return a, nil
	}
	if a.State == "pending" {
		return a, ErrBlocked // An in-process dispatch may still be live.
	}
	if s.Gate == nil || !s.Gate(a.SubjectID, a.TargetRef) {
		return a, ErrBlocked
	}
	request, fileErr := s.Replay.Get(id, a.RequestSHA256)
	if a.ReplayAvailable && fileErr == nil && s.now().Sub(a.CreatedAt) < s.age() {
		if len(a.ControllerReferences) != 1 || request.SubjectID != a.SubjectID || request.WorkspaceID != a.TargetRef || request.ControllerID != a.ControllerReferences[0].ExpectedControllerID || request.CredentialKey != a.ControllerReferences[0].CredentialKey {
			return a, ErrBlocked
		}
		root, carrier, err := s.paths(ctx, request)
		if err != nil {
			return a, err
		}
		return s.dispatch(ctx, a, request, root, carrier)
	}
	if fileErr != nil && !errors.Is(fileErr, os.ErrNotExist) {
		return a, ErrBlocked
	}
	if a.ReplayAvailable {
		if err := s.Replay.Delete(id); err != nil {
			return a, err
		}
		if err := s.Attempts.ExpireReplay(ctx, id); err != nil {
			return a, err
		}
		a.ReplayAvailable, a.ReplayKey = false, ""
	} else if fileErr == nil {
		if err := s.Replay.Delete(id); err != nil {
			return a, err
		}
	}
	if len(a.ControllerReferences) != 1 || a.ControllerReferences[0].Role != "destination" {
		return a, ErrBlocked
	}
	root, workspaceID, err := s.Resolver.Workspace(ctx, a.SubjectID, a.TargetRef)
	if err != nil || root == "" || workspaceID == "" || overlaps(root, s.Replay.Root) {
		return a, ErrBlocked
	}
	ref := a.ControllerReferences[0]
	_, controllerID, generation, err := s.Resolver.Carrier(ctx, a.SubjectID, ref.CredentialKey)
	if err != nil || controllerID != ref.ExpectedControllerID {
		return a, ErrBlocked
	}
	probe := replay.StartRun{SubjectID: a.SubjectID, WorkspaceID: a.TargetRef, ProviderWorkspaceID: workspaceID,
		CredentialKey: ref.CredentialKey, ControllerID: controllerID, ControllerGeneration: generation, OperationID: a.OperationID}
	runs, err := s.Provider.ListRunsByController(ctx, probe, root)
	if err != nil || len(runs) != 1 || runs[0].RunID == "" || runs[0].WorkspaceID != workspaceID || runs[0].ControllerID != controllerID {
		return a, ErrUnknown
	}
	if err := s.Attempts.ResolveMutation(context.WithoutCancel(ctx), a.OperationID, runs[0].RunID); err != nil {
		return a, err
	}
	a.State, a.OutcomeRef = "resolved", runs[0].RunID
	return a, nil
}

// Purge is run at startup and at most six hours apart by the host. It never
// resolves an attempt, even when its replay material has expired.
func (s *StartService) Purge(ctx context.Context) error {
	if !s.valid() {
		return ErrInvalid
	}
	list, err := s.Attempts.Replayable(ctx)
	if err != nil {
		return err
	}
	for _, a := range list {
		if s.now().Sub(a.CreatedAt) < s.age() {
			continue
		}
		if err := s.Replay.Delete(a.OperationID); err != nil {
			return err
		}
		if a.State == "pending" {
			if err := s.markUnknown(ctx, a.OperationID); err != nil {
				return err
			}
		}
		if err := s.Attempts.ExpireReplay(ctx, a.OperationID); err != nil {
			return err
		}
	}
	_, err = s.Replay.PurgeExpired(s.now(), s.age())
	return err
}

// Maintain runs the first expiry pass before the host admits mutations and
// repeats it no less often than the first-release six-hour bound.
func (s *StartService) Maintain(ctx context.Context) error {
	ticker := time.NewTicker(PurgeInterval)
	defer ticker.Stop()
	return s.maintain(ctx, ticker.C, time.After)
}

func (s *StartService) maintain(ctx context.Context, ticks <-chan time.Time, after func(time.Duration) <-chan time.Time) error {
	if s == nil || s.ReportPurgeError == nil || ticks == nil || after == nil {
		return ErrInvalid
	}
	if err := s.Purge(ctx); err != nil {
		return err
	}
	var retry <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticks:
		case <-retry:
		}
		if err := s.Purge(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.ReportPurgeError(err)
			retry = after(purgeRetryInterval)
		} else {
			retry = nil
		}
	}
}
