package action

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type actionRepo struct{ operation OperationState }

func (actionRepo) Converge(context.Context, Bound, Input) (bool, error) { return true, nil }

type nonConvergingRepo struct {
	actionRepo
	err error
}

type failingLocalRepo struct {
	actionRepo
	err error
}

func (r failingLocalRepo) LocalState(context.Context, Bound) (LocalState, error) {
	return LocalState{}, r.err
}

func (r nonConvergingRepo) Converge(context.Context, Bound, Input) (bool, error) {
	return false, r.err
}

func (r actionRepo) Binding(_ context.Context, subject, id string) (session.Binding, error) {
	if subject != "owner" {
		return session.Binding{}, ErrAuthority
	}
	return session.Binding{SubjectID: subject, ID: id, WorkspaceID: "ws", RunID: id, ControllerBindingID: id, ProviderSessionID: "aggregate"}, nil
}
func (r actionRepo) LocalState(context.Context, Bound) (LocalState, error) {
	return LocalState{Ownership: OwnedSession, Operation: r.operation}, nil
}

type actionWorkspace struct{}

func (actionWorkspace) Revalidate(context.Context, string, string) (workspace.Attachment, error) {
	return workspace.Attachment{ProviderID: "provider", CanonicalRoot: "/workspace"}, nil
}

type actionCarrier struct{ missing bool }

type actionAttempts struct{}

func (actionAttempts) BeginWriter(context.Context, Bound, bool, uint64) (string, error) {
	return "attempt", nil
}
func (actionAttempts) FinishWriter(context.Context, string, string) error { return nil }

type failingWriterAttempts struct{ actionAttempts }

func (failingWriterAttempts) FinishWriter(context.Context, string, string) error {
	return errors.New("disk unavailable")
}

func (c actionCarrier) Resolve(_ context.Context, _, id string) (session.Carrier, error) {
	if c.missing {
		return session.Carrier{}, ErrAuthority
	}
	return session.Carrier{ControllerID: id, Generation: 1, AbsolutePath: "/private/carrier"}, nil
}

type actionProvider struct {
	mu               sync.Mutex
	input            Input
	acquire, release int
	fail             error
	revision         uint64
	owner            string
}

