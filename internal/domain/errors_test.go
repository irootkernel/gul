package domain

import (
	"testing"

	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/contract/port"
)

func TestProviderDispositionReachesGeneratedBrowserEnums(t *testing.T) {
	got := BrowserDomainError(port.ProviderError{Code: "WRITER_CONFLICT", Action: "WAIT"})
	if got.Code != gulv1.ErrorCode_ERROR_CODE_WRITER_CONFLICT || got.Action != gulv1.ActionClass_ACTION_CLASS_WAIT {
		t.Fatalf("mapped disposition = %+v", got)
	}
	for _, malformed := range []port.ProviderError{{Code: "FUTURE", Action: "WAIT"}, {Code: "WRITER_CONFLICT", Action: "FUTURE"}} {
		got := BrowserDomainError(malformed)
		if got.Code != gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED || got.Action != gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR {
			t.Errorf("unknown disposition %+v mapped to %+v", malformed, got)
		}
	}
}

func TestEveryProviderDispositionSymbolHasGeneratedBrowserEnum(t *testing.T) {
	browserOnly := map[string]bool{
		"CONTROLLER_CARRIER_INVALID": true,
		"UNSUPPORTED_PATH_ENCODING":  true,
		"INVALID_PAGE_TOKEN":         true,
		"PAGE_TOKEN_EXPIRED":         true,
		"UNAUTHORIZED":               true,
		"SOURCE_UNAVAILABLE":         true,
	}
	mappedCodes := make(map[string]bool)
	for _, code := range port.MappedErrorCodes() {
		mappedCodes[code] = true
		if browserOnly[code] {
			t.Errorf("browser-only code %s produced by provider mapping", code)
		}
		got := BrowserDomainError(port.ProviderError{Code: code, Action: "ABORT"})
		if got.Code.String() != "ERROR_CODE_"+code || got.Action != gulv1.ActionClass_ACTION_CLASS_ABORT {
			t.Errorf("provider code %s mapped to %+v", code, got)
		}
	}
	for _, name := range gulv1.ErrorCode_name {
		if name == "ERROR_CODE_UNSPECIFIED" {
			continue
		}
		code := name[len("ERROR_CODE_"):]
		if !mappedCodes[code] && !browserOnly[code] {
			t.Errorf("browser code %s has no provider or browser-only classification", code)
		}
		delete(browserOnly, code)
	}
	for code := range browserOnly {
		t.Errorf("browser-only classification %s has no generated enum", code)
	}
	for _, action := range port.MappedActions() {
		got := BrowserDomainError(port.ProviderError{Code: "WRITER_CONFLICT", Action: action})
		if got.Code != gulv1.ErrorCode_ERROR_CODE_WRITER_CONFLICT || got.Action.String() != "ACTION_CLASS_"+action {
			t.Errorf("provider action %s mapped to %+v", action, got)
		}
	}
}
