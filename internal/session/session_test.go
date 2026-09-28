package session_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/session/contractprovider"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type workspaces map[string]workspace.Attachment

func (w workspaces) Revalidate(_ context.Context, subject, id string) (workspace.Attachment, error) {
	entry := w[subject+"/"+id]
	if entry.ID == "" {
		return workspace.Attachment{}, workspace.ErrAttachmentNotFound
	}
	return entry, nil
}

type carriers map[string]session.Carrier

func (c carriers) Resolve(_ context.Context, subject, bindingID string) (session.Carrier, error) {
	carrier := c[subject+"/"+bindingID]
	if carrier.ControllerID == "" {
		return session.Carrier{}, session.ErrNotFound
	}
	return carrier, nil
}

type changingProvider struct {
	*scenario.Harness
	profile          string
	availability     publicv1.OrchestratedSessionAvailability
	recovery         publicv1.RecoveryClassification
	closeOperationID string
}

func (p *changingProvider) GetOrchestratedSession(ctx context.Context, request *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error) {
	response, err := p.Harness.GetOrchestratedSession(ctx, request)
	if err == nil && p.availability != publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_UNSPECIFIED {
		response.Session.Availability = p.availability
		response.Session.RecoveryClassification = p.recovery
		response.Session.CloseOperationId = nil
		if p.closeOperationID != "" {
			response.Session.CloseOperationId = &p.closeOperationID
		}
	}
	return response, err
}

type blockingProvider struct {
	session.Provider
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
	once    sync.Once
}

type memoryRepository struct{ bindings map[string]session.Binding }

type failingOperationReferenceRepository struct{ *memoryRepository }

func (failingOperationReferenceRepository) OperationReference(context.Context, session.Binding, string) (string, error) {
	return "", errors.New("local reference write failed")
}

type failedRepository struct{ session.Repository }

func (failedRepository) Binding(context.Context, string, string) (session.Binding, error) {
	return session.Binding{}, errors.New("local storage failed")
}

type deadlineRepository struct{ *memoryRepository }

func (r deadlineRepository) Binding(ctx context.Context, subject, id string) (session.Binding, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return session.Binding{}, errors.New("binding read exceeds refresh deadline")
	}
	return r.memoryRepository.Binding(ctx, subject, id)
}

type blockingBindingRepository struct {
	*memoryRepository
	calls atomic.Int32
}

func (r *blockingBindingRepository) Binding(ctx context.Context, subject, id string) (session.Binding, error) {
	if r.calls.Add(1) == 1 {
		<-ctx.Done()
		return session.Binding{}, ctx.Err()
	}
	return r.memoryRepository.Binding(ctx, subject, id)
}

type canceledProvider struct{}

func (canceledProvider) Snapshot(ctx context.Context, _ workspace.Attachment, _ string, _ session.Carrier) (session.Snapshot, error) {
	return session.Snapshot{}, ctx.Err()
}

type oversizedProvider struct{ fixedProvider }

func (p *oversizedProvider) Snapshot(ctx context.Context, attachment workspace.Attachment, runID string, carrier session.Carrier) (session.Snapshot, error) {
	snapshot, err := p.fixedProvider.Snapshot(ctx, attachment, runID, carrier)
	snapshot.Configuration.ProfileName = strings.Repeat("x", 270000)
	return snapshot, err
}

func (r *memoryRepository) InsertBinding(_ context.Context, binding session.Binding) (session.Binding, error) {
	return binding, nil
}
func (r *memoryRepository) Binding(_ context.Context, subject, id string) (session.Binding, error) {
	binding, ok := r.bindings[subject+"/"+id]
	if !ok {
		return session.Binding{}, session.ErrNotFound
	}
	return binding, nil
}
func (r *memoryRepository) ListBindings(context.Context, string, string) ([]session.Binding, error) {
	return nil, nil
}
func (r *memoryRepository) UpdateSnapshot(context.Context, session.Binding, session.Snapshot) error {
	return nil
}

