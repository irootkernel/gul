// Package session owns Gul's passive binding to provider Orchestrated Sessions.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rootkernel/gul/internal/workspace"
)

var (
	ErrInvalid                = errors.New("invalid session request")
	ErrNotFound               = errors.New("session not found")
	ErrUnavailable            = errors.New("session provider unavailable")
	ErrCarrierUnavailable     = errors.New("session controller carrier unavailable")
	ErrInvalidProjection      = errors.New("invalid provider session projection")
	ErrBindingConflict        = errors.New("primary run already bound")
	ErrPersistenceUnavailable = errors.New("session persistence unavailable")
)

const MaximumObservedMembers = 256
const maximumSnapshotBytes = 262144

// Carrier is resolved by a trusted backend credential store, never a browser.
type Carrier struct {
	AbsolutePath string
	ControllerID string
	Generation   uint64
}

type CarrierResolver interface {
	Resolve(context.Context, string, string) (Carrier, error)
}

type Binding struct {
	SubjectID           string
	ID                  string
	WorkspaceID         string
	RunID               string
	ControllerBindingID string
	ProviderSessionID   string
	Configuration       Configuration
}

type Configuration struct {
	ProfileName           string   `json:"profile_name"`
	Purpose               string   `json:"purpose"`
	PurposeLabel          string   `json:"purpose_label"`
	ModelID               string   `json:"model_id"`
	DefaultEffort         string   `json:"default_effort"`
	RequiredCapabilities  []string `json:"required_capabilities"`
	InstructionSchema     string   `json:"instruction_schema"`
	CommonPrefixVersion   uint32   `json:"common_prefix_version"`
	ModePrefixVersion     uint32   `json:"mode_prefix_version"`
	PurposePrefixVersion  uint32   `json:"purpose_prefix_version"`
	InstructionByteLength uint64   `json:"instruction_byte_length"`
	InstructionSHA256     string   `json:"instruction_sha256"`
}

type Member struct {
	RunID     string
	Lifecycle string
}

// Counts are present only after an authoritative aggregate read.
type Counts struct {
	NonretiredMembers       uint64
	NonterminalSpawns       uint64
	PendingApprovals        uint64
	AcceptedUnfinishedTasks uint64
	UnknownOutcomeTasks     uint64
	PublishedResults        uint64
}

type Snapshot struct {
	ProviderSessionID    string
	PrimaryRunID         string
	ProviderWorkspaceID  string
	Configuration        Configuration
	Lifecycle            string
	Composition          string
	ApprovalPolicy       string
	SpecialistPolicyName string
	Counts               Counts
	CloseProgress        string
	Recovery             string
	AggregateRevision    uint64
	RunRevision          uint64
	ObservedAt           time.Time
	Members              []Member // Observable parent links, never aggregate membership authority.
	MembersTruncated     bool
}

type ExecutionState struct {
	SessionID    string
	Freshness    string
	ObservedAt   time.Time
	StateVersion string
	Snapshot     *Snapshot
}

type Repository interface {
	InsertBinding(context.Context, Binding) (Binding, error)
	Binding(context.Context, string, string) (Binding, error)
	ListBindings(context.Context, string, string) ([]Binding, error)
	UpdateSnapshot(context.Context, Binding, Snapshot) error
}

type Provider interface {
	Snapshot(context.Context, workspace.Attachment, string, Carrier) (Snapshot, error)
}

type Workspace interface {
	Revalidate(context.Context, string, string) (workspace.Attachment, error)
}

type Service struct {
	workspaces   Workspace
	repo         Repository
	provider     Provider
	carriers     CarrierResolver
	mu           sync.Mutex
	flights      map[string]*flight
	last         map[string]ExecutionState
	fingerprints map[string][32]byte
	cacheOrder   []string
}

type flight struct {
	done  chan struct{}
	state ExecutionState
	err   error
}

func NewService(workspaces Workspace, repo Repository, provider Provider, carriers CarrierResolver) *Service {
	return &Service{workspaces: workspaces, repo: repo, provider: provider, carriers: carriers,
		flights: make(map[string]*flight), last: make(map[string]ExecutionState), fingerprints: make(map[string][32]byte)}
}

