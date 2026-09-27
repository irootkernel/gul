package contractprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/interaction"
	"google.golang.org/protobuf/proto"
)

func digest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}
func (p *Provider) card(ctx context.Context, b interaction.Bound, summary interaction.Summary, d *publicv1.ControllerInteraction) (interaction.Card, error) {
	out := interaction.Card{Summary: summary}
	bad := func() (interaction.Card, error) { return interaction.Card{}, interaction.ErrBlocked }
	var payload proto.Message
	switch v := d.Payload.(type) {
	case *publicv1.ControllerInteraction_CommandApproval:
		payload = v.CommandApproval
	case *publicv1.ControllerInteraction_FileChangeApproval:
		payload = v.FileChangeApproval
	case *publicv1.ControllerInteraction_UserInput:
		payload = v.UserInput
	case *publicv1.ControllerInteraction_Unsupported:
		payload = v.Unsupported
	default:
		return bad()
	}
	if payload == nil || !payload.ProtoReflect().IsValid() || proto.Size(payload) > p.payloadLimit {
		return bad()
	}
	expectedSchema := ""
	switch v := d.Payload.(type) {
	case *publicv1.ControllerInteraction_CommandApproval:
		if summary.Kind != interaction.CommandApproval {
			return bad()
		}
		expectedSchema = "dolgorae.interaction.command-approval/v1"
		c := v.CommandApproval
		if c.Title == "" || c.Message == "" || len(c.Title) > 512 || len(c.Message) > 4096 || len(c.GetReason()) > 4096 || len(c.Command) == 0 || len(c.Command) > 256 {
			return bad()
		}
		cwd, err := safePath(c.Cwd, b.Workspace.CanonicalRoot)
		if err != nil {
			return out, err
		}
		for _, arg := range c.Command {
			if len(arg) > 4096 || !utf8.ValidString(arg) {
				return bad()
			}
		}
		out.Command = &interaction.Command{Title: c.Title, Message: c.Message, Reason: c.GetReason(), WorkingDirectory: cwd, Arguments: append([]string(nil), c.Command...)}
	case *publicv1.ControllerInteraction_FileChangeApproval:
		if summary.Kind != interaction.FileApproval {
			return bad()
		}
		expectedSchema = "dolgorae.interaction.file-change-approval/v1"
		f := v.FileChangeApproval
		if f.Title == "" || f.Message == "" || len(f.Title) > 512 || len(f.Message) > 4096 || len(f.GetReason()) > 4096 || !digest(f.SnapshotSha256) || f.SnapshotRevision > 9007199254740991 {
			return bad()
		}
		out.File = &interaction.File{Title: f.Title, Message: f.Message, Reason: f.GetReason()}
		switch representation := f.Representation.(type) {
		case *publicv1.FileChangeApprovalInteraction_InlineChanges:
			if representation.InlineChanges == nil || len(representation.InlineChanges.Items) == 0 || len(representation.InlineChanges.Items) > 4096 {
				return bad()
			}
			total := 0
			for _, c := range representation.InlineChanges.Items {
				if c == nil {
					return bad()
				}
				path, err := safePath(c.Path, b.Workspace.CanonicalRoot)
				if err != nil {
					return out, err
				}
				change := interaction.Change{Path: path, Diff: c.UnifiedDiff}
				total += len(change.Diff)
				if total > 65536 {
					return bad()
				}
				switch c.Kind {
				case publicv1.FileChangeKind_FILE_CHANGE_KIND_ADD:
					change.Kind = "add"
				case publicv1.FileChangeKind_FILE_CHANGE_KIND_UPDATE:
					change.Kind = "update"
				case publicv1.FileChangeKind_FILE_CHANGE_KIND_DELETE:
					change.Kind = "delete"
				default:
					return bad()
				}
				if c.MovePath != nil {
					if change.Kind != "update" {
						return bad()
					}
					change.MovePath, err = safePath(c.MovePath, b.Workspace.CanonicalRoot)
					if err != nil {
						return out, err
					}
				}
				out.File.Changes = append(out.File.Changes, change)
			}
		case *publicv1.FileChangeApprovalInteraction_ChangeArtifact:
			text, err := p.diff(ctx, b, representation.ChangeArtifact)
			if err != nil {
				return out, err
			}
			out.File.VerifiedDiff = text
		default:
			return bad()
		}
	case *publicv1.ControllerInteraction_UserInput:
		if summary.Kind != interaction.UserInput {
			return bad()
		}
		expectedSchema = "dolgorae.interaction.user-input/v1"
		input := v.UserInput
		if len(input.Questions) == 0 || len(input.Questions) > 64 {
			return bad()
		}
		out.Input = &interaction.Input{Blocking: input.IsBlocking}
		seen := map[string]bool{}
		protected := false
		for _, q := range input.Questions {
			if q == nil || q.Id == "" || seen[q.Id] || q.Question == "" || len(q.Id) > 256 || len(q.Header) > 512 || len(q.Question) > 4096 || len(q.GetOptions().GetItems()) > 256 {
				return bad()
			}
			seen[q.Id] = true
			question := interaction.Question{ID: q.Id, Header: q.Header, Prompt: q.Question, AllowsOther: q.AllowsOther, Secret: q.IsSecret}
			protected = protected || q.IsSecret
			labels := map[string]bool{}
			for _, o := range q.GetOptions().GetItems() {
				if o == nil || o.Label == "" || labels[o.Label] || len(o.Label) > 4096 || len(o.Description) > 4096 {
					return bad()
				}
				labels[o.Label] = true
				question.Options = append(question.Options, interaction.Option{Label: o.Label, Description: o.Description})
			}
			out.Input.Questions = append(out.Input.Questions, question)
		}
		if protected != summary.Protected {
			return bad()
		}
	case *publicv1.ControllerInteraction_Unsupported:
		if summary.Kind != interaction.Unsupported || v.Unsupported.OriginalKind != d.Summary.Kind {
			return bad()
		}
		expectedSchema = "dolgorae.interaction.unsupported/v1"
		switch v.Unsupported.Reason {
		case publicv1.UnsupportedInteractionReason_UNSUPPORTED_INTERACTION_REASON_RECOGNIZED_UNSUPPORTED:
			out.UnsupportedReason = "This interaction kind is not supported."
		case publicv1.UnsupportedInteractionReason_UNSUPPORTED_INTERACTION_REASON_UNAVAILABLE:
			out.UnsupportedReason = "This interaction kind is unavailable."
		default:
			return bad()
		}
	}
	if d.ResponseSchemaId != expectedSchema {
		return bad()
	}
	if summary.Kind == interaction.CommandApproval || summary.Kind == interaction.FileApproval {
		if len(d.Decisions) != 3 {
			return bad()
		}
		for i, decision := range d.Decisions {
			if int(decision) != i+1 {
				return bad()
			}
			out.Decisions = append(out.Decisions, interaction.Decision(decision))
		}
	} else if len(d.Decisions) != 0 {
		return bad()
	}
	return out, nil
}

