// Package action derives Gul presentation and operation flags from typed state.
package action

// WriterFacts are independent provider facts. Missing or unknown values remain
// false; a caller must not infer verified policy from writer authority.
type WriterFacts struct {
	Fresh            bool
	ActiveAuthority  bool
	VerifiedPolicy   bool
	UnverifiedPolicy bool
	EffectiveRead    bool
	EffectiveWrite   bool
}
type WriteIntent uint8

const (
	IntentRead WriteIntent = iota
	IntentWrite
)

type WriterMode string

const (
	WriterBlocked    WriterMode = "blocked"
	WriterReadOnly   WriterMode = "read_only"
	WriterWrite      WriterMode = "write"
	WriterUnverified WriterMode = "unverified"
)

type WriterDecision struct {
	Mode        WriterMode
	SubmitWrite bool
}

func EvaluateWriter(f WriterFacts, intent WriteIntent) WriterDecision {
	result := WriterDecision{Mode: WriterBlocked}
	if f.Fresh && f.UnverifiedPolicy && !f.VerifiedPolicy && !f.EffectiveRead && !f.EffectiveWrite {
		result.Mode = WriterUnverified
		return result
	}
	if !f.Fresh || !f.VerifiedPolicy || f.EffectiveRead == f.EffectiveWrite {
		return result
	}
	if f.EffectiveRead || !f.ActiveAuthority {
		result.Mode = WriterReadOnly
		return result
	}
	result.Mode = WriterWrite
	result.SubmitWrite = intent == IntentWrite
	return result
}
