// Package fixture owns explicitly injected fake state for assembled acceptance.
// No production command imports this package and no fixture control is an HTTP route.
package fixture

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/composition"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/host"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/storage"
)

const Password = "assembled fixture password 2026"
const bindingID = "acceptance-binding"
const controllerID = "acceptance-controller"

type Provider struct {
	*scenario.Harness
	Offline               atomic.Bool
	InspectionUnavailable atomic.Bool
	mu                    sync.Mutex
	Closed                []string
	Submitted             []publicv1.WriteIntent
}

func (p *Provider) GetCapabilities(ctx context.Context, q *publicv1.GetCapabilitiesRequest) (*publicv1.GetCapabilitiesResponse, error) {
	if p.Offline.Load() {
		return nil, errors.New("fixture provider unavailable")
	}
	return p.Harness.GetCapabilities(ctx, q)
}
func (p *Provider) InspectWorkspace(ctx context.Context, q *publicv1.InspectWorkspaceRequest) (*publicv1.InspectWorkspaceResponse, error) {
	if p.InspectionUnavailable.Load() {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("fixture workspace transport unavailable"))
	}
	if p.Offline.Load() {
		return nil, errors.New("fixture provider unavailable")
	}
	return p.Harness.InspectWorkspace(ctx, q)
}
func (p *Provider) GetRun(ctx context.Context, q *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	if p.Offline.Load() {
		return nil, errors.New("fixture provider unavailable")
	}
	return p.Harness.GetRun(ctx, q)
}
func (p *Provider) GetOrchestratedSession(ctx context.Context, q *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error) {
	if p.Offline.Load() {
		return nil, errors.New("fixture provider unavailable")
	}
	return p.Harness.GetOrchestratedSession(ctx, q)
}
func (p *Provider) SubmitTurn(ctx context.Context, q *publicv1.SubmitTurnRequest) (*publicv1.SubmitTurnAccepted, error) {
	p.mu.Lock()
	p.Submitted = append(p.Submitted, q.GetWriteIntent())
	p.mu.Unlock()
	if p.Offline.Load() {
		return nil, errors.New("fixture provider unavailable")
	}
	return p.Harness.SubmitTurn(ctx, q)
}
func (p *Provider) CloseRun(ctx context.Context, q *publicv1.CloseRunRequest) (*publicv1.RunMutationResponse, error) {
	p.mu.Lock()
	p.Closed = append(p.Closed, q.GetRun().GetRunId())
	p.mu.Unlock()
	return p.Harness.CloseRun(ctx, q)
}
func (p *Provider) CloseCalls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.Closed...)
}

func (p *Provider) SubmitCalls() []publicv1.WriteIntent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]publicv1.WriteIntent(nil), p.Submitted...)
}

type carriers struct {
	mu            sync.RWMutex
	subject, path string
}

func (c *carriers) Resolve(_ context.Context, subject, id string) (session.Carrier, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if subject == "" || subject != c.subject || id != bindingID {
		return session.Carrier{}, session.ErrCarrierUnavailable
	}
	return session.Carrier{ControllerID: controllerID, Generation: 1, AbsolutePath: c.path}, nil
}

type Fixture struct {
	Data, Workspace string
	Host            *host.Host
	Provider        *Provider
	Runtime         *composition.Runtime
	Store           *storage.Store
	Binding         session.Binding
	Run             *publicv1.RunRef
	carrier         *carriers
	port            int
	transport       *http.Transport
	HTTP            *http.Client
}

