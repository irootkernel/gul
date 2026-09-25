import {expect, test} from "bun:test";
import {create, fromBinary, toBinary} from "@bufbuild/protobuf";
import {
  ActionClass,
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
import {validateArtifactChunkRequest, validateArtifactChunkResponse, validateArtifactMetadata, validateCloseOutcome, validateExecutionState, validatePageRequest, validatePageResponse, validatePromptOriginal} from "./contracts.mjs";

test("the generated browser surface is explicit and contains no upstream private fields", () => {
  expect(Object.keys(DirectSessionService.method).sort()).toEqual([
    "closeRuntime", "getExecutionState", "getPromptHistoryItem", "listPromptHistory", "listSpecialistResults",
  ]);
  expect(Object.keys(WorkspacePresentationService.method).sort()).toEqual([
    "browseRegistrableRoot", "listRegistrableRoots", "listWorkspaces", "registerFromAllowlistPath",
    "registerFromHostSelection", "revalidateWorkspace",
  ]);
  const names = file_gul_v1_gul.messages.flatMap(message => message.fields.map(field => field.name));
  for (const forbidden of ["provider_cursor", "run_id", "controller_carrier", "socket_path", "worker_id", "operation_id", "workspace_absolute_path"]) {
    expect(names).not.toContain(forbidden);
  }
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
    recovery: RecoveryClass.OUTCOME_UNKNOWN, closeOperationRef: "gul-operation",
  });
  expect(fromBinary(GetExecutionStateResponseSchema, validateExecutionState(state)).closeOperationRef).toBe("gul-operation");
  expect(() => validateExecutionState({...state, stateVersion: ""})).toThrow("execution state");
  expect(() => validateExecutionState({...state, lifecycle: SessionLifecycle.UNSPECIFIED})).toThrow("execution state");
  expect(() => validateExecutionState({...state, specialistPolicyName: "x".repeat(270000)})).toThrow("exceeds bound");
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
