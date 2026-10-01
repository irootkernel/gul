// Package composition assembles checked runtime adapters through explicit host injection.
// It never selects a transport, fixture, executable, or credential fallback.
package composition

import (
	"context"
	"errors"
	"sync"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/action"
	actionprovider "github.com/rootkernel/gul/internal/action/contractprovider"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/files"
	"github.com/rootkernel/gul/internal/history"
	historyprovider "github.com/rootkernel/gul/internal/history/contractprovider"
	"github.com/rootkernel/gul/internal/interaction"
	interactionprovider "github.com/rootkernel/gul/internal/interaction/contractprovider"
	"github.com/rootkernel/gul/internal/launch"
	launchprovider "github.com/rootkernel/gul/internal/launch/contractprovider"
	"github.com/rootkernel/gul/internal/mutation"
	mutationprovider "github.com/rootkernel/gul/internal/mutation/contractprovider"
	"github.com/rootkernel/gul/internal/observation"
	observationprovider "github.com/rootkernel/gul/internal/observation/contractprovider"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/reconnect"
	reconnectprovider "github.com/rootkernel/gul/internal/reconnect/contractprovider"
	"github.com/rootkernel/gul/internal/session"
	sessionprovider "github.com/rootkernel/gul/internal/session/contractprovider"
	"github.com/rootkernel/gul/internal/sessionclose"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/submit"
	"github.com/rootkernel/gul/internal/workspace"
	workspaceprovider "github.com/rootkernel/gul/internal/workspace/contractprovider"
)

const providerID = "dolgorae"

type Config struct {
	Port     port.PublicContractPort
	Carriers session.CarrierResolver
	Roots    []string
	Policies []string
}

type Runtime struct {
	store        *storage.Store
	config       Config
	Workspaces   *workspace.Service
	Sessions     *session.Service
	actions      *action.Service
	history      *history.Service
	interactions *interaction.Service
	close        *sessionclose.Service
	submissions  *submit.Service
	mutations    *mutation.SubmitService
	raw          *actionprovider.Provider
	refresh      *sessionclose.AggregateRefresher
	observer     *interaction.Observer
	reconnect    *reconnect.Service
	probe        reconnectprovider.ContractProbe
	mu           sync.Mutex
	syncMu       sync.Mutex
	cancel       context.CancelFunc
	done         chan struct{}
	lifetime     context.Context
}

func New(ctx context.Context, store *storage.Store, c Config) (*Runtime, error) {
	if store == nil || c.Port == nil || c.Carriers == nil {
		return nil, reconnect.ErrUnavailable
	}
	probe := reconnectprovider.ContractProbe{Port: c.Port}
	if err := probe.Check(ctx); err != nil {
		return nil, err
	}
	caps, err := c.Port.GetCapabilities(ctx, &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		return nil, err
	}
	raw, err := actionprovider.New(c.Port, caps)
	if err != nil {
		return nil, err
	}
	hp, err := historyprovider.New(c.Port, caps)
	if err != nil {
		return nil, err
	}
	ip, err := interactionprovider.New(c.Port, caps)
	if err != nil {
		return nil, err
	}
	cp, err := actionprovider.NewSessionControl(c.Port)
	if err != nil {
		return nil, err
	}
	r := &Runtime{store: store, config: c, raw: raw, probe: probe}
	local := store.Presentation()
	r.Workspaces = workspace.NewService(workspaceprovider.ContractProvider{Port: c.Port}, local, nil, c.Roots)
	if err = r.Workspaces.RootConfigurationError(); err != nil {
		return nil, err
	}
	r.Sessions = session.NewService(r.Workspaces, local, sessionprovider.Provider{Port: c.Port}, c.Carriers)
	r.refresh = &sessionclose.AggregateRefresher{Cache: store.CloseRefresh(), Sessions: r.Sessions, Writer: raw, Interactions: ip, History: hp}
	r.observer = &interaction.Observer{Live: observation.NewManager(store.Observation(), observationprovider.Provider{Port: c.Port}, r)}
	r.reconnect = &reconnect.Service{Compatibility: probe, Snapshots: raw, Invalidator: store, Notifier: store, Converger: store.Actions(providerID), Refresher: r.refresh}
	r.actions = &action.Service{Repository: store.Actions(providerID), Workspaces: r.Workspaces, Carriers: c.Carriers, Provider: raw, Attempts: store.WriterAttempts(), Gate: r.reconnect.Ready}
	r.history = history.New(local, r.Workspaces, c.Carriers, hp)
	r.interactions = &interaction.Service{Actions: r.actions, Repository: local, Workspaces: r.Workspaces, Carriers: c.Carriers, Provider: ip, Attempts: store.InteractionAttempts()}
	r.observer.Poll = &interaction.Poller{Service: r.interactions, Notify: store.Observation()}
	r.close = &sessionclose.Service{Repository: store.SessionClose(), Actions: r.actions, Sessions: r.Sessions, Provider: cp, Refresh: r.refresh}
	r.mutations = &mutation.SubmitService{Attempts: store.Attempts(), Provider: mutationprovider.SubmitProvider{Port: c.Port, Bindings: r}, Gate: r.runReady}
	r.submissions = &submit.Service{Actions: r.actions, Dispatcher: mutationprovider.TextDispatcher{Service: r.mutations}}
	return r, nil
}

