import {useRef, useState, type FormEvent} from "react";
import {
  InteractionCardDecision, InteractionCardStatus, InteractionResolutionOutcome,
  type InteractionCard, type ResolveResponse, type UserInputCard,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {useImeSubmitGuard} from "./ime-submit";

type Respond = (body: Uint8Array) => Promise<ResolveResponse>;
const responseLimit = 64 * 1024;

export function collectAnswers(values: FormData, questions: UserInputCard["questions"]) {
  const answers: Record<string, {answers: string[]}> = Object.create(null);
  for (const [index, question] of questions.entries()) {
    const selected = values.getAll(`answer-${index}`).filter((value): value is string => typeof value === "string" && value !== "");
    if (!selected.length) return undefined;
    answers[question.questionId] = {answers: selected};
  }
  return answers;
}

// The caller supplies one non-retrying RPC. No response material is retained
// for reconnect, logging, browser storage or automatic replay.
export async function sendInteractionResponse(body: Uint8Array, respond: Respond): Promise<ResolveResponse | undefined> {
  try {
    if (!body.byteLength || body.byteLength > responseLimit) return undefined;
    return await respond(body);
  } catch {
    return undefined;
  } finally {
    body.fill(0);
  }
}

export function InteractionCardView(props: {card: InteractionCard; respond: Respond; actionable?: boolean}) {
  return <InteractionCardContent key={props.card.summary?.interactionId} {...props} />;
}

function InteractionCardContent({card, respond, actionable = true}: {card: InteractionCard; respond: Respond; actionable?: boolean}) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [feedback, setFeedback] = useState<{basis: InteractionCard; outcome: InteractionOutcome}>();
  const outcome = feedback?.basis === card ? feedback.outcome : undefined;
  const inFlight = useRef(false);
  const ime = useImeSubmitGuard();
  const active = actionable && card.summary?.status === InteractionCardStatus.PENDING && card.actions?.canResolveInteraction === true && (!outcome || outcome === "pending");
  const detail = card.detail;

  async function send(value: unknown, acceptedOutcome: InteractionOutcome) {
    if (!active || inFlight.current) return;
    const body = new TextEncoder().encode(JSON.stringify(value));
    inFlight.current = true;
    setBusy(true);
    setMessage("");
    setFeedback(undefined);
    const result = await sendInteractionResponse(body, respond);
    inFlight.current = false;
    setBusy(false);
    setFeedback({basis: card, outcome: result?.outcome === InteractionResolutionOutcome.RESOLVED
      ? result.resolutionReceipt ? acceptedOutcome : "resolved_elsewhere"
      : result?.outcome === InteractionResolutionOutcome.REENTER ? "pending"
        : result?.outcome === InteractionResolutionOutcome.STALE ? "expiration" : "provider_failure"});
  }

  function answer(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (detail.case !== "userInput" || inFlight.current || !active || ime.blocksSubmit()) return;
    const form = event.currentTarget;
    const values = new FormData(form);
    const answers = collectAnswers(values, detail.value.questions);
    if (!answers) {
      setMessage("Answer every question before sending.");
      return;
    }
    form.reset();
    void send({answers}, "answer");
  }

  const decisions: [InteractionCardDecision, string, string][] = [
    [InteractionCardDecision.ACCEPT_ONCE, "Approve once", "accept_once"],
    [InteractionCardDecision.DECLINE, "Decline", "decline"],
    [InteractionCardDecision.CANCEL, "Cancel", "cancel"],
  ];
  return <section aria-label="Interaction request">
    {detail.case === "commandApproval" && <>
      <h3>{detail.value.title}</h3><p>{detail.value.message}</p><p>{detail.value.reason}</p>
      <ol aria-label="Command arguments">{detail.value.command.map((argument, index) => <li key={index}><code>{argument}</code></li>)}</ol><p>Directory: {detail.value.relativeWorkingDirectory}</p>
    </>}
    {detail.case === "fileApproval" && <>
      <h3>{detail.value.title}</h3><p>{detail.value.message}</p><p>{detail.value.reason}</p>
      {detail.value.changes.map((change, index) => <div key={index}><p>{change.kind}: {change.relativePath}{change.relativeMovePath ? ` → ${change.relativeMovePath}` : ""}</p><pre>{change.unifiedDiff}</pre></div>)}
      {detail.value.verifiedDiff && <pre>{detail.value.verifiedDiff}</pre>}
    </>}
    {detail.case === "userInput" && <form key={card.summary?.interactionId} onSubmit={answer} autoComplete="off"
      onCompositionStart={ime.onCompositionStart} onCompositionEnd={ime.onCompositionEnd}
      onKeyDown={ime.onKeyDown} onKeyUp={ime.onKeyUp}>
      {detail.value.questions.map((question, index) => <fieldset key={question.questionId} disabled={!active || busy}>
        <legend>{question.header}</legend><p>{question.question}</p>
        {question.choices.map(choice => <label key={choice.label}><input type="checkbox" name={`answer-${index}`} value={choice.label} />{choice.label} — {choice.description}</label>)}
        {(question.allowsOther || !question.choices.length) && <label>Answer<input name={`answer-${index}`}
          aria-label={`Answer for question ${index + 1}: ${question.header}`} type={question.isSecret ? "password" : "text"}
          autoComplete="off" onBlur={ime.onInputBlur} /></label>}
      </fieldset>)}
      <button disabled={!active || busy} type="submit" onPointerDown={ime.explicitSubmit}
        onKeyDown={ime.onSubmitButtonKeyDown}>Send answer</button>
    </form>}
    {detail.case === "unsupported" && <p role="alert">{detail.value.blocker}</p>}
    {!detail.case && <p role="alert">Request details are unavailable.</p>}
    {(detail.case === "commandApproval" || detail.case === "fileApproval") && decisions.filter(([id]) => card.decisions.includes(id)).map(([id, label, value]) =>
      <button key={id} disabled={!active || busy} onClick={() => void send({decision: value}, value === "accept_once" ? "approval" : value === "decline" ? "denial" : "cancellation")}>{label}</button>)}
    {!active && card.summary?.status === InteractionCardStatus.PENDING && <p role="alert">A fresh eligible state is required before responding.</p>}
    {(outcome || card.summary?.status === InteractionCardStatus.STALE || card.summary?.status === InteractionCardStatus.RESOLVED) && <InteractionFeedback outcome={card.summary?.status === InteractionCardStatus.STALE ? "expiration" : outcome ?? "resolved_elsewhere"} />}
    {message && <p role="status">{message}</p>}
  </section>;
}

export type InteractionOutcome = "approval" | "denial" | "answer" | "expiration" | "cancellation" | "provider_failure" | "pending" | "resolved_elsewhere";
const outcomeLabels: Record<InteractionOutcome, string> = {
  approval: "Approval confirmed.", denial: "Denial confirmed.", answer: "Answer confirmed.",
  expiration: "Request expired or is no longer actionable.", cancellation: "Cancellation confirmed.",
  provider_failure: "Provider response unavailable. Outcome unknown; refresh before trying again.",
  pending: "Still pending. Enter your response again.", resolved_elsewhere: "Request is already resolved.",
};
export function InteractionFeedback({outcome}: {outcome: InteractionOutcome}) {
  return <p role={outcome === "provider_failure" ? "alert" : "status"} data-outcome={outcome}>{outcomeLabels[outcome]}</p>;
}
