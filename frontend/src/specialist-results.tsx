import {useEffect, useRef, useState} from "react";
import type {ListSpecialistResultsResponse, SpecialistResult} from "../../api/generated/ts/gul/v1/gul_pb";
import {defaultPageSize} from "../../api/generated/ts/gul/v1/bounds";
import {readPromptOriginal, type PromptArtifactClient} from "./prompt-original";
import {assertSnapshot, assertSpecialistPage, appendDistinctPage} from "./paged-provider-list";

type Client = PromptArtifactClient & {listSpecialistResults?: (request: {sessionId: string; pageSize: number; pageToken?: string}) => Promise<ListSpecialistResultsResponse>};
export function SpecialistResults({sessionId, client, refreshRevision}: {sessionId: string; client: Client; refreshRevision: number}) {
  const [items, setItems] = useState<SpecialistResult[]>([]);
  const [snapshot, setSnapshot] = useState("");
  const [next, setNext] = useState("");
  const [error, setError] = useState("");
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    setItems([]); setBody(""); setError(""); setNext(""); setBusy(true);
    void client.listSpecialistResults!({sessionId, pageSize: defaultPageSize}).then(page => {
      if (current !== generation.current) return;
      assertSpecialistPage(page);
      setItems(page.items); setSnapshot(page.snapshotId); setNext(page.nextPageToken ?? "");
    }).catch(() => {if (current === generation.current) setError("Specialist results unavailable. Refresh current state to retry.");})
      .finally(() => {if (current === generation.current) setBusy(false);});
    return () => {generation.current++;};
  }, [client, sessionId, refreshRevision]);
  async function more() {
    const current = generation.current; setBusy(true);
    try {
      const page = await client.listSpecialistResults!({sessionId, pageSize: defaultPageSize, pageToken: next});
      if (current !== generation.current) return;
      assertSpecialistPage(page);
      assertSnapshot(snapshot, page.snapshotId);
      setItems(appendDistinctPage(items, page.items, item => item.resultId)); setNext(page.nextPageToken ?? "");
    } catch {if (current === generation.current) setError("Result snapshot changed. Refresh current state.");}
    finally {if (current === generation.current) setBusy(false);}
  }
  async function open(item: SpecialistResult) {
    const current = generation.current; setBusy(true); setBody(""); setError("");
    try {
      const metadata = await client.getMetadata({sessionId, artifactRef: item.artifactRef});
      if (metadata.byteLength !== item.byteLength || metadata.sha256 !== item.sha256) throw new Error("Result metadata mismatch");
      const text = await readPromptOriginal(client, sessionId, item.artifactRef);
      if (current === generation.current) setBody(text);
    } catch {if (current === generation.current) setError("Specialist result unavailable or failed integrity checks.");}
    finally {if (current === generation.current) setBusy(false);}
  }
  return <section aria-label="Specialist results"><h3>Specialist results</h3>
    <ol>{items.map(item => <li key={item.resultId}><button disabled={busy} onClick={() => void open(item)}>{item.roleLabel} · {item.publicationOrder.toString()}</button></li>)}</ol>
    {next && <button disabled={busy} onClick={() => void more()}>More results</button>}
    {error && <p role="alert">{error}</p>}{body && <pre aria-label="Specialist result original">{body}</pre>}
  </section>;
}
