package port

import (
	"errors"
	"sort"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
)

// ProviderError is safe to translate into Gul's browser DomainError. It never
// carries provider messages, identifiers, or raw gRPC details across the port.
type ProviderError struct {
	Code   string
	Action string
}

func blocked() ProviderError {
	return ProviderError{Code: "PROVIDER_BLOCKED", Action: "OPERATOR_REPAIR"}
}

// MapProviderError uses status only for transport/deadline failures. Semantic
// errors require the accepted typed detail; status text is never inspected.
func MapProviderError(err error) ProviderError {
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return ProviderError{Code: "TRANSPORT_UNAVAILABLE", Action: "REFRESH_SNAPSHOT"}
	}
	var detail *publicv1.DolgoraeErrorDetail
	for _, item := range connectErr.Details() {
		value, decodeErr := item.Value()
		if decodeErr != nil {
			return blocked()
		}
		if typed, ok := value.(*publicv1.DolgoraeErrorDetail); ok {
			if detail != nil {
				return blocked()
			}
			detail = typed
		}
	}
	if detail == nil {
		switch connectErr.Code() {
		case connect.CodeUnavailable:
			return ProviderError{Code: "TRANSPORT_UNAVAILABLE", Action: "REFRESH_SNAPSHOT"}
		case connect.CodeDeadlineExceeded:
			return ProviderError{Code: "DEADLINE_EXCEEDED", Action: "REFRESH_SNAPSHOT"}
		default:
			return ProviderError{Code: "PROTOCOL_INCOMPATIBLE", Action: "OPERATOR_REPAIR"}
		}
	}
	if detail.GetDetailVersion() != 1 {
		return ProviderError{Code: "PROTOCOL_INCOMPATIBLE", Action: "OPERATOR_REPAIR"}
	}
	if detail.GetDolgoraeErrorCode() == "" ||
		detail.GetRetryClassification() == publicv1.RetryClassification_RETRY_CLASSIFICATION_UNSPECIFIED ||
		detail.GetRecoveryClassification() == publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_UNSPECIFIED {
		return blocked()
	}
	if _, ok := publicv1.RetryClassification_name[int32(detail.GetRetryClassification())]; !ok {
		return blocked()
	}
	if _, ok := publicv1.RecoveryClassification_name[int32(detail.GetRecoveryClassification())]; !ok {
		return blocked()
	}
	if _, ok := acceptedErrorCodes[detail.GetDolgoraeErrorCode()]; !ok {
		return blocked()
	}
	action, ok := mapRequiredAction(detail.GetAction())
	if !ok {
		return blocked()
	}
	return ProviderError{Code: mapErrorCode(detail.GetDolgoraeErrorCode()), Action: action}
}

func mapRequiredAction(action publicv1.RequiredClientAction) (string, bool) {
	switch action {
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_NONE, publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_ABORT:
		return "ABORT", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_REQUEST:
		return "FIX_REQUEST", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_CAPABILITIES:
		return "REFRESH_CAPABILITIES", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT:
		return "REFRESH_SNAPSHOT", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_VERIFY_CONTROLLER:
		return "VERIFY_CONTROLLER", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_COMPATIBLE_CONTROLLER:
		return "USE_COMPATIBLE_CONTROLLER", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_NEW_SAME_PRINCIPAL_CONTROLLER:
		return "USE_NEW_SAME_PRINCIPAL_CONTROLLER", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_SUPPORTED_PROFILE:
		return "USE_SUPPORTED_PROFILE", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_USE_TERMINAL_SOURCE:
		return "USE_TERMINAL_SOURCE", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_SUBMIT_WRITE_TURN:
		return "SUBMIT_WRITE_TURN", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT:
		return "WAIT", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RETRY_EXACT_IDEMPOTENCY_KEY:
		return "RETRY_EXACT_IDEMPOTENCY_KEY", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFETCH_INTERACTION:
		return "REFETCH_INTERACTION", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONCILE_RUN:
		return "RECONCILE_RUN", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECOVER_RUN:
		return "RECOVER_RUN", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONNECT_FROM_COMMITTED_CURSOR:
		return "RECONNECT_FROM_COMMITTED_CURSOR", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RESTART_GATEWAY:
		return "RESTART_GATEWAY", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_OPERATOR_REPAIR:
		return "OPERATOR_REPAIR", true
	case publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_SOCKET_PATH:
		return "FIX_SOCKET_PATH", true
	// Continuation is outside the first-release surface (ADR-0050).
	default:
		return "", false
	}
}

