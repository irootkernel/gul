package host

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/auth"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/delivery/web"
	"github.com/rootkernel/gul/internal/gateway"
	"github.com/rootkernel/gul/internal/storage"
	"golang.org/x/sys/unix"
)

var ErrAlreadyRunning = errors.New("Gul core is already running; open Gul.app to attach")
var ErrPortCollision = errors.New("Gul loopback port is occupied; identify its owner before stopping it or select an explicit free port")
var ErrUnverifiedOwner = errors.New("existing Gul core could not be verified; do not start a second runtime")

const NativeHeader = "X-Gul-Native-Host"
const DefaultPort = 17423

// Assembly binds explicit application services to the host-owned store and lifetime.
type Assembly struct {
	gateway   *gateway.Gateway
	Provider  app.ProviderPort
	Lifecycle app.LifecyclePort
	Features  func(*app.Core) api.FeatureHandlers
}

type Config struct {
	DataDirectory string
	Port          int
	// Remote origins are admitted only after the caller's current Serve inspection.
	TailnetHost    string
	ServeInspector ServeInspector
	// Provider configuration is trusted host input, never a browser selection.
	DolgoraeExecutable string
	ProviderHome       string
	WorkspaceRoots     []string
	Policies           []string
	Provider           app.ProviderPort
	Lifecycle          app.LifecyclePort
	// Features supplies handlers after startup. Assemble.Features takes precedence.
	// Assemble overrides Provider/Lifecycle before startup; neither selects a fake.
	Features func(*app.Core, *storage.Store) api.FeatureHandlers
	Assemble func(*storage.Store) (Assembly, error)
}

type ownerRecord struct {
	Version    int    `json:"version"`
	PID        int    `json:"pid"`
	Origin     string `json:"origin"`
	Instance   string `json:"instance"`
	Capability string `json:"capability"`
}
type Host struct {
	operation sync.Mutex
	mu        sync.Mutex
	config    Config
	lock      *os.File
	store     *storage.Store
	Core      *app.Core
	gateway   *gateway.Gateway
	Boundary  *api.BrowserBoundary
	listener  net.Listener
	server    *http.Server
	cancel    context.CancelFunc
	record    ownerRecord
	cert      tls.Certificate
	done      chan struct{}
	serveErr  error
}