// BindPrimary is a trusted application hook for an accepted provider Primary.
// It never accepts a browser-supplied Run ID or credential reference.
func (s *Service) BindPrimary(ctx context.Context, subjectID, workspaceID, runID, controllerBindingID string) (Binding, error) {
	if s == nil || s.repo == nil || s.workspaces == nil || s.provider == nil || s.carriers == nil ||
		subjectID == "" || workspaceID == "" || runID == "" || controllerBindingID == "" {
		return Binding{}, ErrInvalid
	}
	attachment, carrier, err := s.authority(ctx, subjectID, workspaceID, controllerBindingID)
	if err != nil {
		return Binding{}, err
	}
	snapshot, err := s.snapshot(ctx, attachment, runID, carrier)
	if err != nil {
		return Binding{}, err
	}
	id, err := newID()
	if err != nil {
		return Binding{}, err
	}
	binding := Binding{SubjectID: subjectID, ID: id, WorkspaceID: workspaceID, RunID: runID,
		ControllerBindingID: controllerBindingID, ProviderSessionID: snapshot.ProviderSessionID, Configuration: snapshot.Configuration}
	binding, err = s.repo.InsertBinding(ctx, binding)
	if err != nil {
		return Binding{}, repositoryError(err)
	}
	if err := s.repo.UpdateSnapshot(ctx, binding, snapshot); err != nil {
		return Binding{}, repositoryError(err)
	}
	binding.Configuration = snapshot.Configuration
	return binding, nil
}

// List returns only locally bound sessions for this subject and Workspace.
func (s *Service) List(ctx context.Context, subjectID, workspaceID string) ([]Binding, error) {
	if s == nil || s.repo == nil || subjectID == "" || workspaceID == "" {
		return nil, ErrInvalid
	}
	bindings, err := s.repo.ListBindings(ctx, subjectID, workspaceID)
	return bindings, repositoryError(err)
}

func (s *Service) Open(ctx context.Context, subjectID, sessionID string) (Binding, ExecutionState, error) {
	if s == nil || s.repo == nil || subjectID == "" || sessionID == "" {
		return Binding{}, ExecutionState{}, ErrInvalid
	}
	binding, err := s.repo.Binding(ctx, subjectID, sessionID)
	if err != nil {
		return Binding{}, ExecutionState{}, repositoryError(err)
	}
	state, err := s.GetExecutionState(ctx, subjectID, sessionID)
	if err != nil {
		return Binding{}, ExecutionState{}, err
	}
	if state.Snapshot != nil {
		binding.Configuration = state.Snapshot.Configuration
	}
	return binding, state, nil
}

// GetExecutionState coalesces concurrent reads of the same subject/session and
// limits each authoritative refresh. A cached state is never returned as fresh.
func (s *Service) GetExecutionState(ctx context.Context, subjectID, sessionID string) (ExecutionState, error) {
	if s == nil || s.repo == nil || s.workspaces == nil || s.provider == nil || s.carriers == nil || subjectID == "" || sessionID == "" {
		return ExecutionState{}, ErrInvalid
	}
	key := subjectID + "\x00" + sessionID
	s.mu.Lock()
	if pending := s.flights[key]; pending != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ExecutionState{}, ctx.Err()
		case <-pending.done:
			return pending.state, pending.err
		}
	}
	pending := &flight{done: make(chan struct{})}
	s.flights[key] = pending
	s.mu.Unlock()
	go func() {
		state, err := s.refresh(context.WithoutCancel(ctx), subjectID, sessionID, key)
		s.mu.Lock()
		pending.state, pending.err = state, err
		delete(s.flights, key)
		close(pending.done)
		s.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return ExecutionState{}, ctx.Err()
	case <-pending.done:
		return pending.state, pending.err
	}
}

