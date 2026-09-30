import {Code, ConnectError, createClient, type Interceptor, type Transport} from "@connectrpc/connect";
import {createConnectTransport} from "@connectrpc/connect-web";
import {AuthService, type BrowserSession} from "../../api/generated/ts/gul/v1/gul_pb";
import {LocalSetupFailure, validSetupPassword} from "./first-run-setup";

export class AuthFailure extends Error {
  constructor(readonly kind: "credentials" | "rate-limit" | "unavailable") { super("Gul authentication failed"); }
}

export type AuthClient = {
  localSetupAllowed: boolean;
  getSession(): Promise<BrowserSession>;
  setup(password: string): Promise<void>;
  login(password: string): Promise<void>;
  logout(): Promise<void>;
  subscribe(listener: (authenticated: boolean) => void): () => void;
};

// The browser shares this transport with every feature client. Cookies are
// browser-owned and same-origin; CSRF and native setup authority stay in memory.
// T3 mounts this factory on the verified HTTPS origin after native bootstrap.
export function createBrowserAuth(nativeSetupSecret?: string): AuthClient & {transport: Transport} {
  if (location.protocol !== "https:") throw new AuthFailure("unavailable");
  if (nativeSetupSecret !== undefined && !/^[A-Za-z0-9_-]{43}$/.test(nativeSetupSecret)) throw new LocalSetupFailure("denied");
  let setupSecret = nativeSetupSecret;
  let csrf = "";
  let active = new AbortController();
  let generation = 0;
  let expiry: ReturnType<typeof setTimeout> | undefined;
  const listeners = new Set<(authenticated: boolean) => void>();
  function clear(keepLogoutCSRF = false) {
    active.abort(); active = new AbortController(); if (!keepLogoutCSRF) csrf = ""; generation++;
    clearTimeout(expiry);
    for (const listener of listeners) listener(false);
  }
  function accept(session: BrowserSession | undefined): BrowserSession {
    if (!session || (session.authenticated && (!session.accountConfigured || !/^[A-Za-z0-9_-]{43}$/.test(session.csrfToken) || !session.expiresAt))) {
      clear(); throw new AuthFailure("unavailable");
    }
    if (!session.authenticated) { clear(); return session; }
    const expires = Number(session.expiresAt!.seconds) * 1000 + session.expiresAt!.nanos / 1e6;
    if (!Number.isFinite(expires) || expires <= Date.now()) { clear(); throw new AuthFailure("credentials"); }
    if (csrf !== session.csrfToken) { active.abort(); active = new AbortController(); generation++; }
    csrf = session.csrfToken;
    clearTimeout(expiry);
    expiry = setTimeout(() => clear(), Math.min(expires - Date.now(), 7 * 24 * 60 * 60 * 1000));
    for (const listener of listeners) listener(true);
    return session;
  }
  function invalidated(error: unknown, captured: number) {
    if (captured !== generation) return;
    const code = ConnectError.from(error).code;
    if (code === Code.Unauthenticated) clear();
    else if (code === Code.Canceled || code === Code.DeadlineExceeded || code === Code.PermissionDenied) {
      // Session expiry or durable revocation cancels idle streams. Confirm the
      // saved cookie state before leaving protected content on screen.
      void client.getSession({}, authCall).then(response => {
        if (captured === generation) accept(response.session);
      }).catch(() => { if (captured === generation) clear(); });
    }
  }
  const protection: Interceptor = next => async request => {
    request.header.set("X-Gul-Request", "1");
    if (csrf) request.header.set("X-Gul-CSRF", csrf);
    const captured = generation;
    const product = request.service.typeName !== AuthService.typeName;
    const call = product ? {...request, signal: AbortSignal.any([request.signal, active.signal])} : request;
    try {
      const response = await next(call);
      if (response.stream) {
        const source = response.message;
        return {...response, message: (async function* () {
          try { yield* source; } catch (error) { invalidated(error, captured); throw error; }
        })()};
      }
      return response;
    } catch (error) { if (product) invalidated(error, captured); throw error; }
  };
  const transport = createConnectTransport({baseUrl: location.origin, useBinaryFormat: true, useHttpGet: false, interceptors: [protection]});
  const authCall = {timeoutMs: 10_000};
  const client = createClient(AuthService, transport);
  function failure(error: unknown): AuthFailure {
    const code = ConnectError.from(error).code;
    return new AuthFailure(code === Code.Unauthenticated ? "credentials" : code === Code.ResourceExhausted ? "rate-limit" : "unavailable");
  }
  async function getSession() {
    try { return accept((await client.getSession({}, authCall)).session); } catch (error) { throw error instanceof AuthFailure ? error : failure(error); }
  }
  return {
    transport,
    get localSetupAllowed() { return setupSecret !== undefined; },
    getSession,
    async setup(password) {
      if (!setupSecret || !validSetupPassword(password)) throw new LocalSetupFailure("denied");
      const bytes = new TextEncoder().encode(password);
      const headers = new Headers({"X-Gul-Setup": setupSecret});
      try {
        await client.firstRunSetup({password: bytes}, {...authCall, headers});
        setupSecret = undefined;
      } catch (error) {
        // A failed/cancelled reply may follow a durable setup commit.
        const state = await getSession().catch(() => undefined);
        if (state?.accountConfigured) { setupSecret = undefined; return; }
        const code = ConnectError.from(error).code;
        throw new LocalSetupFailure(code === Code.AlreadyExists ? "already-configured" : code === Code.PermissionDenied ? "denied" : code === Code.ResourceExhausted ? "rate-limit" : "unavailable");
      } finally { bytes.fill(0); headers.delete("X-Gul-Setup"); }
    },
    async login(password) {
      if (!validSetupPassword(password)) throw new AuthFailure("credentials");
      const bytes = new TextEncoder().encode(password);
      try { accept((await client.login({password: bytes}, authCall)).session); }
      catch (error) { throw error instanceof AuthFailure ? error : failure(error); }
      finally { bytes.fill(0); }
    },
    async logout() {
      clear(true);
      try { await client.logout({}, authCall); clear(); }
      catch (error) { clear(true); throw failure(error); }
    },
    subscribe(listener) { listeners.add(listener); return () => { listeners.delete(listener); }; },
  };
}
