package observation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

type memoryRepo struct {
	mu        sync.Mutex
	cp        Checkpoint
	commits   []Invalidation
	fail      bool
	validated chan struct{}
	block     <-chan struct{}
}

func (r *memoryRepo) Checkpoint(context.Context, Binding) (Checkpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cp, nil
}
func (r *memoryRepo) Validate(ctx context.Context, _ Binding, c Cursor) error {
	if r.validated != nil {
		select {
		case r.validated <- struct{}{}:
		default:
		}
	}
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cp.Validated = c
	return nil
}
func (r *memoryRepo) Commit(_ context.Context, in Invalidation) (Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return Notification{}, errors.New("disk fault")
	}
	r.commits = append(r.commits, in)
	r.cp.Committed = in.Cursor
	r.cp.Stamp = in.Floor
	return Notification{Sequence: Sequence(len(r.commits)), SessionID: in.Binding.SessionID, CorrelationID: in.CorrelationID, Kind: "projection_invalidated", CreatedAt: in.At}, nil
}

type testStream struct {
	ctx    context.Context
	events chan Envelope
	fail   chan error
}

func (s *testStream) Receive() (Envelope, error) {
	select {
	case e := <-s.events:
		return e, nil
	case err := <-s.fail:
		return Envelope{}, err
	case <-s.ctx.Done():
		return Envelope{}, s.ctx.Err()
	}
}
func (s *testStream) Close() error { return nil }

type testProvider struct {
	s     *testStream
	after chan Cursor
}

func (p testProvider) Watch(ctx context.Context, _ Binding, after Cursor) (Stream, error) {
	p.s.ctx = ctx
	p.after <- after
	return p.s, nil
}

type testRefresh struct {
	mu    sync.Mutex
	masks []Refresh
	runs  []string
}