func (p *actionProvider) Read(_ context.Context, b Bound) (Input, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	in := p.input
	if p.owner != "" {
		in = writerInput()
		if p.owner != b.Binding.RunID {
			in.Writer.Owner = ExternalOwner
			in.Run = readerInput().Run
		}
	}
	if p.revision > 0 {
		in.Run.Stamp.Run = p.revision
		in.Run.Stamp.Head = observationCursor(p.revision)
		in.InteractionStamp = in.Run.Stamp
		in.TimelineHead = in.Run.Stamp.Head
		if in.Writer.Owner == ThisSession {
			in.Writer.Stamp = in.Run.Stamp
		}
	}
	return in, nil
}
func (p *actionProvider) Acquire(_ context.Context, b Bound, revision uint64) (WriterProjection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.acquire++
	if p.fail != nil {
		return WriterProjection{}, p.fail
	}
	if p.owner != "" {
		return WriterProjection{}, ErrWriterBusy
	}
	if revision != p.input.Run.Stamp.Run {
		return WriterProjection{}, ErrBlocked
	}
	p.owner = b.Binding.RunID
	p.revision = revision + 1
	out := writerInput().Writer
	out.Stamp.Run = p.revision
	out.Stamp.Head = observationCursor(p.revision)
	return out, nil
}
func (p *actionProvider) Release(_ context.Context, b Bound, revision uint64) (WriterProjection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.release++
	if p.owner != b.Binding.RunID || revision != p.revision {
		return WriterProjection{}, ErrWriterBusy
	}
	p.owner = ""
	p.revision++
	return readerInput().Writer, nil
}
func actionService(p *actionProvider) *Service {
	return &Service{Repository: actionRepo{NoOperation}, Workspaces: actionWorkspace{}, Carriers: actionCarrier{}, Provider: p, Attempts: actionAttempts{}, Gate: func(string, string) bool { return true }}
}
func TestStartupGateBlocksMutationBeforeObservationResume(t *testing.T) {
	provider := &actionProvider{input: readerInput()}
	service := actionService(provider)
	service.Gate = nil
	result, err := service.Evaluate(t.Context(), "owner", "session", Request{Intent: IntentWrite, CloseIntent: NoCloseIntent})
	if err != nil || !result.Flags.RequiresFreshSnapshot || result.Flags.CanAcquireWriter {
		t.Fatal(result, err)
	}
	if _, err := service.Acquire(t.Context(), "owner", "session"); err == nil || provider.acquire != 0 {
		t.Fatal(err, provider.acquire)
	}
	service.Gate = func(string, string) bool { return true }
	result, err = service.Evaluate(t.Context(), "owner", "session", Request{Intent: IntentWrite, CloseIntent: NoCloseIntent})
	if err != nil || !result.Flags.CanAcquireWriter {
		t.Fatal(result, err)
	}
}
func TestStartupGateClosingAfterReadBlocksWriterDispatch(t *testing.T) {
	provider := &actionProvider{input: readerInput()}
	service := actionService(provider)
	checks := 0
	service.Gate = func(subject, sessionID string) bool {
		if subject != "owner" || sessionID != "session" {
			t.Fatal(subject, sessionID)
		}
		checks++
		return checks < 3
	}
	result, err := service.Acquire(t.Context(), "owner", "session")
	if err != ErrBlocked || result.Evaluation.Blocker != FreshSnapshotRequired || provider.acquire != 0 || checks != 3 {
		t.Fatal(result, err, provider.acquire, checks)
	}
}
func TestServiceRejectsNonConvergedSnapshotsBeforeWriterDispatch(t *testing.T) {
	for _, fault := range []error{nil, errors.New("cache fault")} {
		provider := &actionProvider{input: readerInput()}
		service := actionService(provider)
		service.Repository = nonConvergingRepo{actionRepo: actionRepo{NoOperation}, err: fault}
		evaluation, err := service.Evaluate(t.Context(), "owner", "session", Request{Intent: IntentWrite, CloseIntent: NoCloseIntent})
		if fault == nil {
			if err != nil || evaluation.Blocker != FreshSnapshotRequired || !evaluation.Flags.RequiresFreshSnapshot {
				t.Fatal(evaluation, err)
			}
		} else if err != ErrPersistence || evaluation.Blocker != FreshSnapshotRequired || !evaluation.Flags.RequiresFreshSnapshot || evaluation.Mode != WriterBlocked {
			t.Fatal(evaluation, err)
		}
		if _, err := service.Acquire(t.Context(), "owner", "session"); err == nil || provider.acquire != 0 {
			t.Fatal(err, provider.acquire)
		}
	}
}
func TestServiceClassifiesLocalStateFailureBeforeWriterDispatch(t *testing.T) {
	for _, fault := range []error{errors.New("disk fault"), ErrAuthority} {
		provider := &actionProvider{input: readerInput()}
		service := actionService(provider)
		service.Repository = failingLocalRepo{actionRepo: actionRepo{NoOperation}, err: fault}
		_, err := service.Evaluate(t.Context(), "owner", "session", Request{Intent: IntentWrite, CloseIntent: NoCloseIntent})
		want := ErrPersistence
		if errors.Is(fault, ErrAuthority) {
			want = ErrAuthority
		}
		if !errors.Is(err, want) {
			t.Fatal(err, want)
		}
		if _, err := service.Acquire(t.Context(), "owner", "session"); err == nil || provider.acquire != 0 {
			t.Fatal(err, provider.acquire)
		}
	}
}
func TestGuardedWriterCallsAndSeparateUnownedWindow(t *testing.T) {
	p := &actionProvider{input: readerInput()}
	s := actionService(p)
	accepted, err := s.Acquire(t.Context(), "owner", "first")
	if err != nil || accepted.Accepted.Owner != ThisSession || !accepted.Evaluation.Flags.RequiresFreshSnapshot || p.acquire != 1 {
		t.Fatal(accepted, err)
	}
	if _, err := s.Acquire(t.Context(), "owner", "second"); err != ErrWriterBusy || p.acquire != 1 {
		t.Fatal("takeover reached RPC", err, p.acquire)
	}
	released, err := s.Release(t.Context(), "owner", "first")
	if err != nil || released.Accepted.Owner != NoOwner || p.release != 1 {
		t.Fatal(released, err)
	}
	state, err := s.Evaluate(t.Context(), "owner", "second", Request{Intent: IntentWrite, CloseIntent: NoCloseIntent})
	if err != nil || state.Writer.Owner != NoOwner || !state.Flags.CanAcquireWriter {
		t.Fatal("unowned window hidden", state, err)
	}
	// Another Controller can win this window. Gul does not reserve or enqueue.
	p.fail = ErrWriterBusy
	if _, err := s.Acquire(t.Context(), "owner", "second"); err != ErrWriterBusy {
		t.Fatal(err)
	}
	if p.acquire != 2 || p.release != 1 {
		t.Fatal("automatic transfer/retry", p.acquire, p.release)
	}
}
func TestThreadlessAndActiveTurnAreRejectedBeforeWriterRPC(t *testing.T) {
	for _, change := range []func(*Input){func(in *Input) { in.Run.Thread = Missing; in.Run.Variant = DedicatedUnstarted }, func(in *Input) { in.Run.ActiveTurn = Present; in.Run.Lifecycle = Running }} {
		p := &actionProvider{input: readerInput()}
		change(&p.input)
		s := actionService(p)
		if _, err := s.Acquire(t.Context(), "owner", "session"); err == nil || p.acquire != 0 {
			t.Fatal("blocked acquire invoked", err, p.acquire)
		}
	}
	p := &actionProvider{input: readerInput()}
	p.input.Run.ActiveTurn = Present
	p.input.Run.Lifecycle = WaitingInteraction
	p.input.Run.Pending = 1
	s := actionService(p)
	if result, err := s.RequireSubmit(t.Context(), "owner", "session", IntentRead); err == nil || result.Blocker != ActiveTurnDraft {
		t.Fatal(result, err)
	}
	if result, err := s.InteractionActions(t.Context(), "owner", "session"); err != nil || !result.Flags.CanResolveInteraction {
		t.Fatal(result, err)
	}
}
func TestCredentialAndOperationGuardsNeverInvokeProviderMutation(t *testing.T) {
	for _, missing := range []bool{false, true} {
		p := &actionProvider{input: readerInput()}
		s := actionService(p)
		if missing {
			s.Carriers = actionCarrier{missing: true}
		} else {
			s.Repository = actionRepo{OperationUnknown}
		}
		if _, err := s.Acquire(t.Context(), "owner", "session"); err == nil || p.acquire != 0 {
			t.Fatal(err, p.acquire)
		}
	}
	p := &actionProvider{input: readerInput()}
	s := actionService(p)
	if _, err := s.Acquire(t.Context(), "foreign", "session"); err != ErrAuthority || p.acquire != 0 {
		t.Fatal(err)
	}
}

