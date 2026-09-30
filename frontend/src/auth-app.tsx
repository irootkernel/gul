import {useEffect, useRef, useState, type FormEvent, type ReactNode} from "react";
import {AuthFailure, type AuthClient} from "./auth-client";
import {FirstRunSetup} from "./first-run-setup";
import {useImeSubmitGuard} from "./ime-submit";

export function AuthenticatedApp({client, children}: {client: AuthClient; children: ReactNode}) {
  const [state, setState] = useState<"loading" | "setup" | "login" | "authenticated" | "unavailable">("loading");
  const [reload, setReload] = useState(0);
  const [logoutError, setLogoutError] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);
  useEffect(() => {
    let current = true;
    setState("loading");
    void client.getSession().then(session => {
      if (current) setState(session.authenticated ? "authenticated" : session.accountConfigured ? "login" : "setup");
    }).catch(() => { if (current) setState("unavailable"); });
    const unsubscribe = client.subscribe(authenticated => {
      if (current) setState(previous => authenticated ? "authenticated" : previous === "setup" ? "setup" : "login");
    });
    return () => { current = false; unsubscribe(); };
  }, [client, reload]);
  async function logout() {
    if (loggingOut) return;
    setLoggingOut(true); setLogoutError(false);
    try { await client.logout(); } catch { setLogoutError(true); }
    finally { setState("login"); setLoggingOut(false); }
  }
  if (state === "loading") return <p role="status">Checking your Gul session…</p>;
  if (loggingOut) return <p role="status">Signing out…</p>;
  if (state === "unavailable") return <section><p role="alert">Gul authentication is unavailable.</p><button onClick={() => setReload(value => value + 1)}>Retry session check</button></section>;
  if (state === "setup") return <FirstRunSetup localSetupAllowed={client.localSetupAllowed} onSetup={async password => {
    await client.setup(password); setState("login");
  }} />;
  if (state === "login") return <><LoginForm onLogin={password => client.login(password)} />{logoutError && <p role="alert">Sign-out could not be confirmed. Retry sign-out to revoke the saved session.</p>}{logoutError && <button onClick={() => void logout()} disabled={loggingOut}>Retry sign-out</button>}</>;
  return <><button onClick={() => void logout()} disabled={loggingOut}>{loggingOut ? "Signing out…" : "Sign out"}</button>{children}</>;
}

export function LoginForm({onLogin}: {onLogin: (password: string) => Promise<void>}) {
  const input = useRef<HTMLInputElement>(null);
  const pending = useRef(false);
  const ime = useImeSubmitGuard();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { if (error && !busy) input.current?.focus(); }, [error, busy]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current || !input.current || ime.blocksSubmit()) return;
    const password = input.current.value;
    input.current.value = "";
    pending.current = true; setBusy(true); setError("");
    try { await onLogin(password); }
    catch (failure) { setError(failure instanceof AuthFailure && failure.kind === "credentials" ? "The password is incorrect. Try again." : failure instanceof AuthFailure && failure.kind === "rate-limit" ? "Too many sign-in attempts. Wait a minute before trying again." : "Sign-in is unavailable. Try again later."); }
    finally { pending.current = false; setBusy(false); }
  }
  return <form aria-label="Sign in to Gul" onSubmit={submit} onKeyDown={ime.onKeyDown} onKeyUp={ime.onKeyUp}>
    <h1>Sign in to Gul</h1><label htmlFor="login-password">Password</label>
    <input id="login-password" ref={input} type="password" autoComplete="current-password" required disabled={busy} aria-describedby="login-password-error"
      onCompositionStart={ime.onCompositionStart} onCompositionEnd={ime.onCompositionEnd} onBlur={ime.onInputBlur} />
    <p id="login-password-error" role="alert">{error}</p>
    <button type="submit" disabled={busy} onClick={ime.explicitSubmit} onKeyDown={ime.onSubmitButtonKeyDown}>{busy ? "Signing in…" : "Sign in"}</button>
  </form>;
}
