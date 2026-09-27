import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {renderToStaticMarkup} from "react-dom/server";
import {ActionStateSchema, ActionBlocker, WriterAuthority, WriterEffectiveAccess, WriterPolicyVerification, WriterOwner, WriterAccessMode} from "../../api/generated/ts/gul/v1/gul_pb";
import {WriterPanel, PromptDraft, InterruptConsent} from "./writer-panel";
import {InteractionFeedback, type InteractionOutcome} from "./interaction-card";

test("all six interaction outcomes have distinct visible text", () => {
 const outcomes: InteractionOutcome[] = ["approval", "denial", "answer", "expiration", "cancellation", "provider_failure"];
 const rendered = outcomes.map(outcome => renderToStaticMarkup(<InteractionFeedback outcome={outcome} />));
 expect(new Set(rendered).size).toBe(6);
 for (const [index, html] of rendered.entries()) expect(html).toContain(`data-outcome="${outcomes[index]}"`);
 expect(rendered[5]).toContain('role="alert"'); expect(rendered[5]).toContain("Outcome unknown");
});
test("writer view shows provider state and only enables evaluated writer actions", () => {
 const state = create(ActionStateSchema, {flags: {canAcquireWriter: true}, blocker: ActionBlocker.NONE, mode: WriterAccessMode.READ_ONLY,
  writer: {authority: WriterAuthority.NONE, generation: 8n, effectiveAccess: WriterEffectiveAccess.UNKNOWN, policyVerification: WriterPolicyVerification.UNVERIFIED, owner: WriterOwner.UNOWNED}});
 const noCall = async () => {throw Error("render must not mutate");};
 let html = renderToStaticMarkup(<WriterPanel state={state} acquire={noCall} release={noCall} onState={() => {}} />);
 expect(html).toContain("Unowned"); expect(html).toContain("<dd>8</dd>");
 expect(html).toContain("<button>Acquire writer</button>"); expect(html).toContain('disabled="">Release writer');
 state.flags!.canAcquireWriter = false; state.writer!.owner = WriterOwner.EXTERNAL_MANAGEMENT; state.blocker = ActionBlocker.WRITER_BUSY;
 html = renderToStaticMarkup(<WriterPanel state={state} acquire={noCall} release={noCall} onState={() => {}} />);
 expect(html).toContain("Externally managed"); expect(html).toContain("Writer busy"); expect(html).toContain('disabled="">Acquire writer');
 expect(html).not.toContain("Take over");
});
test("active Turn presentation retains drafts and explicit consent begins unchecked", () => {
 const state = create(ActionStateSchema, {flags: {canResolveInteraction: true}, blocker: ActionBlocker.ACTIVE_TURN_DRAFT});
 const html = renderToStaticMarkup(<PromptDraft state={state} write={false} send={async () => {}} />);
 expect(html).toContain("remains a draft"); expect(html).toContain('disabled="" type="submit"');
 const consent = renderToStaticMarkup(<InterruptConsent confirmed={false} evaluate={() => {}} />);
 expect(consent).not.toContain("checked="); expect(consent).toContain("confirm interruption");
});
