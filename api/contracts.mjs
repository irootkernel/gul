import {toBinary} from "@bufbuild/protobuf";
import {
  ActionClass,
  LaunchExecutionLane,
  LaunchAssurance,
  RuntimeProfileCompatibility,
  ListRuntimeProfilesResponseSchema,
  CheckCompatibilityRequestSchema,
  CheckCompatibilityResponseSchema,
  CloseOutcomeSchema,
  CloseStatus,
  ErrorCode,
  Freshness,
  ProviderState,
  SessionLifecycle,
  SessionComposition,
  ApprovalPolicy,
  CloseProgress,
  RecoveryClass,
  ObservedMemberLifecycle,
  SpecialistResultFormat,
  GetExecutionStateResponseSchema,
  GetPromptHistoryItemResponseSchema,
  GetMetadataResponseSchema,
  ReadChunkResponseSchema,
  ListPromptHistoryResponseSchema,
  ListSpecialistResultsResponseSchema,
} from "./generated/ts/gul/v1/gul_pb.ts";
import {
  maximumPageSize,
  maximumTokenBytes,
  maximumPageMetadataBytes,
  maximumPreviewBytes,
  maximumInlineOriginalBytes,
  maximumArtifactChunkBytes,
  maximumCloseOutcomeBytes,
} from "./generated/ts/gul/v1/bounds.ts";

export {maximumPageMetadataBytes, maximumPreviewBytes};

