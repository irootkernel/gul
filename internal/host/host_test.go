package host

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/auth"
	"github.com/rootkernel/gul/internal/delivery/api"
	"github.com/rootkernel/gul/internal/storage"
)

func config(t *testing.T) Config {
	t.Helper()
	base, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(base, "Gul")
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return Config{DataDirectory: root, Port: port, ProviderHome: filepath.Join(base, "home"), DolgoraeExecutable: filepath.Join(base, "missing-provider")}
}
func started(t *testing.T, c Config) *Host {
	t.Helper()
	h := New(c)
	if e := h.Start(t.Context()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if e := h.Stop(ctx); e != nil {
			t.Error(e)
		}
	})
	return h
}
func client(t *testing.T, h *Host) *http.Client {
	t.Helper()
	leaf, e := x509.ParseCertificate(h.CertificateDER())
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}
}
func TestSingletonAndVerifiedNativeAttach(t *testing.T) {
	c := config(t)
	h := started(t, c)
	if e := New(c).Start(t.Context()); !errors.Is(e, ErrAlreadyRunning) {
		t.Fatalf("second owner: %v", e)
	}
	a, e := Attach(t.Context(), c.DataDirectory)
	if e != nil || a.Origin != h.Origin() || !auth.ValidSecret(a.SetupCredential) || string(a.CertificateDER) != string(h.CertificateDER()) {
		t.Fatalf("verified attach failed: %v", e)
	}
	// Neither an ordinary browser nor a forged native request can mint permission.
	for _, headers := range []map[string]string{{}, {NativeHeader: auth.NewSecret()}, {NativeHeader: h.record.Capability, "Origin": h.Origin()}} {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.Origin()+"/_gul/native-bootstrap", strings.NewReader(`{"challenge":"`+auth.NewSecret()+`"}`))
		for k, v := range headers {
			request.Header.Set(k, v)
		}
		response, e := client(t, h).Do(request)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("forged bootstrap %d", response.StatusCode)
		}
	}
	rpc := gulv1connect.NewAuthServiceClient(client(t, h), h.Origin())
	password := []byte("abcdefghijklmnop")
	setup := connect.NewRequest(&gulv1.FirstRunSetupRequest{Password: password})
	setup.Header().Set("Origin", h.Origin())
	setup.Header().Set(api.BrowserHeader, "1")
	setup.Header().Set(api.SetupHeader, a.SetupCredential)
	if _, e := rpc.FirstRunSetup(t.Context(), setup); e != nil {
		t.Fatal(e)
	}
	login := connect.NewRequest(&gulv1.LoginRequest{Password: password})
	login.Header().Set("Origin", h.Origin())
	login.Header().Set(api.BrowserHeader, "1")
	response, e := rpc.Login(t.Context(), login)
	if e != nil || !response.Msg.Session.Authenticated {
		t.Fatalf("actual hosted login %v", e)
	}
	state := connect.NewRequest(&gulv1.GetSessionRequest{})
	state.Header().Set("Origin", h.Origin())
	state.Header().Set(api.BrowserHeader, "1")
	cookie := response.Header().Get("Set-Cookie")
	state.Header().Set("Cookie", strings.Split(cookie, ";")[0])
	if r, e := rpc.GetSession(t.Context(), state); e != nil || !r.Msg.Session.Authenticated {
		t.Fatalf("cookie %v", e)
	}
	if a, e = Attach(t.Context(), c.DataDirectory); e != nil || a.SetupCredential != "" {
		t.Fatalf("configured native boot %v", e)
	}
}
func TestSpoofStaleCertificateAndCollisionFailClosed(t *testing.T) {
	c := config(t)
	h := started(t, c)
	name := filepath.Join(c.DataDirectory, "core.json")
	original, e := readPrivate(name)
	if e != nil {
		t.Fatal(e)
	}
	for _, alter := range []func(*ownerRecord){func(r *ownerRecord) { r.Instance = auth.NewSecret() }, func(r *ownerRecord) { r.Capability = auth.NewSecret() }, func(r *ownerRecord) { r.Origin = "http://127.0.0.1:9" }, func(r *ownerRecord) { r.PID = 2147483647 }} {
		rec := h.record
		alter(&rec)
		data, _ := json.Marshal(rec)
		if e := writePrivate(name, data); e != nil {
			t.Fatal(e)
		}
		if _, e := Attach(t.Context(), c.DataDirectory); !errors.Is(e, ErrUnverifiedOwner) {
			t.Fatalf("spoof accepted: %v", e)
		}
	}
	if e := writePrivate(name, original); e != nil {
		t.Fatal(e)
	}
	c2 := config(t)
	c2.Port = c.Port
	if e := New(c2).Start(t.Context()); !errors.Is(e, ErrPortCollision) {
		t.Fatalf("port collision %v", e)
	}
	other := config(t)
	if e := privateDirectory(other.DataDirectory); e != nil {
		t.Fatal(e)
	}
	if _, e := certificate(other.DataDirectory); e != nil {
		t.Fatal(e)
	}
	pem, e := readPrivate(filepath.Join(other.DataDirectory, "localhost.pem"))
	if e != nil {
		t.Fatal(e)
	}
	if e := writePrivate(filepath.Join(c.DataDirectory, "localhost.pem"), pem); e != nil {
		t.Fatal(e)
	}
	if _, e := Attach(t.Context(), c.DataDirectory); !errors.Is(e, ErrUnverifiedOwner) {
		t.Fatalf("foreign certificate accepted %v", e)
	}
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	if e := h.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := Attach(t.Context(), c.DataDirectory); !errors.Is(e, ErrUnverifiedOwner) {
		t.Fatalf("stale owner accepted %v", e)
	}
}
func TestProtectedStateAndLoopbackListener(t *testing.T) {
	c := config(t)
	h := started(t, c)
	if ip := h.listener.Addr().(*net.TCPAddr).IP; !ip.IsLoopback() {
		t.Fatal("nonloopback")
	}
	for _, name := range []string{"core.lock", "core.json", "localhost.pem", "gul.sqlite"} {
		info, e := os.Lstat(filepath.Join(c.DataDirectory, name))
		if e != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("unprotected %s %v", name, e)
		}
	}
	if e := os.Chmod(filepath.Join(c.DataDirectory, "core.json"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := Attach(t.Context(), c.DataDirectory); !errors.Is(e, ErrUnverifiedOwner) {
		t.Fatalf("permissive metadata accepted %v", e)
	}
	unsafe := config(t)
	if e := os.Mkdir(unsafe.DataDirectory, 0755); e != nil {
		t.Fatal(e)
	}
	if e := New(unsafe).Start(t.Context()); !errors.Is(e, ErrUnsafeState) {
		t.Fatalf("unsafe directory %v", e)
	}
	sym := config(t)
	if e := os.Symlink(c.DataDirectory, sym.DataDirectory); e != nil {
		t.Fatal(e)
	}
	if e := New(sym).Start(t.Context()); !errors.Is(e, ErrUnsafeState) {
		t.Fatalf("symlink %v", e)
	}
}

// A failed owned lifecycle stop retains the singleton. Releasing it would let
// another invocation start while an owned provider may still be running.
type stopLifecycle struct{ fails bool }

func (*stopLifecycle) Start(context.Context) error { return nil }
func (l *stopLifecycle) Stop(context.Context) error {
	if l.fails {
		return errors.New("owned lifecycle still stopping")
	}
	return nil
}
func TestStopFailureRetainsOwnershipAndRestartReusesDurableState(t *testing.T) {
	c := config(t)
	l := &stopLifecycle{fails: true}
	c.Lifecycle = l
	h := started(t, c)
	view, err := Attach(t.Context(), c.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	rpc := gulv1connect.NewAuthServiceClient(client(t, h), h.Origin())
	setup := connect.NewRequest(&gulv1.FirstRunSetupRequest{Password: []byte("persistent host password")})
	setup.Header().Set("Origin", h.Origin())
	setup.Header().Set(api.BrowserHeader, "1")
	setup.Header().Set(api.SetupHeader, view.SetupCredential)
	if _, err := rpc.FirstRunSetup(t.Context(), setup); err != nil {
		t.Fatal(err)
	}
	login := connect.NewRequest(&gulv1.LoginRequest{Password: []byte("persistent host password")})
	login.Header().Set("Origin", h.Origin())
	login.Header().Set(api.BrowserHeader, "1")
	session, err := rpc.Login(t.Context(), login)
	if err != nil || !session.Msg.Session.Authenticated {
		t.Fatalf("pre-restart login: %v", err)
	}
	cookie := strings.Split(session.Header().Get("Set-Cookie"), ";")[0]
	certificateBefore := h.CertificateDER()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if e := h.Stop(ctx); e == nil {
		t.Fatal("unknown stop accepted")
	}
	if e := New(c).Start(t.Context()); !errors.Is(e, ErrAlreadyRunning) {
		t.Fatalf("released uncertain owner: %v", e)
	}
	if _, err := h.store.Auth().PasswordAccount(t.Context()); err != nil {
		t.Fatalf("failed stop closed owned persistence: %v", err)
	}
	l.fails = false
	if e := h.Stop(t.Context()); e != nil {
		t.Fatal(e)
	}
	next := started(t, c)
	if string(next.CertificateDER()) != string(certificateBefore) {
		t.Fatal("restart changed certificate")
	}
	state := connect.NewRequest(&gulv1.GetSessionRequest{})
	state.Header().Set("Origin", next.Origin())
	state.Header().Set(api.BrowserHeader, "1")
	state.Header().Set("Cookie", cookie)
	nextRPC := gulv1connect.NewAuthServiceClient(client(t, next), next.Origin())
	if persisted, err := nextRPC.GetSession(t.Context(), state); err != nil || !persisted.Msg.Session.Authenticated {
		t.Fatalf("host restart lost durable account/session: %v", err)
	}
	login.Header().Set("Origin", next.Origin())
	if persisted, err := nextRPC.Login(t.Context(), login); err != nil || !persisted.Msg.Session.Authenticated {
		t.Fatalf("host restart lost password verification: %v", err)
	}
	if _, e := Verify(t.Context(), c.DataDirectory); e != nil {
		t.Fatalf("restart verify %v", e)
	}
}

func TestVerifiedHealthProbeDoesNotRotateNativeGrant(t *testing.T) {
	c := config(t)
	h := started(t, c)
	a, e := Attach(t.Context(), c.DataDirectory)
	if e != nil {
		t.Fatal(e)
	}
	for range 2 {
		v, e := Verify(t.Context(), c.DataDirectory)
		if e != nil || v.SetupCredential != "" {
			t.Fatalf("health mint %v", e)
		}
	}
	request := connect.NewRequest(&gulv1.FirstRunSetupRequest{Password: []byte("verify health password")})
	request.Header().Set("Origin", h.Origin())
	request.Header().Set(api.BrowserHeader, "1")
	request.Header().Set(api.SetupHeader, a.SetupCredential)
	rpc := gulv1connect.NewAuthServiceClient(client(t, h), h.Origin())
	if _, e := rpc.FirstRunSetup(t.Context(), request); e != nil {
		t.Fatalf("health revoked native grant: %v", e)
	}
}

func TestStartPreservesAndRejectsInvalidPersistedCertificate(t *testing.T) {
	for _, damaged := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "damaged"}[damaged], func(t *testing.T) {
			c := config(t)
			if err := privateDirectory(c.DataDirectory); err != nil {
				t.Fatal(err)
			}
			pair, err := certificate(c.DataDirectory)
			if err != nil {
				t.Fatal(err)
			}
			var data []byte
			if damaged {
				data = []byte("truncated certificate")
			} else {
				leaf := *pair.Leaf
				leaf.NotBefore = time.Now().Add(-2 * time.Hour)
				leaf.NotAfter = time.Now().Add(-time.Hour)
				key := pair.PrivateKey.(*ecdsa.PrivateKey)
				der, err := x509.CreateCertificate(rand.Reader, &leaf, &leaf, &key.PublicKey, key)
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := x509.MarshalPKCS8PrivateKey(key)
				if err != nil {
					t.Fatal(err)
				}
				data = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})...)
			}
			name := filepath.Join(c.DataDirectory, "localhost.pem")
			if err := writePrivate(name, data); err != nil {
				t.Fatal(err)
			}
			if err := New(c).Start(t.Context()); err == nil {
				t.Fatal("invalid persisted certificate started")
			}
			after, err := readPrivate(name)
			if err != nil || string(after) != string(data) {
				t.Fatalf("invalid certificate replaced: %v", err)
			}
			if _, err := Verify(t.Context(), c.DataDirectory); !errors.Is(err, ErrUnverifiedOwner) {
				t.Fatalf("failed startup became healthy: %v", err)
			}
		})
	}
}

