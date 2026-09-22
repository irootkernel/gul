import {StrictMode} from "react";
import {createRoot} from "react-dom/client";
import {FoundationApp} from "./app";
import "./styles.css";

export function mountFoundation(root: HTMLElement | null) {
  if (root === null) throw new Error("Gul frontend root is missing");
  createRoot(root).render(
    <StrictMode>
      <FoundationApp />
    </StrictMode>,
  );
}

if (typeof document !== "undefined") mountFoundation(document.getElementById("root"));
