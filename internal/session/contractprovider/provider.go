// Package contractprovider adapts the checked public Dolgorae reads to Gul's
// passive session model. It has no mutation or Machine CLI path.
package contractprovider

import (
	"context"
	"errors"
	"sort"
	"time"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type Port interface {
	GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error)
	GetOrchestratedSession(context.Context, *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error)
	ListRuns(context.Context, *publicv1.ListRunsRequest) (*publicv1.ListRunsResponse, error)
}

type Provider struct{ Port Port }

func (p Provider) Snapshot(ctx context.Context, attachment workspace.Attachment, runID string, carrier session.Carrier) (session.Snapshot, error) {
	if p.Port == nil || attachment.CanonicalRoot == "" || attachment.ProviderID == "" || runID == "" || carrier.AbsolutePath == "" || carrier.ControllerID == "" {
		return session.Snapshot{}, session.ErrInvalid
	}
	ref := &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: attachment.CanonicalRoot, ExpectedWorkspaceId: attachment.ProviderID}, RunId: runID}
	runResponse, err := p.Port.GetRun(ctx, &publicv1.GetRunRequest{Run: ref})
	if err != nil {
		return session.Snapshot{}, readError(err)
	}
	if runResponse == nil {
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	run := runResponse.GetRun()
	if run == nil || run.GetRunId() != runID || run.GetWorkspaceId() != attachment.ProviderID ||
		run.GetController().GetControllerId() != carrier.ControllerID ||
		run.GetControlMode() != publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE ||
		run.GetConfiguration() == nil || run.GetConfiguration().GetParent() != nil || run.GetStateRevision() == 0 {
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	aggregateResponse, err := p.Port.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{
		RootRun: ref, Controller: &publicv1.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath,
			ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: carrier.Generation}})
	if err != nil {
		return session.Snapshot{}, readError(err)
	}
	if aggregateResponse == nil {
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	aggregate := aggregateResponse.GetSession()
	if aggregate == nil || aggregate.GetPrimaryRun().GetRunId() != runID ||
		aggregate.GetPrimaryRun().GetWorkspace().GetExpectedWorkspaceId() != attachment.ProviderID ||
		aggregate.GetPrimaryRun().GetWorkspace().GetAbsolutePath() != attachment.CanonicalRoot ||
		aggregate.GetCapturedAt() == nil || !aggregate.GetCapturedAt().IsValid() ||
		(aggregate.CloseOperationId != nil && !session.ValidCloseOperationID(aggregate.GetCloseOperationId())) {
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	switch aggregate.GetAvailability() {
	case publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_UNAVAILABLE:
		return session.Snapshot{}, session.ErrDegraded
	case publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_AVAILABLE,
		publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_RECOVERY_REQUIRED:
	default:
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	lifecycle, composition, approval, closeProgress, recovery :=
		lifecycle(aggregate.GetLifecycle()), composition(aggregate.GetComposition()), approval(aggregate.GetApprovalPolicy()),
		closeProgress(aggregate.GetCloseProgress()), recovery(aggregate.GetRecoveryClassification())
	if lifecycle == "" || composition == "" || approval == "" || closeProgress == "" || recovery == "" {
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	if aggregate.GetAvailability() == publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_RECOVERY_REQUIRED && recovery == "none" {
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	configuration := run.GetConfiguration()
	snapshot := session.Snapshot{ProviderSessionID: aggregate.GetSessionId(), PrimaryRunID: runID,
		ProviderWorkspaceID: attachment.ProviderID, AggregateRevision: aggregate.GetAggregateRevision(),
		RunRevision: run.GetStateRevision(), ObservedAt: aggregate.GetCapturedAt().AsTime().UTC(),
		Lifecycle: lifecycle, Composition: composition, ApprovalPolicy: approval,
		SpecialistPolicyName: aggregate.GetSpecialistPolicyName(), CloseProgress: closeProgress, Recovery: recovery, CloseOperationID: aggregate.GetCloseOperationId(),
		Counts: session.Counts{NonretiredMembers: aggregate.GetNonretiredMemberCount(), NonterminalSpawns: aggregate.GetNonterminalSpawnCount(),
			PendingApprovals: aggregate.GetPendingApprovalCount(), AcceptedUnfinishedTasks: aggregate.GetAcceptedUnfinishedTaskCount(),
			UnknownOutcomeTasks: aggregate.GetUnknownOutcomeTaskCount(), PublishedResults: aggregate.GetPublishedResultCount()},
		Configuration: session.Configuration{ProfileName: configuration.GetProfileName(), Purpose: configuration.GetPurpose().String(),
			PurposeLabel: configuration.GetPurposeLabel(), ModelID: configuration.GetModelId(), DefaultEffort: configuration.GetDefaultEffort(),
			RequiredCapabilities:  append([]string(nil), configuration.GetRequiredCapabilities()...),
			InstructionSchema:     configuration.GetInstructionContract().GetSchemaId(),
			CommonPrefixVersion:   configuration.GetInstructionContract().GetCommonPrefixVersion(),
			ModePrefixVersion:     configuration.GetInstructionContract().GetModePrefixVersion(),
			PurposePrefixVersion:  configuration.GetInstructionContract().GetPurposePrefixVersion(),
			InstructionByteLength: configuration.GetControllerInstructionsByteLength(),
			InstructionSHA256:     configuration.GetControllerInstructionsSha256()}}
	if snapshot.ProviderSessionID == "" || snapshot.AggregateRevision == 0 || snapshot.ObservedAt.After(time.Now().Add(time.Minute)) {
		return session.Snapshot{}, session.ErrInvalidProjection
	}
	// Parent links are observation hints only. Failed discovery cannot replace
	// the independently authoritative aggregate counts or membership.
	listed, listErr := p.Port.ListRuns(ctx, &publicv1.ListRunsRequest{Workspace: ref.Workspace})
	if listErr == nil && listed != nil {
		for _, candidate := range listed.GetItems() {
			if candidate == nil || candidate.GetRunId() == "" || candidate.GetRunId() == runID || candidate.GetWorkspaceId() != attachment.ProviderID {
				continue
			}
			parent := candidate.GetConfiguration().GetParent()
			if parent != nil && parent.GetNamespace() == "run" && parent.GetKind() == "primary" && parent.GetId() == runID {
				if lifecycle := memberLifecycle(candidate.GetLifecycle()); lifecycle != "" {
					snapshot.Members = append(snapshot.Members, session.Member{RunID: candidate.GetRunId(), Lifecycle: lifecycle})
				}
			}
		}
		sort.Slice(snapshot.Members, func(i, j int) bool { return snapshot.Members[i].RunID < snapshot.Members[j].RunID })
		if len(snapshot.Members) > session.MaximumObservedMembers {
			snapshot.Members = snapshot.Members[:session.MaximumObservedMembers]
			snapshot.MembersTruncated = true
		}
	}
	return snapshot, nil
}

func readError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// A rejected identity or credential must not disclose cached session state,
	// including when a provider omitted the required typed error detail.
	if connect.CodeOf(err) == connect.CodeUnauthenticated || connect.CodeOf(err) == connect.CodePermissionDenied {
		return session.ErrInvalidProjection
	}
	mapped := port.MapProviderError(err)
	switch mapped.Code {
	case "TRANSPORT_UNAVAILABLE", "DEADLINE_EXCEEDED":
		return session.ErrUnavailable
	case "PROTOCOL_INCOMPATIBLE":
		return session.ErrIncompatible
	case "RECOVERY_REQUIRED", "OUTCOME_UNKNOWN":
		return session.ErrDegraded
	case "CONTROLLER_MISMATCH", "WRITE_CONTINUATION_CONTROLLER_INVALID", "INVALID_REQUEST", "OPERATOR_ACTION_REQUIRED":
		return session.ErrInvalidProjection
	}
	switch mapped.Action {
	case "WAIT":
		return session.ErrBusy
	case "REFRESH_CAPABILITIES":
		return session.ErrIncompatible
	case "RECONCILE_RUN", "RECOVER_RUN":
		return session.ErrDegraded
	}
	return session.ErrInvalidProjection
}

func memberLifecycle(value publicv1.RunLifecycle) string {
	switch value {
	case publicv1.RunLifecycle_RUN_LIFECYCLE_STARTING:
		return "starting"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_IDLE:
		return "idle"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_RUNNING:
		return "running"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_WAITING_INTERACTION:
		return "waiting_interaction"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_RECONCILIATION_REQUIRED:
		return "reconciliation_required"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED:
		return "paused"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED:
		return "closed"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_START_FAILED:
		return "start_failed"
	case publicv1.RunLifecycle_RUN_LIFECYCLE_OUTCOME_UNKNOWN:
		return "outcome_unknown"
	default:
		return ""
	}
}

func lifecycle(v publicv1.OrchestratedSessionLifecycle) string {
	switch v {
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_CREATING:
		return "creating"
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ACTIVE:
		return "active"
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_DEGRADED:
		return "degraded"
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_RECOVERING:
		return "recovering"
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_COMPLETING:
		return "completing"
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ABORTING:
		return "aborting"
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_COMPLETED:
		return "completed"
	case publicv1.OrchestratedSessionLifecycle_ORCHESTRATED_SESSION_LIFECYCLE_ABORTED:
		return "aborted"
	default:
		return ""
	}
}

func composition(v publicv1.OrchestratedSessionComposition) string {
	switch v {
	case publicv1.OrchestratedSessionComposition_ORCHESTRATED_SESSION_COMPOSITION_STANDALONE_PRIMARY:
		return "standalone_primary"
	case publicv1.OrchestratedSessionComposition_ORCHESTRATED_SESSION_COMPOSITION_BROKERED_HIERARCHY:
		return "brokered_hierarchy"
	default:
		return ""
	}
}

func approval(v publicv1.OrchestratedSessionApprovalPolicy) string {
	switch v {
	case publicv1.OrchestratedSessionApprovalPolicy_ORCHESTRATED_SESSION_APPROVAL_POLICY_USER_APPROVAL_REQUIRED:
		return "user_approval_required"
	case publicv1.OrchestratedSessionApprovalPolicy_ORCHESTRATED_SESSION_APPROVAL_POLICY_FULLY_DELEGATED:
		return "fully_delegated"
	default:
		return ""
	}
}

func closeProgress(v publicv1.SessionCloseProgress) string {
	switch v {
	case publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_NONE:
		return "none"
	case publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING:
		return "settling"
	case publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED:
		return "confirmed"
	case publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_ABORTED:
		return "confirmed"
	case publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_RECOVERY_REQUIRED:
		return "recovery_required"
	case publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_OUTCOME_UNKNOWN:
		return "outcome_unknown"
	default:
		return ""
	}
}

func recovery(v publicv1.RecoveryClassification) string {
	switch v {
	case publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE:
		return "none"
	case publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED:
		return "snapshot_required"
	case publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_RECONCILE_REQUIRED:
		return "reconcile_required"
	case publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_OUTCOME_UNKNOWN:
		return "outcome_unknown"
	default:
		return ""
	}
}

var _ session.Provider = Provider{}
