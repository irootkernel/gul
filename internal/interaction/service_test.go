package interaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

type bindings struct{}

func (bindings) Binding(_ context.Context, subject, id string) (session.Binding, error) {
	if subject != "owner" {
		return session.Binding{}, ErrAuthority
	}
	return session.Binding{SubjectID: subject, ID: id, WorkspaceID: "ws", RunID: id, ControllerBindingID: "bound"}, nil
}

type workspaces struct{}

func (workspaces) Revalidate(context.Context, string, string) (workspace.Attachment, error) {
	return workspace.Attachment{ID: "ws", CanonicalRoot: "/workspace", ProviderID: "provider"}, nil
}

type credentials struct {
	calls atomic.Int32
	fail  bool
}

func (c *credentials) Resolve(context.Context, string, string) (session.Carrier, error) {
	c.calls.Add(1)
	if c.fail {
		return session.Carrier{}, errors.New("credential canary-secret")
	}
	return session.Carrier{AbsolutePath: "/private/carrier", ControllerID: "controller", Generation: 1}, nil
}

type fakeProvider struct {
	mu                              sync.Mutex
	calls, reads                    int
	limit                           int
	status                          Status
	loss, unavailable, leavePending bool
	request                         []byte // Alias only: ownership clearing is observable after invocation.
	observed                        chan string
}

func (p *fakeProvider) ResponseLimit() int {
	if p.limit > 0 {
		return p.limit
	}
	return MaximumResponseBytes
}
func (p *fakeProvider) Pending(context.Context, Bound) (PendingState, error) {
	return PendingState{Stamp: observation.Stamp{Head: "2", Run: 2, Interaction: 2}}, nil
}
func (p *fakeProvider) Card(_ context.Context, _ Bound, id string) (Card, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	if p.unavailable {
		return Card{}, ErrUnavailable
	}
	return Card{Summary: Summary{ID: id, Kind: UserInput, Status: p.status, Protected: true}, Input: &Input{Questions: []Question{{ID: "q", Secret: true}}}}, nil
}
func (p *fakeProvider) Resolve(_ context.Context, _ Bound, _, _ string, body []byte) (Resolution, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.request = body
	if !p.leavePending {
		p.status = Resolved
	}
	if p.loss {
		return Resolution{}, errors.New("canary-secret transport echo")
	}
	return Resolution{Status: Resolved, Receipt: "opaque"}, nil
}
func (p *fakeProvider) Observe(ctx context.Context, b Bound) (PendingState, error) {
	if b.Carrier.ControllerID != "" {
		return PendingState{}, ErrAuthority
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > PollTimeout {
		return PendingState{}, ErrInvalid
	}
	if p.observed != nil {
		p.observed <- b.Binding.ID
	}
	return PendingState{Stamp: observation.Stamp{Head: "2", Run: 2, Interaction: 2}}, nil
}
func service(p *fakeProvider, c *credentials) *Service {
	return &Service{Repository: bindings{}, Workspaces: workspaces{}, Carriers: c, Provider: p}
}

func TestResponseLossNeverReplaysOrRetainsInput(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprint(pending), func(t *testing.T) {
			p := &fakeProvider{status: Pending, loss: true, leavePending: pending}
			s := service(p, &credentials{})
			body := []byte(`{"answers":{"q":{"answers":["canary-secret"]}}}`)
			out, err := s.Resolve(t.Context(), "owner", "session", "id", body)
			want := OutcomeResolved
			if pending {
				want = OutcomeReenter
			}
			if err != nil || out.Outcome != want || p.calls != 1 || p.reads != 2 {
				t.Fatalf("outcome %+v %v calls%d reads%d", out, err, p.calls, p.reads)
			}
			for _, data := range [][]byte{body, p.request} {
				for _, b := range data {
					if b != 0 {
						t.Fatal("retained response")
					}
				}
			}
			if strings.Contains(fmt.Sprintf("%+v", out), "canary-secret") {
				t.Fatal("response in result")
			}
		})
	}
}
func TestResponseBoundsAndAuthorityPrecedeInvocation(t *testing.T) {
	for _, limit := range []int{128, MaximumResponseBytes} {
		p := &fakeProvider{status: Pending, limit: limit}
		s := service(p, &credentials{})
		body := []byte(strings.Repeat("x", limit+1))
		_, err := s.Resolve(t.Context(), "owner", "session", "id", body)
		if !errors.Is(err, ErrInvalid) || p.calls != 0 || p.reads != 0 {
			t.Fatal("oversize reached provider")
		}
	}
	p := &fakeProvider{status: Pending}
	c := &credentials{fail: true}
	s := service(p, c)
	if _, err := s.Get(t.Context(), "owner", "session", "id"); err != ErrAuthority || strings.Contains(err.Error(), "canary") {
		t.Fatal("unsafe binding error", err)
	}
	if _, err := s.List(t.Context(), "owner", "session"); err != nil {
		t.Fatal("observer required Controller", err)
	}
	if _, err := s.List(t.Context(), "other", "session"); err != ErrAuthority {
		t.Fatal("foreign account")
	}
}
func TestNormalizeRejectsUnexpectedFieldsAndQuestions(t *testing.T) {
	card := Card{Summary: Summary{Kind: UserInput}, Input: &Input{Questions: []Question{{ID: "q", Options: []Option{{Label: "yes"}}}}}}
	for _, body := range []string{`{"answers":{"other":{"answers":["yes"]}}}`, `{"answers":{"q":{"answers":["no"]}}}`, `{"answers":{"q":{"answers":["yes"],"secret":"x"}}}`, `{"answers":{}}`, `{"answers":{"q":{"answers":["yes"]}}} {}`} {
		if _, err := normalize([]byte(body), card); err != ErrInvalid {
			t.Fatal(body, err)
		}
	}
	if _, err := normalize([]byte(`{"answers":{"q":{"answers":["yes"]}}}`), card); err != nil {
		t.Fatal(err)
	}
	card = Card{Summary: Summary{Kind: CommandApproval}, Decisions: []Decision{Decline}}
	if _, err := normalize([]byte(`{"decision":"accept_once"}`), card); err != ErrInvalid {
		t.Fatal("unadvertised decision")
	}
	if _, err := normalize([]byte(`{"decision":"decline"}`), card); err != nil {
		t.Fatal(err)
	}
}

