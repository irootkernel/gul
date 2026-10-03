// Package contractprovider validates the pinned Controller-safe read contract.
package contractprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/observation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Port interface {
	GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error)
	ListRunTimelineItems(context.Context, *publicv1.ListRunTimelineItemsRequest) (*publicv1.ListRunTimelineItemsResponse, error)
	ListOrchestratedSessionResults(context.Context, *publicv1.ListOrchestratedSessionResultsRequest) (*publicv1.ListOrchestratedSessionResultsResponse, error)
	GetArtifact(context.Context, *publicv1.GetArtifactRequest) (*publicv1.GetArtifactResponse, error)
	ReadArtifactChunk(context.Context, *publicv1.ReadArtifactChunkRequest) (*publicv1.ReadArtifactChunkResponse, error)
}
type Provider struct {
	port                Port
	maxArtifact         uint64
	maxChunk, maxInline uint32
	visibility          map[publicv1.ArtifactVisibility]bool
}

func New(port Port, caps *publicv1.GetCapabilitiesResponse) (*Provider, error) {
	if port == nil || caps == nil || !known(caps.ProtoReflect()) || caps.Protocol == nil || caps.Protocol.TimelineProtocolVersion != 1 || caps.Features == nil || !caps.Features.ArtifactRetrieval || !caps.Features.ControllerTimeline || caps.Artifacts == nil {
		return nil, history.ErrBlocked
	}
	a := caps.Artifacts
	if a.MaximumArtifactSize == 0 || a.MaximumChunkSize == 0 || a.MaximumInlineResponseBytes == 0 || !a.DigestVerificationRequired || !a.ExactByteLengthReported {
		return nil, history.ErrBlocked
	}
	methods := map[string]bool{}
	for _, m := range caps.SupportedMethods {
		if methods[m] {
			return nil, history.ErrBlocked
		}
		methods[m] = true
	}
	for _, m := range []string{"RunService.GetRun", "ObservationService.ListRunTimelineItems", "OrchestrationService.ListOrchestratedSessionResults", "ArtifactService.GetArtifact", "ArtifactService.ReadArtifactChunk"} {
		if !methods[m] {
			return nil, history.ErrBlocked
		}
	}
	p := &Provider{port: port, maxArtifact: min(a.MaximumArtifactSize, history.MaximumArtifactBytes), maxChunk: min(a.MaximumChunkSize, history.MaximumChunkBytes), maxInline: a.MaximumInlineResponseBytes, visibility: map[publicv1.ArtifactVisibility]bool{}}
	for _, v := range a.VisibilityClasses {
		if v == 0 || p.visibility[v] {
			return nil, history.ErrBlocked
		}
		p.visibility[v] = true
	}
	if !p.visibility[publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY] {
		return nil, history.ErrBlocked
	}
	return p, nil
}
func ref(b history.Bound) *publicv1.RunRef {
	return &publicv1.RunRef{RunId: b.Binding.RunID, Workspace: &publicv1.WorkspaceRef{AbsolutePath: b.Workspace.CanonicalRoot, ExpectedWorkspaceId: b.Workspace.ProviderID}}
}
func carrier(b history.Bound) *publicv1.ControllerCarrierRef {
	return &publicv1.ControllerCarrierRef{AbsoluteFilePath: b.Carrier.AbsolutePath, ExpectedControllerId: b.Carrier.ControllerID, ExpectedControllerGeneration: b.Carrier.Generation}
}
func stamp(s *publicv1.ProjectionStamp) observation.Stamp {
	return observation.Stamp{Head: observation.Cursor(s.GetCapturedHeadCursor()), Run: s.GetRunStateRevision(), Writer: s.GetWriterStateRevision(), Interaction: s.GetInteractionStateRevision()}
}
func failure(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return history.ErrUnavailable
}
func id(s string) bool { return s != "" && len(s) <= 1024 && utf8.ValidString(s) }
func sha(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func (p *Provider) Snapshot(ctx context.Context, b history.Bound) (history.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return history.Snapshot{}, err
	}
	v, err := p.port.GetRun(ctx, &publicv1.GetRunRequest{Run: ref(b)})
	if err != nil {
		return history.Snapshot{}, failure(ctx)
	}
	if v == nil || v.Run == nil || !known(v.ProtoReflect()) {
		return history.Snapshot{}, history.ErrBlocked
	}
	r := v.Run
	if r.RunId != b.Binding.RunID || r.WorkspaceId != b.Workspace.ProviderID || r.Configuration == nil || r.Configuration.Parent != nil || r.ControlMode != publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE || r.Lifecycle == 0 || r.Controller == nil || r.Controller.Kind != publicv1.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT || !stamp(r.Stamp).Valid() || r.StateRevision != r.Stamp.RunStateRevision || r.EventCursor != r.Stamp.CapturedHeadCursor {
		return history.Snapshot{}, history.ErrBlocked
	}
	if r.Controller.ControllerId != b.Carrier.ControllerID || r.Controller.Generation != b.Carrier.Generation {
		return history.Snapshot{}, history.ErrAuthority
	}
	out := history.Snapshot{Stamp: stamp(r.Stamp), State: strings.ToLower(strings.TrimPrefix(r.Lifecycle.String(), "RUN_LIFECYCLE_"))}
	if r.LastFinalResponse != nil {
		switch final := r.LastFinalResponse.Value.(type) {
		case *publicv1.FinalResponse_InlineUtf8:
			if !utf8.ValidString(final.InlineUtf8) || len(final.InlineUtf8) > int(p.maxInline) {
				return history.Snapshot{}, history.ErrLimit
			}
			out.Final = &history.Content{Inline: &final.InlineUtf8}
		case *publicv1.FinalResponse_Artifact:
			a, err := p.artifact(final.Artifact)
			if err != nil {
				return history.Snapshot{}, err
			}
			if a.Kind != uint32(publicv1.ArtifactKind_ARTIFACT_KIND_FINAL_RESPONSE) {
				return history.Snapshot{}, history.ErrBlocked
			}
			out.Final = &history.Content{Artifact: &a}
		case *publicv1.FinalResponse_Unavailable:
			out.FinalUnavailable = true
		default:
			return history.Snapshot{}, history.ErrBlocked
		}
	}
	return out, nil
}
func (p *Provider) artifact(a *publicv1.ArtifactRef) (history.Artifact, error) {
	if a == nil || !known(a.ProtoReflect()) || !id(a.ArtifactId) || a.Kind == 0 || !p.visibility[a.Visibility] || !sha(a.Sha256) {
		return history.Artifact{}, history.ErrBlocked
	}
	if a.ByteLength > p.maxArtifact {
		return history.Artifact{}, history.ErrLimit
	}
	switch a.MediaType {
	case "text/plain", "text/plain; charset=utf-8", "text/markdown", "text/markdown; charset=utf-8", "text/x-diff":
	default:
		return history.Artifact{}, history.ErrBlocked
	}
	return history.Artifact{ID: a.ArtifactId, Kind: uint32(a.Kind), Visibility: uint32(a.Visibility), MediaType: a.MediaType, Length: a.ByteLength, SHA256: a.Sha256}, nil
}
func wire(a history.Artifact) *publicv1.ArtifactRef {
	return &publicv1.ArtifactRef{ArtifactId: a.ID, Kind: publicv1.ArtifactKind(a.Kind), Visibility: publicv1.ArtifactVisibility(a.Visibility), MediaType: a.MediaType, ByteLength: a.Length, Sha256: a.SHA256}
}
func (p *Provider) ReadArtifact(ctx context.Context, b history.Bound, a history.Artifact) ([]byte, error) {
	if _, err := p.artifact(wire(a)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	meta, err := p.port.GetArtifact(ctx, &publicv1.GetArtifactRequest{Run: ref(b), Controller: carrier(b), ArtifactId: a.ID})
	if err != nil {
		return nil, failure(ctx)
	}
	if meta == nil || !known(meta.ProtoReflect()) || !proto.Equal(meta.Artifact, wire(a)) || meta.MaximumChunkSize == 0 {
		return nil, history.ErrBlocked
	}
	data := make([]byte, 0, int(a.Length))
	valid := false
	defer func() {
		if !valid {
			clear(data)
		}
	}()
	// Even an empty artifact receives a metadata/integrity check; no chunk is needed.
	for uint64(len(data)) < a.Length {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		length := uint32(min(uint64(min(p.maxChunk, meta.MaximumChunkSize)), a.Length-uint64(len(data))))
		v, err := p.port.ReadArtifactChunk(ctx, &publicv1.ReadArtifactChunkRequest{Run: ref(b), Controller: carrier(b), ArtifactId: a.ID, Offset: uint64(len(data)), Length: length})
		if err != nil {
			return nil, failure(ctx)
		}
		if v == nil || !known(v.ProtoReflect()) || v.ArtifactId != a.ID || v.Offset != uint64(len(data)) || v.Length != length || len(v.Data) != int(length) || v.TotalByteLength != a.Length || v.Sha256 != a.SHA256 || v.Eof != (uint64(len(data))+uint64(length) == a.Length) {
			return nil, history.ErrBlocked
		}
		data = append(data, v.Data...)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != a.SHA256 || !utf8.Valid(data) {
		return nil, history.ErrBlocked
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	valid = true
	return data, nil
}
func (p *Provider) Timeline(ctx context.Context, b history.Bound, after string, limit uint32) (history.Timeline, error) {
	if limit == 0 || limit > 100 || after != "" && !observation.Cursor(after).Valid() {
		return history.Timeline{}, history.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return history.Timeline{}, err
	}
	v, err := p.port.ListRunTimelineItems(ctx, &publicv1.ListRunTimelineItemsRequest{Run: ref(b), Controller: carrier(b), AfterCursor: after, Limit: limit, TimelineVersion: 1})
	if err != nil {
		return history.Timeline{}, failure(ctx)
	}
	if v == nil || !known(v.ProtoReflect()) || len(v.Items) > int(limit) || proto.Size(v) > history.MaximumArtifactBytes+1<<20 || !stamp(v.Stamp).Valid() || v.CapturedHeadCursor != v.Stamp.CapturedHeadCursor {
		return history.Timeline{}, history.ErrBlocked
	}
	out := history.Timeline{Head: v.CapturedHeadCursor, Next: v.GetNextAfterCursor(), Stamp: stamp(v.Stamp)}
	last := observation.Cursor(after)
	if after == "" {
		last = "0"
	}
	seen := map[string]bool{}
	for _, item := range v.Items {
		if item == nil || item.RunId != b.Binding.RunID || !id(item.TurnId) || item.OccurredAt == nil || !item.OccurredAt.IsValid() || !observation.Cursor(item.Cursor).Valid() || observation.Cursor(item.Cursor).Compare(last) <= 0 || observation.Cursor(item.Cursor).Compare(out.Stamp.Head) > 0 || item.Status == nil || item.GetStatus() == 0 {
			return history.Timeline{}, history.ErrBlocked
		}
		if item.ProviderItemId != nil && (!id(*item.ProviderItemId) || seen[*item.ProviderItemId]) {
			return history.Timeline{}, history.ErrBlocked
		}
		if item.ProviderItemId != nil {
			seen[*item.ProviderItemId] = true
		}
		e, err := p.entry(item)
		if err != nil {
			return history.Timeline{}, err
		}
		out.Items = append(out.Items, e)
		last = observation.Cursor(item.Cursor)
	}
	if v.NextAfterCursor != nil && (out.Next == "" || len(out.Items) == 0 || out.Next != string(last) || last.Compare(out.Stamp.Head) >= 0) {
		return history.Timeline{}, history.ErrBlocked
	}
	return out, nil
}
func (p *Provider) entry(v *publicv1.TimelineItem) (history.SourceEntry, error) {
	e := history.SourceEntry{Kind: history.Kind(v.Type), Cursor: v.Cursor, ProviderID: v.GetProviderItemId(), TurnID: v.TurnId, At: v.OccurredAt.AsTime(), Status: strings.ToLower(strings.TrimPrefix(v.GetStatus().String(), "TIMELINE_ITEM_STATUS_"))}
	switch v.Type {
	case publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_USER_INPUT_ACCEPTED:
		if v.GetStatus() != publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_ACCEPTED {
			return e, history.ErrBlocked
		}
	case publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_ASSISTANT_RESPONSE_FINAL:
		if v.GetStatus() != publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_FINAL {
			return e, history.ErrBlocked
		}
	case publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_OPENED, publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_RESOLVED:
		want := publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_OPENED
		if v.Type == publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_RESOLVED {
			want = publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_RESOLVED
		}
		if v.GetStatus() != want || !id(v.GetInteractionId()) || v.GetInteractionKind() == 0 || v.GetInteractionStatus() == 0 || v.GetInteractionSafeTitle() == "" || len(v.GetInteractionSafeTitle()) > 512 {
			return e, history.ErrBlocked
		}
		state := v.GetInteractionStatus()
		if v.Type == publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_OPENED && state != publicv1.InteractionStatus_INTERACTION_STATUS_PENDING || v.Type == publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_INTERACTION_RESOLVED && state != publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED && state != publicv1.InteractionStatus_INTERACTION_STATUS_STALE {
			return e, history.ErrBlocked
		}
		e.InteractionID = v.GetInteractionId()
		e.InteractionKind = strings.ToLower(strings.TrimPrefix(v.GetInteractionKind().String(), "INTERACTION_KIND_"))
		e.InteractionStatus = strings.ToLower(strings.TrimPrefix(v.GetInteractionStatus().String(), "INTERACTION_STATUS_"))
		e.Title = v.GetInteractionSafeTitle()
	case publicv1.TimelineItemType_TIMELINE_ITEM_TYPE_TURN_TERMINAL:
		if v.GetStatus() < publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_COMPLETED || v.GetStatus() > publicv1.TimelineItemStatus_TIMELINE_ITEM_STATUS_OUTCOME_UNKNOWN {
			return e, history.ErrBlocked
		}
	default:
		return e, history.ErrBlocked
	}
	if e.Kind == history.Human || e.Kind == history.Assistant {
		switch c := v.Content.(type) {
		case *publicv1.TimelineItem_InlineText:
			if !utf8.ValidString(c.InlineText) || e.Kind == history.Assistant && len(c.InlineText) > int(p.maxInline) || len(c.InlineText) > history.MaximumArtifactBytes || len(c.InlineText) > history.MaximumInlineBytes && uint64(len(c.InlineText)) > p.maxArtifact {
				return e, history.ErrLimit
			}
			e.Content.Inline = &c.InlineText
		case *publicv1.TimelineItem_Artifact:
			a, err := p.artifact(c.Artifact)
			if err != nil {
				return e, err
			}
			want := uint32(publicv1.ArtifactKind_ARTIFACT_KIND_USER_INPUT)
			if e.Kind == history.Assistant {
				want = uint32(publicv1.ArtifactKind_ARTIFACT_KIND_FINAL_RESPONSE)
			}
			if a.Kind != want {
				return e, history.ErrBlocked
			}
			e.Content.Artifact = &a
		default:
			return e, history.ErrBlocked
		}
	} else if v.Content != nil {
		return e, history.ErrBlocked
	}
	if e.Kind != history.Human && len(v.Images) > 0 {
		return e, history.ErrBlocked
	}
	if len(v.Images) > 16 {
		return e, history.ErrLimit
	}
	for i, img := range v.Images {
		if img == nil || img.Ordinal != uint32(i) || img.Detail == 0 || !sha(img.Sha256) || img.ByteLength == 0 || len(img.MediaType) > 128 || !strings.HasPrefix(img.MediaType, "image/") {
			return e, history.ErrBlocked
		}
		e.Images = append(e.Images, history.Image{Ordinal: img.Ordinal, Detail: strings.ToLower(strings.TrimPrefix(img.Detail.String(), "IMAGE_DETAIL_")), MediaType: img.MediaType, Length: img.ByteLength, SHA256: img.Sha256})
	}
	return e, nil
}
func (p *Provider) Results(ctx context.Context, b history.Bound, cursor string, limit uint32) (history.Results, error) {
	if limit == 0 || limit > 100 || len(cursor) > 4096 {
		return history.Results{}, history.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return history.Results{}, err
	}
	request := &publicv1.ListOrchestratedSessionResultsRequest{RootRun: ref(b), Controller: carrier(b), Limit: limit, ProjectionVersion: 1}
	if cursor != "" {
		request.PageCursor = &cursor
	}
	v, err := p.port.ListOrchestratedSessionResults(ctx, request)
	if err != nil {
		return history.Results{}, failure(ctx)
	}
	if v == nil || !known(v.ProtoReflect()) || v.SourceRevision == 0 || v.CapturedAt == nil || !v.CapturedAt.IsValid() || len(v.Items) > int(limit) || proto.Size(v) > 1<<20 {
		return history.Results{}, history.ErrBlocked
	}
	out := history.Results{Head: v.CapturedPublicationHead, Revision: v.SourceRevision, CapturedAt: v.CapturedAt.AsTime(), Next: v.GetNextPageCursor()}
	seen := map[string]bool{}
	var order uint64
	for _, item := range v.Items {
		if item == nil || !id(item.ResultId) || seen[item.ResultId] || !id(item.TaskId) || item.SpecialistRun == nil || !id(item.SpecialistRun.RunId) || item.SpecialistRun.RunId == b.Binding.RunID || !proto.Equal(item.SpecialistRun.Workspace, ref(b).Workspace) || !proto.Equal(item.ArtifactOwner, ref(b)) || item.SpecialistRole == "" || len(item.SpecialistRole) > 512 || item.PublicationOrder <= order || item.PublicationOrder > v.CapturedPublicationHead || item.PublishedAt == nil || !item.PublishedAt.IsValid() || item.Format != publicv1.OrchestratedResultFormat_ORCHESTRATED_RESULT_FORMAT_UTF8_TEXT {
			return history.Results{}, history.ErrBlocked
		}
		a, err := p.artifact(item.Artifact)
		if err != nil {
			return history.Results{}, err
		}
		if a.Kind != uint32(publicv1.ArtifactKind_ARTIFACT_KIND_FINAL_RESPONSE) || a.Length != item.ByteLength || a.SHA256 != item.Sha256 {
			return history.Results{}, history.ErrBlocked
		}
		seen[item.ResultId] = true
		order = item.PublicationOrder
		out.Items = append(out.Items, history.SourceResult{ID: item.ResultId, TaskID: item.TaskId, SpecialistID: item.SpecialistRun.RunId, Role: item.SpecialistRole, Order: order, At: item.PublishedAt.AsTime(), Artifact: a})
	}
	if v.NextPageCursor != nil && (out.Next == "" || len(out.Next) > 4096 || out.Next == cursor || len(out.Items) == 0 || order >= out.Head) {
		return history.Results{}, history.ErrBlocked
	}
	return out, nil
}

func known(m protoreflect.Message) bool {
	if !m.IsValid() || len(m.GetUnknown()) != 0 {
		return false
	}
	ok := true
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		check := func(v protoreflect.Value) bool {
			if f.Kind() == protoreflect.EnumKind {
				return f.Enum().Values().ByNumber(v.Enum()) != nil
			}
			if f.Kind() == protoreflect.MessageKind {
				return known(v.Message())
			}
			return true
		}
		if f.IsList() {
			for i := 0; i < v.List().Len(); i++ {
				if !check(v.List().Get(i)) {
					ok = false
					break
				}
			}
		} else {
			ok = check(v)
		}
		return ok
	})
	return ok
}
