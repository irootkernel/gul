package contractprovider

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/sessionclose"
)

// SessionControlPort keeps root mutations separate from observation and Writer
// authority. Its implementation must not transparently retry tokenless calls.
type SessionControlPort interface {
	CloseRun(context.Context, *publicv1.CloseRunRequest) (*publicv1.RunMutationResponse, error)
	RecoverRun(context.Context, *publicv1.RecoverRunRequest) (*publicv1.RunMutationResponse, error)
	ReconcileRun(context.Context, *publicv1.ReconcileRunRequest) (*publicv1.RunMutationResponse, error)
}

type SessionControlProvider struct{ port SessionControlPort }

func NewSessionControl(port SessionControlPort) (*SessionControlProvider, error) {
	if port == nil {
		return nil, sessionclose.ErrInvalid
	}
	return &SessionControlProvider{port: port}, nil
}

func (p *SessionControlProvider) Mutate(ctx context.Context, b action.Bound, kind sessionclose.Kind, revision uint64, interrupt bool) (sessionclose.Mutation, error) {
	if p == nil || p.port == nil || revision == 0 || b.Binding.RunID == "" || b.Workspace.ProviderID == "" || b.Workspace.CanonicalRoot == "" || b.Carrier.ControllerID == "" || b.Carrier.Generation == 0 || b.Carrier.AbsolutePath == "" {
		return sessionclose.Mutation{}, sessionclose.ErrInvalid
	}
	timeout := sessionclose.RecoveryTimeout
	if kind == sessionclose.Close {
		timeout = sessionclose.CloseTimeout
	} else if kind != sessionclose.Recover && kind != sessionclose.Reconcile {
		return sessionclose.Mutation{}, sessionclose.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var response *publicv1.RunMutationResponse
	var err error
	switch kind {
	case sessionclose.Close:
		response, err = p.port.CloseRun(ctx, &publicv1.CloseRunRequest{Run: ref(b), Controller: carrier(b), ExpectedStateRevision: revision, Interrupt: interrupt})
	case sessionclose.Recover:
		response, err = p.port.RecoverRun(ctx, &publicv1.RecoverRunRequest{Run: ref(b), Controller: carrier(b), ExpectedStateRevision: revision})
	case sessionclose.Reconcile:
		response, err = p.port.ReconcileRun(ctx, &publicv1.ReconcileRunRequest{Run: ref(b), Controller: carrier(b), ExpectedStateRevision: revision})
	}
	if err != nil {
		// A partial success and an error cannot establish either acceptance or
		// rejection. Reconciliation must discover the authoritative outcome.
		if response != nil {
			return sessionclose.Mutation{}, sessionclose.ErrUnavailable
		}
		return sessionControlError(err, b.Binding.RunID, kind)
	}
	if response == nil || response.Run == nil || response.Context == nil || response.Context.ProtocolVersion != 1 || response.Context.ServerInstanceId == "" || !known(response.ProtoReflect()) || (response.Context.OperationId != nil && !session.ValidCloseOperationID(response.Context.GetOperationId())) {
		return sessionclose.Mutation{}, sessionclose.ErrUnavailable
	}
	r := response.Run
	if r.RunId != b.Binding.RunID || r.WorkspaceId != b.Workspace.ProviderID || r.Configuration == nil || r.Configuration.ProfileName == "" || r.Configuration.Parent != nil || r.Controller == nil || r.Controller.ControllerId != b.Carrier.ControllerID || r.Controller.Generation != b.Carrier.Generation || r.Controller.Kind == 0 || r.StateRevision < revision || r.StateRevision != r.GetStamp().GetRunStateRevision() || r.EventCursor != r.GetStamp().GetCapturedHeadCursor() {
		return sessionclose.Mutation{}, sessionclose.ErrUnavailable
	}
	run, err := runFacts(r)
	if err != nil || !run.Stamp.Valid() || run.Lifecycle == 0 || run.Variant == 0 || run.Control == 0 || run.Lane == 0 || run.Access == 0 || run.Verification == 0 || run.Authority == 0 || run.Reconciliation == 0 || run.Requested == 0 || run.Achieved == 0 || run.Background == 0 || run.Recovery == 0 || run.RecoveryAction == 0 {
		return sessionclose.Mutation{}, sessionclose.ErrUnavailable
	}
	return sessionclose.Mutation{Run: run, OperationID: response.Context.GetOperationId()}, nil
}

func sessionControlError(err error, runID string, kind sessionclose.Kind) (sessionclose.Mutation, error) {
	var rpc *connect.Error
	if !errors.As(err, &rpc) || len(rpc.Details()) != 1 {
		return sessionclose.Mutation{}, sessionclose.ErrUnavailable
	}
	value, decodeErr := rpc.Details()[0].Value()
	d, ok := value.(*publicv1.DolgoraeErrorDetail)
	if decodeErr != nil || !ok || !known(d.ProtoReflect()) || d.DetailVersion != 1 || d.RetryClassification != publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN || (d.RunId != nil && d.GetRunId() != runID) || d.TurnId != nil || d.InteractionId != nil || d.IdempotencyKey != nil || d.SafeResumeCursor != nil {
		return sessionclose.Mutation{}, sessionclose.ErrUnavailable
	}
	if kind == sessionclose.Close && d.DolgoraeErrorCode == "SESSION_CLOSE_IN_PROGRESS" && rpc.Code() == connect.CodeFailedPrecondition && d.GetRunId() == runID && session.ValidCloseOperationID(d.GetOperationId()) && d.Action == publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT && d.RecoveryClassification == publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED {
		return sessionclose.Mutation{Pending: true, OperationID: d.GetOperationId()}, nil
	}
	// Only these pinned preacceptance policies prove rejection. In particular,
	// recovery-required and outcome-unknown errors cannot become REJECTED.
	status, required, recovery := rejectionPolicy(d.DolgoraeErrorCode)
	if status == 0 || rpc.Code() != status || d.Action != required || d.RecoveryClassification != recovery || d.OperationId != nil {
		return sessionclose.Mutation{}, sessionclose.ErrUnavailable
	}
	mapped := port.MapProviderError(err)
	return sessionclose.Mutation{RejectionCode: mapped.Code, NextAction: mapped.Action}, nil
}

func rejectionPolicy(code string) (connect.Code, publicv1.RequiredClientAction, publicv1.RecoveryClassification) {
	recovery := publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE
	switch code {
	case "INVALID_ARGUMENT":
		return connect.CodeInvalidArgument, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_REQUEST, recovery
	case "CONTROLLER_MISMATCH", "OBSERVER_MUTATION_FORBIDDEN":
		return connect.CodePermissionDenied, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_VERIFY_CONTROLLER, recovery
	case "CONTROL_MODE_CONTROLLER_MISMATCH":
		return connect.CodePermissionDenied, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_COMPATIBLE_CONTROLLER, recovery
	case "RUN_STATE_CONFLICT":
		return connect.CodeAborted, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT, publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED
	case "RUN_BUSY", "WRITER_BUSY":
		return connect.CodeAborted, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT, recovery
	case "RUN_NOT_FOUND", "WORKSPACE_NOT_INITIALIZED":
		return connect.CodeNotFound, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_ABORT, recovery
	default:
		return 0, 0, 0
	}
}

var _ sessionclose.Provider = (*SessionControlProvider)(nil)
