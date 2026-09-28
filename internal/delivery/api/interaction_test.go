package api

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/proto"
)

type interactionWorkspace struct{}

func (interactionWorkspace) Revalidate(context.Context, string, string) (workspace.Attachment, error) {
	return workspace.Attachment{ID: "ws", CanonicalRoot: "/workspace", ProviderID: "provider"}, nil
}

type interactionCarrier struct{}

func (interactionCarrier) Resolve(context.Context, string, string) (session.Carrier, error) {
	return session.Carrier{ControllerID: "controller", AbsolutePath: "/private/carrier", Generation: 1}, nil
}

type interactionProvider struct {
	mu              sync.Mutex
	resolved        bool
	calls, accepted int
	unavailable     bool
}

func (*interactionProvider) ResponseLimit() int { return 65536 }
func (p *interactionProvider) Pending(context.Context, interaction.Bound) (interaction.PendingState, error) {
	return interaction.PendingState{Stamp: observation.Stamp{Head: "2", Run: 2, Interaction: 2}}, nil
}
func (p *interactionProvider) Observe(ctx context.Context, b interaction.Bound) (interaction.PendingState, error) {
	return p.Pending(ctx, b)
}
func (p *interactionProvider) Card(_ context.Context, _ interaction.Bound, id string) (interaction.Card, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unavailable {
		return interaction.Card{}, errors.New("canary-secret-private")
	}
	status := interaction.Pending
	if p.resolved {
		status = interaction.Resolved
	}
	return interaction.Card{Summary: interaction.Summary{ID: id, Kind: interaction.UserInput, Status: status, Protected: true, CreatedAt: time.Now()}, Input: &interaction.Input{Questions: []interaction.Question{{ID: "q", Prompt: "Enter value", Secret: true}}}}, nil
}
func (p *interactionProvider) Resolve(_ context.Context, _ interaction.Bound, _, _ string, _ []byte) (interaction.Resolution, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if !p.resolved {
		p.accepted++
		p.resolved = true
	}
	return interaction.Resolution{}, errors.New("lost response canary-secret-private")
}
func TestInteractionRPCCompetingClientsAndSecretAbsence(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.Context(), filepath.Join(dir, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = store.Presentation().CreateAttachment(t.Context(), workspace.Attachment{SubjectID: "owner", ID: "ws", CanonicalRoot: "/workspace", ProviderID: "provider", FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	binding, err := store.Presentation().InsertBinding(t.Context(), session.Binding{SubjectID: "owner", ID: "session", WorkspaceID: "ws", RunID: "run", ControllerBindingID: "controller", ProviderSessionID: "provider-session"})
	if err != nil {
		t.Fatal(err)
	}
	provider := &interactionProvider{}
	service := &interaction.Service{Actions: interactionActions{}, Repository: store.Presentation(), Workspaces: interactionWorkspace{}, Carriers: interactionCarrier{}, Provider: provider, Attempts: store.InteractionAttempts()}
	core := app.NewCore(app.Dependencies{Provider: ready{}, Persistence: ready{}, Authorization: allow{}})
	if err = core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer core.Stop(context.Background())
	handler := &InteractionHandler{Core: core, Principal: func(context.Context) (app.Principal, error) { return app.Principal{Subject: "owner"}, nil }, Interactions: service}
	_, rpc := gulv1connect.NewInteractionPresentationServiceHandler(handler)
	server := httptest.NewServer(rpc)
	defer server.Close()
	client := gulv1connect.NewInteractionPresentationServiceClient(server.Client(), server.URL)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			result, err := client.Resolve(t.Context(), connect.NewRequest(&gulv1.ResolveRequest{SessionId: binding.ID, InteractionId: "interaction", ResponseJson: []byte(`{"answers":{"q":{"answers":["canary-secret-private"]}}}`)}))
			if err != nil {
				t.Error(err)
				return
			}
			if result.Msg.Outcome != gulv1.InteractionResolutionOutcome_INTERACTION_RESOLUTION_OUTCOME_RESOLVED {
				t.Error("did not converge", result.Msg)
			}
			encoded, _ := proto.Marshal(result.Msg)
			if bytes.Contains(encoded, []byte("canary-secret-private")) {
				t.Error("secret in RPC result")
			}
		})
	}
	wg.Wait()
	if provider.accepted != 1 || provider.calls > 2 {
		t.Fatal("provider-first acceptance", provider.accepted, provider.calls)
	}
	// Exercise the actual journal used for interaction polling and verify that its
	// replay message, SQLite pages and WAL retain only notification metadata.
	bound := interaction.Bound{Binding: binding, Workspace: workspace.Attachment{CanonicalRoot: "/workspace", ProviderID: "provider"}}
	if err = store.Observation().InteractionChanged(t.Context(), bound, observation.Stamp{Head: "2", Run: 2, Interaction: 2}); err != nil {
		t.Fatal(err)
	}
	delivery, err := store.Observation().ReadDelivery(t.Context(), "owner", binding.ID, 0)
	if err != nil || len(delivery.Events) != 1 {
		t.Fatal(delivery, err)
	}
	for _, name := range []string{"gul.sqlite", "gul.sqlite-wal", "gul.sqlite-shm"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("canary-secret-private")) {
			t.Fatal("secret persisted", name)
		}
	}
	provider.mu.Lock()
	provider.unavailable = true
	provider.mu.Unlock()
	_, err = client.GetCard(t.Context(), connect.NewRequest(&gulv1.GetCardRequest{SessionId: binding.ID, InteractionId: "interaction"}))
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("canary-secret-private")) {
		t.Fatal("raw provider error escaped")
	}
	body := []byte("canary-secret-private")
	denied := &InteractionHandler{}
	_, err = denied.Resolve(t.Context(), connect.NewRequest(&gulv1.ResolveRequest{ResponseJson: body}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated || !bytes.Equal(body, make([]byte, len(body))) {
		t.Fatal("unauthorized body retained")
	}
}

type interactionActions struct{}

func (interactionActions) InteractionActions(context.Context, string, string) (action.Evaluation, error) {
	return action.Evaluation{Flags: action.Flags{CanResolveInteraction: true}}, nil
}