type notifier struct{ calls atomic.Int32 }

func (n *notifier) InteractionChanged(context.Context, Bound, observation.Stamp) error {
	n.calls.Add(1)
	return nil
}
func TestPollingChecksEveryNonliveRunWithoutControllerAndCoalesces(t *testing.T) {
	p := &fakeProvider{observed: make(chan string, 32)}
	c := &credentials{fail: true}
	n := &notifier{}
	poll := &Poller{Service: service(p, c), Notify: n}
	defer poll.Close()
	targets := make([]PollTarget, 24)
	for i := range targets {
		targets[i] = PollTarget{"owner", fmt.Sprint(i)}
	}
	if err := poll.Set(t.Context(), targets); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	deadline := time.After(time.Second)
	for len(seen) < len(targets) {
		select {
		case id := <-p.observed:
			seen[id] = true
		case <-deadline:
			t.Fatal("run starved", len(seen))
		}
	}
	if c.calls.Load() != 0 {
		t.Fatal("poll required mutation authority")
	}
	poll.Close()
	before := n.calls.Load()
	if _, err := poll.Check(t.Context(), targets[0], PollState{Stamp: observation.Stamp{Head: "2", Run: 2, Interaction: 2}}); err != nil {
		t.Fatal(err)
	}
	if n.calls.Load() != before {
		t.Fatal("unchanged snapshot emitted notification")
	}
	if PollInterval+PollTimeout > 10*time.Second {
		t.Fatal("discovery bound")
	}
}

func TestValidResponsesAtProviderAndLocalByteBoundaries(t *testing.T) {
	for _, limit := range []int{128, MaximumResponseBytes} {
		for _, extra := range []int{0, 1} {
			p := &fakeProvider{status: Pending, limit: limit}
			s := service(p, &credentials{})
			text := `{"answers":{"q":{"answers":["canary-secret"]}}}`
			body := []byte(text + strings.Repeat(" ", limit-len(text)+extra))
			out, err := s.Resolve(t.Context(), "owner", "session", "id", body)
			if extra == 0 && (err != nil || p.calls != 1 || out.Outcome != OutcomeResolved) {
				t.Fatal("exact valid boundary rejected", limit, err)
			}
			if extra == 1 && (err != ErrInvalid || p.calls != 0 || p.reads != 0) {
				t.Fatal("oversize was parsed or invoked", limit, err)
			}
		}
	}
}
func TestUserInputRequiresEveryQuestionAndNonemptyAnswers(t *testing.T) {
	card := Card{Summary: Summary{Kind: UserInput}, Input: &Input{Questions: []Question{{ID: "a"}, {ID: "b"}}}}
	for _, body := range []string{`{"answers":{"a":{"answers":["yes"]}}}`, `{"answers":{"a":{"answers":[""]},"b":{"answers":["yes"]}}}`} {
		if _, err := normalize([]byte(body), card); err != ErrInvalid {
			t.Fatal("invalid normalized", err)
		}
	}
	if _, err := normalize([]byte(`{"answers":{"a":{"answers":["yes"]},"b":{"answers":["yes"]}}}`), card); err != nil {
		t.Fatal(err)
	}
}

type regressedProvider struct{ *fakeProvider }

func (p regressedProvider) Observe(context.Context, Bound) (PendingState, error) {
	return PendingState{Stamp: observation.Stamp{Head: "1", Run: 1, Interaction: 1}}, nil
}
func TestRegressedPollingStampInvalidatesWithoutAdvancing(t *testing.T) {
	p := &fakeProvider{}
	s := service(p, &credentials{})
	s.Provider = regressedProvider{p}
	n := &notifier{}
	poll := &Poller{Service: s, Notify: n}
	previous := PollState{Stamp: observation.Stamp{Head: "2", Run: 2, Interaction: 2}}
	after, err := poll.Check(t.Context(), PollTarget{"owner", "session"}, previous)
	if err != ErrBlocked || after.Stamp != previous.Stamp || !after.Invalidated || n.calls.Load() != 1 {
		t.Fatal("regression hidden", after, err, n.calls.Load())
	}
}

