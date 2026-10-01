import {createClient, type Transport} from "@connectrpc/connect";
import {
  ArtifactPresentationService, DirectSessionService, FileService, RuntimeService, DiagnosticsService, ClientEventService,
  InteractionPresentationService, WorkspacePresentationService, WriterActionService,
} from "../../api/generated/ts/gul/v1/gul_pb";
import type {OperatorClients} from "./operator-app";

// All feature calls use the AuthClient's protected Connect transport. Never
// create a second transport here: it would lose CSRF, expiry and cancellation.
export function createOperatorClients(transport: Transport): OperatorClients {
  const workspace = createClient(WorkspacePresentationService, transport);
  const sessions = createClient(DirectSessionService, transport);
  const files = createClient(FileService, transport);
  const artifacts = createClient(ArtifactPresentationService, transport);
  const interactions = createClient(InteractionPresentationService, transport);
  const writer = createClient(WriterActionService, transport);
  const runtime = createClient(RuntimeService, transport);
  const diagnostics = createClient(DiagnosticsService, transport);
  const events = createClient(ClientEventService, transport);
  return {
    runtime, diagnostics,
    workspace,
    sessions,
    files: {
      listDirectory: request => files.listDirectory({...request, relativePath: request.relativePath || "."}),
      readPreview: request => files.readPreview(request),
      getGitStatus: request => files.getGitStatus({...request, relativePath: request.relativePath || "."}),
      compareFixedRevisions: request => files.compareFixedRevisions({...request, relativePath: request.relativePath || "."}),
      refreshFiles: request => files.refreshFiles(request),
    },
    details: {
      submit: request => sessions.submit(request),
      listSpecialistResults: request => sessions.listSpecialistResults(request),
      watch: (request, signal) => events.watchClientEvents(request, {signal}),
      listConversation: request => sessions.listConversation(request),
      getConversationEntry: request => sessions.getConversationEntry(request),
      listPromptHistory: request => sessions.listPromptHistory(request),
      getPromptHistoryItem: request => sessions.getPromptHistoryItem(request),
      getExecutionState: request => sessions.getExecutionState(request),
      closeRuntime: request => sessions.closeRuntime(request),
      getMetadata: request => artifacts.getMetadata(request),
      readChunk: request => artifacts.readChunk(request),
      listPending: request => interactions.listPending(request),
      getCard: request => interactions.getCard(request),
      resolve: request => interactions.resolve(request),
      getActionState: request => writer.getActionState(request),
      acquireWriter: async request => {
        const result = await writer.acquireWriter(request);
        return result.state ? {state: result.state} : {};
      },
      releaseWriter: async request => {
        const result = await writer.releaseWriter(request);
        return result.state ? {state: result.state} : {};
      },
    },
  };
}