func (s *Service) refresh(ctx context.Context, subjectID, sessionID, key string) (ExecutionState, error) {
	binding, err := s.repo.Binding(ctx, subjectID, sessionID)
	if err != nil {
		return ExecutionState{}, repositoryError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	attachment, carrier, err := s.authority(ctx, subjectID, binding.WorkspaceID, binding.ControllerBindingID)
	if err == nil {
		var snapshot Snapshot
		snapshot, err = s.snapshot(ctx, attachment, binding.RunID, carrier)
		if err == nil && snapshot.ProviderSessionID != binding.ProviderSessionID {
			err = ErrInvalidProjection
		}
		if err == nil {
			if err := s.repo.UpdateSnapshot(ctx, binding, snapshot); err != nil {
				return ExecutionState{}, repositoryError(err)
			}
		}
		if err == nil {
			state := ExecutionState{SessionID: sessionID, Freshness: "fresh", ObservedAt: snapshot.ObservedAt, Snapshot: &snapshot}
			withoutTime := snapshot
			withoutTime.ObservedAt = time.Time{}
			encoded, marshalErr := json.Marshal(withoutTime)
			if marshalErr != nil {
				return ExecutionState{}, marshalErr
			}
			fingerprint := sha256.Sum256(encoded)
			s.mu.Lock()
			if previous, same := s.fingerprints[key]; same && previous == fingerprint {
				state.StateVersion = s.last[key].StateVersion
			} else {
				state.StateVersion, err = newID()
			}
			if err == nil {
				if _, exists := s.last[key]; !exists {
					if len(s.cacheOrder) == 256 {
						oldest := s.cacheOrder[0]
						delete(s.last, oldest)
						delete(s.fingerprints, oldest)
						s.cacheOrder = s.cacheOrder[1:]
					}
					s.cacheOrder = append(s.cacheOrder, key)
				}
				s.last[key] = state
				s.fingerprints[key] = fingerprint
			}
			s.mu.Unlock()
			if err != nil {
				return ExecutionState{}, err
			}
			return state, nil
		}
	}
	if !errors.Is(err, ErrUnavailable) {
		return ExecutionState{}, err
	}
	s.mu.Lock()
	cached, ok := s.last[key]
	s.mu.Unlock()
	if ok {
		cached.Freshness = "stale"
		return cached, nil
	}
	return ExecutionState{SessionID: sessionID, Freshness: "unavailable"}, nil
}

func (s *Service) authority(ctx context.Context, subjectID, workspaceID, bindingID string) (workspace.Attachment, Carrier, error) {
	attachment, err := s.workspaces.Revalidate(ctx, subjectID, workspaceID)
	if err != nil {
		if errors.Is(err, workspace.ErrProviderUnavailable) || errors.Is(err, workspace.ErrProfileServerUnavailable) || errors.Is(err, context.DeadlineExceeded) {
			return workspace.Attachment{}, Carrier{}, ErrUnavailable
		}
		return workspace.Attachment{}, Carrier{}, err
	}
	carrier, err := s.carriers.Resolve(ctx, subjectID, bindingID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return workspace.Attachment{}, Carrier{}, ErrUnavailable
		}
		if ctx.Err() != nil {
			return workspace.Attachment{}, Carrier{}, ctx.Err()
		}
		return workspace.Attachment{}, Carrier{}, fmt.Errorf("%w: %v", ErrCarrierUnavailable, err)
	}
	if carrier.AbsolutePath == "" || carrier.ControllerID == "" {
		return workspace.Attachment{}, Carrier{}, ErrInvalid
	}
	return attachment, carrier, nil
}

func (s *Service) snapshot(ctx context.Context, attachment workspace.Attachment, runID string, carrier Carrier) (Snapshot, error) {
	snapshot, err := s.provider.Snapshot(ctx, attachment, runID, carrier)
	if err != nil {
		if errors.Is(err, ErrInvalidProjection) {
			return Snapshot{}, err
		}
		return Snapshot{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if snapshot.ProviderWorkspaceID != attachment.ProviderID || snapshot.PrimaryRunID != runID ||
		snapshot.ProviderSessionID == "" || snapshot.AggregateRevision == 0 || snapshot.RunRevision == 0 ||
		snapshot.ObservedAt.IsZero() || snapshot.Lifecycle == "" || snapshot.Composition == "" ||
		snapshot.ApprovalPolicy == "" || snapshot.CloseProgress == "" || snapshot.Recovery == "" {
		return Snapshot{}, ErrInvalidProjection
	}
	if len(snapshot.Members) > MaximumObservedMembers || (snapshot.MembersTruncated && len(snapshot.Members) != MaximumObservedMembers) {
		return Snapshot{}, ErrInvalidProjection
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || len(encoded) > maximumSnapshotBytes {
		return Snapshot{}, ErrInvalidProjection
	}
	return snapshot, nil
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func repositoryError(err error) error {
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrInvalidProjection) ||
		errors.Is(err, ErrBindingConflict) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrPersistenceUnavailable, err)
}
