package launch_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/launch"
	"github.com/rootkernel/gul/internal/launch/contractprovider"
	"google.golang.org/protobuf/proto"
)

type profilePort struct {
	profile *publicv1.ProfileProjection
	err     error
}

func (p profilePort) ListProfiles(context.Context, *publicv1.ListProfilesRequest) (*publicv1.ListProfilesResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &publicv1.ListProfilesResponse{Items: []*publicv1.ProfileProjection{proto.Clone(p.profile).(*publicv1.ProfileProjection)}}, nil
}
func (p profilePort) GetProfile(_ context.Context, request *publicv1.GetProfileRequest) (*publicv1.GetProfileResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	if request.GetProfileName() != p.profile.GetName() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("unknown profile"))
	}
	return &publicv1.GetProfileResponse{Profile: proto.Clone(p.profile).(*publicv1.ProfileProjection)}, nil
}

func candidateProfile() *publicv1.ProfileProjection {
	return &publicv1.ProfileProjection{Name: "profile", Compatibility: publicv1.ProfileCompatibility_PROFILE_COMPATIBILITY_COMPATIBLE,
		RuntimeVersion: proto.String("runtime-1"), Models: []*publicv1.ModelCapability{{ModelId: "model", IsDefault: true, SupportedEfforts: []string{"medium", "high"}}},
		SupportedExecutionLanes: []publicv1.ExecutionLane{publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, publicv1.ExecutionLane_EXECUTION_LANE_SHARED_READONLY},
		MaximumAssurance:        publicv1.AssuranceLevel_ASSURANCE_LEVEL_VERIFIED_THREAD_SCOPED_CONTROL,
		FeatureFlags:            []string{"safe_client_projection"}, AccessPolicyTransition: publicv1.SupportState_SUPPORT_STATE_UNVERIFIED,
		BackgroundExecution: &publicv1.BackgroundExecutionCapabilities{Support: publicv1.SupportState_SUPPORT_STATE_UNAVAILABLE}}
}

func choice() launch.Choice {
	return launch.Choice{ProfileName: "profile", ModelID: "model", Effort: "medium", Lane: "dedicated",
		RequiredAssurance: "best_effort_personal_alpha", PolicyName: "preprovisioned"}
}

func TestScenarioGlobalProfileAndExplicitProspectiveLaunch(t *testing.T) {
	h := scenario.New(time.Now())
	svc := launch.NewService(contractprovider.Provider{Port: h}, []string{"preprovisioned", "preprovisioned", ""})
	profiles, policies, err := svc.List(t.Context())
	if err != nil || len(profiles) != 1 || profiles[0].Name != "default" || profiles[0].Compatibility != "compatible" ||
		profiles[0].RuntimeVersion == "" || len(profiles[0].Models) != 1 || len(profiles[0].SupportedLanes) != 1 ||
		profiles[0].MaximumAssurance != "best_effort_personal_alpha" || len(policies) != 1 {
		t.Fatalf("global profile catalog = %+v, %+v, %v", profiles, policies, err)
	}
	selected := launch.Choice{ProfileName: "default", ModelID: "scenario-model", Effort: "medium", Lane: "dedicated",
		RequiredAssurance: "best_effort_personal_alpha", PolicyName: "preprovisioned"}
	configuration, err := svc.Check(t.Context(), selected)
	if err != nil || configuration.ControlMode != "direct_interactive" || configuration.Purpose != "interactive" ||
		configuration.ControllerKind != "interactive_client" || configuration.OrchestrationUseCase != "dolgorae_orchestrated_session" ||
		configuration.PolicyName != "preprovisioned" || configuration.Lane != "dedicated" || configuration.SharedReadOnlyWarning != "" {
		t.Fatalf("prospective launch = %+v, %v", configuration, err)
	}
	selected.Lane = "shared_readonly"
	selected.AcknowledgeSharedReadOnly = true
	if _, err := svc.Check(t.Context(), selected); !errors.Is(err, launch.ErrUnsupported) {
		t.Fatalf("unsupported scenario lane = %v", err)
	}
	selected.ProfileName = "removed"
	if _, err := svc.Check(t.Context(), selected); !errors.Is(err, launch.ErrInvalidChoice) {
		t.Fatalf("unknown scenario profile = %v", err)
	}
}