func (r *testRefresh) Refresh(_ context.Context, binding Binding, mask Refresh, _ Stamp) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.masks = append(r.masks, mask)
	r.runs = append(r.runs, binding.RunID)
	return nil
}
func testBinding() Binding {
	return Binding{SubjectID: "owner", SessionID: "session", ProviderID: "dolgorae", RunID: "run", WorkspaceID: "workspace", AbsoluteRoot: "/workspace"}
}
func testEvent(n int) Envelope {
	c := Cursor(fmt.Sprint(n))
	return Envelope{Event: &Event{Kind: "TurnStateChanged", Cursor: c, ID: fmt.Sprintf("event-%d", n), RunID: "run", WorkspaceID: "workspace", Stamp: Stamp{Head: c, Run: uint64(n)}, At: time.Now(), Refresh: Run | Timeline}}
}
func bridgeFixture(t *testing.T, r *memoryRepo) (*Bridge, *testStream, context.CancelFunc, <-chan error, *testRefresh) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	stream := &testStream{events: make(chan Envelope, QueueLimit*4), fail: make(chan error, 1)}
	provider := testProvider{s: stream, after: make(chan Cursor, 1)}
	refresh := &testRefresh{}
	b, err := NewBridge(r, provider, refresh, testBinding())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	select {
	case <-provider.after:
	case <-time.After(time.Second):
		t.Fatal("no subscription")
	}
	return b, stream, cancel, done, refresh
}
func receiveNotification(t *testing.T, b *Bridge) Notification {
	t.Helper()
	select {
	case n := <-b.Notifications():
		return n
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
		return Notification{}
	}
}
func receiveDone(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not stop")
		return nil
	}
}
func TestCommitBeforeDeliveryCoalescesAndDeduplicates(t *testing.T) {
	repo := &memoryRepo{}
	b, s, cancel, done, refresh := bridgeFixture(t, repo)
	s.events <- testEvent(2)
	s.events <- testEvent(4)
	replay := testEvent(4)
	replay.Event.Replay = true
	s.events <- replay
	n := receiveNotification(t, b)
	cp, _ := repo.Checkpoint(t.Context(), testBinding())
	if cp.Committed != "4" || n.Sequence != 1 || n.CorrelationID == "event-4" {
		t.Fatalf("delivery preceded safe commit: %+v %+v", n, cp)
	}
	s.events <- replay
	select {
	case n := <-b.Notifications():
		t.Fatalf("duplicate delivery: %+v", n)
	case <-time.After(2 * CoalesceInterval):
	}
	if state := b.State(); state.Committed != "4" || state.Validated != "4" || state.Generation != 1 || state.LastSnapshot.IsZero() {
		t.Fatalf("state %+v", state)
	}
	refresh.mu.Lock()
	if len(refresh.masks) != 1 || refresh.masks[0] != (Run|Timeline) {
		t.Errorf("refreshes %v", refresh.masks)
	}
	refresh.mu.Unlock()
	cancel()
	if !errors.Is(receiveDone(t, done), context.Canceled) {
		t.Fatal("cancel result")
	}
}
func TestStorageFailureKeepsReplayBoundaryAndNoDelivery(t *testing.T) {
	repo := &memoryRepo{fail: true}
	b, s, _, done, _ := bridgeFixture(t, repo)
	s.events <- testEvent(3)
	if receiveDone(t, done) == nil {
		t.Fatal("fault lost")
	}
	cp, _ := repo.Checkpoint(t.Context(), testBinding())
	if cp.Validated != "3" || cp.Committed != "" {
		t.Fatalf("unsafe checkpoint %+v", cp)
	}
	select {
	case <-b.Notifications():
		t.Fatal("emitted uncommitted event")
	default:
	}
}
func TestBoundedQueueOnlyStopsAffectedRun(t *testing.T) {
	release := make(chan struct{})
	repo := &memoryRepo{block: release, validated: make(chan struct{}, 1)}
	b, s, _, done, refresh := bridgeFixture(t, repo)
	s.events <- testEvent(2)
	<-repo.validated
	for i := 3; i < QueueLimit+8; i++ {
		s.events <- testEvent(i)
	}
	select {
	case <-s.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("queue did not overflow")
	}
	close(release)
	if !errors.Is(receiveDone(t, done), ErrSlowConsumer) {
		t.Fatal("overflow not classified")
	}
	if b.State().Connection != "slow_consumer" {
		t.Fatal(b.State())
	}
	refresh.mu.Lock()
	if len(refresh.masks) == 0 || refresh.masks[len(refresh.masks)-1] != AllAggregates {
		t.Errorf("missing overflow repair: %v", refresh.masks)
	}
	refresh.mu.Unlock()
	other, otherStream, cancel, otherDone, _ := bridgeFixture(t, &memoryRepo{})
	otherStream.events <- testEvent(2)
	receiveNotification(t, other)
	cancel()
	receiveDone(t, otherDone)
}
func TestAdvisoriesAndInvalidEnvelopesDoNotAdvanceCursors(t *testing.T) {
	for _, kind := range []string{"heartbeat", "terminal", "shutdown", "foreign", "rewind", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			repo := &memoryRepo{cp: Checkpoint{Validated: "4", Committed: "4", Stamp: Stamp{Head: "4", Run: 4}}}
			b, s, _, done, _ := bridgeFixture(t, repo)
			e := Envelope{RunID: "run", Head: "4"}
			switch kind {
			case "heartbeat":
				e.Heartbeat = time.Now()
			case "terminal":
				e.End = "terminal"
			case "shutdown":
				e.End = "shutdown"
			case "foreign":
				e.RunID = "foreign"
				e.Heartbeat = time.Now()
			case "rewind":
				e = testEvent(2)
			case "unknown":
				e.End = "unknown"
			}
			s.events <- e
			if kind == "heartbeat" {
				// The ordered end proves the preceding heartbeat was consumed.
				s.events <- Envelope{RunID: "run", Head: "4", End: "shutdown"}
			}
			err := receiveDone(t, done)
			if kind == "terminal" && err != nil {
				t.Fatal(err)
			}
			if kind != "terminal" && err == nil {
				t.Fatal("missing failure")
			}
			cp, _ := repo.Checkpoint(t.Context(), testBinding())
			if kind == "heartbeat" && !b.State().LastHeartbeat.Equal(e.Heartbeat) {
				t.Fatalf("heartbeat not recorded: %+v", b.State())
			}
			if cp.Committed != "4" || cp.Validated != "4" {
				t.Fatal(cp)
			}
			select {
			case <-b.Notifications():
				t.Fatal("advisory delivered as semantic update")
			default:
			}
		})
	}
}