/** @param {string} value */
const byteLength = value => new TextEncoder().encode(value).length;
/** @param {Array<string | number>} values */
const hasDuplicates = values => new Set(values).size !== values.length;

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").ListRuntimeProfilesResponse} response */
export function validateRuntimeProfiles(response) {
  const validLanes = [LaunchExecutionLane.DEDICATED, LaunchExecutionLane.SHARED_READONLY];
  const validAssurance = [LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA,
    LaunchAssurance.VERIFIED_THREAD_SCOPED_CONTROL, LaunchAssurance.STRONG_PROCESS_CONTAINMENT];
  if (!response.sharedReadonlyWarning || response.profiles.length > 100 || response.preprovisionedPolicyNames.length > 100 ||
      hasDuplicates(response.profiles.map(profile => profile.name)) || hasDuplicates(response.preprovisionedPolicyNames) ||
      response.profiles.some(profile => !profile.name || ![
        RuntimeProfileCompatibility.COMPATIBLE, RuntimeProfileCompatibility.INCOMPATIBLE,
        RuntimeProfileCompatibility.UNVERIFIED, RuntimeProfileCompatibility.UNAVAILABLE,
      ].includes(profile.compatibility) ||
        (profile.compatibility === RuntimeProfileCompatibility.COMPATIBLE &&
          (!profile.runtimeVersion || !profile.models.length || !profile.supportedLanes.length ||
            !validAssurance.includes(profile.maximumAssurance))) ||
        hasDuplicates(profile.models.map(model => model.modelId)) || hasDuplicates(profile.supportedLanes) ||
        profile.models.some(model => !model.modelId || !model.supportedEfforts.length ||
          model.supportedEfforts.some(effort => !effort) || hasDuplicates(model.supportedEfforts)) ||
        profile.supportedLanes.some(lane => !validLanes.includes(lane))) ||
      response.preprovisionedPolicyNames.some(name => !name)) {
    throw new Error("invalid Gul runtime profile catalog");
  }
  const bytes = toBinary(ListRuntimeProfilesResponseSchema, response);
  if (bytes.length > maximumPageMetadataBytes) throw new Error("Gul runtime profile catalog exceeds bound");
  return bytes;
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").CheckCompatibilityRequest} request */
export function validateLaunchChoice(request) {
  if (!request.profileName || !request.modelId || !request.effort || !request.policyName ||
      ![LaunchExecutionLane.DEDICATED, LaunchExecutionLane.SHARED_READONLY].includes(request.lane) ||
      ![LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA, LaunchAssurance.VERIFIED_THREAD_SCOPED_CONTROL,
        LaunchAssurance.STRONG_PROCESS_CONTAINMENT].includes(request.requiredAssurance) ||
      (request.lane === LaunchExecutionLane.SHARED_READONLY && !request.acknowledgeSharedReadonly)) {
    throw new Error("invalid Gul launch choice");
  }
  const bytes = toBinary(CheckCompatibilityRequestSchema, request);
  if (bytes.length > maximumPageMetadataBytes) throw new Error("Gul launch choice exceeds bound");
  return bytes;
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").CheckCompatibilityResponse} response */
export function validateLaunchConfiguration(response) {
  const value = response.configuration;
  if (!value || !value.profileName || !value.modelId || !value.effort || !value.policyName ||
      value.controlMode !== "direct_interactive" || value.purpose !== "interactive" ||
      value.controllerKind !== "interactive_client" || value.orchestrationUseCase !== "dolgorae_orchestrated_session" ||
      ![LaunchExecutionLane.DEDICATED, LaunchExecutionLane.SHARED_READONLY].includes(value.lane) ||
      ![LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA, LaunchAssurance.VERIFIED_THREAD_SCOPED_CONTROL,
        LaunchAssurance.STRONG_PROCESS_CONTAINMENT].includes(value.requiredAssurance) ||
      (value.lane === LaunchExecutionLane.SHARED_READONLY && !value.sharedReadonlyWarning)) {
    throw new Error("invalid Gul prospective launch configuration");
  }
  const bytes = toBinary(CheckCompatibilityResponseSchema, response);
  if (bytes.length > maximumPageMetadataBytes) throw new Error("Gul launch configuration exceeds bound");
  return bytes;
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").ListPromptHistoryRequest | import("./generated/ts/gul/v1/gul_pb.ts").ListSpecialistResultsRequest} request */
export function validatePageRequest(request) {
  if (!request.sessionId || request.pageSize > maximumPageSize || byteLength(request.pageToken ?? "") > maximumTokenBytes) {
    throw new Error("invalid Gul page request");
  }
}

/**
 * @param {import("./generated/ts/gul/v1/gul_pb.ts").ListPromptHistoryResponse | import("./generated/ts/gul/v1/gul_pb.ts").ListSpecialistResultsResponse} response
 * @param {"history" | "results"} kind
 */
export function validatePageResponse(response, kind) {
  const schema = kind === "history" ? ListPromptHistoryResponseSchema : kind === "results" ? ListSpecialistResultsResponseSchema : null;
  if (!schema || !response.snapshotId || response.freshness === Freshness.UNSPECIFIED ||
      byteLength(response.nextPageToken ?? "") > maximumTokenBytes ||
      (response.traversalComplete && response.nextPageToken !== undefined)) {
    throw new Error("invalid Gul page response");
  }
  if (toBinary(schema, response).length > maximumPageMetadataBytes) {
    throw new Error("Gul page metadata exceeds bound");
  }
  if (kind === "history") {
    const history = /** @type {import("./generated/ts/gul/v1/gul_pb.ts").ListPromptHistoryResponse} */ (response);
    for (const item of history.items) {
      if (!item.promptItemId || !item.ordinal || !item.conversationEntryId ||
          byteLength(item.preview) > maximumPreviewBytes) {
        throw new Error("invalid Gul prompt history item");
      }
    }
  } else {
    const results = /** @type {import("./generated/ts/gul/v1/gul_pb.ts").ListSpecialistResultsResponse} */ (response);
    for (const item of results.items) {
      if (!item.resultId || !item.specialistViewId || !item.publicationOrder ||
          item.format !== SpecialistResultFormat.UTF8_TEXT || !item.artifactRef ||
          !/^[0-9a-f]{64}$/.test(item.sha256)) {
        throw new Error("invalid Gul specialist result item");
      }
    }
  }
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").GetPromptHistoryItemResponse} response */
export function validatePromptOriginal(response) {
  const content = response.original?.content;
  if (!response.promptItemId || !response.ordinal || !response.conversationEntryId ||
      !content || (content.case !== "inlineUtf8" && content.case !== "artifactRef") ||
      (content.case === "artifactRef" && (!content.value || byteLength(content.value) > maximumTokenBytes)) ||
      (content.case === "inlineUtf8" && byteLength(content.value) > maximumInlineOriginalBytes)) {
    throw new Error("invalid Gul prompt original");
  }
  return toBinary(GetPromptHistoryItemResponseSchema, response);
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").GetExecutionStateResponse} response */
export function validateExecutionState(response) {
  if (!response.sessionId || ![Freshness.FRESH, Freshness.STALE, Freshness.UNAVAILABLE].includes(response.freshness) ||
      ![ProviderState.READY, ProviderState.DISCONNECTED, ProviderState.INCOMPATIBLE, ProviderState.BUSY, ProviderState.DEGRADED].includes(response.providerState) ||
      (response.freshness === Freshness.FRESH && response.providerState !== ProviderState.READY && response.providerState !== ProviderState.DEGRADED)) {
    throw new Error("invalid Gul execution state");
  }
  if (response.freshness === Freshness.UNAVAILABLE) {
    if (response.counts || response.observedMembers?.length || response.observedMembersTruncated || response.stateVersion || response.observedAt) {
      throw new Error("invalid Gul unavailable execution state");
    }
    const bytes = toBinary(GetExecutionStateResponseSchema, response);
    if (bytes.length > maximumPageMetadataBytes) throw new Error("Gul execution state exceeds bound");
    return bytes;
  }
  if (!response.stateVersion ||
      (response.freshness === Freshness.FRESH && !response.counts) ||
      response.lifecycle === SessionLifecycle.UNSPECIFIED ||
      response.composition === SessionComposition.UNSPECIFIED ||
      response.approvalPolicy === ApprovalPolicy.UNSPECIFIED ||
      response.closeProgress === CloseProgress.UNSPECIFIED ||
      response.recovery === RecoveryClass.UNSPECIFIED) {
    throw new Error("invalid Gul execution state");
  }
  if ((response.observedMembers?.length || 0) > 256 ||
      (response.observedMembersTruncated && response.observedMembers?.length !== 256) ||
      response.observedMembers?.some(member => !member.observedRef ||
      member.lifecycle === ObservedMemberLifecycle.UNSPECIFIED)) {
    throw new Error("invalid Gul observed member");
  }
  const bytes = toBinary(GetExecutionStateResponseSchema, response);
  if (bytes.length > maximumPageMetadataBytes) throw new Error("Gul execution state exceeds bound");
  return bytes;
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").ReadChunkRequest} request */
export function validateArtifactChunkRequest(request) {
  if (!request.sessionId || !request.artifactRef ||
      typeof request.offset !== "bigint" || request.offset < 0n ||
      request.length < 1 || request.length > maximumArtifactChunkBytes) {
    throw new Error("invalid Gul artifact chunk request");
  }
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").GetMetadataResponse} response */
export function validateArtifactMetadata(response) {
  if (!response.artifactRef || !response.mediaType ||
      byteLength(response.artifactRef) > maximumTokenBytes ||
      !/^[0-9a-f]{64}$/.test(response.sha256)) {
    throw new Error("invalid Gul artifact metadata");
  }
  const bytes = toBinary(GetMetadataResponseSchema, response);
  if (bytes.length > maximumPageMetadataBytes) throw new Error("Gul artifact metadata exceeds bound");
  return bytes;
}

/**
 * @param {import("./generated/ts/gul/v1/gul_pb.ts").ReadChunkResponse} response
 * @param {import("./generated/ts/gul/v1/gul_pb.ts").ReadChunkRequest} request
 */
export function validateArtifactChunkResponse(response, request) {
  validateArtifactChunkRequest(request);
  if (!response.data || response.data.length > maximumArtifactChunkBytes ||
      response.data.length > request.length ||
      (response.data.length === 0 && request.offset < response.totalLength) ||
      request.offset + BigInt(response.data.length) > response.totalLength ||
      !/^[0-9a-f]{64}$/.test(response.sha256)) {
    throw new Error("invalid Gul artifact chunk response");
  }
  return toBinary(ReadChunkResponseSchema, response);
}

/** @param {import("./generated/ts/gul/v1/gul_pb.ts").CloseOutcome} outcome */
export function validateCloseOutcome(outcome) {
  if (!outcome.closeAttemptId || !Object.values(CloseStatus).includes(outcome.status) ||
      outcome.status === CloseStatus.UNSPECIFIED ||
      (outcome.nextAction !== ActionClass.UNSPECIFIED && !Object.values(ActionClass).includes(outcome.nextAction))) {
    throw new Error("invalid Gul close outcome");
  }
  if (outcome.status === CloseStatus.REJECTED) {
    if (!outcome.rejection || outcome.rejection.code === ErrorCode.UNSPECIFIED ||
        !Object.values(ErrorCode).includes(outcome.rejection.code) ||
        outcome.rejection.action === ActionClass.UNSPECIFIED ||
        !Object.values(ActionClass).includes(outcome.rejection.action)) {
      throw new Error("rejected close requires a typed code and action");
    }
  } else if (outcome.rejection) {
    throw new Error("non-rejected close must not carry a rejection");
  }
  const bytes = toBinary(CloseOutcomeSchema, outcome);
  if (bytes.length > maximumCloseOutcomeBytes) throw new Error("Gul close outcome exceeds bound");
  return bytes;
}
