package gateway

import (
	"context"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"time"
)

var _ port.PublicContractPort = (*Gateway)(nil)

func (g *Gateway) InspectWorkspace(ctx context.Context, request *publicv1.InspectWorkspaceRequest) (*publicv1.InspectWorkspaceResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RuntimeService.InspectWorkspace", request, false, 5*time.Second, c.runtime.InspectWorkspace)
}

func (g *Gateway) ListProfiles(ctx context.Context, request *publicv1.ListProfilesRequest) (*publicv1.ListProfilesResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RuntimeService.ListProfiles", request, false, 5*time.Second, c.runtime.ListProfiles)
}

func (g *Gateway) GetProfile(ctx context.Context, request *publicv1.GetProfileRequest) (*publicv1.GetProfileResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RuntimeService.GetProfile", request, false, 5*time.Second, c.runtime.GetProfile)
}

func (g *Gateway) StartRun(ctx context.Context, request *publicv1.StartRunRequest) (*publicv1.StartRunResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.StartRun", request, true, 30*time.Second, c.run.StartRun)
}

func (g *Gateway) ListRuns(ctx context.Context, request *publicv1.ListRunsRequest) (*publicv1.ListRunsResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.ListRuns", request, false, 5*time.Second, c.run.ListRuns)
}

func (g *Gateway) GetRun(ctx context.Context, request *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.GetRun", request, false, 5*time.Second, c.run.GetRun)
}

func (g *Gateway) SubmitTurn(ctx context.Context, request *publicv1.SubmitTurnRequest) (*publicv1.SubmitTurnAccepted, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.SubmitTurn", request, true, 20*time.Second, c.run.SubmitTurn)
}

func (g *Gateway) InterruptTurn(ctx context.Context, request *publicv1.InterruptTurnRequest) (*publicv1.RunMutationResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.InterruptTurn", request, true, 10*time.Second, c.run.InterruptTurn)
}

func (g *Gateway) PauseRun(ctx context.Context, request *publicv1.PauseRunRequest) (*publicv1.RunMutationResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.PauseRun", request, true, 10*time.Second, c.run.PauseRun)
}

func (g *Gateway) ResumeRun(ctx context.Context, request *publicv1.ResumeRunRequest) (*publicv1.RunMutationResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.ResumeRun", request, true, 10*time.Second, c.run.ResumeRun)
}

func (g *Gateway) CloseRun(ctx context.Context, request *publicv1.CloseRunRequest) (*publicv1.RunMutationResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.CloseRun", request, true, 10*time.Second, c.run.CloseRun)
}

func (g *Gateway) RecoverRun(ctx context.Context, request *publicv1.RecoverRunRequest) (*publicv1.RunMutationResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.RecoverRun", request, true, 60*time.Second, c.run.RecoverRun)
}

func (g *Gateway) ReconcileRun(ctx context.Context, request *publicv1.ReconcileRunRequest) (*publicv1.RunMutationResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "RunService.ReconcileRun", request, true, 60*time.Second, c.run.ReconcileRun)
}

func (g *Gateway) ListRunTimelineItems(ctx context.Context, request *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "ObservationService.ListRunTimelineItems", request, false, 5*time.Second, c.observation.ListRunTimelineItems)
}

func (g *Gateway) ListPendingInteractions(ctx context.Context, request *publicv1.ListPendingInteractionsRequest) (*publicv1.ListPendingInteractionsResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "InteractionService.ListPendingInteractions", request, false, 5*time.Second, c.interaction.ListPendingInteractions)
}

func (g *Gateway) GetControllerInteraction(ctx context.Context, request *publicv1.GetControllerInteractionRequest) (*publicv1.GetControllerInteractionResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "InteractionService.GetControllerInteraction", request, false, 5*time.Second, c.interaction.GetControllerInteraction)
}

func (g *Gateway) ResolveInteraction(ctx context.Context, request *publicv1.ResolveInteractionRequest) (*publicv1.ResolveInteractionResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "InteractionService.ResolveInteraction", request, true, 20*time.Second, c.interaction.ResolveInteraction)
}

func (g *Gateway) GetWorkspaceWriterStatus(ctx context.Context, request *publicv1.GetWorkspaceWriterStatusRequest) (*publicv1.GetWorkspaceWriterStatusResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "WriterService.GetWorkspaceWriterStatus", request, false, 5*time.Second, c.writer.GetWorkspaceWriterStatus)
}

func (g *Gateway) AcquireWriter(ctx context.Context, request *publicv1.AcquireWriterRequest) (*publicv1.WriterState, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "WriterService.AcquireWriter", request, true, 15*time.Second, c.writer.AcquireWriter)
}

func (g *Gateway) ReleaseWriter(ctx context.Context, request *publicv1.ReleaseWriterRequest) (*publicv1.WriterState, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "WriterService.ReleaseWriter", request, true, 15*time.Second, c.writer.ReleaseWriter)
}

func (g *Gateway) VerifyController(ctx context.Context, request *publicv1.VerifyControllerRequest) (*publicv1.VerifyControllerResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "ControllerService.VerifyController", request, false, 5*time.Second, c.controller.VerifyController)
}

func (g *Gateway) GetArtifact(ctx context.Context, request *publicv1.GetArtifactRequest) (*publicv1.GetArtifactResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "ArtifactService.GetArtifact", request, false, 5*time.Second, c.artifact.GetArtifact)
}

func (g *Gateway) ReadArtifactChunk(ctx context.Context, request *publicv1.ReadArtifactChunkRequest) (*publicv1.ReadArtifactChunkResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "ArtifactService.ReadArtifactChunk", request, false, 5*time.Second, c.artifact.ReadArtifactChunk)
}

func (g *Gateway) GetOrchestratedSession(ctx context.Context, request *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "OrchestrationService.GetOrchestratedSession", request, false, 5*time.Second, c.orchestration.GetOrchestratedSession)
}

func (g *Gateway) ListOrchestratedSessionResults(ctx context.Context, request *publicv1.ListOrchestratedSessionResultsRequest) (*publicv1.ListOrchestratedSessionResultsResponse, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	return call(ctx, c, "OrchestrationService.ListOrchestratedSessionResults", request, false, 5*time.Second, c.orchestration.ListOrchestratedSessionResults)
}
