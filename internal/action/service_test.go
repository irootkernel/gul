package action

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type actionRepo struct{ operation OperationState }

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
	return &Service{Repository: actionRepo{NoOperation}, Workspaces: actionWorkspace{}, Carriers: actionCarrier{}, Provider: p}
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
