package scenario

import (
	"context"
	"slices"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
)

func (h *Harness) ListPendingInteractions(_ context.Context, request *publicv1.ListPendingInteractionsRequest) (*publicv1.ListPendingInteractionsResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ListPendingInteractions"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	response := &publicv1.ListPendingInteractionsResponse{Context: h.response(), Stamp: r.stamp()}
	ids := make([]string, 0, len(r.interactions))
	for id := range r.interactions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		interaction := r.interactions[id]
		if interaction.GetSummary().GetStatus() == publicv1.InteractionStatus_INTERACTION_STATUS_PENDING {
			response.Items = append(response.Items, copyOf(interaction.GetSummary()))
		}
	}
	return response, nil
}

func (h *Harness) GetControllerInteraction(_ context.Context, request *publicv1.GetControllerInteractionRequest) (*publicv1.GetControllerInteractionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("GetControllerInteraction"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	interaction := r.interactions[request.GetInteractionId()]
	if interaction == nil {
		return nil, invalid()
	}
	return &publicv1.GetControllerInteractionResponse{Context: h.response(), Interaction: copyOf(interaction)}, nil
}

func (h *Harness) ResolveInteraction(_ context.Context, request *publicv1.ResolveInteractionRequest) (*publicv1.ResolveInteractionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ResolveInteraction"); err != nil {
		return nil, err
	}
	if request == nil || request.GetIdempotencyKey() == "" || len(request.GetResponseJson()) == 0 {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	key := interactionRequestKey{request.GetInteractionId(), request.GetIdempotencyKey()}
	body := digest(request.GetResponseJson())
	if prior := r.interactionKeys[key]; prior != nil {
		if r.interactionBodies[key] != body {
			return nil, conflict()
		}
		return copyOf(prior), nil
	}
	if r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return nil, conflict()
	}
	interaction := r.interactions[request.GetInteractionId()]
	if interaction == nil || interaction.GetSummary().GetStatus() != publicv1.InteractionStatus_INTERACTION_STATUS_PENDING {
		return nil, conflict()
	}
	interaction.Summary.Status = publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED
	interaction.Summary.ResolvedAt = h.timestamp()
	r.projection.PendingInteractionCount--
	if r.session != nil {
		r.session.PendingApprovalCount--
	}
	if r.projection.GetLifecycle() != publicv1.RunLifecycle_RUN_LIFECYCLE_PAUSED {
		r.projection.Lifecycle = lifecycleFromWork(r)
	}
	h.emit(r, &publicv1.DurableRunEvent{Event: &publicv1.DurableRunEvent_InteractionResolved{InteractionResolved: &publicv1.InteractionResolvedEvent{InteractionId: request.GetInteractionId(), Outcome: publicv1.InteractionOutcome_INTERACTION_OUTCOME_ANSWERED}}})
	response := &publicv1.ResolveInteractionResponse{Context: h.response(), InteractionId: request.GetInteractionId(),
		Status: publicv1.InteractionStatus_INTERACTION_STATUS_RESOLVED, ResolutionReceipt: h.next("interaction-receipt")}
	r.interactionKeys[key] = copyOf(response)
	r.interactionBodies[key] = body
	if err := h.after("ResolveInteraction"); err != nil {
		return nil, err
	}
	return response, nil
}