func TestSelectedProfileErrorActions(t *testing.T) {
	typedError := func(code string, action publicv1.RequiredClientAction, version uint32) error {
		err := connect.NewError(connect.CodeFailedPrecondition, errors.New("provider detail stays private"))
		detail, detailErr := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{
			DetailVersion: version, DolgoraeErrorCode: code, Action: action,
			RetryClassification:    publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN,
			RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE,
		})
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		err.AddDetail(detail)
		return err
	}
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{"removed profile", typedError("PROFILE_NOT_FOUND", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_SUPPORTED_PROFILE, 1), launch.ErrUnsupported},
		{"invalid choice", typedError("INVALID_ARGUMENT", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_REQUEST, 1), launch.ErrInvalidChoice},
		{"unknown detail version", typedError("PROFILE_NOT_FOUND", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_SUPPORTED_PROFILE, 2), launch.ErrInvalidProjection},
		{"unknown error code", typedError("FUTURE_PROFILE_ERROR", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_SUPPORTED_PROFILE, 1), launch.ErrInvalidProjection},
		{"missing typed detail", connect.NewError(connect.CodeInvalidArgument, errors.New("PROFILE_NOT_FOUND")), launch.ErrInvalidProjection},
		{"transport unavailable", connect.NewError(connect.CodeUnavailable, errors.New("offline")), launch.ErrProviderUnavailable},
		{"cancelled", context.Canceled, context.Canceled},
		{"deadline", context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := launch.NewService(contractprovider.Provider{Port: profilePort{err: test.err}}, []string{"preprovisioned"})
			if _, err := svc.Check(t.Context(), choice()); !errors.Is(err, test.want) {
				t.Fatalf("selected profile error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestUnsupportedChoicesAndSharedReadOnlyConsent(t *testing.T) {
	profile := candidateProfile()
	svc := launch.NewService(contractprovider.Provider{Port: profilePort{profile: profile}}, []string{"preprovisioned"})
	base := choice()
	for name, mutate := range map[string]func(*launch.Choice){
		"unknown model":      func(c *launch.Choice) { c.ModelID = "other" },
		"unsupported effort": func(c *launch.Choice) { c.Effort = "low" },
		"excess assurance":   func(c *launch.Choice) { c.RequiredAssurance = "strong_process_containment" },
		"unknown policy":     func(c *launch.Choice) { c.PolicyName = "not-preprovisioned" },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			if _, err := svc.Check(t.Context(), c); !errors.Is(err, launch.ErrUnsupported) {
				t.Fatalf("unsupported choice = %v", err)
			}
		})
	}
	for name, mutate := range map[string]func(*launch.Choice){
		"missing policy":    func(c *launch.Choice) { c.PolicyName = "" },
		"invalid lane":      func(c *launch.Choice) { c.Lane = "writer" },
		"invalid assurance": func(c *launch.Choice) { c.RequiredAssurance = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			if _, err := svc.Check(t.Context(), c); !errors.Is(err, launch.ErrInvalidChoice) {
				t.Fatalf("invalid choice = %v", err)
			}
		})
	}
	tooLong := base
	tooLong.ProfileName = strings.Repeat("p", 257)
	if _, err := svc.Check(t.Context(), tooLong); !errors.Is(err, launch.ErrInvalidChoice) {
		t.Fatalf("oversize profile name = %v", err)
	}
	base.Lane = "shared_readonly"
	if _, err := svc.Check(t.Context(), base); !errors.Is(err, launch.ErrInvalidChoice) {
		t.Fatalf("unacknowledged shared read-only = %v", err)
	}
	base.AcknowledgeSharedReadOnly = true
	configuration, err := svc.Check(t.Context(), base)
	if err != nil || configuration.SharedReadOnlyWarning != launch.SharedReadOnlyWarning || configuration.Lane != "shared_readonly" {
		t.Fatalf("acknowledged shared read-only = %+v, %v", configuration, err)
	}
	profile.Compatibility = publicv1.ProfileCompatibility_PROFILE_COMPATIBILITY_UNVERIFIED
	if _, err := svc.Check(t.Context(), base); !errors.Is(err, launch.ErrUnsupported) {
		t.Fatalf("unverified profile = %v", err)
	}
}

