import {useRef, useState, type FormEvent} from "react";
import {
  InteractionCardDecision, InteractionCardStatus, InteractionResolutionOutcome,
  type InteractionCard, type ResolveResponse, type UserInputCard,
} from "../../api/generated/ts/gul/v1/gul_pb";

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

export function InteractionCardView({card, respond}: {card: InteractionCard; respond: Respond}) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const inFlight = useRef(false);
  const active = card.summary?.status === InteractionCardStatus.PENDING;
  const detail = card.detail;

  async function send(value: unknown) {
    if (!active || inFlight.current) return;
    const body = new TextEncoder().encode(JSON.stringify(value));
    inFlight.current = true;
    setBusy(true);
    setMessage("");
    const result = await sendInteractionResponse(body, respond);
    inFlight.current = false;
    setBusy(false);
    setMessage(result?.outcome === InteractionResolutionOutcome.RESOLVED ? "Response confirmed."
      : result?.outcome === InteractionResolutionOutcome.REENTER ? "Still pending. Enter your response again."
        : result?.outcome === InteractionResolutionOutcome.STALE ? "This request has expired or is no longer actionable."
          : "Response not confirmed. Refresh the request before trying again.");
  }

  function answer(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (detail.case !== "userInput" || inFlight.current || !active) return;
    const form = event.currentTarget;
    const values = new FormData(form);
    const answers = collectAnswers(values, detail.value.questions);
    if (!answers) {
      setMessage("Answer every question before sending.");
      return;
    }
    form.reset();
    void send({answers});
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
    {detail.case === "userInput" && <form key={card.summary?.interactionId} onSubmit={answer} autoComplete="off">
      {detail.value.questions.map((question, index) => <fieldset key={question.questionId} disabled={!active || busy}>
        <legend>{question.header}</legend><p>{question.question}</p>
        {question.choices.map(choice => <label key={choice.label}><input type="checkbox" name={`answer-${index}`} value={choice.label} />{choice.label} — {choice.description}</label>)}
        {(question.allowsOther || !question.choices.length) && <label>Answer<input name={`answer-${index}`} type={question.isSecret ? "password" : "text"} autoComplete="off" /></label>}
      </fieldset>)}
      <button disabled={!active || busy} type="submit">Send answer</button>
    </form>}
    {detail.case === "unsupported" && <p role="alert">{detail.value.blocker}</p>}
    {!detail.case && <p role="alert">Request details are unavailable.</p>}
    {(detail.case === "commandApproval" || detail.case === "fileApproval") && decisions.filter(([id]) => card.decisions.includes(id)).map(([id, label, value]) =>
      <button key={id} disabled={!active || busy} onClick={() => void send({decision: value})}>{label}</button>)}
    {message && <p role="status">{message}</p>}
  </section>;
}
