package contractprovider

import (
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"github.com/rootkernel/gul/internal/observation"
)

// WriterDecision maps Writer-only facts to verified access presentation.
// Complete request admission uses action.Evaluate and fresh Run facts.
func WriterDecision(writer *publicv1.WriterState, runID string, fresh bool, intent publicv1.WriteIntent) (action.WriterDecision, publicv1.WriteIntent) {
	stamp := writer.GetStamp()
	validStamp := stamp != nil && (observation.Stamp{Head: observation.Cursor(stamp.GetCapturedHeadCursor()), Run: stamp.GetRunStateRevision(), Writer: stamp.GetWriterStateRevision(), Interaction: stamp.GetInteractionStateRevision()}).Valid()
	facts := action.WriterFacts{Fresh: fresh && validStamp && writer.GetStateRevision() > 0 && writer.GetStateRevision() == stamp.GetWriterStateRevision() && runID != "",
		ActiveAuthority: writer.GetAuthorityState() == publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE && writer.GetOwnerRunId() == runID,
		VerifiedPolicy:  writer.GetPolicyVerification() == publicv1.PolicyVerification_POLICY_VERIFICATION_VERIFIED,
		EffectiveRead:   writer.GetEffectiveAccess() == publicv1.EffectiveAccess_EFFECTIVE_ACCESS_READ,
		EffectiveWrite:  writer.GetEffectiveAccess() == publicv1.EffectiveAccess_EFFECTIVE_ACCESS_WRITE}
	requested := action.IntentRead
	if intent == publicv1.WriteIntent_WRITE_INTENT_WRITE {
		requested = action.IntentWrite
	}
	decision := action.EvaluateWriter(facts, requested)
	flag := publicv1.WriteIntent_WRITE_INTENT_READ
	if decision.SubmitWrite {
		flag = publicv1.WriteIntent_WRITE_INTENT_WRITE
	}
	return decision, flag
}
