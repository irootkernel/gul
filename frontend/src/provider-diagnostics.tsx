import {useEffect, useState} from "react";
import type {GetSummaryResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {operatorError} from "./domain-errors";

export type DiagnosticSummary = Pick<GetSummaryResponse, "providerReady" | "persistenceReady"> & Partial<GetSummaryResponse>;
export type DiagnosticsClient = {getSummary(request: object): Promise<DiagnosticSummary>};

export function ProviderSummary({summary}: {summary: DiagnosticSummary}) {
  const health = summary.providerReady ? "Connected"
    : summary.providerHealth === "incompatible" ? "Incompatible"
    : summary.providerHealth === "restarting" ? "Restarting"
    : summary.providerHealth === "restart_exhausted" ? "Restart limit reached"
    : summary.providerHealth === "collision" ? "Gateway already running" : "Unavailable";
  const capabilities = summary.providerCapabilities;
  return <div role="status" aria-label="Runtime provider">
    <p>Dolgorae: {health}{summary.providerVersion && ` · Version ${summary.providerVersion}`}</p>
    {summary.providerRestartsRemaining !== undefined && <p>Remaining restart attempts: {summary.providerRestartsRemaining}</p>}
    {summary.providerSocketCleanupUnsafe && <p>Gateway socket cleanup requires inspection.</p>}
    {capabilities && <dl>
      <dt>Persistent runs</dt><dd>{capabilities.persistentRuns ? "Supported" : "Unavailable"}</dd>
      <dt>Artifact retrieval</dt><dd>{capabilities.artifactRetrieval ? "Supported" : "Unavailable"}</dd>
      <dt>General reader/writer access</dt><dd>{capabilities.readerWriterAccess ? "Supported" : "Unavailable"}</dd>
      <dt>Dedicated writing</dt><dd>{capabilities.dedicatedWriterSupport ? "Supported" : "Unavailable"}</dd>
      <dt>First WRITE through submission</dt><dd>{capabilities.firstWriteViaSubmitTurn ? "Supported" : "Unavailable"}</dd>
    </dl>}
  </div>;
}

export function ProviderDiagnostics({client}: {client: DiagnosticsClient}) {
  const [summary, setSummary] = useState<DiagnosticSummary>();
  const [error, setError] = useState("");
  useEffect(() => {
    let current = true;
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const value = await client.getSummary({});
        if (current) {setSummary(value); setError("");}
      } catch (reason) {if (current) {setSummary(undefined); setError(operatorError(reason));}}
      if (current) timer = setTimeout(() => void refresh(), 5000);
    };
    void refresh();
    return () => {current = false; clearTimeout(timer);};
  }, [client]);
  return <details><summary>Runtime status</summary>
    {summary ? <ProviderSummary summary={summary} /> : <p role="status">{error || "Checking runtime…"}</p>}
  </details>;
}
