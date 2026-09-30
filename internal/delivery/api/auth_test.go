package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/auth"
	"github.com/rootkernel/gul/internal/files"
	"github.com/rootkernel/gul/internal/presentation"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type authFixture struct {
	server   *httptest.Server
	boundary *BrowserBoundary
	store    *storage.Store
	client   gulv1connect.AuthServiceClient
	core     *app.Core
	fileRoot string
}

func newAuthFixture(t *testing.T, configured bool) *authFixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.Context(), filepath.Join(dir, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	accounts := auth.NewService(store.Auth())
	if configured {
		if err := store.Auth().CreateAccount(t.Context(), "owner", time.Now()); err != nil {
			t.Fatal(err)
		}
		grant, err := accounts.GrantLocalSetup(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := accounts.FirstRunSetup(t.Context(), grant, []byte("browser password 한글 😀")); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	boundary, err := NewBrowserBoundary(accounts, auth.NewSessions(store.Auth()), origin, "https://gul.example.ts.net")
	if err != nil {
		t.Fatal(err)
	}
	core := app.NewCore(app.Dependencies{Authorization: boundary, Persistence: ready{}})
	if err := core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { core.Stop(context.Background()) })
	// Deliberately inject a foreign resolver: assembly must replace it.
	foreign := func(context.Context) (app.Principal, error) { return app.Principal{Subject: "foreign"}, nil }
	fileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(fileRoot)
	if err != nil {
		t.Fatal(err)
	}
	identity := info.Sys().(*syscall.Stat_t)
	fileEntry := workspace.Attachment{SubjectID: "owner", ID: "entry", CanonicalRoot: fileRoot, ProviderID: "provider-workspace", FileDevice: fmt.Sprint(identity.Dev), FileInode: fmt.Sprint(identity.Ino)}
	routes, err := NewAuthenticatedRoutes(boundary, FeatureHandlers{
		Runtime:     &RuntimeHandler{Core: core, Principal: foreign},
		Workspace:   &WorkspaceHandler{Core: core, Presentation: presentation.NewService(store.Presentation()), Principal: foreign},
		Direct:      &DirectPresentationHandler{Core: core, Principal: foreign},
		Artifact:    &ArtifactHandler{Core: core, Principal: foreign},
		Interaction: &InteractionHandler{Core: core, Principal: foreign},
		Writer:      &WriterHandler{Core: core, Principal: foreign},
		Files:       &FileHandler{Core: core, Principal: foreign, Files: files.NewService(fileAttachments{fileEntry})},
		Events:      &ClientEventHandler{Core: core, Principal: foreign, Events: &eventReader{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = routes
	server.StartTLS()
	t.Cleanup(server.Close)
	return &authFixture{server, boundary, store, gulv1connect.NewAuthServiceClient(server.Client(), server.URL), core, fileRoot}
}

func authHeaders[T any](f *authFixture, msg *T, token, csrf string) *connect.Request[T] {
	r := connect.NewRequest(msg)
	r.Header().Set("Origin", f.server.URL)
	r.Header().Set(BrowserHeader, "1")
	if token != "" {
		r.Header().Set("Cookie", SessionCookie+"="+token)
	}
	if csrf != "" {
		r.Header().Set(CSRFHeader, csrf)
	}
	return r
}

func loginFixture(t *testing.T, f *authFixture, old string) (string, string) {
	t.Helper()
	r, err := f.client.Login(t.Context(), authHeaders(f, &gulv1.LoginRequest{Password: []byte("browser password 한글 😀")}, old, ""))
	if err != nil {
		t.Fatal(err)
	}
	cookies := (&http.Response{Header: http.Header{"Set-Cookie": r.Header().Values("Set-Cookie")}}).Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session cookie")
	}
	c := cookies[0]
	if c.Name != SessionCookie || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" || c.MaxAge != 604800 || !auth.ValidSecret(c.Value) {
		t.Fatal("unsafe session cookie")
	}
	if !r.Msg.Session.Authenticated || r.Msg.Session.CsrfToken != auth.CSRFToken(c.Value) || r.Msg.Session.ExpiresAt == nil || time.Until(r.Msg.Session.ExpiresAt.AsTime()) < auth.SessionLifetime-time.Minute {
		t.Fatal("invalid browser session projection")
	}
	return c.Value, r.Msg.Session.CsrfToken
}

func TestAuthRPCSetupLoginRotationAndLogout(t *testing.T) {
	f := newAuthFixture(t, false)
	state, err := f.client.GetSession(t.Context(), authHeaders(f, &gulv1.GetSessionRequest{}, "", ""))
	if err != nil || state.Msg.Session.AccountConfigured || state.Msg.Session.Authenticated {
		t.Fatal("anonymous state", err)
	}
	request := authHeaders(f, &gulv1.FirstRunSetupRequest{Password: []byte("browser password 한글 😀")}, "", "")
	if _, err := f.client.FirstRunSetup(t.Context(), request); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("remote takeover allowed", err)
	}
	grant, err := f.boundary.GrantNativeSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	request.Header().Set(SetupHeader, grant)
	if _, err := f.client.FirstRunSetup(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.FirstRunSetup(t.Context(), request); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("reused bootstrap accepted", err)
	}
	if _, err := f.boundary.GrantNativeSetup(t.Context()); err != auth.ErrAlreadyConfigured {
		t.Fatal(err)
	}
	first, csrf := loginFixture(t, f, "")
	state, err = f.client.GetSession(t.Context(), authHeaders(f, &gulv1.GetSessionRequest{}, first, ""))
	if err != nil || state.Msg.Session.CsrfToken != csrf {
		t.Fatal("saved browser session", err)
	}
	second, secondCSRF := loginFixture(t, f, first)
	if second == first {
		t.Fatal("login failed to rotate")
	}
	if _, err := f.boundary.Sessions.Inspect(t.Context(), first); err != auth.ErrInvalidSession {
		t.Fatal("old login survived", err)
	}
	if _, err := f.client.Logout(t.Context(), authHeaders(f, &gulv1.LogoutRequest{}, second, "bad")); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("CSRF logout accepted", err)
	}
	response, err := f.client.Logout(t.Context(), authHeaders(f, &gulv1.LogoutRequest{}, second, secondCSRF))
	if err != nil {
		t.Fatal(err)
	}
	cookies := (&http.Response{Header: http.Header{"Set-Cookie": response.Header().Values("Set-Cookie")}}).Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("logout did not clear protected cookie")
	}
	state, err = f.client.GetSession(t.Context(), authHeaders(f, &gulv1.GetSessionRequest{}, second, ""))
	if err != nil || state.Msg.Session.Authenticated || state.Msg.Session.CsrfToken != "" {
		t.Fatal("revoked session survived", err)
	}
	if _, err := f.client.Logout(t.Context(), authHeaders(f, &gulv1.LogoutRequest{}, second, secondCSRF)); err != nil {
		t.Fatal("logout not idempotent", err)
	}
}

