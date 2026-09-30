import {afterEach, describe, expect, test} from "bun:test";
import {create, fromBinary, toBinary} from "@bufbuild/protobuf";
import {createClient, Code, ConnectError} from "@connectrpc/connect";
import {FirstRunSetupResponseSchema, GetSessionResponseSchema, LoginRequestSchema, LoginResponseSchema, LogoutResponseSchema, WorkspacePresentationService, NavigationResponseSchema, ClientEventService, ClientEventSchema} from "../../api/generated/ts/gul/v1/gul_pb";
import {createBrowserAuth, AuthFailure} from "./auth-client";
import {LocalSetupFailure} from "./first-run-setup";

const originalFetch = globalThis.fetch;
const originalEncode = TextEncoder.prototype.encode;
const originalLocation = Object.getOwnPropertyDescriptor(globalThis, "location");
afterEach(() => {
  globalThis.fetch = originalFetch;
  TextEncoder.prototype.encode = originalEncode;
  if (originalLocation) Object.defineProperty(globalThis, "location", originalLocation);
  else Reflect.deleteProperty(globalThis, "location");
});
function origin(protocol = "https:") { Object.defineProperty(globalThis, "location", {configurable: true, value: {protocol, origin: `${protocol}//gul.invalid`}}); }
const csrf = "c".repeat(43);
const session = () => ({accountConfigured: true, authenticated: true, csrfToken: csrf, expiresAt: {seconds: BigInt(Math.floor(Date.now() / 1000) + 604800), nanos: 0}});
function reply(schema: Parameters<typeof create>[0], value: Record<string, unknown>) {
  const bytes = toBinary(schema, create(schema, value));
  return new Response(bytes, {headers: {"Content-Type": "application/proto"}});
}
function denied(code: string) { return new Response(JSON.stringify({code, message: "private transport detail"}), {status: code === "unavailable" ? 503 : 401, headers: {"Content-Type": "application/json"}}); }

