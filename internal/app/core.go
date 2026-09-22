package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrCoreNotRunning         = errors.New("gul core is not running")
	ErrAccessDenied           = errors.New("product access denied")
	ErrProviderUnavailable    = errors.New("runtime provider unavailable")
	ErrPersistenceUnavailable = errors.New("persistence unavailable")
)

type lifecycleState uint8

const (
	stateStopped lifecycleState = iota
	stateStarting
	stateRunning
	stateStopping
)

// Core is the delivery-independent application foundation shared by headless
// and desktop hosts. It owns composition and lifecycle, not transport or
// provider-specific domain behavior.
type Core struct {
	mu            sync.RWMutex
	state         lifecycleState
	transition    chan struct{}
	accessIdle    chan struct{}
	accessCount   int
	lifecycle     LifecyclePort
	provider      ProviderPort
	persistence   PersistencePort
	authorization AuthorizationPort
}

func NewCore(dependencies Dependencies) *Core {
	lifecycle := dependencies.Lifecycle
	if lifecycle == nil {
		lifecycle = noopLifecycle{}
	}
	provider := dependencies.Provider
	if provider == nil {
		provider = unavailableProvider{}
	}
	persistence := dependencies.Persistence
	if persistence == nil {
		persistence = unavailablePersistence{}
	}
	authorization := dependencies.Authorization
	if authorization == nil {
		authorization = denyAuthorization{}
	}

	idle := make(chan struct{})
	close(idle)
	return &Core{
		state:         stateStopped,
		accessIdle:    idle,
		lifecycle:     lifecycle,
		provider:      provider,
		persistence:   persistence,
		authorization: authorization,
	}
}

func (c *Core) Start(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("start Gul core: %w", err)
		}
		c.mu.Lock()
		switch c.state {
		case stateRunning:
			c.mu.Unlock()
			return nil
		case stateStarting, stateStopping:
			transition := c.transition
			c.mu.Unlock()
			if err := waitFor(ctx, transition); err != nil {
				return fmt.Errorf("wait to start Gul core: %w", err)
			}
			continue
		default:
			c.state = stateStarting
			c.transition = make(chan struct{})
			c.mu.Unlock()
		}
		break
	}

	err := c.lifecycle.Start(ctx)
	if err == nil {
		err = ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.state = stateStopped
		close(c.transition)
		return fmt.Errorf("start Gul core lifecycle: %w", err)
	}
	c.state = stateRunning
	close(c.transition)
	return nil
}

func (c *Core) Stop(ctx context.Context) error {
	var idle chan struct{}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stop Gul core: %w", err)
		}
		c.mu.Lock()
		switch c.state {
		case stateStopped:
			c.mu.Unlock()
			return nil
		case stateStarting, stateStopping:
			transition := c.transition
			c.mu.Unlock()
			if err := waitFor(ctx, transition); err != nil {
				return fmt.Errorf("wait to stop Gul core: %w", err)
			}
			continue
		default:
			c.state = stateStopping
			c.transition = make(chan struct{})
			idle = c.accessIdle
			c.mu.Unlock()
		}
		break
	}

	if err := waitFor(ctx, idle); err != nil {
		c.mu.Lock()
		c.state = stateRunning
		close(c.transition)
		c.mu.Unlock()
		return fmt.Errorf("wait for product access to finish: %w", err)
	}
	if err := ctx.Err(); err != nil {
		c.mu.Lock()
		c.state = stateRunning
		close(c.transition)
		c.mu.Unlock()
		return fmt.Errorf("stop Gul core: %w", err)
	}

	err := c.lifecycle.Stop(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.state = stateRunning
		close(c.transition)
		return fmt.Errorf("stop Gul core lifecycle: %w", err)
	}
	c.state = stateStopped
	close(c.transition)
	return nil
}

func (c *Core) Running() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == stateRunning
}

// RequireProductAccess is the common fail-closed gate for later product
// handlers. Authorization runs before availability checks so an unauthenticated
// caller cannot probe provider or persistence health.
func (c *Core) RequireProductAccess(ctx context.Context, principal Principal) error {
	if !c.beginAccess() {
		return ErrCoreNotRunning
	}
	defer c.endAccess()
	if err := c.authorization.Authorize(ctx, principal); err != nil {
		return fmt.Errorf("%w: %v", ErrAccessDenied, err)
	}
	if !c.Running() {
		return ErrCoreNotRunning
	}
	if err := c.provider.Ready(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	if !c.Running() {
		return ErrCoreNotRunning
	}
	if err := c.persistence.Ready(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
	}
	if !c.Running() {
		return ErrCoreNotRunning
	}
	return nil
}

func (c *Core) beginAccess() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != stateRunning {
		return false
	}
	if c.accessCount == 0 {
		c.accessIdle = make(chan struct{})
	}
	c.accessCount++
	return true
}

func (c *Core) endAccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessCount--
	if c.accessCount == 0 {
		close(c.accessIdle)
	}
}

func waitFor(ctx context.Context, done <-chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type noopLifecycle struct{}

func (noopLifecycle) Start(ctx context.Context) error { return ctx.Err() }
func (noopLifecycle) Stop(ctx context.Context) error  { return ctx.Err() }

type unavailableProvider struct{}

func (unavailableProvider) Ready(context.Context) error { return ErrProviderUnavailable }

type unavailablePersistence struct{}

func (unavailablePersistence) Ready(context.Context) error { return ErrPersistenceUnavailable }

type denyAuthorization struct{}

func (denyAuthorization) Authorize(context.Context, Principal) error { return ErrAccessDenied }
