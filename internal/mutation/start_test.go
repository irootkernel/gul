package mutation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/replay"
	"github.com/rootkernel/gul/internal/storage"
)

type startResolver struct{}

type rootedResolver struct{ root string }

func (r rootedResolver) Workspace(context.Context, string, string) (string, string, error) {
	return r.root, "provider-workspace", nil
}
func (r rootedResolver) Carrier(ctx context.Context, subject, key string) (string, string, uint64, error) {
	return (startResolver{}).Carrier(ctx, subject, key)
}

func (startResolver) Workspace(_ context.Context, subject, id string) (string, string, error) {
	if subject != "owner" || id != "workspace" {
		return "", "", ErrBlocked
	}
	return "/trusted/workspace", "provider-workspace", nil
}
func (startResolver) Carrier(_ context.Context, subject, key string) (string, string, uint64, error) {
	if subject != "owner" || key != "controllers/destination" {
		return "", "", 0, ErrBlocked
	}
	return "/trusted/carrier", "controller", 1, nil
}

type startProvider struct {
	calls, effects int
	lostResponse   bool
	run            StartResult
}

func (p *startProvider) StartRun(_ context.Context, request replay.StartRun, root, carrier string) (StartResult, error) {
	p.calls++
	if root != "/trusted/workspace" || carrier != "/trusted/carrier" || request.IdempotencyKey != "stable-key" {
		return StartResult{}, ErrBlocked
	}
	if p.run.RunID == "" {
		p.effects++
		p.run = StartResult{RunID: "run-1", WorkspaceID: request.ProviderWorkspaceID, ControllerID: request.ControllerID}
	}
	if p.lostResponse {
		p.lostResponse = false
		return StartResult{}, context.DeadlineExceeded
	}
	return p.run, nil
}
func (p *startProvider) ListRunsByController(_ context.Context, request replay.StartRun, root string) ([]StartResult, error) {
	if root != "/trusted/workspace" || request.ControllerID != "controller" || p.run.RunID == "" {
		return nil, nil
	}
	return []StartResult{p.run}, nil
}

func startFixture(now time.Time) replay.StartRun {
	return replay.StartRun{OperationID: "attempt_1", SubjectID: "owner", WorkspaceID: "workspace", ProviderWorkspaceID: "provider-workspace",
		CredentialKey: "controllers/destination", ControllerID: "controller", ControllerGeneration: 1, IdempotencyKey: "stable-key",
		ProfileName: "profile", ControlMode: 1, ExecutionLane: 1, Purpose: 1, RequiredAssurance: 1, CreatedAt: now}
}

func startStore(t *testing.T) (*storage.Store, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	replayRoot := filepath.Join(root, "replay")
	if err := os.Mkdir(replayRoot, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "gul.sqlite")
	db, err := storage.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path, replayRoot
}

func TestLostStartResponseReplaysExactKeyAfterRestart(t *testing.T) {
	db, path, root := startStore(t)
	now := time.Now().UTC()
	provider := &startProvider{lostResponse: true}
	service := StartService{Attempts: db.Attempts(), Replay: replay.Store{Root: root}, Resolver: startResolver{}, Provider: provider, Gate: func(string, string) bool { return true }, Now: func() time.Time { return now }}
	request := startFixture(now)
	a, err := service.Start(t.Context(), request)
	if !errors.Is(err, ErrUnknown) || a.State != "outcome_unknown" || provider.calls != 1 || provider.effects != 1 {
		t.Fatalf("lost response = %+v, %v; calls=%d effects=%d", a, err, provider.calls, provider.effects)
	}
	// The browser's identical retry reads the existing attempt; it does not
	// initiate a second provider call or infer non-acceptance from history.
	a, err = service.Start(t.Context(), request)
	if err != nil || a.State != "outcome_unknown" || provider.calls != 1 {
		t.Fatalf("browser retry = %+v, %v; calls=%d", a, err, provider.calls)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service.Attempts = db.Attempts()
	a, err = service.Recover(t.Context(), request.OperationID)
	if err != nil || a.State != "resolved" || a.OutcomeRef != "run-1" || provider.calls != 2 || provider.effects != 1 {
		t.Fatalf("exact recovery = %+v, %v; calls=%d effects=%d", a, err, provider.calls, provider.effects)
	}
	if _, err := os.Lstat(filepath.Join(root, request.OperationID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolved material remains: %v", err)
	}
}

func TestExpiredStartNeverReplaysAndOnlyAuthoritativeListResolves(t *testing.T) {
	db, _, root := startStore(t)
	now := time.Now().UTC()
	provider := &startProvider{lostResponse: true}
	service := StartService{Attempts: db.Attempts(), Replay: replay.Store{Root: root}, Resolver: startResolver{}, Provider: provider, Gate: func(string, string) bool { return true }, Now: func() time.Time { return now }, MaxAge: time.Hour}
	request := startFixture(now)
	if _, err := service.Start(t.Context(), request); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if err := service.Purge(t.Context()); err != nil {
		t.Fatal(err)
	}
	a, err := db.Attempts().Mutation(t.Context(), request.OperationID)
	if err != nil || a.ReplayAvailable || a.State != "outcome_unknown" {
		t.Fatalf("expired attempt = %+v, %v", a, err)
	}
	resolved, err := service.Recover(t.Context(), request.OperationID)
	if err != nil || resolved.OutcomeRef != "run-1" || provider.calls != 1 || provider.effects != 1 {
		t.Fatalf("secondary reconciliation = %+v, %v; calls=%d effects=%d", resolved, err, provider.calls, provider.effects)
	}
}

func TestVersionDriftBlocksStartReplayUntilCompatibilityReturns(t *testing.T) {
	db, _, root := startStore(t)
	provider := &startProvider{lostResponse: true}
	compatible := true
	service := StartService{Attempts: db.Attempts(), Replay: replay.Store{Root: root}, Resolver: startResolver{}, Provider: provider,
		Gate: func(string, string) bool { return compatible }}
	request := startFixture(time.Now().UTC())
	if _, err := service.Start(t.Context(), request); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	compatible = false
	if _, err := service.Recover(t.Context(), request.OperationID); !errors.Is(err, ErrBlocked) || provider.calls != 1 {
		t.Fatalf("version drift replayed: %v; calls=%d", err, provider.calls)
	}
	compatible = true
	if a, err := service.Recover(t.Context(), request.OperationID); err != nil || a.OutcomeRef != "run-1" || provider.effects != 1 {
		t.Fatalf("compatible recovery = %+v, %v; effects=%d", a, err, provider.effects)
	}
}

func TestReplayRootInsideWorkspaceBlocksAllocation(t *testing.T) {
	db, _, root := startStore(t)
	provider := &startProvider{}
	service := StartService{Attempts: db.Attempts(), Replay: replay.Store{Root: root}, Resolver: rootedResolver{root: filepath.Dir(root)},
		Provider: provider, Gate: func(string, string) bool { return true }}
	if _, err := service.Start(t.Context(), startFixture(time.Now().UTC())); !errors.Is(err, ErrBlocked) || provider.calls != 0 {
		t.Fatalf("unsafe replay placement = %v, calls=%d", err, provider.calls)
	}
}
