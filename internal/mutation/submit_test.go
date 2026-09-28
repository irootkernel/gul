package mutation

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/storage"
)

type submitProvider struct {
	calls, effects int
	lostResponse   bool
	proof          SubmitEvidence
}

func (p *submitProvider) SubmitTurn(_ context.Context, request SubmitRequest) (string, error) {
	p.calls++
	if request.IdempotencyKey != "stable-submit-key" || string(request.Canonical) != "prompt-secret-and-image-secret" {
		return "", ErrBlocked
	}
	if p.effects == 0 {
		p.effects++
	}
	if p.lostResponse {
		p.lostResponse = false
		return "", context.DeadlineExceeded
	}
	return "turn-1", nil
}
func (p *submitProvider) ReconcileTurn(_ context.Context, _ SubmitReference) (SubmitEvidence, error) {
	return p.proof, nil
}

func submitFixture() SubmitRequest {
	return SubmitRequest{OperationID: "submit_1", SubjectID: "owner", RunID: "run-1", BindingID: "binding-1",
		ControllerID: "controller", IdempotencyKey: "stable-submit-key", Canonical: []byte("prompt-secret-and-image-secret"), CreatedAt: time.Now().UTC()}
}

func TestSubmitLostResponseUsesOnlySameProcessExactRequest(t *testing.T) {
	db, path, _ := startStore(t)
	provider := &submitProvider{lostResponse: true, proof: SubmitEvidence{Status: "unknown"}}
	s := SubmitService{Attempts: db.Attempts(), Provider: provider, Gate: func(string, string) bool { return true }}
	request := submitFixture()
	a, err := s.Submit(t.Context(), request)
	if !errors.Is(err, ErrUnknown) || a.State != "outcome_unknown" || provider.calls != 1 || provider.effects != 1 {
		t.Fatalf("lost submit = %+v, %v; calls=%d effects=%d", a, err, provider.calls, provider.effects)
	}
	if a.ReplayAvailable || a.ReplayKey != "" {
		t.Fatalf("prompt acquired durable replay metadata: %+v", a)
	}
	if _, err := s.Submit(t.Context(), request); err != nil || provider.calls != 1 {
		t.Fatalf("browser retry transmitted: %v; calls=%d", err, provider.calls)
	}
	for _, filename := range []string{path, path + "-wal"} {
		b, err := os.ReadFile(filename)
		if err == nil && strings.Contains(string(b), "prompt-secret") {
			t.Fatalf("prompt persisted in %s", filename)
		}
	}
	a, err = s.RecoverTurn(t.Context(), request.OperationID)
	if err != nil || a.OutcomeRef != "turn-1" || provider.calls != 2 || provider.effects != 1 {
		t.Fatalf("same-process replay = %+v, %v; calls=%d effects=%d", a, err, provider.calls, provider.effects)
	}
}

func TestRestartWithoutSubmitMaterialStaysUnknownUntilExactProof(t *testing.T) {
	db, path, _ := startStore(t)
	provider := &submitProvider{lostResponse: true, proof: SubmitEvidence{Status: "unknown"}}
	s := SubmitService{Attempts: db.Attempts(), Provider: provider, Gate: func(string, string) bool { return true }}
	request := submitFixture()
	if _, err := s.Submit(t.Context(), request); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	s.DropProcessMemory()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = SubmitService{Attempts: db.Attempts(), Provider: provider, Gate: func(string, string) bool { return true }}
	if _, err := s.RecoverTurn(t.Context(), request.OperationID); !errors.Is(err, ErrUnknown) || provider.calls != 1 {
		t.Fatalf("restart inferred nonacceptance: %v; calls=%d", err, provider.calls)
	}
	provider.proof = SubmitEvidence{Status: "accepted", TurnID: "turn-1"}
	a, err := s.RecoverTurn(t.Context(), request.OperationID)
	if err != nil || a.State != "resolved" || a.OutcomeRef != "turn-1" || provider.calls != 1 {
		t.Fatalf("provider proof = %+v, %v; calls=%d", a, err, provider.calls)
	}
}

func TestObservationGapAndVersionDriftCannotAuthorizeSubmitResend(t *testing.T) {
	db, _, _ := startStore(t)
	provider := &submitProvider{lostResponse: true, proof: SubmitEvidence{Status: "unknown"}}
	compatible := true
	s := SubmitService{Attempts: db.Attempts(), Provider: provider, Gate: func(string, string) bool { return compatible }}
	request := submitFixture()
	if _, err := s.Submit(t.Context(), request); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	compatible = false
	if _, err := s.RecoverTurn(t.Context(), request.OperationID); !errors.Is(err, ErrBlocked) || provider.calls != 1 {
		t.Fatalf("version drift replayed: %v; calls=%d", err, provider.calls)
	}
	s.DropProcessMemory() // process exit loses exact prompt/image bytes
	compatible = true
	// The provider returned an incomplete event/timeline view. Unknown is not
	// proof of non-acceptance even when the current page omits this Turn.
	if _, err := s.RecoverTurn(t.Context(), request.OperationID); !errors.Is(err, ErrUnknown) || provider.calls != 1 {
		t.Fatalf("event gap replayed: %v; calls=%d", err, provider.calls)
	}
}

func TestSubmitMemoryExpiresAndEvictsWithoutRetainingSecrets(t *testing.T) {
	s := &SubmitService{}
	first := submitFixture()
	first.Canonical = []byte("old-secret")
	s.retain(first, time.Minute)
	old := s.live[first.OperationID]
	for n, id := range []string{"submit_2", "submit_3", "submit_4", "submit_5"} {
		request := submitFixture()
		request.OperationID = id
		request.CreatedAt = first.CreatedAt.Add(time.Duration(n+1) * time.Second)
		s.retain(request, time.Minute)
	}
	if len(s.live) != MaximumLiveSubmits || s.live[first.OperationID] != nil || strings.Contains(string(old.request.Canonical), "secret") {
		t.Fatal("evicted submit material remains live")
	}
	last := submitFixture()
	last.OperationID = "expiring"
	last.Canonical = []byte("expiring-secret")
	s.retain(last, time.Millisecond)
	expiring := s.live[last.OperationID]
	deadline := time.After(time.Second)
	for {
		s.mu.Lock()
		_, retained := s.live[last.OperationID]
		s.mu.Unlock()
		if !retained {
			break
		}
		select {
		case <-deadline:
			t.Fatal("submit material did not expire")
		case <-time.After(time.Millisecond):
		}
	}
	if strings.Contains(string(expiring.request.Canonical), "secret") {
		t.Fatal("expired submit material was not cleared")
	}
	s.DropProcessMemory()
}
