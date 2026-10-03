package composition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/launch"
	"github.com/rootkernel/gul/internal/mutation"
	"github.com/rootkernel/gul/internal/operation"
	"github.com/rootkernel/gul/internal/replay"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/storage"
)

func creationKey(subject, attempt string) string {
	digest := sha256.Sum256([]byte("create\x00" + subject + "\x00" + attempt))
	return hex.EncodeToString(digest[:])
}

func (r *Runtime) CreateSession(ctx context.Context, subject, workspace, attempt string, choice launch.Choice) (session.CreationOutcome, error) {
	if r.starts == nil || r.Controllers == nil {
		return session.CreationOutcome{}, session.ErrUnavailable
	}
	if !session.ValidID(attempt) || !session.ValidID(workspace) {
		return session.CreationOutcome{}, session.ErrInvalid
	}
	r.createMu.Lock()
	defer r.createMu.Unlock()
	return r.createSession(ctx, subject, workspace, attempt, choice)
}

func (r *Runtime) createSession(ctx context.Context, subject, workspace, attempt string, choice launch.Choice) (session.CreationOutcome, error) {
	settings, err := json.Marshal(choice)
	if err != nil {
		return session.CreationOutcome{}, session.ErrInvalid
	}
	key := creationKey(subject, attempt)
	c, err := r.store.Presentation().Creation(ctx, key)
	if err == nil {
		if c.SubjectID != subject || c.WorkspaceID != workspace || c.ChoiceJSON != string(settings) {
			return session.CreationOutcome{}, session.ErrInvalid
		}
		// A browser retry observes the existing result; it never replays allocation.
		if a, e := r.store.Attempts().Mutation(ctx, key); e == nil {
			return r.finishCreation(ctx, c, a)
		} else if !errors.Is(e, operation.ErrNotFound) {
			return session.CreationOutcome{}, e
		}
	} else if !errors.Is(err, session.ErrNotFound) {
		return session.CreationOutcome{}, err
	}
	configuration, err := r.launch.Check(ctx, choice)
	if err != nil {
		return session.CreationOutcome{}, err
	}
	w, err := r.Workspaces.Revalidate(ctx, subject, workspace)
	if err != nil {
		return session.CreationOutcome{}, err
	}
	if c.OperationID == "" {
		credential, err := r.Controllers.Create(ctx, subject, configuration.PolicyName)
		if err != nil {
			return session.CreationOutcome{}, err
		}
		c = storage.Creation{OperationID: key, SubjectID: subject, WorkspaceID: workspace, AttemptID: attempt, ChoiceJSON: string(settings), ControllerBindingID: credential.BindingID, CreatedAt: time.Now().UTC()}
		if err = r.store.Presentation().ReserveCreation(ctx, c); err != nil {
			return session.CreationOutcome{}, err
		}
	}
	credential, err := r.store.Presentation().ControllerCredential(ctx, subject, c.ControllerBindingID)
	if err != nil {
		return session.CreationOutcome{}, err
	}
	lane := pb.ExecutionLane_EXECUTION_LANE_DEDICATED
	if configuration.Lane == "shared_readonly" {
		lane = pb.ExecutionLane_EXECUTION_LANE_SHARED_READONLY
	}
	assurance := pb.AssuranceLevel(pb.AssuranceLevel_value["ASSURANCE_LEVEL_"+strings.ToUpper(configuration.RequiredAssurance)])
	instructions := "Operate this Gul session under its explicit configuration. Observe pending user Interactions and publish Specialist results through Dolgorae."
	request := replay.StartRun{OperationID: key, SubjectID: subject, WorkspaceID: workspace, ProviderWorkspaceID: w.ProviderID, CredentialKey: credential.CredentialKey, ControllerID: credential.ExpectedControllerID, ControllerGeneration: credential.Generation, IdempotencyKey: key, ProfileName: configuration.ProfileName, ControlMode: int32(pb.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE), ExecutionLane: int32(lane), Purpose: int32(pb.PurposeKind_PURPOSE_KIND_INTERACTIVE), Model: &configuration.ModelID, Effort: &configuration.Effort, RequiredAssurance: int32(assurance), Instructions: &instructions, CreatedAt: c.CreatedAt}
	a, err := r.starts.Start(ctx, request)
	if errors.Is(err, mutation.ErrUnknown) {
		return session.CreationOutcome{Unknown: true}, nil
	}
	if err != nil {
		return session.CreationOutcome{}, err
	}
	return r.finishCreation(ctx, c, a)
}