func TestProfileProjectionAndTransportFailClosed(t *testing.T) {
	for name, mutate := range map[string]func(*publicv1.ProfileProjection){
		"no efforts":         func(p *publicv1.ProfileProjection) { p.Models[0].SupportedEfforts = nil },
		"empty effort":       func(p *publicv1.ProfileProjection) { p.Models[0].SupportedEfforts = []string{""} },
		"no runtime version": func(p *publicv1.ProfileProjection) { p.RuntimeVersion = nil },
		"no maximum assurance": func(p *publicv1.ProfileProjection) {
			p.MaximumAssurance = publicv1.AssuranceLevel_ASSURANCE_LEVEL_UNSPECIFIED
		},
		"unknown interaction": func(p *publicv1.ProfileProjection) { p.SupportedInteractionKinds = []publicv1.InteractionKind{99} },
		"duplicate model": func(p *publicv1.ProfileProjection) {
			p.Models = append(p.Models, proto.Clone(p.Models[0]).(*publicv1.ModelCapability))
		},
		"duplicate effort": func(p *publicv1.ProfileProjection) { p.Models[0].SupportedEfforts = []string{"medium", "medium"} },
		"duplicate lane": func(p *publicv1.ProfileProjection) {
			p.SupportedExecutionLanes = []publicv1.ExecutionLane{publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED}
		},
	} {
		t.Run(name, func(t *testing.T) {
			profile := candidateProfile()
			mutate(profile)
			provider := contractprovider.Provider{Port: profilePort{profile: profile}}
			if _, err := provider.ListProfiles(t.Context()); !errors.Is(err, launch.ErrInvalidProjection) {
				t.Fatalf("malformed profile = %v", err)
			}
		})
	}
	profile := candidateProfile()
	profile.SupportedExecutionLanes = []publicv1.ExecutionLane{99}
	provider := contractprovider.Provider{Port: profilePort{profile: profile}}
	if _, err := provider.ListProfiles(t.Context()); !errors.Is(err, launch.ErrInvalidProjection) {
		t.Fatalf("unknown lane = %v", err)
	}
	oversize := candidateProfile()
	oversize.FeatureFlags = []string{strings.Repeat("x", 262144)}
	provider = contractprovider.Provider{Port: profilePort{profile: oversize}}
	if _, err := provider.ListProfiles(t.Context()); !errors.Is(err, launch.ErrInvalidProjection) {
		t.Fatalf("oversize profile list = %v", err)
	}
	if _, err := provider.GetProfile(t.Context(), "profile"); !errors.Is(err, launch.ErrInvalidProjection) {
		t.Fatalf("oversize selected profile = %v", err)
	}
	provider = contractprovider.Provider{Port: profilePort{profile: candidateProfile(), err: connect.NewError(connect.CodeUnavailable, errors.New("offline"))}}
	if _, err := provider.ListProfiles(t.Context()); !errors.Is(err, launch.ErrProviderUnavailable) {
		t.Fatalf("transport failure = %v", err)
	}
	provider = contractprovider.Provider{Port: profilePort{profile: candidateProfile(), err: connect.NewError(connect.CodePermissionDenied, errors.New("denied"))}}
	if _, err := provider.GetProfile(t.Context(), "profile"); !errors.Is(err, launch.ErrInvalidProjection) {
		t.Fatalf("semantic failure = %v", err)
	}
}
