// Package contractprovider reads the pinned user-global Profile registry for
// prospective launch choices. It does not create a Run or carrier.
package contractprovider

import (
	"context"
	"errors"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/launch"
	"google.golang.org/protobuf/proto"
)

type Port interface {
	ListProfiles(context.Context, *publicv1.ListProfilesRequest) (*publicv1.ListProfilesResponse, error)
	GetProfile(context.Context, *publicv1.GetProfileRequest) (*publicv1.GetProfileResponse, error)
}

type Provider struct{ Port Port }

const maximumProfileProjectionBytes = 262144

func (p Provider) ListProfiles(ctx context.Context) ([]launch.Profile, error) {
	if p.Port == nil {
		return nil, launch.ErrProviderUnavailable
	}
	response, err := p.Port.ListProfiles(ctx, &publicv1.ListProfilesRequest{})
	if err != nil {
		return nil, readError(err)
	}
	if response == nil || len(response.GetItems()) > 100 || proto.Size(response) > maximumProfileProjectionBytes {
		return nil, launch.ErrInvalidProjection
	}
	profiles := make([]launch.Profile, 0, len(response.GetItems()))
	seen := make(map[string]bool)
	for _, item := range response.GetItems() {
		profile, err := translate(item)
		if err != nil || seen[profile.Name] {
			return nil, launch.ErrInvalidProjection
		}
		seen[profile.Name] = true
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (p Provider) GetProfile(ctx context.Context, name string) (launch.Profile, error) {
	if p.Port == nil {
		return launch.Profile{}, launch.ErrProviderUnavailable
	}
	if name == "" {
		return launch.Profile{}, launch.ErrInvalidChoice
	}
	response, err := p.Port.GetProfile(ctx, &publicv1.GetProfileRequest{ProfileName: name})
	if err != nil {
		mapped := port.MapProviderError(err)
		if mapped.Action == "USE_SUPPORTED_PROFILE" {
			return launch.Profile{}, launch.ErrUnsupported
		}
		if mapped.Code == "INVALID_REQUEST" && mapped.Action == "FIX_REQUEST" {
			return launch.Profile{}, launch.ErrInvalidChoice
		}
		return launch.Profile{}, readError(err)
	}
	if response == nil || proto.Size(response) > maximumProfileProjectionBytes {
		return launch.Profile{}, launch.ErrInvalidProjection
	}
	profile, err := translate(response.GetProfile())
	if err != nil || profile.Name != name {
		return launch.Profile{}, launch.ErrInvalidProjection
	}
	return profile, nil
}

func translate(source *publicv1.ProfileProjection) (launch.Profile, error) {
	if source == nil || source.GetName() == "" || len(source.GetName()) > 256 || len(source.GetRuntimeVersion()) > 256 ||
		len(source.GetModels()) > 100 || len(source.GetSupportedExecutionLanes()) > 2 || len(source.GetFeatureFlags()) > 128 ||
		len(source.GetSupportedInteractionKinds()) > 32 {
		return launch.Profile{}, launch.ErrInvalidProjection
	}
	compatibility := ""
	switch source.GetCompatibility() {
	case publicv1.ProfileCompatibility_PROFILE_COMPATIBILITY_COMPATIBLE:
		compatibility = "compatible"
	case publicv1.ProfileCompatibility_PROFILE_COMPATIBILITY_INCOMPATIBLE:
		compatibility = "incompatible"
	case publicv1.ProfileCompatibility_PROFILE_COMPATIBILITY_UNVERIFIED:
		compatibility = "unverified"
	case publicv1.ProfileCompatibility_PROFILE_COMPATIBILITY_UNAVAILABLE:
		compatibility = "unavailable"
	default:
		return launch.Profile{}, launch.ErrInvalidProjection
	}
	profile := launch.Profile{Name: source.GetName(), Compatibility: compatibility, RuntimeVersion: source.GetRuntimeVersion(),
		FeatureFlags: append([]string(nil), source.GetFeatureFlags()...), NativeSubagentsEnabled: source.GetNativeSubagents().GetEnabled()}
	seenModels := make(map[string]bool)
	for _, model := range source.GetModels() {
		if model == nil || model.GetModelId() == "" || len(model.GetModelId()) > 256 ||
			len(model.GetSupportedEfforts()) == 0 || len(model.GetSupportedEfforts()) > 16 || seenModels[model.GetModelId()] {
			return launch.Profile{}, launch.ErrInvalidProjection
		}
		seenModels[model.GetModelId()] = true
		seenEfforts := make(map[string]bool)
		for _, effort := range model.GetSupportedEfforts() {
			if effort == "" || len(effort) > 256 || seenEfforts[effort] {
				return launch.Profile{}, launch.ErrInvalidProjection
			}
			seenEfforts[effort] = true
		}
		profile.Models = append(profile.Models, launch.Model{ID: model.GetModelId(), Default: model.GetIsDefault(),
			SupportedEfforts: append([]string(nil), model.GetSupportedEfforts()...)})
	}
	seenLanes := make(map[string]bool)
	for _, lane := range source.GetSupportedExecutionLanes() {
		var selected string
		switch lane {
		case publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED:
			selected = "dedicated"
		case publicv1.ExecutionLane_EXECUTION_LANE_SHARED_READONLY:
			selected = "shared_readonly"
		default:
			return launch.Profile{}, launch.ErrInvalidProjection
		}
		if seenLanes[selected] {
			return launch.Profile{}, launch.ErrInvalidProjection
		}
		seenLanes[selected] = true
		profile.SupportedLanes = append(profile.SupportedLanes, selected)
	}
	switch source.GetMaximumAssurance() {
	case publicv1.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA:
		profile.MaximumAssurance = "best_effort_personal_alpha"
	case publicv1.AssuranceLevel_ASSURANCE_LEVEL_VERIFIED_THREAD_SCOPED_CONTROL:
		profile.MaximumAssurance = "verified_thread_scoped_control"
	case publicv1.AssuranceLevel_ASSURANCE_LEVEL_STRONG_PROCESS_CONTAINMENT:
		profile.MaximumAssurance = "strong_process_containment"
	default:
		if compatibility == "compatible" {
			return launch.Profile{}, launch.ErrInvalidProjection
		}
	}
	profile.AccessPolicyTransition = support(source.GetAccessPolicyTransition())
	profile.BackgroundExecution = support(source.GetBackgroundExecution().GetSupport())
	if profile.AccessPolicyTransition == "" || profile.BackgroundExecution == "" {
		return launch.Profile{}, launch.ErrInvalidProjection
	}
	for _, kind := range source.GetSupportedInteractionKinds() {
		if kind == publicv1.InteractionKind_INTERACTION_KIND_UNSPECIFIED {
			return launch.Profile{}, launch.ErrInvalidProjection
		}
		if _, ok := publicv1.InteractionKind_name[int32(kind)]; !ok {
			return launch.Profile{}, launch.ErrInvalidProjection
		}
		profile.SupportedInteractions = append(profile.SupportedInteractions, kind.String())
	}
	if compatibility == "compatible" && (profile.RuntimeVersion == "" || len(profile.Models) == 0 || len(profile.SupportedLanes) == 0) {
		return launch.Profile{}, launch.ErrInvalidProjection
	}
	return profile, nil
}

func support(value publicv1.SupportState) string {
	switch value {
	case publicv1.SupportState_SUPPORT_STATE_UNSPECIFIED:
		return "unknown"
	case publicv1.SupportState_SUPPORT_STATE_SUPPORTED:
		return "supported"
	case publicv1.SupportState_SUPPORT_STATE_UNAVAILABLE:
		return "unavailable"
	case publicv1.SupportState_SUPPORT_STATE_UNVERIFIED:
		return "unverified"
	case publicv1.SupportState_SUPPORT_STATE_UNSAFE:
		return "unsafe"
	default:
		return ""
	}
}

func readError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	mapped := port.MapProviderError(err)
	if mapped.Code == "TRANSPORT_UNAVAILABLE" || mapped.Code == "DEADLINE_EXCEEDED" {
		return launch.ErrProviderUnavailable
	}
	return launch.ErrInvalidProjection
}
