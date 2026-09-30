import {describe, expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {FirstRunSetup, validSetupPassword} from "./first-run-setup";

describe("local password setup", () => {
  test("has no remote setup form, username or second-account surface", () => {
    const denied = renderToStaticMarkup(<FirstRunSetup localSetupAllowed={false} onSetup={async () => {}} />);
    expect(denied).toContain("Open Gul on this Mac");
    expect(denied).not.toContain("<form");
    const allowed = renderToStaticMarkup(<FirstRunSetup localSetupAllowed onSetup={async () => {}} />);
    expect(allowed.match(/<input /g)).toHaveLength(1);
    expect(allowed).toContain('type="password"');
    expect(allowed).toContain('autoComplete="new-password"');
    expect(allowed).not.toContain("username");
  });

  test("matches the UTF-8 byte and Unicode character policy", () => {
    expect(validSetupPassword("short password")).toBe(false);
    expect(validSetupPassword("한".repeat(15))).toBe(true);
    expect(validSetupPassword("😀".repeat(15))).toBe(true);
    expect(validSetupPassword("a".repeat(1024))).toBe(true);
    expect(validSetupPassword("a".repeat(1025))).toBe(false);
    expect(validSetupPassword("한".repeat(342))).toBe(false);
    expect(validSetupPassword("a".repeat(15) + "\ud800")).toBe(false);
  });
});