func (r *Runtime) PendingCreations(ctx context.Context, subject, workspace string) ([]string, error) {
	if _, err := r.Workspaces.Revalidate(ctx, subject, workspace); err != nil {
		return nil, err
	}
	return r.store.Presentation().PendingCreationLabels(ctx, subject, workspace)
}

func (r *Runtime) RecoverCreation(ctx context.Context, subject, workspace, attempt string) (session.CreationOutcome, error) {
	if r.starts == nil || !session.ValidID(attempt) || !session.ValidID(workspace) {
		return session.CreationOutcome{}, session.ErrInvalid
	}
	r.createMu.Lock()
	defer r.createMu.Unlock()
	c, err := r.store.Presentation().Creation(ctx, creationKey(subject, attempt))
	if errors.Is(err, session.ErrNotFound) {
		return session.CreationOutcome{}, session.ErrNotFound
	}
	if err != nil {
		return session.CreationOutcome{}, err
	}
	if c.SubjectID != subject || c.WorkspaceID != workspace {
		return session.CreationOutcome{}, session.ErrNotFound
	}
	if _, e := r.store.Attempts().Mutation(ctx, c.OperationID); errors.Is(e, operation.ErrNotFound) {
		var choice launch.Choice
		if json.Unmarshal([]byte(c.ChoiceJSON), &choice) != nil {
			return session.CreationOutcome{}, session.ErrInvalid
		}
		return r.createSession(ctx, subject, workspace, attempt, choice)
	} else if e != nil {
		return session.CreationOutcome{}, e
	}
	a, err := r.starts.Recover(ctx, c.OperationID)
	if errors.Is(err, mutation.ErrUnknown) || errors.Is(err, mutation.ErrBlocked) {
		return session.CreationOutcome{Unknown: true}, nil
	}
	if err != nil {
		return session.CreationOutcome{}, err
	}
	return r.finishCreation(ctx, c, a)
}

func (r *Runtime) finishCreation(ctx context.Context, c storage.Creation, a operation.MutationAttempt) (session.CreationOutcome, error) {
	if a.SubjectID != c.SubjectID || a.Kind != "StartRun" || a.TargetRef != c.WorkspaceID {
		return session.CreationOutcome{}, session.ErrInvalid
	}
	if a.State != "resolved" || a.OutcomeRef == "" {
		return session.CreationOutcome{Unknown: true}, nil
	}
	b, err := r.store.Presentation().BindingByRun(ctx, c.SubjectID, a.OutcomeRef)
	if err == nil {
		if b.WorkspaceID != c.WorkspaceID {
			return session.CreationOutcome{}, session.ErrBindingConflict
		}
		return session.CreationOutcome{SessionID: b.ID}, nil
	}
	if !errors.Is(err, session.ErrNotFound) {
		return session.CreationOutcome{}, err
	}
	b, err = r.Sessions.BindPrimary(ctx, c.SubjectID, c.WorkspaceID, a.OutcomeRef, c.ControllerBindingID)
	if err != nil {
		return session.CreationOutcome{Unknown: true}, nil
	}
	return session.CreationOutcome{SessionID: b.ID}, nil
}

func (r *Runtime) Workspace(ctx context.Context, subject, id string) (string, string, error) {
	w, err := r.Workspaces.Revalidate(ctx, subject, id)
	return w.CanonicalRoot, w.ProviderID, err
}
func (r *Runtime) Carrier(ctx context.Context, subject, key string) (string, string, uint64, error) {
	c, err := r.store.Presentation().CredentialByKey(ctx, subject, key)
	if err != nil {
		return "", "", 0, err
	}
	v, err := r.Controllers.Resolve(ctx, subject, c.BindingID)
	return v.AbsolutePath, v.ControllerID, v.Generation, err
}
func (r *Runtime) startReady(subject, workspace string) bool {
	r.mu.Lock()
	blocked := r.creationPurgeBlocked
	r.mu.Unlock()
	if blocked {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	owner, err := r.store.Presentation().AccountSubject(ctx)
	return err == nil && owner == subject && r.probe.Check(ctx) == nil
}
