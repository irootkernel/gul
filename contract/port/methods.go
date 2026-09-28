package port

import "slices"

// RequiredMethods is the pinned first-release consumer RPC inventory. Contract
// checks compare it with the upstream consumer profile and operation map.
func RequiredMethods() []string {
	return slices.Clone(requiredMethods)
}

var requiredMethods = []string{
	"ArtifactService.GetArtifact", "ArtifactService.ReadArtifactChunk",
	"ControllerService.VerifyController",
	"InteractionService.GetControllerInteraction", "InteractionService.ListPendingInteractions", "InteractionService.ResolveInteraction",
	"ObservationService.ListRunTimelineItems", "ObservationService.WatchRunEvents",
	"OrchestrationService.GetOrchestratedSession", "OrchestrationService.ListOrchestratedSessionResults",
	"RunService.CloseRun", "RunService.GetRun", "RunService.InterruptTurn", "RunService.ListRuns",
	"RunService.PauseRun", "RunService.ReconcileRun", "RunService.RecoverRun", "RunService.ResumeRun",
	"RunService.StartRun", "RunService.SubmitTurn",
	"RuntimeService.GetCapabilities", "RuntimeService.GetProfile", "RuntimeService.InspectWorkspace", "RuntimeService.ListProfiles",
	"WriterService.AcquireWriter", "WriterService.GetWorkspaceWriterStatus", "WriterService.ReleaseWriter",
}
