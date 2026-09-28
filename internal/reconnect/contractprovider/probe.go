package contractprovider

import (
	"context"
	"fmt"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/reconnect"
	"github.com/rootkernel/gul/internal/recovery"
	"google.golang.org/protobuf/proto"
)

// ContractProbe uses the pinned public local-gRPC handshake. It is a checked
// adapter, not a live transport or a Machine CLI fallback.
type ContractProbe struct{ Port port.RuntimePort }

func (p ContractProbe) Check(ctx context.Context) error {
	if p.Port == nil {
		return reconnect.ErrUnavailable
	}
	caps, err := p.Port.GetCapabilities(ctx, &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		return err
	}
	if caps == nil || proto.Size(caps) > 1024*1024 || caps.Context == nil || caps.Context.ProtocolVersion != 1 ||
		caps.Context.ServerInstanceId == "" || caps.Protocol == nil || caps.Protocol.RpcProtocolVersion != 1 ||
		caps.Protocol.MinimumClientProtocolVersion > 1 || caps.Protocol.MaximumClientProtocolVersion < 1 ||
		caps.Protocol.EventProtocolVersion != 1 || caps.Protocol.TimelineProtocolVersion != 1 ||
		caps.Protocol.EventProjectionVersion != 1 || caps.Protocol.GrpcErrorDetailVersion != 1 ||
		caps.DescriptorSha256 != port.DescriptorSHA256 || caps.Features == nil || !caps.Features.PersistentRuns ||
		!caps.Features.EventReplay || !caps.Features.ControllerTimeline || !caps.Features.SafeClientProjection ||
		!caps.Features.ControllerBinding || !caps.Features.PublicLocalSocket {
		return fmt.Errorf("%w: reconnect contract", recovery.ErrIncompatible)
	}
	methods := make(map[string]bool, len(caps.SupportedMethods))
	for _, method := range caps.SupportedMethods {
		methods[method] = true
	}
	for _, method := range []string{"RunService.GetRun", "ObservationService.WatchRunEvents", "ObservationService.ListRunTimelineItems", "WriterService.GetWorkspaceWriterStatus", "InteractionService.ListPendingInteractions"} {
		if !methods[method] {
			return fmt.Errorf("%w: missing %s", recovery.ErrIncompatible, method)
		}
	}
	return nil
}
