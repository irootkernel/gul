import {useEffect, useRef, useState} from "react";
import {
  ActionCloseIntent, ActionWriteIntent, ApprovalPolicy, CloseProgress, CloseStatus,
  Freshness, InteractionCardStatus, LaunchAssurance, ProviderState, SessionLifecycle, WriterAccessMode,
  type ActionState, type CloseRuntimeResponse, type GetActionStateResponse,
  type GetCardResponse, type GetExecutionStateResponse, type InteractionCard,
  type ListPendingResponse, type ResolveResponse,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {ConversationPanel, type ConversationClient} from "./conversation-panel";
import {domainErrorMessage, domainErrorRequiresExternalAction, externalActionRequired, operatorError} from "./domain-errors";
import {InteractionCardView} from "./interaction-card";
import {PromptHistoryPanel, type PromptHistoryClient} from "./prompt-history";
import {ProviderStatus} from "./provider-status";
import {ActionBlockerMessage, InterruptConsent, WriterPanel} from "./writer-panel";
import {WriterStatus} from "./writer-status";

export type SessionDetailClient = ConversationClient & PromptHistoryClient & {
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
  const [interrupt, setInterrupt] = useState(false);
  const [closeMessage, setCloseMessage] = useState("");
  const [closeAttemptRecorded, setCloseAttemptRecorded] = useState(false);
  const [closeExternalRejected, setCloseExternalRejected] = useState(false);
  const [closing, setClosing] = useState(false);
  const closePending = useRef(false);
  const actionGeneration = useRef(0);
  const lastConsent = useRef(interrupt);

  useEffect(() => {
    let current = true;
    const actionRequest = ++actionGeneration.current;
    setExecution(undefined); setAction(undefined); setEligibilityPending(true); setCards([]); setPendingCount(undefined); setPendingFailed(false); setPendingNotice(""); setCardError("");
    setExecutionError(""); setActionError(""); setExecutionExternal(false); setActionExternal(false); setPendingExternal(false); setCardExternal(false);
    void Promise.allSettled([
      client.getExecutionState({sessionId}),
      client.getActionState({sessionId, writeIntent: ActionWriteIntent.READ, closeIntent: ActionCloseIntent.COMPLETE, interruptConfirmed: interrupt}),
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
      const loaded = await Promise.allSettled(pending.value.summaries.map(summary => client.getCard({sessionId, interactionId: summary.interactionId})));
      if (!current) return;
      setCards(loaded.flatMap((result, index) => {
        if (result.status !== "fulfilled") return [];
        const card = result.value.card;
        if (!card?.summary || card.summary.interactionId !== pending.value.summaries[index]?.interactionId ||
            card.summary.status !== InteractionCardStatus.PENDING) return [];
        return [card];
      }));
      const externalCard = loaded.find(result => result.status === "rejected" && externalActionRequired(result.reason));
      if (externalCard?.status === "rejected") {setCardExternal(true); setCardError(operatorError(externalCard.reason));}
      else if (loaded.some((result, index) => result.status === "rejected" ||
          result.value.card?.summary?.interactionId !== pending.value.summaries[index]?.interactionId ||
          result.value.card?.summary?.status !== InteractionCardStatus.PENDING)) {
        setCardError("Some interaction requests are unavailable. Check current state before acting.");
      }
    });
    return () => {current = false;};
  }, [client, sessionId, reload]);

  useEffect(() => {
    if (lastConsent.current === interrupt) return;
    lastConsent.current = interrupt;
    let current = true;
    const actionRequest = ++actionGeneration.current;
    setEligibilityPending(true);
    void client.getActionState({sessionId, writeIntent: ActionWriteIntent.READ, closeIntent: ActionCloseIntent.COMPLETE, interruptConfirmed: interrupt})
      .then(result => {if (current && actionGeneration.current === actionRequest) {setAction(result.state); setActionError(""); setActionExternal(false);}})
      .catch(reason => {if (current && actionGeneration.current === actionRequest) {setAction(undefined); setActionError(operatorError(reason)); setActionExternal(externalActionRequired(reason));}})
      .finally(() => {if (current && actionGeneration.current === actionRequest) setEligibilityPending(false);});
    return () => {current = false;};
  }, [client, sessionId, interrupt]);

  useEffect(() => {
    if (!execution) {onActivity(undefined); return () => onActivity(undefined);}
    const activity = SessionLifecycle[execution.lifecycle] ?? "Unavailable";
    const policy = ApprovalPolicy[execution.approvalPolicy] ?? "Unavailable";
    const writer = action ? WriterAccessMode[action.mode] ?? "Unavailable" : "Unavailable";
    const assurance = action?.writer?.achievedAssurance ? LaunchAssurance[action.writer.achievedAssurance] ?? "Unavailable" : "Unavailable";
    onActivity({provider: ProviderState[execution.providerState] ?? "Unavailable", activity, writer, policy, assurance, interactions: pendingCount});
    return () => onActivity(undefined);
  }, [execution, action, pendingCount, onActivity]);

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
      {!externalError && !closeExternalRejected && !action?.flags?.requiresOperatorAction && <button type="button" onClick={() => setReload(value => value + 1)}>Refresh current state</button>}
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
      <button type="button" aria-current={view === "conversation" ? "page" : undefined} onClick={() => setView("conversation")}>Conversation</button>
      <button type="button" aria-current={view === "history" ? "page" : undefined} onClick={() => setView("history")}>Prompt History</button>
    </div>
    <div hidden={view !== "conversation"}><ConversationPanel sessionId={sessionId} client={client} focusEntryId={focusEntryId} /></div>
    <div hidden={view !== "history"}><PromptHistoryPanel sessionId={sessionId} client={client} onOpenTurn={entryId => {setFocusEntryId(entryId); setView("conversation");}} /></div>
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