func New(data, workspace string, port int) (*Fixture, error) {
	parent, err := filepath.EvalSymlinks(filepath.Dir(data))
	if err != nil {
		return nil, err
	}
	data = filepath.Join(parent, filepath.Base(data))
	if err := os.MkdirAll(workspace, 0700); err != nil {
		return nil, err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, err
	}
	if port == 0 {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		port = l.Addr().(*net.TCPAddr).Port
		l.Close()
	}
	f := &Fixture{Data: data, Workspace: workspace, port: port, Provider: &Provider{Harness: scenario.NewRealtime()}, carrier: &carriers{path: filepath.Join(workspace, ".dolgorae", "controller.json")}}
	if err := f.Provider.RegisterController(scenario.ControllerSpec{ID: controllerID, Generation: 1, CarrierPath: f.carrier.path, OrchestrationLaunch: true, PolicyName: "preprovisioned"}); err != nil {
		return nil, err
	}
	return f, nil
}
func (f *Fixture) Start(ctx context.Context) error {
	f.Host = host.New(host.Config{DataDirectory: f.Data, Port: f.port, Assemble: func(store *storage.Store) (host.Assembly, error) {
		r, err := composition.New(ctx, store, composition.Config{Port: f.Provider, Carriers: f.carrier, Roots: []string{f.Workspace}, Policies: []string{"preprovisioned"}})
		if err != nil {
			return host.Assembly{}, err
		}
		f.Runtime = r
		f.Store = store
		return host.Assembly{Provider: r, Lifecycle: r, Features: r.Features}, nil
	}})
	if err := f.Host.Start(ctx); err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(f.Host.CertificateDER())
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	f.transport = &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}
	f.HTTP = &http.Client{Transport: f.transport, Timeout: 20 * time.Second}
	return nil
}
func (f *Fixture) Initialize(ctx context.Context) error {
	configured, err := f.Host.Boundary.Accounts.AccountConfigured(ctx)
	if err != nil {
		return err
	}
	if !configured {
		view, err := host.Attach(ctx, f.Data)
		if err != nil {
			return err
		}
		q := connect.NewRequest(&gulv1.FirstRunSetupRequest{Password: []byte(Password)})
		q.Header().Set("Origin", view.Origin)
		q.Header().Set(api.BrowserHeader, "1")
		q.Header().Set(api.SetupHeader, view.SetupCredential)
		if _, err = gulv1connect.NewAuthServiceClient(f.HTTP, view.Origin).FirstRunSetup(ctx, q); err != nil {
			return err
		}
	}
	account, err := f.Store.Auth().PasswordAccount(ctx)
	if err != nil {
		return err
	}
	f.carrier.mu.Lock()
	f.carrier.subject = account.SubjectID
	f.carrier.mu.Unlock()
	ws, err := f.Runtime.Workspaces.RegisterFromAllowlistPath(ctx, account.SubjectID, "root-1", ".")
	if err != nil {
		return err
	}
	bindings, err := f.Store.Presentation().ListBindings(ctx, account.SubjectID, ws.ID)
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		carrier, _ := f.carrier.Resolve(ctx, account.SubjectID, bindingID)
		response, err := f.Provider.StartRun(ctx, &publicv1.StartRunRequest{Workspace: &publicv1.WorkspaceRef{AbsolutePath: ws.CanonicalRoot, ExpectedWorkspaceId: ws.ProviderID}, Controller: &publicv1.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: controllerID, ExpectedControllerGeneration: 1}, IdempotencyKey: "fixture-start", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
		if err != nil {
			return err
		}
		f.Binding, err = f.Runtime.Sessions.BindPrimary(ctx, account.SubjectID, ws.ID, response.Run.RunId, bindingID)
		if err != nil {
			return err
		}
	} else {
		f.Binding = bindings[0]
	}
	f.Run = &publicv1.RunRef{RunId: f.Binding.RunID, Workspace: &publicv1.WorkspaceRef{AbsolutePath: ws.CanonicalRoot, ExpectedWorkspaceId: ws.ProviderID}}
	if err = f.Store.Presentation().PutBinding(ctx, storage.BindingReference{SubjectID: account.SubjectID, BindingID: bindingID, CredentialKey: "acceptance/controller", ExpectedControllerID: controllerID, Health: "healthy"}); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = f.Runtime.Synchronize(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.Join(err, fmt.Errorf("reconnect state: %+v", f.Runtime.ReconnectState(f.Binding.SubjectID, f.Binding.ID)))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
func (f *Fixture) Stop(ctx context.Context) error {
	if f.transport != nil {
		f.transport.CloseIdleConnections()
	}
	return f.Host.Stop(ctx)
}
func (f *Fixture) Restart(ctx context.Context) error {
	if err := f.Stop(ctx); err != nil {
		return err
	}
	if err := f.Start(ctx); err != nil {
		return err
	}
	return f.Initialize(ctx)
}
func (f *Fixture) Complete() error {
	return f.Provider.CompleteTurn(f.Run.RunId, publicv1.TurnStatus_TURN_STATUS_COMPLETED)
}
func (f *Fixture) Approval() error {
	return f.Provider.OpenInteraction(f.Run.RunId, &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "approval", Kind: publicv1.InteractionKind_INTERACTION_KIND_COMMAND_EXECUTION_APPROVAL}, ResponseSchemaId: "dolgorae.interaction.command-approval/v1", Decisions: []publicv1.InteractionDecision{publicv1.InteractionDecision_INTERACTION_DECISION_ACCEPT_ONCE, publicv1.InteractionDecision_INTERACTION_DECISION_DECLINE, publicv1.InteractionDecision_INTERACTION_DECISION_CANCEL}, Payload: &publicv1.ControllerInteraction_CommandApproval{CommandApproval: &publicv1.CommandApprovalInteraction{Title: "Fixture approval", Message: "Allow this fixture command?", Command: []string{"printf", "fixture"}, Cwd: &publicv1.PathProjection{Value: &publicv1.PathProjection_Utf8Path{Utf8Path: f.Workspace}}}}})
}
func (f *Fixture) Results() error {
	child, err := f.Provider.SpawnSpecialist(f.Run.RunId, "Reviewer")
	if err != nil {
		return err
	}
	if err = f.Provider.PublishResult(f.Run.RunId, &publicv1.OrchestratedSessionResult{ResultId: "result-1", TaskId: "task-1", SpecialistRun: child, SpecialistRole: "Reviewer"}, []byte("Specialist original\r\n한글 <script>inert</script>")); err != nil {
		return err
	}
	return f.Provider.CompleteSpecialist(child.RunId)
}
