// Package contractprovider projects checked provider messages into Gul cards.
package contractprovider

import (
	"context"
	"path/filepath"
	"strings"
	"unicode/utf8"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/interaction"
	"github.com/rootkernel/gul/internal/observation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Port interface {
	GetRun(context.Context, *publicv1.GetRunRequest) (*publicv1.GetRunResponse, error)
	ListPendingInteractions(context.Context, *publicv1.ListPendingInteractionsRequest) (*publicv1.ListPendingInteractionsResponse, error)
	GetControllerInteraction(context.Context, *publicv1.GetControllerInteractionRequest) (*publicv1.GetControllerInteractionResponse, error)
	ResolveInteraction(context.Context, *publicv1.ResolveInteractionRequest) (*publicv1.ResolveInteractionResponse, error)
	GetArtifact(context.Context, *publicv1.GetArtifactRequest) (*publicv1.GetArtifactResponse, error)
	ReadArtifactChunk(context.Context, *publicv1.ReadArtifactChunkRequest) (*publicv1.ReadArtifactChunkResponse, error)
}
type Provider struct {
	port                        Port
	responseLimit, payloadLimit int
	kinds                       map[publicv1.InteractionKind]publicv1.InteractionSupport
}

// New consumes negotiated capabilities. Transport assembly must disable retries
// and hedging, as required by the pinned mutation client policy.
func New(port Port, caps *publicv1.InteractionCapabilities) (*Provider, error) {
	if port == nil || caps == nil || !known(caps.ProtoReflect()) || caps.MaximumResponseBytes == 0 || caps.MaximumSafePayloadBytes == 0 {
		return nil, interaction.ErrBlocked
	}
	p := &Provider{port: port, responseLimit: min(int(caps.MaximumResponseBytes), interaction.MaximumResponseBytes), payloadLimit: min(int(caps.MaximumSafePayloadBytes), interaction.GUL_MAX_SAFE_INTERACTION_PAYLOAD_BYTES), kinds: map[publicv1.InteractionKind]publicv1.InteractionSupport{}}
	advertised := map[publicv1.InteractionKind]bool{}
	for _, k := range caps.KnownKinds {
		if classify(k) == 0 || advertised[k] {
			return nil, interaction.ErrBlocked
		}
		advertised[k] = true
	}
	for _, item := range caps.Items {
		if item == nil || !advertised[item.Kind] || p.kinds[item.Kind] != 0 || item.Support == 0 {
			return nil, interaction.ErrBlocked
		}
		if classify(item.Kind) == interaction.Unsupported && item.Support == publicv1.InteractionSupport_INTERACTION_SUPPORT_SUPPORTED {
			return nil, interaction.ErrBlocked
		}
		p.kinds[item.Kind] = item.Support
	}
	if len(p.kinds) != len(advertised) || len(advertised) == 0 {
		return nil, interaction.ErrBlocked
	}
	return p, nil
}
func (p *Provider) ResponseLimit() int { return p.responseLimit }
func ref(b interaction.Bound) *publicv1.RunRef {
	return &publicv1.RunRef{RunId: b.Binding.RunID, Workspace: &publicv1.WorkspaceRef{AbsolutePath: b.Workspace.CanonicalRoot, ExpectedWorkspaceId: b.Workspace.ProviderID}}
}
func carrier(b interaction.Bound) *publicv1.ControllerCarrierRef {
	return &publicv1.ControllerCarrierRef{AbsoluteFilePath: b.Carrier.AbsolutePath, ExpectedControllerId: b.Carrier.ControllerID, ExpectedControllerGeneration: b.Carrier.Generation}
}
func stamp(s *publicv1.ProjectionStamp) observation.Stamp {
	return observation.Stamp{Head: observation.Cursor(s.GetCapturedHeadCursor()), Run: s.GetRunStateRevision(), Writer: s.GetWriterStateRevision(), Interaction: s.GetInteractionStateRevision()}
}
func classify(k publicv1.InteractionKind) interaction.Kind {
	switch k {
	case publicv1.InteractionKind_INTERACTION_KIND_COMMAND_EXECUTION_APPROVAL:
		return interaction.CommandApproval
	case publicv1.InteractionKind_INTERACTION_KIND_FILE_CHANGE_APPROVAL:
		return interaction.FileApproval
	case publicv1.InteractionKind_INTERACTION_KIND_USER_INPUT:
		return interaction.UserInput
	case publicv1.InteractionKind_INTERACTION_KIND_PERMISSION_REQUEST, publicv1.InteractionKind_INTERACTION_KIND_MCP_ELICITATION, publicv1.InteractionKind_INTERACTION_KIND_CONNECTOR_APPROVAL, publicv1.InteractionKind_INTERACTION_KIND_UNSUPPORTED_REQUEST:
		return interaction.Unsupported
	default:
		return 0
	}
}
func (p *Provider) summary(s *publicv1.InteractionSummary, b interaction.Bound) (interaction.Summary, error) {
	if s == nil || !known(s.ProtoReflect()) || s.InteractionId == "" || len(s.InteractionId) > 256 || s.RunId != b.Binding.RunID || classify(s.Kind) == 0 || p.kinds[s.Kind] == 0 || p.kinds[s.Kind] == publicv1.InteractionSupport_INTERACTION_SUPPORT_UNAVAILABLE || s.ControllerKind == 0 || s.CreatedAt == nil || !s.CreatedAt.IsValid() || s.StateRevision == 0 || len(s.SafeTitle) > 512 {
		return interaction.Summary{}, interaction.ErrBlocked
	}
	out := interaction.Summary{ID: s.InteractionId, Kind: classify(s.Kind), CreatedAt: s.CreatedAt.AsTime(), Protected: s.ContainsProtectedInput, RequiresUser: s.RequiresUserEscalation}
	switch s.Status {
	case publicv1.InteractionStatus_INTERACTION_STATUS_PENDING:
		out.Status = interaction.Pending
	case publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED:
		out.Status = interaction.Resolved
	case publicv1.InteractionStatus_INTERACTION_STATUS_STALE:
		out.Status = interaction.Stale
	default:
		return out, interaction.ErrBlocked
	}
	if s.ExpiresAt != nil {
		if !s.ExpiresAt.IsValid() {
			return out, interaction.ErrBlocked
		}
		v := s.ExpiresAt.AsTime()
		out.ExpiresAt = &v
	}
	if s.ResolvedAt != nil {
		if !s.ResolvedAt.IsValid() {
			return out, interaction.ErrBlocked
		}
		v := s.ResolvedAt.AsTime()
		out.ResolvedAt = &v
	}
	if out.RequiresUser != (out.Status == interaction.Pending && out.Kind != interaction.Unsupported) {
		return out, interaction.ErrBlocked
	}
	return out, nil
}
func (p *Provider) pending(ctx context.Context, b interaction.Bound) (interaction.PendingState, *publicv1.ListPendingInteractionsResponse, error) {
	response, err := p.port.ListPendingInteractions(ctx, &publicv1.ListPendingInteractionsRequest{Run: ref(b)})
	if err != nil {
		return interaction.PendingState{}, nil, interaction.ErrUnavailable
	}
	if response == nil || len(response.Items) > 256 || proto.Size(response) > 1024*1024 || !known(response.ProtoReflect()) || !stamp(response.Stamp).Valid() {
		return interaction.PendingState{}, nil, interaction.ErrBlocked
	}
	out := interaction.PendingState{Stamp: stamp(response.Stamp)}
	seen := map[string]bool{}
	for _, item := range response.Items {
		summary, err := p.summary(item, b)
		if err != nil || summary.Status != interaction.Pending || seen[summary.ID] || item.StateRevision > out.Stamp.Interaction {
			return interaction.PendingState{}, nil, interaction.ErrBlocked
		}
		seen[summary.ID] = true
		out.Items = append(out.Items, summary)
	}
	return out, response, nil
}
func (p *Provider) Pending(ctx context.Context, b interaction.Bound) (interaction.PendingState, error) {
	out, _, err := p.pending(ctx, b)
	return out, err
}
func (p *Provider) run(ctx context.Context, b interaction.Bound, authorized bool) (observation.Stamp, error) {
	response, err := p.port.GetRun(ctx, &publicv1.GetRunRequest{Run: ref(b)})
	if err != nil {
		return observation.Stamp{}, interaction.ErrUnavailable
	}
	r := response.GetRun()
	if r == nil || r.RunId != b.Binding.RunID || r.WorkspaceId != b.Workspace.ProviderID || !known(r.ProtoReflect()) || !stamp(r.Stamp).Valid() || r.StateRevision != r.Stamp.RunStateRevision {
		return observation.Stamp{}, interaction.ErrBlocked
	}
	if authorized && (b.Carrier.ControllerID == "" || r.GetController().GetControllerId() != b.Carrier.ControllerID || r.GetController().GetGeneration() != b.Carrier.Generation || r.ControlMode != publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE) {
		return observation.Stamp{}, interaction.ErrAuthority
	}
	return stamp(r.Stamp), nil
}
func (p *Provider) Observe(ctx context.Context, b interaction.Bound) (interaction.PendingState, error) {
	run, err := p.run(ctx, b, false)
	if err != nil {
		return interaction.PendingState{}, err
	}
	pending, err := p.Pending(ctx, b)
	if err != nil {
		return pending, err
	}
	if run != pending.Stamp {
		return interaction.PendingState{}, interaction.ErrBlocked
	}
	return pending, nil
}
func (p *Provider) Card(ctx context.Context, b interaction.Bound, id string) (interaction.Card, error) {
	run, err := p.run(ctx, b, true)
	if err != nil {
		return interaction.Card{}, err
	}
	pending, raw, err := p.pending(ctx, b)
	if err != nil {
		return interaction.Card{}, err
	}
	response, err := p.port.GetControllerInteraction(ctx, &publicv1.GetControllerInteractionRequest{Run: ref(b), Controller: carrier(b), InteractionId: id})
	if err != nil {
		return interaction.Card{}, interaction.ErrUnavailable
	}
	detail := response.GetInteraction()
	if detail == nil || detail.GetSummary().GetInteractionId() != id || !known(detail.ProtoReflect()) || stamp(detail.Stamp) != pending.Stamp || run != pending.Stamp || detail.GetSummary().GetStateRevision() > pending.Stamp.Interaction {
		return interaction.Card{}, interaction.ErrBlocked
	}
	summary, err := p.summary(detail.Summary, b)
	if err != nil {
		return interaction.Card{}, err
	}
	found := false
	for _, candidate := range raw.Items {
		if candidate.InteractionId == id {
			if !proto.Equal(candidate, detail.Summary) {
				return interaction.Card{}, interaction.ErrBlocked
			}
			found = true
		}
	}
	if (summary.Status == interaction.Pending) != found {
		return interaction.Card{}, interaction.ErrBlocked
	}
	return p.card(ctx, b, summary, detail)
}
func (p *Provider) Resolve(ctx context.Context, b interaction.Bound, id, key string, body []byte) (interaction.Resolution, error) {
	if len(body) == 0 || len(body) > p.responseLimit {
		return interaction.Resolution{}, interaction.ErrInvalid
	}
	if _, err := p.run(ctx, b, true); err != nil {
		return interaction.Resolution{}, err
	}
	response, err := p.port.ResolveInteraction(ctx, &publicv1.ResolveInteractionRequest{Run: ref(b), Controller: carrier(b), InteractionId: id, IdempotencyKey: key, ResponseJson: body})
	if err != nil {
		return interaction.Resolution{}, interaction.ErrUnavailable
	}
	if response == nil || !known(response.ProtoReflect()) || response.InteractionId != id || response.Status != publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED || response.ResolutionReceipt == "" || len(response.ResolutionReceipt) > 256 {
		return interaction.Resolution{}, interaction.ErrBlocked
	}
	return interaction.Resolution{Status: interaction.Resolved, Receipt: response.ResolutionReceipt}, nil
}
func safePath(p *publicv1.PathProjection, root string) (string, error) {
	value, ok := p.GetValue().(*publicv1.PathProjection_Utf8Path)
	if !ok || value.Utf8Path == "" || len(value.Utf8Path) > 4096 || !utf8.ValidString(value.Utf8Path) || strings.ContainsAny(value.Utf8Path, "\x00\\") {
		return "", interaction.ErrPath
	}
	path := value.Utf8Path
	if filepath.IsAbs(path) {
		var err error
		path, err = filepath.Rel(root, path)
		if err != nil {
			return "", interaction.ErrPath
		}
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", interaction.ErrPath
	}
	return clean, nil
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
