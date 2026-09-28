package contractprovider

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/reconnect"
	"github.com/rootkernel/gul/internal/recovery"
)

type alteredRuntime struct {
	port.RuntimePort
	change func(*publicv1.GetCapabilitiesResponse)
}

func (r alteredRuntime) GetCapabilities(ctx context.Context, req *publicv1.GetCapabilitiesRequest) (*publicv1.GetCapabilitiesResponse, error) {
	capabilities, err := r.RuntimePort.GetCapabilities(ctx, req)
	if err == nil {
		r.change(capabilities)
	}
	return capabilities, err
}

func TestContractProbeRejectsProtocolAndMethodDrift(t *testing.T) {
	provider := scenario.New(time.Now())
	if err := (ContractProbe{}).Check(t.Context()); !errors.Is(err, reconnect.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := (ContractProbe{Port: provider}).Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*publicv1.GetCapabilitiesResponse){
		func(c *publicv1.GetCapabilitiesResponse) { c.Protocol.EventProtocolVersion++ },
		func(c *publicv1.GetCapabilitiesResponse) { c.DescriptorSha256 = "changed" },
		func(c *publicv1.GetCapabilitiesResponse) { c.SupportedMethods = nil },
		func(c *publicv1.GetCapabilitiesResponse) { c.Protocol.MaximumClientProtocolVersion = 0 },
		func(c *publicv1.GetCapabilitiesResponse) { c.Features.PersistentRuns = false },
		func(c *publicv1.GetCapabilitiesResponse) { c.Features.ArtifactRetrieval = false },
		func(c *publicv1.GetCapabilitiesResponse) { c.ControllerCarrier = nil },
		func(c *publicv1.GetCapabilitiesResponse) { c.ControllerCarrier.SchemaSha256 = "changed" },
		func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts.DigestVerificationRequired = false },
		func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts.MaximumInlineResponseBytes = 0 },
		func(c *publicv1.GetCapabilitiesResponse) { c.Interactions = nil },
		func(c *publicv1.GetCapabilitiesResponse) { c.Interactions.MaximumResponseBytes = 0 },
		func(c *publicv1.GetCapabilitiesResponse) { c.Interactions.Items = nil },
		func(c *publicv1.GetCapabilitiesResponse) { c.Interactions.Items[0].Support = -1 },
		func(c *publicv1.GetCapabilitiesResponse) {
			c.Artifacts.VisibilityClasses = append(c.Artifacts.VisibilityClasses, c.Artifacts.VisibilityClasses[0])
		},
		func(c *publicv1.GetCapabilitiesResponse) {
			c.SupportedMethods = append(c.SupportedMethods, c.SupportedMethods[0])
		},
		func(c *publicv1.GetCapabilitiesResponse) {
			c.AccessPolicyTransition = publicv1.SupportState_SUPPORT_STATE_UNSPECIFIED
		},
		func(c *publicv1.GetCapabilitiesResponse) { c.SupportedTransports = nil },
		func(c *publicv1.GetCapabilitiesResponse) {
			c.SupportedMethods = slices.DeleteFunc(c.SupportedMethods, func(method string) bool { return method == "WriterService.GetWorkspaceWriterStatus" })
		},
		func(c *publicv1.GetCapabilitiesResponse) {
			c.SupportedMethods = slices.DeleteFunc(c.SupportedMethods, func(method string) bool { return method == "RunService.CloseRun" })
		},
	} {
		err := (ContractProbe{Port: alteredRuntime{provider, change}}).Check(t.Context())
		if !errors.Is(err, recovery.ErrIncompatible) {
			t.Fatal(err)
		}
	}
}
