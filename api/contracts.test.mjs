import {expect, test} from "bun:test";
import {create, fromBinary, toBinary} from "@bufbuild/protobuf";
import {
  ActionClass,
  RuntimeService,
  ClientEventService,
  InteractionPresentationService,
  InteractionCardSchema,
  InteractionCardSummarySchema,
  ResolveRequestSchema,
  ClientEventSchema,
  LaunchExecutionLane,
  LaunchAssurance,
  RuntimeProfileCompatibility,
  ListRuntimeProfilesResponseSchema,
  CheckCompatibilityRequestSchema,
  CheckCompatibilityResponseSchema,
  CloseOutcomeSchema,
  CloseStatus,
  DirectSessionService,
  WorkspacePresentationService,
  ErrorCode,
  Freshness,
  SessionLifecycle,
  SessionComposition,
  ApprovalPolicy,
  CloseProgress,
  RecoveryClass,
  ObservedMemberLifecycle,
  ObservedMemberSchema,
  SpecialistResultFormat,
  SpecialistResultSchema,
  GetExecutionStateResponseSchema,
  GetPromptHistoryItemResponseSchema,
  GetMetadataResponseSchema,
  ReadChunkResponseSchema,
  ListPromptHistoryResponseSchema,
  ListSpecialistResultsResponseSchema,
  PromptHistoryItemSchema,
  PromptOriginalSchema,
  file_gul_v1_gul,
} from "./generated/ts/gul/v1/gul_pb.ts";
import {validateArtifactChunkRequest, validateArtifactChunkResponse, validateArtifactMetadata, validateCloseOutcome, validateExecutionState, validatePageRequest, validatePageResponse, validatePromptOriginal, validateRuntimeProfiles, validateLaunchChoice, validateLaunchConfiguration} from "./contracts.mjs";

test("the generated browser surface is explicit and contains no upstream private fields", () => {
  expect(Object.keys(ClientEventService.method)).toEqual(["watchClientEvents"]);
  expect(ClientEventService.method.watchClientEvents.methodKind).toBe("server_streaming");
  expect(ClientEventSchema.fields.map(field => field.name)).toEqual(["delivery_sequence", "session_id", "correlation_id", "kind", "created_at"]);
  expect(Object.keys(RuntimeService.method).sort()).toEqual(["checkCompatibility", "listRuntimeProfiles"]);
  expect(Object.keys(DirectSessionService.method).sort()).toEqual([
    "closeRuntime", "getDirectSessionPresentation", "getExecutionState", "getPromptHistoryItem", "listDirectSessions", "listPromptHistory", "listSpecialistResults",
    "renameDirectSession", "setDirectSessionArchived", "setDirectSessionFavorite",
  ]);
  expect(Object.keys(WorkspacePresentationService.method).sort()).toEqual([
    "browseRegistrableRoot", "getNavigation", "listRegistrableRoots", "listWorkspaces", "registerFromAllowlistPath",
    "registerFromHostSelection", "removeWorkspaceEntry", "renameWorkspace", "revalidateWorkspace", "setNavigation", "setWorkspaceFavorite", "setWorkspaceHidden",
  ]);
  const names = file_gul_v1_gul.messages.flatMap(message => message.fields.map(field => field.name));
  for (const forbidden of ["provider_cursor", "run_id", "controller_carrier", "socket_path", "worker_id", "operation_id", "workspace_absolute_path"]) {
    expect(names).not.toContain(forbidden);
  }
});

