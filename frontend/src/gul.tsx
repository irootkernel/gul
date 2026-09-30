import {StrictMode} from "react";
import {createRoot, type Root} from "react-dom/client";
import {FoundationApp} from "./app";
import {OperatorApp, type OperatorClients} from "./operator-app";
import {AuthenticatedApp} from "./auth-app";
import type {AuthClient} from "./auth-client";
export {createBrowserAuth} from "./auth-client";
import "./styles.css";

let mountedRoot: Root | undefined;
let mountedElement: HTMLElement | undefined;

function applicationRoot(root: HTMLElement | null): Root {
  if (root === null) throw new Error("Gul frontend root is missing");
  if (mountedElement && mountedElement !== root) throw new Error("Gul frontend root changed");
  mountedElement = root;
  mountedRoot ??= createRoot(root);
  return mountedRoot;
}

export function mountFoundation(root: HTMLElement | null) {
  applicationRoot(root).render(
    <StrictMode>
      <FoundationApp />
    </StrictMode>,
  );
}

export function mountOperator(root: HTMLElement | null, clients: OperatorClients, writerActive = false) {
  applicationRoot(root).render(<StrictMode><OperatorApp clients={clients} writerActive={writerActive} /></StrictMode>);
}

// T3 supplies the verified host bootstrap and typed clients sharing the protected
// transport. Automatic startup remains fail-closed until that host exists.
export function mountAuthenticated(root: HTMLElement | null, auth: AuthClient, clients: OperatorClients) {
  applicationRoot(root).render(<StrictMode><AuthenticatedApp client={auth}><OperatorApp clients={clients} /></AuthenticatedApp></StrictMode>);
}

if (typeof document !== "undefined") mountFoundation(document.getElementById("root"));