func TestAuthOriginAndBruteForceBeforePasswordWork(t *testing.T) {
	f := newAuthFixture(t, true)
	for _, kind := range []string{"missing-origin", "foreign", "null", "duplicate", "spoof-host", "missing-header", "forwarded"} {
		r := authHeaders(f, &gulv1.LoginRequest{Password: []byte("browser password 한글 😀")}, "", "")
		switch kind {
		case "missing-origin":
			r.Header().Del("Origin")
		case "foreign":
			r.Header().Set("Origin", "https://evil.invalid")
		case "null":
			r.Header().Set("Origin", "null")
		case "duplicate":
			r.Header().Add("Origin", f.server.URL)
		case "spoof-host":
			r.Header().Set("Origin", "https://gul.example.ts.net")
		case "missing-header":
			r.Header().Del(BrowserHeader)
		case "forwarded":
			r.Header().Set("Origin", "https://gul.example.ts.net")
			r.Header().Set("X-Forwarded-Host", "gul.example.ts.net")
		}
		if _, err := f.client.Login(t.Context(), r); connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatal(kind, err)
		}
	}
	for range 5 {
		if _, err := f.client.Login(t.Context(), authHeaders(f, &gulv1.LoginRequest{Password: []byte("incorrect password")}, "", "")); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatal(err)
		}
	}
	if _, err := f.client.Login(t.Context(), authHeaders(f, &gulv1.LoginRequest{Password: []byte("browser password 한글 😀")}, "", "")); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatal("global attempt limit bypassed", err)
	}
	f.boundary.mu.Lock()
	f.boundary.loginWindow.start = time.Now().Add(-time.Minute)
	f.boundary.mu.Unlock()
	loginFixture(t, f, "")
}

