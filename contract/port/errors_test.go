package port

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"connectrpc.com/connect"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func providerError(t *testing.T, code string, action publicv1.RequiredClientAction) error {
	t.Helper()
	detail, err := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{
		DetailVersion:          1,
		DolgoraeErrorCode:      code,
		Action:                 action,
		RetryClassification:    publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN,
		RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := connect.NewError(connect.CodeFailedPrecondition, nil)
	provider.AddDetail(detail)
	return provider
}

func TestAcceptedPolicyCatalogAndBrowserMapping(t *testing.T) {
	bytes, err := os.ReadFile("../upstream/dolgorae-grpc-error-mapping-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		RequiredActionOverrides         map[string][]string                   `json:"required_action_overrides"`
		StatusOverrides                 map[string][]string                   `json:"status_overrides"`
		RetryClassificationOverrides    map[string][]string                   `json:"retry_classification_overrides"`
		RecoveryClassificationOverrides map[string][]string                   `json:"recovery_classification_overrides"`
		MethodOverrides                 map[string]map[string]json.RawMessage `json:"method_overrides"`
		MethodClassOverrides            map[string]map[string]json.RawMessage `json:"method_class_overrides"`
	}
	if err := json.Unmarshal(bytes, &policy); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{})
	for _, group := range []map[string][]string{policy.RequiredActionOverrides, policy.StatusOverrides, policy.RetryClassificationOverrides, policy.RecoveryClassificationOverrides} {
		for _, codes := range group {
			for _, code := range codes {
				seen[code] = struct{}{}
			}
		}
	}
	for _, group := range []map[string]map[string]json.RawMessage{policy.MethodOverrides, policy.MethodClassOverrides} {
		for _, overrides := range group {
			for code := range overrides {
				seen[code] = struct{}{}
			}
		}
	}
	if len(seen) != len(acceptedErrorCodes) {
		t.Fatalf("accepted catalog = %d codes; policy = %d", len(acceptedErrorCodes), len(seen))
	}
	for code := range seen {
		if _, ok := acceptedErrorCodes[code]; !ok {
			t.Fatalf("unmapped upstream code %s", code)
		}
		if mapped := mapErrorCode(code); mapped == "" {
			t.Fatalf("empty Gul code for %s", code)
		}
	}
}

