import {describe, expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {AuthenticatedApp, LoginForm} from "./auth-app";
import {AuthFailure, type AuthClient} from "./auth-client";

describe("authentication presentation", () => {
  test("does not render product content before the server session check", () => {
    const client = {localSetupAllowed: false} as AuthClient;
    const html = renderToStaticMarkup(<AuthenticatedApp client={client}><p>Protected operator</p></AuthenticatedApp>);
    expect(html).toContain("Checking your Gul session");
    expect(html).not.toContain("Protected operator");
  });
  test("the login form has one uncontrolled password and no subject field", () => {
    const html = renderToStaticMarkup(<LoginForm onLogin={async () => {}} />);
    expect(html.match(/<input /g)).toHaveLength(1);
    expect(html).toContain('autoComplete="current-password"');
    expect(html).not.toContain("username");
    expect(html).not.toContain('value="');
    expect(new AuthFailure("credentials").message).toBe("Gul authentication failed");
  });
});
