package observation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"time"
)

const QueueLimit = 64
const CoalesceInterval = 50 * time.Millisecond

// Bridge owns no mutation client. One instance serves one bound Run, so a slow
// stream or browser cannot lock another Run or delay unary provider operations.
type Bridge struct {
	repo          Repository
	provider      Provider
	refresh       Refresher
	binding       Binding
	mu            sync.Mutex
	state         SubscriptionState
	notifications chan Notification
	running       bool
}

func NewBridge(repo Repository, provider Provider, refresh Refresher, binding Binding) (*Bridge, error) {
	if repo == nil || provider == nil || refresh == nil || !binding.Valid() {
		return nil, ErrUnbound
	}
	return &Bridge{repo: repo, provider: provider, refresh: refresh, binding: binding, state: SubscriptionState{RunID: binding.RunID, Connection: "stale"}, notifications: make(chan Notification, 1)}, nil
}
func (b *Bridge) State() SubscriptionState           { b.mu.Lock(); defer b.mu.Unlock(); return b.state }
func (b *Bridge) Notifications() <-chan Notification { return b.notifications }
func (b *Bridge) notify(n Notification) {
	select {
	case b.notifications <- n:
	default:
		select {
		case <-b.notifications:
		default:
		}
		select {
		case b.notifications <- n:
		default:
		}
	}
}
func (b *Bridge) update(fn func(*SubscriptionState)) { b.mu.Lock(); defer b.mu.Unlock(); fn(&b.state) }

