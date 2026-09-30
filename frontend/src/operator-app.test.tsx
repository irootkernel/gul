import {afterEach, expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {OperatorApp, type OperatorClients} from "./operator-app";

const originalStorage = Object.getOwnPropertyDescriptor(globalThis, "sessionStorage");
afterEach(() => {
  if (originalStorage) Object.defineProperty(globalThis, "sessionStorage", originalStorage);
  else Reflect.deleteProperty(globalThis, "sessionStorage");
});

test("only admitted presentation tabs survive an initial render", () => {
  for (const [value, expected] of [["chat", "Chat"], ["files", "Files"], ["sessions", "Sessions"], ["untrusted", "Sessions"], [null, "Sessions"]]) {
    Object.defineProperty(globalThis, "sessionStorage", {configurable: true, value: {getItem: () => value}});
    const html = renderToStaticMarkup(<OperatorApp clients={{} as OperatorClients} />);
    expect(html).toContain(`aria-current="page">${expected}</button>`);
    expect(html).toContain("Loading workspaces");
  }
});

test("unavailable presentation storage falls back to Sessions", () => {
  Object.defineProperty(globalThis, "sessionStorage", {configurable: true, get() {throw new Error("storage unavailable");}});
  const html = renderToStaticMarkup(<OperatorApp clients={{} as OperatorClients} />);
  expect(html).toContain('aria-current="page">Sessions</button>');
});
