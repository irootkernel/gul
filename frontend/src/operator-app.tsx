import {useCallback, useEffect, useRef, useState} from "react";
import type {DirectSessionPresentation, NavigationResponse, WorkspaceEntry} from "../../api/generated/ts/gul/v1/gul_pb";
import {FilePane, rootFileLocation, type FileClient, type FileLocation} from "./file-pane";
import {externalActionRequired, operatorError} from "./domain-errors";
import {RuntimeChoices, type RuntimeClient} from "./runtime-choices";
import {ProviderDiagnostics, type DiagnosticsClient} from "./provider-diagnostics";
import {SessionDetail, type SessionActivity, type SessionDetailClient} from "./session-detail";

type WorkspaceClient = {
  listWorkspaces(request: object): Promise<{workspaces: WorkspaceEntry[]}>;
  getNavigation(request: object): Promise<NavigationResponse>;
  setNavigation(request: {workspaceId: string; sessionId: string}): Promise<NavigationResponse>;
};

type SessionClient = {
  listDirectSessions(request: {workspaceId: string}): Promise<{sessions: DirectSessionPresentation[]}>;
};

export type OperatorClients = {workspace: WorkspaceClient; sessions: SessionClient; files: FileClient; details?: SessionDetailClient; runtime?: RuntimeClient; diagnostics?: DiagnosticsClient};

type OperatorTab = "sessions" | "chat" | "files";
const tabStorageKey = "gul.presentation.tab";
function savedTab(): OperatorTab {
  try {
    const value = sessionStorage.getItem(tabStorageKey);
    return value === "chat" || value === "files" ? value : "sessions";
  } catch { return "sessions"; }
}