// A file approval cannot become actionable from an unverified artifact reference.
func (p *Provider) diff(ctx context.Context, b interaction.Bound, expected *publicv1.ArtifactRef) (string, error) {
	if expected == nil || expected.ArtifactId == "" || expected.Kind != publicv1.ArtifactKind_ARTIFACT_KIND_FILE_CHANGE_DIFF || expected.Visibility != publicv1.ArtifactVisibility_ARTIFACT_VISIBILITY_CONTROLLER_ONLY || expected.MediaType != "text/x-diff" || expected.ByteLength == 0 || expected.ByteLength > p.maxArtifact || !digest(expected.Sha256) {
		return "", interaction.ErrBlocked
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	meta, err := p.port.GetArtifact(ctx, &publicv1.GetArtifactRequest{Run: ref(b), Controller: carrier(b), ArtifactId: expected.ArtifactId})
	if err != nil {
		return "", interaction.ErrUnavailable
	}
	if meta == nil || !known(meta.ProtoReflect()) || !proto.Equal(meta.Artifact, expected) || meta.MaximumChunkSize == 0 {
		return "", interaction.ErrBlocked
	}
	data := make([]byte, 0, int(expected.ByteLength))
	defer clear(data)
	for uint64(len(data)) < expected.ByteLength {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		length := min(uint64(meta.MaximumChunkSize), uint64(p.maxChunk), expected.ByteLength-uint64(len(data)))
		chunk, err := p.port.ReadArtifactChunk(ctx, &publicv1.ReadArtifactChunkRequest{Run: ref(b), Controller: carrier(b), ArtifactId: expected.ArtifactId, Offset: uint64(len(data)), Length: uint32(length)})
		if err != nil {
			return "", interaction.ErrUnavailable
		}
		if chunk == nil || !known(chunk.ProtoReflect()) || chunk.ArtifactId != expected.ArtifactId || chunk.Offset != uint64(len(data)) || uint64(chunk.Length) != length || len(chunk.Data) != int(length) || chunk.TotalByteLength != expected.ByteLength || chunk.Sha256 != expected.Sha256 || chunk.Eof != (uint64(len(data))+length == expected.ByteLength) {
			return "", interaction.ErrBlocked
		}
		data = append(data, chunk.Data...)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected.Sha256 || !utf8.Valid(data) {
		return "", interaction.ErrBlocked
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(data), nil
}
