package port

import (
	"context"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
)

// PublicContractPort is the pinned 27-method consumer surface. An adapter may
// implement it with the generated local-gRPC client; no optional RPC, generic
// action, Machine CLI fallback, or production fake is part of this interface.
type PublicContractPort interface {
	RuntimePort
	RunPort
	ObservationPort
	InteractionPort
	WriterPort
	ControllerPort
	ArtifactPort
	OrchestrationPort
}

type RuntimePort interface {
	GetCapabilities(context.Context, *publicv1.GetCapabilitiesRequest) (*publicv1.GetCapabilitiesResponse, error)
	InspectWorkspace(context.Context, *publicv1.InspectWorkspaceRequest) (*publicv1.InspectWorkspaceResponse, error)
	ListProfiles(context.Context, *publicv1.ListProfilesRequest) (*publicv1.ListProfilesResponse, error)
	GetProfile(context.Context, *publicv1.GetProfileRequest) (*publicv1.GetProfileResponse, error)
}

type RunPort interface {
	StartRun(context.Context, *publicv1.StartRunRequest) (*publicv1.StartRunResponse, error)
	ListRuns(context.Context, *publicv1.ListRunsRequest) (*publicv1.ListRunsResponse, error)
	GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error)
	SubmitTurn(context.Context, *publicv1.SubmitTurnRequest) (*publicv1.SubmitTurnAccepted, error)
	InterruptTurn(context.Context, *publicv1.InterruptTurnRequest) (*publicv1.RunMutationResponse, error)
	PauseRun(context.Context, *publicv1.PauseRunRequest) (*publicv1.RunMutationResponse, error)
	ResumeRun(context.Context, *publicv1.ResumeRunRequest) (*publicv1.RunMutationResponse, error)
	CloseRun(context.Context, *publicv1.CloseRunRequest) (*publicv1.RunMutationResponse, error)
	RecoverRun(context.Context, *publicv1.RecoverRunRequest) (*publicv1.RunMutationResponse, error)
	ReconcileRun(context.Context, *publicv1.ReconcileRunRequest) (*publicv1.RunMutationResponse, error)
}

type EventStream interface {
	Receive() (*publicv1.RunEventEnvelope, error)
	Close() error
}

type ObservationPort interface {
	WatchRunEvents(context.Context, *publicv1.WatchRunEventsRequest) (EventStream, error)
	ListRunTimelineItems(context.Context, *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error)
}

type InteractionPort interface {
	ListPendingInteractions(context.Context, *publicv1.ListPendingInteractionsRequest) (*publicv1.ListPendingInteractionsResponse, error)
	GetControllerInteraction(context.Context, *publicv1.GetControllerInteractionRequest) (*publicv1.GetControllerInteractionResponse, error)
	ResolveInteraction(context.Context, *publicv1.ResolveInteractionRequest) (*publicv1.ResolveInteractionResponse, error)
}

type WriterPort interface {
	GetWorkspaceWriterStatus(context.Context, *publicv1.GetWorkspaceWriterStatusRequest) (*publicv1.GetWorkspaceWriterStatusResponse, error)
	AcquireWriter(context.Context, *publicv1.AcquireWriterRequest) (*publicv1.WriterState, error)
	ReleaseWriter(context.Context, *publicv1.ReleaseWriterRequest) (*publicv1.WriterState, error)
}

type ControllerPort interface {
	VerifyController(context.Context, *publicv1.VerifyControllerRequest) (*publicv1.VerifyControllerResponse, error)
}

type ArtifactPort interface {
	GetArtifact(context.Context, *publicv1.GetArtifactRequest) (*publicv1.GetArtifactResponse, error)
	ReadArtifactChunk(context.Context, *publicv1.ReadArtifactChunkRequest) (*publicv1.ReadArtifactChunkResponse, error)
}

type OrchestrationPort interface {
	GetOrchestratedSession(context.Context, *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error)
	ListOrchestratedSessionResults(context.Context, *publicv1.ListOrchestratedSessionResultsRequest) (*publicv1.ListOrchestratedSessionResultsResponse, error)
}