func (r *Runtime) Features(core *app.Core) api.FeatureHandlers {
	metadata := presentation.NewService(r.store.Presentation())
	return api.FeatureHandlers{
		Runtime:   &api.RuntimeHandler{Core: core, Launch: launch.NewService(launchprovider.Provider{Port: r.config.Port}, r.config.Policies)},
		Workspace: &api.WorkspaceHandler{Core: core, Workspaces: r.Workspaces, Presentation: metadata},
		Direct:    &api.DirectPresentationHandler{Core: core, Presentation: metadata, Sessions: r.Sessions, History: r.history, Close: r.close, Submissions: r.submissions},
		Artifact:  &api.ArtifactHandler{Core: core, History: r.history}, Interaction: &api.InteractionHandler{Core: core, Interactions: r.interactions}, Writer: &api.WriterHandler{Core: core, Actions: r.actions},
		Files: &api.FileHandler{Core: core, Files: files.NewService(r.store.Presentation())}, Events: &api.ClientEventHandler{Core: core, Events: r.store.Observation()}, Diagnostics: &api.DiagnosticsHandler{Core: core},
	}
}
func (r *Runtime) Ready(ctx context.Context) error { return r.probe.Check(ctx) }

// Start scopes every stream, poller and retry to the host rather than a request.
// Provider loss leaves the authenticated local host running with closed gates.
func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.lifetime, r.cancel = context.WithCancel(context.Background())
	r.done = make(chan struct{})
	r.reconnect.Observer = reconnect.ManagerObserver{Observer: r.observer, ProviderID: providerID, Lifetime: r.lifetime}
	go r.run(r.lifetime, r.done)
	return nil
}
func (r *Runtime) Stop(ctx context.Context) error {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	r.mu.Lock()
	r.cancel = nil
	r.mu.Unlock()
	return nil
}
func (r *Runtime) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	defer r.mutations.DropProcessMemory()
	defer r.observer.Close()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_ = r.Synchronize(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Synchronize reconstructs authority and refreshes projections without replaying mutations.
func (r *Runtime) Synchronize(ctx context.Context) error {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	bindings, err := r.store.Presentation().BoundSessions(ctx)
	if err != nil {
		return err
	}
	var bounds []action.Bound
	for _, binding := range bindings {
		w, e := r.store.Presentation().Attachment(ctx, binding.SubjectID, binding.WorkspaceID)
		if e != nil {
			return e
		}
		carrier, e := r.config.Carriers.Resolve(ctx, binding.SubjectID, binding.ControllerBindingID)
		if e != nil {
			return e
		}
		bounds = append(bounds, action.Bound{Binding: binding, Workspace: w, Carrier: carrier})
	}
	if err = r.probe.Check(ctx); err != nil {
		return errors.Join(err, r.disconnect(ctx, bounds))
	}
	var watched []observation.WatchedRun
	var needsRecovery bool
	for _, b := range bounds {
		if _, err = r.Workspaces.Revalidate(ctx, b.Binding.SubjectID, b.Binding.WorkspaceID); err != nil {
			return errors.Join(err, r.disconnect(ctx, bounds))
		}
		if !r.reconnect.Ready(b.Binding.SubjectID, b.Binding.ID) {
			needsRecovery = true
			continue
		}
		input, e := r.raw.Read(ctx, b)
		if e != nil {
			return errors.Join(e, r.disconnect(ctx, bounds))
		}
		ready, e := r.store.Actions(providerID).Converge(ctx, b, input)
		if e != nil {
			return e
		}
		if !ready {
			if e = r.refresh.Refresh(ctx, b, input.Run); e != nil {
				return e
			}
			input, e = r.raw.Read(ctx, b)
			if e != nil {
				return e
			}
			ready, e = r.store.Actions(providerID).Converge(ctx, b, input)
			if e != nil {
				return e
			}
			if !ready {
				return reconnect.ErrUnavailable
			}
		}
		if input.Run.Lifecycle == action.Closed || input.Run.Lifecycle == action.StartFailed {
			continue
		}
		watched = append(watched, observation.WatchedRun{Candidate: observation.Candidate{RunID: b.Binding.RunID, PendingInteraction: input.Run.Pending > 0, Recovery: input.Run.Recovery != action.NoRecovery, ActiveTurn: input.Run.ActiveTurn == action.Present}, Binding: observation.Binding{SubjectID: b.Binding.SubjectID, SessionID: b.Binding.ID, ProviderID: providerID, RunID: b.Binding.RunID, WorkspaceID: b.Workspace.ProviderID, AbsoluteRoot: b.Workspace.CanonicalRoot}})
	}
	if needsRecovery {
		return r.reconnect.Recover(ctx, bounds)
	}
	_, err = r.observer.Update(r.lifetime, time.Now(), watched)
	return err
}

// Disconnect once per loss; a failed durable invalidation is retried.
func (r *Runtime) disconnect(ctx context.Context, bounds []action.Bound) error {
	var changed []action.Bound
	for _, b := range bounds {
		status := r.reconnect.State(b.Binding.SubjectID, b.Binding.ID)
		if status.Connection != "disconnected" || status.Blocker != "provider" {
			changed = append(changed, b)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return r.reconnect.Disconnect(ctx, changed)
}

// Refresh is the observation bridge. Raw reads avoid the gate it helps open.
func (r *Runtime) Refresh(ctx context.Context, b observation.Binding, _ observation.Refresh, floor observation.Stamp) error {
	bound, err := r.bound(ctx, b.SubjectID, b.SessionID)
	if err != nil {
		return err
	}
	if bound.Binding.RunID != b.RunID || bound.Workspace.ProviderID != b.WorkspaceID || bound.Workspace.CanonicalRoot != b.AbsoluteRoot {
		return action.ErrAuthority
	}
	input, err := r.raw.Read(ctx, bound)
	if err != nil {
		return err
	}
	if !input.Run.Stamp.Covers(floor) {
		return observation.ErrRefresh
	}
	if err = r.refresh.Refresh(ctx, bound, input.Run); err != nil {
		return err
	}
	return nil
}
func (r *Runtime) bound(ctx context.Context, subject, id string) (action.Bound, error) {
	b, err := r.store.Presentation().Binding(ctx, subject, id)
	if err != nil {
		return action.Bound{}, err
	}
	w, err := r.Workspaces.Revalidate(ctx, subject, b.WorkspaceID)
	if err != nil {
		return action.Bound{}, err
	}
	c, err := r.config.Carriers.Resolve(ctx, subject, b.ControllerBindingID)
	if err != nil {
		return action.Bound{}, err
	}
	return action.Bound{Binding: b, Workspace: w, Carrier: c}, nil
}
func (r *Runtime) runReady(ctx context.Context, subject, run string) bool {
	b, err := r.store.Presentation().BindingByRun(ctx, subject, run)
	return err == nil && r.reconnect.Ready(subject, b.ID)
}
func (r *Runtime) Resolve(ctx context.Context, subject, run, binding, controller string) (string, string, string, uint64, error) {
	b, err := r.store.Presentation().BindingByRun(ctx, subject, run)
	if err != nil {
		return "", "", "", 0, err
	}
	if b.ControllerBindingID != binding {
		return "", "", "", 0, action.ErrAuthority
	}
	bound, err := r.bound(ctx, subject, b.ID)
	if err != nil {
		return "", "", "", 0, err
	}
	if bound.Carrier.ControllerID != controller {
		return "", "", "", 0, action.ErrAuthority
	}
	return bound.Workspace.CanonicalRoot, bound.Workspace.ProviderID, bound.Carrier.AbsolutePath, bound.Carrier.Generation, nil
}

func (r *Runtime) ReconnectState(subject, id string) reconnect.Status {
	return r.reconnect.State(subject, id)
}