func TestSetupAndLoginShareTheAttemptWindow(t *testing.T) {
	f := newAuthFixture(t, false)
	for range 3 {
		_, err := f.client.FirstRunSetup(t.Context(), authHeaders(f, &gulv1.FirstRunSetupRequest{}, "", ""))
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatal("setup attempt not denied", err)
		}
	}
	for range 2 {
		_, _ = f.client.Login(t.Context(), authHeaders(f, &gulv1.LoginRequest{Password: []byte("incorrect password")}, "", ""))
	}
	_, err := f.client.FirstRunSetup(t.Context(), authHeaders(f, &gulv1.FirstRunSetupRequest{}, "", ""))
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatal("setup bypassed shared login budget", err)
	}
}

func TestFeatureRequestBodyBoundBeforeDecode(t *testing.T) {
	f := newAuthFixture(t, true)
	token, csrf := loginFixture(t, f, "")
	for _, size := range []int{32, 256*1024 + 1} {
		body := `{"unknown":"` + strings.Repeat("a", size) + `"}`
		r := httptest.NewRequest(http.MethodPost, f.server.URL+gulv1connect.WorkspacePresentationServiceGetNavigationProcedure, strings.NewReader(body))
		r.Header.Set("Origin", f.server.URL)
		r.Header.Set(BrowserHeader, "1")
		r.Header.Set(CSRFHeader, csrf)
		r.Header.Set("Cookie", SessionCookie+"="+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Connect-Protocol-Version", "1")
		w := httptest.NewRecorder()
		f.server.Config.Handler.ServeHTTP(w, r)
		if size == 32 && w.Code != http.StatusOK {
			t.Fatal("bounded request did not reach feature", w.Code, w.Body.String())
		}
		if size > 256*1024 && (w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "configured max 262144")) {
			t.Fatal("oversize body reached feature decode", w.Code, w.Body.String())
		}
	}
}

