package deployment

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestValidateServeStatus(t *testing.T) {
	want := ServeExpectation{
		HostPort: "gul.tail123.ts.net:443", LoopbackPort: 18443,
		Authenticated: true, LocalCertificateVerified: true, NoServiceRoutesVerified: true,
	}
	valid, err := os.ReadFile("testdata/serve-valid.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateServeStatus(valid, want); err != nil {
		t.Fatalf("valid status: %v", err)
	}
	for _, name := range []string{"serve-funnel.json", "serve-uds.json", "serve-misroute.json"} {
		t.Run(name, func(t *testing.T) {
			status, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(status) {
				t.Fatal("negative deployment fixture must reach policy validation")
			}
			if err := ValidateServeStatus(status, want); err == nil {
				t.Fatal("unsafe Serve status accepted")
			}
		})
	}
	for _, test := range []struct{ name, before, after string }{
		{"HTTP ingress", `"HTTPS": true`, `"HTTP": true`},
		{"TCP forward", `"HTTPS": true`, `"TCPForward": "unix:/private/gul.sock"`},
		{"extra mount", `"/": {"Proxy": "https+insecure://127.0.0.1:18443"}`, `"/": {"Proxy": "https+insecure://127.0.0.1:18443"}, "/admin": {"Text": "open"}`},
		{"alternate proxy", `http://127.0.0.1:8080`, `http://127.0.0.1:18443`},
		{"alternate zero-prefixed proxy", `http://127.0.0.1:8080`, `http://127.0.0.1:018443`},
		{"alternate zero-prefixed HTTPS proxy", `http://127.0.0.1:8080`, `https+insecure://127.0.0.1:00018443`},
		{"other TCP forward", `"8443": {"HTTPS": true}`, `"8443": {"TCPForward": "127.0.0.1:18443"}`},
		{"other zero-prefixed TCP forward", `"8443": {"HTTPS": true}`, `"8443": {"TCPForward": "127.0.0.1:018443"}`},
		{"other Unix socket", `http://127.0.0.1:8080`, `unix:/private/dolgorae.sock`},
		{"other TCP Unix socket", `"8443": {"HTTPS": true}`, `"8443": {"TCPForward": "unix:/private/dolgorae.sock"}`},
		{"foreground", `"TCP":`, `"Foreground": {"session": {}}, "TCP":`},
		{"services", `"TCP":`, `"Services": {"svc:gul": {}}, "TCP":`},
		{"unknown field", `"TCP":`, `"Unknown": true, "TCP":`},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := strings.Replace(string(valid), test.before, test.after, 1)
			if err := ValidateServeStatus([]byte(status), want); err == nil {
				t.Fatal("unsafe Serve status accepted")
			}
		})
	}
	for _, status := range [][]byte{nil, []byte(`null`), []byte(`{}`), append(valid, []byte(` {}`)...)} {
		if err := ValidateServeStatus(status, want); err == nil {
			t.Fatalf("invalid status accepted: %q", status)
		}
	}
	withoutProof := want
	withoutProof.Authenticated = false
	if err := ValidateServeStatus(valid, withoutProof); err == nil {
		t.Fatal("unauthenticated listener accepted")
	}
	withoutProof = want
	withoutProof.LocalCertificateVerified = false
	if err := ValidateServeStatus(valid, withoutProof); err == nil {
		t.Fatal("unverified certificate accepted")
	}
	withoutProof = want
	withoutProof.NoServiceRoutesVerified = false
	if err := ValidateServeStatus(valid, withoutProof); err == nil {
		t.Fatal("unverified service routes accepted")
	}
}
