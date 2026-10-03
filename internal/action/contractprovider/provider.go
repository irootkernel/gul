package contractprovider

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Port interface {
	GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error)
	GetWorkspaceWriterStatus(context.Context, *publicv1.GetWorkspaceWriterStatusRequest) (*publicv1.GetWorkspaceWriterStatusResponse, error)
	GetProfile(context.Context, *publicv1.GetProfileRequest) (*publicv1.GetProfileResponse, error)
	ListPendingInteractions(context.Context, *publicv1.ListPendingInteractionsRequest) (*publicv1.ListPendingInteractionsResponse, error)
	ListRunTimelineItems(context.Context, *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error)
	GetOrchestratedSession(context.Context, *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error)
	AcquireWriter(context.Context, *publicv1.AcquireWriterRequest) (*publicv1.WriterState, error)
	ReleaseWriter(context.Context, *publicv1.ReleaseWriterRequest) (*publicv1.WriterState, error)
	InterruptTurn(context.Context, *publicv1.InterruptTurnRequest) (*publicv1.RunMutationResponse, error)
}
type Provider struct {
	port            Port
	capabilities    action.Capabilities
	timelineVersion uint32
}

func New(port Port, caps *publicv1.GetCapabilitiesResponse) (*Provider, error) {
	if port == nil || caps == nil || !known(caps.ProtoReflect()) || caps.Features == nil || caps.Protocol == nil || caps.Protocol.TimelineProtocolVersion != 1 || caps.AccessPolicyTransition == 0 {
		return nil, action.ErrInvalid
	}
	methods := map[string]bool{}
	for _, method := range caps.SupportedMethods {
		if method == "" || methods[method] {
			return nil, action.ErrInvalid
		}
		methods[method] = true
	}
	var dedicatedWriter bool
	for _, lane := range caps.GetLanes().GetItems() {
		if lane.GetLane() == publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED {
			dedicatedWriter = lane.GetWriterSupport()
		}
	}
	f := caps.Features
	c := action.Capabilities{Checked: f.PersistentRuns && f.ControllerBinding && f.SafeClientProjection && f.ControlModes, Submit: methods["RunService.SubmitTurn"] && f.PersistentRuns, Acquire: methods["WriterService.AcquireWriter"], Release: methods["WriterService.ReleaseWriter"], Interrupt: methods["RunService.InterruptTurn"], Resolve: methods["InteractionService.ResolveInteraction"], Recover: methods["RunService.RecoverRun"], Reconcile: methods["RunService.ReconcileRun"], VerifyController: methods["ControllerService.VerifyController"], Pause: methods["RunService.PauseRun"], Resume: methods["RunService.ResumeRun"], Close: methods["RunService.CloseRun"], ReaderWriter: f.ReaderWriterAccess, DedicatedWriter: dedicatedWriter, DurableWriter: f.DurableWriterAuthority, ThreadlessAcquire: f.ThreadlessAcquireWrite, FirstWriteViaSubmit: f.FirstWriteViaSubmitTurn, Transition: action.Support(caps.AccessPolicyTransition)}
	return &Provider{port: port, capabilities: c, timelineVersion: caps.Protocol.TimelineProtocolVersion}, nil
}
func ref(b action.Bound) *publicv1.RunRef {
	return &publicv1.RunRef{Workspace: &publicv1.WorkspaceRef{AbsolutePath: b.Workspace.CanonicalRoot, ExpectedWorkspaceId: b.Workspace.ProviderID}, RunId: b.Binding.RunID}
}
func carrier(b action.Bound) *publicv1.ControllerCarrierRef {
	return &publicv1.ControllerCarrierRef{AbsoluteFilePath: b.Carrier.AbsolutePath, ExpectedControllerId: b.Carrier.ControllerID, ExpectedControllerGeneration: b.Carrier.Generation}
}
func stamp(s *publicv1.ProjectionStamp) observation.Stamp {
	return observation.Stamp{Head: observation.Cursor(s.GetCapturedHeadCursor()), Run: s.GetRunStateRevision(), Writer: s.GetWriterStateRevision(), Interaction: s.GetInteractionStateRevision()}
}
func (p *Provider) Read(ctx context.Context, b action.Bound) (action.Input, error) {
	if p == nil || p.port == nil {
		return action.Input{}, action.ErrUnavailable
	}
	rr, err := p.port.GetRun(ctx, &publicv1.GetRunRequest{Run: ref(b)})
	if err != nil {
		return action.Input{}, safeError(err)
	}
	if rr == nil || rr.Run == nil || !known(rr.ProtoReflect()) {
		return action.Input{}, action.ErrBlocked
	}
	r := rr.Run
	if r.RunId != b.Binding.RunID || r.WorkspaceId != b.Workspace.ProviderID || r.Configuration == nil || r.Configuration.Parent != nil || r.Controller == nil || r.Controller.ControllerId == "" || r.Controller.Generation == 0 || r.Controller.Kind == 0 || r.StateRevision != r.GetStamp().GetRunStateRevision() || r.EventCursor != r.GetStamp().GetCapturedHeadCursor() {
		return action.Input{}, action.ErrBlocked
	}
	run, err := runFacts(r)
	if err != nil {
		return action.Input{}, err
	}
	wr, err := p.port.GetWorkspaceWriterStatus(ctx, &publicv1.GetWorkspaceWriterStatusRequest{Workspace: ref(b).Workspace})
	if err != nil {
		return action.Input{}, safeError(err)
	}
	if wr == nil {
		return action.Input{}, action.ErrBlocked
	}
	writer, err := writerProjection(wr.Writer, b)
	if err != nil {
		return action.Input{}, err
	}
	in := action.Input{Checked: true, Freshness: action.Fresh, Run: run, Writer: writer, Capabilities: p.capabilities}
	pr, err := p.port.GetProfile(ctx, &publicv1.GetProfileRequest{ProfileName: r.Configuration.ProfileName})
	if err != nil {
		return action.Input{}, safeError(err)
	}
	if pr == nil || pr.Profile == nil || !known(pr.ProtoReflect()) || pr.Profile.Name != r.Configuration.ProfileName || pr.Profile.BackgroundExecution == nil {
		return action.Input{}, action.ErrBlocked
	}
	profile := pr.Profile
	in.Profile = action.ProfileFacts{Compatibility: action.Compatibility(profile.Compatibility), Transition: action.Support(profile.AccessPolicyTransition), BackgroundControl: action.Support(profile.BackgroundExecution.Support), MaximumAssurance: action.Assurance(profile.MaximumAssurance)}
	for _, lane := range profile.SupportedExecutionLanes {
		if lane == r.ExecutionLane {
			in.Profile.SupportsLane = true
		}
	}
	// Writer observation remains visible without a usable Controller. Sensitive
	// timeline/aggregate reads and all mutations remain unavailable.
	if b.Carrier.ControllerID == "" || b.Carrier.ControllerID != r.Controller.ControllerId || b.Carrier.Generation != r.Controller.Generation {
		in.Checked = false
		return in, nil
	}
	in.ControllerMatches = true
	pending, err := p.port.ListPendingInteractions(ctx, &publicv1.ListPendingInteractionsRequest{Run: ref(b)})
	if err != nil {
		return action.Input{}, safeError(err)
	}
	if pending == nil || !known(pending.ProtoReflect()) || len(pending.Items) > 256 || uint32(len(pending.Items)) != r.PendingInteractionCount || proto.Size(pending) > 1024*1024 {
		return action.Input{}, action.ErrBlocked
	}
	ids := map[string]bool{}
	for _, item := range pending.Items {
		if item == nil || item.InteractionId == "" || ids[item.InteractionId] || item.RunId != r.RunId || item.Status != publicv1.InteractionStatus_INTERACTION_STATUS_PENDING || item.Kind == 0 || item.StateRevision > pending.GetStamp().GetInteractionStateRevision() {
			return action.Input{}, action.ErrBlocked
		}
		ids[item.InteractionId] = true
	}
	in.InteractionStamp = stamp(pending.Stamp)
	timeline, err := p.port.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: ref(b), Controller: carrier(b), AfterCursor: r.GetStamp().GetCapturedHeadCursor(), Limit: 1, TimelineVersion: p.timelineVersion})
	if err != nil {
		return action.Input{}, safeError(err)
	}
	if timeline == nil || !known(timeline.ProtoReflect()) || len(timeline.Items) > 1 || proto.Size(timeline) > 1024*1024 || stamp(timeline.Stamp) != run.Stamp {
		return action.Input{}, action.ErrBlocked
	}
	in.TimelineHead = observation.Cursor(timeline.CapturedHeadCursor)
	ar, err := p.port.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: ref(b), Controller: carrier(b)})
	if err != nil {
		return action.Input{}, safeError(err)
	}
	if ar == nil || ar.Session == nil || !known(ar.ProtoReflect()) {
		return action.Input{}, action.ErrBlocked
	}
	a := ar.Session
	if a.SessionId != b.Binding.ProviderSessionID || !proto.Equal(a.PrimaryRun, ref(b)) || a.AggregateRevision == 0 || a.RequiredAction == 0 || a.Availability == 0 || a.CapturedAt == nil || !a.CapturedAt.IsValid() {
		return action.Input{}, action.ErrBlocked
	}
	freshness := action.Fresh
	if (a.Availability != publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_AVAILABLE && a.Availability != publicv1.OrchestratedSessionAvailability_ORCHESTRATED_SESSION_AVAILABILITY_RECOVERY_REQUIRED) || time.Since(a.CapturedAt.AsTime()) > 10*time.Second || a.CapturedAt.AsTime().After(time.Now().Add(time.Minute)) {
		freshness = action.Stale
	}
	in.Aggregate = action.Aggregate{Directive: aggregateDirective(a.RequiredAction), Freshness: freshness, Revision: a.AggregateRevision, Lifecycle: action.SessionLifecycle(a.Lifecycle), CloseProgress: action.CloseProgress(a.CloseProgress), Recovery: action.AggregateRecovery(a.RecoveryClassification), CloseIntent: action.CloseIntent(a.CloseIntent), NonretiredMembers: a.NonretiredMemberCount, NonterminalSpawns: a.NonterminalSpawnCount, PendingApprovals: a.PendingApprovalCount, AcceptedUnfinishedTasks: a.AcceptedUnfinishedTaskCount, UnknownOutcomeTasks: a.UnknownOutcomeTaskCount}
	if a.RequiredAction == publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION {
		in.Run.Lineage = action.ContinuationRequired
	}
	return in, nil
}
func (p *Provider) Acquire(ctx context.Context, b action.Bound, revision uint64) (action.WriterProjection, error) {
	w, err := p.port.AcquireWriter(ctx, &publicv1.AcquireWriterRequest{Run: ref(b), Controller: carrier(b), ExpectedStateRevision: revision})
	if err != nil {
		return action.WriterProjection{}, safeError(err)
	}
	return writerProjection(w, b)
}
func (p *Provider) Release(ctx context.Context, b action.Bound, revision uint64) (action.WriterProjection, error) {
	w, err := p.port.ReleaseWriter(ctx, &publicv1.ReleaseWriterRequest{Run: ref(b), Controller: carrier(b), ExpectedStateRevision: revision})
	if err != nil {
		return action.WriterProjection{}, safeError(err)
	}
	return writerProjection(w, b)
}