func TestAllDeclaredFeatureRoutesUseTheSameProtection(t *testing.T) {
	f := newAuthFixture(t, true)
	token, csrf := loginFixture(t, f, "")
	services := gulv1.File_gul_v1_gul_proto.Services()
	var delegated atomic.Int32
	expected := 0
	spy := f.boundary.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := f.boundary.Principal(r.Context())
		if err != nil || principal.Subject != "owner" || f.boundary.Authorize(r.Context(), principal) != nil {
			t.Error("untrusted principal", err)
		}
		delegated.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < services.Len(); i++ {
		service := services.Get(i)
		if service.Name() == "AuthService" {
			continue
		}
		for j := 0; j < service.Methods().Len(); j++ {
			expected++
			path := "/" + string(service.FullName()) + "/" + string(service.Methods().Get(j).Name())
			for _, kind := range []string{"anonymous", "csrf", "missing-csrf", "duplicate-csrf", "origin", "duplicate-cookie", "malformed-cookie", "valid"} {
				r := httptest.NewRequest(http.MethodPost, f.server.URL+path, strings.NewReader("{}"))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Connect-Protocol-Version", "1")
				r.Header.Set("Origin", f.server.URL)
				r.Header.Set(BrowserHeader, "1")
				r.Header.Set("Cookie", SessionCookie+"="+token)
				r.Header.Set(CSRFHeader, csrf)
				want := http.StatusNoContent
				switch kind {
				case "anonymous":
					r.Header.Del("Cookie")
					want = http.StatusUnauthorized
				case "csrf":
					r.Header.Set(CSRFHeader, "forged")
					want = http.StatusForbidden
				case "missing-csrf":
					r.Header.Del(CSRFHeader)
					want = http.StatusForbidden
				case "duplicate-csrf":
					r.Header.Add(CSRFHeader, csrf)
					want = http.StatusForbidden
				case "origin":
					r.Header.Set("Origin", "https://evil.invalid")
					want = http.StatusForbidden
				case "duplicate-cookie":
					r.Header.Add("Cookie", SessionCookie+"="+token)
					want = http.StatusUnauthorized
				case "malformed-cookie":
					r.Header.Set("Cookie", SessionCookie+"=short")
					want = http.StatusUnauthorized
				}
				w := httptest.NewRecorder()
				if kind == "valid" {
					spy.ServeHTTP(w, r)
				} else {
					f.server.Config.Handler.ServeHTTP(w, r)
				}
				if w.Code != want {
					t.Fatalf("%s %s: status%d want%d: %s", path, kind, w.Code, want, w.Body.String())
				}
			}
		}
	}
	if delegated.Load() != int32(expected) {
		t.Fatal("route inventory incomplete")
	}
	client := gulv1connect.NewWorkspacePresentationServiceClient(f.server.Client(), f.server.URL)
	if _, err := client.GetNavigation(t.Context(), authHeaders(f, &gulv1.GetNavigationRequest{}, token, csrf)); err != nil {
		t.Fatal("authenticated local route/authorization not wired", err)
	}
}

type accountStateRepository struct {
	auth.AccountRepository
	account auth.PasswordAccount
	err     error
}

func (r accountStateRepository) PasswordAccount(context.Context) (auth.PasswordAccount, error) {
	return r.account, r.err
}