func mapErrorCode(code string) string {
	switch code {
	case "PROTOCOL_VERSION_UNSUPPORTED", "UNSUPPORTED_SCHEMA_VERSION", "COMPATIBILITY_REJECTED":
		return "PROTOCOL_INCOMPATIBLE"
	case "CONTROLLER_MISMATCH", "CONTROL_MODE_CONTROLLER_MISMATCH":
		return "CONTROLLER_MISMATCH"
	case "WRITE_CONTINUATION_CONTROLLER_INVALID":
		return "WRITE_CONTINUATION_CONTROLLER_INVALID"
	case "WRITER_BUSY", "STALE_WRITER_GENERATION", "CROSS_CONTROLLER_RELEASE_REQUIRED":
		return "WRITER_CONFLICT"
	case "THREADLESS_REQUIRES_WRITE_TURN":
		return "THREADLESS_REQUIRES_WRITE_TURN"
	case "INTERACTION_STALE":
		return "INTERACTION_STALE"
	case "INTERACTION_ALREADY_RESOLVED":
		return "INTERACTION_ALREADY_RESOLVED"
	case "RECOVERY_REQUIRED", "DEDICATED_HISTORY_BARRIER_FAILED":
		return "RECOVERY_REQUIRED"
	case "OUTCOME_UNKNOWN", "INTERACTION_OUTCOME_UNKNOWN":
		return "OUTCOME_UNKNOWN"
	case "SLOW_CONSUMER":
		return "SLOW_CONSUMER"
	case "ARTIFACT_NOT_FOUND", "ARTIFACT_INTEGRITY_FAILURE":
		return "ARTIFACT_UNAVAILABLE"
	case "RUN_STATE_CONFLICT", "SESSION_CLOSE_IN_PROGRESS":
		return "RUN_STATE_CONFLICT"
	case "RPC_SERVER_ALREADY_RUNNING":
		return "RPC_SERVER_ALREADY_RUNNING"
	case "RUNTIME_PATH_INVALID", "RUNTIME_PATH_COLLISION", "RPC_SOCKET_UNSAFE":
		return "RUNTIME_PATH_UNAVAILABLE"
	case "TRANSPORT_FAILURE", "SERVER_SHUTDOWN", "DEDICATED_SERVER_START_FAILED":
		return "TRANSPORT_UNAVAILABLE"
	case "OPERATION_TIMEOUT":
		return "DEADLINE_EXCEEDED"
	case "RUN_BUSY":
		return "RUN_STATE_CONFLICT"
	case "INTERACTION_RESPONSE_TOO_LARGE", "INTERACTION_PAYLOAD_TOO_LARGE", "PROTOCOL_FRAME_TOO_LARGE":
		return "LIMIT_EXCEEDED"
	case "OPERATOR_MISMATCH", "CONTROLLER_RESET_NOT_ALLOWED", "AUDIT_INTEGRITY_FAILURE", "RUN_STATE_INVARIANT_VIOLATION":
		return "OPERATOR_ACTION_REQUIRED"
	case "INVALID_ARGUMENT", "CONTROL_MODE_REQUIRED", "PURPOSE_REQUIRED", "EXECUTION_LANE_REQUIRED", "ARTIFACT_RANGE_INVALID", "EVENT_CURSOR_INVALID", "INTERACTION_RESPONSE_INVALID":
		return "INVALID_REQUEST"
	default:
		return "PROVIDER_BLOCKED"
	}
}

// MappedErrorCodes returns the browser-code output range for the accepted
// provider catalog, including status-only and fail-closed dispositions.
func MappedErrorCodes() []string {
	codes := map[string]struct{}{
		"TRANSPORT_UNAVAILABLE": {},
		"DEADLINE_EXCEEDED":     {},
		"PROTOCOL_INCOMPATIBLE": {},
		"PROVIDER_BLOCKED":      {},
	}
	for code := range acceptedErrorCodes {
		codes[mapErrorCode(code)] = struct{}{}
	}
	result := make([]string, 0, len(codes))
	for code := range codes {
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}

// MappedActions returns the browser-action output range for accepted
// first-release provider actions and the fail-closed disposition.
func MappedActions() []string {
	actions := map[string]struct{}{"OPERATOR_REPAIR": {}}
	for number := range publicv1.RequiredClientAction_name {
		if action, ok := mapRequiredAction(publicv1.RequiredClientAction(number)); ok {
			actions[action] = struct{}{}
		}
	}
	result := make([]string, 0, len(actions))
	for action := range actions {
		result = append(result, action)
	}
	sort.Strings(result)
	return result
}