test("global Profile and launch choice require supported typed fields and shared read-only consent", () => {
  const catalog = create(ListRuntimeProfilesResponseSchema, {
    profiles: [{name: "profile", compatibility: RuntimeProfileCompatibility.COMPATIBLE, runtimeVersion: "runtime-1",
      models: [{modelId: "model", isDefault: true, supportedEfforts: ["medium"]}],
      supportedLanes: [LaunchExecutionLane.DEDICATED, LaunchExecutionLane.SHARED_READONLY],
      maximumAssurance: LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA,
      featureFlags: ["safe_client_projection"], accessPolicyTransition: "unverified", backgroundExecution: "unavailable"}],
    preprovisionedPolicyNames: ["named-policy"], sharedReadonlyWarning: "Shared read-only access is permanent.",
  });
  expect(() => validateRuntimeProfiles(catalog)).not.toThrow();
  expect(() => validateRuntimeProfiles({...catalog, profiles: [{...catalog.profiles[0], runtimeVersion: ""}]})).toThrow("runtime profile");
  expect(() => validateRuntimeProfiles({...catalog, profiles: [{...catalog.profiles[0], models: [{...catalog.profiles[0].models[0], supportedEfforts: [""]}]}]})).toThrow("runtime profile");
  expect(() => validateRuntimeProfiles({...catalog, profiles: Array.from({length: 101}, (_, index) => ({...catalog.profiles[0], name: `profile-${index}`}))})).toThrow("runtime profile");
  expect(() => validateRuntimeProfiles({...catalog, preprovisionedPolicyNames: ["named-policy", "named-policy"]})).toThrow("runtime profile");
  expect(() => validateRuntimeProfiles({...catalog, sharedReadonlyWarning: ""})).toThrow("runtime profile");
  expect(() => validateRuntimeProfiles({...catalog, profiles: [catalog.profiles[0], catalog.profiles[0]]})).toThrow("runtime profile");
  expect(() => validateRuntimeProfiles({...catalog, profiles: [{...catalog.profiles[0], featureFlags: ["x".repeat(262144)]}]})).toThrow("exceeds bound");
  const choice = create(CheckCompatibilityRequestSchema, {profileName: "profile", modelId: "model", effort: "medium",
    lane: LaunchExecutionLane.SHARED_READONLY, requiredAssurance: LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA,
    policyName: "named-policy"});
  expect(() => validateLaunchChoice(choice)).toThrow("launch choice");
  choice.acknowledgeSharedReadonly = true;
  expect(() => validateLaunchChoice(choice)).not.toThrow();
  expect(() => validateLaunchChoice({...choice, lane: LaunchExecutionLane.UNSPECIFIED})).toThrow("launch choice");
  expect(() => validateLaunchChoice({...choice, modelId: "m".repeat(262144)})).toThrow("exceeds bound");
  const configured = create(CheckCompatibilityResponseSchema, {configuration: {profileName: "profile", modelId: "model", effort: "medium",
    lane: LaunchExecutionLane.SHARED_READONLY, requiredAssurance: LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA,
    policyName: "named-policy", controlMode: "direct_interactive", purpose: "interactive", controllerKind: "interactive_client",
    orchestrationUseCase: "dolgorae_orchestrated_session", sharedReadonlyWarning: "Shared read-only access is permanent."}});
  expect(() => validateLaunchConfiguration(configured)).not.toThrow();
  expect(() => validateLaunchConfiguration({...configured, configuration: {...configured.configuration, sharedReadonlyWarning: ""}})).toThrow("launch configuration");
  for (const key of ["controlMode", "purpose", "controllerKind", "orchestrationUseCase"]) {
    expect(() => validateLaunchConfiguration({...configured, configuration: {...configured.configuration, [key]: "unsupported"}})).toThrow("launch configuration");
  }
  expect(() => validateLaunchConfiguration({...configured, configuration: undefined})).toThrow("launch configuration");
  expect(() => validateLaunchConfiguration({...configured, configuration: {...configured.configuration, profileName: "p".repeat(262144)}})).toThrow("exceeds bound");
});

