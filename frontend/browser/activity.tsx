import {create} from "@bufbuild/protobuf";
import {timestampFromDate} from "@bufbuild/protobuf/wkt";
import {Code, ConnectError} from "@connectrpc/connect";
import {createRoot} from "react-dom/client";
import {
  ActionFlagsSchema, ActionStateSchema, ApprovalPolicy, CloseProgress, CloseStatus, ConversationEntrySchema,
  ConversationKind, ConversationStatus, Freshness, GetActionStateResponseSchema, GetCardResponseSchema,
  GetConversationEntryResponseSchema, GetExecutionStateResponseSchema, GetPromptHistoryItemResponseSchema,
  InteractionCardDecision, InteractionCardSchema, InteractionCardStatus, InteractionCardSummarySchema,
  ListConversationResponseSchema, ListPendingResponseSchema, ListPromptHistoryResponseSchema,
  PromptHistoryItemSchema, PromptOriginalSchema, ProviderState, ResolveResponseSchema, SessionLifecycle,
  WriterAccessMode, CloseRuntimeResponseSchema, CloseOutcomeSchema, InteractionResolutionOutcome,
  CommandApprovalCardSchema,
  CompareFixedRevisionsResponseSchema, DirectSessionPresentationSchema, GetGitStatusResponseSchema,
  ListDirectoryResponseSchema, ListWorkspacesResponseSchema, NavigationResponseSchema, ReadPreviewResponseSchema,
  WorkspaceEntrySchema,
  ActionClass, DomainErrorSchema, ErrorCode,
  GetMetadataResponseSchema, ReadChunkResponseSchema,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {SessionDetail, type SessionDetailClient} from "../src/session-detail";
import {OperatorApp, type OperatorClients} from "../src/operator-app";
import "../src/styles.css";

const calls = {close: 0, resolve: 0, conversation: 0, history: 0, original: 0, entry: 0, originalFailureSettled: 0, originalSuccessSettled: 0, promptOriginalSettled: 0, action: 0, pending: 0, card: 0, execution: 0};
const params = new URL(location.href).searchParams;
const faults = {history: params.has("history-failure") ? 1 : 0};
const blocker = params.get("blocker");
function typedFailure(name: string) {
  return new ConnectError("private diagnostic", Code.FailedPrecondition, undefined,
    [{desc: DomainErrorSchema, value: {code: ErrorCode[name as keyof typeof ErrorCode] as ErrorCode, action: ActionClass.OPERATOR_REPAIR}}]);
}
Object.assign(window, {fixture: Object.assign(calls, {faults})});
const first = create(ConversationEntrySchema, {entryId: "entry-1", kind: ConversationKind.HUMAN,
  status: ConversationStatus.ACCEPTED, preview: "repeat", hasOriginal: true});
const answer = create(ConversationEntrySchema, {entryId: "entry-2", kind: ConversationKind.ASSISTANT,
  status: ConversationStatus.FINAL, preview: "final preview", previewTruncated: true, hasOriginal: true});
const otherAnswer = create(ConversationEntrySchema, {entryId: "entry-other", kind: ConversationKind.ASSISTANT,
  status: ConversationStatus.FINAL, preview: "other final preview", hasOriginal: true});
let originalReads = 0;
const opened = create(ConversationEntrySchema, {entryId: "entry-opened", kind: ConversationKind.INTERACTION_OPENED,
  status: ConversationStatus.OPENED, title: "Approval opened", interactionRef: "approval-1"});
const second = create(ConversationEntrySchema, {entryId: "entry-3", kind: ConversationKind.HUMAN,
  status: ConversationStatus.ACCEPTED, preview: "repeat", hasOriginal: true});
const prompts = [create(PromptHistoryItemSchema, {promptItemId: "prompt-1", ordinal: 1n, acceptedAt: params.has("invalid-time")
  ? {...timestampFromDate(new Date("2026-09-29T12:00:00Z")), seconds: 9223372036854775807n}
  : params.has("invalid-nanos") ? {...timestampFromDate(new Date("2026-09-29T12:00:00Z")), nanos: 1_000_000_000}
    : timestampFromDate(new Date("2026-09-29T12:00:00Z")), preview: "repeat", conversationEntryId: first.entryId}),
  create(PromptHistoryItemSchema, {promptItemId: "prompt-2", ordinal: 2n, acceptedAt: timestampFromDate(new Date("2026-09-29T12:01:00Z")), preview: "repeat", conversationEntryId: second.entryId})];
const summary = create(InteractionCardSummarySchema, {interactionId: "approval-1", status: InteractionCardStatus.PENDING});
const card = create(InteractionCardSchema, {summary, actions: create(ActionFlagsSchema, {canResolveInteraction: true}),
  decisions: [InteractionCardDecision.ACCEPT_ONCE], detail: params.has("input-card")
    ? {case: "userInput", value: {questions: [{questionId: "ko", header: "Korean answer", question: "Enter Korean text", isSecret: false}]}}
    : {case: "commandApproval",
      value: create(CommandApprovalCardSchema, {title: "Approve command", message: "Needs your decision", command: ["make", "test"]})}});
const artifactBodies = new Map(["artifact-prompt", "artifact-response"].map(ref => [ref, new TextEncoder().encode(ref === "artifact-prompt" ? "# 한글 prompt original" : "# Complete final answer")]));
const artifactDigests = new Map(await Promise.all([...artifactBodies].map(async ([ref, bytes]) => [ref,
  Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))).map(byte => byte.toString(16).padStart(2, "0")).join("")] as const)));

