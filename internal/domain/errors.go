package domain

import (
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/contract/port"
)

// BrowserDomainError converts the typed consumer-port disposition to the
// generated Gul browser enums. An unknown internal symbol remains blocked.
func BrowserDomainError(provider port.ProviderError) *gulv1.DomainError {
	code, codeOK := gulv1.ErrorCode_value["ERROR_CODE_"+provider.Code]
	action, actionOK := gulv1.ActionClass_value["ACTION_CLASS_"+provider.Action]
	if !codeOK || !actionOK || code == 0 || action == 0 {
		return &gulv1.DomainError{
			Code:   gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED,
			Action: gulv1.ActionClass_ACTION_CLASS_OPERATOR_REPAIR,
		}
	}
	return &gulv1.DomainError{Code: gulv1.ErrorCode(code), Action: gulv1.ActionClass(action)}
}
