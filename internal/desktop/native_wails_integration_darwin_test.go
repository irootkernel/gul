package desktop

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/rootkernel/gul/internal/auth"
	acceptancenative "github.com/rootkernel/gul/test/acceptance/native"
)

func nativeTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Gul isolated native test"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

const nativeHelperMarker = "GUL_NATIVE_WAILS_HELPER"

// TestMain keeps AppKit's event loop on the process main thread. The parent
// test owns the isolated HTTPS fixture and terminates this child through stdin.
func TestMain(m *testing.M) {
	if os.Getenv(nativeHelperMarker) != "1" {
		os.Exit(m.Run())
	}
	der, err := base64.StdEncoding.DecodeString(os.Getenv("GUL_NATIVE_TEST_DER"))
	if err != nil {
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		_, _ = io.CopyN(io.Discard, os.Stdin, 1)
		cancel()
	}()
	view := WailsHost{URL: os.Getenv("GUL_NATIVE_TEST_URL"), CertificateDER: der,
		SetupCredential: os.Getenv("GUL_NATIVE_TEST_CREDENTIAL")}
	installer := installNativeHTTPS
	if os.Getenv("GUL_NATIVE_ASSEMBLED_PROBE") == "1" {
		installer = func(window unsafe.Pointer, origin string, pin [sha256.Size]byte, script string, der []byte) bool {
			if !installNativeHTTPS(window, origin, pin, script, der) {
				return false
			}
			acceptancenative.Probe(window, nativeAcceptanceScript)
			return true
		}
	}
	timeout := desktopLoadTimeout
	if os.Getenv("GUL_NATIVE_TEST_FAILURE") == "install" {
		installer = func(unsafe.Pointer, string, [sha256.Size]byte, string, []byte) bool { return false }
	}
	if os.Getenv("GUL_NATIVE_TEST_FAILURE") == "load" {
		timeout = time.Second // The isolated fixture controls the production deadline path.
	}
	if err := view.run(ctx, func() {}, installer, timeout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestNativeWailsHTTPSWindow(t *testing.T) {
	if os.Getenv("GUL_RUN_NATIVE_WAILS_TEST") != "1" {
		t.Skip("set GUL_RUN_NATIVE_WAILS_TEST=1 for the isolated AppKit/WebKit check")
	}
	requests := make(chan string, 4)
	var popupRequests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/popup":
			popupRequests.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/":
			requests <- "get"
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><script>
window.open('/popup', '_blank');
const value = window.__GUL_NATIVE_BOOTSTRAP__?.setupCredential;
fetch('/probe', {method:'POST', body: typeof value === 'string' && value.length === 43 ? 'present' : 'missing'}).then(() => { location.href = '/next'; });
</script>`)
		case r.Method == http.MethodGet && r.URL.Path == "/next":
			requests <- "next-get"
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><script>
fetch('/next-probe', {method:'POST', body: window.__GUL_NATIVE_BOOTSTRAP__?.setupCredential?.length === 43 ? 'next-present' : 'next-missing'});
</script>`)
		case r.Method == http.MethodPost && r.URL.Path == "/probe":
			fallthrough
		case r.Method == http.MethodPost && r.URL.Path == "/next-probe":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 16))
			requests <- string(body)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{nativeTestCertificate(t)}}
	server.StartTLS()
	defer server.Close()
	leaf, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil || leaf.VerifyHostname("127.0.0.1") != nil {
		t.Fatal("HTTPS fixture has no loopback leaf")
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), nativeHelperMarker+"=1", "GUL_NATIVE_TEST_URL="+server.URL,
		"GUL_NATIVE_TEST_DER="+base64.StdEncoding.EncodeToString(leaf.Raw),
		"GUL_NATIVE_TEST_CREDENTIAL="+auth.NewSecret(), "HOME="+root,
		"CFFIXED_USER_HOME="+root, "TMPDIR="+root)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	finished := false
	defer func() {
		_ = stdin.Close()
		if !finished {
			_ = cmd.Process.Kill()
			<-wait
		}
	}()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	seenGet, seenProbe, seenNextGet, seenNextProbe := false, false, false, false
	for !seenGet || !seenProbe || !seenNextGet || !seenNextProbe {
		select {
		case err := <-wait:
			finished = true
			t.Fatalf("native WebKit child exited before HTTPS probe: %v: %s", err, output.String())
		case event := <-requests:
			switch event {
			case "get":
				seenGet = true
			case "present":
				seenProbe = true
			case "next-get":
				seenNextGet = true
			case "next-present":
				seenNextProbe = true
			default:
				t.Fatalf("WebKit bootstrap probe failed: %q", event)
			}
		case <-deadline.C:
			_ = cmd.Process.Kill()
			<-wait
			finished = true
			t.Fatalf("native WebKit navigation timed out (initial GET %t, probe %t, next GET %t, next probe %t): %s",
				seenGet, seenProbe, seenNextGet, seenNextProbe, output.String())
		}
	}
	if popupRequests.Load() != 0 {
		t.Fatal("native popup escaped the single-window boundary")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-wait:
		finished = true
		if err != nil {
			t.Fatal(fmt.Errorf("native WebKit child: %w: %s", err, output.String()))
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-wait
		finished = true
		t.Fatal("native WebKit window did not close")
	}
}

func TestNativeWailsStartupFailureExitsWithoutUntrustedLoad(t *testing.T) {
	if os.Getenv("GUL_RUN_NATIVE_WAILS_TEST") != "1" {
		t.Skip("set GUL_RUN_NATIVE_WAILS_TEST=1 for the isolated AppKit/WebKit check")
	}
	for _, failure := range []string{"install", "load"} {
		t.Run(failure, func(t *testing.T) {
			requests := make(chan struct{}, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requests <- struct{}{}:
				default:
				}
				<-r.Context().Done() // HTTPS starts but never finishes navigation.
			}))
			server.TLS = &tls.Config{Certificates: []tls.Certificate{nativeTestCertificate(t)}}
			server.StartTLS()
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			root := t.TempDir()
			cmd := exec.CommandContext(ctx, os.Args[0])
			cmd.Env = append(os.Environ(), nativeHelperMarker+"=1", "GUL_NATIVE_TEST_URL="+server.URL,
				"GUL_NATIVE_TEST_DER="+base64.StdEncoding.EncodeToString(server.TLS.Certificates[0].Certificate[0]),
				"GUL_NATIVE_TEST_FAILURE="+failure, "HOME="+root, "CFFIXED_USER_HOME="+root, "TMPDIR="+root)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil || err == nil {
				t.Fatalf("native failure did not exit: %v: %s", err, output)
			}
			expected := "trust installation failed"
			if failure == "load" {
				expected = "HTTPS load timed out"
			}
			if !strings.Contains(string(output), expected) {
				t.Fatalf("missing actionable failure: %s", output)
			}
			select {
			case <-requests:
				if failure == "install" {
					t.Fatal("HTTPS loaded before trust was installed")
				}
			default:
				if failure == "load" {
					t.Fatal("deadline fixture never attempted HTTPS navigation")
				}
			}
		})
	}
}
