package contractprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type wireFake struct {
	detail                  *publicv1.ControllerInteraction
	drift, foreign, corrupt bool
	calls                   int
	artifact                []byte
}

func (f *wireFake) GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	id := "controller"
	if f.foreign {
		id = "other"
	}
	return &publicv1.GetRunResponse{Run: &publicv1.RunProjection{RunId: "run", WorkspaceId: "provider", StateRevision: 2, ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, Controller: &publicv1.ControllerProjection{ControllerId: id, Generation: 1}, Stamp: wireStamp()}}, nil
}
func wireStamp() *publicv1.ProjectionStamp {
	return &publicv1.ProjectionStamp{CapturedHeadCursor: "2", RunStateRevision: 2, InteractionStateRevision: 2}
}
func (f *wireFake) ListPendingInteractions(context.Context, *publicv1.ListPendingInteractionsRequest) (*publicv1.ListPendingInteractionsResponse, error) {
	response := &publicv1.ListPendingInteractionsResponse{Stamp: wireStamp()}
	if f.detail.Summary.Status == publicv1.InteractionStatus_INTERACTION_STATUS_PENDING {
		response.Items = []*publicv1.InteractionSummary{proto.Clone(f.detail.Summary).(*publicv1.InteractionSummary)}
	}
	return response, nil
}
func (f *wireFake) GetControllerInteraction(_ context.Context, r *publicv1.GetControllerInteractionRequest) (*publicv1.GetControllerInteractionResponse, error) {
	f.calls++
	if r.Controller.GetExpectedControllerId() != "controller" {
		return nil, errors.New("private")
	}
	d := proto.Clone(f.detail).(*publicv1.ControllerInteraction)
	if f.drift {
		d.Stamp.RunStateRevision++
	}
	return &publicv1.GetControllerInteractionResponse{Interaction: d}, nil
}
func (f *wireFake) ResolveInteraction(context.Context, *publicv1.ResolveInteractionRequest) (*publicv1.ResolveInteractionResponse, error) {
	return &publicv1.ResolveInteractionResponse{InteractionId: "id", Status: publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED, ResolutionReceipt: "opaque"}, nil
}
func (f *wireFake) GetArtifact(context.Context, *publicv1.GetArtifactRequest) (*publicv1.GetArtifactResponse, error) {
	return &publicv1.GetArtifactResponse{Artifact: f.detail.GetFileChangeApproval().GetChangeArtifact(), MaximumChunkSize: 7}, nil
}
func (f *wireFake) ReadArtifactChunk(_ context.Context, r *publicv1.ReadArtifactChunkRequest) (*publicv1.ReadArtifactChunkResponse, error) {
	data := append([]byte(nil), f.artifact[r.Offset:r.Offset+uint64(r.Length)]...)
	if f.corrupt {
		data[0] ^= 1
	}
	return &publicv1.ReadArtifactChunkResponse{ArtifactId: r.ArtifactId, Offset: r.Offset, Length: r.Length, Data: data, Eof: r.Offset+uint64(r.Length) == uint64(len(f.artifact)), TotalByteLength: uint64(len(f.artifact)), Sha256: f.detail.GetFileChangeApproval().GetChangeArtifact().Sha256}, nil
}
func bound() interaction.Bound {
	return interaction.Bound{Binding: session.Binding{ID: "session", RunID: "run", SubjectID: "owner"}, Workspace: workspace.Attachment{CanonicalRoot: "/workspace", ProviderID: "provider"}, Carrier: session.Carrier{ControllerID: "controller", Generation: 1, AbsolutePath: "/private/carrier"}}
}
func caps() *publicv1.InteractionCapabilities {
	c := &publicv1.InteractionCapabilities{MaximumResponseBytes: 1024 * 1024, MaximumSafePayloadBytes: 8 * 1024 * 1024}
	for i := 1; i <= 7; i++ {
		kind := publicv1.InteractionKind(i)
		support := publicv1.InteractionSupport_INTERACTION_SUPPORT_SUPPORTED
		if classify(kind) == interaction.Unsupported {
			support = publicv1.InteractionSupport_INTERACTION_SUPPORT_RECOGNIZED_UNSUPPORTED
		}
		c.KnownKinds = append(c.KnownKinds, kind)
		c.Items = append(c.Items, &publicv1.InteractionCapability{Kind: kind, Support: support})
	}
	return c
}
func detail(kind publicv1.InteractionKind) *publicv1.ControllerInteraction {
	d := &publicv1.ControllerInteraction{Summary: &publicv1.InteractionSummary{InteractionId: "id", RunId: "run", Kind: kind, Status: publicv1.InteractionStatus_INTERACTION_STATUS_PENDING, ControllerKind: publicv1.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT, CreatedAt: timestamppb.New(time.Now()), StateRevision: 2, RequiresUserEscalation: classify(kind) != interaction.Unsupported}, Stamp: wireStamp()}
	switch classify(kind) {
	case interaction.CommandApproval:
		d.ResponseSchemaId = "dolgorae.interaction.command-approval/v1"
		d.Payload = &publicv1.ControllerInteraction_CommandApproval{CommandApproval: &publicv1.CommandApprovalInteraction{Title: "Command", Message: "Run tests", Command: []string{"make", "test"}, Cwd: &publicv1.PathProjection{Value: &publicv1.PathProjection_Utf8Path{Utf8Path: "/workspace"}}}}
	case interaction.FileApproval:
		d.ResponseSchemaId = "dolgorae.interaction.file-change-approval/v1"
		d.Payload = &publicv1.ControllerInteraction_FileChangeApproval{FileChangeApproval: &publicv1.FileChangeApprovalInteraction{Title: "Files", Message: "Update code", SnapshotSha256: strings.Repeat("a", 64), Representation: &publicv1.FileChangeApprovalInteraction_InlineChanges{InlineChanges: &publicv1.InlineFileChanges{Items: []*publicv1.FileChangeProjection{{Path: &publicv1.PathProjection{Value: &publicv1.PathProjection_Utf8Path{Utf8Path: "src/main.go"}}, Kind: publicv1.FileChangeKind_FILE_CHANGE_KIND_UPDATE, UnifiedDiff: "+changed"}}}}}}
	case interaction.UserInput:
		d.ResponseSchemaId = "dolgorae.interaction.user-input/v1"
		d.Summary.ContainsProtectedInput = true
		d.Payload = &publicv1.ControllerInteraction_UserInput{UserInput: &publicv1.UserInputInteraction{Questions: []*publicv1.InteractionQuestion{{Id: "q", Header: "Value", Question: "Enter credential", IsSecret: true}}}}
	default:
		d.ResponseSchemaId = "dolgorae.interaction.unsupported/v1"
		d.Payload = &publicv1.ControllerInteraction_Unsupported{Unsupported: &publicv1.UnsupportedInteraction{OriginalKind: kind, Reason: publicv1.UnsupportedInteractionReason_UNSUPPORTED_INTERACTION_REASON_RECOGNIZED_UNSUPPORTED}}
	}
	if classify(kind) == interaction.CommandApproval || classify(kind) == interaction.FileApproval {
		d.Decisions = []publicv1.InteractionDecision{1, 2, 3}
	}
	return d
}
func TestEveryAdvertisedKindAndTypedVariant(t *testing.T) {
	descriptor := publicv1.InteractionKind(0).Descriptor()
	if descriptor.Values().Len() != 8 {
		t.Fatal("reconcile advertised kind matrix")
	}
	payload := (&publicv1.ControllerInteraction{}).ProtoReflect().Descriptor().Oneofs().ByName("payload")
	if payload.Fields().Len() != 4 {
		t.Fatal("reconcile payload matrix")
	}
	for i := 1; i <= 7; i++ {
		f := &wireFake{detail: detail(publicv1.InteractionKind(i))}
		p, err := New(f, caps())
		if err != nil {
			t.Fatal(err)
		}
		card, err := p.Card(t.Context(), bound(), "id")
		if err != nil || card.Summary.Kind != classify(publicv1.InteractionKind(i)) {
			t.Fatalf("kind%d %v", i, err)
		}
		if f.calls != 1 {
			t.Fatal("missing authorized detail")
		}
	}
}
func TestRejectsDriftUnknownMismatchAndUnrestrictedPaths(t *testing.T) {
	cases := map[string]func(*wireFake){
		"stamp": func(f *wireFake) { f.drift = true }, "controller": func(f *wireFake) { f.foreign = true },
		"kind":            func(f *wireFake) { f.detail.Summary.Kind = publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT },
		"unknown enum":    func(f *wireFake) { f.detail.Summary.Kind = 99 },
		"unknown field":   func(f *wireFake) { f.detail.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 1}) },
		"missing context": func(f *wireFake) { f.detail.GetCommandApproval().Command = nil },
		"path": func(f *wireFake) {
			f.detail.GetCommandApproval().Cwd = &publicv1.PathProjection{Value: &publicv1.PathProjection_Utf8Path{Utf8Path: "/etc/passwd"}}
		},
		"opaque path": func(f *wireFake) {
			f.detail.GetCommandApproval().Cwd = &publicv1.PathProjection{Value: &publicv1.PathProjection_OpaquePath{OpaquePath: []byte{255}}}
		},
		"schema":    func(f *wireFake) { f.detail.ResponseSchemaId = "future" },
		"decisions": func(f *wireFake) { f.detail.Decisions = []publicv1.InteractionDecision{1} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := &wireFake{detail: detail(1)}
			change(f)
			p, _ := New(f, caps())
			if _, err := p.Card(t.Context(), bound(), "id"); err == nil {
				t.Fatal("accepted invalid detail")
			}
			if f.foreign && f.calls != 0 {
				t.Fatal("wrong controller fetched detail")
			}
		})
	}
}
func TestEffectiveLimitsUseSelectedPayloadAndCapabilityClassification(t *testing.T) {
	f := &wireFake{detail: detail(1)}
	size := proto.Size(f.detail.GetCommandApproval())
	for _, delta := range []int{-1, 0} {
		c := caps()
		c.MaximumSafePayloadBytes = uint32(size + delta)
		p, err := New(f, c)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Card(t.Context(), bound(), "id")
		if (err == nil) != (delta == 0) {
			t.Fatal("selected payload boundary", size, delta, err)
		}
	}
	c := caps()
	c.MaximumSafePayloadBytes = 20 * 1024 * 1024
	c.MaximumResponseBytes = 20 * 1024 * 1024
	p, _ := New(f, c)
	if p.payloadLimit != 8388608 || p.ResponseLimit() != 65536 {
		t.Fatal("local limit")
	}
	for _, mutate := range []func(*publicv1.InteractionCapabilities){func(c *publicv1.InteractionCapabilities) { c.KnownKinds = append(c.KnownKinds, 99) }, func(c *publicv1.InteractionCapabilities) { c.Items = c.Items[:6] }, func(c *publicv1.InteractionCapabilities) {
		c.Items[2].Support = publicv1.InteractionSupport_INTERACTION_SUPPORT_SUPPORTED
	}} {
		c := caps()
		mutate(c)
		if _, err := New(f, c); err == nil {
			t.Fatal("accepted unclassified kind")
		}
	}
}
func TestArtifactIsCompletelyVerifiedBeforeApproval(t *testing.T) {
	f := &wireFake{detail: detail(2), artifact: []byte("--- a/src/main.go\n+++ b/src/main.go\n+changed\n")}
	sum := sha256.Sum256(f.artifact)
	f.detail.GetFileChangeApproval().Representation = &publicv1.FileChangeApprovalInteraction_ChangeArtifact{ChangeArtifact: &publicv1.ArtifactRef{ArtifactId: "artifact", Kind: publicv1.ArtifactKind_ARTIFACT_KIND_FILE_CHANGE_DIFF, Visibility: publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY, MediaType: "text/x-diff", ByteLength: uint64(len(f.artifact)), Sha256: hex.EncodeToString(sum[:])}}
	p, _ := New(f, caps())
	card, err := p.Card(t.Context(), bound(), "id")
	if err != nil || card.File.VerifiedDiff != string(f.artifact) {
		t.Fatal("verified diff", err)
	}
	f.corrupt = true
	if _, err := p.Card(t.Context(), bound(), "id"); err == nil {
		t.Fatal("approved corrupt artifact")
	}
}

