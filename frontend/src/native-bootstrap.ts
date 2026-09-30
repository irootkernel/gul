type NativeBootstrap = {setupCredential?: string};
declare global { interface Window { __GUL_NATIVE_BOOTSTRAP__?: NativeBootstrap; } }

export function consumeNativeBootstrap(): string | undefined {
  const bootstrap = window.__GUL_NATIVE_BOOTSTRAP__;
  delete window.__GUL_NATIVE_BOOTSTRAP__;
  // The host injects only into its pinned local HTTPS document. Reject a
  // stray field on remote origins; the server independently checks the grant.
  return location.protocol === "https:" && location.hostname === "127.0.0.1"
    ? bootstrap?.setupCredential : undefined;
}
