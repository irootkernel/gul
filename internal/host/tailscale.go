package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	neturl "net/url"
	"os/exec"
	"strconv"
	"time"

	"github.com/rootkernel/gul/internal/deployment"
)

// TailscaleInspector invokes only read-only status commands. Serve status JSON
// is the complete native ServeConfig; its Services and Foreground fields are
// checked by the closed deployment validator, not inferred from capabilities.
type TailscaleInspector struct{ HostPort string }
type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("Tailscale status exceeds its bound")
	}
	return b.Buffer.Write(p)
}
func nativeStatus(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tailscale", args...)
	output := &boundedOutput{limit: 1 << 20}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return nil, errors.New("Tailscale read-only inspection is unavailable")
	}
	return output.Bytes(), nil
}
func (i TailscaleInspector) Inspect(ctx context.Context) ([]byte, bool, error) {
	return i.inspect(ctx, nativeStatus)
}
func (i TailscaleInspector) inspect(ctx context.Context, read func(context.Context, ...string) ([]byte, error)) ([]byte, bool, error) {
	host, _, err := net.SplitHostPort(i.HostPort)
	if err != nil {
		return nil, false, err
	}
	version, err := read(ctx, "version", "--daemon", "--json")
	if err != nil || !completeServeVersion(version) {
		return nil, false, errors.New("Tailscale CLI and daemon Serve snapshot versions are unverified; remote access blocked")
	}
	node, err := read(ctx, "status", "--json")
	if err != nil {
		return nil, false, err
	}
	if _, err = deployment.InspectTailscaleStatus(node, host); err != nil {
		return nil, false, errors.New("Tailscale node does not match the configured remote endpoint")
	}
	status, err := read(ctx, "serve", "status", "--json")
	if err != nil {
		return nil, false, err
	}
	return status, true, nil
}

// The inspected v1.102.4 source marshals the complete ServeConfig. Unknown
// versions cannot supply the Service-route completeness proof until qualified.
func completeServeVersion(data []byte) bool {
	if len(data) == 0 || len(data) > 1<<20 {
		return false
	}
	var version struct {
		MajorMinorPatch string `json:"majorMinorPatch"`
		GitCommit       string `json:"gitCommit"`
		Long            string `json:"long"`
		DaemonLong      string `json:"daemonLong"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if d.Decode(&version) != nil || d.Decode(new(any)) != io.EOF {
		return false
	}
	return version.MajorMinorPatch == "1.102.4" && version.GitCommit == "3caf7d9e7dcaba589cfc58beda596929733e4fea" && version.Long != "" && version.DaemonLong == version.Long
}

// CheckTailnet refreshes diagnostics independently of the request admission
// snapshot. It neither mints native setup authority nor changes Serve.
func CheckTailnet(ctx context.Context, root, hostPort string) error {
	return checkTailnet(ctx, root, hostPort, TailscaleInspector{HostPort: hostPort})
}
func checkTailnet(ctx context.Context, root, hostPort string, inspector ServeInspector) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	view, err := Verify(ctx, root)
	if err != nil {
		return err
	}
	u, err := neturl.Parse(view.Origin)
	if err != nil {
		return err
	}
	port, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil {
		return err
	}
	snapshot, source, err := inspector.Inspect(ctx)
	if err != nil {
		return err
	}
	return deployment.ValidateServeStatus(snapshot, deployment.ServeExpectation{HostPort: hostPort, LoopbackPort: uint16(port), Authenticated: true, LocalCertificateVerified: true, NoServiceRoutesVerified: source})
}
