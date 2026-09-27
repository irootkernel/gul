package contractprovider

import (
	"context"
	"errors"
	"testing"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/action"
	"google.golang.org/protobuf/proto"
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

type independentWriterPort struct {
	*wireActions
	response *publicv1.GetWorkspaceWriterStatusResponse
	err      error
	onRead   func(context.Context) error
	calls    int
	request  *publicv1.GetWorkspaceWriterStatusRequest
}

func (p *independentWriterPort) GetWorkspaceWriterStatus(ctx context.Context, q *publicv1.GetWorkspaceWriterStatusRequest) (*publicv1.GetWorkspaceWriterStatusResponse, error) {
	p.calls++
	p.request = q
	if p.onRead != nil {
		return nil, p.onRead(ctx)
	}
	return p.response, p.err
}

func TestReadWriterPreservesIndependentOwnerlessProjection(t *testing.T) {
	f, bound := fixture()
	wire := &independentWriterPort{wireActions: f, response: &publicv1.GetWorkspaceWriterStatusResponse{Writer: f.writer}}
	p, err := New(wire, f.caps)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.ReadWriter(t.Context(), bound)
	if err != nil {
		t.Fatal(err)
	}
	if wire.calls != 1 || f.sensitive != 0 || !proto.Equal(wire.request.Workspace, ref(bound).Workspace) {
		t.Fatal("writer read escaped its independent workspace boundary")
	}
	if out.Owner != action.NoOwner || out.Authority != action.Unowned || out.Generation != 7 || out.Access != action.UnknownAccess || out.Verification != action.Unverified || out.Reconciliation != action.NoReconciliation || out.Revision != 2 || out.Stamp.Writer != 2 || out.Stamp.Run != 0 || out.Stamp.Head != "" || out.Stamp.Interaction != 0 || out.Lane != 0 || out.Requested != 0 || out.Achieved != 0 {
		t.Fatalf("ownerless projection borrowed Run state: %+v", out)
	}
}

func TestReadWriterRejectsUnavailableAndUncheckedProjection(t *testing.T) {
	for _, name := range []string{"transport", "nil response", "nil writer", "unknown writer enum", "unknown response field"} {
		t.Run(name, func(t *testing.T) {
			f, bound := fixture()
			wire := &independentWriterPort{wireActions: f, response: &publicv1.GetWorkspaceWriterStatusResponse{Writer: f.writer}}
			want := action.ErrBlocked
			switch name {
			case "transport":
				wire.response, wire.err, want = nil, errors.New("private transport failure"), action.ErrUnavailable
			case "nil response":
				wire.response = nil
			case "nil writer":
				wire.response.Writer = nil
			case "unknown writer enum":
				wire.response.Writer.AuthorityState = 999
			case "unknown response field":
				wire.response.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			}
			p, err := New(wire, f.caps)
			if err != nil {
				t.Fatal(err)
			}
			out, err := p.ReadWriter(t.Context(), bound)
			if !errors.Is(err, want) || out != (action.WriterProjection{}) || wire.calls != 1 || f.sensitive != 0 {
				t.Fatalf("read = %+v, %v; calls %d", out, err, wire.calls)
			}
		})
	}
}

func TestReadWriterCancellationBeforeAndDuringRead(t *testing.T) {
	for _, before := range []bool{true, false} {
		f, bound := fixture()
		ctx, cancel := context.WithCancel(t.Context())
		wire := &independentWriterPort{wireActions: f}
		wire.onRead = func(received context.Context) error {
			if received != ctx {
				t.Fatal("writer call lost caller context")
			}
			cancel()
			return received.Err()
		}
		if before {
			cancel()
		}
		p, err := New(wire, f.caps)
		if err != nil {
			t.Fatal(err)
		}
		out, err := p.ReadWriter(ctx, bound)
		cancel()
		expectedCalls := 1
		if before {
			expectedCalls = 0
		}
		if !errors.Is(err, context.Canceled) || out != (action.WriterProjection{}) || wire.calls != expectedCalls || f.sensitive != 0 {
			t.Fatalf("before=%v read=%+v, %v; calls=%d", before, out, err, wire.calls)
		}
	}
}