func TestStartRejectsInvalidTailnetEndpoints(t *testing.T) {
	for _, endpoint := range []string{"example.com:443", "Node.ts.net:443", ".ts.net:443", "node.ts.net:0", "node.ts.net:0443", "node.ts.net:"} {
		t.Run(endpoint, func(t *testing.T) {
			c := config(t)
			c.TailnetHost = endpoint
			if err := New(c).Start(t.Context()); err == nil {
				t.Fatal("invalid endpoint admitted")
			}
			c.TailnetHost = ""
			started(t, c)
		})
	}
}

func TestNativeBoundaryRejectsMalformedRequests(t *testing.T) {
	h := started(t, config(t))
	body := `{"challenge":"` + auth.NewSecret() + `"}`
	for _, test := range []struct {
		name, method, suffix, body string
		duplicate                  bool
		status                     int
	}{
		{"method", "GET", "", body, false, http.StatusForbidden},
		{"query", "POST", "?x=1", body, false, http.StatusForbidden},
		{"duplicate capability", "POST", "", body, true, http.StatusForbidden},
		{"body bound", "POST", "", body + strings.Repeat(" ", 1024), false, http.StatusBadRequest},
		{"unknown field", "POST", "", strings.TrimSuffix(body, "}") + `,"extra":true}`, false, http.StatusBadRequest},
		{"trailing JSON", "POST", "", body + ` {}`, false, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, _ := http.NewRequestWithContext(t.Context(), test.method, h.Origin()+"/_gul/native-bootstrap"+test.suffix, strings.NewReader(test.body))
			r.Header.Set(NativeHeader, h.record.Capability)
			if test.duplicate {
				r.Header.Add(NativeHeader, h.record.Capability)
			}
			response, err := client(t, h).Do(r)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != test.status {
				t.Fatalf("malformed request admitted: %d", response.StatusCode)
			}
		})
	}
}