func (h *Harness) GetWorkspaceWriterStatus(_ context.Context, request *publicv1.GetWorkspaceWriterStatusRequest) (*publicv1.GetWorkspaceWriterStatusResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("GetWorkspaceWriterStatus"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	w, err := h.workspaceFor(request.GetWorkspace())
	if err != nil {
		return nil, err
	}
	return &publicv1.GetWorkspaceWriterStatusResponse{Writer: w.snapshot()}, nil
}

func (h *Harness) AcquireWriter(_ context.Context, request *publicv1.AcquireWriterRequest) (*publicv1.WriterState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("AcquireWriter"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	w, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if r.closeChoice != nil || r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return nil, conflict()
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if w.writer.GetAuthorityState() != publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE {
		return nil, providerError("WRITER_BUSY")
	}
	w.writer.AuthorityState = publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_ACTIVE
	w.writer.OwnerRunId = pointer(r.projection.GetRunId())
	w.writer.WriterGeneration++
	w.writer.StateRevision++
	r.projection.EffectivePolicy.Access = publicv1.EffectiveAccess_EFFECTIVE_ACCESS_WRITE
	r.projection.WriterAuthority.State = w.writer.AuthorityState
	r.projection.WriterAuthority.WriterGeneration = w.writer.WriterGeneration
	h.changed(r)
	if err := h.after("AcquireWriter"); err != nil {
		return nil, err
	}
	return w.snapshot(), nil
}

func (h *Harness) ReleaseWriter(_ context.Context, request *publicv1.ReleaseWriterRequest) (*publicv1.WriterState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ReleaseWriter"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	w, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		return nil, err
	}
	if r.projection.GetLifecycle() == publicv1.RunLifecycle_RUN_LIFECYCLE_CLOSED {
		return nil, conflict()
	}
	if err = checkRevision(r, request.GetExpectedStateRevision()); err != nil {
		return nil, err
	}
	if w.writer.GetOwnerRunId() != r.projection.GetRunId() {
		return nil, conflict()
	}
	w.writer.AuthorityState = publicv1.WriterAuthorityState_WRITER_AUTHORITY_STATE_NONE
	w.writer.OwnerRunId = nil
	w.writer.WriterGeneration++
	w.writer.StateRevision++
	r.projection.EffectivePolicy.Access = publicv1.EffectiveAccess_EFFECTIVE_ACCESS_READ
	r.projection.WriterAuthority.State = w.writer.AuthorityState
	r.projection.WriterAuthority.WriterGeneration = w.writer.WriterGeneration
	h.changed(r)
	if err := h.after("ReleaseWriter"); err != nil {
		return nil, err
	}
	return w.snapshot(), nil
}

func (h *Harness) VerifyController(_ context.Context, request *publicv1.VerifyControllerRequest) (*publicv1.VerifyControllerResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("VerifyController"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	_, r, err := h.runFor(request.GetRun())
	if err != nil {
		return nil, err
	}
	if err = checkController(r, request.GetController()); err != nil {
		policy := scenarioErrorPolicies["CONTROLLER_MISMATCH"]
		policy.action = publicv1.RequiredClientAction_REQUIRED_CLIENT_ACTION_ABORT
		return nil, errorWithPolicy("CONTROLLER_MISMATCH", policy)
	}
	return &publicv1.VerifyControllerResponse{Context: h.response(), RunId: r.projection.GetRunId(),
		Controller: copyOf(r.projection.GetController()), VerifiedAt: h.timestamp()}, nil
}

func (h *Harness) artifactFor(ref *publicv1.RunRef, id string, controller *publicv1.ControllerCarrierRef) (*run, []byte, error) {
	_, r, err := h.runFor(ref)
	if err != nil {
		return nil, nil, err
	}
	if err = checkController(r, controller); err != nil {
		return nil, nil, err
	}
	data, ok := r.artifacts[id]
	if !ok {
		return nil, nil, providerError("ARTIFACT_NOT_FOUND")
	}
	return r, data, nil
}

func (h *Harness) GetArtifact(_ context.Context, request *publicv1.GetArtifactRequest) (*publicv1.GetArtifactResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("GetArtifact"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	r, _, err := h.artifactFor(request.GetRun(), request.GetArtifactId(), request.GetController())
	if err != nil {
		return nil, err
	}
	return &publicv1.GetArtifactResponse{Context: h.response(), Artifact: copyOf(r.artifactRefs[request.GetArtifactId()]), MaximumChunkSize: maximumChunkSize}, nil
}

func (h *Harness) ReadArtifactChunk(_ context.Context, request *publicv1.ReadArtifactChunkRequest) (*publicv1.ReadArtifactChunkResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ReadArtifactChunk"); err != nil {
		return nil, err
	}
	if request == nil || request.GetLength() == 0 || request.GetLength() > maximumChunkSize {
		return nil, invalid()
	}
	_, data, err := h.artifactFor(request.GetRun(), request.GetArtifactId(), request.GetController())
	if err != nil {
		return nil, err
	}
	if request.GetOffset() > uint64(len(data)) {
		return nil, invalid()
	}
	end := min(request.GetOffset()+uint64(request.GetLength()), uint64(len(data)))
	chunk := append([]byte(nil), data[request.GetOffset():end]...)
	return &publicv1.ReadArtifactChunkResponse{Context: h.response(), ArtifactId: request.GetArtifactId(), Offset: request.GetOffset(),
		Length: uint32(len(chunk)), Data: chunk, Eof: end == uint64(len(data)), TotalByteLength: uint64(len(data)), Sha256: digest(data)}, nil
}

func (h *Harness) sessionFor(ref *publicv1.RunRef, controller *publicv1.ControllerCarrierRef) (*run, error) {
	_, r, err := h.runFor(ref)
	if err != nil {
		return nil, err
	}
	if err = checkController(r, controller); err != nil {
		return nil, err
	}
	if r.session == nil {
		return nil, invalid()
	}
	return r, nil
}

func (h *Harness) GetOrchestratedSession(_ context.Context, request *publicv1.GetOrchestratedSessionRequest) (*publicv1.GetOrchestratedSessionResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("GetOrchestratedSession"); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, invalid()
	}
	r, err := h.sessionFor(request.GetRootRun(), request.GetController())
	if err != nil {
		return nil, err
	}
	return &publicv1.GetOrchestratedSessionResponse{Context: h.response(), Session: copyOf(r.session)}, nil
}

func (h *Harness) ListOrchestratedSessionResults(_ context.Context, request *publicv1.ListOrchestratedSessionResultsRequest) (*publicv1.ListOrchestratedSessionResultsResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.before("ListOrchestratedSessionResults"); err != nil {
		return nil, err
	}
	if request == nil || request.GetProjectionVersion() != 1 || request.GetLimit() > maximumPageLimit {
		return nil, invalid()
	}
	r, err := h.sessionFor(request.GetRootRun(), request.GetController())
	if err != nil {
		return nil, err
	}
	head, offset := len(r.results), 0
	if request.PageCursor != nil {
		head, offset, err = parseCursor(request.GetPageCursor(), "result", r.projection.GetRunId(), len(r.results))
		if err != nil {
			return nil, err
		}
	}
	end := min(offset+boundedLimit(request.GetLimit()), head)
	revision := r.session.GetSourceRevision()
	capturedAt := h.timestamp()
	if head > 0 {
		revision = r.resultRevisions[head-1]
		capturedAt = copyOf(r.results[head-1].GetPublishedAt())
	}
	response := &publicv1.ListOrchestratedSessionResultsResponse{Context: h.response(),
		CapturedPublicationHead: uint64(head), SourceRevision: revision, CapturedAt: capturedAt}
	for _, item := range r.results[offset:end] {
		response.Items = append(response.Items, copyOf(item))
	}
	if end < head {
		response.NextPageCursor = pointer(pageCursor("result", r.projection.GetRunId(), head, end))
	}
	return response, nil
}
