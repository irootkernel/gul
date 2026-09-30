package host

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const qualifiedVersion = `{"majorMinorPatch":"1.102.4","gitCommit":"3caf7d9e7dcaba589cfc58beda596929733e4fea","long":"1.102.4-t3caf7d9e7-g084ee3b64","daemonLong":"1.102.4-t3caf7d9e7-g084ee3b64"}`

func TestInspectorRequiresCompleteSnapshotVersion(t *testing.T) {
	for _, version := range []string{qualifiedVersion, "null", "{}", qualifiedVersion + " {}", strings.Replace(qualifiedVersion, "1.102.4", "1.100.0", 1), strings.Replace(qualifiedVersion, "3caf7d9e7dcaba589cfc58beda596929733e4fea", "unknown", 1), strings.Replace(qualifiedVersion, `"daemonLong":"1.102.4-t3caf7d9e7-g084ee3b64"`, `"daemonLong":"1.100.0"`, 1)} {
		var calls []string
		read := func(_ context.Context, args ...string) ([]byte, error) {
			command := strings.Join(args, " ")
			calls = append(calls, command)
			switch command {
			case "version --daemon --json":
				return []byte(version), nil
			case "status --json":
				return []byte(`{"BackendState":"Running","Self":{"Online":true,"DNSName":"gul.tail123.ts.net."}}`), nil
			case "serve status --json":
				return serveSnapshot(18443, false), nil
			default:
				return nil, errors.New("unexpected mutation")
			}
		}
		status, proof, err := (TailscaleInspector{HostPort: "gul.tail123.ts.net:443"}).inspect(t.Context(), read)
		if version == qualifiedVersion {
			if err != nil || !proof || len(status) == 0 || strings.Join(calls, ";") != "version --daemon --json;status --json;serve status --json" {
				t.Fatalf("qualified snapshot: %v %v %v", proof, calls, err)
			}
		} else if err == nil || proof || len(status) != 0 || len(calls) != 1 {
			t.Fatalf("unknown source admitted: %v %v %v", proof, calls, err)
		}
	}
}

func TestTailnetDiagnosticBindsVerifiedOwnerAndSnapshot(t *testing.T) {
	c := config(t)
	h := started(t, c)
	for _, test := range []struct {
		name      string
		port      int
		proof     bool
		wantError bool
	}{
		{"matching", c.Port, true, false},
		{"misroute", c.Port + 1, true, true},
		{"unknown snapshot source", c.Port, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			inspector := &fixtureServe{status: serveSnapshot(test.port, false), source: test.proof}
			err := checkTailnet(t.Context(), c.DataDirectory, "gul.tail123.ts.net:443", inspector)
			if (err != nil) != test.wantError || inspector.calls != 1 {
				t.Fatalf("diagnostic: %v, inspections %d", err, inspector.calls)
			}
		})
	}
	if err := h.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	inspector := &fixtureServe{status: serveSnapshot(c.Port, false), source: true}
	if err := checkTailnet(t.Context(), c.DataDirectory, "gul.tail123.ts.net:443", inspector); !errors.Is(err, ErrUnverifiedOwner) || inspector.calls != 0 {
		t.Fatalf("unverified owner reached remote diagnostic: %v", err)
	}
}
