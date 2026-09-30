package host

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixtureServe struct {
	mu     sync.Mutex
	status []byte
	source bool
	calls  int
}

func (f *fixtureServe) Inspect(context.Context) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return append([]byte(nil), f.status...), f.source, nil
}
func serveSnapshot(port int, funnel bool) []byte {
	return []byte(fmt.Sprintf(`{"TCP":{"443":{"HTTPS":true}},"Web":{"gul.tail123.ts.net:443":{"Handlers":{"/":{"Proxy":"https+insecure://127.0.0.1:%d"}}}},"AllowFunnel":{"gul.tail123.ts.net:443":%t}}`, port, funnel))
}
func TestRemoteAdmissionUsesCurrentServeAndExactHost(t *testing.T) {
	c := config(t)
	c.TailnetHost = "gul.tail123.ts.net:443"
	f := &fixtureServe{status: serveSnapshot(c.Port, false), source: true}
	c.ServeInspector = f
	h := started(t, c)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, h.Origin()+"/", nil)
	req.Host = "gul.tail123.ts.net"
	response, e := client(t, h).Do(req)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("verified remote %d", response.StatusCode)
	}
	f.mu.Lock()
	f.status = serveSnapshot(c.Port, true)
	f.mu.Unlock()
	time.Sleep(time.Second + 10*time.Millisecond)
	response, e = client(t, h).Do(req)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Funnel accepted %d", response.StatusCode)
	}
	local, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, h.Origin()+"/", nil)
	response, e = client(t, h).Do(local)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("remote blocker prevented local %d", response.StatusCode)
	}
	req.Host = "forged.example"
	response, e = client(t, h).Do(req)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("foreign Host accepted")
	}
}
func TestUnverifiedServeSourceDoesNotAdmitRemote(t *testing.T) {
	c := config(t)
	c.TailnetHost = "gul.tail123.ts.net:443"
	c.ServeInspector = &fixtureServe{status: serveSnapshot(c.Port, false), source: false}
	h := started(t, c)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, h.Origin()+"/", nil)
	req.Host = "gul.tail123.ts.net"
	r, e := client(t, h).Do(req)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("unverified config admitted")
	}
}

func TestRemoteAdmissionPreservesNonDefaultHTTPSPort(t *testing.T) {
	c := config(t)
	c.TailnetHost = "gul.tail123.ts.net:8443"
	c.ServeInspector = &fixtureServe{status: []byte(strings.ReplaceAll(strings.ReplaceAll(string(serveSnapshot(c.Port, false)), `"443"`, `"8443"`), ".ts.net:443", ".ts.net:8443")), source: true}
	h := started(t, c)
	r, _ := http.NewRequestWithContext(t.Context(), "GET", h.Origin()+"/", nil)
	r.Host = c.TailnetHost
	response, err := client(t, h).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("non-default HTTPS port rejected: %d", response.StatusCode)
	}
	origin, err := remoteOrigin(c.TailnetHost)
	if err != nil || origin != "https://"+c.TailnetHost {
		t.Fatalf("non-default origin changed: %q %v", origin, err)
	}
}

// This proxy uses the qualified Serve transport contract: preserve inbound
// Host and overwrite forwarding headers, including on a Funnel request.
func TestProxyCannotClaimLocalOnlyHost(t *testing.T) {
	for _, remoteSelected := range []bool{false, true} {
		t.Run(fmt.Sprint(remoteSelected), func(t *testing.T) {
			c := config(t)
			if remoteSelected {
				c.TailnetHost = "gul.tail123.ts.net:443"
				c.ServeInspector = &fixtureServe{status: serveSnapshot(c.Port, true), source: true}
			}
			h := started(t, c)
			backend, _ := url.Parse(h.Origin())
			proxy := httptest.NewTLSServer(&httputil.ReverseProxy{
				Transport: client(t, h).Transport,
				Rewrite: func(r *httputil.ProxyRequest) {
					r.SetURL(backend)
					r.Out.Host = r.In.Host
					r.Out.Header.Set("X-Forwarded-Host", r.In.Host)
					r.Out.Header.Set("X-Forwarded-Proto", "https")
					r.Out.Header.Set("X-Forwarded-For", "100.64.0.9")
					r.Out.Header.Set("Tailscale-Funnel-Request", "?1")
				},
			})
			defer proxy.Close()
			proxyClient := proxy.Client()
			proxyClient.Transport.(*http.Transport).TLSClientConfig.ServerName = "gul.tail123.ts.net"
			// Only this isolated proxy's front certificate is trusted here.
			proxyClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
			for _, path := range []string{"/", "/gul.v1.AuthService/GetSession", "/_gul/native-bootstrap"} {
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy.URL+path, strings.NewReader(`{}`))
				req.Host = backend.Host
				req.Header.Set("Origin", h.Origin())
				req.Header.Set("X-Forwarded-Host", "") // Cannot erase the proxy's marker.
				response, err := proxyClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != http.StatusForbidden {
					t.Fatalf("forwarded local claim reached %s: %d", path, response.StatusCode)
				}
			}
			response, err := client(t, h).Get(h.Origin())
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatal("deployment blocker prevented direct local delivery")
			}
		})
	}
}

func TestProxyMarkersOnlyDenyLocalRouting(t *testing.T) {
	for _, name := range []string{"X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-For", "Forwarded", "Tailscale-Funnel-Request"} {
		header := make(http.Header)
		header.Set(name, "")
		if !proxyMarked(header) {
			t.Fatalf("empty marker %s accepted", name)
		}
	}
	if proxyMarked(http.Header{"Origin": {"https://127.0.0.1:17423"}}) {
		t.Fatal("ordinary browser mistaken for proxy")
	}
}
