// Package operation defines non-secret mutation-attempt records shared by the
// coordination core and its isolated persistence adapter.
package operation

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("operation attempt not found")
var ErrConflict = errors.New("conflicting mutation attempt")

type ControllerReference struct {
	Role                 string
	ExpectedControllerID string
	CredentialKey        string
	BindingID            string
}

type Attempt struct {
	OperationID          string
	SubjectID            string
	Kind                 string
	RequestSHA256        string
	ReplayKey            string
	ReplayAvailable      bool
	ControllerReferences []ControllerReference
	State                string
	CreatedAt            time.Time
}

type OperationAttempt = Attempt

type MutationAttempt struct {
	OperationAttempt
	TargetRef           string
	DeadlineAt          time.Time
	ReconciliationRoute string
	OutcomeRef          string
}