const clients: SessionDetailClient = {
  listConversation: async request => {calls.conversation++;
    if (params.has("conversation-delay") && calls.conversation > 1) await new Promise(resolve => setTimeout(resolve, 100));
    return create(ListConversationResponseSchema,
    request.pageToken === "more" && params.has("conversation-before-linked")
      ? {snapshotId: "conversation-1", items: [opened], nextPageToken: "last"}
      : request.pageToken ? {snapshotId: params.has("conversation-snapshot-change") ? "conversation-2" : "conversation-1",
      items: params.has("conversation-overlap") ? [answer, second] : [second], traversalComplete: true}
      : {snapshotId: "conversation-1", items: params.has("original-race") ? [first, answer, otherAnswer]
        : params.has("entry-interaction") ? [first, opened, answer] : [first, answer], nextPageToken: "more", traversalComplete: params.has("invalid-page")});},
  getConversationEntry: async ({entryId}) => {calls.entry++;
    if (entryId === answer.entryId && params.has("original-race") && ++originalReads === 1) {
      await new Promise(resolve => setTimeout(resolve, 500));
      calls.originalFailureSettled++;
      throw Error("fixture delayed original failure");
    }
    if (entryId === answer.entryId && params.has("original-success-race")) {
      if (++originalReads === 2) throw Error("fixture newer original failure");
      await new Promise(resolve => setTimeout(resolve, 500));
      calls.originalSuccessSettled++;
    }
    if (entryId === second.entryId && params.has("linked-delay")) await new Promise(resolve => setTimeout(resolve, 500));
    if (entryId === second.entryId && params.has("linked-failure")) throw Error("fixture linked failure");
    return create(GetConversationEntryResponseSchema, {entry: params.has("entry-drift") && entryId === answer.entryId ? first
      : entryId === second.entryId && params.has("linked-missing") ? undefined
        : entryId === second.entryId && params.has("linked-oversize") ? create(ConversationEntrySchema, {...second, preview: "x".repeat(1025)})
          : entryId === second.entryId && params.has("linked-exact") ? create(ConversationEntrySchema, {...second, preview: "x".repeat(1024)})
          : [first, answer, second, otherAnswer].find(item => item.entryId === entryId),
      original: create(PromptOriginalSchema, {content: params.has("artifact") || params.has("artifact-mismatch")
        ? {case: "artifactRef", value: entryId === answer.entryId ? "artifact-response" : "artifact-prompt"}
        : {case: "inlineUtf8", value: params.has("inline-oversize") ? "x".repeat(262145)
          : entryId === answer.entryId ? "Complete final answer" : entryId === otherAnswer.entryId ? "Other final answer" : `Exact ${entryId}`}})});
  },
  listPromptHistory: async request => {
    calls.history++;
    if (faults.history) {faults.history--; throw Error("fixture history unavailable");}
    return create(ListPromptHistoryResponseSchema, request.pageToken ? {snapshotId: params.has("history-snapshot-change") ? "prompts-2" : "prompts-1",
      items: params.has("history-overlap") ? prompts : [prompts[1]!], traversalComplete: true}
      : {snapshotId: "prompts-1", items: [prompts[0]!], nextPageToken: "more"});
  },
  getPromptHistoryItem: async ({promptItemId}) => {calls.original++; const item = prompts.find(value => value.promptItemId === promptItemId)!;
    if (calls.original === 1 && params.has("prompt-original-race")) {
      await new Promise(resolve => setTimeout(resolve, 500));
      calls.promptOriginalSettled++;
      if (params.get("prompt-original-race") === "failure") throw Error("fixture delayed Prompt History original failure");
    }
    return create(GetPromptHistoryItemResponseSchema, {promptItemId, ordinal: params.has("prompt-drift") ? item.ordinal + 1n : item.ordinal,
      conversationEntryId: item.conversationEntryId,
      original: create(PromptOriginalSchema, {content: params.has("artifact") || params.has("artifact-mismatch")
        ? {case: "artifactRef", value: "artifact-prompt"} : {case: "inlineUtf8", value: params.has("inline-oversize")
          ? "x".repeat(262145) : `Exact ${item.conversationEntryId}`}})});},
  getMetadata: async ({artifactRef}) => {const bytes = artifactBodies.get(artifactRef)!;
    return create(GetMetadataResponseSchema, {artifactRef, mediaType: "text/markdown; charset=utf-8", byteLength: BigInt(bytes.length), sha256: artifactDigests.get(artifactRef)!});},
  readChunk: async ({artifactRef, offset, length}) => {const bytes = artifactBodies.get(artifactRef)!;
    return create(ReadChunkResponseSchema, {data: bytes.slice(Number(offset), Number(offset) + length), totalLength: BigInt(bytes.length),
      sha256: params.has("artifact-mismatch") ? "0".repeat(64) : artifactDigests.get(artifactRef)!});},
  getExecutionState: async () => {calls.execution++; if (blocker) throw typedFailure(blocker); if (params.has("execution-failure")) throw Error("fixture execution failure");
    const mode = params.get("close");
    const progress = mode === "projection_unknown" ? CloseProgress.OUTCOME_UNKNOWN
      : mode === "projection_recovery" ? CloseProgress.RECOVERY_REQUIRED
        : mode === "settling" || mode === "in_progress" && calls.close ? CloseProgress.SETTLING
          : mode === "confirmed_projection" ? CloseProgress.CONFIRMED : CloseProgress.NONE;
    return create(GetExecutionStateResponseSchema, {sessionId: "session-1", freshness: mode === "stale" ? Freshness.STALE : Freshness.FRESH,
    stateVersion: "state-1", lifecycle: SessionLifecycle.ACTIVE, approvalPolicy: ApprovalPolicy.USER_APPROVAL_REQUIRED,
    providerState: ProviderState.READY, closeProgress: progress});},
  getActionState: async () => {calls.action++; if (params.has("eligibility-failure")) throw new ConnectError("private diagnostic", Code.Unavailable);
    return create(GetActionStateResponseSchema, {state: create(ActionStateSchema, {
    mode: WriterAccessMode.READ_ONLY, flags: create(ActionFlagsSchema, {canRequestSessionClose: true, requiresCloseConfirmation: true,
      blockedByOutcomeUnknown: params.has("eligibility-unknown")})})});},
  listPending: async () => {calls.pending++; if (params.has("pending-typed")) throw typedFailure("PROVIDER_BLOCKED");
    if (params.has("pending-failure")) throw Error("private pending failure");
    return create(ListPendingResponseSchema, {summaries: params.has("pending-overflow")
      ? Array.from({length: 101}, (_, index) => create(InteractionCardSummarySchema, {...summary, interactionId: `approval-${index}`}))
      : [summary]});},
  getCard: async () => {calls.card++; if (params.has("card-typed")) throw typedFailure("PROFILE_MISSING");
    if (params.has("pending-overflow")) throw Error("private card failure");
    if (params.has("card-failure")) throw Error("private card failure");
    return create(GetCardResponseSchema, {card: params.has("card-mismatch") ? create(InteractionCardSchema, {...card,
      summary: create(InteractionCardSummarySchema, {interactionId: "wrong-card", status: InteractionCardStatus.PENDING})})
      : params.has("card-resolved") ? create(InteractionCardSchema, {...card,
        summary: create(InteractionCardSummarySchema, {interactionId: "approval-1", status: InteractionCardStatus.RESOLVED})}) : card});},
  resolve: async () => {calls.resolve++; return create(ResolveResponseSchema, {outcome: InteractionResolutionOutcome.RESOLVED, resolutionReceipt: "receipt"});},
  closeRuntime: async () => {calls.close++; const mode = params.get("close"); if (mode === "transport") throw Error("fixture lost response");
    if (mode === "missing") return create(CloseRuntimeResponseSchema);
    return create(CloseRuntimeResponseSchema, {outcome: create(CloseOutcomeSchema,
    {status: mode === "in_progress" ? CloseStatus.IN_PROGRESS : mode === "confirmed_receipt" ? CloseStatus.CONFIRMED
      : mode === "rejected" || mode === "rejected_external" ? CloseStatus.REJECTED : mode === "recovery" ? CloseStatus.RECOVERY_REQUIRED
        : mode === "unspecified" ? CloseStatus.UNSPECIFIED : CloseStatus.OUTCOME_UNKNOWN,
      rejection: mode === "rejected_external" ? create(DomainErrorSchema, {code: ErrorCode.PROFILE_MISSING, action: ActionClass.OPERATOR_REPAIR}) : undefined})});},
  acquireWriter: async () => ({state: create(ActionStateSchema)}),
  releaseWriter: async () => ({state: create(ActionStateSchema)}),
};