test("history preserves distinct accepted items and exact original Unicode", () => {
  const first = create(PromptHistoryItemSchema, {
    promptItemId: "item-1", ordinal: 1n, preview: "같은 문장", conversationEntryId: "entry-1",
  });
  const second = create(PromptHistoryItemSchema, {
    promptItemId: "item-2", ordinal: 2n, preview: "같은 문장", conversationEntryId: "entry-2",
  });
  const page = create(ListPromptHistoryResponseSchema, {
    snapshotId: "snapshot-1", items: [first, second], nextPageToken: "gul-token", freshness: Freshness.FRESH,
  });
  validatePageResponse(page, "history");
  const decoded = fromBinary(ListPromptHistoryResponseSchema, toBinary(ListPromptHistoryResponseSchema, page));
  expect(decoded.items.map(item => item.promptItemId)).toEqual(["item-1", "item-2"]);
  expect(decoded.items.map(item => item.preview)).toEqual(["같은 문장", "같은 문장"]);
  const original = create(GetPromptHistoryItemResponseSchema, {
    promptItemId: "item-1", ordinal: 1n, conversationEntryId: "entry-1",
    original: create(PromptOriginalSchema, {content: {case: "inlineUtf8", value: "첫 줄\r\n둘째 줄\n한글"}}),
  });
  expect(fromBinary(GetPromptHistoryItemResponseSchema, toBinary(GetPromptHistoryItemResponseSchema, original)).original?.content.value)
    .toBe("첫 줄\r\n둘째 줄\n한글");
  expect(() => validatePromptOriginal(original)).not.toThrow();
  expect(() => validatePromptOriginal(create(GetPromptHistoryItemResponseSchema, {
    ...original, original: create(PromptOriginalSchema, {content: {case: "inlineUtf8", value: "가".repeat(100000)}}),
  }))).toThrow("prompt original");
  const referenced = create(GetPromptHistoryItemResponseSchema, {
    ...original, original: create(PromptOriginalSchema, {content: {case: "artifactRef", value: "artifact"}}),
  });
  expect(fromBinary(GetPromptHistoryItemResponseSchema, validatePromptOriginal(referenced)).original.content.value).toBe("artifact");
  for (const value of ["", "가".repeat(1400)]) {
    expect(() => validatePromptOriginal({...referenced, original: {content: {case: "artifactRef", value}}})).toThrow("prompt original");
  }
});

test("empty continuation is not completion and page bounds fail closed", () => {
  const empty = create(ListPromptHistoryResponseSchema, {
    snapshotId: "snapshot-1", nextPageToken: "next", traversalComplete: false, freshness: Freshness.FRESH,
  });
  expect(() => validatePageResponse(empty, "history")).not.toThrow();
  expect(empty.items).toEqual([]);
  expect(() => validatePageResponse({...empty, traversalComplete: true}, "history")).toThrow();
  expect(() => validatePageRequest({sessionId: "session", pageSize: 101})).toThrow();
  expect(() => validatePageRequest({sessionId: "session", pageSize: 50, pageToken: "x".repeat(4097)})).toThrow();
  expect(() => validatePageRequest({sessionId: "session", pageSize: 50, pageToken: "가".repeat(1400)})).toThrow();
  expect(() => validatePageResponse({...empty, nextPageToken: "가".repeat(1400)}, "history")).toThrow();
  const oversized = create(ListPromptHistoryResponseSchema, {
    snapshotId: "snapshot", freshness: Freshness.FRESH,
    items: [create(PromptHistoryItemSchema, {promptItemId: "item", ordinal: 1n, conversationEntryId: "entry", preview: "a".repeat(1025)})],
  });
  expect(() => validatePageResponse(oversized, "history")).toThrow("prompt history item");
  const oversizedMetadata = create(ListPromptHistoryResponseSchema, {
    snapshotId: "snapshot", freshness: Freshness.FRESH,
    items: [create(PromptHistoryItemSchema, {promptItemId: "i".repeat(270000), ordinal: 1n, conversationEntryId: "entry"})],
  });
  expect(() => validatePageResponse(oversizedMetadata, "history")).toThrow("exceeds bound");
  const missingSource = create(ListSpecialistResultsResponseSchema, {snapshotId: "snapshot", freshness: Freshness.UNAVAILABLE});
  expect(() => validatePageResponse(missingSource, "results")).not.toThrow();
  expect(missingSource.freshness).toBe(Freshness.UNAVAILABLE);
  expect(() => validatePageRequest({sessionId: "", pageSize: 50})).toThrow();
  expect(() => validatePageResponse({...empty, snapshotId: ""}, "history")).toThrow();
  expect(() => validatePageResponse({...empty, freshness: Freshness.UNSPECIFIED}, "history")).toThrow();
});