// The caller supplies clients built from the authenticated transport. Saved
// presentation tab state never replaces fresh server navigation or runtime reads.
export function OperatorApp({clients, writerActive = false}: {clients: OperatorClients; writerActive?: boolean}) {
  const [workspaces, setWorkspaces] = useState<WorkspaceEntry[]>([]);
  const [sessions, setSessions] = useState<DirectSessionPresentation[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [tab, setTab] = useState<OperatorTab>(savedTab);
  const [fileLocations, setFileLocations] = useState<Record<string, FileLocation>>({});
  const [workspaceError, setWorkspaceError] = useState("");
  const [workspaceExternal, setWorkspaceExternal] = useState(false);
  const [sessionsError, setSessionsError] = useState("");
  const [sessionsExternal, setSessionsExternal] = useState(false);
  const [navigationError, setNavigationError] = useState("");
  const [loading, setLoading] = useState(true);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [navigationBusy, setNavigationBusy] = useState(false);
  const [workspaceReload, setWorkspaceReload] = useState(0);
  const [sessionsReload, setSessionsReload] = useState(0);
  const [activity, setActivity] = useState<SessionActivity>();
  const onActivity = useCallback((value: SessionActivity | undefined) => setActivity(value), []);
  const navigationPending = useRef(false);
  const chatHeading = useRef<HTMLHeadingElement>(null);

  useEffect(() => { try { sessionStorage.setItem(tabStorageKey, tab); } catch { /* Storage can be unavailable. */ } }, [tab]);

  useEffect(() => {
    let current = true;
    setLoading(true);
    setWorkspaceError("");
    setWorkspaceExternal(false);
    void Promise.all([clients.workspace.listWorkspaces({}), clients.workspace.getNavigation({})])
      .then(([listed, navigation]) => {
        if (!current) return;
        const visible = listed.workspaces.filter(entry => !entry.hidden);
        setWorkspaces(visible);
        setWorkspaceId(visible.some(entry => entry.workspaceId === navigation.workspaceId) ? navigation.workspaceId : visible[0]?.workspaceId ?? "");
        setSessionId(navigation.sessionId);
        setWorkspaceError("");
      })
      .catch(reason => {if (current) {setWorkspaceError(operatorError(reason, "Workspace navigation is unavailable. Retry workspace load.")); setWorkspaceExternal(externalActionRequired(reason));}})
      .finally(() => {if (current) setLoading(false);});
    return () => {current = false;};
  }, [clients.workspace, workspaceReload]);

  useEffect(() => {
    let current = true;
    setSessions([]);
    setSessionsError("");
    setSessionsExternal(false);
    if (!workspaceId) {setSessionsLoading(false); return () => {current = false;};}
    setSessionsLoading(true);
    void clients.sessions.listDirectSessions({workspaceId})
      .then(result => {if (current) setSessions(result.sessions.filter(entry => !entry.archived));})
      .catch(reason => {if (current) {setSessionsError(operatorError(reason, "Sessions are unavailable. Retry sessions.")); setSessionsExternal(externalActionRequired(reason));}})
      .finally(() => {if (current) setSessionsLoading(false);});
    return () => {current = false;};
  }, [clients.sessions, workspaceId, sessionsReload]);

  async function chooseWorkspace(nextWorkspaceId: string) {
    if (navigationPending.current) return;
    navigationPending.current = true;
    setNavigationBusy(true);
    try {
      const next = await clients.workspace.setNavigation({workspaceId: nextWorkspaceId, sessionId: ""});
      if (next.workspaceId !== workspaceId || next.sessionId !== sessionId) setActivity(undefined);
      setWorkspaceId(next.workspaceId);
      setSessionId(next.sessionId);
      if (next.workspaceId !== workspaceId) setSessions([]);
      setTab("sessions");
      setNavigationError("");
    } catch (reason) {setNavigationError(operatorError(reason, "Workspace selection is unavailable. Try again."));}
    finally {navigationPending.current = false; setNavigationBusy(false);}
  }

  async function chooseSession(nextSessionId: string) {
    if (navigationPending.current) return;
    navigationPending.current = true;
    setNavigationBusy(true);
    try {
      const next = await clients.workspace.setNavigation({workspaceId, sessionId: nextSessionId});
      if (next.sessionId !== sessionId) setActivity(undefined);
      setSessionId(next.sessionId);
      setTab("chat");
      setNavigationError("");
      requestAnimationFrame(() => chatHeading.current?.focus());
    } catch (reason) {setNavigationError(operatorError(reason, "Session selection is unavailable. Try again."));}
    finally {navigationPending.current = false; setNavigationBusy(false);}
  }

  const workspace = workspaces.find(entry => entry.workspaceId === workspaceId);
  const session = sessions.find(entry => entry.sessionId === sessionId);
  const location = fileLocations[workspaceId] ?? rootFileLocation;
  return <main className="operator" aria-label="Gul operator">
    <header className="operator__header"><h1>Gul</h1><p>{workspace?.displayName ?? "Choose a workspace"}</p>
      {session && activity && <p className="operator__header-status" role="status">
        Provider {activity.provider} · {activity.activity} · Writer {activity.writer} · Policy {activity.policy} · Assurance {activity.assurance} · Requests {activity.interactions ?? "Unavailable"}
      </p>}
    </header>
    <nav className="operator__mobile-nav" aria-label="Main sections">
      {(["sessions", "chat", "files"] as const).map(value => <button key={value} type="button" aria-current={tab === value ? "page" : undefined}
        onClick={() => setTab(value)}>{value === "sessions" ? "Sessions" : value === "chat" ? "Chat" : "Files"}</button>)}
    </nav>
    {workspaceError && <p role="alert" className="operator__error">{workspaceError} {!workspaceExternal && <button type="button" onClick={() => setWorkspaceReload(value => value + 1)}>Retry workspace load</button>}</p>}
    {navigationError && <p role="alert" className="operator__error">{navigationError}</p>}
    {loading ? <p role="status">Loading workspaces…</p> : <div className={`operator__panes operator__panes--${tab}`}>
      <aside className="operator__pane operator__sessions" aria-label="Workspaces and sessions">
        <h2>Sessions</h2>
        {clients.diagnostics && <ProviderDiagnostics client={clients.diagnostics} />}
        {clients.runtime && <RuntimeChoices client={clients.runtime} />}
        <label>Workspace <select aria-label="Workspace" value={workspaceId} disabled={navigationBusy} onChange={event => void chooseWorkspace(event.currentTarget.value)}>
          {!workspaceId && <option value="">Choose a workspace</option>}
          {workspaces.map(entry => <option key={entry.workspaceId} value={entry.workspaceId}>{entry.displayName}</option>)}
        </select></label>
        {workspaceId && <ul>{sessions.map(entry => <li key={entry.sessionId}><button type="button" disabled={navigationBusy}
          aria-current={sessionId === entry.sessionId ? "true" : undefined}
          onClick={() => void chooseSession(entry.sessionId)}>{entry.displayName}</button>
          {entry.sessionId === sessionId && activity && <dl className="operator__activity">
            <dt>Provider</dt><dd>{activity.provider}</dd><dt>Activity</dt><dd>{activity.activity}</dd>
            <dt>Writer</dt><dd>{activity.writer}</dd><dt>Policy</dt><dd>{activity.policy}</dd>
            <dt>Assurance</dt><dd>{activity.assurance}</dd><dt>Interactions</dt><dd>{activity.interactions ?? "Unavailable"}</dd>
          </dl>}</li>)}</ul>}
        {sessionsLoading && <p role="status">Loading sessions…</p>}
        {sessionsError && <p role="alert">{sessionsError} {!sessionsExternal && <button type="button" onClick={() => setSessionsReload(value => value + 1)}>Retry sessions</button>}</p>}
        {workspaceId && !sessionsLoading && !sessionsError && !sessions.length && <p>No sessions in this workspace.</p>}
      </aside>
      <section className="operator__pane operator__chat" aria-label="Conversation">
        <h2 ref={chatHeading} tabIndex={-1}>Chat</h2>
        {session ? <><p>{session.displayName}</p>{clients.details
          ? <SessionDetail key={sessionId} sessionId={sessionId} client={clients.details} onActivity={onActivity} />
          : <p>Current session presentation is unavailable.</p>}</> : <p>Select a session to view its conversation.</p>}
      </section>
      <div className="operator__pane operator__files">
        {workspaceId ? <FilePane key={workspaceId} workspaceId={workspaceId} client={clients.files} location={location}
          onLocation={next => setFileLocations(current => ({...current, [workspaceId]: next}))} writerActive={writerActive} />
          : <section aria-label="Workspace files"><h2>Files</h2><p>Select a workspace to browse files.</p></section>}
      </div>
    </div>}
  </main>;
}
