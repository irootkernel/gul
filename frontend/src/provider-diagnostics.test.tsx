import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {renderToStaticMarkup} from "react-dom/server";
import {GetSummaryResponseSchema} from "../../api/generated/ts/gul/v1/gul_pb";
import {ProviderSummary} from "./provider-diagnostics";

test("released general access and dedicated writing remain independent", () => {
  const summary = create(GetSummaryResponseSchema, {providerReady: true, persistenceReady: true, providerVersion: "0.1.3", providerRestartsRemaining: 4,
    providerCapabilities: {readerWriterAccess: false, dedicatedWriterSupport: true, firstWriteViaSubmitTurn: true}});
  const html = renderToStaticMarkup(<ProviderSummary summary={summary} />);
  expect(html).toContain("Connected");
  expect(html).toContain("Version 0.1.3");
  expect(html).toContain("General reader/writer access</dt><dd>Unavailable");
  expect(html).toContain("Dedicated writing</dt><dd>Supported");
});

test("provider loss changes availability without promoting cached support to readiness", () => {
  const summary = create(GetSummaryResponseSchema, {providerHealth: "restarting", persistenceReady: true});
  const html = renderToStaticMarkup(<ProviderSummary summary={summary} />);
  expect(html).toContain("Restarting");
  expect(html).not.toContain("Connected");
});
