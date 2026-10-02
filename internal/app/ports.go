package app

import "context"

// Principal identifies the caller presented to the authorization boundary.
// Production authentication owns how a principal is established.
type Principal struct {
	Subject string
}

// LifecyclePort hosts process-local startup and shutdown work. It must not be
// used to hide provider-specific behavior from the provider port.
type LifecyclePort interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// ProviderPort reports whether the configured runtime provider can currently
// serve product behavior. Typed provider capabilities are added separately.
type ProviderPort interface {
	Ready(context.Context) error
}

// ProviderStatus contains safe availability facts, never provider paths,
// credentials, raw diagnostics or request bodies.
type ProviderStatus struct {
	Health                                                            string
	RestartsRemaining                                                 uint32
	Version                                                           string
	Protocol                                                          uint32
	ChannelReady                                                      bool
	SocketCleanupUnsafe                                               bool
	PersistentRuns, EventReplay, ControllerBinding, Artifacts         bool
	ReaderWriter, DedicatedWriter, DurableWriter, FirstWriteViaSubmit bool
}

type ProviderDiagnostics interface{ Status() ProviderStatus }

// PersistencePort reports whether Gul-owned persistence can currently serve
// product behavior. Repository APIs are added with their owning tasks.
type PersistencePort interface {
	Ready(context.Context) error
}

// AuthorizationPort decides whether a previously established principal may
// enter the product surface. Authentication and operation policy remain
// separate concerns.
type AuthorizationPort interface {
	Authorize(context.Context, Principal) error
}

// Dependencies are the replaceable boundaries used by both headless and
// desktop delivery. Missing dependencies are replaced by fail-closed defaults.
type Dependencies struct {
	Lifecycle     LifecyclePort
	Provider      ProviderPort
	Persistence   PersistencePort
	Authorization AuthorizationPort
}