type slowProvider struct {
	*fakeProvider
	slowStarted chan struct{}
	once        sync.Once
}

func (p *slowProvider) Observe(ctx context.Context, b Bound) (PendingState, error) {
	if b.Binding.ID == "slow" {
		p.once.Do(func() { close(p.slowStarted) })
		<-ctx.Done()
		return PendingState{}, ctx.Err()
	}
	return p.fakeProvider.Observe(ctx, b)
}
func TestSlowPollingRunDoesNotStarveOtherRunAcrossIntervals(t *testing.T) {
	p := &fakeProvider{observed: make(chan string, 8)}
	slow := &slowProvider{fakeProvider: p, slowStarted: make(chan struct{})}
	s := service(p, &credentials{fail: true})
	s.Provider = slow
	poll := &Poller{Service: s, Notify: &notifier{}}
	defer poll.Close()
	start := time.Now()
	if err := poll.Set(t.Context(), []PollTarget{{"owner", "slow"}, {"owner", "fast"}}); err != nil {
		t.Fatal(err)
	}
	<-slow.slowStarted
	for range 2 {
		select {
		case id := <-p.observed:
			if id != "fast" {
				t.Fatal(id)
			}
		case <-time.After(7 * time.Second):
			t.Fatal("fast Run starved by slow Run")
		}
	}
	if time.Since(start) > 7*time.Second {
		t.Fatal("polling exceeded discovery bound")
	}
}

func TestEmptyAnswerIsRejectedBeforeResolveInvocation(t *testing.T) {
	p := &fakeProvider{status: Pending}
	s := service(p, &credentials{})
	_, err := s.Resolve(t.Context(), "owner", "session", "id", []byte(`{"answers":{"q":{"answers":[""]}}}`))
	if err != ErrInvalid || p.calls != 0 {
		t.Fatal("invalid answer reached Resolve", err, p.calls)
	}
}

type recoveryProvider struct {
	*fakeProvider
	stamp observation.Stamp
	err   error
}

func (p *recoveryProvider) Observe(context.Context, Bound) (PendingState, error) {
	return PendingState{Stamp: p.stamp}, p.err
}

type recoveryNotifier struct {
	stamps []observation.Stamp
	fail   bool
}

func (n *recoveryNotifier) InteractionChanged(_ context.Context, _ Bound, stamp observation.Stamp) error {
	if n.fail {
		return ErrUnavailable
	}
	n.stamps = append(n.stamps, stamp)
	return nil
}
func TestPollingRecoveryNotifiesSameStampOnceAndPreservesRegressionFloor(t *testing.T) {
	for _, failure := range []string{"read", "invalid", "regression", "notification"} {
		t.Run(failure, func(t *testing.T) {
			stamp := observation.Stamp{Head: "2", Run: 2, Interaction: 2}
			p := &recoveryProvider{fakeProvider: &fakeProvider{}, stamp: stamp}
			s := service(p.fakeProvider, &credentials{fail: true})
			s.Provider = p
			n := &recoveryNotifier{}
			poll := &Poller{Service: s, Notify: n}
			target := PollTarget{"owner", "session"}
			state, err := poll.Check(t.Context(), target, PollState{})
			if err != nil || state.Stamp != stamp || state.Invalidated {
				t.Fatal(state, err)
			}
			switch failure {
			case "read":
				p.err = ErrUnavailable
			case "invalid":
				p.stamp = observation.Stamp{}
			case "regression":
				p.stamp = observation.Stamp{Head: "1", Run: 1, Interaction: 1}
			case "notification":
				p.err = ErrUnavailable
				n.fail = true
			}
			state, err = poll.Check(t.Context(), target, state)
			if err == nil || state.Stamp != stamp || !state.Invalidated {
				t.Fatal("failure lost regression floor", state, err)
			}
			// A failed cycle must never allow an older projection to become current.
			p.err = nil
			p.stamp = observation.Stamp{Head: "1", Run: 1, Interaction: 1}
			n.fail = false
			state, err = poll.Check(t.Context(), target, state)
			if err != ErrBlocked || state.Stamp != stamp || !state.Invalidated {
				t.Fatal("regression accepted", state, err)
			}
			p.stamp = stamp
			n.fail = true
			state, err = poll.Check(t.Context(), target, state)
			if err == nil || !state.Invalidated {
				t.Fatal("failed recovery publication cleared invalidation", state, err)
			}
			n.fail = false
			before := len(n.stamps)
			state, err = poll.Check(t.Context(), target, state)
			if err != nil || state.Invalidated || len(n.stamps) != before+1 || n.stamps[before] != stamp {
				t.Fatal("recovery hidden", state, err, n.stamps)
			}
			state, err = poll.Check(t.Context(), target, state)
			if err != nil || len(n.stamps) != before+1 {
				t.Fatal("healthy stamp not coalesced", state, err, n.stamps)
			}
		})
	}
}