func TestTerminalDetailCannotExceedInteractionStamp(t *testing.T) {
	for _, status := range []publicv1.InteractionStatus{publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED, publicv1.InteractionStatus_INTERACTION_STATUS_STALE} {
		f := &wireFake{detail: detail(1)}
		f.detail.Summary.Status = status
		f.detail.Summary.RequiresUserEscalation = false
		f.detail.Summary.StateRevision = 3
		p, _ := New(f, caps())
		if _, err := p.Card(t.Context(), bound(), "id"); err != interaction.ErrBlocked {
			t.Fatal("future terminal revision", status, err)
		}
	}
}

type inconsistentRun struct{ *wireFake }

func (f inconsistentRun) GetRun(ctx context.Context, r *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	out, err := f.wireFake.GetRun(ctx, r)
	out.Run.StateRevision = 3
	return out, err
}
func TestRunRevisionMustAgreeWithCompleteStamp(t *testing.T) {
	f := &wireFake{detail: detail(1)}
	p, _ := New(inconsistentRun{f}, caps())
	if _, err := p.Card(t.Context(), bound(), "id"); err != interaction.ErrBlocked || f.calls != 0 {
		t.Fatal("inconsistent Run accepted", err)
	}
}
func TestActualSelectedPayloadAtEightMiB(t *testing.T) {
	f := &wireFake{detail: detail(2)}
	files := f.detail.GetFileChangeApproval()
	inline := files.GetInlineChanges()
	inline.Items = nil
	for range 2048 {
		inline.Items = append(inline.Items, &publicv1.FileChangeProjection{Path: &publicv1.PathProjection{Value: &publicv1.PathProjection_Utf8Path{Utf8Path: strings.Repeat("p", 4000)}}, Kind: publicv1.FileChangeKind_FILE_CHANGE_KIND_UPDATE, UnifiedDiff: "+"})
	}
	const limit = 8 * 1024 * 1024
	for _, item := range inline.Items {
		remaining := limit - proto.Size(files)
		if remaining <= 0 {
			break
		}
		path := item.Path.GetValue().(*publicv1.PathProjection_Utf8Path)
		path.Utf8Path += strings.Repeat("x", min(remaining, 4096-len(path.Utf8Path)))
	}
	if proto.Size(files) != limit {
		t.Fatal("fixture size", proto.Size(files))
	}
	c := caps()
	c.MaximumSafePayloadBytes = 16 * 1024 * 1024
	p, _ := New(f, c)
	if _, err := p.Card(t.Context(), bound(), "id"); err != nil {
		t.Fatal("exact 8MiB selected payload rejected", err)
	}
	// The full envelope is larger; only the selected submessage owns this limit.
	if proto.Size(f.detail) <= limit {
		t.Fatal("fixture does not distinguish envelope")
	}
	for _, item := range inline.Items {
		path := item.Path.GetValue().(*publicv1.PathProjection_Utf8Path)
		if len(path.Utf8Path) < 4096 {
			path.Utf8Path += "x"
			break
		}
	}
	if proto.Size(files) != limit+1 {
		t.Fatal("oversize fixture", proto.Size(files))
	}
	if _, err := p.Card(t.Context(), bound(), "id"); err != interaction.ErrBlocked {
		t.Fatal("8MiB+1 accepted", err)
	}
}
