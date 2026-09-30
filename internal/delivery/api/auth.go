package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/auth"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const SessionCookie = "__Host-gul-session"
const CSRFHeader = "X-Gul-CSRF"
const BrowserHeader = "X-Gul-Request"
const SetupHeader = "X-Gul-Setup"

type authRequestKey struct{}
type authRequest struct {
	owner *BrowserBoundary
	token string
	grant *auth.LocalSetupGrant
}

type requestWindow struct {
	start time.Time
	count int
}

func (w *requestWindow) allow(now time.Time, limit int) bool {
	if w.start.IsZero() || !now.Before(w.start.Add(time.Minute)) {
		w.start, w.count = now, 0
	}
	if w.count >= limit {
		return false
	}
	w.count++
	return true
}

// BrowserBoundary is shared by all mounted RPCs and the core authorization
// port. Origins are exact HTTPS origins supplied by verified host composition.
// Forwarded headers, loopback source addresses and browser subjects grant no
// authority. T3 owns the listener and protected native credential delivery.
type BrowserBoundary struct {
	Accounts      *auth.Service
	Sessions      *auth.Sessions
	localOrigin   string
	origins       map[string]bool
	now           func() time.Time
	mu            sync.Mutex
	setupDigest   string
	setupGrant    *auth.LocalSetupGrant
	loginWindow   requestWindow
	productWindow requestWindow
	controlWindow requestWindow
}

func NewBrowserBoundary(accounts *auth.Service, sessions *auth.Sessions, localOrigin string, remoteOrigins ...string) (*BrowserBoundary, error) {
	if accounts == nil || sessions == nil {
		return nil, errors.New("authentication dependencies unavailable")
	}
	b := &BrowserBoundary{Accounts: accounts, Sessions: sessions, localOrigin: localOrigin, origins: make(map[string]bool), now: time.Now}
	for _, origin := range append([]string{localOrigin}, remoteOrigins...) {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != origin {
			return nil, errors.New("invalid authenticated browser origin")
		}
		if origin == localOrigin {
			ip := net.ParseIP(u.Hostname())
			if ip == nil || !ip.IsLoopback() {
				return nil, errors.New("native setup origin must use a loopback IP")
			}
		}
		b.origins[origin] = true
	}
	return b, nil
}

// GrantNativeSetup is a host API, never an RPC. The return value must be
// delivered only through T3's protected native bootstrap, never a URL or log.
// A new grant supersedes the previous transport credential.
func (b *BrowserBoundary) GrantNativeSetup(ctx context.Context) (string, error) {
	grant, err := b.Accounts.GrantLocalSetup(ctx)
	if err != nil {
		return "", err
	}
	secret := auth.NewSecret()
	b.mu.Lock()
	b.setupDigest, b.setupGrant = auth.SecretDigest(secret), grant
	b.mu.Unlock()
	return secret, nil
}

func cookieToken(r *http.Request) (string, error) {
	var token string
	count := 0
	for _, cookie := range r.Cookies() {
		if cookie.Name == SessionCookie {
			token = cookie.Value
			count++
		}
	}
	if count > 1 || (count == 1 && !auth.ValidSecret(token)) {
		return "", auth.ErrInvalidSession
	}
	return token, nil
}

