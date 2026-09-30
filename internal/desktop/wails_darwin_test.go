package desktop

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func testLeaf(t *testing.T, ip net.IP, expired bool) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	start, end := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	if expired {
		start, end = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Gul local"},
		NotBefore: start, NotAfter: end, IPAddresses: []net.IP{ip},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Gul local"},
		NotBefore: start, NotAfter: end, IPAddresses: []net.IP{ip},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestWailsHostRequiresPinnedLocalHTTPS(t *testing.T) {
	if !probeDelegateForwarding() {
		t.Fatal("native delegate did not forward Wails navigation callback")
	}
	leaf := testLeaf(t, net.IPv4(127, 0, 0, 1), false)
	host := WailsHost{URL: "https://127.0.0.1:43121/", CertificateDER: leaf,
		SetupCredential: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	origin, pin, script, err := host.nativeConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	if origin != "https://127.0.0.1:43121" || pin != sha256.Sum256(leaf) ||
		!strings.Contains(script, "__GUL_NATIVE_BOOTSTRAP__") || !strings.Contains(script, "location.origin ===") {
		t.Fatal("native trust or bootstrap contract changed")
	}
	encoded, _ := json.Marshal(host.SetupCredential)
	if !strings.Contains(script, string(encoded)) {
		t.Fatal("credential is not safely JavaScript encoded")
	}
	if !probeNativeHTTPS(origin, origin+"/", pin, leaf) {
		t.Fatal("native trust rejected pinned loopback leaf")
	}
	for _, candidate := range []string{"https://localhost:43121/", "https://127.0.0.1:43122/", "http://127.0.0.1:43121/", "https://127.0.0.2:43121/"} {
		if probeNativeHTTPS(origin, candidate, pin, leaf) {
			t.Errorf("native trust accepted foreign origin %q", candidate)
		}
	}
	if probeNativeHTTPS(origin, origin+"/", pin, testLeaf(t, net.IPv4(127, 0, 0, 1), false)) {
		t.Fatal("native trust accepted different leaf")
	}
	expired := testLeaf(t, net.IPv4(127, 0, 0, 1), true)
	if probeNativeHTTPS(origin, origin+"/", sha256.Sum256(expired), expired) {
		t.Fatal("native trust accepted expired pinned leaf")
	}
	if err := (WailsHost{}).Run(nil); err == nil {
		t.Fatal("zero-value host started without trust configuration")
	}
	for _, candidate := range []string{
		"http://127.0.0.1:43121/", "https://localhost:43121/", "https://[::1]:43121/",
		"https://127.0.0.2:43121/", "https://127.0.0.1/", "https://127.0.0.1:043121/",
		"https://127.0.0.1:43121/else", "https://127.0.0.1:43121/?token=x",
		"https://user@127.0.0.1:43121/", "https://127.0.0.1:43121/#fragment",
	} {
		host.URL = candidate
		if _, _, _, err := host.nativeConfiguration(); err == nil {
			t.Errorf("accepted URL %q", candidate)
		}
	}
}

func TestWailsHostRejectsInvalidLeafAndSecret(t *testing.T) {
	host := WailsHost{URL: "https://127.0.0.1:43121/", SetupCredential: "secret"}
	for _, leaf := range [][]byte{nil, []byte("not a certificate"),
		testLeaf(t, net.IPv4(127, 0, 0, 2), false),
		testLeaf(t, net.IPv4(127, 0, 0, 1), true)} {
		host.CertificateDER = leaf
		if _, _, _, err := host.nativeConfiguration(); err == nil {
			t.Fatal("accepted invalid TLS leaf")
		}
	}
	host.CertificateDER = testLeaf(t, net.IPv4(127, 0, 0, 1), false)
	host.SetupCredential = "invalid"
	if _, _, _, err := host.nativeConfiguration(); err == nil {
		t.Fatal("accepted invalid setup credential")
	}
	host.SetupCredential = ""
	if _, _, script, err := host.nativeConfiguration(); err != nil || script != "" {
		t.Fatal("configured account requires a script-free login window")
	}
}

func TestWailsHostHasNoEmbeddedAssetServer(t *testing.T) {
	options := wailsOptions()
	if options.Name != "Gul" || options.OnShutdown != nil || options.ShouldQuit == nil ||
		!options.Mac.ApplicationShouldTerminateAfterLastWindowClosed ||
		len(options.Services) != 0 || options.Assets.Handler != nil {
		t.Fatal("desktop shell must use the shared HTTPS host")
	}
}
