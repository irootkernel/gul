import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {renderToStaticMarkup} from "react-dom/server";
import {InteractionCardSchema, InteractionCardStatus, InteractionCardKind, InteractionCardDecision, ResolveResponseSchema, InteractionResolutionOutcome} from "../../api/generated/ts/gul/v1/gul_pb";
import {InteractionCardView, collectAnswers, sendInteractionResponse} from "./interaction-card";

const respond = async () => create(ResolveResponseSchema, {outcome: InteractionResolutionOutcome.RESOLVED});
test("provider question IDs survive JSON serialization without becoming form property names", () => {
  const card = create(InteractionCardSchema, {summary: {status: InteractionCardStatus.PENDING},
    detail: {case: "userInput", value: {questions: ["__proto__", "reset", "constructor"].map(questionId => ({questionId}))}}});
  if (card.detail.case !== "userInput") throw Error("fixture");
  const values = new FormData();
  values.append("answer-0", "first"); values.append("answer-1", "second");
  expect(collectAnswers(values, card.detail.value.questions)).toBeUndefined();
  values.append("answer-2", "third");
  const decoded = JSON.parse(JSON.stringify(collectAnswers(values, card.detail.value.questions)));
  expect(Object.keys(decoded)).toEqual(["__proto__", "reset", "constructor"]);
  expect(decoded.__proto__.answers).toEqual(["first"]);
  expect(decoded.reset.answers).toEqual(["second"]);
  expect(decoded.constructor.answers).toEqual(["third"]);
  const html = renderToStaticMarkup(<InteractionCardView card={card} respond={respond} />);
  expect(html).toContain('name="answer-0"'); expect(html).toContain('name="answer-1"');
  expect(html).not.toContain('name="reset"'); expect(html).not.toContain('name="__proto__"');
});
test("card kinds render decision context and secret fields without persisted answers", () => {
  const card = create(InteractionCardSchema, {summary: {interactionId: "question", kind: InteractionCardKind.USER_INPUT, status: InteractionCardStatus.PENDING},
    detail: {case: "userInput", value: {isBlocking: true, questions: [{questionId: "token", header: "Credential", question: "Enter access value", isSecret: true}]}}});
  const html = renderToStaticMarkup(<InteractionCardView card={card} respond={respond} />);
  expect(html).toContain('type="password"'); expect(html).toContain('autoComplete="off"');
  expect(html).toContain("Enter access value"); expect(html).not.toContain('value="');
  card.detail = {case: "commandApproval", value: { $typeName: "gul.v1.CommandApprovalCard", title: "Approve", message: "Run command", reason: "Build", command: ["make", "test"], relativeWorkingDirectory: "."}};
  card.decisions = [InteractionCardDecision.ACCEPT_ONCE, InteractionCardDecision.DECLINE, InteractionCardDecision.CANCEL];
  const command = renderToStaticMarkup(<InteractionCardView card={card} respond={respond} />);
  expect(command).toContain("<code>make</code>"); expect(command).toContain("<code>test</code>"); expect(command).toContain("Decline"); expect(command).toContain("Build");
});
test("one-shot responses are cleared after success, rejection and oversize without replay", async () => {
  for (const fail of [false, true]) {
    const body = new TextEncoder().encode('{"answers":{"q":{"answers":["canary-secret"]}}}');
    let calls = 0;
    const result = await sendInteractionResponse(body, async value => {calls++; expect(value).toBe(body); if (fail) throw Error("canary-secret"); return respond();});
    expect(calls).toBe(1); expect(body.every(byte => byte === 0)).toBe(true);
    expect(result === undefined).toBe(fail);
  }
  const large = new Uint8Array(65537).fill(1);
  await sendInteractionResponse(large, async () => {throw Error("must not call");});
  expect(large.every(byte => byte === 0)).toBe(true);
});