func TestSessionCheckNeverOffersSetupForUnavailableAccount(t *testing.T) {
	for _, kind := range []string{"empty-subject", "damaged-hash", "repository-error", "closed-store"} {
		t.Run(kind, func(t *testing.T) {
			f := newAuthFixture(t, true)
			account, err := f.store.Auth().PasswordAccount(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			r := accountStateRepository{AccountRepository: f.store.Auth(), account: account}
			switch kind {
			case "empty-subject":
				r.account.SubjectID = ""
			case "damaged-hash":
				r.account.Hash = "damaged"
			case "repository-error":
				r.err = errors.New("private repository detail")
			case "closed-store":
				if err := f.store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if kind != "closed-store" {
				f.boundary.Accounts = auth.NewService(r)
			}
			if configured, err := f.boundary.Accounts.AccountConfigured(t.Context()); configured || !errors.Is(err, auth.ErrUnavailable) {
				t.Fatal("unsafe account reported setup state", configured, err)
			}
			response, err := f.client.GetSession(t.Context(), authHeaders(f, &gulv1.GetSessionRequest{}, "", ""))
			var failure *connect.Error
			if response != nil || !errors.As(err, &failure) || failure.Code() != connect.CodeUnavailable || failure.Message() != "authentication unavailable" {
				t.Fatal("unsafe account offered setup or leaked error", response, err)
			}
		})
	}
}

func TestAuthPasswordBuffersAndTransportBounds(t *testing.T) {
	f := newAuthFixture(t, false)
	grant, err := f.boundary.Accounts.GrantLocalSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	h := &AuthHandler{Boundary: f.boundary}
	ctx := context.WithValue(t.Context(), authRequestKey{}, &authRequest{owner: f.boundary, grant: grant})
	for _, kind := range []string{"setup", "login", "denied"} {
		password := []byte("browser password 한글 😀")
		if kind == "setup" {
			_, err = h.FirstRunSetup(ctx, connect.NewRequest(&gulv1.FirstRunSetupRequest{Password: password}))
		} else {
			callctx := ctx
			if kind == "denied" {
				callctx = t.Context()
			}
			_, err = h.Login(callctx, connect.NewRequest(&gulv1.LoginRequest{Password: password}))
		}
		if kind != "denied" && err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(password, make([]byte, len(password))) {
			t.Fatal("decoded password retained")
		}
	}
	r := httptest.NewRequest(http.MethodPost, f.server.URL+gulv1connect.AuthServiceLoginProcedure, strings.NewReader(`{"password":"`+strings.Repeat("a", 6000)+`"}`))
	r.Header.Set("Origin", f.server.URL)
	r.Header.Set(BrowserHeader, "1")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Connect-Protocol-Version", "1")
	w := httptest.NewRecorder()
	f.server.Config.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "configured max 4096") {
		t.Fatal("oversized auth body accepted", w.Code)
	}
}

func TestAuthStreamTerminatesOnLogoutRevocationAndExpiry(t *testing.T) {
	for _, kind := range []string{"logout", "external-revoke", "expiry"} {
		t.Run(kind, func(t *testing.T) {
			f := newAuthFixture(t, true)
			token, csrf := loginFixture(t, f, "")
			if kind == "expiry" {
				token = auth.NewSecret()
				csrf = auth.CSRFToken(token)
				if err := f.store.Auth().IssueSession(t.Context(), auth.SecretDigest(token), "owner", time.Now().Add(600*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
			}
			client := gulv1connect.NewClientEventServiceClient(f.server.Client(), f.server.URL)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			stream, err := client.WatchClientEvents(ctx, authHeaders(f, &gulv1.WatchClientEventsRequest{SessionId: "session"}, token, csrf))
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if !stream.Receive() {
				t.Fatal("stream did not open", stream.Err())
			}
			if kind == "logout" {
				_, err = f.client.Logout(t.Context(), authHeaders(f, &gulv1.LogoutRequest{}, token, csrf))
			}
			if kind == "external-revoke" {
				err = auth.NewSessions(f.store.Auth()).Revoke(t.Context(), token)
			}
			if err != nil {
				t.Fatal(err)
			}
			if stream.Receive() {
				t.Fatal("event delivered after session invalidation")
			}
			if ctx.Err() != nil {
				t.Fatal("stream survived until test deadline")
			}
			if stream.Err() == nil {
				t.Fatal("stream closed without auth/expiry error")
			}
		})
	}
}

func TestBrowserBoundaryRejectsInvalidOriginsAndMissingComposition(t *testing.T) {
	f := newAuthFixture(t, true)
	for _, origin := range []string{"https://localhost", "https://192.0.2.1:443", "https://[2001:db8::1]"} {
		if _, err := NewBrowserBoundary(f.boundary.Accounts, f.boundary.Sessions, origin); err == nil || err.Error() != "native setup origin must use a loopback IP" {
			t.Fatal("non-loopback native origin not rejected by IP guard", origin, err)
		}
	}
	if _, err := NewBrowserBoundary(f.boundary.Accounts, f.boundary.Sessions, "https://[::1]:8443"); err != nil {
		t.Fatal("IPv6 loopback native origin rejected", err)
	}
	for _, origin := range []string{"http://127.0.0.1", "https://user:pass@localhost", "https://localhost/", "https://localhost?x=1", "https://localhost#secret", "https://", "*"} {
		if _, err := NewBrowserBoundary(f.boundary.Accounts, f.boundary.Sessions, origin); err == nil {
			t.Fatal("invalid origin accepted", origin)
		}
	}
	if _, err := NewAuthenticatedRoutes(nil, FeatureHandlers{}); err == nil {
		t.Fatal("missing boundary accepted")
	}
	if _, err := f.boundary.Principal(t.Context()); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("unbound principal accepted")
	}
	r, _ := http.NewRequest(http.MethodGet, f.server.URL+gulv1connect.AuthServiceGetSessionProcedure, nil)
	response, err := f.server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusForbidden || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("GET/cache/CORS bypass")
	}
}

func TestIdleFileStreamClosesWhenBrowserSessionIsRevoked(t *testing.T) {
	f := newAuthFixture(t, true)
	token, csrf := loginFixture(t, f, "")
	client := gulv1connect.NewFileServiceClient(f.server.Client(), f.server.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	// The file stream sends no initial item. Invalidate once to establish that
	// the watcher is open, then revoke while it is idle without another event.
	type openedStream struct {
		stream *connect.ServerStreamForClient[gulv1.FileChange]
		err    error
	}
	result := make(chan openedStream, 1)
	go func() {
		stream, err := client.WatchFileChanges(ctx, authHeaders(f, &gulv1.WatchFileChangesRequest{WorkspaceId: "entry"}, token, csrf))
		result <- openedStream{stream, err}
	}()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var opened openedStream
	for opened.stream == nil {
		select {
		case <-ticker.C:
			if err := os.WriteFile(filepath.Join(f.fileRoot, "public.txt"), []byte("public fixture"), 0600); err != nil {
				t.Fatal(err)
			}
		case opened = <-result:
			if opened.err != nil {
				t.Fatal("file stream did not open", opened.err)
			}
		case <-ctx.Done():
			t.Fatal("file stream did not open")
		}
	}
	stream := opened.stream
	defer stream.Close()
	if !stream.Receive() {
		t.Fatal("file stream did not deliver initial invalidation", stream.Err())
	}
	if err := auth.NewSessions(f.store.Auth()).Revoke(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if stream.Receive() {
		t.Fatal("file stream survived revocation")
	}
	if ctx.Err() != nil || stream.Err() == nil {
		t.Fatal("idle file stream was not bounded by session revocation")
	}
}

func TestSetupCredentialIsLocalBoundAndSupersedesPriorSecret(t *testing.T) {
	f := newAuthFixture(t, false)
	first, err := f.boundary.GrantNativeSetup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.boundary.GrantNativeSetup(t.Context())
	if err != nil || first == second {
		t.Fatal(err)
	}
	for _, kind := range []string{"superseded", "foreign", "remote", "http"} {
		r := httptest.NewRequest(http.MethodPost, f.server.URL+gulv1connect.AuthServiceFirstRunSetupProcedure, strings.NewReader("{}"))
		r.Header.Set("Origin", f.server.URL)
		r.Header.Set(BrowserHeader, "1")
		r.Header.Set(SetupHeader, second)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Connect-Protocol-Version", "1")
		switch kind {
		case "superseded":
			r.Header.Set(SetupHeader, first)
		case "foreign":
			r.Header.Set(SetupHeader, auth.NewSecret())
		case "remote":
			r.Header.Set("Origin", "https://gul.example.ts.net")
			r.Host = "gul.example.ts.net"
		case "http":
			r.TLS = nil
		}
		w := httptest.NewRecorder()
		f.server.Config.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatal("setup authority accepted", kind, w.Code)
		}
	}
	if configured, err := f.boundary.Accounts.AccountConfigured(t.Context()); err != nil || configured {
		t.Fatal("denied setup created account", err)
	}
}

func TestProductAndAnonymousStateRateLimitsAreBounded(t *testing.T) {
	f := newAuthFixture(t, true)
	token, csrf := loginFixture(t, f, "")
	spy := f.boundary.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, path := range []string{gulv1connect.WorkspacePresentationServiceSetNavigationProcedure, gulv1connect.AuthServiceGetSessionProcedure} {
		for i := 0; i < 121; i++ {
			r := httptest.NewRequest(http.MethodPost, f.server.URL+path, nil)
			r.Header.Set("Origin", f.server.URL)
			r.Header.Set(BrowserHeader, "1")
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Connect-Protocol-Version", "1")
			if path != gulv1connect.AuthServiceGetSessionProcedure {
				r.Header.Set("Cookie", SessionCookie+"="+token)
				r.Header.Set(CSRFHeader, csrf)
			}
			w := httptest.NewRecorder()
			spy.ServeHTTP(w, r)
			want := http.StatusNoContent
			if i == 120 {
				want = http.StatusTooManyRequests
			}
			if w.Code != want {
				t.Fatal("rate limit boundary", path, i, w.Code, want)
			}
		}
	}
}