func TestDisconnectRefreshesAndResumesCommittedCursor(t *testing.T) {
	for _, failure := range []error{io.EOF, ErrSlowConsumer} {
		t.Run(failure.Error(), func(t *testing.T) {
			repo := &memoryRepo{cp: Checkpoint{Validated: "4", Committed: "4", Stamp: Stamp{Head: "4", Run: 4}}}
			b, stream, _, done, refresh := bridgeFixture(t, repo)
			stream.fail <- failure
			want := ErrRefresh
			if errors.Is(failure, ErrSlowConsumer) {
				want = ErrSlowConsumer
			}
			if !errors.Is(receiveDone(t, done), want) {
				t.Fatal("disconnect classification")
			}
			if len(refresh.masks) != 1 || refresh.masks[0] != AllAggregates || b.State().LastSnapshot.IsZero() {
				t.Fatalf("repair missing: %v %+v", refresh.masks, b.State())
			}
			cp, _ := repo.Checkpoint(t.Context(), testBinding())
			if cp.Committed != "4" || cp.Validated != "4" {
				t.Fatal("disconnect advanced checkpoint", cp)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			provider := testProvider{s: &testStream{events: make(chan Envelope), fail: make(chan error)}, after: make(chan Cursor, 1)}
			b.provider = provider
			restarted := make(chan error, 1)
			go func() { restarted <- b.Run(ctx) }()
			select {
			case after := <-provider.after:
				if after != "4" {
					t.Fatal("unsafe replay boundary", after)
				}
			case <-time.After(time.Second):
				t.Fatal("restart did not subscribe")
			}
			cancel()
			receiveDone(t, restarted)
		})
	}
}

func TestWindowRecentActivityAndStableTie(t *testing.T) {
	now := time.Now()
	for _, visible := range []bool{false, true} {
		var runs []Candidate
		for i := 0; i < 7; i++ {
			runs = append(runs, Candidate{RunID: fmt.Sprint(i), ActiveTurn: true})
		}
		runs = append(runs, Candidate{RunID: "a", Visible: visible, Recent: now}, Candidate{RunID: "z", Visible: visible, Recent: now.Add(time.Second)})
		w := &Window{}
		if modes := w.Select(now, runs); modes["z"] != "live" || modes["a"] != "polling" {
			t.Fatal("recent activity lost to ID order", modes)
		}
		runs[7].Recent = runs[8].Recent
		if modes := w.Select(now.Add(DemotionDelay), runs); modes["a"] != "live" || modes["z"] != "polling" {
			t.Fatal("equal activity did not use stable ID", modes)
		}
	}
}
func TestCrashUncertaintyRefreshesBeforeCommittedResume(t *testing.T) {
	repo := &memoryRepo{cp: Checkpoint{Validated: "9", Committed: "4", Stamp: Stamp{Head: "4", Run: 4}}}
	b, _, cancel, done, refresh := bridgeFixture(t, repo)
	refresh.mu.Lock()
	if len(refresh.masks) != 1 || refresh.masks[0] != AllAggregates {
		t.Errorf("repair %v", refresh.masks)
	}
	refresh.mu.Unlock()
	if b.State().Committed != "4" {
		t.Fatal(b.State())
	}
	cancel()
	receiveDone(t, done)
}
func TestWindowCapPriorityAndHysteresis(t *testing.T) {
	now := time.Now()
	var runs []Candidate
	for i := 0; i < 10; i++ {
		runs = append(runs, Candidate{RunID: fmt.Sprint(i)})
	}
	w := &Window{}
	modes := w.Select(now, runs)
	for i := 0; i < 10; i++ {
		if (modes[fmt.Sprint(i)] == "live") != (i < 8) {
			t.Fatal(modes)
		}
	}
	runs[9].Visible = true
	modes = w.Select(now.Add(time.Second), runs)
	if modes["9"] != "polling" {
		t.Fatal("ordinary churn", modes)
	}
	runs[8].PendingInteraction = true
	modes = w.Select(now.Add(2*time.Second), runs)
	if modes["8"] != "live" {
		t.Fatal("safety not admitted", modes)
	}
	modes = w.Select(now.Add(31*time.Second), runs)
	if modes["9"] != "live" {
		t.Fatal("hysteresis never expired", modes)
	}
	count := 0
	for _, mode := range modes {
		if mode == "live" {
			count++
		}
	}
	if count != LiveLimit {
		t.Fatal(count)
	}
}

type countingProvider struct {
	mu        sync.Mutex
	live, max int
	started   chan struct{}
}

type perRunRepo map[string]*memoryRepo

func (r perRunRepo) Checkpoint(ctx context.Context, b Binding) (Checkpoint, error) {
	return r[b.RunID].Checkpoint(ctx, b)
}
func (r perRunRepo) Validate(ctx context.Context, b Binding, c Cursor) error {
	return r[b.RunID].Validate(ctx, b, c)
}
func (r perRunRepo) Commit(ctx context.Context, in Invalidation) (Notification, error) {
	return r[in.Binding.RunID].Commit(ctx, in)
}

type sharedProvider struct {
	mu      sync.Mutex
	streams map[string]*testStream
	started chan string
	calls   int
}

func (p *sharedProvider) Watch(ctx context.Context, b Binding, _ Cursor) (Stream, error) {
	p.mu.Lock()
	s := p.streams[b.RunID]
	s.ctx = ctx
	p.mu.Unlock()
	p.started <- b.RunID
	return s, nil
}

// This unary fake shares the provider used by every managed subscription.
func (p *sharedProvider) Acquire(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	p.calls++
	return nil
}

func TestManagerSaturationPreservesSiblingAndUnaryProgress(t *testing.T) {
	release := make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	repos := perRunRepo{
		"slow": {block: release, validated: make(chan struct{}, 1)},
		"fast": {},
	}
	p := &sharedProvider{streams: make(map[string]*testStream), started: make(chan string, 2)}
	var runs []WatchedRun
	for _, id := range []string{"slow", "fast"} {
		p.streams[id] = &testStream{events: make(chan Envelope, QueueLimit*4), fail: make(chan error, 1)}
		binding := testBinding()
		binding.RunID, binding.SessionID = id, id
		runs = append(runs, WatchedRun{Candidate: Candidate{RunID: id}, Binding: binding})
	}
	refresh := &testRefresh{}
	m := NewManager(repos, p, refresh)
	defer m.Stop()
	if _, err := m.Update(t.Context(), time.Now(), runs); err != nil {
		t.Fatal(err)
	}
	for range runs {
		select {
		case <-p.started:
		case <-time.After(time.Second):
			t.Fatal("stream not started")
		}
	}
	send := func(id string, n int) {
		e := testEvent(n)
		e.Event.RunID = id
		p.streams[id].events <- e
	}
	send("slow", 1)
	select {
	case <-repos["slow"].validated:
	case <-time.After(time.Second):
		t.Fatal("slow validation did not block")
	}
	for i := 2; i < QueueLimit+8; i++ {
		send("slow", i)
	}
	select {
	case <-p.streams["slow"].ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("slow queue did not saturate")
	}
	// Keep the slow repository blocked while sibling delivery and a unary
	// mutation complete on the same provider. Neither may await its repair.
	send("fast", 2)
	unary := make(chan error, 1)
	go func() { unary <- p.Acquire(t.Context()) }()
	if n := receiveNotification(t, m.bridges["fast"]); n.SessionID != "fast" {
		t.Fatal(n)
	}
	select {
	case err := <-unary:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("unary blocked behind slow stream")
	}
	unblock.Do(func() { close(release) })
	select {
	case <-m.streams["slow"].done:
	case <-time.After(time.Second):
		t.Fatal("slow stream repair did not finish")
	}
	if state, _ := m.State("fast"); state.Connection != "connected" || state.Committed != "2" {
		t.Fatal("sibling interrupted", state)
	}
	refresh.mu.Lock()
	defer refresh.mu.Unlock()
	repairs := 0
	for i, mask := range refresh.masks {
		if mask == AllAggregates {
			repairs++
			if refresh.runs[i] != "slow" {
				t.Fatal("repaired unaffected run", refresh.runs)
			}
		}
	}
	if repairs != 1 || p.calls != 1 {
		t.Fatal("missing targeted repair or unary mutation", refresh.masks, p.calls)
	}
}

type countingStream struct {
	ctx  context.Context
	p    *countingProvider
	once sync.Once
}

func (p *countingProvider) Watch(ctx context.Context, _ Binding, _ Cursor) (Stream, error) {
	p.mu.Lock()
	p.live++
	if p.live > p.max {
		p.max = p.live
	}
	p.mu.Unlock()
	p.started <- struct{}{}
	return &countingStream{ctx: ctx, p: p}, nil
}
func (s *countingStream) Receive() (Envelope, error) { <-s.ctx.Done(); return Envelope{}, s.ctx.Err() }
func (s *countingStream) Close() error {
	s.once.Do(func() { s.p.mu.Lock(); s.p.live--; s.p.mu.Unlock() })
	return nil
}
func TestManagerNeverExceedsPhysicalStreamCap(t *testing.T) {
	p := &countingProvider{started: make(chan struct{}, 32)}
	m := NewManager(&memoryRepo{}, p, &testRefresh{})
	defer m.Stop()
	now := time.Now()
	var runs []WatchedRun
	for i := 0; i < 10; i++ {
		id := fmt.Sprint(i)
		b := testBinding()
		b.RunID = id
		runs = append(runs, WatchedRun{Candidate: Candidate{RunID: id}, Binding: b})
	}
	if _, err := m.Update(t.Context(), now, runs); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < LiveLimit; i++ {
		select {
		case <-p.started:
		case <-time.After(time.Second):
			t.Fatal("stream not started")
		}
	}
	runs[9].PendingInteraction = true
	if modes, err := m.Update(t.Context(), now.Add(time.Second), runs); err != nil || modes["9"] != "live" {
		t.Fatal(modes, err)
	}
	select {
	case <-p.started:
	case <-time.After(time.Second):
		t.Fatal("priority stream not started")
	}
	p.mu.Lock()
	if p.max > LiveLimit {
		t.Errorf("physical streams %d", p.max)
	}
	p.mu.Unlock()
	m.Stop()
	p.mu.Lock()
	if p.live != 0 {
		t.Error("streams survived stop")
	}
	p.mu.Unlock()
}

type finiteProvider struct {
	envelopes []Envelope
	err       error
}
type finiteStream struct{ envelopes []Envelope }

func (p finiteProvider) Watch(context.Context, Binding, Cursor) (Stream, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &finiteStream{envelopes: append([]Envelope(nil), p.envelopes...)}, nil
}
func (s *finiteStream) Receive() (Envelope, error) {
	if len(s.envelopes) == 0 {
		return Envelope{}, io.EOF
	}
	e := s.envelopes[0]
	s.envelopes = s.envelopes[1:]
	return e, nil
}
func (s *finiteStream) Close() error { return nil }
func TestTypedEndCannotBeOvertakenByEOF(t *testing.T) {
	for _, end := range []string{"terminal", "shutdown"} {
		t.Run(end, func(t *testing.T) {
			repo := &memoryRepo{}
			refresh := &testRefresh{}
			b, err := NewBridge(repo, finiteProvider{envelopes: []Envelope{testEvent(2), {RunID: "run", Head: "2", End: end}}}, refresh, testBinding())
			if err != nil {
				t.Fatal(err)
			}
			err = b.Run(t.Context())
			want := "terminal"
			if end == "shutdown" {
				want = "restarting"
				if !errors.Is(err, ErrRefresh) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if state := b.State(); state.Connection != want || state.Committed != "2" {
				t.Fatalf("lost ordered end: %+v", state)
			}
			if end == "terminal" && refresh.masks[len(refresh.masks)-1] != (Run|Timeline|Artifacts) {
				t.Fatal(refresh.masks)
			}
		})
	}
}
func TestRestoredStampRejectsRevisionRegression(t *testing.T) {
	for _, stamp := range []Stamp{{Head: "13", Run: 13, Writer: 3, Interaction: 10}, {Head: "13", Run: 13, Writer: 4, Interaction: 9}} {
		repo := &memoryRepo{cp: Checkpoint{Validated: "12", Committed: "12", Stamp: Stamp{Head: "12", Run: 12, Writer: 4, Interaction: 10}}}
		b, s, _, done, _ := bridgeFixture(t, repo)
		event := testEvent(13)
		event.Event.Stamp = stamp
		s.events <- event
		if !errors.Is(receiveDone(t, done), ErrInvalid) {
			t.Fatal("regression accepted")
		}
		if b.State().Committed != "12" {
			t.Fatal(b.State())
		}
		select {
		case <-b.Notifications():
			t.Fatal("regression delivered")
		default:
		}
	}
}

type failingRefresh struct{}

func (failingRefresh) Refresh(context.Context, Binding, Refresh, Stamp) error { return ErrRefresh }
func TestStartupFailuresAreDisconnected(t *testing.T) {
	for _, stage := range []string{"watch", "refresh"} {
		t.Run(stage, func(t *testing.T) {
			repo := &memoryRepo{}
			var refresh Refresher = &testRefresh{}
			if stage == "refresh" {
				repo.cp = Checkpoint{Validated: "2", Committed: "0"}
				refresh = failingRefresh{}
			}
			b, err := NewBridge(repo, finiteProvider{err: ErrRefresh}, refresh, testBinding())
			if err != nil {
				t.Fatal(err)
			}
			if b.Run(t.Context()) == nil || b.State().Connection != "disconnected" {
				t.Fatal(b.State())
			}
		})
	}
}
func TestManagerPreservesMetadataAcrossDemotion(t *testing.T) {
	p := &countingProvider{started: make(chan struct{}, 32)}
	m := NewManager(&memoryRepo{}, p, &testRefresh{})
	defer m.Stop()
	now := time.Now()
	var runs []WatchedRun
	for i := 0; i < 9; i++ {
		id := fmt.Sprint(i)
		b := testBinding()
		b.RunID = id
		runs = append(runs, WatchedRun{Candidate: Candidate{RunID: id}, Binding: b})
	}
	if _, err := m.Update(t.Context(), now, runs); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		<-p.started
	}
	m.bridges["7"].update(func(s *SubscriptionState) { s.LastHeartbeat = now; s.LastSnapshot = now })
	runs[8].ActiveTurn = true
	if _, err := m.Update(t.Context(), now.Add(time.Second), runs); err != nil {
		t.Fatal(err)
	}
	<-p.started
	demoted, ok := m.State("7")
	if !ok || demoted.Connection != "polling" || demoted.Generation != 1 || demoted.LastHeartbeat != now {
		t.Fatalf("demoted %+v %v", demoted, ok)
	}
	runs[7].PendingInteraction = true
	runs[8].ActiveTurn = false
	if _, err := m.Update(t.Context(), now.Add(31*time.Second), runs); err != nil {
		t.Fatal(err)
	}
	<-p.started
	promoted, ok := m.State("7")
	if !ok || promoted.Generation != 2 || promoted.ReconnectAttempts != 1 || promoted.LastHeartbeat != now || promoted.LastSnapshot != now {
		t.Fatalf("promoted %+v", promoted)
	}
}
