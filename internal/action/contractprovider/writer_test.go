package contractprovider

import (
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestWriterOwnerPolicyAndIntentCannotBeInferred(t *testing.T) {
	source := &publicv1.WriterState{StateRevision: 1, OwnerRunId: proto.String("run"), AuthorityState: publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE, PolicyVerification: publicv1.PolicyVerification_POLICY_VERIFICATION_VERIFIED, EffectiveAccess: publicv1.EffectiveAccess_EFFECTIVE_ACCESS_WRITE, Stamp: &publicv1.ProjectionStamp{CapturedHeadCursor: "2", RunStateRevision: 2, WriterStateRevision: 1}}
	for _, owner := range []string{"run", "other", ""} {
		for _, fresh := range []bool{false, true} {
			for _, intent := range []publicv1.WriteIntent{publicv1.WriteIntent_WRITE_INTENT_READ, publicv1.WriteIntent_WRITE_INTENT_WRITE} {
				w := proto.Clone(source).(*publicv1.WriterState)
				w.OwnerRunId = proto.String(owner)
				d, flag := WriterDecision(w, "run", fresh, intent)
				write := owner == "run" && fresh
				if (d.Mode == action.WriterWrite) != write || (flag == publicv1.WriteIntent_WRITE_INTENT_WRITE) != (write && intent == publicv1.WriteIntent_WRITE_INTENT_WRITE) {
					t.Fatal(d, flag)
				}
			}
		}
	}
	for _, verification := range []publicv1.PolicyVerification{0, publicv1.PolicyVerification_POLICY_VERIFICATION_UNVERIFIED, publicv1.PolicyVerification_POLICY_VERIFICATION_FAILED, 99} {
		source.PolicyVerification = verification
		d, flag := WriterDecision(source, "run", true, publicv1.WriteIntent_WRITE_INTENT_WRITE)
		if d.Mode == action.WriterWrite || flag == publicv1.WriteIntent_WRITE_INTENT_WRITE {
			t.Fatal(d, flag)
		}
	}
}
