package host

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rootkernel/gul/internal/deployment"
)

// ServeInspector supplies current read-only native Serve evidence. An absent or
// failed inspector never admits a tailnet origin. Tests supply explicit fixtures.
type ServeInspector interface {
	Inspect(context.Context) ([]byte, bool, error)
}

type remoteGate struct {
	mu        sync.Mutex
	hostPort  string
	port      uint16
	inspector ServeInspector
	checked   time.Time
	err       error
}

// Qualified Tailscale Serve preserves the incoming Host, but overwrites these
// proxy headers. Their presence denies local-only routing; it grants no identity
// or authorization. A caller cannot remove the proxy's generated headers.
func proxyMarked(headers http.Header) bool {
	for _, name := range []string{"X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-For", "Forwarded", "Tailscale-Funnel-Request"} {
		if _, exists := headers[http.CanonicalHeaderKey(name)]; exists {
			return true
		}
	}
	return false
}

func (g *remoteGate) verify(ctx context.Context) error {
	if !g.mu.TryLock() {
		return errors.New("tailnet inspection is already running")
	}
	defer g.mu.Unlock()
	if time.Since(g.checked) < time.Second {
		return g.err
	}
	if g.inspector == nil {
		return errors.New("tailnet Serve inspection is unavailable")
	}
	status, noServices, err := g.inspector.Inspect(ctx)
	if err == nil {
		err = deployment.ValidateServeStatus(status, deployment.ServeExpectation{HostPort: g.hostPort, LoopbackPort: g.port, Authenticated: true, LocalCertificateVerified: true, NoServiceRoutesVerified: noServices})
	}
	g.checked = time.Now()
	g.err = err
	return err
}
func (g *remoteGate) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(g.hostPort)
		origin, _ := remoteOrigin(g.hostPort)
		if r.Host == g.hostPort || r.Host == host || r.Header.Get("Origin") == origin {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if g.verify(ctx) != nil {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "tailnet access unavailable; verify Serve and disable Funnel", http.StatusServiceUnavailable)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func remoteOrigin(hostPort string) (string, error) {
	host, port, e := deployment.ParseTailnetEndpoint(hostPort)
	if e != nil {
		return "", e
	}
	if port == "443" {
		return "https://" + host, nil
	}
	return "https://" + hostPort, nil
}
