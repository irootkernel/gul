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
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type wirePort struct {
	run        *publicv1.RunProjection
	timeline   *publicv1.ListRunTimelineItemsResponse
	results    *publicv1.ListOrchestratedSessionResultsResponse
	artifact   *publicv1.ArtifactRef
	body       []byte
	maxChunk   uint32
	chunks     []*publicv1.ReadArtifactChunkRequest
	corrupt    func(*publicv1.ReadArtifactChunkResponse)
	metaChange func(*publicv1.GetArtifactResponse)
	cancel     context.CancelFunc
	calls      int
}

func (p *wirePort) GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error) {
	return &publicv1.GetRunResponse{Run: proto.Clone(p.run).(*publicv1.RunProjection)}, nil
}
func (p *wirePort) ListRunTimelineItems(_ context.Context, r *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error) {
	p.calls++
	return proto.Clone(p.timeline).(*publicv1.ListRunTimelineItemsResponse), nil
}
func (p *wirePort) ListOrchestratedSessionResults(context.Context, *publicv1.ListOrchestratedSessionResultsRequest) (*publicv1.ListOrchestratedSessionResultsResponse, error) {
	return proto.Clone(p.results).(*publicv1.ListOrchestratedSessionResultsResponse), nil
}
func (p *wirePort) GetArtifact(_ context.Context, r *publicv1.GetArtifactRequest) (*publicv1.GetArtifactResponse, error) {
	if r.ArtifactId != p.artifact.ArtifactId || r.Controller == nil {
		return nil, errors.New("private error")
	}
	v := &publicv1.GetArtifactResponse{Artifact: proto.Clone(p.artifact).(*publicv1.ArtifactRef), MaximumChunkSize: p.maxChunk}
	if p.metaChange != nil {
		p.metaChange(v)
	}
	return v, nil
}
func (p *wirePort) ReadArtifactChunk(_ context.Context, r *publicv1.ReadArtifactChunkRequest) (*publicv1.ReadArtifactChunkResponse, error) {
	p.chunks = append(p.chunks, proto.Clone(r).(*publicv1.ReadArtifactChunkRequest))
	v := &publicv1.ReadArtifactChunkResponse{ArtifactId: r.ArtifactId, Offset: r.Offset, Length: r.Length, Data: append([]byte(nil), p.body[r.Offset:r.Offset+uint64(r.Length)]...), Eof: r.Offset+uint64(r.Length) == uint64(len(p.body)), TotalByteLength: uint64(len(p.body)), Sha256: p.artifact.Sha256}
	if p.corrupt != nil {
		p.corrupt(v)
	}
	if p.cancel != nil {
		p.cancel()
	}
	return v, nil
}
func ptr[T any](v T) *T   { return &v }
func sum(v []byte) string { h := sha256.Sum256(v); return hex.EncodeToString(h[:]) }
func capabilities() *publicv1.GetCapabilitiesResponse {
	return &publicv1.GetCapabilitiesResponse{Protocol: &publicv1.ProtocolCapabilities{TimelineProtocolVersion: 1}, Features: &publicv1.RuntimeFeatureCapabilities{ArtifactRetrieval: true, ControllerTimeline: true}, Artifacts: &publicv1.ArtifactCapabilities{MaximumArtifactSize: 32 << 20, MaximumChunkSize: 1 << 20, MaximumInlineResponseBytes: 1 << 20, DigestVerificationRequired: true, ExactByteLengthReported: true, VisibilityClasses: []publicv1.ArtifactVisibility{publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_OBSERVER, publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY}}, SupportedMethods: []string{"RunService.GetRun", "ObservationService.ListRunTimelineItems", "OrchestrationService.ListOrchestratedSessionResults", "ArtifactService.GetArtifact", "ArtifactService.ReadArtifactChunk"}}
}
func fixture(t *testing.T) (*Provider, *wirePort, history.Bound) {
	t.Helper()
	b := history.Bound{Binding: session.Binding{SubjectID: "owner", ID: "session", RunID: "private-run", ProviderSessionID: "private-session"}, Workspace: workspace.Attachment{ProviderID: "workspace", CanonicalRoot: "/workspace"}, Carrier: session.Carrier{ControllerID: "controller", AbsolutePath: "/private/carrier", Generation: 1}}
	stamp := &publicv1.ProjectionStamp{CapturedHeadCursor: "10", RunStateRevision: 10, WriterStateRevision: 1, InteractionStateRevision: 1}
	w := &wirePort{run: &publicv1.RunProjection{RunId: b.Binding.RunID, WorkspaceId: b.Workspace.ProviderID, ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, Lifecycle: publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED, Controller: &publicv1.ControllerProjection{ControllerId: "controller", Generation: 1, Kind: publicv1.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT}, Configuration: &publicv1.RunConfigurationProjection{}, Stamp: stamp, StateRevision: 10, EventCursor: "10"}, maxChunk: 1 << 20}
	w.body = []byte(strings.Repeat("한글\r\n", 50000))
	w.artifact = &publicv1.ArtifactRef{ArtifactId: "../../opaque/not-a-path", Kind: publicv1.ArtifactKind_ARTIFACT_KIND_USER_INPUT, Visibility: publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY, MediaType: "text/plain; charset=utf-8", ByteLength: uint64(len(w.body)), Sha256: sum(w.body)}
	w.timeline = &publicv1.ListRunTimelineItemsResponse{CapturedHeadCursor: "10", Stamp: stamp, Items: []*publicv1.TimelineItem{{Type: publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_USER_INPUT_ACCEPTED, RunId: b.Binding.RunID, TurnId: "turn", Cursor: "2", ProviderItemId: ptr("provider-item"), OccurredAt: timestamppb.New(time.Unix(1, 0)), Status: ptr(publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_ACCEPTED), Content: &publicv1.TimelineItem_InlineText{InlineText: "原文\r\n"}}}}
	w.results = &publicv1.ListOrchestratedSessionResultsResponse{SourceRevision: 1, CapturedAt: timestamppb.New(time.Unix(1, 0))}
	p, err := New(w, capabilities())
	if err != nil {
		t.Fatal(err)
	}
	return p, w, b
}
func TestRequiredCapabilityAndProviderLocalBounds(t *testing.T) {
	for name, change := range map[string]func(*publicv1.GetCapabilitiesResponse){"missing-artifact": func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts = nil }, "feature": func(c *publicv1.GetCapabilitiesResponse) { c.Features.ArtifactRetrieval = false }, "digest": func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts.DigestVerificationRequired = false }, "length": func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts.ExactByteLengthReported = false }, "visibility": func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts.VisibilityClasses = nil }, "inline-bound": func(c *publicv1.GetCapabilitiesResponse) { c.Artifacts.MaximumInlineResponseBytes = 0 }, "method": func(c *publicv1.GetCapabilitiesResponse) { c.SupportedMethods = c.SupportedMethods[:4] }, "version": func(c *publicv1.GetCapabilitiesResponse) { c.Protocol.TimelineProtocolVersion = 2 }} {
		t.Run(name, func(t *testing.T) {
			_, w, _ := fixture(t)
			c := capabilities()
			change(c)
			if _, err := New(w, c); !errors.Is(err, history.ErrBlocked) {
				t.Fatal(err)
			}
		})
	}
	for _, providerMax := range []uint64{1024, 128 << 20} {
		p, w, b := fixture(t)
		c := capabilities()
		c.Artifacts.MaximumArtifactSize = providerMax
		p, err := New(w, c)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := p.artifact(w.artifact)
		if providerMax < uint64(len(w.body)) {
			if _, err = p.artifact(w.artifact); !errors.Is(err, history.ErrLimit) {
				t.Fatal(err)
			}
		} else {
			a.Length = history.MaximumArtifactBytes + 1
			if _, err = p.ReadArtifact(t.Context(), b, a); !errors.Is(err, history.ErrLimit) {
				t.Fatal(err)
			}
		}
	}
}
func TestVerifiedArtifactChunksAndCorruptionNeverExposePartialBytes(t *testing.T) {
	for _, maximum := range []uint32{1024, 1 << 20} {
		p, w, b := fixture(t)
		c := capabilities()
		c.Artifacts.MaximumChunkSize = maximum
		var err error
		p, err = New(w, c)
		if err != nil {
			t.Fatal(err)
		}
		a, err := p.artifact(w.artifact)
		if err != nil {
			t.Fatal(err)
		}
		data, err := p.ReadArtifact(t.Context(), b, a)
		if err != nil || string(data) != string(w.body) {
			t.Fatal(len(data), err)
		}
		for _, r := range w.chunks {
			if r.Length > min(maximum, history.MaximumChunkBytes) || r.Controller == nil || r.ArtifactId != w.artifact.ArtifactId {
				t.Fatal(r)
			}
		}
	}
	for name, corrupt := range map[string]func(*publicv1.ReadArtifactChunkResponse){"offset": func(v *publicv1.ReadArtifactChunkResponse) { v.Offset++ }, "truncated": func(v *publicv1.ReadArtifactChunkResponse) { v.Data = v.Data[:len(v.Data)-1] }, "total": func(v *publicv1.ReadArtifactChunkResponse) { v.TotalByteLength++ }, "digest": func(v *publicv1.ReadArtifactChunkResponse) { v.Data[0] ^= 1 }, "early-eof": func(v *publicv1.ReadArtifactChunkResponse) { v.Eof = true }, "id": func(v *publicv1.ReadArtifactChunkResponse) { v.ArtifactId = "other" }} {
		t.Run(name, func(t *testing.T) {
			p, w, b := fixture(t)
			w.corrupt = corrupt
			a, _ := p.artifact(w.artifact)
			data, err := p.ReadArtifact(t.Context(), b, a)
			if err == nil || data != nil {
				t.Fatal("partial exposed", len(data), err)
			}
		})
	}
	p, w, b := fixture(t)
	a, _ := p.artifact(w.artifact)
	w.metaChange = func(v *publicv1.GetArtifactResponse) { v.Artifact.Sha256 = strings.Repeat("0", 64) }
	if data, err := p.ReadArtifact(t.Context(), b, a); err == nil || data != nil || len(w.chunks) != 0 {
		t.Fatal(data, err)
	}
	p, w, b = fixture(t)
	a, _ = p.artifact(w.artifact)
	ctx, cancel := context.WithCancel(t.Context())
	w.cancel = cancel
	if data, err := p.ReadArtifact(ctx, b, a); !errors.Is(err, context.Canceled) || data != nil {
		t.Fatal(len(data), err)
	}
}
func TestSafeTimelineClosedKindsWireInlineAndAuthorization(t *testing.T) {
	p, w, b := fixture(t)
	if s, err := p.Snapshot(t.Context(), b); err != nil || s.State != "closed" {
		t.Fatal(s, err)
	}
	b.Carrier.ControllerID = "other"
	if _, err := p.Snapshot(t.Context(), b); !errors.Is(err, history.ErrAuthority) {
		t.Fatal(err)
	}
	b.Carrier.ControllerID = "controller"
	inline := strings.Repeat("x", 300<<10)
	w.timeline.Items[0].Content = &publicv1.TimelineItem_InlineText{InlineText: inline}
	page, err := p.Timeline(t.Context(), b, "", 1)
	if err != nil || *page.Items[0].Content.Inline != inline {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*publicv1.TimelineItem){"unknown": func(v *publicv1.TimelineItem) { v.Type = 99 }, "status": func(v *publicv1.TimelineItem) { v.Status = nil }, "wrong-status": func(v *publicv1.TimelineItem) { v.Status = ptr(publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_FINAL) }, "run": func(v *publicv1.TimelineItem) { v.RunId = "foreign" }, "cursor": func(v *publicv1.TimelineItem) { v.Cursor = "02" }, "turn": func(v *publicv1.TimelineItem) { v.TurnId = "" }, "future": func(v *publicv1.TimelineItem) { v.Cursor = "11" }, "absent-original": func(v *publicv1.TimelineItem) { v.Content = nil }, "unknown-field": func(v *publicv1.TimelineItem) { v.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) }} {
		t.Run(name, func(t *testing.T) {
			p, w, b := fixture(t)
			change(w.timeline.Items[0])
			if _, err := p.Timeline(t.Context(), b, "", 1); err == nil {
				t.Fatal("corruption admitted")
			}
		})
	}
	for _, kind := range []publicv1.TimelineItemType{publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_ASSISTANT_RESPONSE_FINAL, publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_OPENED, publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_RESOLVED, publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_TURN_TERMINAL} {
		p, w, b := fixture(t)
		v := w.timeline.Items[0]
		v.Type = kind
		v.Status = ptr(publicv1.TimelineItemStatus(kind))
		if kind == publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_TURN_TERMINAL {
			v.Status = ptr(publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_FAILED)
		}
		if kind >= publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_OPENED {
			v.Content = nil
		}
		if kind == 3 || kind == 4 {
			v.InteractionId = ptr("private-interaction")
			v.InteractionSafeTitle = ptr("Approval")
			v.InteractionKind = ptr(publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT)
			v.InteractionStatus = ptr(publicv1.InteractionStatus_INTERACTION_STATUS_PENDING)
			if kind == 4 {
				v.InteractionStatus = ptr(publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED)
			}
		}
		if page, err := p.Timeline(t.Context(), b, "", 1); err != nil || len(page.Items) != 1 {
			t.Fatal(kind, page, err)
		}
	}
}
func TestResultsRetainPrimaryArtifactOwnershipAndRejectForeignMetadata(t *testing.T) {
	p, w, b := fixture(t)
	w.artifact.Kind = publicv1.ArtifactKind_ARTIFACT_KIND_FINAL_RESPONSE
	item := &publicv1.OrchestratedSessionResult{ResultId: "result", TaskId: "task", SpecialistRun: &publicv1.RunRef{RunId: "child", Workspace: ref(b).Workspace}, ArtifactOwner: ref(b), SpecialistRole: "reviewer", PublicationOrder: 1, PublishedAt: timestamppb.New(time.Unix(1, 0)), Format: publicv1.OrchestratedResultFormat_ORCHESTRATED_RESULT_FORMAT_UTF8_TEXT, ByteLength: w.artifact.ByteLength, Sha256: w.artifact.Sha256, Artifact: w.artifact}
	w.results.CapturedPublicationHead = 1
	w.results.Items = []*publicv1.OrchestratedSessionResult{item}
	if got, err := p.Results(t.Context(), b, "", 1); err != nil || len(got.Items) != 1 {
		t.Fatal(got, err)
	}
	for _, change := range []func(*publicv1.OrchestratedSessionResult){func(v *publicv1.OrchestratedSessionResult) { v.ArtifactOwner = v.SpecialistRun }, func(v *publicv1.OrchestratedSessionResult) { v.ByteLength++ }, func(v *publicv1.OrchestratedSessionResult) { v.PublicationOrder = 2 }, func(v *publicv1.OrchestratedSessionResult) { v.Format = 0 }, func(v *publicv1.OrchestratedSessionResult) { v.SpecialistRole = strings.Repeat("x", 513) }} {
		v := proto.Clone(item).(*publicv1.OrchestratedSessionResult)
		change(v)
		w.results.Items = []*publicv1.OrchestratedSessionResult{v}
		if _, err := p.Results(t.Context(), b, "", 1); err == nil {
			t.Fatal("result corruption admitted")
		}
	}
}

