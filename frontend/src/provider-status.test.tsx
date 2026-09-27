import {expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {Freshness, ProviderState} from "../../api/generated/ts/gul/v1/gul_pb";
import {ProviderStatus} from "./provider-status";

test("healthy provider displays a current connected snapshot", () => {
  const html = renderToStaticMarkup(<ProviderStatus state={ProviderState.READY} freshness={Freshness.FRESH} />);
  expect(html).toContain(">Connected<");
  expect(html).toContain("Current provider snapshot");
  expect(html).not.toContain("stale");
});

test("provider failures remain distinct and explicitly mark retained snapshots stale", () => {
  for (const [state, label] of [
    [ProviderState.DISCONNECTED, "Disconnected"], [ProviderState.INCOMPATIBLE, "Incompatible"],
    [ProviderState.BUSY, "Busy"], [ProviderState.DEGRADED, "Degraded"],
  ] as const) {
    const html = renderToStaticMarkup(<ProviderStatus state={state} freshness={Freshness.STALE} />);
    expect(html).toContain(`>${label}<`);
    expect(html).toContain("Last known snapshot is stale");
    expect(html).not.toContain("Current provider snapshot");
    expect(html).not.toContain("button");
    expect(html).not.toContain("Closed");
  }
});

test("fresh degraded state and unavailable observations have independent labels", () => {
  let html = renderToStaticMarkup(<ProviderStatus state={ProviderState.DEGRADED} freshness={Freshness.FRESH} />);
  expect(html).toContain(">Degraded<");
  expect(html).toContain("Current provider snapshot");
  html = renderToStaticMarkup(<ProviderStatus state={ProviderState.DISCONNECTED} freshness={Freshness.UNAVAILABLE} />);
  expect(html).toContain("No current snapshot");
  expect(html).not.toContain("Last known snapshot");
  html = renderToStaticMarkup(<ProviderStatus state={99 as ProviderState} freshness={Freshness.UNAVAILABLE} />);
  expect(html).toContain("Status unavailable");
  expect(html).not.toContain("Connected");
});