func (b *BrowserBoundary) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		deny := func(err error) { _ = connect.NewErrorWriter().Write(w, r, err) }
		origins := r.Header.Values("Origin")
		if r.TLS == nil || r.Method != http.MethodPost || r.URL.RawQuery != "" || len(origins) != 1 || !b.origins[origins[0]] || r.Header.Get(BrowserHeader) != "1" {
			deny(accessError(connect.CodePermissionDenied, "browser request denied"))
			return
		}
		u, _ := url.Parse(origins[0])
		if r.Host != u.Host {
			deny(accessError(connect.CodePermissionDenied, "browser request denied"))
			return
		}
		token, err := cookieToken(r)
		if err != nil {
			deny(accessError(connect.CodeUnauthenticated, "session unavailable"))
			return
		}
		state := &authRequest{owner: b, token: token}
		ctx := context.WithValue(r.Context(), authRequestKey{}, state)
		switch r.URL.Path {
		case gulv1connect.AuthServiceLoginProcedure, gulv1connect.AuthServiceFirstRunSetupProcedure:
			b.mu.Lock()
			allowed := b.loginWindow.allow(b.now(), 5)
			b.mu.Unlock()
			if !allowed {
				deny(connect.NewError(connect.CodeResourceExhausted, errors.New("authentication rate limit; retry later")))
				return
			}
			if r.URL.Path == gulv1connect.AuthServiceFirstRunSetupProcedure {
				secret := r.Header.Get(SetupHeader)
				b.mu.Lock()
				if origins[0] == b.localOrigin && auth.ValidSecret(secret) && b.setupGrant != nil && subtle.ConstantTimeCompare([]byte(auth.SecretDigest(secret)), []byte(b.setupDigest)) == 1 {
					state.grant = b.setupGrant
				}
				b.mu.Unlock()
				r.Header.Del(SetupHeader)
				if state.grant == nil {
					deny(authError(auth.ErrSetupDenied))
					return
				}
			}
		case gulv1connect.AuthServiceGetSessionProcedure:
			// Same-origin custom-header POST protects even the anonymous state read.
			b.mu.Lock()
			allowed := b.controlWindow.allow(b.now(), 120)
			b.mu.Unlock()
			if !allowed {
				deny(connect.NewError(connect.CodeResourceExhausted, errors.New("request rate limit; retry later")))
				return
			}
		case gulv1connect.AuthServiceLogoutProcedure:
			b.mu.Lock()
			allowed := b.controlWindow.allow(b.now(), 120)
			b.mu.Unlock()
			if !allowed {
				deny(connect.NewError(connect.CodeResourceExhausted, errors.New("request rate limit; retry later")))
				return
			}
			if token != "" && !matchesCSRF(r, token) {
				deny(accessError(connect.CodePermissionDenied, "browser request denied"))
				return
			}
		default:
			if token == "" {
				deny(accessError(connect.CodeUnauthenticated, "authentication required"))
				return
			}
			if !matchesCSRF(r, token) {
				deny(accessError(connect.CodePermissionDenied, "browser request denied"))
				return
			}
			b.mu.Lock()
			allowed := b.productWindow.allow(b.now(), 120)
			b.mu.Unlock()
			if !allowed {
				deny(connect.NewError(connect.CodeResourceExhausted, errors.New("request rate limit; retry later")))
				return
			}
			stream := r.URL.Path == gulv1connect.ClientEventServiceWatchClientEventsProcedure || r.URL.Path == gulv1connect.FileServiceWatchFileChangesProcedure
			bound, release, err := b.Sessions.Bind(ctx, token, stream)
			if err != nil {
				deny(authError(err))
				return
			}
			defer release()
			ctx = bound
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func matchesCSRF(r *http.Request, token string) bool {
	values := r.Header.Values(CSRFHeader)
	return len(values) == 1 && subtle.ConstantTimeCompare([]byte(values[0]), []byte(auth.CSRFToken(token))) == 1
}

func (b *BrowserBoundary) Principal(ctx context.Context) (app.Principal, error) {
	state, ok := ctx.Value(authRequestKey{}).(*authRequest)
	if !ok || state.owner != b {
		return app.Principal{}, auth.ErrInvalidSession
	}
	session, err := b.Sessions.Current(ctx)
	if err != nil {
		return app.Principal{}, err
	}
	return app.Principal{Subject: session.SubjectID}, nil
}

func (b *BrowserBoundary) Authorize(ctx context.Context, principal app.Principal) error {
	current, err := b.Principal(ctx)
	if err != nil || current.Subject != principal.Subject {
		return app.ErrAccessDenied
	}
	return nil
}

type AuthHandler struct{ Boundary *BrowserBoundary }

func (h *AuthHandler) state(ctx context.Context) (*authRequest, error) {
	state, ok := ctx.Value(authRequestKey{}).(*authRequest)
	if h == nil || h.Boundary == nil || !ok || state.owner != h.Boundary {
		return nil, authError(auth.ErrInvalidSession)
	}
	return state, nil
}

func (h *AuthHandler) FirstRunSetup(ctx context.Context, r *connect.Request[gulv1.FirstRunSetupRequest]) (*connect.Response[gulv1.FirstRunSetupResponse], error) {
	defer clear(r.Msg.Password)
	defer func() { r.Msg.Password = nil }()
	state, err := h.state(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.Boundary.Accounts.FirstRunSetup(ctx, state.grant, r.Msg.Password); err != nil {
		return nil, authError(err)
	}
	h.Boundary.mu.Lock()
	if h.Boundary.setupGrant == state.grant {
		h.Boundary.setupGrant, h.Boundary.setupDigest = nil, ""
	}
	h.Boundary.mu.Unlock()
	return connect.NewResponse(&gulv1.FirstRunSetupResponse{Session: &gulv1.BrowserSession{AccountConfigured: true}}), nil
}

func (h *AuthHandler) Login(ctx context.Context, r *connect.Request[gulv1.LoginRequest]) (*connect.Response[gulv1.LoginResponse], error) {
	defer clear(r.Msg.Password)
	defer func() { r.Msg.Password = nil }()
	state, err := h.state(ctx)
	if err != nil {
		return nil, err
	}
	subject, err := h.Boundary.Accounts.VerifyPassword(ctx, r.Msg.Password)
	if err != nil {
		return nil, authError(err)
	}
	if state.token != "" {
		if err := h.Boundary.Sessions.Revoke(ctx, state.token); err != nil {
			return nil, authError(err)
		}
	}
	credentials, err := h.Boundary.Sessions.Issue(ctx, subject)
	if err != nil {
		return nil, authError(err)
	}
	response := connect.NewResponse(&gulv1.LoginResponse{Session: &gulv1.BrowserSession{AccountConfigured: true, Authenticated: true, CsrfToken: credentials.CSRF, ExpiresAt: timestamppb.New(credentials.Session.ExpiresAt)}})
	response.Header().Set("Set-Cookie", (&http.Cookie{Name: SessionCookie, Value: credentials.Token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionLifetime.Seconds()), Expires: credentials.Session.ExpiresAt}).String())
	return response, nil
}

func (h *AuthHandler) Logout(ctx context.Context, _ *connect.Request[gulv1.LogoutRequest]) (*connect.Response[gulv1.LogoutResponse], error) {
	state, err := h.state(ctx)
	if err != nil {
		return nil, err
	}
	if state.token != "" {
		if err := h.Boundary.Sessions.Revoke(ctx, state.token); err != nil {
			return nil, authError(err)
		}
	}
	response := connect.NewResponse(&gulv1.LogoutResponse{})
	response.Header().Set("Set-Cookie", (&http.Cookie{Name: SessionCookie, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)}).String())
	return response, nil
}