// Run returns a typed snapshot/reconnect requirement; reconnect scheduling and
// provider restart coordination belong to E5. Cancellation never mutates a Run.
func (b *Bridge) Run(ctx context.Context) (result error) {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return ErrInvalid
	}
	b.running = true
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.running = false
		if result != nil && (b.state.Connection == "connecting" || b.state.Connection == ConnectionConnected) {
			b.state.Connection = ConnectionDisconnected
		}
	}()
	cp, err := b.repo.Checkpoint(ctx, b.binding)
	if err != nil {
		return err
	}
	if cp.Committed == "" {
		cp.Committed = "0"
	}
	if cp.Validated == "" {
		cp.Validated = "0"
	}
	if !cp.Committed.Valid() || !cp.Validated.Valid() || cp.Validated.Compare(cp.Committed) < 0 {
		return ErrInvalid
	}
	if cp.Committed != "0" && (!cp.Stamp.Valid() || cp.Stamp.Head != cp.Committed) {
		return ErrRefresh
	}
	b.update(func(s *SubscriptionState) {
		s.Validated = cp.Validated
		s.Committed = cp.Committed
		s.Generation++
		if s.Generation > 1 {
			s.ReconnectAttempts++
		}
		s.Connection = "connecting"
	})
	if cp.Validated != cp.Committed || b.State().Generation > 1 {
		if err := b.refresh.Refresh(ctx, b.binding, AllAggregates, Stamp{}); err != nil {
			return err
		}
		b.update(func(s *SubscriptionState) { s.LastSnapshot = time.Now().UTC() })
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := b.provider.Watch(streamCtx, b.binding, cp.Committed)
	if err != nil {
		return err
	}
	b.update(func(s *SubscriptionState) { s.Connection = ConnectionConnected })
	queue := make(chan Envelope, QueueLimit)
	failed := make(chan error, 1)
	receiverDone := make(chan struct{})
	defer func() { cancel(); _ = stream.Close(); <-receiverDone }()
	go func() {
		defer close(receiverDone)
		for {
			envelope, err := stream.Receive()
			if err != nil {
				failed <- err
				return
			}
			select {
			case queue <- envelope:
				if envelope.End != "" {
					return
				}
			case <-streamCtx.Done():
				return
			default:
				failed <- ErrSlowConsumer
				cancel()
				return
			}
		}
	}()
	lastStamp := cp.Stamp
	var pending *Invalidation
	timer := time.NewTimer(CoalesceInterval)
	defer timer.Stop()
	flush := func() error {
		if pending == nil {
			return nil
		}
		n, err := b.repo.Commit(ctx, *pending)
		if err != nil {
			return err
		}
		lastStamp = pending.Floor
		committed := pending.Cursor
		needs, floor := pending.Refresh, pending.Floor
		b.update(func(s *SubscriptionState) { s.Committed = committed })
		b.notify(n)
		pending = nil
		if needs != 0 {
			if err := b.refresh.Refresh(ctx, b.binding, needs, floor); err != nil {
				return err
			}
			b.update(func(s *SubscriptionState) { s.LastSnapshot = time.Now().UTC() })
		}
		return nil
	}
	fail := func(err error) error {
		b.update(func(s *SubscriptionState) { s.Connection = ConnectionDisconnected })
		if errors.Is(err, ErrSlowConsumer) || errors.Is(err, ErrRefresh) {
			if errors.Is(err, ErrSlowConsumer) {
				b.update(func(s *SubscriptionState) { s.Connection = ConnectionSlowConsumer })
			}
			if refreshErr := b.refresh.Refresh(ctx, b.binding, AllAggregates, Stamp{}); refreshErr != nil {
				return errors.Join(err, refreshErr)
			}
			b.update(func(s *SubscriptionState) { s.LastSnapshot = time.Now().UTC() })
			return err
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case err := <-failed:
			// Do not commit queued/validated work after a stream failure. Resume from
			// the last transaction boundary; duplicate accepted events are harmless.
			if errors.Is(err, io.EOF) {
				err = ErrRefresh
			}
			return fail(err)
		case <-timer.C:
			if err := flush(); err != nil {
				return fail(err)
			}
			timer.Reset(CoalesceInterval)
		case envelope := <-queue:
			if event := envelope.Event; event != nil {
				if event.RunID != b.binding.RunID || event.WorkspaceID != b.binding.WorkspaceID || !event.Cursor.Valid() || !event.Stamp.Valid() || event.Cursor != event.Stamp.Head || event.ID == "" || event.At.IsZero() {
					return fail(ErrInvalid)
				}
				state := b.State()
				if event.Cursor.Compare(state.Committed) <= 0 {
					if event.Replay {
						continue
					}
					return fail(ErrInvalid)
				}
				if pending != nil && event.Cursor.Compare(pending.Cursor) <= 0 {
					if event.Replay && event.Cursor == pending.Cursor {
						continue
					}
					return fail(ErrInvalid)
				}
				if lastStamp.Valid() && !event.Stamp.Covers(lastStamp) {
					return fail(ErrInvalid)
				}
				if pending != nil && !event.Stamp.Covers(pending.Floor) {
					return fail(ErrInvalid)
				}
				if err := b.repo.Validate(ctx, b.binding, event.Cursor); err != nil {
					return fail(err)
				}
				b.update(func(s *SubscriptionState) { s.Validated = event.Cursor })
				if pending == nil {
					pending = &Invalidation{Binding: b.binding}
				}
				pending.Cursor = event.Cursor
				pending.Floor = event.Stamp
				pending.Refresh |= event.Refresh
				digest := sha256.Sum256([]byte(b.binding.ProviderID + "\x00" + b.binding.RunID + "\x00" + event.ID))
				pending.CorrelationID = hex.EncodeToString(digest[:])
				pending.At = event.At
				continue
			}
			if envelope.RunID != b.binding.RunID || !envelope.Head.Valid() {
				return fail(ErrInvalid)
			}
			if !envelope.Heartbeat.IsZero() {
				b.update(func(s *SubscriptionState) { s.LastHeartbeat = envelope.Heartbeat })
				// The head is advisory and never advances a checkpoint or delivery domain.
				if envelope.Head != b.State().Validated {
					return fail(ErrRefresh)
				}
				continue
			}
			if err := flush(); err != nil {
				return fail(err)
			}
			switch envelope.End {
			case "terminal":
				if err := b.refresh.Refresh(ctx, b.binding, Run|Timeline|Artifacts, Stamp{}); err != nil {
					return fail(err)
				}
				b.update(func(s *SubscriptionState) { s.Connection = ConnectionTerminal; s.LastSnapshot = time.Now().UTC() })
				return nil
			case "shutdown":
				b.update(func(s *SubscriptionState) { s.Connection = ConnectionRestarting })
				return ErrRefresh
			default:
				return fail(ErrInvalid)
			}
		}
	}
}