func (p *Provider) Interrupt(ctx context.Context, b action.Bound, revision uint64) error {
	if revision == 0 || b.Carrier.ControllerID == "" {
		return action.ErrInvalid
	}
	reply, err := p.port.InterruptTurn(ctx, &publicv1.InterruptTurnRequest{Run: ref(b), Controller: carrier(b), ExpectedStateRevision: revision})
	if err != nil {
		return safeError(err)
	}
	if reply == nil || reply.Run == nil || !known(reply.ProtoReflect()) {
		return action.ErrOutcomeUnknown
	}
	r := reply.Run
	if r.RunId != b.Binding.RunID || r.WorkspaceId != b.Workspace.ProviderID || r.Controller == nil || r.Controller.ControllerId != b.Carrier.ControllerID || r.Controller.Generation != b.Carrier.Generation ||
		r.StateRevision < revision || r.StateRevision != r.GetStamp().GetRunStateRevision() || r.EventCursor != r.GetStamp().GetCapturedHeadCursor() || !stamp(r.Stamp).Valid() {
		return action.ErrOutcomeUnknown
	}
	if _, err := runFacts(r); err != nil {
		return action.ErrOutcomeUnknown
	}
	return nil
}
func safeError(err error) error {
	var rpc *connect.Error
	if errors.As(err, &rpc) {
		var detail *publicv1.DolgoraeErrorDetail
		for _, d := range rpc.Details() {
			value, decodeErr := d.Value()
			if decodeErr != nil {
				return action.ErrUnavailable
			}
			if typed, ok := value.(*publicv1.DolgoraeErrorDetail); ok {
				if detail != nil {
					return action.ErrUnavailable
				}
				detail = typed
			}
		}
		if detail != nil && known(detail.ProtoReflect()) && detail.DetailVersion == 1 && detail.RetryClassification != 0 && detail.RecoveryClassification != 0 && detail.Action == publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION && (detail.DolgoraeErrorCode == "ACCESS_TRANSITION_UNSUPPORTED" || detail.DolgoraeErrorCode == "SHARED_RUN_WRITE_FORBIDDEN") {
			return action.ErrUnsupportedTransition
		}
	}
	mapped := port.MapProviderError(err)
	if mapped.Code == "WRITER_CONFLICT" {
		return action.ErrWriterBusy
	}
	if mapped.Action == "USE_TERMINAL_SOURCE" || mapped.Code == "THREADLESS_REQUIRES_WRITE_TURN" {
		return action.ErrUnsupportedTransition
	}
	return action.ErrUnavailable
}
func known(m protoreflect.Message) bool {
	if !m.IsValid() || len(m.GetUnknown()) != 0 {
		return false
	}
	valid := true
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		check := func(v protoreflect.Value) bool {
			if f.Kind() == protoreflect.EnumKind {
				return f.Enum().Values().ByNumber(v.Enum()) != nil
			}
			if f.Kind() == protoreflect.MessageKind {
				return known(v.Message())
			}
			return true
		}
		if f.IsList() {
			for i := 0; i < v.List().Len(); i++ {
				if !check(v.List().Get(i)) {
					valid = false
					break
				}
			}
		} else {
			valid = check(v)
		}
		return valid
	})
	return valid
}

var _ action.Provider = (*Provider)(nil)

func aggregateDirective(required publicv1.RequiredClientAction) action.AggregateDirective {
	switch required {
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_NONE:
		return action.NoAggregateAction
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFETCH_INTERACTION, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONNECT_FROM_COMMITTED_CURSOR:
		return action.RefreshAggregate
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONCILE_RUN:
		return action.ReconcileAggregate
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECOVER_RUN:
		return action.RecoverAggregate
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_OPERATOR_REPAIR:
		return action.RepairAggregate
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION:
		return action.ContinueWrite
	default:
		return action.BlockedAggregate
	}
}
