import {useEffect, useRef, useState, type FormEvent} from "react";
import {useImeSubmitGuard} from "./ime-submit";
import {minimumPasswordCharacters, maximumPasswordBytes} from "../../api/generated/ts/gul/v1/bounds";

export const minPasswordCharacters = minimumPasswordCharacters;
export const maxPasswordBytes = maximumPasswordBytes;
const passwordPolicy = `Use at least ${minPasswordCharacters} characters and at most ${maxPasswordBytes} UTF-8 bytes.`;

// The transport maps only the service's closed error categories to this type.
// Raw exception messages never become setup UI text.
export class LocalSetupFailure extends Error {
  constructor(readonly kind: "unavailable" | "denied" | "already-configured" | "rate-limit") {
    super("Local Gul setup failed");
  }
}

export function validSetupPassword(password: string): boolean {
  return password.isWellFormed() && Array.from(password).length >= minPasswordCharacters &&
    new TextEncoder().encode(password).length <= maxPasswordBytes;
}

type Props = {
  localSetupAllowed: boolean;
  onSetup: (password: string) => Promise<void>;
};

// The host-authorized view is injected here. Its display flag grants no server
// authority: the service independently checks the native one-time permission.
export function FirstRunSetup({localSetupAllowed, onSetup}: Props) {
  const input = useRef<HTMLInputElement>(null);
  const ime = useImeSubmitGuard();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [complete, setComplete] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (error && !busy) input.current?.focus();
  }, [error, busy]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!localSetupAllowed || pending.current || complete || !input.current || ime.blocksSubmit()) return;
    const password = input.current.value;
    input.current.value = "";
    if (!validSetupPassword(password)) {
      setError(passwordPolicy);
      input.current.focus();
      return;
    }
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      await onSetup(password);
      setComplete(true);
    } catch (failure) {
      // Never render a transport exception, request body or password.
      if (failure instanceof LocalSetupFailure && failure.kind === "unavailable") {
        setError("Gul cannot safely access its account data. Close Gul and check a verified backup before retrying.");
      } else if (failure instanceof LocalSetupFailure && failure.kind === "rate-limit") {
        setError("Too many setup attempts. Wait a minute before trying again.");
      } else if (failure instanceof LocalSetupFailure && failure.kind === "already-configured") {
        setError("Gul already has a password. Close and reopen Gul locally to sign in.");
      } else {
        setError("Setup could not finish. Reopen Gul locally to try again.");
      }
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  if (complete) return <p role="status">Your Gul password is configured.</p>;
  if (!localSetupAllowed) return <p role="status">Open Gul on this Mac to set your password.</p>;
  return <form onSubmit={submit} onKeyDown={ime.onKeyDown} onKeyUp={ime.onKeyUp} aria-label="Set your Gul password">
    <h1>Set your Gul password</h1>
    <p>Gul has one account. {passwordPolicy}</p>
    <label htmlFor="setup-password">Password</label>
    <input id="setup-password" ref={input} type="password" autoComplete="new-password" required
      onCompositionStart={ime.onCompositionStart} onCompositionEnd={ime.onCompositionEnd} onBlur={ime.onInputBlur}
      aria-describedby="setup-password-error" disabled={busy} />
    <p id="setup-password-error" role="alert">{error}</p>
    <button type="submit" disabled={busy} onClick={ime.explicitSubmit} onKeyDown={ime.onSubmitButtonKeyDown}>{busy ? "Setting password…" : "Set password"}</button>
  </form>;
}
