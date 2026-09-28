package contractprovider

import (
	"context"
	"fmt"
	"slices"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/reconnect"
	"github.com/rootkernel/gul/internal/recovery"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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
	if caps == nil || proto.Size(caps) > 1024*1024 || !known(caps.ProtoReflect()) || caps.Context == nil || caps.Context.ProtocolVersion != 1 ||
		caps.Context.ServerInstanceId == "" || caps.Protocol == nil || caps.Protocol.RpcProtocolVersion != 1 ||
		caps.Protocol.MinimumClientProtocolVersion > 1 || caps.Protocol.MaximumClientProtocolVersion < 1 ||
		caps.Protocol.EventProtocolVersion != 1 || caps.Protocol.TimelineProtocolVersion != 1 ||
		caps.Protocol.EventProjectionVersion != 1 || caps.Protocol.GrpcErrorDetailVersion != 1 ||
		caps.DescriptorSha256 != port.DescriptorSHA256 || caps.Features == nil || !caps.Features.PersistentRuns ||
		!caps.Features.EventReplay || !caps.Features.ControllerTimeline || !caps.Features.SafeClientProjection ||
		!caps.Features.ControllerBinding || !caps.Features.PublicLocalSocket || !caps.Features.ReaderWriterAccess ||
		!caps.Features.DurableWriterAuthority || !caps.Features.ArtifactRetrieval || !caps.Features.ControlModes ||
		caps.ControllerCarrier == nil || caps.ControllerCarrier.SchemaId != "dolgorae.controller-credential/v1" ||
		caps.ControllerCarrier.SchemaVersion != 1 || caps.ControllerCarrier.SchemaSha256 != port.ControllerCredentialSchemaSHA256 ||
		!caps.ControllerCarrier.SameUidRequired || !caps.ControllerCarrier.RegularFileRequired ||
		!caps.ControllerCarrier.SymlinksForbidden ||
		caps.ControllerCarrier.CarrierRootPolicy != publicv1.ControllerCarrierRootPolicy_CONTROLLER_CARRIER_ROOT_POLICY_DOLGORAE_OWNED_HOME ||
		!slices.Contains(caps.ControllerCarrier.AcceptedControllerKinds, publicv1.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT) ||
		caps.Artifacts == nil || caps.Artifacts.MaximumArtifactSize == 0 || caps.Artifacts.MaximumChunkSize == 0 ||
		caps.Artifacts.MaximumInlineResponseBytes == 0 || !validInteractions(caps.Interactions) ||
		!caps.Artifacts.DigestVerificationRequired || !caps.Artifacts.ExactByteLengthReported ||
		!slices.Contains(caps.Artifacts.VisibilityClasses, publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY) ||
		!slices.Contains(caps.SupportedControlModes, publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE) ||
		!slices.Contains(caps.SupportedTransports, publicv1.PublicTransport_PUBLIC_TRANSPORT_LOCAL_GRPC) ||
		caps.ProfileLaunchMode != publicv1.ProfileLaunchMode_PROFILE_LAUNCH_MODE_DOLGORAE_OWNED_DIRECT_EXECUTABLE ||
		caps.AccessPolicyTransition == publicv1.SupportState_SUPPORT_STATE_UNSPECIFIED {
		return fmt.Errorf("%w: reconnect contract", recovery.ErrIncompatible)
	}
	visibility := make(map[publicv1.ArtifactVisibility]bool, len(caps.Artifacts.VisibilityClasses))
	for _, class := range caps.Artifacts.VisibilityClasses {
		if class == publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_UNSPECIFIED || visibility[class] {
			return fmt.Errorf("%w: artifact visibility", recovery.ErrIncompatible)
		}
		visibility[class] = true
	}
	methods := make(map[string]bool, len(caps.SupportedMethods))
	for _, method := range caps.SupportedMethods {
		if method == "" || methods[method] {
			return fmt.Errorf("%w: duplicate or empty method", recovery.ErrIncompatible)
		}
		methods[method] = true
	}
	for _, method := range port.RequiredMethods() {
		if !methods[method] {
			return fmt.Errorf("%w: missing %s", recovery.ErrIncompatible, method)
		}
	}
	return nil
}

func validInteractions(c *publicv1.InteractionCapabilities) bool {
	if c == nil || c.MaximumResponseBytes == 0 || c.MaximumSafePayloadBytes == 0 || len(c.KnownKinds) == 0 || len(c.Items) != len(c.KnownKinds) {
		return false
	}
	kinds := make(map[publicv1.InteractionKind]bool, len(c.KnownKinds))
	for _, kind := range c.KnownKinds {
		if kind < publicv1.InteractionKind_INTERACTION_KIND_COMMAND_EXECUTION_APPROVAL || kind > publicv1.InteractionKind_INTERACTION_KIND_UNSUPPORTED_REQUEST || kinds[kind] {
			return false
		}
		kinds[kind] = true
	}
	seen := make(map[publicv1.InteractionKind]bool, len(c.Items))
	for _, item := range c.Items {
		if item == nil || !kinds[item.Kind] || seen[item.Kind] || item.Support < publicv1.InteractionSupport_INTERACTION_SUPPORT_SUPPORTED ||
			item.Support > publicv1.InteractionSupport_INTERACTION_SUPPORT_UNAVAILABLE ||
			item.Kind == publicv1.InteractionKind_INTERACTION_KIND_UNSUPPORTED_REQUEST && item.Support == publicv1.InteractionSupport_INTERACTION_SUPPORT_SUPPORTED ||
			(item.Kind == publicv1.InteractionKind_INTERACTION_KIND_PERMISSION_REQUEST || item.Kind == publicv1.InteractionKind_INTERACTION_KIND_MCP_ELICITATION || item.Kind == publicv1.InteractionKind_INTERACTION_KIND_CONNECTOR_APPROVAL) && item.Support == publicv1.InteractionSupport_INTERACTION_SUPPORT_SUPPORTED {
			return false
		}
		seen[item.Kind] = true
	}
	return true
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
