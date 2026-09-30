package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// NodeInspection describes the local node reported by `tailscale status --json`.
// FunnelCapabilityPresent reports eligibility, not whether Funnel is enabled.
// This status does not prove the completeness of Serve or Service routes.
type NodeInspection struct {
	DNSName                 string
	FunnelCapabilityPresent bool
}

// InspectTailscaleStatus verifies the connected node and expected tailnet DNS
// name from a bounded, read-only status snapshot. It makes no Service-route
// assertion; ValidateServeStatus inspects the separate Serve configuration.
func InspectTailscaleStatus(status []byte, expectedDNSName string) (NodeInspection, error) {
	if len(status) == 0 || len(status) > 1<<20 {
		return NodeInspection{}, errors.New("Tailscale status is empty or exceeds 1 MiB")
	}
	if expectedDNSName == "" || !strings.HasSuffix(expectedDNSName, ".ts.net") || strings.ToLower(expectedDNSName) != expectedDNSName {
		return NodeInspection{}, errors.New("invalid expected tailnet DNS name")
	}
	var snapshot struct {
		BackendState string `json:"BackendState"`
		Self         *struct {
			DNSName string                     `json:"DNSName"`
			Online  bool                       `json:"Online"`
			CapMap  map[string]json.RawMessage `json:"CapMap"`
		} `json:"Self"`
	}
	decoder := json.NewDecoder(bytes.NewReader(status))
	if err := decoder.Decode(&snapshot); err != nil {
		return NodeInspection{}, fmt.Errorf("decode Tailscale status: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return NodeInspection{}, errors.New("Tailscale status contains trailing JSON")
	}
	if snapshot.BackendState != "Running" || snapshot.Self == nil || !snapshot.Self.Online || strings.TrimSuffix(snapshot.Self.DNSName, ".") != expectedDNSName {
		return NodeInspection{}, errors.New("Tailscale node is offline or has a different DNS name")
	}
	_, capable := snapshot.Self.CapMap["funnel"]
	return NodeInspection{DNSName: expectedDNSName, FunnelCapabilityPresent: capable}, nil
}
