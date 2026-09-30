package desktop

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/rootkernel/gul/internal/auth"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// WailsHost opens the shared authenticated listener in the Wails WebView.
// CertificateDER is the protected on-disk TLS leaf, never browser data.
type WailsHost struct {
	URL             string
	CertificateDER  []byte
	SetupCredential string
}

const desktopLoadTimeout = 30 * time.Second

func (h WailsHost) Run(onShutdown func()) error {
	return h.RunContext(context.Background(), onShutdown)
}

// RunContext closes the native window on process cancellation. The caller
// retains responsibility for draining only the core it owns.
func (h WailsHost) RunContext(ctx context.Context, onShutdown func()) error {
	return h.run(ctx, onShutdown, installNativeHTTPS, desktopLoadTimeout)
}

func (h WailsHost) run(ctx context.Context, onShutdown func(), install func(unsafe.Pointer, string, [sha256.Size]byte, string, []byte) bool, timeout time.Duration) error {
	origin, pin, script, err := h.nativeConfiguration()
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}
	app := application.New(wailsOptions())
	if onShutdown != nil {
		defer onShutdown()
	}
	// Wails starts its initial load while creating the native window. The
	// inert page lets us install trust and the script before any HTTPS load.
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "Gul", Width: 1100, Height: 760,
		URL: "about:blank", Hidden: true,
	})
	ready, done := make(chan struct{}), make(chan struct{})
	failure := make(chan error, 1)
	fail := func(err error) {
		select {
		case failure <- err:
		default:
		}
		app.Quit()
	}
	// ApplicationStarted is emitted after the native implementation and its
	// event loop exist. Quit before that point would silently do nothing.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go func() {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-done:
				return
			case <-ctx.Done():
				app.Quit()
				return
			case <-timer.C:
				fail(errors.New("desktop HTTPS load timed out; verify the local Gul owner and reopen the window"))
				return
			case <-ready:
			}
			select {
			case <-done:
			case <-ctx.Done():
				app.Quit()
			}
		}()
	})
	var navigation sync.Mutex
	phase := 0
	window.RegisterHook(events.Mac.WebViewDidFinishNavigation, func(*application.WindowEvent) {
		navigation.Lock()
		defer navigation.Unlock()
		switch phase {
		case 0:
			phase = 1
			var ok bool
			application.InvokeSync(func() {
				ok = install(window.NativeWindow(), origin, pin, script, h.CertificateDER)
			})
			if !ok {
				fail(errors.New("desktop HTTPS trust installation failed"))
				return
			}
			window.SetURL(h.URL)
		case 1:
			phase = 2
			close(ready)
			window.Show()
		}
	})
	err = app.Run()
	close(done)
	select {
	case startupErr := <-failure:
		return errors.Join(err, startupErr)
	default:
		return err
	}
}

func (h WailsHost) nativeConfiguration() (string, [sha256.Size]byte, string, error) {
	var empty [sha256.Size]byte
	u, err := url.Parse(h.URL)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() != "127.0.0.1" ||
		u.User != nil || u.Port() == "" || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", empty, "", errors.New("desktop HTTPS URL must be the exact loopback origin")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != u.Port() {
		return "", empty, "", errors.New("desktop HTTPS port is invalid")
	}
	origin := "https://127.0.0.1:" + u.Port()
	if h.URL != origin && h.URL != origin+"/" {
		return "", empty, "", errors.New("desktop HTTPS URL is not canonical")
	}
	cert, err := x509.ParseCertificate(h.CertificateDER)
	if err != nil || cert.IsCA || time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) ||
		cert.VerifyHostname("127.0.0.1") != nil || !hasLoopbackIP(cert.IPAddresses) {
		return "", empty, "", errors.New("desktop HTTPS leaf is invalid")
	}
	if h.SetupCredential != "" && !auth.ValidSecret(h.SetupCredential) {
		return "", empty, "", errors.New("desktop setup credential is invalid")
	}
	var script string
	if h.SetupCredential != "" {
		encodedOrigin, _ := json.Marshal(origin)
		encodedCredential, _ := json.Marshal(h.SetupCredential)
		// The script runs in the main frame only and discloses its field
		// solely to the verified loopback origin.
		script = "if (location.origin === " + string(encodedOrigin) +
			") Object.defineProperty(window, '__GUL_NATIVE_BOOTSTRAP__', {value: {setupCredential: " +
			string(encodedCredential) + "}, configurable: true});"
	}
	return origin, sha256.Sum256(cert.Raw), script, nil
}

func hasLoopbackIP(ips []net.IP) bool {
	for _, ip := range ips {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			return true
		}
	}
	return false
}

func wailsOptions() application.Options {
	return application.Options{
		Name: "Gul",
		// AppKit termination bypasses Go defers. Stop its event loop and let
		// Run return through the caller's owned-core cleanup instead.
		ShouldQuit: func() bool { stopNativeApplication(); return false },
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	}
}