const operatorClients: OperatorClients = {
  details: clients,
  workspace: {
    listWorkspaces: async () => {const blocked = params.get("workspace-block"); if (blocked) throw typedFailure(blocked);
      if (params.has("workspace-untyped")) throw new ConnectError("private diagnostic", Code.Unavailable);
      return create(ListWorkspacesResponseSchema, {workspaces: [create(WorkspaceEntrySchema, {workspaceId: "workspace-1", displayName: "Fixture Workspace"})]});},
    getNavigation: async () => create(NavigationResponseSchema, {workspaceId: "workspace-1", sessionId: "session-1"}),
    setNavigation: async request => {if (params.has("selection-untyped")) throw new ConnectError("private diagnostic", Code.Unavailable);
      return create(NavigationResponseSchema, request);},
  },
  sessions: {listDirectSessions: async () => {if (params.has("sessions-untyped")) throw new ConnectError("private diagnostic", Code.Unavailable);
    return {sessions: [create(DirectSessionPresentationSchema,
    {workspaceId: "workspace-1", sessionId: "session-1", displayName: "Fixture Session"})]};}},
  files: {
    listDirectory: async () => create(ListDirectoryResponseSchema),
    readPreview: async () => create(ReadPreviewResponseSchema),
    getGitStatus: async () => create(GetGitStatusResponseSchema),
    compareFixedRevisions: async () => create(CompareFixedRevisionsResponseSchema),
    refreshFiles: async () => ({}),
  },
};

createRoot(document.getElementById("root")!).render(new URL(location.href).searchParams.has("integrated")
  ? <OperatorApp clients={operatorClients} />
  : <main className="operator"><div className="operator__panes operator__panes--chat"><section className="operator__pane operator__chat">
    <SessionDetail sessionId="session-1" client={clients} onActivity={value => {document.getElementById("activity")!.textContent = JSON.stringify(value);}} />
    <output id="activity" />
  </section></div></main>);
