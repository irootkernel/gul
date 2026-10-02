package storage

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
)

func publicCredential(binding, key, controller string) ControllerCredential {
	return ControllerCredential{BindingReference: BindingReference{BindingID: binding, SubjectID: "owner", CredentialKey: key, ExpectedControllerID: controller, Health: "unknown"}, Generation: 1, InstanceID: "installation", FileIdentity: FileIdentity{Device: 1, Inode: 2, Size: 256, Modified: 1}}
}

func TestControllerAdoptionIsAtomicAndComparedToPriorBinding(t *testing.T) {
	s, bound := closeRefreshFixture(t)
	defer s.Close()
	r := s.Presentation()
	old := publicCredential(bound.Binding.ControllerBindingID, "old.json", "controller-id")
	if err := r.RegisterCredential(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	changed := old.BindingReference
	changed.CredentialKey = "retargeted.json"
	if err := r.PutBinding(t.Context(), changed); err == nil {
		t.Fatal("legacy upsert retargeted a protected credential")
	}
	conflict := publicCredential("other-binding", "other.json", "other-controller")
	if err := r.RegisterCredential(t.Context(), conflict); err != nil {
		t.Fatal(err)
	}
	candidate := publicCredential("adopted-binding", "other.json", "controller-id")
	if _, err := r.AdoptCredential(t.Context(), bound.Binding, candidate); !errors.Is(err, session.ErrBindingConflict) {
		t.Fatal("conflicting adoption accepted", err)
	}
	if _, err := r.ControllerCredential(t.Context(), "owner", old.BindingID); err != nil {
		t.Fatal("failed adoption removed prior credential", err)
	}
	candidate.CredentialKey = "adopted.json"
	candidate.AllocationStarted = true
	adopted, err := r.AdoptCredential(t.Context(), bound.Binding, candidate)
	if err != nil || adopted.ControllerBindingID != candidate.BindingID {
		t.Fatal("matching adoption failed", err)
	}
	if _, err = r.ControllerCredential(t.Context(), "owner", old.BindingID); err == nil {
		t.Fatal("prior binding remains authorized")
	}
	if mask := refreshMask(t, s, bound); mask != observation.AllAggregates|observation.Session|observation.Artifacts {
		t.Fatal("dependent projections were not invalidated", mask)
	}
	if _, err = r.AdoptCredential(t.Context(), bound.Binding, publicCredential("stale-binding", "stale.json", "controller-id")); !errors.Is(err, session.ErrBindingConflict) {
		t.Fatal("stale adoption replaced current binding", err)
	}
	if _, err = r.InsertBinding(t.Context(), session.Binding{SubjectID: "owner", ID: "second-session", WorkspaceID: adopted.WorkspaceID, RunID: "second-run", ControllerBindingID: candidate.BindingID, ProviderSessionID: "second-provider"}); !errors.Is(err, session.ErrBindingConflict) {
		t.Fatal("credential reused across Direct Sessions", err)
	}
	if again, err := r.InsertBinding(t.Context(), adopted); err != nil || again.ID != adopted.ID {
		t.Fatal("same Run rediscovery rejected", err)
	}
}

func TestControllerCleanupAndDurableAttemptExcludeEachOther(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	if err := s.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	c := publicCredential("unallocated", "new.json", "new-controller")
	if err := s.Presentation().RegisterCredential(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	attempt := MutationAttempt{OperationAttempt: OperationAttempt{OperationID: "allocation", SubjectID: "owner", Kind: "StartRun", RequestSHA256: strings.Repeat("a", 64), ReplayKey: "allocation", ReplayAvailable: true, ControllerReferences: []ControllerReference{{Role: "destination", BindingID: c.BindingID, CredentialKey: c.CredentialKey, ExpectedControllerID: c.ExpectedControllerID}}, State: "pending", CreatedAt: now}, TargetRef: "workspace", DeadlineAt: now.Add(time.Minute), ReconciliationRoute: "start_run"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var removal, dispatch error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		removal = s.Presentation().ReserveCredentialRemoval(t.Context(), "owner", c.BindingID)
	}()
	go func() { defer wg.Done(); <-start; _, _, dispatch = s.Attempts().BeginMutation(t.Context(), attempt) }()
	close(start)
	wg.Wait()
	if (removal == nil) == (dispatch == nil) {
		t.Fatal("cleanup and durable allocation did not exclude each other", removal, dispatch)
	}
	if removal == nil && s.Presentation().MarkCredentialAllocated(t.Context(), "owner", c.BindingID, "allocation", "workspace") == nil {
		t.Fatal("dispatch reserved a credential being removed")
	}
	if dispatch == nil && s.Presentation().ReserveCredentialRemoval(t.Context(), "owner", c.BindingID) == nil {
		t.Fatal("durable allocation lost its credential")
	}
}
