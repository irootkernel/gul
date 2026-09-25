// Package launch validates a prospective Direct Session configuration without
// allocating a provider Run or Controller credential.
package launch

import (
	"context"
	"errors"
)

var (
	ErrInvalidChoice       = errors.New("invalid launch choice")
	ErrUnsupported         = errors.New("launch choice is unsupported")
	ErrProviderUnavailable = errors.New("profile provider unavailable")
	ErrInvalidProjection   = errors.New("invalid profile projection")
)

const SharedReadOnlyWarning = "Shared read-only access is permanent for this session. It cannot be changed to dedicated access."

type Model struct {
	ID               string
	Default          bool
	SupportedEfforts []string
}

type Profile struct {
	Name                   string
	Compatibility          string
	RuntimeVersion         string
	Models                 []Model
	SupportedLanes         []string
	MaximumAssurance       string
	FeatureFlags           []string
	SupportedInteractions  []string
	AccessPolicyTransition string
	BackgroundExecution    string
	NativeSubagentsEnabled bool
}

type Choice struct {
	ProfileName               string
	ModelID                   string
	Effort                    string
	Lane                      string
	RequiredAssurance         string
	PolicyName                string
	AcknowledgeSharedReadOnly bool
}

// Configuration is an explicit prospective request. E2-T3 revalidates it
// before StartRun and owns the actual mutation and carrier creation.
type Configuration struct {
	ProfileName           string
	ModelID               string
	Effort                string
	Lane                  string
	RequiredAssurance     string
	PolicyName            string
	ControlMode           string
	Purpose               string
	ControllerKind        string
	OrchestrationUseCase  string
	SharedReadOnlyWarning string
}

type Provider interface {
	ListProfiles(context.Context) ([]Profile, error)
	GetProfile(context.Context, string) (Profile, error)
}

type Service struct {
	provider Provider
	policies []string
}

// Policy names come only from trusted local preprovisioning configuration.
func NewService(provider Provider, policyNames []string) *Service {
	policies := make([]string, 0, len(policyNames))
	seen := make(map[string]bool)
	for _, name := range policyNames {
		if name != "" && !seen[name] {
			policies = append(policies, name)
			seen[name] = true
		}
	}
	return &Service{provider: provider, policies: policies}
}

func (s *Service) List(ctx context.Context) ([]Profile, []string, error) {
	if s == nil || s.provider == nil {
		return nil, nil, ErrProviderUnavailable
	}
	profiles, err := s.provider.ListProfiles(ctx)
	if err != nil {
		return nil, nil, err
	}
	return profiles, append([]string(nil), s.policies...), nil
}

func (s *Service) Check(ctx context.Context, choice Choice) (Configuration, error) {
	if s == nil || s.provider == nil {
		return Configuration{}, ErrProviderUnavailable
	}
	if choice.ProfileName == "" || choice.ModelID == "" || choice.Effort == "" || choice.PolicyName == "" ||
		len(choice.ProfileName) > 256 || len(choice.ModelID) > 256 || len(choice.Effort) > 256 || len(choice.PolicyName) > 256 ||
		(choice.Lane != "dedicated" && choice.Lane != "shared_readonly") || assuranceRank(choice.RequiredAssurance) == 0 {
		return Configuration{}, ErrInvalidChoice
	}
	if choice.Lane == "shared_readonly" && !choice.AcknowledgeSharedReadOnly {
		return Configuration{}, ErrInvalidChoice
	}
	if !contains(s.policies, choice.PolicyName) {
		return Configuration{}, ErrUnsupported
	}
	profile, err := s.provider.GetProfile(ctx, choice.ProfileName)
	if err != nil {
		return Configuration{}, err
	}
	if profile.Name != choice.ProfileName || profile.Compatibility == "" {
		return Configuration{}, ErrInvalidProjection
	}
	if profile.Compatibility != "compatible" || profile.RuntimeVersion == "" ||
		!contains(profile.SupportedLanes, choice.Lane) || assuranceRank(profile.MaximumAssurance) < assuranceRank(choice.RequiredAssurance) {
		return Configuration{}, ErrUnsupported
	}
	var selected *Model
	for index := range profile.Models {
		if profile.Models[index].ID == choice.ModelID {
			selected = &profile.Models[index]
			break
		}
	}
	if selected == nil || !contains(selected.SupportedEfforts, choice.Effort) {
		return Configuration{}, ErrUnsupported
	}
	configuration := Configuration{ProfileName: choice.ProfileName, ModelID: choice.ModelID, Effort: choice.Effort,
		Lane: choice.Lane, RequiredAssurance: choice.RequiredAssurance, PolicyName: choice.PolicyName,
		ControlMode: "direct_interactive", Purpose: "interactive", ControllerKind: "interactive_client",
		OrchestrationUseCase: "dolgorae_orchestrated_session"}
	if choice.Lane == "shared_readonly" {
		configuration.SharedReadOnlyWarning = SharedReadOnlyWarning
	}
	return configuration, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assuranceRank(value string) int {
	switch value {
	case "best_effort_personal_alpha":
		return 1
	case "verified_thread_scoped_control":
		return 2
	case "strong_process_containment":
		return 3
	default:
		return 0
	}
}
