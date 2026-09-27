package action

import "testing"

func TestWriteRequiresAllIndependentFactsAndExplicitIntent(t *testing.T) {
	for bits := 0; bits < 32; bits++ {
		f := WriterFacts{Fresh: bits&1 != 0, ActiveAuthority: bits&2 != 0, VerifiedPolicy: bits&4 != 0, EffectiveRead: bits&8 != 0, EffectiveWrite: bits&16 != 0}
		for _, intent := range []WriteIntent{IntentRead, IntentWrite, 99} {
			got := EvaluateWriter(f, intent)
			write := f.Fresh && f.ActiveAuthority && f.VerifiedPolicy && !f.EffectiveRead && f.EffectiveWrite
			if (got.Mode == WriterWrite) != write || got.SubmitWrite != (write && intent == IntentWrite) {
				t.Fatalf("facts %+v intent %v -> %+v", f, intent, got)
			}
		}
	}
}
