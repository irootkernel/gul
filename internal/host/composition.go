package host

import (
	"context"

	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/files"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/launch"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/sessionclose"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

// defaultFeatures composes the completed local services against one core and
// store. Runtime ports deliberately have no source until E2 supplies one.
// Config.Assemble supplies E14 services before startup and handlers afterward.
func defaultFeatures(core *app.Core, store *storage.Store) api.FeatureHandlers {
	local := store.Presentation()
	workspaces := workspace.NewService(offlineRuntime{}, local, nil, nil)
	carriers := offlineRuntime{}
	sessions := session.NewService(workspaces, local, carriers, carriers)
	actions := &action.Service{
		Repository: store.Actions("dolgorae"), Workspaces: workspaces,
		Carriers: carriers, Provider: carriers, Attempts: store.WriterAttempts(),
		// No runtime has completed compatibility and observation admission.
		Gate: func(string, string) bool { return false },
	}
	historyService := history.New(local, workspaces, carriers, offlineHistory{})
	interactions := &interaction.Service{Actions: actions, Repository: local,
		Workspaces: workspaces, Carriers: carriers, Provider: offlineInteraction{},
		Attempts: store.InteractionAttempts()}
	closeService := &sessionclose.Service{Repository: store.SessionClose(), Actions: actions,
		Sessions: sessions, Provider: carriers, Refresh: offlineRuntime{}}
	metadata := presentation.NewService(local)
	return api.FeatureHandlers{
		Runtime:   &api.RuntimeHandler{Core: core, Launch: launch.NewService(nil, nil)},
		Workspace: &api.WorkspaceHandler{Core: core, Workspaces: workspaces, Presentation: metadata},
		Direct: &api.DirectPresentationHandler{Core: core, Presentation: metadata,
			Sessions: sessions, Close: closeService, History: historyService},
		Artifact:    &api.ArtifactHandler{Core: core, History: historyService},
		Interaction: &api.InteractionHandler{Core: core, Interactions: interactions},
		Writer:      &api.WriterHandler{Core: core, Actions: actions},
		Files:       &api.FileHandler{Core: core, Files: files.NewService(local)},
		Events:      &api.ClientEventHandler{Core: core, Events: store.Observation()},
		Diagnostics: &api.DiagnosticsHandler{Core: core},
	}
}

// offlineRuntime satisfies only the domain ports needed to keep the hosted
// routes typed and fail closed. It never invents a provider identity or carrier.
type offlineRuntime struct{}

func (offlineRuntime) InspectWorkspace(context.Context, string, *string) (workspace.Inspection, error) {
	return workspace.Inspection{}, workspace.ErrProviderUnavailable
}
func (offlineRuntime) Resolve(context.Context, string, string) (session.Carrier, error) {
	return session.Carrier{}, session.ErrCarrierUnavailable
}
func (offlineRuntime) Snapshot(context.Context, workspace.Attachment, string, session.Carrier) (session.Snapshot, error) {
	return session.Snapshot{}, session.ErrUnavailable
}
func (offlineRuntime) Read(context.Context, action.Bound) (action.Input, error) {
	return action.Input{}, action.ErrUnavailable
}
func (offlineRuntime) Acquire(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	return action.WriterProjection{}, action.ErrUnavailable
}
func (offlineRuntime) Release(context.Context, action.Bound, uint64) (action.WriterProjection, error) {
	return action.WriterProjection{}, action.ErrUnavailable
}

type offlineHistory struct{}

func (offlineHistory) Snapshot(context.Context, history.Bound) (history.Snapshot, error) {
	return history.Snapshot{}, history.ErrUnavailable
}
func (offlineHistory) Timeline(context.Context, history.Bound, string, uint32) (history.Timeline, error) {
	return history.Timeline{}, history.ErrUnavailable
}
func (offlineHistory) Results(context.Context, history.Bound, string, uint32) (history.Results, error) {
	return history.Results{}, history.ErrUnavailable
}
func (offlineHistory) ReadArtifact(context.Context, history.Bound, history.Artifact) ([]byte, error) {
	return nil, history.ErrUnavailable
}

type offlineInteraction struct{}

func (offlineInteraction) ResponseLimit() int { return interaction.MaximumResponseBytes }
func (offlineInteraction) Pending(context.Context, interaction.Bound) (interaction.PendingState, error) {
	return interaction.PendingState{}, interaction.ErrUnavailable
}
func (offlineInteraction) Card(context.Context, interaction.Bound, string) (interaction.Card, error) {
	return interaction.Card{}, interaction.ErrUnavailable
}
func (offlineInteraction) Resolve(context.Context, interaction.Bound, string, string, []byte) (interaction.Resolution, error) {
	return interaction.Resolution{}, interaction.ErrUnavailable
}
func (offlineInteraction) Observe(context.Context, interaction.Bound) (interaction.PendingState, error) {
	return interaction.PendingState{}, interaction.ErrUnavailable
}
func (offlineRuntime) Mutate(context.Context, action.Bound, sessionclose.Kind, uint64, bool) (sessionclose.Mutation, error) {
	return sessionclose.Mutation{}, sessionclose.ErrUnavailable
}
func (offlineRuntime) Refresh(context.Context, action.Bound, action.RunFacts) error {
	return sessionclose.ErrUnavailable
}