func TestSpecificAcceptedErrorFamiliesStayDistinct(t *testing.T) {
	for want, codes := range map[string][]string{
		"PROTOCOL_INCOMPATIBLE":                 {"PROTOCOL_VERSION_UNSUPPORTED", "UNSUPPORTED_SCHEMA_VERSION", "COMPATIBILITY_REJECTED"},
		"CONTROLLER_MISMATCH":                   {"CONTROLLER_MISMATCH", "CONTROL_MODE_CONTROLLER_MISMATCH"},
		"WRITE_CONTINUATION_CONTROLLER_INVALID": {"WRITE_CONTINUATION_CONTROLLER_INVALID"},
		"WRITER_CONFLICT":                       {"WRITER_BUSY", "STALE_WRITER_GENERATION", "CROSS_CONTROLLER_RELEASE_REQUIRED"},
		"THREADLESS_REQUIRES_WRITE_TURN":        {"THREADLESS_REQUIRES_WRITE_TURN"},
		"INTERACTION_STALE":                     {"INTERACTION_STALE"},
		"INTERACTION_ALREADY_RESOLVED":          {"INTERACTION_ALREADY_RESOLVED"},
		"RECOVERY_REQUIRED":                     {"RECOVERY_REQUIRED", "DEDICATED_HISTORY_BARRIER_FAILED"},
		"OUTCOME_UNKNOWN":                       {"OUTCOME_UNKNOWN", "INTERACTION_OUTCOME_UNKNOWN"},
		"SLOW_CONSUMER":                         {"SLOW_CONSUMER"},
		"ARTIFACT_UNAVAILABLE":                  {"ARTIFACT_NOT_FOUND", "ARTIFACT_INTEGRITY_FAILURE"},
		"RUN_STATE_CONFLICT":                    {"RUN_STATE_CONFLICT", "SESSION_CLOSE_IN_PROGRESS", "RUN_BUSY"},
		"RPC_SERVER_ALREADY_RUNNING":            {"RPC_SERVER_ALREADY_RUNNING"},
		"RUNTIME_PATH_UNAVAILABLE":              {"RUNTIME_PATH_INVALID", "RUNTIME_PATH_COLLISION", "RPC_SOCKET_UNSAFE"},
		"TRANSPORT_UNAVAILABLE":                 {"TRANSPORT_FAILURE", "SERVER_SHUTDOWN", "DEDICATED_SERVER_START_FAILED"},
		"DEADLINE_EXCEEDED":                     {"OPERATION_TIMEOUT"},
		"LIMIT_EXCEEDED":                        {"INTERACTION_RESPONSE_TOO_LARGE", "INTERACTION_PAYLOAD_TOO_LARGE", "PROTOCOL_FRAME_TOO_LARGE"},
		"OPERATOR_ACTION_REQUIRED":              {"OPERATOR_MISMATCH", "CONTROLLER_RESET_NOT_ALLOWED", "AUDIT_INTEGRITY_FAILURE", "RUN_STATE_INVARIANT_VIOLATION"},
		"INVALID_REQUEST":                       {"INVALID_ARGUMENT", "CONTROL_MODE_REQUIRED", "PURPOSE_REQUIRED", "EXECUTION_LANE_REQUIRED", "ARTIFACT_RANGE_INVALID", "EVENT_CURSOR_INVALID", "INTERACTION_RESPONSE_INVALID"},
	} {
		for _, code := range codes {
			if got := mapErrorCode(code); got != want {
				t.Errorf("%s mapped to %s, want %s", code, got, want)
			}
		}
	}
}

