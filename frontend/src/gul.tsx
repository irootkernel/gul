import {StrictMode} from "react";
import {createRoot, type Root} from "react-dom/client";
import {FoundationApp} from "./app";
import {OperatorApp, type OperatorClients} from "./operator-app";
import {AuthenticatedApp} from "./auth-app";
import type {AuthClient} from "./auth-client";
import {createBrowserAuth} from "./auth-client";
import {createOperatorClients} from "./operator-clients";
import {consumeNativeBootstrap} from "./native-bootstrap";
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

export function mountAuthenticated(root: HTMLElement | null, auth: AuthClient, clients: OperatorClients) {
  applicationRoot(root).render(<StrictMode><AuthenticatedApp client={auth}><OperatorApp clients={clients} /></AuthenticatedApp></StrictMode>);
}

export function startBrowser(root: HTMLElement | null) {
  try {
    const auth = createBrowserAuth(consumeNativeBootstrap());
    mountAuthenticated(root, auth, createOperatorClients(auth.transport));
  } catch {
    mountFoundation(root);
  }
}

if (typeof document !== "undefined") {
  startBrowser(document.getElementById("root"));
  if ("serviceWorker" in navigator && location.protocol === "https:") {
    void navigator.serviceWorker.register("/service-worker.js", {scope: "/"}).catch(() => {});
  }
}