func TestTimelineInteractionStatusMatchesAppendOnlyEvent(t *testing.T) {
	for _, kind := range []publicv1.TimelineItemType{publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_OPENED, publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_RESOLVED} {
		for _, state := range []publicv1.InteractionStatus{publicv1.InteractionStatus_INTERACTION_STATUS_PENDING, publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED, publicv1.InteractionStatus_INTERACTION_STATUS_STALE} {
			p, w, b := fixture(t)
			v := w.timeline.Items[0]
			v.Type = kind
			v.Status = ptr(publicv1.TimelineItemStatus(kind))
			v.Content = nil
			v.InteractionId = ptr("interaction")
			v.InteractionKind = ptr(publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT)
			v.InteractionStatus = ptr(state)
			v.InteractionSafeTitle = ptr("Approval")
			wantValid := kind == publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_OPENED && state == publicv1.InteractionStatus_INTERACTION_STATUS_PENDING || kind == publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_RESOLVED && state != publicv1.InteractionStatus_INTERACTION_STATUS_PENDING
			_, err := p.Timeline(t.Context(), b, "", 1)
			if wantValid && err != nil || !wantValid && !errors.Is(err, history.ErrBlocked) {
				t.Fatalf("kind %v state %v: %v", kind, state, err)
			}
			v.InteractionSafeTitle = ptr("")
			if _, err = p.Timeline(t.Context(), b, "", 1); !errors.Is(err, history.ErrBlocked) {
				t.Fatal("empty safe title admitted", err)
			}
		}
	}
}