test("execution state preserves typed close semantics and rejects missing authority", () => {
  const state = create(GetExecutionStateResponseSchema, {
    sessionId: "session", stateVersion: "version", freshness: Freshness.FRESH,
    lifecycle: SessionLifecycle.CLOSING, composition: SessionComposition.BROKERED_HIERARCHY,
    approvalPolicy: ApprovalPolicy.USER_APPROVAL_REQUIRED, closeProgress: CloseProgress.OUTCOME_UNKNOWN,
    recovery: RecoveryClass.OUTCOME_UNKNOWN, closeOperationRef: "gul-operation", counts: {nonretiredMembers: 1n},
    observedMembers: [{observedRef: "observed-1", lifecycle: ObservedMemberLifecycle.RUNNING}],
  });
  expect(fromBinary(GetExecutionStateResponseSchema, validateExecutionState(state)).closeOperationRef).toBe("gul-operation");
  expect(() => validateExecutionState({...state, stateVersion: ""})).toThrow("execution state");
  expect(() => validateExecutionState({...state, lifecycle: SessionLifecycle.UNSPECIFIED})).toThrow("execution state");
  expect(() => validateExecutionState({...state, specialistPolicyName: "x".repeat(270000)})).toThrow("exceeds bound");
  expect(() => validateExecutionState({...state, counts: undefined})).toThrow("execution state");
  expect(() => validateExecutionState({...state, observedMembers: [{observedRef: "observed-1", lifecycle: ObservedMemberLifecycle.UNSPECIFIED}]})).toThrow("observed member");
  const unavailable = create(GetExecutionStateResponseSchema, {sessionId: "session", freshness: Freshness.UNAVAILABLE});
  expect(() => validateExecutionState(unavailable)).not.toThrow();
  expect(() => validateExecutionState({...unavailable, sessionId: "x".repeat(270000)})).toThrow("exceeds bound");
  expect(() => validateExecutionState({...unavailable, counts: {nonretiredMembers: 0n}})).toThrow("unavailable");
  for (const fields of [
    {stateVersion: "fabricated"},
    {observedMembers: state.observedMembers},
    {observedMembersTruncated: true},
    {observedAt: {seconds: 1n, nanos: 0}},
  ]) {
    expect(() => validateExecutionState({...unavailable, ...fields})).toThrow("unavailable");
  }
  expect(() => validateExecutionState({...state, observedMembers: Array.from({length: 257}, (_, i) => ({observedRef: `member-${i}`, lifecycle: ObservedMemberLifecycle.RUNNING}))})).toThrow("observed member");
  expect(() => validateExecutionState({...state, observedMembersTruncated: true})).toThrow("observed member");
  const bounded = create(GetExecutionStateResponseSchema, {...state, observedMembers: Array.from({length: 256}, (_, i) => create(ObservedMemberSchema, {observedRef: `member-${i}`, lifecycle: ObservedMemberLifecycle.RUNNING})), observedMembersTruncated: true});
  expect(() => validateExecutionState(bounded)).not.toThrow();
});

test("result items retain Gul references and reject malformed integrity fields", () => {
  const result = create(SpecialistResultSchema, {
    resultId: "result", specialistViewId: "view", roleLabel: "reviewer", publicationOrder: 1n,
    format: SpecialistResultFormat.UTF8_TEXT, byteLength: 3n, sha256: "a".repeat(64), artifactRef: "artifact",
  });
  const page = create(ListSpecialistResultsResponseSchema, {snapshotId: "snapshot", freshness: Freshness.FRESH, items: [result]});
  validatePageResponse(page, "results");
  expect(fromBinary(ListSpecialistResultsResponseSchema, toBinary(ListSpecialistResultsResponseSchema, page)).items[0].artifactRef).toBe("artifact");
  expect(() => validatePageResponse({...page, items: [{...result, sha256: "bad"}]}, "results")).toThrow("result item");
  expect(() => validatePageResponse({...page, items: [{...result, format: SpecialistResultFormat.UNSPECIFIED}]}, "results")).toThrow("result item");
  expect(() => validateArtifactChunkRequest({sessionId: "session", artifactRef: "artifact", length: 262145})).toThrow("chunk request");
  expect(() => validateArtifactChunkRequest({sessionId: "session", artifactRef: "artifact", length: 0})).toThrow("chunk request");
  expect(() => validateArtifactChunkRequest({sessionId: "session", artifactRef: "artifact", length: 1})).toThrow("chunk request");
  expect(() => validateArtifactChunkRequest({sessionId: "session", artifactRef: "artifact", offset: -1n, length: 1})).toThrow("chunk request");
});