func TestWaitAfterCleanStop(t *testing.T) {
	h := started(t, config(t))
	if err := h.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := h.Wait(ctx); err != nil {
		t.Fatalf("clean stop: %v", err)
	}
}

func TestFailedStartupRetainsOwnershipUntilCoreStopSucceeds(t *testing.T) {
	c := config(t)
	lifecycle := &stopLifecycle{fails: true}
	c.Lifecycle = lifecycle
	c.TailnetHost = "gul.tail123.ts.net:443"
	inspector := &fixtureServe{status: serveSnapshot(c.Port, false), source: true}
	c.ServeInspector = inspector
	c.Features = func(core *app.Core, store *storage.Store) api.FeatureHandlers {
		if err := os.WriteFile(filepath.Join(c.DataDirectory, "core.json"), []byte("unsafe owner record"), 0644); err != nil {
			t.Fatal(err)
		}
		return defaultFeatures(core, store)
	}
	h := New(c)
	if err := h.Start(t.Context()); err == nil {
		t.Fatal("unsafe owner record published")
	}
	if inspector.calls != 0 {
		t.Fatal("failed publication admitted remote inspection after starting HTTP")
	}
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", fmt.Sprint(c.Port)), time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("failed publication left a reachable listener")
	}
	if err := New(c).Start(t.Context()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("uncertain startup released ownership: %v", err)
	}
	if err := h.Stop(t.Context()); err == nil {
		t.Fatal("unknown stop accepted")
	}
	lifecycle.fails = false
	if err := h.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	c.Features = nil
	if err := os.Remove(filepath.Join(c.DataDirectory, "core.json")); err != nil {
		t.Fatal(err)
	}
	started(t, c)
}

