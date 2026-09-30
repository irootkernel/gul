package deployment

import "testing"

func TestInspectTailscaleStatus(t *testing.T) {
	const node = `{"BackendState":"Running","Self":{"DNSName":"gul.tail123.ts.net.","Online":true,"CapMap":{"funnel":[],"https":[]}},"Peer":{"ignored":{}}}`
	got, err := InspectTailscaleStatus([]byte(node), "gul.tail123.ts.net")
	if err != nil || got.DNSName != "gul.tail123.ts.net" || !got.FunnelCapabilityPresent {
		t.Fatalf("connected node: %+v, %v", got, err)
	}
	for _, test := range []string{
		`{"BackendState":"Stopped","Self":{"DNSName":"gul.tail123.ts.net.","Online":true}}`,
		`{"BackendState":"Running","Self":{"DNSName":"other.tail123.ts.net.","Online":true}}`,
		`{"BackendState":"Running","Self":{"DNSName":"gul.tail123.ts.net.","Online":false}}`,
		`null`,
		`{}`,
		node + `{}`,
	} {
		if _, err := InspectTailscaleStatus([]byte(test), "gul.tail123.ts.net"); err == nil {
			t.Fatalf("invalid node status accepted: %q", test)
		}
	}
}