test("artifact responses enforce integrity and requested chunk bounds", () => {
  const metadata = create(GetMetadataResponseSchema, {artifactRef: "artifact", mediaType: "text/plain", byteLength: 3n, sha256: "a".repeat(64)});
  expect(() => validateArtifactMetadata(metadata)).not.toThrow();
  expect(() => validateArtifactMetadata({...metadata, sha256: "bad"})).toThrow("metadata");
  const request = {sessionId: "session", artifactRef: "artifact", offset: 1n, length: 2};
  const chunk = create(ReadChunkResponseSchema, {data: new Uint8Array([1, 2]), totalLength: 3n, sha256: "a".repeat(64)});
  expect(() => validateArtifactChunkResponse(chunk, request)).not.toThrow();
  expect(() => validateArtifactChunkResponse({...chunk, data: new Uint8Array(3)}, request)).toThrow("chunk response");
  expect(() => validateArtifactChunkResponse({...chunk, totalLength: 2n}, request)).toThrow("chunk response");
  expect(() => validateArtifactChunkResponse({...chunk, sha256: "bad"}, request)).toThrow("chunk response");
  expect(() => validateArtifactChunkResponse({...chunk, data: new Uint8Array(0)}, request)).toThrow("chunk response");
  expect(() => validateArtifactChunkResponse({...chunk, data: new Uint8Array(0)}, {...request, offset: 3n})).not.toThrow();
});

test("close outcomes keep rejection, pending, confirmation, and ambiguity distinct", () => {
  const rejected = create(CloseOutcomeSchema, {
    status: CloseStatus.REJECTED, closeAttemptId: "attempt-1",
    rejection: {code: ErrorCode.RUN_STATE_CONFLICT, action: ActionClass.REFRESH_SNAPSHOT},
  });
  expect(() => validateCloseOutcome(rejected)).not.toThrow();
  for (const status of [CloseStatus.IN_PROGRESS, CloseStatus.CONFIRMED, CloseStatus.OUTCOME_UNKNOWN, CloseStatus.RECOVERY_REQUIRED]) {
    const outcome = create(CloseOutcomeSchema, {status, closeAttemptId: "attempt-1"});
    expect(fromBinary(CloseOutcomeSchema, validateCloseOutcome(outcome)).status).toBe(status);
  }
  expect(() => validateCloseOutcome(create(CloseOutcomeSchema, {status: CloseStatus.REJECTED, closeAttemptId: "attempt-1"}))).toThrow("typed code");
  expect(() => validateCloseOutcome(create(CloseOutcomeSchema, {status: CloseStatus.CONFIRMED, closeAttemptId: "attempt-1", rejection: rejected.rejection}))).toThrow("must not carry");
  expect(() => validateCloseOutcome(create(CloseOutcomeSchema, {status: 99, closeAttemptId: "attempt-1"}))).toThrow("invalid Gul close outcome");
  expect(() => validateCloseOutcome({...rejected, rejection: {...rejected.rejection, code: 999}})).toThrow("typed code");
  expect(() => validateCloseOutcome({...rejected, rejection: {...rejected.rejection, action: 999}})).toThrow("typed code");
  expect(() => validateCloseOutcome({...rejected, nextAction: 999})).toThrow("invalid Gul close outcome");
  expect(() => validateCloseOutcome(create(CloseOutcomeSchema, {status: CloseStatus.CONFIRMED, closeAttemptId: "x".repeat(17000)}))).toThrow("exceeds bound");
});


test("interaction cards use typed Gul fields and a body-only response", () => {
  expect(Object.keys(InteractionPresentationService.method)).toEqual(["listPending", "getCard", "resolve"]);
  expect(InteractionCardSchema.fields.map(field => field.name)).toEqual(["summary", "decisions", "command_approval", "file_approval", "user_input", "unsupported"]);
  expect(InteractionCardSummarySchema.fields.map(field => field.name)).not.toContain("run_id");
  expect(ResolveRequestSchema.fields.map(field => field.name)).toEqual(["session_id", "interaction_id", "response_json"]);
});
