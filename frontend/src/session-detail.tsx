import {useEffect, useRef, useState} from "react";
import {Code, ConnectError} from "@connectrpc/connect";
import {
  ActionBlocker, ActionFailureSchema, SubmitOutcome, type SubmitResponse, type ListSpecialistResultsResponse, type ClientEvent,
  ActionCloseIntent, ActionWriteIntent, ApprovalPolicy, CloseProgress, CloseStatus,
  Freshness, InteractionCardStatus, LaunchAssurance, ProviderState, SessionLifecycle, WriterAccessMode,
  type ActionState, type CloseRuntimeResponse, type GetActionStateResponse,
  type GetCardResponse, type GetExecutionStateResponse, type InteractionCard,
  type ListPendingResponse, type ResolveResponse,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {SpecialistResults} from "./specialist-results";
import {ConversationPanel, type ConversationClient} from "./conversation-panel";
import {domainErrorMessage, domainErrorRequiresExternalAction, externalActionRequired, operatorError} from "./domain-errors";
import {InteractionCardView} from "./interaction-card";
import {PromptHistoryPanel, type PromptHistoryClient} from "./prompt-history";
import {ProviderStatus} from "./provider-status";
import {ActionBlockerMessage, InterruptConsent, WriterPanel, PromptDraft} from "./writer-panel";
import {WriterStatus} from "./writer-status";

export type SessionDetailClient = ConversationClient & PromptHistoryClient & {
  submit?(request: {sessionId: string; attemptId: string; text: string; writeIntent: ActionWriteIntent}): Promise<SubmitResponse>;
  listSpecialistResults?(request: {sessionId: string; pageSize: number; pageToken?: string}): Promise<ListSpecialistResultsResponse>;
  watch?(request: {sessionId: string; afterDeliverySequence: bigint}, signal: AbortSignal): AsyncIterable<ClientEvent>;
  getExecutionState(request: {sessionId: string}): Promise<GetExecutionStateResponse>;
  getActionState(request: {sessionId: string; writeIntent: ActionWriteIntent; closeIntent: ActionCloseIntent; interruptConfirmed: boolean}): Promise<GetActionStateResponse>;
  listPending(request: {sessionId: string}): Promise<ListPendingResponse>;
  getCard(request: {sessionId: string; interactionId: string}): Promise<GetCardResponse>;
  resolve(request: {sessionId: string; interactionId: string; responseJson: Uint8Array}): Promise<ResolveResponse>;
  closeRuntime(request: {sessionId: string; interrupt: boolean; attemptId: string}): Promise<CloseRuntimeResponse>;
  acquireWriter(request: {sessionId: string}): Promise<{state?: ActionState}>;
  releaseWriter(request: {sessionId: string}): Promise<{state?: ActionState}>;
};

export type SessionActivity = {provider: string; activity: string; writer: string; policy: string; assurance: string; interactions: number | undefined};

const maximumPendingCards = 100;

const closeOutcomeUnknown = "Close outcome unresolved. Inspect provider state; do not repeat the request.";
const closeRecoveryRequired = "Close recovery required. Follow provider recovery outside Gul.";

function closeLabel(progress: CloseProgress) {
  switch (progress) {
    case CloseProgress.SETTLING: return "Session close is pending provider settlement.";
    case CloseProgress.CONFIRMED: return "Whole-session close confirmed by current provider projection.";
    case CloseProgress.OUTCOME_UNKNOWN: return closeOutcomeUnknown;
    case CloseProgress.RECOVERY_REQUIRED: return closeRecoveryRequired;
    default: return "No close in progress.";
  }
}

export function SessionDetail({sessionId, client, onActivity}: {sessionId: string; client: SessionDetailClient; onActivity: (value: SessionActivity | undefined) => void}) {
  const [write, setWrite] = useState(false);
  const [contentRevision, setContentRevision] = useState(0);
  const [eventError, setEventError] = useState("");
  const [eventReconnect, setEventReconnect] = useState(0);
  const [execution, setExecution] = useState<GetExecutionStateResponse>();
  const [action, setAction] = useState<ActionState>();
  const [eligibilityPending, setEligibilityPending] = useState(true);
  const [cards, setCards] = useState<InteractionCard[]>([]);
  const [pendingCount, setPendingCount] = useState<number>();
  const [pendingFailed, setPendingFailed] = useState(false);
  const [pendingNotice, setPendingNotice] = useState("");
  const [cardError, setCardError] = useState("");
  const [executionError, setExecutionError] = useState("");
  const [actionError, setActionError] = useState("");
  const [executionExternal, setExecutionExternal] = useState(false);
  const [actionExternal, setActionExternal] = useState(false);
  const [pendingExternal, setPendingExternal] = useState(false);
  const [cardExternal, setCardExternal] = useState(false);
  const [reload, setReload] = useState(0);
  const [view, setView] = useState<"conversation" | "history">("conversation");
  const [focusEntryId, setFocusEntryId] = useState("");
  const [focusRequest, setFocusRequest] = useState(0);
  const [interrupt, setInterrupt] = useState(false);
  const [closeMessage, setCloseMessage] = useState("");
  const [closeAttemptRecorded, setCloseAttemptRecorded] = useState(false);
  const [closeExternalRejected, setCloseExternalRejected] = useState(false);
  const [closing, setClosing] = useState(false);
  const closePending = useRef(false);
  const actionGeneration = useRef(0);
  const lastConsent = useRef(interrupt);
  const conversationTab = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!closeAttemptRecorded || execution?.closeProgress === CloseProgress.CONFIRMED) return;
    const timer = setInterval(() => setReload(value => value + 1), 5000);
    return () => clearInterval(timer);
  }, [closeAttemptRecorded, execution?.closeProgress]);

  useEffect(() => {
    let current = true;
    const actionRequest = ++actionGeneration.current;
    setExecution(undefined); setAction(undefined); setEligibilityPending(true); setCards([]); setPendingCount(undefined); setPendingFailed(false); setPendingNotice(""); setCardError("");
    setExecutionError(""); setActionError(""); setExecutionExternal(false); setActionExternal(false); setPendingExternal(false); setCardExternal(false);
    void Promise.allSettled([
      client.getExecutionState({sessionId}),
      client.getActionState({sessionId, writeIntent: write ? ActionWriteIntent.WRITE : ActionWriteIntent.READ, closeIntent: ActionCloseIntent.COMPLETE, interruptConfirmed: interrupt}),
      client.listPending({sessionId}),
    ]).then(async ([state, eligibility, pending]) => {
      if (!current) return;
      if (state.status === "fulfilled") setExecution(state.value);
      if (actionGeneration.current === actionRequest) {
        if (eligibility.status === "fulfilled") setAction(eligibility.value.state);
        setEligibilityPending(false);
      }
      if (state.status === "rejected") {setExecutionError(operatorError(state.reason)); setExecutionExternal(externalActionRequired(state.reason));}
      if (eligibility.status === "rejected" && actionGeneration.current === actionRequest) {
        setActionError(operatorError(eligibility.reason)); setActionExternal(externalActionRequired(eligibility.reason));
      }
      if (pending.status !== "fulfilled") {
        setPendingFailed(true);
        const external = externalActionRequired(pending.reason);
        setPendingExternal(external);
        setPendingNotice(external ? operatorError(pending.reason) : "Interaction requests are unavailable. Check provider state before acting.");
        return;
      }
      setPendingCount(pending.value.summaries.length);
      const admittedSummaries = pending.value.summaries.slice(0, maximumPendingCards);
      const truncated = admittedSummaries.length < pending.value.summaries.length;
      const loaded = await Promise.allSettled(admittedSummaries.map(summary => client.getCard({sessionId, interactionId: summary.interactionId})));
      if (!current) return;
      setCards(loaded.flatMap((result, index) => {
        if (result.status !== "fulfilled") return [];
        const card = result.value.card;
        if (!card?.summary || card.summary.interactionId !== admittedSummaries[index]?.interactionId ||
            card.summary.status !== InteractionCardStatus.PENDING) return [];
        return [card];
      }));
      const externalCard = loaded.find(result => result.status === "rejected" && externalActionRequired(result.reason));
      if (externalCard?.status === "rejected") {setCardExternal(true); setCardError(operatorError(externalCard.reason));}
      else if (truncated || loaded.some((result, index) => result.status === "rejected" ||
          result.value.card?.summary?.interactionId !== admittedSummaries[index]?.interactionId ||
          result.value.card?.summary?.status !== InteractionCardStatus.PENDING)) {
        setCardError("Some interaction requests are unavailable. Check current state before acting.");
      }
    });
    return () => {current = false;};
  }, [client, sessionId, reload, write]);

  useEffect(() => {
    if (lastConsent.current === interrupt) return;
    lastConsent.current = interrupt;
    let current = true;
    const actionRequest = ++actionGeneration.current;
    setEligibilityPending(true);
    void client.getActionState({sessionId, writeIntent: write ? ActionWriteIntent.WRITE : ActionWriteIntent.READ, closeIntent: ActionCloseIntent.COMPLETE, interruptConfirmed: interrupt})
      .then(result => {if (current && actionGeneration.current === actionRequest) {setAction(result.state); setActionError(""); setActionExternal(false);}})
      .catch(reason => {if (current && actionGeneration.current === actionRequest) {setAction(undefined); setActionError(operatorError(reason)); setActionExternal(externalActionRequired(reason));}})
      .finally(() => {if (current && actionGeneration.current === actionRequest) setEligibilityPending(false);});
    return () => {current = false;};
  }, [client, sessionId, interrupt, write]);

  useEffect(() => {
    if (!execution) {onActivity(undefined); return () => onActivity(undefined);}
    const activity = SessionLifecycle[execution.lifecycle] ?? "Unavailable";
    const policy = ApprovalPolicy[execution.approvalPolicy] ?? "Unavailable";
    const writer = action ? WriterAccessMode[action.mode] ?? "Unavailable" : "Unavailable";
    const assurance = action?.writer?.achievedAssurance ? LaunchAssurance[action.writer.achievedAssurance] ?? "Unavailable" : "Unavailable";
    onActivity({provider: ProviderState[execution.providerState] ?? "Unavailable", activity, writer, policy, assurance, interactions: pendingCount});
    return () => onActivity(undefined);
  }, [execution, action, pendingCount, onActivity]);

  useEffect(() => {
    if (!client.watch) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    setEventError("");
    void (async () => {
      try {
        for await (const event of client.watch!({sessionId, afterDeliverySequence: 0n}, controller.signal)) {
          if (controller.signal.aborted || event.sessionId !== sessionId) continue;
          setReload(value => value + 1);
          setContentRevision(value => value + 1);
        }
        if (!controller.signal.aborted) throw new Error("Observation disconnected");
      } catch {
        if (!controller.signal.aborted) {
          setAction(undefined); setEventError("Observation disconnected. Checking fresh state before reconnect.");
          timer = setTimeout(() => setEventReconnect(value => value + 1), 3000);
        }
      }
    })();
    return () => {controller.abort(); clearTimeout(timer);};
  }, [client, sessionId, eventReconnect]);

  async function send(text: string) {
    if (!client.submit) throw new Error("Submit unavailable");
    const result = await client.submit({sessionId, attemptId: crypto.randomUUID(), text, writeIntent: write ? ActionWriteIntent.WRITE : ActionWriteIntent.READ});
    setAction(result.state);
    setReload(value => value + 1);
    if (result.outcome !== SubmitOutcome.ACCEPTED) {
      const blocker = result.outcome === SubmitOutcome.REJECTED
        ? result.state?.blocker ?? ActionBlocker.FRESH_SNAPSHOT_REQUIRED : ActionBlocker.UNRESOLVED_OUTCOME;
      throw new ConnectError("Submission was not accepted", Code.FailedPrecondition, undefined,
        [{desc: ActionFailureSchema, value: {blocker}}]);
    }
    setContentRevision(value => value + 1);
  }

  async function changeWriter(kind: "acquire" | "release") {
    const result = await (kind === "acquire" ? client.acquireWriter({sessionId}) : client.releaseWriter({sessionId}));
    if (!result.state) throw new Error("Writer state unavailable");
    setReload(value => value + 1);
    return result.state;
  }

  const error = executionError || actionError;
  const externalError = executionExternal || actionExternal || pendingExternal || cardExternal;
  const closeEligible = !closeAttemptRecorded && !closeExternalRejected && !eligibilityPending && !!action?.flags?.canRequestSessionClose && !action.flags.blockedByOutcomeUnknown &&
    !action.flags.requiresOperatorAction && execution?.freshness === Freshness.FRESH && execution.closeProgress === CloseProgress.NONE &&
    (!action.flags.requiresCloseConfirmation || interrupt);
  const actionsCurrent = execution?.freshness === Freshness.FRESH && !!action && !eligibilityPending && !externalError && !closeExternalRejected && !action.flags?.requiresOperatorAction;

  async function close() {
    if (closePending.current || !closeEligible) return;
    closePending.current = true; setClosing(true); setCloseMessage("");
    try {
      const result = await client.closeRuntime({sessionId, interrupt, attemptId: crypto.randomUUID()});
      if (result.outcome?.status === CloseStatus.IN_PROGRESS) {setCloseAttemptRecorded(true); setCloseMessage("Close request accepted; provider settlement pending.");}
      else if (result.outcome?.status === CloseStatus.OUTCOME_UNKNOWN || result.outcome?.status === CloseStatus.UNSPECIFIED || !result.outcome) {
        setCloseAttemptRecorded(true); setCloseMessage(closeOutcomeUnknown);
      }
      else if (result.outcome.status === CloseStatus.RECOVERY_REQUIRED) {setCloseAttemptRecorded(true); setCloseMessage(closeRecoveryRequired);}
      else if (result.outcome.status === CloseStatus.REJECTED) {
        const rejection = result.outcome.rejection;
        if (rejection && domainErrorRequiresExternalAction(rejection)) {
          setCloseExternalRejected(true);
          setCloseAttemptRecorded(true);
        }
        setCloseMessage(rejection ? `Close rejected. ${domainErrorMessage(rejection.code, rejection.action)}` : "Close rejected. Check current state before another request.");
      }
      else {setCloseAttemptRecorded(true); setCloseMessage("Close response received. Checking the current provider projection.");}
      setReload(value => value + 1);
    } catch {setCloseAttemptRecorded(true); setCloseMessage(closeOutcomeUnknown);}
    finally {closePending.current = false; setClosing(false);}
  }

  return <div className="session-detail">
    {eventError && <p role="alert">{eventError}</p>}
    <section className="session-detail__interactions" aria-label="Action required">
      <h3>Action required {pendingCount ? `(${pendingCount})` : ""}</h3>
      {cardError && <p role="alert">{cardError}</p>}
      {cards.length ? <>{!actionsCurrent && <p role="alert">Current eligibility is unavailable. Interaction details remain visible; responses are disabled.</p>}
        {cards.map(card => <InteractionCardView key={card.summary?.interactionId} card={card} actionable={actionsCurrent}
          respond={responseJson => client.resolve({sessionId, interactionId: card.summary!.interactionId, responseJson})} />)}</>
        : pendingCount === 0 ? <p>No pending interaction requests.</p>
          : pendingFailed ? <p role="alert">{pendingNotice}</p>
            : pendingCount === undefined ? <p role="status">Loading interaction requests…</p>
            : <p role="alert">Interaction details are unavailable. Check provider state before acting.</p>}
    </section>
    <div className="session-detail__summary">
      <h3>Current activity</h3>
      {!externalError && !closeExternalRejected && !action?.flags?.requiresOperatorAction && <button type="button" onClick={() => {setReload(value => value + 1); setContentRevision(value => value + 1);}}>Refresh current state</button>}
      {error && <p role="alert">{error}</p>}
      {!execution && !error && <p role="status">Loading current state…</p>}
      {execution && <>
        <ProviderStatus state={execution.providerState} freshness={execution.freshness} />
        <p>Activity: {SessionLifecycle[execution.lifecycle] ?? "Unavailable"}</p>
        <p>Approval policy: {ApprovalPolicy[execution.approvalPolicy] ?? "Unavailable"}</p>
        <p>Specialist policy: {execution.specialistPolicyName || "Unavailable"}</p>
        <p>Pending approvals: {execution.counts?.pendingApprovals?.toString() ?? "Unavailable"}</p>
        <p role={execution.closeProgress === CloseProgress.OUTCOME_UNKNOWN || execution.closeProgress === CloseProgress.RECOVERY_REQUIRED ? "alert" : "status"}>{closeLabel(execution.closeProgress)}</p>
      </>}
      {action && <><WriterStatus mode={action.mode} /><ActionBlockerMessage blocker={action.blocker} />
        {action.flags?.requiresOperatorAction && <p role="alert">Operator action required in Dolgorae. Follow the provider recovery or migration procedure.</p>}
        {action.flags?.blockedByOutcomeUnknown && <p role="alert">Outcome unresolved. Inspect the provider before acting.</p>}
      </>}
    </div>
    <div className="session-detail__tabs" role="group" aria-label="Conversation sections">
      <button type="button" ref={conversationTab} aria-current={view === "conversation" ? "page" : undefined} onClick={() => setView("conversation")}>Conversation</button>
      <button type="button" aria-current={view === "history" ? "page" : undefined} onClick={() => setView("history")}>Prompt History</button>
    </div>
    <div hidden={view !== "conversation"}><ConversationPanel refreshRevision={contentRevision} sessionId={sessionId} client={client}
      focusEntryId={focusEntryId} focusRequest={focusRequest} active={view === "conversation"}
      focusAnchor={conversationTab} /></div>
    <div hidden={view !== "history"}><PromptHistoryPanel refreshRevision={contentRevision} sessionId={sessionId} client={client}
      onOpenTurn={entryId => {setFocusEntryId(entryId); setFocusRequest(value => value + 1);
        setView("conversation"); conversationTab.current?.focus();}} /></div>
    {client.submit && <section aria-label="Submit prompt">
      <label>Prompt access <select value={write ? "write" : "read"} onChange={event => setWrite(event.currentTarget.value === "write")}><option value="read">Read</option><option value="write">Write</option></select></label>
      <PromptDraft state={actionsCurrent && !eventError ? action : undefined} write={write} send={send} />
    </section>}
    {client.listSpecialistResults && <SpecialistResults sessionId={sessionId} client={client} refreshRevision={contentRevision} />}
    {action && actionsCurrent && <WriterPanel state={action} acquire={() => changeWriter("acquire")} release={() => changeWriter("release")} onState={setAction} />}
    <section aria-label="Session close">
      <h3>Close session</h3>
      <p>Close coordinates all owned work. Active work may be interrupted.</p>
      <InterruptConsent confirmed={interrupt} evaluate={setInterrupt} />
      <button type="button" disabled={closing || !closeEligible} onClick={() => void close()}>Request whole-session close</button>
      {closeMessage && <p role="alert">{closeMessage}</p>}
    </section>
  </div>;
}