describe("same-origin browser authentication transport", () => {
  test("rejects malformed native setup authority before any request", () => {
    origin(); let calls = 0;
    globalThis.fetch = (async _url => { calls++; return denied("unavailable"); }) as typeof globalThis.fetch;
    for (const secret of ["s".repeat(42), "s".repeat(44), "!".repeat(43)]) {
      expect(() => createBrowserAuth(secret)).toThrow(LocalSetupFailure);
    }
    expect(calls).toBe(0);
  });
  test("failed setup with unconfigured readback retains native authority and a closed retry category", async () => {
    origin();
    for (const [code, kind] of [["permission_denied", "denied"], ["resource_exhausted", "rate-limit"], ["unavailable", "unavailable"]]) {
      let checks = 0;
      globalThis.fetch = (async url => {
        if (String(url).endsWith("/FirstRunSetup")) return denied(code!);
        checks++;
        return reply(GetSessionResponseSchema, {session: {accountConfigured: false}});
      }) as typeof globalThis.fetch;
      const client = createBrowserAuth("s".repeat(43)); const states: boolean[] = [];
      const unsubscribe = client.subscribe(value => states.push(value));
      const error = await client.setup(" exact 한글 😀 password ").catch(error => error);
      expect(error).toBeInstanceOf(LocalSetupFailure); expect(error.kind).toBe(kind);
      expect(checks).toBe(1); expect(states).toEqual([false]);
      expect(client.localSetupAllowed).toBe(true); unsubscribe();
    }
  });
  test("requires HTTPS and keeps CSRF on every typed feature request", async () => {
    origin("http:"); expect(() => createBrowserAuth()).toThrow(AuthFailure);
    origin();
    let passwordMatches = false;
    let protectedCalls = 0;
    globalThis.fetch = (async (url, options) => {
      expect(String(url).startsWith("https://gul.invalid/")).toBe(true);
      const headers = new Headers(options?.headers);
      expect(headers.get("X-Gul-Request")).toBe("1");
      expect(options?.method).toBe("POST");
      if (String(url).endsWith("/Login")) {
        const request = fromBinary(LoginRequestSchema, options?.body as Uint8Array);
        passwordMatches = new TextDecoder().decode(request.password) === " exact 한글 😀 password ";
        return reply(LoginResponseSchema, {session: session()});
      }
      if (String(url).endsWith("/Logout")) return reply(LogoutResponseSchema, {});
      expect(headers.get("X-Gul-CSRF")).toBe(csrf); protectedCalls++;
      return reply(NavigationResponseSchema, {});
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth();
    await client.login(" exact 한글 😀 password ");
    await createClient(WorkspacePresentationService, client.transport).getNavigation({});
    expect(passwordMatches).toBe(true); expect(protectedCalls).toBe(1);
    await client.logout();
  });
  test("checks saved setup after an uncertain response and forgets native authority", async () => {
    origin(); let setupCalls = 0;
    globalThis.fetch = (async (url, options) => {
      if (String(url).endsWith("/FirstRunSetup")) {
        expect(new Headers(options?.headers).get("X-Gul-Setup")).toBe("s".repeat(43)); setupCalls++;
        return denied("unavailable");
      }
      return reply(GetSessionResponseSchema, {session: {accountConfigured: true}});
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth("s".repeat(43));
    await client.setup(" exact 한글 😀 password ");
    expect(setupCalls).toBe(1); expect(client.localSetupAllowed).toBe(false);
    await expect(client.setup(" exact 한글 😀 password ")).rejects.toThrow("Local Gul setup failed");
  });
  test("sanitizes errors and retains only the CSRF needed to retry uncertain logout", async () => {
    origin(); let failLogout = true; let logoutCalls = 0;
    globalThis.fetch = (async (url, options) => {
      if (String(url).endsWith("/Login")) return reply(LoginResponseSchema, {session: session()});
      if (String(url).endsWith("/Logout")) {
        expect(new Headers(options?.headers).get("X-Gul-CSRF")).toBe(csrf); logoutCalls++;
        if (failLogout) return denied("unavailable");
        return reply(LogoutResponseSchema, {});
      }
      return denied("unauthenticated");
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth(); const states: boolean[] = [];
    const unsubscribe = client.subscribe(value => states.push(value));
    await client.login(" exact 한글 😀 password ");
    await expect(client.logout()).rejects.toThrow("Gul authentication failed");
    expect(states.at(-1)).toBe(false);
    failLogout = false; await client.logout(); expect(logoutCalls).toBe(2);
    await expect(client.login(" exact 한글 😀 password ")).resolves.toBeUndefined();
    await expect(createClient(WorkspacePresentationService, client.transport).getNavigation({})).rejects.toThrow(ConnectError);
    expect(states.at(-1)).toBe(false);
    unsubscribe();
  });
  test("logout aborts outstanding feature requests and rejects invalid password encoding", async () => {
    origin(); let sent = 0; let productSignal: AbortSignal | undefined;
    globalThis.fetch = (async (url, options) => {
      sent++;
      if (String(url).endsWith("/Login")) return reply(LoginResponseSchema, {session: session()});
      if (String(url).endsWith("/Logout")) return reply(LogoutResponseSchema, {});
      productSignal = options?.signal ?? undefined;
      return await new Promise<Response>((_resolve, reject) => {
        productSignal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), {once: true});
      });
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth();
    await expect(client.login("a".repeat(15) + "\ud800")).rejects.toThrow(AuthFailure);
    await expect(client.login("a".repeat(1025))).rejects.toThrow(AuthFailure);
    expect(sent).toBe(0);
    await client.login(" exact 한글 😀 password ");
    const pending = createClient(WorkspacePresentationService, client.transport).getNavigation({});
    const settled = pending.catch(error => error);
    await new Promise(resolve => setTimeout(resolve, 0));
    await client.logout();
    expect(productSignal?.aborted).toBe(true);
    expect((await settled).code).toBe(Code.Canceled);
  });
  test("server cancellation rechecks the saved session before retaining product access", async () => {
    origin(); let checked = 0;
    globalThis.fetch = (async url => {
      if (String(url).endsWith("/Login")) return reply(LoginResponseSchema, {session: session()});
      if (String(url).endsWith("/GetSession")) { checked++; return reply(GetSessionResponseSchema, {session: {accountConfigured: true}}); }
      throw new DOMException("Aborted", "AbortError");
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth(); const states: boolean[] = [];
    const unsubscribe = client.subscribe(value => states.push(value));
    await client.login(" exact 한글 😀 password ");
    await createClient(WorkspacePresentationService, client.transport).getNavigation({}).catch(() => {});
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(checked).toBe(1); expect(states.at(-1)).toBe(false);
    unsubscribe();
  });

  test("streamed messages pass through and a terminal auth error rechecks before retaining access", async () => {
    origin();
    function envelope(flags: number, bytes: Uint8Array) {
      const frame = new Uint8Array(5 + bytes.length);
      frame[0] = flags; new DataView(frame.buffer).setUint32(1, bytes.length);
      frame.set(bytes, 5); return frame;
    }
    for (const code of ["canceled", "permission_denied"]) {
      let checked = 0;
      globalThis.fetch = (async (url, options) => {
        if (String(url).endsWith("/Login")) return reply(LoginResponseSchema, {session: session()});
        if (String(url).endsWith("/GetSession")) { checked++; return reply(GetSessionResponseSchema, {session: {accountConfigured: true}}); }
        expect(new Headers(options?.headers).get("X-Gul-CSRF")).toBe(csrf);
        const message = envelope(0, toBinary(ClientEventSchema, create(ClientEventSchema, {deliverySequence: 7n, sessionId: "saved-session"})));
        const end = envelope(2, new TextEncoder().encode(JSON.stringify({error: {code, message: "private stream detail"}})));
        const body = new Uint8Array(message.length + end.length); body.set(message); body.set(end, message.length);
        return new Response(body, {headers: {"Content-Type": "application/connect+proto"}});
      }) as typeof globalThis.fetch;
      const client = createBrowserAuth(); const states: boolean[] = [];
      const unsubscribe = client.subscribe(value => states.push(value));
      await client.login(" exact 한글 😀 password ");
      const messages: bigint[] = [];
      let failure: unknown;
      try { for await (const event of createClient(ClientEventService, client.transport).watchClientEvents({})) messages.push(event.deliverySequence); }
      catch (error) { failure = error; }
      await new Promise(resolve => setTimeout(resolve, 0));
      expect(messages).toEqual([7n]); expect(failure).toBeInstanceOf(ConnectError);
      expect(ConnectError.from(failure).code).toBe(code === "canceled" ? Code.Canceled : Code.PermissionDenied);
      expect(checked).toBe(1); expect(states).toEqual([true, false]); unsubscribe();
    }
  });

  test("a stale CSRF after another tab logs in refreshes authority without replaying the failed feature", async () => {
    origin(); let checked = 0; let calls = 0;
    const rotated = "r".repeat(43);
    globalThis.fetch = (async (url, options) => {
      if (String(url).endsWith("/Login")) return reply(LoginResponseSchema, {session: session()});
      if (String(url).endsWith("/Logout")) return reply(LogoutResponseSchema, {});
      if (String(url).endsWith("/GetSession")) { checked++; return reply(GetSessionResponseSchema, {session: {...session(), csrfToken: rotated}}); }
      calls++;
      if (calls === 1) return denied("permission_denied");
      expect(new Headers(options?.headers).get("X-Gul-CSRF")).toBe(rotated);
      return reply(NavigationResponseSchema, {});
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth();
    await client.login(" exact 한글 😀 password ");
    const feature = createClient(WorkspacePresentationService, client.transport);
    await expect(feature.getNavigation({})).rejects.toThrow(ConnectError);
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(calls).toBe(1); expect(checked).toBe(1);
    await feature.getNavigation({}); expect(calls).toBe(2);
    await client.logout();
  });

  test("the projected expiry removes idle access and clears the CSRF", async () => {
    origin(); let lastHeader: string | null = null;
    globalThis.fetch = (async (url, options) => {
      if (String(url).endsWith("/Login")) {
        const end = Date.now() + 100;
        return reply(LoginResponseSchema, {session: {...session(), expiresAt: {seconds: BigInt(Math.floor(end / 1000)), nanos: (end % 1000) * 1e6}}});
      }
      lastHeader = new Headers(options?.headers).get("X-Gul-CSRF");
      return denied("unauthenticated");
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth();
    let ended!: () => void;
    const expired = new Promise<void>(resolve => { ended = resolve; });
    const states: boolean[] = [];
    const unsubscribe = client.subscribe(value => { states.push(value); if (!value) ended(); });
    await client.login(" exact 한글 😀 password ");
    await expired;
    expect(states).toEqual([true, false]);
    await createClient(WorkspacePresentationService, client.transport).getNavigation({}).catch(() => {});
    expect(lastHeader).toBeNull(); unsubscribe();
  });

  test("converted password buffers are cleared after both successful and failed auth calls", async () => {
    origin(); let lastPassword: Uint8Array | undefined;
    const password = " exact 한글 😀 password ";
    TextEncoder.prototype.encode = function(value) {
      const bytes = originalEncode.call(this, value);
      if (value === password) lastPassword = bytes;
      return bytes;
    };
    let fail = false;
    globalThis.fetch = (async url => {
      if (String(url).endsWith("/GetSession")) return reply(GetSessionResponseSchema, {session: {accountConfigured: false}});
      if (fail) return denied("unavailable");
      if (String(url).endsWith("/FirstRunSetup")) return reply(FirstRunSetupResponseSchema, {});
      if (String(url).endsWith("/Logout")) return reply(LogoutResponseSchema, {});
      return reply(LoginResponseSchema, {session: session()});
    }) as typeof globalThis.fetch;
    for (const failed of [false, true]) {
      fail = failed;
      const client = createBrowserAuth("s".repeat(43));
      await client.setup(password).catch(() => {});
      expect(lastPassword!.every(byte => byte === 0)).toBe(true);
      await client.login(password).catch(() => {});
      expect(lastPassword!.every(byte => byte === 0)).toBe(true);
      if (!failed) await client.logout();
    }
  });

  test("stalled logout removes product access immediately and has a bounded deadline", async () => {
    origin(); let signal: AbortSignal | undefined;
    globalThis.fetch = (async (url, options) => {
      if (String(url).endsWith("/Login")) return reply(LoginResponseSchema, {session: session()});
      signal = options?.signal ?? undefined;
      return await new Promise<Response>((_resolve, reject) => {
        signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), {once: true});
      });
    }) as typeof globalThis.fetch;
    const client = createBrowserAuth(); const states: boolean[] = [];
    const unsubscribe = client.subscribe(value => states.push(value));
    await client.login(" exact 한글 😀 password ");
    const pending = client.logout().catch(error => error);
    expect(states.at(-1)).toBe(false);
    expect(await pending).toBeInstanceOf(AuthFailure);
    expect(signal?.aborted).toBe(true);
    unsubscribe();
  }, 15_000);
});
