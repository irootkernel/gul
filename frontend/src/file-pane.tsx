import {useEffect, useLayoutEffect, useRef, useState} from "react";
import {FileNodeKind, type CompareFixedRevisionsResponse, type FileEntry,
  type GetGitStatusResponse, type ReadPreviewResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {FileComparison, FileStatus} from "./file-comparison";
import {FilePreview} from "./file-preview";

export type FileClient = {
  listDirectory(request: {workspaceId: string; relativePath: string; pageSize: number; pageToken: string}): Promise<{entries: FileEntry[]; nextPageToken: string}>;
  readPreview(request: {workspaceId: string; relativePath: string}): Promise<ReadPreviewResponse>;
  getGitStatus(request: {workspaceId: string; relativePath: string}): Promise<GetGitStatusResponse>;
  compareFixedRevisions(request: {workspaceId: string; relativePath: string}): Promise<CompareFixedRevisionsResponse>;
  refreshFiles(request: {workspaceId: string}): Promise<unknown>;
};

export type FileLocation = {directory: string; selected: string; view: "explorer" | "preview"};
export const rootFileLocation: FileLocation = {directory: "", selected: "", view: "explorer"};

function childPath(directory: string, name: string) {
  return directory ? `${directory}/${name}` : name;
}

function parentPath(directory: string) {
  return directory.slice(0, Math.max(0, directory.lastIndexOf("/")));
}

export function FilePane({workspaceId, client, location, onLocation, writerActive}: {
  workspaceId: string;
  client: FileClient;
  location: FileLocation;
  onLocation: (location: FileLocation) => void;
  writerActive: boolean;
}) {
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [nextPageToken, setNextPageToken] = useState("");
  const [preview, setPreview] = useState<ReadPreviewResponse>();
  const [status, setStatus] = useState<GetGitStatusResponse>();
  const [comparison, setComparison] = useState<CompareFixedRevisionsResponse>();
  const [directoryError, setDirectoryError] = useState("");
  const [previewError, setPreviewError] = useState("");
  const [gitError, setGitError] = useState("");
  const [actionError, setActionError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [busy, setBusy] = useState(false);
  const [directoryLoading, setDirectoryLoading] = useState(false);
  const scope = useRef("");
  const locationEpoch = useRef(0);
  const focusTarget = useRef<"path" | "back" | "selected" | null>(null);
  const pathRef = useRef<HTMLElement>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);
  scope.current = `${workspaceId}\0${location.directory}\0${location.selected}\0${refresh}`;

  useLayoutEffect(() => {locationEpoch.current++;}, [workspaceId, location.directory, location.selected, location.view]);

  useLayoutEffect(() => {
    // These destinations come from clicks in the visible pane, so one focus attempt suffices.
    if (focusTarget.current === "path" && location.view === "explorer") pathRef.current?.focus();
    else if (focusTarget.current === "back" && location.view === "preview") backRef.current?.focus();
    else if (focusTarget.current === "selected" && location.view === "explorer")
      (selectedRef.current ?? pathRef.current)?.focus();
    focusTarget.current = null;
  }, [location.directory, location.selected, location.view]);

  useEffect(() => {
    let current = true;
    setEntries([]);
    setNextPageToken("");
    setDirectoryError("");
    setDirectoryLoading(true);
    void client.listDirectory({workspaceId, relativePath: location.directory, pageSize: 50, pageToken: ""})
      .then(page => { if (current) {setEntries(page.entries); setNextPageToken(page.nextPageToken);} })
      .catch(() => { if (current) setDirectoryError("Directory is unavailable. Refresh to try again."); })
      .finally(() => {if (current) setDirectoryLoading(false);});
    return () => {current = false;};
  }, [client, workspaceId, location.directory, refresh]);

  useEffect(() => {setActionError("");}, [workspaceId, location.directory, location.selected, location.view]);

  useLayoutEffect(() => {
    let current = true;
    setPreview(undefined);
    setStatus(undefined);
    setComparison(undefined);
    setPreviewError("");
    setGitError("");
    if (!location.selected) return () => {current = false;};
    const request = {workspaceId, relativePath: location.selected};
    void client.readPreview(request)
      .then(nextPreview => {if (current) setPreview(nextPreview);})
      .catch(() => {if (current) setPreviewError("File preview is unavailable. Refresh to try again.");});
    void client.getGitStatus(request)
      .then(nextStatus => {if (current) setStatus(nextStatus);})
      .catch(() => {if (current) setGitError("Git status is unavailable. File browsing remains available.");});
    return () => {current = false;};
  }, [client, workspaceId, location.selected, refresh]);

  async function loadMore() {
    if (!nextPageToken || busy) return;
    const token = nextPageToken;
    const requestScope = scope.current;
    setBusy(true);
    try {
      const page = await client.listDirectory({workspaceId, relativePath: location.directory, pageSize: 50, pageToken: token});
      if (scope.current === requestScope) {
        setEntries(items => [...items, ...page.entries]);
        setNextPageToken(page.nextPageToken);
        setDirectoryError("");
      }
    } catch { if (scope.current === requestScope) setDirectoryError("More files are unavailable. Try again."); }
    finally {setBusy(false);}
  }

  async function review() {
    if (!location.selected || busy) return;
    const requestScope = scope.current;
    const requestEpoch = locationEpoch.current;
    setBusy(true);
    try {
      const result = await client.compareFixedRevisions({workspaceId, relativePath: location.selected});
      if (scope.current === requestScope && locationEpoch.current === requestEpoch) {setComparison(result); setActionError("");}
    } catch { if (scope.current === requestScope && locationEpoch.current === requestEpoch) setActionError("Git review is unavailable. The current preview remains available."); }
    finally {setBusy(false);}
  }

  async function refreshFiles() {
    if (busy) return;
    const requestScope = scope.current;
    const requestEpoch = locationEpoch.current;
    setBusy(true);
    try {
      await client.refreshFiles({workspaceId});
      if (scope.current === requestScope && locationEpoch.current === requestEpoch) {
        setRefresh(value => value + 1);
        setActionError("");
      }
    } catch {if (scope.current === requestScope && locationEpoch.current === requestEpoch) setActionError("File refresh is unavailable. Try again.");}
    finally {setBusy(false);}
  }

  const selectedName = location.selected.split("/").at(-1);
  return <section className="file-pane" aria-label="Workspace files">
    <header className="file-pane__header">
      <h2>Files</h2>
      <button type="button" disabled={busy} onClick={() => void refreshFiles()}>Refresh files</button>
    </header>
    {writerActive && <p className="file-pane__warning" role="status">A writer is active. Previews may show intermediate file state.</p>}
    {actionError && <p role="alert">{actionError}</p>}
    {location.view === "preview" && location.selected ? <>
      <button type="button" ref={backRef} onClick={() => {focusTarget.current = "selected"; onLocation({...location, view: "explorer"});}}>Back to explorer</button>
      <h3>{selectedName}</h3>
      {status && <FileStatus status={status} />}
      {gitError && <p role="status">{gitError}</p>}
      {previewError && <p role="alert">{previewError}</p>}
      {preview ? <FilePreview preview={preview} /> : !previewError && <p role="status">Loading preview…</p>}
      <button type="button" disabled={busy || !preview} onClick={() => void review()}>Compare HEAD and Working</button>
      {comparison && <FileComparison comparison={comparison} />}
    </> : <>
      <nav aria-label="File path" className="file-pane__path" ref={pathRef} tabIndex={-1}>
        {location.directory && <button type="button" onClick={() => {focusTarget.current = "path"; onLocation({directory: parentPath(location.directory), selected: "", view: "explorer"});}}>Parent directory</button>}
        <span>{location.directory || "Workspace root"}</span>
      </nav>
      {directoryError && <p role="alert">{directoryError}</p>}
      {directoryLoading && <p role="status">Loading files…</p>}
      <ul className="file-pane__list">
        {entries.map(entry => <li key={entry.name}>
          {entry.providerManagedDenied ? <span aria-label={`${entry.name}, provider managed, unavailable`}>{entry.name} · Provider managed</span> :
            <button type="button" ref={location.selected === childPath(location.directory, entry.name) ? selectedRef : undefined}
              aria-current={location.selected === childPath(location.directory, entry.name) ? "true" : undefined}
              onClick={() => {focusTarget.current = entry.kind === FileNodeKind.DIRECTORY ? "path" : "back";
                onLocation(entry.kind === FileNodeKind.DIRECTORY
                  ? {directory: childPath(location.directory, entry.name), selected: "", view: "explorer"}
                  : {directory: location.directory, selected: childPath(location.directory, entry.name), view: "preview"});}}>
              {entry.kind === FileNodeKind.DIRECTORY ? "▸ " : ""}{entry.name}
            </button>}
        </li>)}
      </ul>
      {nextPageToken && <button type="button" disabled={busy} onClick={() => void loadMore()}>Load more files</button>}
    </>}
  </section>;
}
