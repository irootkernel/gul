package session_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type statusProvider struct {
	fixedProvider
	err       error
	lifecycle string
	recovery  string
}

func (p *statusProvider) Snapshot(ctx context.Context, attachment workspace.Attachment, runID string, carrier session.Carrier) (session.Snapshot, error) {
	if p.err != nil {
		return session.Snapshot{}, p.err
	}
	snapshot, err := p.fixedProvider.Snapshot(ctx, attachment, runID, carrier)
	if p.lifecycle != "" {
		snapshot.Lifecycle = p.lifecycle
	}
	if p.recovery != "" {
		snapshot.Recovery = p.recovery
	}
	return snapshot, err
}

func providerStatusService(p *statusProvider) *session.Service {
	repo := &memoryRepository{bindings: map[string]session.Binding{
		"owner/session": {SubjectID: "owner", ID: "session", WorkspaceID: "workspace", RunID: "run", ControllerBindingID: "controller", ProviderSessionID: "provider-session"},
	}}
	return session.NewService(workspaces{"owner/workspace": {ID: "workspace", ProviderID: "provider-workspace"}}, repo, p,
		carriers{"owner/controller": {AbsolutePath: "/carrier", ControllerID: "controller", Generation: 1}})
}

func TestProviderFailuresRetainOnlyExplicitlyStaleSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		state session.ProviderState
	}{
		{"disconnected", session.ErrUnavailable, session.ProviderDisconnected},
		{"incompatible", session.ErrIncompatible, session.ProviderIncompatible},
		{"busy", session.ErrBusy, session.ProviderBusy},
		{"degraded", session.ErrDegraded, session.ProviderDegraded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &statusProvider{}
			svc := providerStatusService(provider)
			fresh, err := svc.GetExecutionState(t.Context(), "owner", "session")
			if err != nil || fresh.ProviderState != session.ProviderReady || fresh.Freshness != "fresh" || fresh.Snapshot == nil {
				t.Fatalf("initial state = %+v, %v", fresh, err)
			}
			provider.err = tc.err
			stale, err := svc.GetExecutionState(t.Context(), "owner", "session")
			if err != nil || stale.ProviderState != tc.state || stale.Freshness != "stale" || stale.StateVersion != fresh.StateVersion || !reflect.DeepEqual(stale.Snapshot, fresh.Snapshot) {
				t.Fatalf("failed observation = %+v, %v", stale, err)
			}
			unavailable, err := providerStatusService(provider).GetExecutionState(t.Context(), "owner", "session")
			if err != nil || unavailable.ProviderState != tc.state || unavailable.Freshness != "unavailable" || unavailable.Snapshot != nil || unavailable.StateVersion != "" {
				t.Fatalf("uncached observation = %+v, %v", unavailable, err)
			}
			provider.err = nil
			recovered, err := svc.GetExecutionState(t.Context(), "owner", "session")
			if err != nil || recovered.ProviderState != session.ProviderReady || recovered.Freshness != "fresh" || recovered.Snapshot.Lifecycle != fresh.Snapshot.Lifecycle {
				t.Fatalf("restored observation = %+v, %v", recovered, err)
			}
		})
	}
}

func TestProviderIdentityAndCredentialFailureNeverDiscloseCachedSnapshot(t *testing.T) {
	for _, failure := range []error{session.ErrInvalidProjection, session.ErrCarrierUnavailable} {
		provider := &statusProvider{}
		svc := providerStatusService(provider)
		if _, err := svc.GetExecutionState(t.Context(), "owner", "session"); err != nil {
			t.Fatal(err)
		}
		provider.err = failure
		state, err := svc.GetExecutionState(t.Context(), "owner", "session")
		if !errors.Is(err, failure) || state.Snapshot != nil {
			t.Fatalf("authority failure = %+v, %v", state, err)
		}
	}
}

func TestAuthoritativeRecoverySnapshotRemainsFreshAndDegraded(t *testing.T) {
	for _, provider := range []*statusProvider{{lifecycle: "degraded"}, {lifecycle: "recovering"}, {recovery: "outcome_unknown"}} {
		state, err := providerStatusService(provider).GetExecutionState(t.Context(), "owner", "session")
		if err != nil || state.ProviderState != session.ProviderDegraded || state.Freshness != "fresh" || state.Snapshot == nil {
			t.Fatalf("authoritative recovery = %+v, %v", state, err)
		}
	}
}