func (h *AuthHandler) GetSession(ctx context.Context, _ *connect.Request[gulv1.GetSessionRequest]) (*connect.Response[gulv1.GetSessionResponse], error) {
	state, err := h.state(ctx)
	if err != nil {
		return nil, err
	}
	configured, err := h.Boundary.Accounts.AccountConfigured(ctx)
	if err != nil {
		return nil, authError(err)
	}
	result := &gulv1.BrowserSession{AccountConfigured: configured}
	if state.token != "" {
		session, err := h.Boundary.Sessions.Inspect(ctx, state.token)
		if err != nil && !errors.Is(err, auth.ErrInvalidSession) {
			return nil, authError(err)
		}
		if err == nil {
			result.Authenticated, result.CsrfToken, result.ExpiresAt = true, auth.CSRFToken(state.token), timestamppb.New(session.ExpiresAt)
		}
	}
	return connect.NewResponse(&gulv1.GetSessionResponse{Session: result}), nil
}

func authError(err error) error {
	code, message := connect.CodeUnavailable, "authentication unavailable"
	switch {
	case errors.Is(err, auth.ErrSetupDenied):
		code, message = connect.CodePermissionDenied, "local setup denied"
	case errors.Is(err, auth.ErrAlreadyConfigured):
		code, message = connect.CodeAlreadyExists, "password already configured"
	case errors.Is(err, auth.ErrInvalidPassword):
		code, message = connect.CodeInvalidArgument, "invalid password"
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrInvalidSession):
		code, message = connect.CodeUnauthenticated, "authentication required"
	case errors.Is(err, context.Canceled):
		code, message = connect.CodeCanceled, "authentication cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		code, message = connect.CodeDeadlineExceeded, "authentication timed out"
	}
	return accessError(code, message)
}

var _ gulv1connect.AuthServiceHandler = (*AuthHandler)(nil)