func (p *blockingProvider) Snapshot(ctx context.Context, attachment workspace.Attachment, runID string, carrier session.Carrier) (session.Snapshot, error) {
	p.calls.Add(1)
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		return session.Snapshot{}, errors.New("refresh deadline is unbounded")
	}
	p.once.Do(func() { close(p.started) })
	select {
	case <-ctx.Done():
		return session.Snapshot{}, ctx.Err()
	case <-p.release:
		return p.Provider.Snapshot(ctx, attachment, runID, carrier)
	}
}

func (p *changingProvider) GetRun(ctx context.Context, request *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	response, err := p.Harness.GetRun(ctx, request)
	if err == nil && p.profile != "" {
		configuration := response.Run.Configuration
		configuration.ProfileName = p.profile
		configuration.Purpose = publicv1.PurposeKind_PURPOSE_KIND_REVIEW
		label := "Provider review"
		configuration.PurposeLabel = &label
		configuration.ModelId = "provider-model"
		configuration.DefaultEffort = "high"
		configuration.RequiredCapabilities = []string{"review"}
		configuration.InstructionContract = &publicv1.InstructionContractProjection{SchemaId: "instructions/v1", CommonPrefixVersion: 1}
		configuration.ControllerInstructionsByteLength = 12
		configuration.ControllerInstructionsSha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}
	return response, err
}

