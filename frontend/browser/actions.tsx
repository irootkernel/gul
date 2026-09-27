import {useState} from "react";
import {createRoot} from "react-dom/client";
import {create} from "@bufbuild/protobuf";
import {Code, ConnectError} from "@connectrpc/connect";
import {InteractionCardView} from "../src/interaction-card";
import {WriterPanel, PromptDraft, InterruptConsent} from "../src/writer-panel";
import {
  ActionStateSchema, ActionBlocker, ActionFailureSchema, WriterOwner, WriterAuthority,
  WriterEffectiveAccess, WriterPolicyVerification, InteractionCardSchema,
  InteractionCardStatus, InteractionCardDecision, ResolveResponseSchema, InteractionResolutionOutcome,
} from "../../api/generated/ts/gul/v1/gul_pb";

// Explicit fake boundary for browser behavior; this fixture is never bundled in the app.
const scenario = new URLSearchParams(location.search).get("case") ?? "approval";
const evidence = {calls: 0, cleared: false, keys: [] as string[], matched: false, consent: false};
Object.assign(window, {fixture: evidence});
const card = create(InteractionCardSchema, {
  summary: {interactionId: scenario, status: scenario === "expiration" ? InteractionCardStatus.STALE : InteractionCardStatus.PENDING},
  actions: {canResolveInteraction: true},
  decisions: [InteractionCardDecision.ACCEPT_ONCE, InteractionCardDecision.DECLINE, InteractionCardDecision.CANCEL],
  detail: scenario === "answer" ? {case: "userInput", value: {questions: [
    {questionId: "__proto__", header: "Prototype question", question: "First answer", isSecret: true},
    {questionId: "reset", header: "Reset question", question: "Second answer", isSecret: true},
  ]}} : {case: "commandApproval", value: {title: "Command request", command: ["fixture"]}},
});
function Controls() {
  const [state, setState] = useState(create(ActionStateSchema, {
    blocker: scenario === "prompt" ? ActionBlocker.ACTIVE_TURN_DRAFT : ActionBlocker.NONE,
    flags: {canReleaseWriter: scenario === "writer", canAcquireWriter: scenario === "busy", canSubmitWrite: scenario.startsWith("prompt_write_")},
    writer: {owner: scenario === "writer" ? WriterOwner.THIS_SESSION : WriterOwner.UNOWNED, generation: 8n, authority: WriterAuthority.ACTIVE, effectiveAccess: WriterEffectiveAccess.WRITE, policyVerification: WriterPolicyVerification.VERIFIED},
  }));
  const [consent, setConsent] = useState(false);
  return <>
    {scenario.startsWith("prompt") ? <>
      <PromptDraft state={state} write={scenario !== "prompt"} send={async () => {
        evidence.calls++;
        if (scenario === "prompt_write_unknown") throw new Error("private diagnostic");
        if (scenario.startsWith("prompt_write_")) throw new ConnectError("private diagnostic", Code.FailedPrecondition, undefined, [{desc: ActionFailureSchema, value: create(ActionFailureSchema, {blocker: scenario === "prompt_write_busy" ? ActionBlocker.WRITER_BUSY : ActionBlocker.UNSUPPORTED_TRANSITION})}]);
      }} />
      <button onClick={() => setState(create(ActionStateSchema, {blocker: ActionBlocker.NONE, flags: {canSubmitRead: true}}))}>Observe terminal Turn</button>
      <InterruptConsent confirmed={consent} evaluate={value => {evidence.consent = value; setConsent(value);}} />
    </> : <WriterPanel state={state} onState={setState} release={async () => {
      evidence.calls++;
      return create(ActionStateSchema, {blocker: ActionBlocker.FRESH_SNAPSHOT_REQUIRED, flags: {requiresFreshSnapshot: true}, writer: {owner: WriterOwner.UNOWNED, generation: 8n}});
    }} acquire={async () => {
      evidence.calls++;
      throw new ConnectError("private diagnostic", Code.FailedPrecondition, undefined, [{desc: ActionFailureSchema, value: create(ActionFailureSchema, {blocker: ActionBlocker.WRITER_BUSY})}]);
    }} />}
  </>;
}
createRoot(document.getElementById("root")!).render((["writer", "busy"].includes(scenario) || scenario.startsWith("prompt")) ? <Controls /> : <InteractionCardView card={card} respond={async body => {
  evidence.calls++;
  if (scenario === "answer") {
    const value = JSON.parse(new TextDecoder().decode(body));
    evidence.keys = Object.keys(value.answers);
    evidence.matched = value.answers.__proto__?.answers[0] === "first-fixture" && value.answers.reset?.answers[0] === "second-fixture";
  }
  setTimeout(() => {evidence.cleared = body.every(byte => byte === 0);}, 0);
  if (scenario === "provider_failure") throw new Error("private diagnostic");
  return create(ResolveResponseSchema, {outcome: InteractionResolutionOutcome.RESOLVED, resolutionReceipt: "receipt"});
}} />);
