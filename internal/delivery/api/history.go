package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"github.com/rootkernel/gul/api/generated/go/gul/v1/gulv1connect"
	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/domain"
	"github.com/rootkernel/gul/internal/history"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *DirectPresentationHandler) historyAccess(ctx context.Context) (string, error) {
	subject, err := h.access(ctx)
	if err != nil {
		return "", err
	}
	if h.History == nil {
		return "", historyError(history.ErrUnavailable)
	}
	return subject, nil
}
func promptEntry(e history.Entry) *gulv1.PromptHistoryItem {
	return &gulv1.PromptHistoryItem{PromptItemId: e.PromptID, Ordinal: e.Ordinal, AcceptedAt: timestamppb.New(e.At), Preview: e.Preview, PreviewTruncated: e.Truncated, ConversationEntryId: e.ID}
}
func original(v history.Original) *gulv1.PromptOriginal {
	if v.Inline != nil {
		return &gulv1.PromptOriginal{Content: &gulv1.PromptOriginal_InlineUtf8{InlineUtf8: *v.Inline}}
	}
	return &gulv1.PromptOriginal{Content: &gulv1.PromptOriginal_ArtifactRef{ArtifactRef: v.ArtifactRef}}
}
func nextToken(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func (h *DirectPresentationHandler) ListPromptHistory(ctx context.Context, r *connect.Request[gulv1.ListPromptHistoryRequest]) (*connect.Response[gulv1.ListPromptHistoryResponse], error) {
	subject, err := h.historyAccess(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.History.ListHistory(ctx, subject, r.Msg.SessionId, r.Msg.GetPageToken(), r.Msg.PageSize)
	if err != nil {
		return nil, historyError(err)
	}
	out := &gulv1.ListPromptHistoryResponse{SnapshotId: v.SnapshotID, NextPageToken: nextToken(v.Next), TraversalComplete: v.Complete, Freshness: gulv1.Freshness_FRESHNESS_FRESH, ObservedAt: timestamppb.New(v.ObservedAt)}
	for _, e := range v.Items {
		out.Items = append(out.Items, promptEntry(e))
	}
	if proto.Size(out) > domain.MaximumPageMetadataBytes {
		return nil, historyError(history.ErrLimit)
	}
	return connect.NewResponse(out), nil
}
func (h *DirectPresentationHandler) GetPromptHistoryItem(ctx context.Context, r *connect.Request[gulv1.GetPromptHistoryItemRequest]) (*connect.Response[gulv1.GetPromptHistoryItemResponse], error) {
	subject, err := h.historyAccess(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.History.GetOriginal(ctx, subject, r.Msg.SessionId, r.Msg.PromptItemId, true)
	if err != nil {
		return nil, historyError(err)
	}
	return connect.NewResponse(&gulv1.GetPromptHistoryItemResponse{PromptItemId: v.Entry.PromptID, Ordinal: v.Entry.Ordinal, AcceptedAt: timestamppb.New(v.Entry.At), ConversationEntryId: v.Entry.ID, Original: original(v)}), nil
}
func conversationEntry(e history.Entry) *gulv1.ConversationEntry {
	status := map[string]gulv1.ConversationStatus{"accepted": gulv1.ConversationStatus_CONVERSATION_STATUS_ACCEPTED, "final": gulv1.ConversationStatus_CONVERSATION_STATUS_FINAL, "opened": gulv1.ConversationStatus_CONVERSATION_STATUS_OPENED, "resolved": gulv1.ConversationStatus_CONVERSATION_STATUS_RESOLVED, "completed": gulv1.ConversationStatus_CONVERSATION_STATUS_COMPLETED, "failed": gulv1.ConversationStatus_CONVERSATION_STATUS_FAILED, "interrupted": gulv1.ConversationStatus_CONVERSATION_STATUS_INTERRUPTED, "outcome_unknown": gulv1.ConversationStatus_CONVERSATION_STATUS_OUTCOME_UNKNOWN}[e.Status]
	out := &gulv1.ConversationEntry{EntryId: e.ID, TurnRef: e.TurnRef, Kind: gulv1.ConversationKind(e.Kind), Status: status, OccurredAt: timestamppb.New(e.At), Preview: e.Preview, PreviewTruncated: e.Truncated, HasOriginal: e.HasOriginal, InteractionRef: e.InteractionRef, InteractionKind: e.InteractionKind, InteractionStatus: e.InteractionStatus, Title: e.Title}
	for _, img := range e.Images {
		out.Images = append(out.Images, &gulv1.ConversationImage{Ordinal: img.Ordinal, Detail: img.Detail, MediaType: img.MediaType, ByteLength: img.Length, Sha256: img.SHA256})
	}
	return out
}
func (h *DirectPresentationHandler) ListConversation(ctx context.Context, r *connect.Request[gulv1.ListConversationRequest]) (*connect.Response[gulv1.ListConversationResponse], error) {
	subject, err := h.historyAccess(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.History.ListConversation(ctx, subject, r.Msg.SessionId, r.Msg.GetPageToken(), r.Msg.PageSize)
	if err != nil {
		return nil, historyError(err)
	}
	out := &gulv1.ListConversationResponse{SnapshotId: v.SnapshotID, NextPageToken: nextToken(v.Next), TraversalComplete: v.Complete, Freshness: gulv1.Freshness_FRESHNESS_FRESH, ObservedAt: timestamppb.New(v.ObservedAt), RunState: v.RunState}
	for _, e := range v.Items {
		out.Items = append(out.Items, conversationEntry(e))
	}
	if proto.Size(out) > domain.MaximumPageMetadataBytes {
		return nil, historyError(history.ErrLimit)
	}
	return connect.NewResponse(out), nil
}
func (h *DirectPresentationHandler) GetConversationEntry(ctx context.Context, r *connect.Request[gulv1.GetConversationEntryRequest]) (*connect.Response[gulv1.GetConversationEntryResponse], error) {
	subject, err := h.historyAccess(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.History.GetOriginal(ctx, subject, r.Msg.SessionId, r.Msg.EntryId, false)
	if err != nil {
		return nil, historyError(err)
	}
	return connect.NewResponse(&gulv1.GetConversationEntryResponse{Entry: conversationEntry(v.Entry), Original: original(v)}), nil
}
func (h *DirectPresentationHandler) ListSpecialistResults(ctx context.Context, r *connect.Request[gulv1.ListSpecialistResultsRequest]) (*connect.Response[gulv1.ListSpecialistResultsResponse], error) {
	subject, err := h.historyAccess(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.History.ListResults(ctx, subject, r.Msg.SessionId, r.Msg.GetPageToken(), r.Msg.PageSize)
	if err != nil {
		return nil, historyError(err)
	}
	out := &gulv1.ListSpecialistResultsResponse{SnapshotId: v.SnapshotID, NextPageToken: nextToken(v.Next), TraversalComplete: v.Complete, Freshness: gulv1.Freshness_FRESHNESS_FRESH, ObservedAt: timestamppb.New(v.ObservedAt)}
	for _, e := range v.Items {
		out.Items = append(out.Items, &gulv1.SpecialistResult{ResultId: e.ID, SpecialistViewId: e.ViewID, RoleLabel: e.Role, PublicationOrder: e.Order, PublishedAt: timestamppb.New(e.At), Format: gulv1.SpecialistResultFormat_SPECIALIST_RESULT_FORMAT_UTF8_TEXT, ByteLength: e.Length, Sha256: e.SHA256, ArtifactRef: e.ArtifactRef})
	}
	if proto.Size(out) > domain.MaximumPageMetadataBytes {
		return nil, historyError(history.ErrLimit)
	}
	return connect.NewResponse(out), nil
}

// ArtifactHandler is unmounted until E8 authenticates product routes.
type ArtifactHandler struct {
	gulv1connect.UnimplementedArtifactPresentationServiceHandler
	Core      *app.Core
	Principal PrincipalResolver
	History   *history.Service
}

var _ gulv1connect.ArtifactPresentationServiceHandler = (*ArtifactHandler)(nil)

func (h *ArtifactHandler) access(ctx context.Context) (string, error) {
	if h == nil || h.Core == nil || h.Principal == nil || h.History == nil {
		return "", accessError(connect.CodeUnauthenticated, "product access unavailable")
	}
	return localAccess(ctx, h.Core, h.Principal)
}
func (h *ArtifactHandler) GetMetadata(ctx context.Context, r *connect.Request[gulv1.GetMetadataRequest]) (*connect.Response[gulv1.GetMetadataResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.History.GetMetadata(ctx, subject, r.Msg.SessionId, r.Msg.ArtifactRef)
	if err != nil {
		return nil, historyError(err)
	}
	return connect.NewResponse(&gulv1.GetMetadataResponse{ArtifactRef: v.Ref, MediaType: v.MediaType, ByteLength: v.Length, Sha256: v.SHA256}), nil
}
func (h *ArtifactHandler) ReadChunk(ctx context.Context, r *connect.Request[gulv1.ReadChunkRequest]) (*connect.Response[gulv1.ReadChunkResponse], error) {
	subject, err := h.access(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.History.ReadChunk(ctx, subject, r.Msg.SessionId, r.Msg.ArtifactRef, r.Msg.Offset, r.Msg.Length)
	if err != nil {
		return nil, historyError(err)
	}
	return connect.NewResponse(&gulv1.ReadChunkResponse{Data: v.Data, TotalLength: v.Length, Sha256: v.SHA256}), nil
}
func historyError(err error) error {
	code, kind, action := connect.CodeUnavailable, gulv1.ErrorCode_ERROR_CODE_SOURCE_UNAVAILABLE, gulv1.ActionClass_ACTION_CLASS_REFRESH_SNAPSHOT
	switch {
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, errors.New("history read canceled"))
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("history read deadline exceeded"))
	case errors.Is(err, domain.ErrInvalidPageToken):
		code, kind = connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN
	case errors.Is(err, domain.ErrPageTokenExpired):
		code, kind = connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_PAGE_TOKEN_EXPIRED
	case errors.Is(err, domain.ErrInvalidPageSize), errors.Is(err, history.ErrInvalid):
		code, kind = connect.CodeInvalidArgument, gulv1.ErrorCode_ERROR_CODE_INVALID_REQUEST
	case errors.Is(err, history.ErrAuthority):
		code, kind, action = connect.CodePermissionDenied, gulv1.ErrorCode_ERROR_CODE_UNAUTHORIZED, gulv1.ActionClass_ACTION_CLASS_ABORT
	case errors.Is(err, history.ErrBlocked):
		code, kind = connect.CodeFailedPrecondition, gulv1.ErrorCode_ERROR_CODE_PROVIDER_BLOCKED
	case errors.Is(err, history.ErrLimit), errors.Is(err, domain.ErrPageTokenCapacity):
		code, kind = connect.CodeResourceExhausted, gulv1.ErrorCode_ERROR_CODE_LIMIT_EXCEEDED
	}
	out := connect.NewError(code, errors.New("history read unavailable"))
	detail, e := connect.NewErrorDetail(&gulv1.DomainError{Code: kind, Action: action})
	if e == nil {
		out.AddDetail(detail)
	}
	return out
}