func TestPrimaryBindingsAndAuthoritativeState(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, "gul.sqlite")
	store, err := storage.Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	for _, subject := range []string{"owner", "other"} {
		if err := store.Auth().CreateAccount(ctx, subject, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	h := scenario.New(time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC))
	provider := &changingProvider{Harness: h}
	ws := workspaces{}
	cs := carriers{}
	var bindings []session.Binding
	for index := 1; index <= 3; index++ {
		workspaceID, path := "workspace-a", "/workspace/a"
		if index == 3 {
			workspaceID, path = "workspace-b", "/workspace/b"
		}
		inspected, err := h.InspectWorkspace(ctx, &publicv1.InspectWorkspaceRequest{AbsolutePath: path})
		if err != nil {
			t.Fatal(err)
		}
		entry := workspace.Attachment{SubjectID: "owner", ID: workspaceID, CanonicalRoot: path,
			ProviderID: inspected.GetWorkspaceId(), FileDevice: "1", FileInode: workspaceID, DisplayName: workspaceID}
		ws["owner/"+workspaceID] = entry
		if index == 1 || index == 3 {
			if err := store.Presentation().CreateAttachment(ctx, entry); err != nil {
				t.Fatal(err)
			}
		}
		controllerID := "controller-" + string(rune('0'+index))
		carrierPath := "/carrier/" + controllerID
		if err := h.RegisterController(scenario.ControllerSpec{ID: controllerID, Generation: 1, CarrierPath: carrierPath,
			OrchestrationLaunch: true, PolicyName: "preprovisioned"}); err != nil {
			t.Fatal(err)
		}
		cs["owner/"+controllerID] = session.Carrier{AbsolutePath: carrierPath, ControllerID: controllerID, Generation: 1}
		started, err := h.StartRun(ctx, &publicv1.StartRunRequest{Workspace: &publicv1.WorkspaceRef{AbsolutePath: path, ExpectedWorkspaceId: entry.ProviderID},
			Controller:     &publicv1.ControllerCarrierRef{AbsoluteFilePath: carrierPath, ExpectedControllerId: controllerID, ExpectedControllerGeneration: 1},
			IdempotencyKey: controllerID, ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
			ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
		if err != nil {
			t.Fatal(err)
		}
		svc := session.NewService(ws, store.Presentation(), contractprovider.Provider{Port: provider}, cs)
		binding, err := svc.BindPrimary(ctx, "owner", workspaceID, started.GetRun().GetRunId(), controllerID)
		if err != nil {
			t.Fatal(err)
		}
		bindings = append(bindings, binding)
		if index == 1 {
			if err := store.Presentation().RenameDirectSession(ctx, "owner", binding.ID, "Custom session"); err != nil {
				t.Fatal(err)
			}
		}
		again, err := svc.BindPrimary(ctx, "owner", workspaceID, binding.RunID, controllerID)
		if err != nil || again.ID != binding.ID {
			t.Fatalf("duplicate binding = %+v, %v", again, err)
		}
		if index == 1 {
			entry, err := store.Presentation().DirectSession(ctx, "owner", binding.ID)
			if err != nil || entry.DisplayName != "Custom session" {
				t.Fatalf("rediscovered presentation = %+v, %v", entry, err)
			}
		}
	}
	svc := session.NewService(ws, store.Presentation(), contractprovider.Provider{Port: provider}, cs)
	if _, err := (contractprovider.Provider{Port: provider}).Snapshot(ctx, ws["owner/workspace-a"], bindings[0].RunID,
		session.Carrier{AbsolutePath: "/carrier/other", ControllerID: "other", Generation: 1}); err == nil {
		t.Fatal("wrong controller read accepted")
	}
	if err := h.RegisterController(scenario.ControllerSpec{ID: "plain", Generation: 1, CarrierPath: "/carrier/plain"}); err != nil {
		t.Fatal(err)
	}
	plain, err := h.StartRun(ctx, &publicv1.StartRunRequest{Workspace: &publicv1.WorkspaceRef{AbsolutePath: "/workspace/a", ExpectedWorkspaceId: ws["owner/workspace-a"].ProviderID},
		Controller:     &publicv1.ControllerCarrierRef{AbsoluteFilePath: "/carrier/plain", ExpectedControllerId: "plain", ExpectedControllerGeneration: 1},
		IdempotencyKey: "plain", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE,
		ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	cs["owner/plain"] = session.Carrier{AbsolutePath: "/carrier/plain", ControllerID: "plain", Generation: 1}
	if _, err := svc.BindPrimary(ctx, "owner", "workspace-a", plain.GetRun().GetRunId(), "plain"); err == nil {
		t.Fatal("non-session root became Gul session")
	}
	var group sync.WaitGroup
	for _, binding := range bindings {
		group.Add(1)
		go func(id string) {
			defer group.Done()
			state, err := svc.GetExecutionState(ctx, "owner", id)
			if err != nil || state.Freshness != "fresh" {
				t.Errorf("concurrent session %s = %+v, %v", id, state, err)
			}
		}(binding.ID)
	}
	group.Wait()
	list, err := svc.List(ctx, "owner", "workspace-a")
	if err != nil || len(list) != 2 {
		t.Fatalf("workspace-a sessions = %+v, %v", list, err)
	}
	list, err = svc.List(ctx, "owner", "workspace-b")
	if err != nil || len(list) != 1 {
		t.Fatalf("workspace-b sessions = %+v, %v", list, err)
	}
	if list, err := svc.List(ctx, "other", "workspace-a"); err != nil || len(list) != 0 {
		t.Fatalf("cross-subject list = %+v, %v", list, err)
	}
	if _, _, err := svc.Open(ctx, "other", bindings[0].ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("cross-subject open = %v", err)
	}
	state, err := svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || state.Freshness != "fresh" || state.Snapshot == nil || state.Snapshot.Counts.NonretiredMembers != 0 || state.Snapshot.Composition != "standalone_primary" {
		t.Fatalf("initial state = %+v, %v", state, err)
	}
	initialVersion := state.StateVersion
	unchanged, err := svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || unchanged.StateVersion != initialVersion {
		t.Fatalf("unchanged state version = %+v, %v", unchanged, err)
	}
	provider.profile = "changed-by-provider"
	state, err = svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || state.Snapshot.Configuration.ProfileName != "changed-by-provider" || state.Snapshot.Configuration.PurposeLabel != "Provider review" ||
		state.Snapshot.Configuration.ModelID != "provider-model" || state.Snapshot.Configuration.DefaultEffort != "high" ||
		len(state.Snapshot.Configuration.RequiredCapabilities) != 1 || state.Snapshot.Configuration.InstructionSchema != "instructions/v1" ||
		state.Snapshot.Configuration.InstructionSHA256 == "" || state.StateVersion == initialVersion {
		t.Fatalf("provider configuration refresh = %+v, %v", state, err)
	}
	initialVersion = state.StateVersion
	if _, err := h.SpawnSpecialist(bindings[0].RunID, "reviewer"); err != nil {
		t.Fatal(err)
	}
	state, err = svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || state.Snapshot.Counts.NonretiredMembers != 1 || len(state.Snapshot.Members) != 1 || state.StateVersion == initialVersion {
		t.Fatalf("observed specialist = %+v, %v", state, err)
	}
	for index := 0; index < 256; index++ {
		if _, err := h.SpawnSpecialist(bindings[0].RunID, fmt.Sprintf("observer-%03d", index)); err != nil {
			t.Fatal(err)
		}
	}
	state, err = svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || state.Snapshot.Counts.NonretiredMembers != 257 || len(state.Snapshot.Members) != 256 || !state.Snapshot.MembersTruncated {
		t.Fatalf("bounded member observations = %+v, %v", state, err)
	}
	for index := 1; index < len(state.Snapshot.Members); index++ {
		if state.Snapshot.Members[index-1].RunID >= state.Snapshot.Members[index].RunID {
			t.Fatal("member observations are not sorted")
		}
	}
	if err := h.FaultNext("GetOrchestratedSession", scenario.BeforeCommit, errors.New("offline")); err != nil {
		t.Fatal(err)
	}
	state, err = svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || state.Freshness != "stale" || state.StateVersion == "" {
		t.Fatalf("stale state = %+v, %v", state, err)
	}
	for _, availability := range []publicv1.OrchestratedSessionAvailability{
		publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_UNAVAILABLE,
	} {
		provider.availability = availability
		degraded, err := svc.GetExecutionState(ctx, "owner", bindings[0].ID)
		if err != nil || degraded.Freshness != "stale" || degraded.StateVersion != state.StateVersion {
			t.Fatalf("provider availability %s = %+v, %v", availability, degraded, err)
		}
		noCache := session.NewService(ws, store.Presentation(), contractprovider.Provider{Port: provider}, cs)
		unavailable, err := noCache.GetExecutionState(ctx, "owner", bindings[0].ID)
		if err != nil || unavailable.Freshness != "unavailable" || unavailable.Snapshot != nil {
			t.Fatalf("provider availability without cache %s = %+v, %v", availability, unavailable, err)
		}
	}
	provider.availability = publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_RECOVERY_REQUIRED
	provider.recovery = publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_OUTCOME_UNKNOWN
	provider.closeOperationID = "provider-close-operation"
	recovered, err := svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || recovered.Freshness != "fresh" || recovered.Snapshot == nil || recovered.Snapshot.Recovery != "outcome_unknown" || recovered.CloseOperationRef == "" || recovered.CloseOperationRef == provider.closeOperationID {
		t.Fatalf("typed recovery snapshot = %+v, %v", recovered, err)
	}
	restarted := session.NewService(ws, store.Presentation(), contractprovider.Provider{Port: provider}, cs)
	again, err := restarted.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || again.CloseOperationRef != recovered.CloseOperationRef {
		t.Fatalf("operation reference after restart = %+v, %v", again, err)
	}
	provider.recovery = publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE
	if _, err := svc.GetExecutionState(ctx, "owner", bindings[0].ID); !errors.Is(err, session.ErrInvalidProjection) {
		t.Fatalf("contradictory recovery availability = %v", err)
	}
	provider.closeOperationID = ""
	provider.availability = publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_UNSPECIFIED
	if err := h.FaultNext("GetOrchestratedSession", scenario.BeforeCommit,
		connect.NewError(connect.CodePermissionDenied, errors.New("unauthorized"))); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetExecutionState(ctx, "owner", bindings[0].ID); !errors.Is(err, session.ErrInvalidProjection) {
		t.Fatalf("semantic provider error = %v", err)
	}
	if err := h.FaultNext("ListRuns", scenario.BeforeCommit, errors.New("list offline")); err != nil {
		t.Fatal(err)
	}
	state, err = svc.GetExecutionState(ctx, "owner", bindings[0].ID)
	if err != nil || state.Freshness != "fresh" || state.Snapshot == nil || state.Snapshot.Counts.NonretiredMembers != 257 || len(state.Snapshot.Members) != 0 {
		t.Fatalf("aggregate survives ListRuns failure = %+v, %v", state, err)
	}
	conflict := bindings[0]
	conflict.ID = "different-local-session"
	conflict.ControllerBindingID = "different-controller"
	if _, err := store.Presentation().InsertBinding(ctx, conflict); !errors.Is(err, session.ErrBindingConflict) {
		t.Fatalf("conflicting Primary rebind = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	reopened := session.NewService(ws, store.Presentation(), contractprovider.Provider{Port: provider}, cs)
	binding, state, err := reopened.Open(ctx, "owner", bindings[2].ID)
	if err != nil || binding.RunID != bindings[2].RunID || state.Freshness != "fresh" || binding.Configuration.ProfileName != "changed-by-provider" {
		t.Fatalf("reopened binding = %+v, %+v, %v", binding, state, err)
	}
}

func TestSameSessionReadCoalescesAndFollowerCanCancel(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(ctx, filepath.Join(dir, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Auth().CreateAccount(ctx, "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	entry := workspace.Attachment{SubjectID: "owner", ID: "workspace", CanonicalRoot: "/workspace", ProviderID: "provider-workspace", FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}
	if err := store.Presentation().CreateAttachment(ctx, entry); err != nil {
		t.Fatal(err)
	}
	base := &fixedProvider{}
	carrier := carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller", Generation: 1}}
	ws := workspaces{"owner/workspace": entry}
	binding, err := session.NewService(ws, store.Presentation(), base, carrier).BindPrimary(ctx, "owner", "workspace", "run", "controller")
	if err != nil {
		t.Fatal(err)
	}
	blocked := &blockingProvider{Provider: base, started: make(chan struct{}), release: make(chan struct{})}
	svc := session.NewService(ws, store.Presentation(), blocked, carrier)
	result := make(chan error, 2)
	go func() { _, err := svc.GetExecutionState(ctx, "owner", binding.ID); result <- err }()
	<-blocked.started
	followerCtx, cancel := context.WithCancel(ctx)
	cancelled := make(chan error, 1)
	go func() { _, err := svc.GetExecutionState(followerCtx, "owner", binding.ID); cancelled <- err }()
	cancel()
	if err := <-cancelled; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled follower = %v", err)
	}
	go func() { _, err := svc.GetExecutionState(ctx, "owner", binding.ID); result <- err }()
	time.Sleep(30 * time.Millisecond)
	close(blocked.release)
	for range 2 {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	if calls := blocked.calls.Load(); calls != 1 {
		t.Fatalf("coalesced provider calls = %d", calls)
	}
	blocked = &blockingProvider{Provider: base, started: make(chan struct{}), release: make(chan struct{})}
	svc = session.NewService(ws, store.Presentation(), blocked, carrier)
	leaderCtx, cancelLeader := context.WithCancel(ctx)
	leaderResult := make(chan error, 1)
	go func() { _, err := svc.GetExecutionState(leaderCtx, "owner", binding.ID); leaderResult <- err }()
	<-blocked.started
	followerResult := make(chan error, 1)
	go func() { _, err := svc.GetExecutionState(ctx, "owner", binding.ID); followerResult <- err }()
	time.Sleep(30 * time.Millisecond)
	cancelLeader()
	if err := <-leaderResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled leader = %v", err)
	}
	close(blocked.release)
	if err := <-followerResult; err != nil {
		t.Fatalf("healthy follower after leader cancellation = %v", err)
	}
	if calls := blocked.calls.Load(); calls != 1 {
		t.Fatalf("leader-canceled provider calls = %d", calls)
	}
	drift := session.NewService(ws, store.Presentation(), &fixedProvider{sessionID: "different-provider-session"}, carrier)
	if _, err := drift.GetExecutionState(ctx, "owner", binding.ID); !errors.Is(err, session.ErrInvalidProjection) {
		t.Fatalf("provider session identity drift = %v", err)
	}
}

func TestStaleCacheEvictsOldestOf257Sessions(t *testing.T) {
	ctx := t.Context()
	repo := &memoryRepository{bindings: make(map[string]session.Binding)}
	for index := 0; index < 257; index++ {
		id := fmt.Sprintf("session-%03d", index)
		repo.bindings["owner/"+id] = session.Binding{SubjectID: "owner", ID: id, WorkspaceID: "workspace", RunID: id,
			ControllerBindingID: "controller", ProviderSessionID: "provider-session"}
	}
	entry := workspace.Attachment{SubjectID: "owner", ID: "workspace", ProviderID: "provider-workspace"}
	provider := &fixedProvider{}
	svc := session.NewService(workspaces{"owner/workspace": entry}, repo, provider,
		carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller", Generation: 1}})
	for index := 0; index < 257; index++ {
		id := fmt.Sprintf("session-%03d", index)
		state, err := svc.GetExecutionState(ctx, "owner", id)
		if err != nil || state.Freshness != "fresh" {
			t.Fatalf("cache fill %s = %+v, %v", id, state, err)
		}
	}
	provider.offline = true
	oldest, err := svc.GetExecutionState(ctx, "owner", "session-000")
	if err != nil || oldest.Freshness != "unavailable" {
		t.Fatalf("evicted state = %+v, %v", oldest, err)
	}
	newest, err := svc.GetExecutionState(ctx, "owner", "session-256")
	if err != nil || newest.Freshness != "stale" {
		t.Fatalf("retained state = %+v, %v", newest, err)
	}
}

func TestOperationReferencePersistenceFailureDoesNotReturnCachedState(t *testing.T) {
	repo := &memoryRepository{bindings: map[string]session.Binding{
		"owner/session": {SubjectID: "owner", ID: "session", WorkspaceID: "workspace", RunID: "run", ControllerBindingID: "controller", ProviderSessionID: "provider-session"},
	}}
	provider := &fixedProvider{}
	svc := session.NewService(workspaces{"owner/workspace": {ID: "workspace", ProviderID: "provider-workspace"}},
		failingOperationReferenceRepository{repo}, provider,
		carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller", Generation: 1}})
	if state, err := svc.GetExecutionState(t.Context(), "owner", "session"); err != nil || state.Freshness != "fresh" {
		t.Fatalf("initial snapshot = %+v, %v", state, err)
	}
	provider.closeOperationID = "provider-close"
	state, err := svc.GetExecutionState(t.Context(), "owner", "session")
	if !errors.Is(err, session.ErrPersistenceUnavailable) || state.Snapshot != nil || state.Freshness != "" {
		t.Fatalf("reference write exposed cached state = %+v, %v", state, err)
	}
}

func TestCancelledBindPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	svc := session.NewService(workspaces{"owner/workspace": {ID: "workspace", ProviderID: "provider-workspace"}},
		&memoryRepository{}, canceledProvider{}, carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller"}})
	if _, err := svc.BindPrimary(ctx, "owner", "workspace", "run", "controller"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled binding = %v", err)
	}
}

func TestRepositoryFailureIsPersistenceClass(t *testing.T) {
	svc := session.NewService(workspaces{}, failedRepository{}, &fixedProvider{}, carriers{})
	if _, err := svc.GetExecutionState(t.Context(), "owner", "session"); !errors.Is(err, session.ErrPersistenceUnavailable) {
		t.Fatalf("repository failure = %v", err)
	}
}

func TestRefreshDeadlineIncludesBindingRead(t *testing.T) {
	entry := workspace.Attachment{SubjectID: "owner", ID: "workspace", ProviderID: "provider-workspace"}
	repo := deadlineRepository{&memoryRepository{bindings: map[string]session.Binding{
		"owner/session": {SubjectID: "owner", ID: "session", WorkspaceID: "workspace", RunID: "run",
			ControllerBindingID: "controller", ProviderSessionID: "provider-session"},
	}}}
	svc := session.NewService(workspaces{"owner/workspace": entry}, repo, &fixedProvider{},
		carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller", Generation: 1}})
	state, err := svc.GetExecutionState(t.Context(), "owner", "session")
	if err != nil || state.Freshness != "fresh" {
		t.Fatalf("binding read outside refresh deadline = %+v, %v", state, err)
	}
}

func TestTimedOutBindingReadReleasesRefreshFlight(t *testing.T) {
	entry := workspace.Attachment{SubjectID: "owner", ID: "workspace", ProviderID: "provider-workspace"}
	repo := &blockingBindingRepository{memoryRepository: &memoryRepository{bindings: map[string]session.Binding{
		"owner/session": {SubjectID: "owner", ID: "session", WorkspaceID: "workspace", RunID: "run",
			ControllerBindingID: "controller", ProviderSessionID: "provider-session"},
	}}}
	svc := session.NewService(workspaces{"owner/workspace": entry}, repo, &fixedProvider{},
		carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller", Generation: 1}})
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Second)
	defer cancel()
	if _, err := svc.GetExecutionState(ctx, "owner", "session"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked binding read = %v", err)
	}
	state, err := svc.GetExecutionState(t.Context(), "owner", "session")
	if err != nil || state.Freshness != "fresh" || repo.calls.Load() != 2 {
		t.Fatalf("refresh after timed-out binding = %+v, %v; binding calls = %d", state, err, repo.calls.Load())
	}
}

func TestOversizedProviderSnapshotIsRejectedBeforePersistence(t *testing.T) {
	ctx := t.Context()
	svc := session.NewService(workspaces{"owner/workspace": {ID: "workspace", ProviderID: "provider-workspace"}},
		&memoryRepository{}, &oversizedProvider{}, carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller"}})
	if _, err := svc.BindPrimary(ctx, "owner", "workspace", "run", "controller"); !errors.Is(err, session.ErrInvalidProjection) {
		t.Fatalf("oversized provider projection = %v", err)
	}
}

type fixedProvider struct {
	offline          bool
	sessionID        string
	closeOperationID string
}

func (p *fixedProvider) Snapshot(ctx context.Context, attachment workspace.Attachment, runID string, _ session.Carrier) (session.Snapshot, error) {
	if p.offline {
		return session.Snapshot{}, errors.New("provider offline")
	}
	id := p.sessionID
	if id == "" {
		id = "provider-session"
	}
	return session.Snapshot{ProviderSessionID: id, PrimaryRunID: runID, ProviderWorkspaceID: attachment.ProviderID,
		Lifecycle: "active", Composition: "standalone_primary", ApprovalPolicy: "user_approval_required", CloseProgress: "none", Recovery: "none",
		AggregateRevision: 1, RunRevision: 1, CloseOperationID: p.closeOperationID, ObservedAt: time.Now().UTC()}, nil
}

func (r *memoryRepository) OperationReference(_ context.Context, b session.Binding, providerID string) (string, error) {
	return "opaque-" + b.ID, nil
}
