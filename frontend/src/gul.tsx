import {StrictMode} from "react";
import {createRoot, type Root} from "react-dom/client";
import {FoundationApp} from "./app";
import {OperatorApp, type OperatorClients} from "./operator-app";
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

export function mountOperator(root: HTMLElement | null, clients: OperatorClients) {
  applicationRoot(root).render(<StrictMode><OperatorApp clients={clients} /></StrictMode>);
}

if (typeof document !== "undefined") mountFoundation(document.getElementById("root"));
