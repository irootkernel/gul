import {afterEach, expect, test} from "bun:test";
import {consumeNativeBootstrap} from "./native-bootstrap";

const originalWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
const originalLocation = Object.getOwnPropertyDescriptor(globalThis, "location");
afterEach(() => {
  if (originalWindow) Object.defineProperty(globalThis, "window", originalWindow);
  else Reflect.deleteProperty(globalThis, "window");
  if (originalLocation) Object.defineProperty(globalThis, "location", originalLocation);
  else Reflect.deleteProperty(globalThis, "location");
});

test("native grant is consumed once on pinned local HTTPS origin", () => {
  const view = {__GUL_NATIVE_BOOTSTRAP__: {setupCredential: "s".repeat(43)}};
  Object.defineProperty(globalThis, "window", {configurable: true, value: view});
  Object.defineProperty(globalThis, "location", {configurable: true, value: {protocol: "https:", hostname: "127.0.0.1"}});
  expect(consumeNativeBootstrap()).toBe("s".repeat(43));
  expect("__GUL_NATIVE_BOOTSTRAP__" in view).toBe(false);
  expect(consumeNativeBootstrap()).toBeUndefined();
});

test("remote and insecure documents discard a stray native grant", () => {
  for (const [protocol, hostname] of [["https:", "gul.example"], ["http:", "127.0.0.1"]]) {
    const view = {__GUL_NATIVE_BOOTSTRAP__: {setupCredential: "s".repeat(43)}};
    Object.defineProperty(globalThis, "window", {configurable: true, value: view});
    Object.defineProperty(globalThis, "location", {configurable: true, value: {protocol, hostname}});
    expect(consumeNativeBootstrap()).toBeUndefined();
    expect("__GUL_NATIVE_BOOTSTRAP__" in view).toBe(false);
  }
});
