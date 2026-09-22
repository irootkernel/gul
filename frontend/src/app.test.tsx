import {describe, expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {FoundationApp} from "./app";
import {mountFoundation} from "./gul";

describe("FoundationApp", () => {
  test("renders a fail-closed foundation instead of product controls", () => {
    const markup = renderToStaticMarkup(<FoundationApp />);

    expect(markup).toContain("<h1 id=\"foundation-title\">Gul</h1>");
    expect(markup).toContain("Product access remains unavailable until authentication is configured.");
    for (const control of ["<a ", "<button", "<form", "<input", "<select", "<textarea"]) {
      expect(markup).not.toContain(control);
    }
  });

  test("fails closed when the browser root is missing", () => {
    expect(() => mountFoundation(null)).toThrow("Gul frontend root is missing");
  });
});
