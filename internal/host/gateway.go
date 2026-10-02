package host

import (
	"context"
	"errors"
	"time"

	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/composition"
	"github.com/rootkernel/gul/internal/controller"
	"github.com/rootkernel/gul/internal/gateway"
	"github.com/rootkernel/gul/internal/storage"
)

type productionLifecycle struct {
	gateway *gateway.Gateway
	runtime *composition.Runtime
}

func (p *productionLifecycle) Start(ctx context.Context) error {
	if p.runtime != nil {
		return p.runtime.Start(ctx)
	}
	return nil // Gateway startup already ran under the host singleton lock.
}
func (p *productionLifecycle) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var err error
	if p.runtime != nil {
		err = p.runtime.Stop(ctx)
	}
	return errors.Join(err, p.gateway.Stop(ctx))
}

// productionAssembly is shared by ordinary desktop and headless startup. The
// explicit injected assembly remains available for fixture-qualified composition.
func productionAssembly(ctx context.Context, store *storage.Store, config Config) (Assembly, error) {
	roots, err := store.Presentation().WorkspaceRoots(ctx)
	if err != nil {
		return Assembly{}, err
	}
	roots = append(roots, config.WorkspaceRoots...)
	g := gateway.New(gateway.Config{Executable: config.DolgoraeExecutable, Home: config.ProviderHome, WorkspaceRoots: roots})
	if err = g.Start(ctx); err != nil {
		return Assembly{}, err
	}
	return assembleGateway(store, config, g)
}

func assembleGateway(store *storage.Store, config Config, g *gateway.Gateway) (Assembly, error) {
	lifetime := &productionLifecycle{gateway: g}
	assembled := Assembly{Provider: g, Lifecycle: lifetime, gateway: g}
	if caps := g.NegotiatedCapabilities(); caps != nil {
		carriers, err := controller.New(context.Background(), store, config.ProviderHome, config.WorkspaceRoots, caps)
		if err != nil {
			stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return Assembly{}, errors.Join(err, g.Stop(stop))
		}
		g.SetCarrierValidator(carriers.ValidateRPC)
		r, err := composition.NewQualified(store, composition.Config{Port: g, Carriers: carriers, Roots: config.WorkspaceRoots, Policies: config.Policies}, caps)
		if err != nil {
			stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return Assembly{}, errors.Join(err, g.Stop(stop))
		}
		lifetime.runtime = r
		assembled.runtime = r
		assembled.Features = r.Features
	}
	return assembled, nil
}

var _ app.LifecyclePort = (*productionLifecycle)(nil)
