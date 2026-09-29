import {useEffect, useRef, useState, type RefObject} from "react";
import {ConversationKind, ConversationStatus, type ConversationEntry, type GetConversationEntryResponse,
  type ListConversationResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {resolvePromptOriginal, type PromptArtifactClient} from "./prompt-original";
import {maximumPreviewBytes} from "../../api/generated/ts/gul/v1/bounds";
import {appendDistinctPage, assertConversationPage, assertSnapshot} from "./paged-provider-list";

export type ConversationClient = PromptArtifactClient & {
  listConversation(request: {sessionId: string; pageToken?: string; pageSize: number}): Promise<ListConversationResponse>;
  getConversationEntry(request: {sessionId: string; entryId: string}): Promise<GetConversationEntryResponse>;
};

function label(entry: ConversationEntry) {
  switch (entry.kind) {
    case ConversationKind.HUMAN: return "You";
    case ConversationKind.ASSISTANT: return "Assistant";
    case ConversationKind.INTERACTION_OPENED: return "Request opened";
    case ConversationKind.INTERACTION_RESOLVED: return "Request resolved";
    case ConversationKind.TURN_TERMINAL: return "Turn finished";
    default: return "Update";
  }
}

export function ConversationPanel({sessionId, client, focusEntryId, focusRequest, active, focusAnchor}: {
  sessionId: string; client: ConversationClient; focusEntryId: string; focusRequest: number; active: boolean;
  focusAnchor: RefObject<HTMLButtonElement | null>;
}) {
  const [entries, setEntries] = useState<ConversationEntry[]>([]);
  const [snapshotId, setSnapshotId] = useState("");
  const [nextToken, setNextToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [focused, setFocused] = useState<ConversationEntry>();
  const [focusError, setFocusError] = useState<{entryId: string; message: string}>();
  const [expanded, setExpanded] = useState<Record<string, string>>({});
  const [expandError, setExpandError] = useState("");
  const generation = useRef(0);
  const requestedFocusId = useRef("");
  const linkedStatus = useRef<HTMLParagraphElement>(null);
  const linkedError = useRef<HTMLParagraphElement>(null);
  const focusedRequest = useRef(0);

  useEffect(() => {
    if (!active || !focusEntryId || focusRequest === focusedRequest.current) return;
    const target = focused?.entryId === focusEntryId ? linkedStatus.current
      : focusError?.entryId === focusEntryId ? linkedError.current : null;
    // A delayed read must not interrupt a control the user focused meanwhile.
    if (target && document.activeElement !== focusAnchor.current) {focusedRequest.current = focusRequest; return;}
    target?.focus();
    if (target && document.activeElement === target) focusedRequest.current = focusRequest;
  }, [active, focused, focusError, focusEntryId, focusRequest, focusAnchor]);

  useEffect(() => {
    let current = true;
    generation.current++;
    setLoading(true); setBusy(false); setEntries([]); setNextToken(""); setSnapshotId(""); setFocused(undefined); setFocusError(undefined); requestedFocusId.current = ""; setError(""); setExpanded({});
    void client.listConversation({sessionId, pageSize: 50}).then(page => {
      if (!current) return;
      assertConversationPage(page);
      setEntries(page.items); setSnapshotId(page.snapshotId); setNextToken(page.nextPageToken ?? "");
    }).catch(() => {if (current) setError("Conversation is unavailable. Refresh to try again.");})
      .finally(() => {if (current) setLoading(false);});
    return () => {current = false;};
  }, [client, sessionId, reload]);

  useEffect(() => {
    let current = true;
    if (!focusEntryId) {setFocused(undefined); setFocusError(undefined); requestedFocusId.current = ""; return () => {current = false;};}
    const loaded = entries.find(entry => entry.entryId === focusEntryId);
    if (loaded) {setFocused(loaded); setFocusError(undefined); return () => {current = false;};}
    if (requestedFocusId.current === focusEntryId) return () => {current = false;};
    requestedFocusId.current = focusEntryId;
    setFocused(undefined); setFocusError(undefined);
    void client.getConversationEntry({sessionId, entryId: focusEntryId})
      .then(result => {if (current) {if (result.entry?.entryId === focusEntryId &&
          new TextEncoder().encode(result.entry.preview).length <= maximumPreviewBytes) setFocused(result.entry);
        else setFocusError({entryId: focusEntryId, message: "The linked Turn is unavailable. Refresh conversation to try again."});}})
      .catch(() => {if (current) setFocusError({entryId: focusEntryId, message: "The linked Turn is unavailable. Refresh conversation to try again."});});
    return () => {current = false;};
  }, [client, sessionId, focusEntryId, entries]);

  async function loadMore() {
    if (!nextToken || busy) return;
    const currentGeneration = generation.current;
    setBusy(true);
    try {
      const page = await client.listConversation({sessionId, pageToken: nextToken, pageSize: 50});
      if (currentGeneration !== generation.current) return;
      assertConversationPage(page);
      assertSnapshot(snapshotId, page.snapshotId);
      setEntries(current => appendDistinctPage(current, page.items, entry => entry.entryId));
      setNextToken(page.nextPageToken ?? ""); setError("");
    } catch {if (currentGeneration === generation.current) setError("More conversation is unavailable. Refresh to try again.");}
    finally {if (currentGeneration === generation.current) setBusy(false);}
  }

  async function openOriginal(entry: ConversationEntry) {
    const currentGeneration = generation.current;
    setExpandError("");
    try {
      const result = await client.getConversationEntry({sessionId, entryId: entry.entryId});
      if (result.entry?.entryId !== entry.entryId || result.entry.kind !== entry.kind) throw new Error("Entry changed");
      const original = await resolvePromptOriginal(client, sessionId, result.original?.content);
      if (currentGeneration === generation.current) setExpanded(current => ({...current, [entry.entryId]: original}));
    } catch {if (currentGeneration === generation.current) setExpandError("Full response is unavailable. Refresh current state before trying again.");}
  }

  return <section aria-label="Conversation timeline">
    <h3>Conversation</h3>
    <button type="button" onClick={() => setReload(value => value + 1)}>Refresh conversation</button>
    {loading && <p role="status">Loading conversation…</p>}
    {error && <p role="alert">{error}</p>}
    {expandError && <p role="alert">{expandError}</p>}
    {focusError && <p role="alert" ref={linkedError} tabIndex={-1}>{focusError.message}</p>}
    {focused && <p role="status" ref={linkedStatus} tabIndex={-1}>Linked Turn: {focused.preview}{!entries.some(entry => entry.entryId === focused.entryId) && " (outside the loaded conversation page)"}</p>}
    {!loading && !entries.length && !error && <p>No conversation entries yet.</p>}
    <ol>{entries.map((entry, index) => <li key={entry.entryId} id={`conversation-${entry.entryId}`} aria-current={focused?.entryId === entry.entryId ? "true" : undefined}>
      <strong>{label(entry)}</strong>
      {entry.status === ConversationStatus.OUTCOME_UNKNOWN && <p role="alert">Outcome unresolved. Refresh current state before acting.</p>}
      {entry.kind === ConversationKind.HUMAN || entry.kind === ConversationKind.ASSISTANT
        ? <><p>{entry.preview}{entry.previewTruncated ? "…" : ""}</p>
          {entry.kind === ConversationKind.ASSISTANT && entry.hasOriginal && <>
            <button type="button" aria-label={`View full response for conversation entry ${index + 1}`}
              onClick={() => void openOriginal(entry)}>View full response</button>
            {expanded[entry.entryId] !== undefined && <pre>{expanded[entry.entryId]}</pre>}
          </>}</>
        : <p>{entry.title || ConversationStatus[entry.status] || "Update"}</p>}
    </li>)}</ol>
    {nextToken && <button type="button" disabled={busy} onClick={() => void loadMore()}>More conversation</button>}
  </section>;
}
