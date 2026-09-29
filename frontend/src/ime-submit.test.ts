import {expect, test} from "bun:test";
import {createImeSubmitGuard} from "./ime-submit";

const enter = {key: "Enter", code: "Enter", keyCode: 13, isComposing: false};
const chromeCommit = {...enter, keyCode: 229, isComposing: true};

test("composition alone blocks submission and a non-Enter IME key cannot strand it", () => {
  let time = 100;
  const guard = createImeSubmitGuard(() => time);
  guard.compositionStart();
  expect(guard.blocksSubmit()).toBe(true);
  guard.keyDown({key: "Process", code: "KeyA", keyCode: 229, isComposing: true});
  guard.compositionEnd();
  expect(guard.blocksSubmit()).toBe(false);
  guard.explicitSubmit();
  expect(guard.blocksSubmit()).toBe(false);
});

test("both composition/Enter orders block commit submission but allow later Enter", () => {
  let time = 100;
  const guard = createImeSubmitGuard(() => time);
  guard.compositionStart();
  guard.keyDown(chromeCommit);
  guard.compositionEnd();
  expect(guard.blocksSubmit()).toBe(true);
  guard.keyUp(enter);
  expect(guard.blocksSubmit()).toBe(false);

  time = 200;
  guard.compositionStart();
  guard.compositionEnd();
  guard.keyDown(enter);
  expect(guard.blocksSubmit()).toBe(true);
  guard.keyUp(enter);
  expect(guard.blocksSubmit()).toBe(false);
  time = 201;
  guard.keyDown(enter);
  expect(guard.blocksSubmit()).toBe(false);
});

test("a missing keyup expires and an explicit button submission can proceed", () => {
  let time = 100;
  const guard = createImeSubmitGuard(() => time);
  guard.compositionStart();
  guard.keyDown(chromeCommit);
  guard.compositionEnd();
  expect(guard.blocksSubmit()).toBe(true);
  guard.explicitSubmit();
  expect(guard.blocksSubmit()).toBe(false);
  guard.compositionStart();
  guard.keyDown(chromeCommit);
  guard.compositionEnd();
  time = 1100;
  expect(guard.blocksSubmit()).toBe(false);
});

test("the post-composition Enter window is bounded", () => {
  let time = 100;
  const guard = createImeSubmitGuard(() => time);
  guard.compositionStart();
  guard.compositionEnd();
  time = 350;
  guard.keyDown(enter);
  expect(guard.blocksSubmit()).toBe(true);
  guard.keyUp(enter);
  time = 351;
  guard.keyDown(enter);
  expect(guard.blocksSubmit()).toBe(false);
});

test("leaving an input recovers a composition without an end event", () => {
  const guard = createImeSubmitGuard();
  guard.compositionStart();
  expect(guard.blocksSubmit()).toBe(true);
  guard.inputBlur();
  expect(guard.blocksSubmit()).toBe(false);
  guard.compositionStart();
  expect(guard.blocksSubmit()).toBe(true);
});