func observationCursor(revision uint64) observation.Cursor {
	return observation.Cursor(strconv.FormatUint(revision, 10))
}

func TestWriterAttemptPersistenceFailureFailsClosedAfterDispatch(t *testing.T) {
	for _, providerError := range []error{nil, ErrUnavailable} {
		p := &actionProvider{input: readerInput(), fail: providerError}
		s := actionService(p)
		s.Attempts = failingWriterAttempts{}
		_, err := s.Acquire(t.Context(), "owner", "session")
		want := ErrOutcomeUnknown
		if providerError != nil {
			want = ErrPersistence
		}
		if !errors.Is(err, want) || p.acquire != 1 {
			t.Fatalf("post-dispatch persistence failure = %v, calls=%d", err, p.acquire)
		}
	}
	p := &actionProvider{input: readerInput()}
	s := actionService(p)
	s.Attempts = nil
	if _, err := s.Acquire(t.Context(), "owner", "session"); !errors.Is(err, ErrPersistence) || p.acquire != 0 {
		t.Fatalf("missing attempt store dispatched = %v, calls=%d", err, p.acquire)
	}
}

func TestAmbiguousWriterFailureIsNeverRetried(t *testing.T) {
	p := &actionProvider{input: readerInput(), fail: ErrUnavailable}
	s := actionService(p)
	if _, err := s.Acquire(t.Context(), "owner", "session"); err != ErrOutcomeUnknown || p.acquire != 1 {
		t.Fatal(err, p.acquire)
	}
}

func TestStaleAggregateRejectsBeforeWriterAndSubmitAdmission(t *testing.T) {
	for _, change := range []func(*Input){func(in *Input) { in.Aggregate.Freshness = Stale }, func(in *Input) { in.Aggregate.Freshness = UnavailableSnapshot }, func(in *Input) { in.Aggregate.Revision = 0 }} {
		p := &actionProvider{input: readerInput()}
		change(&p.input)
		s := actionService(p)
		if _, err := s.Acquire(t.Context(), "owner", "session"); err != ErrBlocked || p.acquire != 0 {
			t.Fatal(err, p.acquire)
		}
		if got, err := s.RequireSubmit(t.Context(), "owner", "session", IntentRead); err != ErrBlocked || got.Blocker != FreshSnapshotRequired {
			t.Fatal(got, err)
		}
	}
	p := &actionProvider{input: readerInput()}
	p.input.Run.Thread = Missing
	p.input.Run.Variant = DedicatedUnstarted
	p.input.Run.Access = UnknownAccess
	p.input.Run.Verification = Unverified
	s := actionService(p)
	if _, err := s.RequireSubmit(t.Context(), "owner", "session", IntentWrite); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Acquire(t.Context(), "owner", "session"); err == nil || p.acquire != 0 {
		t.Fatal(err, p.acquire)
	}
}
