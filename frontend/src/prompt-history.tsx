import {useEffect, useRef, useState} from "react";
import type {GetPromptHistoryItemResponse, ListPromptHistoryResponse, PromptHistoryItem} from "../../api/generated/ts/gul/v1/gul_pb";
import {resolvePromptOriginal, type PromptArtifactClient} from "./prompt-original";
import {appendDistinctPage, assertHistoryPage, assertSnapshot} from "./paged-provider-list";

export type PromptHistoryClient = PromptArtifactClient & {
  listPromptHistory(request: {sessionId: string; pageToken?: string; pageSize: number}): Promise<ListPromptHistoryResponse>;
  getPromptHistoryItem(request: {sessionId: string; promptItemId: string}): Promise<GetPromptHistoryItemResponse>;
};

function acceptedAt(item: PromptHistoryItem) {
  const time = item.acceptedAt;
  if (!time || !Number.isInteger(time.nanos) || time.nanos < 0 || time.nanos >= 1_000_000_000) return "Time unavailable";
  const date = new Date(Number(time.seconds) * 1000 + time.nanos / 1e6);
  return Number.isFinite(date.getTime()) ? date.toISOString() : "Time unavailable";
}

export function PromptHistoryPanel({sessionId, client, onOpenTurn}: {
  sessionId: string; client: PromptHistoryClient; onOpenTurn: (entryId: string) => void;
}) {
  const [items, setItems] = useState<PromptHistoryItem[]>([]);
  const [snapshotId, setSnapshotId] = useState("");
  const [nextToken, setNextToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [selected, setSelected] = useState("");
  const [original, setOriginal] = useState("");
  const [originalLoading, setOriginalLoading] = useState(false);
  const [originalError, setOriginalError] = useState("");
  const [originalReload, setOriginalReload] = useState(0);
  const generation = useRef(0);
  const selectedItem = items.find(entry => entry.promptItemId === selected);

  useEffect(() => {
    let current = true;
    generation.current++;
    setItems([]); setSnapshotId(""); setNextToken(""); setSelected(""); setOriginal(""); setError(""); setLoading(true); setBusy(false);
    void client.listPromptHistory({sessionId, pageSize: 50}).then(page => {
      if (!current) return;
      assertHistoryPage(page);
      setItems(page.items); setSnapshotId(page.snapshotId); setNextToken(page.nextPageToken ?? "");
    }).catch(() => {if (current) setError("Prompt History is unavailable. Refresh to try again.");})
      .finally(() => {if (current) setLoading(false);});
    return () => {current = false;};
  }, [client, sessionId, reload]);

  useEffect(() => {
    let current = true;
    setOriginal(""); setOriginalError(""); setOriginalLoading(false);
    const item = selectedItem;
    if (!item) return () => {current = false;};
    setOriginalLoading(true);
    void client.getPromptHistoryItem({sessionId, promptItemId: selected}).then(async detail => {
      if (detail.promptItemId !== item.promptItemId || detail.ordinal !== item.ordinal ||
          detail.conversationEntryId !== item.conversationEntryId) throw new Error("Prompt identity changed");
      return resolvePromptOriginal(client, sessionId, detail.original?.content);
    }).then(text => {if (current) setOriginal(text);})
      .catch(() => {if (current) setOriginalError("Full original is unavailable. Select it again to retry.");})
      .finally(() => {if (current) setOriginalLoading(false);});
    return () => {current = false;};
  }, [client, sessionId, selected, selectedItem?.ordinal, selectedItem?.conversationEntryId, originalReload]);

  async function loadMore() {
    if (!nextToken || busy) return;
    const currentGeneration = generation.current;
    setBusy(true);
    try {
      const page = await client.listPromptHistory({sessionId, pageToken: nextToken, pageSize: 50});
      if (currentGeneration !== generation.current) return;
      assertHistoryPage(page);
      assertSnapshot(snapshotId, page.snapshotId);
      setItems(current => appendDistinctPage(current, page.items, item => item.promptItemId));
      setNextToken(page.nextPageToken ?? ""); setError("");
    } catch {if (currentGeneration === generation.current) setError("More Prompt History is unavailable. Refresh to try again.");}
    finally {if (currentGeneration === generation.current) setBusy(false);}
  }

  return <section aria-label="Prompt History">
    <h3>Prompt History</h3>
    <p>Accepted prompts from this session</p>
    <button type="button" onClick={() => setReload(value => value + 1)}>Refresh Prompt History</button>
    {loading && <p role="status">Loading Prompt History…</p>}
    {error && <p role="alert">{error}</p>}
    {!loading && !items.length && !error && <p>No accepted prompts yet.</p>}
    <ol>{items.map(item => <li key={item.promptItemId}>
      <p>#{item.ordinal.toString()} · {acceptedAt(item)}</p>
      <p>{item.preview}{item.previewTruncated ? "…" : ""}</p>
      <button type="button" onClick={() => {setSelected(item.promptItemId); setOriginalReload(value => value + 1);}}>View full original</button>
      <button type="button" onClick={() => onOpenTurn(item.conversationEntryId)}>Open matching Turn</button>
      {selected === item.promptItemId && <div aria-label="Full original prompt">
        {originalLoading ? <p role="status">Loading original…</p> : originalError ? <p role="alert">{originalError}</p> : <pre>{original}</pre>}
      </div>}
    </li>)}</ol>
    {nextToken && <button type="button" disabled={busy} onClick={() => void loadMore()}>More accepted prompts</button>}
  </section>;
}
