package gateway

import (
	"bytes"
	"encoding/json"
	"sync"
)

// The public serve command emits one bounded readiness envelope. This writer
// retains only its closed failure code, never raw stdout or diagnostic prose.
// Semantic readiness still requires the socket checks and gRPC handshake.
type readiness struct {
	mu      sync.Mutex
	buffer  []byte
	settled bool
	code    string
}

func (r *readiness) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settled {
		return len(data), nil
	}
	if len(r.buffer)+len(data) > 65536 {
		r.settled = true
		r.buffer = nil
		return len(data), nil
	}
	r.buffer = append(r.buffer, data...)
	if index := bytes.IndexByte(r.buffer, '\n'); index >= 0 {
		var envelope struct {
			Command string `json:"command"`
			Error   struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(r.buffer[:index], &envelope) == nil && envelope.Command == "serve" {
			switch envelope.Error.Code {
			case "RPC_SERVER_ALREADY_RUNNING", "RPC_SOCKET_UNSAFE", "PROTOCOL_INCOMPATIBLE":
				r.code = envelope.Error.Code
			}
		}
		r.settled = true
		r.buffer = nil
	}
	return len(data), nil
}
func (r *readiness) failure() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.code {
	case "RPC_SERVER_ALREADY_RUNNING":
		return ErrAlreadyRunning
	case "RPC_SOCKET_UNSAFE":
		return ErrSocket
	case "PROTOCOL_INCOMPATIBLE":
		return ErrIdentity
	}
	return ErrUnavailable
}
