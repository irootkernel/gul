import {create} from "@bufbuild/protobuf";
import {createRoot} from "react-dom/client";
import {BrowserSessionSchema} from "../../api/generated/ts/gul/v1/gul_pb";
import {AuthenticatedApp} from "../src/auth-app";
import {AuthFailure, type AuthClient} from "../src/auth-client";
import {LocalSetupFailure} from "../src/first-run-setup";

// Component fixture only; no product transport, account or cookie is created.
const scenario = new URLSearchParams(location.search).get("case") ?? "success";
let configured = !["setup", "setup-denied", "setup-rate", "remote"].includes(scenario);
let authenticated = false;
const listeners = new Set<(value: boolean) => void>();
const evidence = {calls: 0, logoutCalls: 0, matched: false, resolve: () => {}, expire: () => {
  authenticated = false; for (const listener of listeners) listener(false);
}};
Object.assign(window, {fixture: evidence});
const client: AuthClient = {
  localSetupAllowed: scenario.startsWith("setup"),
  async getSession() {
    if (scenario === "unavailable") throw new Error("private account detail");
    return create(BrowserSessionSchema, {accountConfigured: configured, authenticated});
  },
  async setup(password) {
    evidence.calls++; evidence.matched = password === "한글 암호 😀 exact  spaces";
    if (scenario === "setup-denied" || scenario === "setup-rate") {
      // The real client's healthy, unconfigured readback clears auth and
      // notifies subscribers before returning the closed setup failure.
      for (const listener of listeners) listener(false);
      throw new LocalSetupFailure(scenario === "setup-rate" ? "rate-limit" : "denied");
    }
    configured = true;
  },
  async login(password) {
    evidence.calls++; evidence.matched = password === "한글 암호 😀 exact  spaces";
    if (scenario === "credentials") throw new AuthFailure("credentials");
    if (scenario === "rate") throw new AuthFailure("rate-limit");
    if (scenario === "failure") throw new Error("private password detail: " + password);
    if (scenario === "pending") await new Promise<void>(resolve => {evidence.resolve = resolve;});
    authenticated = true; for (const listener of listeners) listener(true);
  },
  async logout() {
    evidence.logoutCalls++;
    authenticated = false; for (const listener of listeners) listener(false);
    if (scenario === "logout-pending") await new Promise<void>(resolve => {evidence.resolve = resolve;});
    if (scenario === "logout-failure" && evidence.logoutCalls === 1) throw new Error("private revocation detail");
  },
  subscribe(listener) {listeners.add(listener); return () => {listeners.delete(listener);};},
};
createRoot(document.getElementById("root")!).render(<AuthenticatedApp client={client}><p>Protected operator</p></AuthenticatedApp>);
