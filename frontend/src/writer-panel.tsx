import {useRef, useState, type FormEvent} from "react";
import {Code, ConnectError} from "@connectrpc/connect";
import {
  ActionBlocker, ActionFailureSchema, WriterAuthority, WriterEffectiveAccess,
  WriterPolicyVerification, WriterOwner, LaunchExecutionLane, LaunchAssurance,
  WriterAccessMode, type ActionState,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {maximumSubmitTextBytes} from "../../api/generated/ts/gul/v1/bounds";
import {useImeSubmitGuard} from "./ime-submit";

const blockers: Partial<Record<ActionBlocker, string>> = {
  [ActionBlocker.PROVIDER_INCOMPATIBLE]: "Provider state is incompatible or incomplete.",
  [ActionBlocker.FRESH_SNAPSHOT_REQUIRED]: "Refresh current state before acting.",
  [ActionBlocker.CREDENTIAL_BLOCKED]: "Controller access is unavailable.",
  [ActionBlocker.UNRESOLVED_OUTCOME]: "The previous outcome is unresolved.",
  [ActionBlocker.BACKGROUND_BLOCKED]: "Background execution blocks this action.",
  [ActionBlocker.WRITER_BUSY]: "Writer busy. Another owner controls write access.",
  [ActionBlocker.UNSUPPORTED_TRANSITION]: "This session cannot change access. Its source remains unchanged.",
  [ActionBlocker.ACTIVE_TURN_DRAFT]: "A Turn is active. Your new prompt remains a draft.",
  [ActionBlocker.INTERRUPT_CONFIRMATION_REQUIRED]: "Confirm interruption of active work before proceeding.",
  [ActionBlocker.RECOVERY_BLOCKED]: "Fresh recovery eligibility is required.",
  [ActionBlocker.PROVIDER_FAILURE]: "Provider response unavailable. Refresh current state.",
};
export function ActionBlockerMessage({blocker}: {blocker: ActionBlocker}) {
  if (blocker === ActionBlocker.NONE) return null;
  return <p role="alert">{blockers[blocker] ?? "Action eligibility is unavailable."}</p>;
}
function failureBlocker(error: unknown): ActionBlocker {
  if (error instanceof ConnectError) {
    const details = error.findDetails(ActionFailureSchema);
    if (details.length === 1) return details[0]!.blocker;
  }
  return ActionBlocker.UNRESOLVED_OUTCOME;
}
type WriterPanelProps = {
  state: ActionState;
  acquire: () => Promise<ActionState>;
  release: () => Promise<ActionState>;
  onState: (state: ActionState) => void;
};
export function WriterPanel({state, acquire, release, onState}: WriterPanelProps) {
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<{basis: ActionState; blocker: ActionBlocker}>();
  const inFlight = useRef(false);
  const w = state.writer;
  async function mutate(kind: "acquire" | "release") {
    if (inFlight.current || failure?.basis === state || !(kind === "acquire" ? state.flags?.canAcquireWriter : state.flags?.canReleaseWriter)) return;
    inFlight.current = true; setBusy(true); setFailure(undefined);
    try {onState(await (kind === "acquire" ? acquire() : release()));}
    catch (error) {setFailure({basis: state, blocker: failureBlocker(error)});}
    finally {inFlight.current = false; setBusy(false);}
  }
  const owner = w?.owner === WriterOwner.UNOWNED ? "Unowned — another session may acquire now."
    : w?.owner === WriterOwner.THIS_SESSION ? "This session" : "Externally managed; direct takeover is unavailable.";
  return <section aria-label="Writer state">
    <h3>Writer state</h3>
    <dl>
      <dt>Mode</dt><dd>{state.mode === WriterAccessMode.WRITE ? "WRITE" : state.mode === WriterAccessMode.READ_ONLY ? "Read only" : "Blocked"}</dd>
      <dt>Authority</dt><dd>{w ? WriterAuthority[w.authority] ?? "Unknown" : "Unavailable"}</dd>
      <dt>Generation</dt><dd>{w?.generation.toString() ?? "Unavailable"}</dd>
      <dt>Effective access</dt><dd>{w ? WriterEffectiveAccess[w.effectiveAccess] ?? "Unknown" : "Unavailable"}</dd>
      <dt>Policy verification</dt><dd>{w ? WriterPolicyVerification[w.policyVerification] ?? "Unknown" : "Unavailable"}</dd>
      <dt>Lane</dt><dd>{w?.lane ? LaunchExecutionLane[w.lane] : "Not applicable"}</dd>
      <dt>Requested assurance</dt><dd>{w?.requestedAssurance ? LaunchAssurance[w.requestedAssurance] : "Not applicable"}</dd>
      <dt>Achieved assurance</dt><dd>{w?.achievedAssurance ? LaunchAssurance[w.achievedAssurance] : "Not applicable"}</dd>
      <dt>Owner</dt><dd>{owner}</dd>
    </dl>
    {w?.backgroundBlocked && <p>Background execution blocks writer changes.</p>}
    {w?.recoveryBlocked && <p>Provider recovery blocks writer changes.</p>}
    <ActionBlockerMessage blocker={failure && failure.basis === state ? failure.blocker : state?.blocker ?? ActionBlocker.FRESH_SNAPSHOT_REQUIRED} />
    <button disabled={busy || failure?.basis === state || !state.flags?.canAcquireWriter} onClick={() => void mutate("acquire")}>Acquire writer</button>
    <button disabled={busy || failure?.basis === state || !state.flags?.canReleaseWriter} onClick={() => void mutate("release")}>Release writer</button>
    <p>Release and acquire are separate operations. No writer is reserved between them.</p>
  </section>;
}

export function PromptDraft({state, write, send}: {state: ActionState | undefined; write: boolean; send: (text: string) => Promise<void>}) {
  const [draft, setDraft] = useState("");
  const [failure, setFailure] = useState<{basis: ActionState | undefined; blocker: ActionBlocker; message: string | undefined}>();
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false);
  const ime = useImeSubmitGuard();
  const tooLarge = new TextEncoder().encode(draft).byteLength > maximumSubmitTextBytes;
  const allowed = failure?.basis !== state && (write ? state?.flags?.canSubmitWrite : state?.flags?.canSubmitRead);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!allowed || tooLarge || !draft || inFlight.current || ime.blocksSubmit()) return;
    inFlight.current = true; setBusy(true);
    try {await send(draft); setDraft("");} catch (error) {setFailure({basis: state, blocker: failureBlocker(error), message: error instanceof ConnectError && error.code === Code.InvalidArgument ? "Check the prompt and submission options, then try again." : undefined});}
    finally {inFlight.current = false; setBusy(false);}
  }
  return <form onSubmit={submit} aria-label="Prompt draft" onCompositionStart={ime.onCompositionStart}
    onCompositionEnd={ime.onCompositionEnd} onKeyDown={ime.onKeyDown} onKeyUp={ime.onKeyUp}>
    <label>Prompt<textarea aria-label="Prompt" value={draft} disabled={busy} onBlur={ime.onInputBlur}
      onChange={event => setDraft(event.currentTarget.value)} /></label>
    {tooLarge && <p role="alert">Prompt exceeds the {maximumSubmitTextBytes.toLocaleString("en-US")} byte limit. Shorten it before sending.</p>}
    {failure && failure.basis === state && failure.message ? <p role="alert">{failure.message}</p> :
      failure && failure.basis === state ? <ActionBlockerMessage blocker={failure.blocker} /> :
      state ? <ActionBlockerMessage blocker={state.blocker} /> : <p role="status">Checking current state…</p>}
    <button disabled={!allowed || tooLarge || busy || !draft} type="submit" onPointerDown={ime.explicitSubmit}
      onKeyDown={ime.onSubmitButtonKeyDown}>Send prompt</button>
  </form>;
}

export function InterruptConsent({confirmed, evaluate}: {confirmed: boolean; evaluate: (confirmed: boolean) => void}) {
  return <label><input type="checkbox" checked={confirmed} onChange={event => evaluate(event.currentTarget.checked)} />I confirm interruption of active owned work.</label>;
}