func TestProviderErrorMappingAndFailClosedCases(t *testing.T) {
	for _, test := range []struct {
		code   string
		action publicv1.RequiredClientAction
		want   ProviderError
	}{
		{"CONTROLLER_MISMATCH", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_VERIFY_CONTROLLER, ProviderError{"CONTROLLER_MISMATCH", "VERIFY_CONTROLLER"}},
		{"THREADLESS_REQUIRES_WRITE_TURN", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_SUBMIT_WRITE_TURN, ProviderError{"THREADLESS_REQUIRES_WRITE_TURN", "SUBMIT_WRITE_TURN"}},
		{"OUTCOME_UNKNOWN", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_RECONCILE_RUN, ProviderError{"OUTCOME_UNKNOWN", "RECONCILE_RUN"}},
		{"TRANSPORT_FAILURE", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT, ProviderError{"TRANSPORT_UNAVAILABLE", "REFRESH_SNAPSHOT"}},
		{"RUN_BUSY", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT, ProviderError{"RUN_STATE_CONFLICT", "WAIT"}},
		{"INTERACTION_RESPONSE_TOO_LARGE", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_REQUEST, ProviderError{"LIMIT_EXCEEDED", "FIX_REQUEST"}},
		{"RPC_SOCKET_UNSAFE", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_FIX_SOCKET_PATH, ProviderError{"RUNTIME_PATH_UNAVAILABLE", "FIX_SOCKET_PATH"}},
		{"SHARED_RUN_WRITE_FORBIDDEN", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION, blocked()},
		{"FUTURE_CODE", publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_ABORT, blocked()},
		{"RUN_BUSY", publicv1.RequiredClientAction(999), blocked()},
	} {
		if got := MapProviderError(providerError(t, test.code, test.action)); got != test.want {
			t.Errorf("%s mapped to %+v, want %+v", test.code, got, test.want)
		}
	}
	for _, test := range []struct {
		code connect.Code
		want string
	}{
		{connect.CodeUnavailable, "TRANSPORT_UNAVAILABLE"},
		{connect.CodeDeadlineExceeded, "DEADLINE_EXCEEDED"},
		{connect.CodeFailedPrecondition, "PROTOCOL_INCOMPATIBLE"},
	} {
		if got := MapProviderError(connect.NewError(test.code, nil)); got.Code != test.want {
			t.Errorf("missing detail %v mapped to %+v", test.code, got)
		}
	}
	if got := MapProviderError(errors.New("transport failed")); got != (ProviderError{Code: "TRANSPORT_UNAVAILABLE", Action: "REFRESH_SNAPSHOT"}) {
		t.Errorf("non-connect transport error mapped to %+v", got)
	}
	detail, err := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{DetailVersion: 1, DolgoraeErrorCode: "RUN_BUSY", Action: publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT, RetryClassification: publicv1.RetryClassification_RETRY_CLASSIFICATION_FORBIDDEN, RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := connect.NewError(connect.CodeFailedPrecondition, nil)
	duplicate.AddDetail(detail)
	duplicate.AddDetail(detail)
	if got := MapProviderError(duplicate); got != blocked() {
		t.Errorf("duplicate detail mapped to %+v", got)
	}
}

func TestEveryAcceptedActionHasAClosedFirstReleaseDisposition(t *testing.T) {
	for number := range publicv1.RequiredClientAction_name {
		action := publicv1.RequiredClientAction(number)
		mapped, ok := mapRequiredAction(action)
		if action == publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_UNSPECIFIED ||
			action == publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION {
			if ok {
				t.Errorf("unsupported action %v mapped to %q", action, mapped)
			}
			continue
		}
		if !ok || mapped == "" {
			t.Errorf("accepted action %v has no Gul disposition", action)
		}
	}
}

func TestMalformedTypedDetailFailsClosed(t *testing.T) {
	for _, detail := range []*publicv1.DolgoraeErrorDetail{
		{DetailVersion: 1, DolgoraeErrorCode: "RUN_BUSY", Action: publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT},
		{DetailVersion: 1, DolgoraeErrorCode: "RUN_BUSY", Action: publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_WAIT, RetryClassification: 99, RecoveryClassification: publicv1.RecoveryClassification_RECOVERY_CLASSIFICATION_NONE},
	} {
		encoded, err := connect.NewErrorDetail(detail)
		if err != nil {
			t.Fatal(err)
		}
		provider := connect.NewError(connect.CodeFailedPrecondition, nil)
		provider.AddDetail(encoded)
		if got := MapProviderError(provider); got != blocked() {
			t.Errorf("malformed detail mapped to %+v", got)
		}
	}
}

func TestUnknownDetailVersionIsProtocolIncompatible(t *testing.T) {
	detail, err := connect.NewErrorDetail(&publicv1.DolgoraeErrorDetail{DetailVersion: 2, DolgoraeErrorCode: "RUN_BUSY"})
	if err != nil {
		t.Fatal(err)
	}
	provider := connect.NewError(connect.CodeFailedPrecondition, nil)
	provider.AddDetail(detail)
	if got := MapProviderError(provider); got != (ProviderError{Code: "PROTOCOL_INCOMPATIBLE", Action: "OPERATOR_REPAIR"}) {
		t.Errorf("future detail version mapped to %+v", got)
	}
}

func TestUndecodableTypedDetailFailsClosed(t *testing.T) {
	detail, err := connect.NewErrorDetail(&anypb.Any{
		TypeUrl: "type.googleapis.com/" + string(proto.MessageName(&publicv1.DolgoraeErrorDetail{})),
		Value:   []byte{0xff},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := connect.NewError(connect.CodeFailedPrecondition, nil)
	provider.AddDetail(detail)
	if got := MapProviderError(provider); got != blocked() {
		t.Errorf("undecodable detail mapped to %+v", got)
	}
}
