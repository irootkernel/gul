import {create} from "@bufbuild/protobuf";
import {createRoot} from "react-dom/client";
import {
  CompareFixedRevisionsResponseSchema, DirectSessionPresentationSchema, FileEntrySchema,
  FileGitState, FileNodeKind, FilePreviewKind, GetGitStatusResponseSchema,
  ListDirectSessionsResponseSchema, ListDirectoryResponseSchema,
  ListWorkspacesResponseSchema, NavigationResponseSchema, ReadPreviewResponseSchema,
  WorkspaceEntrySchema,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {OperatorApp, type OperatorClients} from "../src/operator-app";
import "../src/styles.css";

let navigation = {workspaceId: "alpha", sessionId: "one"};
const calls = {navigation: 0, directory: 0, preview: 0, compare: 0, refresh: 0};
const faults = {workspaceLoad: new URL(location.href).searchParams.has("workspace-fault") ? 1 : 0,
  sessions: 0, directory: 0, more: 0, preview: 0, compare: 0, refresh: 0, navigation: 0,
  echoCurrentWorkspace: 0, overlappingPage: 0, overlappingDirectory: 0, navigationDelayMs: 0, moreDelayMs: 0, compareDelayMs: 0, refreshDelayMs: 0};
Object.assign(window, {fixture: Object.assign(calls, {faults})});

function fails(key: Exclude<keyof typeof faults, "navigationDelayMs" | "moreDelayMs" | "compareDelayMs" | "refreshDelayMs">) {
  if (!faults[key]) return false;
  faults[key]--;
  return true;
}

const clients: OperatorClients = {
  workspace: {
    listWorkspaces: async () => {
      if (fails("workspaceLoad")) throw new Error("Workspace unavailable");
      return create(ListWorkspacesResponseSchema, {workspaces: [
        create(WorkspaceEntrySchema, {workspaceId: "alpha", displayName: "Alpha"}),
        create(WorkspaceEntrySchema, {workspaceId: "beta", displayName: "Beta"}),
        create(WorkspaceEntrySchema, {workspaceId: "gamma", displayName: "Gamma"}),
        create(WorkspaceEntrySchema, {workspaceId: "secret", displayName: "Hidden", hidden: true}),
      ]});
    },
    getNavigation: async () => create(NavigationResponseSchema,
      new URL(location.href).searchParams.has("stale-nav") ? {workspaceId: "secret", sessionId: ""} : navigation),
    setNavigation: async request => {
      calls.navigation++;
      if (fails("navigation")) throw new Error("Navigation unavailable");
      if (fails("echoCurrentWorkspace")) return create(NavigationResponseSchema, navigation);
      if (faults.navigationDelayMs) await new Promise(resolve => setTimeout(resolve, faults.navigationDelayMs));
      navigation = request;
      return create(NavigationResponseSchema, navigation);
    },
  },
  sessions: {
    listDirectSessions: async ({workspaceId}) => {
      if (fails("sessions")) throw new Error("Sessions unavailable");
      return create(ListDirectSessionsResponseSchema, {sessions:
        workspaceId === "alpha" ? [create(DirectSessionPresentationSchema, {sessionId: "one", workspaceId, displayName: "First session"}),
          create(DirectSessionPresentationSchema, {sessionId: "other", workspaceId, displayName: "Another session"}),
          create(DirectSessionPresentationSchema, {sessionId: "archived", workspaceId, displayName: "Archived session", archived: true})]
          : workspaceId === "beta" ? [create(DirectSessionPresentationSchema, {sessionId: "two", workspaceId, displayName: "Second session"})] : [],
      });
    },
  },
  files: {
    listDirectory: async ({workspaceId, relativePath, pageToken}) => {
      calls.directory++;
      if (fails(pageToken ? "more" : "directory")) throw new Error("Directory unavailable");
      if (pageToken && faults.moreDelayMs) await new Promise(resolve => setTimeout(resolve, faults.moreDelayMs));
      return create(ListDirectoryResponseSchema, {entries: workspaceId === "beta"
        ? (pageToken && faults.overlappingPage
          ? [create(FileEntrySchema, {name: "beta.txt", kind: faults.overlappingDirectory ? FileNodeKind.DIRECTORY : FileNodeKind.REGULAR_FILE, byteLength: 12n}),
              create(FileEntrySchema, {name: "second.txt", kind: FileNodeKind.REGULAR_FILE})]
          : [create(FileEntrySchema, {name: pageToken ? "second.txt" : "beta.txt", kind: FileNodeKind.REGULAR_FILE})])
        : relativePath === "docs"
          ? [create(FileEntrySchema, {name: "README.md", kind: FileNodeKind.REGULAR_FILE})]
          : [create(FileEntrySchema, {name: "docs", kind: FileNodeKind.DIRECTORY}),
              create(FileEntrySchema, {name: ".dolgorae", providerManagedDenied: true})],
        nextPageToken: workspaceId === "beta" && !pageToken ? "more" : "",
      });
    },
    readPreview: async ({relativePath}) => {
      calls.preview++;
      if (fails("preview")) throw new Error("Preview unavailable");
      return create(ReadPreviewResponseSchema, {kind: FilePreviewKind.TEXT, text: `Preview of ${relativePath}`});
    },
    getGitStatus: async ({workspaceId}) => {
      if (workspaceId === "beta") throw new Error("Git unavailable");
      return create(GetGitStatusResponseSchema, {state: FileGitState.NOT_REPOSITORY});
    },
    compareFixedRevisions: async ({relativePath}) => {
      calls.compare++;
      if (faults.compareDelayMs) await new Promise(resolve => setTimeout(resolve, faults.compareDelayMs));
      if (fails("compare")) throw new Error("Comparison unavailable");
      return create(CompareFixedRevisionsResponseSchema, {state: FileGitState.NOT_REPOSITORY,
        working: create(ReadPreviewResponseSchema, {kind: FilePreviewKind.TEXT, text: `Working ${relativePath}`}),
      });
    },
    refreshFiles: async () => {calls.refresh++;
      if (faults.refreshDelayMs) await new Promise(resolve => setTimeout(resolve, faults.refreshDelayMs));
      if (fails("refresh")) throw new Error("Refresh unavailable"); return {};},
  },
};

createRoot(document.getElementById("root")!).render(<OperatorApp clients={clients} writerActive />);