func TestWaitReportsUnexpectedListenerFailure(t *testing.T) {
	h := started(t, config(t))
	if err := h.listener.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := h.Wait(ctx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected server failure not reported: %v", err)
	}
}

func TestInvalidLoopbackPortsFailClosed(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		c := config(t)
		c.Port = port
		h := New(c)
		if err := h.Start(t.Context()); err == nil || !strings.Contains(err.Error(), "invalid Gul loopback port") {
			t.Fatalf("port %d: %v", port, err)
		}
		if h.lock != nil || h.store != nil {
			t.Fatal("invalid port retained ownership")
		}
	}
}

func TestZeroPortSelectsFixedDefault(t *testing.T) {
	c := config(t)
	c.Port = 0
	h := New(c)
	if err := h.Start(t.Context()); err != nil {
		if errors.Is(err, ErrPortCollision) {
			return // An existing owner is never displaced or given a fallback.
		}
		t.Fatal(err)
	}
	defer func() {
		if err := h.Stop(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	if h.Origin() != "https://127.0.0.1:17423" {
		t.Fatalf("default port changed: %s", h.Origin())
	}
}

func TestNativeBootstrapUnavailableAccountFailsClosed(t *testing.T) {
	c := config(t)
	h := started(t, c)
	h.Boundary.Accounts = auth.NewService(nil)
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost,
		h.Origin()+"/_gul/native-bootstrap", strings.NewReader(`{"challenge":"`+auth.NewSecret()+`"}`))
	request.Header.Set(NativeHeader, h.record.Capability)
	response, err := client(t, h).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unavailable account bootstrap: %d", response.StatusCode)
	}
	if _, err := Attach(t.Context(), c.DataDirectory); !errors.Is(err, ErrUnverifiedOwner) {
		t.Fatalf("unavailable account attached: %v", err)
	}
}
