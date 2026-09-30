// Package deployment validates the read-only Tailscale Serve snapshot and
// renders a user launchd agent. It does not change host configuration.
package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ServeExpectation is supplied by the host after it has verified its own
// authenticated HTTPS listener, pinned local certificate, and absence of
// alternate Tailscale Service routes to Gul. HostPort is the
// exact tailnet DNS name and external HTTPS port (for example, node.ts.net:443).
type ServeExpectation struct {
	HostPort                 string
	LoopbackPort             uint16
	Authenticated            bool
	LocalCertificateVerified bool
	NoServiceRoutesVerified  bool
}

// ParseTailnetEndpoint is the shared host, Serve and launchd endpoint contract.
func ParseTailnetEndpoint(endpoint string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(endpoint)
	if err != nil || !strings.HasSuffix(host, ".ts.net") || host == ".ts.net" || strings.ToLower(host) != host {
		return "", "", errors.New("invalid tailnet HTTPS endpoint")
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil || number == 0 || strconv.FormatUint(number, 10) != port {
		return "", "", errors.New("invalid tailnet HTTPS port")
	}
	return host, port, nil
}

// ValidateServeStatus checks the output of `tailscale serve status --json`.
// It is deliberately pure: the caller obtains the snapshot and must refresh it
// after any Serve configuration change. An error means remote readiness is blocked.
func ValidateServeStatus(status []byte, want ServeExpectation) error {
	if len(status) == 0 || len(status) > 1<<20 {
		return errors.New("Serve status is empty or exceeds 1 MiB")
	}
	_, port, err := ParseTailnetEndpoint(want.HostPort)
	if err != nil || want.LoopbackPort == 0 {
		return errors.New("invalid expected tailnet HTTPS endpoint or loopback port")
	}
	if !want.Authenticated || !want.LocalCertificateVerified || !want.NoServiceRoutesVerified {
		return errors.New("authenticated loopback HTTPS, local certificate, and Service routes must be verified")
	}
	var config serveConfig
	decoder := json.NewDecoder(bytes.NewReader(status))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("decode Serve status: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("Serve status contains trailing JSON")
	}
	if bytes.Equal(bytes.TrimSpace(status), []byte("null")) {
		return errors.New("Serve status is null")
	}
	for _, enabled := range config.AllowFunnel {
		if enabled {
			return errors.New("Funnel is enabled on this node")
		}
	}
	if len(config.Foreground) != 0 || len(config.Services) != 0 {
		return errors.New("foreground or service Serve configuration cannot be qualified")
	}
	tcp := config.TCP[port]
	if tcp == nil || !tcp.HTTPS || tcp.HTTP || tcp.TCPForward != "" || tcp.TerminateTLS != "" || tcp.ProxyProtocol != 0 {
		return errors.New("expected endpoint is not exclusive HTTPS Serve")
	}
	web := config.Web[want.HostPort]
	if web == nil || len(web.Handlers) != 1 || web.Handlers["/"] == nil {
		return errors.New("expected endpoint must have one root proxy handler")
	}
	target := "https+insecure://127.0.0.1:" + strconv.FormatUint(uint64(want.LoopbackPort), 10)
	handler := web.Handlers["/"]
	if handler.Proxy != target && handler.Proxy != target+"/" {
		return errors.New("Serve proxy does not match verified loopback HTTPS target")
	}
	if handler.Path != "" || handler.Text != "" || handler.Redirect != "" || len(handler.AcceptAppCaps) != 0 {
		return errors.New("Serve root handler contains an extra delivery mode")
	}
	for address, other := range config.Web {
		if address == want.HostPort || other == nil {
			continue
		}
		for _, candidate := range other.Handlers {
			if candidate != nil {
				if strings.HasPrefix(candidate.Proxy, "unix:") || proxyUsesPort(candidate.Proxy, want.LoopbackPort) {
					return errors.New("another Serve endpoint forwards to a Unix socket or the Gul listener")
				}
			}
		}
	}
	for key, other := range config.TCP {
		if key != port && other != nil && (strings.HasPrefix(other.TCPForward, "unix:") || proxyUsesPort(other.TCPForward, want.LoopbackPort)) {
			return errors.New("another TCP Serve endpoint forwards to a Unix socket or the Gul listener")
		}
	}
	return nil
}

type serveConfig struct {
	TCP         map[string]*tcpHandler     `json:"TCP"`
	Web         map[string]*webServer      `json:"Web"`
	AllowFunnel map[string]bool            `json:"AllowFunnel"`
	Foreground  map[string]json.RawMessage `json:"Foreground"`
	Services    map[string]json.RawMessage `json:"Services"`
}

type tcpHandler struct {
	HTTPS         bool   `json:"HTTPS"`
	HTTP          bool   `json:"HTTP"`
	TCPForward    string `json:"TCPForward"`
	TerminateTLS  string `json:"TerminateTLS"`
	ProxyProtocol int    `json:"ProxyProtocol"`
}

type webServer struct {
	Handlers map[string]*webHandler `json:"Handlers"`
}

type webHandler struct {
	Proxy         string   `json:"Proxy"`
	Path          string   `json:"Path"`
	Text          string   `json:"Text"`
	Redirect      string   `json:"Redirect"`
	AcceptAppCaps []string `json:"AcceptAppCaps"`
}

func proxyUsesPort(raw string, port uint16) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Host != "" {
		return parsed.Port() == strconv.FormatUint(uint64(port), 10)
	}
	_, value, err := net.SplitHostPort(raw)
	return err == nil && value == strconv.FormatUint(uint64(port), 10)
}