func DefaultDirectory() (string, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(home, "Library", "Application Support", "Gul"), nil
}
func New(c Config) *Host       { return &Host{config: c} }
func (h *Host) Origin() string { h.mu.Lock(); defer h.mu.Unlock(); return h.record.Origin }
func (h *Host) CertificateDER() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.cert.Certificate) == 0 {
		return nil
	}
	return append([]byte(nil), h.cert.Certificate[0]...)
}
func (h *Host) Start(ctx context.Context) (err error) {
	h.operation.Lock()
	defer h.operation.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server != nil {
		return nil
	}
	root := h.config.DataDirectory
	if e := privateDirectory(root); e != nil {
		return e
	}
	lock, e := privateOpen(filepath.Join(root, "core.lock"), unix.O_RDWR|unix.O_CREAT)
	if e != nil {
		return e
	}
	if e = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		lock.Close()
		if e == unix.EWOULDBLOCK {
			return ErrAlreadyRunning
		}
		return e
	}
	h.lock = lock
	defer func() {
		if err != nil {
			if h.listener != nil {
				h.listener.Close()
				h.listener = nil
			}
			if h.Core != nil {
				stop, c := context.WithTimeout(context.Background(), 10*time.Second)
				defer c()
				if stopErr := h.Core.Stop(stop); stopErr != nil {
					err = errors.Join(err, stopErr)
					return // Keep persistence and ownership until explicit Stop succeeds.
				}
			}
			if h.store != nil {
				h.store.Close()
				h.store = nil
			}
			unix.Flock(int(lock.Fd()), unix.LOCK_UN)
			lock.Close()
			h.lock = nil
		}
	}()
	h.cert, e = certificate(root)
	if e != nil {
		return e
	}
	port := h.config.Port
	if port < 0 || port > 65535 {
		return errors.New("invalid Gul loopback port")
	}
	if port == 0 {
		port = DefaultPort
	}
	h.listener, e = net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if e != nil {
		if errors.Is(e, unix.EADDRINUSE) {
			return errors.Join(ErrPortCollision, e)
		}
		return fmt.Errorf("bind Gul loopback HTTPS listener: %w", e)
	}
	h.record = ownerRecord{Version: 1, PID: os.Getpid(), Origin: "https://" + h.listener.Addr().String(), Instance: auth.NewSecret(), Capability: auth.NewSecret()}
	h.store, e = storage.Open(ctx, filepath.Join(root, "gul.sqlite"))
	if e != nil {
		return e
	}
	var remoteOrigins []string
	var remote *remoteGate
	if h.config.TailnetHost != "" {
		origin, e := remoteOrigin(h.config.TailnetHost)
		if e != nil {
			return e
		}
		remoteOrigins = []string{origin}
		remote = &remoteGate{hostPort: h.config.TailnetHost, port: uint16(port), inspector: h.config.ServeInspector}
	}
	accounts := auth.NewService(h.store.Auth())
	sessions := auth.NewSessions(h.store.Auth())
	h.Boundary, e = api.NewBrowserBoundary(accounts, sessions, h.record.Origin, remoteOrigins...)
	if e != nil {
		return e
	}
	provider, lifecycle := h.config.Provider, h.config.Lifecycle
	var assembled Assembly
	if h.config.Assemble != nil {
		assembled, e = h.config.Assemble(h.store)
		if e != nil {
			return e
		}
		provider, lifecycle = assembled.Provider, assembled.Lifecycle
	} else if provider == nil && lifecycle == nil {
		assembled, e = productionAssembly(ctx, h.store, h.config)
		if e != nil {
			return e
		}
		provider, lifecycle = assembled.Provider, assembled.Lifecycle
	}
	h.gateway = assembled.gateway
	h.Core = app.NewCore(app.Dependencies{Authorization: h.Boundary, Persistence: h.store, Provider: provider, Lifecycle: lifecycle})
	if e = h.Core.Start(ctx); e != nil {
		return e
	}
	features := defaultFeatures(h.Core, h.store)
	if h.config.Features != nil {
		features = h.config.Features(h.Core, h.store)
	}
	if assembled.Features != nil {
		features = assembled.Features(h.Core)
	}
	routes, e := api.NewAuthenticatedRoutes(h.Boundary, features)
	if e != nil {
		return e
	}
	mux := http.NewServeMux()

	// All RPCs share the same boundary; unmatched service names fail closed.
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/gul.v1.") {
			routes.ServeHTTP(w, r)
			return
		}
		web.BrowserHandler().ServeHTTP(w, r)
	}))
	mux.HandleFunc("/_gul/verify", h.nativeHandler)
	mux.HandleFunc("/_gul/native-bootstrap", h.nativeHandler)
	var handler http.Handler = mux
	if remote != nil {
		handler = remote.protect(mux)
	}
	allowedHosts := map[string]bool{h.listener.Addr().String(): true}
	if remote != nil {
		origin, _ := url.Parse(remoteOrigins[0])
		allowedHosts[origin.Host] = true
	}
	protectedHandler := handler
	localHost := h.listener.Addr().String()
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[r.Host] || (r.Host == localHost && proxyMarked(r.Header)) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Gul host unavailable", http.StatusForbidden)
			return
		}
		protectedHandler.ServeHTTP(w, r)
	})
	// Publish ownership before admitting handlers. A publication failure can
	// then close persistence without racing a request against that store.
	data, _ := json.Marshal(h.record)
	if e = writePrivate(filepath.Join(root, "core.json"), data); e != nil {
		return e
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.server = &http.Server{Handler: handler, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{h.cert}}, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024, BaseContext: func(net.Listener) context.Context { return serverCtx }}
	h.done = make(chan struct{})
	h.serveErr = nil
	server := h.server
	listener := h.listener
	done := h.done
	go func() {
		e := server.ServeTLS(listener, "", "")
		h.mu.Lock()
		if !errors.Is(e, http.ErrServerClosed) {
			h.serveErr = e
		}
		h.mu.Unlock()
		close(done)
	}()
	if remote != nil {
		probeCtx, c := context.WithTimeout(ctx, 3*time.Second)
		_ = remote.verify(probeCtx)
		c()
	}
	return nil
}
func (h *Host) nativeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	peer, _, e := net.SplitHostPort(r.RemoteAddr)
	origin, _ := url.Parse(h.record.Origin)
	if e != nil || net.ParseIP(peer) == nil || !net.ParseIP(peer).IsLoopback() || r.TLS == nil || r.Host != origin.Host || r.Method != http.MethodPost || r.URL.RawQuery != "" || len(r.Header.Values(NativeHeader)) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get(NativeHeader)), []byte(h.record.Capability)) != 1 || len(r.Header.Values("Origin")) != 0 {
		http.Error(w, "native host unavailable", http.StatusForbidden)
		return
	}
	var in struct {
		Challenge string `json:"challenge"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || !auth.ValidSecret(in.Challenge) || d.Decode(new(any)) != io.EOF {
		http.Error(w, "native host unavailable", http.StatusBadRequest)
		return
	}
	out := struct {
		Instance        string `json:"instance"`
		Challenge       string `json:"challenge"`
		SetupCredential string `json:"setupCredential,omitempty"`
	}{Instance: h.record.Instance, Challenge: in.Challenge}
	if r.URL.Path == "/_gul/native-bootstrap" {
		secret, e := h.Boundary.GrantNativeSetup(r.Context())
		if e != nil && !errors.Is(e, auth.ErrAlreadyConfigured) {
			http.Error(w, "native setup unavailable", http.StatusServiceUnavailable)
			return
		}
		out.SetupCredential = secret
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
func (h *Host) Stop(ctx context.Context) error {
	h.operation.Lock()
	defer h.operation.Unlock()
	h.mu.Lock()
	server := h.server
	if server == nil && h.lock == nil {
		h.mu.Unlock()
		return nil
	}
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Unlock()
	var err error
	if server != nil {
		err = server.Shutdown(ctx)
		if err != nil {
			err = errors.Join(err, server.Close())
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Core != nil {
		if stopErr := h.Core.Stop(ctx); stopErr != nil {
			return errors.Join(err, stopErr)
		}
	}
	if h.store != nil {
		err = errors.Join(err, h.store.Close())
	}
	// Keep the stable lock inode and private record. A successor rewrites only
	// after acquiring this lock; attach always performs a fresh nonce exchange.
	err = errors.Join(err, unix.Flock(int(h.lock.Fd()), unix.LOCK_UN), h.lock.Close())
	h.server = nil
	h.lock = nil
	h.store = nil
	return err
}

// Attachment owns no core lifecycle. Closing a desktop window attached to a
// headless host must never stop the existing owner's runtime.
type Attachment struct {
	Origin          string
	CertificateDER  []byte
	SetupCredential string
}

func Attach(ctx context.Context, root string) (Attachment, error) { return attach(ctx, root, true) }
func Verify(ctx context.Context, root string) (Attachment, error) { return attach(ctx, root, false) }
func attach(ctx context.Context, root string, bootstrap bool) (Attachment, error) {
	if e := checkPrivateDirectory(root, false); e != nil {
		return Attachment{}, errors.Join(ErrUnverifiedOwner, e)
	}
	lock, e := privateOpen(filepath.Join(root, "core.lock"), unix.O_RDWR)
	if e != nil {
		return Attachment{}, ErrUnverifiedOwner
	}
	defer lock.Close()
	if e = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); e == nil {
		unix.Flock(int(lock.Fd()), unix.LOCK_UN)
		return Attachment{}, ErrUnverifiedOwner
	} else if e != unix.EWOULDBLOCK {
		return Attachment{}, ErrUnverifiedOwner
	}
	data, e := readPrivate(filepath.Join(root, "core.json"))
	if e != nil {
		return Attachment{}, ErrUnverifiedOwner
	}
	var rec ownerRecord
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if d.Decode(&rec) != nil || d.Decode(new(any)) != io.EOF || rec.Version != 1 || rec.PID <= 0 || unix.Kill(rec.PID, 0) != nil || !auth.ValidSecret(rec.Instance) || !auth.ValidSecret(rec.Capability) {
		return Attachment{}, ErrUnverifiedOwner
	}
	u, e := url.Parse(rec.Origin)
	if e != nil || u.Scheme != "https" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.String() != rec.Origin {
		return Attachment{}, ErrUnverifiedOwner
	}
	// Attach never creates/rotates a certificate; only the singleton owner may.
	pemBytes, e := readPrivate(filepath.Join(root, "localhost.pem"))
	if e != nil {
		return Attachment{}, ErrUnverifiedOwner
	}
	pair, e := tls.X509KeyPair(pemBytes, pemBytes)
	clear(pemBytes)
	if e != nil {
		return Attachment{}, ErrUnverifiedOwner
	}
	leaf, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil {
		return Attachment{}, ErrUnverifiedOwner
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, VerifyConnection: func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 || !bytes.Equal(state.PeerCertificates[0].Raw, pair.Certificate[0]) {
			return ErrUnverifiedOwner
		}
		return nil
	}}, Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	challenge := auth.NewSecret()
	body, _ := json.Marshal(map[string]string{"challenge": challenge})
	endpoint := "/_gul/verify"
	if bootstrap {
		endpoint = "/_gul/native-bootstrap"
	}
	request, e := http.NewRequestWithContext(ctx, http.MethodPost, rec.Origin+endpoint, strings.NewReader(string(body)))
	if e != nil {
		return Attachment{}, ErrUnverifiedOwner
	}
	request.Header.Set(NativeHeader, rec.Capability)
	request.Header.Set("Content-Type", "application/json")
	response, e := client.Do(request)
	if e != nil {
		return Attachment{}, ErrUnverifiedOwner
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Attachment{}, ErrUnverifiedOwner
	}
	var result struct {
		Instance        string `json:"instance"`
		Challenge       string `json:"challenge"`
		SetupCredential string `json:"setupCredential"`
	}
	d = json.NewDecoder(io.LimitReader(response.Body, 4097))
	d.DisallowUnknownFields()
	if d.Decode(&result) != nil || d.Decode(new(any)) != io.EOF || result.Instance != rec.Instance || result.Challenge != challenge || (result.SetupCredential != "" && !auth.ValidSecret(result.SetupCredential)) {
		return Attachment{}, ErrUnverifiedOwner
	}
	return Attachment{Origin: rec.Origin, CertificateDER: append([]byte(nil), pair.Certificate[0]...), SetupCredential: result.SetupCredential}, nil
}
func (h *Host) NativeView(ctx context.Context) (Attachment, error) {
	return Attach(ctx, h.config.DataDirectory)
}
func (h *Host) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.serveErr == nil {
			return nil
		}
		return fmt.Errorf("Gul host stopped: %w", h.serveErr)
	}
}
