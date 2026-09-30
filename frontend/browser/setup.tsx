import {createRoot} from "react-dom/client";
import {FirstRunSetup, LocalSetupFailure} from "../src/first-run-setup";

// Explicit component fixture only; this entry is never in the product bundle.
const scenario = new URLSearchParams(location.search).get("case") ?? "success";
const evidence = {calls: 0, matched: false, resolve: () => {}};
Object.assign(window, {fixture: evidence});
createRoot(document.getElementById("root")!).render(<FirstRunSetup localSetupAllowed={scenario !== "remote"} onSetup={async password => {
  evidence.calls++;
  evidence.matched = password === "한글 암호 😀 exact  spaces";
  if (scenario === "failure") throw new Error(`private diagnostic: ${password}`);
  if (scenario === "unavailable") throw new LocalSetupFailure("unavailable");
  if (scenario === "already-configured") throw new LocalSetupFailure("already-configured");
  if (scenario === "pending") await new Promise<void>(resolve => {evidence.resolve = resolve;});
}} />);