func TestReleasedImageMetadataUsesZeroBasedOrdinals(t *testing.T) {
	p, w, b := fixture(t)
	w.timeline.Items[0].Images = []*publicv1.ImageInputMetadata{
		{Ordinal: 0, Detail: publicv1.ImageDetail_IMAGE_DETAIL_AUTO, MediaType: "image/png", ByteLength: 67, Sha256: strings.Repeat("a", 64)},
		{Ordinal: 1, Detail: publicv1.ImageDetail_IMAGE_DETAIL_HIGH, MediaType: "image/jpeg", ByteLength: 91, Sha256: strings.Repeat("b", 64)},
	}
	page, err := p.Timeline(t.Context(), b, "", 1)
	if err != nil || len(page.Items[0].Images) != 2 || page.Items[0].Images[0].Ordinal != 0 || page.Items[0].Images[1].Ordinal != 1 || page.Items[0].Images[0].SHA256 != strings.Repeat("a", 64) {
		t.Fatal(page, err)
	}
	for _, ordinal := range []uint32{0, 2} {
		w.timeline.Items[0].Images[1].Ordinal = ordinal
		if _, err = p.Timeline(t.Context(), b, "", 1); err == nil {
			t.Fatal("duplicate or missing image ordinal admitted")
		}
	}
}
