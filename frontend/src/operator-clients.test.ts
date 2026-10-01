import {afterEach, expect, test} from "bun:test";
import {create, fromBinary, toBinary} from "@bufbuild/protobuf";
import {
  WriterAccessMode, AcquireWriterResponseSchema, ReleaseWriterResponseSchema, GetActionStateResponseSchema, GetMetadataResponseSchema, NavigationResponseSchema,
  ListDirectSessionsResponseSchema, ListDirectoryRequestSchema, ListDirectoryResponseSchema, LoginResponseSchema,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {createBrowserAuth} from "./auth-client";
import {createOperatorClients} from "./operator-clients";

const originalFetch = globalThis.fetch;
const originalLocation = Object.getOwnPropertyDescriptor(globalThis, "location");
afterEach(() => {
  globalThis.fetch = originalFetch;
  if (originalLocation) Object.defineProperty(globalThis, "location", originalLocation);
  else Reflect.deleteProperty(globalThis, "location");
});

test("typed feature clients use actual Connect methods and the auth transport", async () => {
  Object.defineProperty(globalThis, "location", {configurable: true, value: {protocol: "https:", origin: "https://gul.example"}});
  const csrfToken = "c".repeat(43);
  const calls: string[] = [];
  const paths = [
    "/gul.v1.AuthService/Login", "/gul.v1.WorkspacePresentationService/GetNavigation",
    "/gul.v1.DirectSessionService/ListDirectSessions", "/gul.v1.FileService/ListDirectory",
    "/gul.v1.WriterActionService/GetActionState", "/gul.v1.ArtifactPresentationService/GetMetadata",
  ];
  globalThis.fetch = (async (url, options) => {
    const path = new URL(String(url)).pathname;
    calls.push(path);
    const headers = new Headers(options?.headers);
    expect(headers.get("X-Gul-Request")).toBe("1");
    if (!path.endsWith("/Login")) expect(headers.get("X-Gul-CSRF")).toBe(csrfToken);
    if (path === paths[3]) expect(fromBinary(ListDirectoryRequestSchema, new Uint8Array(options?.body as ArrayBuffer)).relativePath).toBe(".");
    const bytes = path === paths[0] ? toBinary(LoginResponseSchema, create(LoginResponseSchema,
      {session: {accountConfigured: true, authenticated: true, csrfToken,
        expiresAt: {seconds: BigInt(Math.floor(Date.now() / 1000) + 3600)}}}))
      : path === paths[1] ? toBinary(NavigationResponseSchema, create(NavigationResponseSchema, {}))
      : path === paths[2] ? toBinary(ListDirectSessionsResponseSchema, create(ListDirectSessionsResponseSchema, {}))
      : path === paths[3] ? toBinary(ListDirectoryResponseSchema, create(ListDirectoryResponseSchema, {}))
      : path === paths[4] ? toBinary(GetActionStateResponseSchema, create(GetActionStateResponseSchema, {}))
      : path === paths[5] ? toBinary(GetMetadataResponseSchema, create(GetMetadataResponseSchema, {}))
      : undefined;
    if (!bytes) throw new Error(`Unexpected Connect method: ${path}`);
    return new Response(bytes, {headers: {"Content-Type": "application/proto"}});
  }) as typeof globalThis.fetch;

  const auth = createBrowserAuth();
  await auth.login("a".repeat(15));
  const clients = createOperatorClients(auth.transport);
  await clients.workspace.getNavigation({});
  await clients.sessions.listDirectSessions({workspaceId: "workspace"});
  await clients.files.listDirectory({workspaceId: "workspace", relativePath: "", pageSize: 10, pageToken: ""});
  await clients.details!.getActionState({sessionId: "session", writeIntent: 0, closeIntent: 0, interruptConfirmed: false});
  await clients.details!.getMetadata({sessionId: "session", artifactRef: "ref"});
  expect(calls).toEqual(paths);
});

test("writer adapters preserve present state and absent state through Connect", async () => {
  Object.defineProperty(globalThis, "location", {configurable: true, value: {protocol: "https:", origin: "https://gul.example"}});
  let present = true;
  const methods: string[] = [];
  globalThis.fetch = (async url => {
    const method = new URL(String(url)).pathname.split("/").at(-1)!;
    methods.push(method);
    if (method === "Login") return new Response(toBinary(LoginResponseSchema, create(LoginResponseSchema,
      {session: {accountConfigured: true, authenticated: true, csrfToken: "c".repeat(43),
        expiresAt: {seconds: BigInt(Math.floor(Date.now() / 1000) + 3600)}}})), {headers: {"Content-Type": "application/proto"}});
    const state = present ? {state: {mode: WriterAccessMode.WRITE}} : {};
    const bytes = method === "AcquireWriter" ? toBinary(AcquireWriterResponseSchema, create(AcquireWriterResponseSchema, state))
      : method === "ReleaseWriter" ? toBinary(ReleaseWriterResponseSchema, create(ReleaseWriterResponseSchema, state)) : undefined;
    if (!bytes) throw Error(`Unexpected writer method: ${method}`);
    return new Response(bytes, {headers: {"Content-Type": "application/proto"}});
  }) as typeof globalThis.fetch;
  const auth = createBrowserAuth();
  await auth.login("a".repeat(15));
  const details = createOperatorClients(auth.transport).details!;
  const request = {sessionId: "session"};
  for (const withState of [true, false]) {
    present = withState;
    const acquired = await details.acquireWriter(request);
    const released = await details.releaseWriter(request);
    if (withState) {
      expect(acquired.state?.mode).toBe(WriterAccessMode.WRITE);
      expect(released.state?.mode).toBe(WriterAccessMode.WRITE);
    } else {
      expect(acquired).toEqual({});
      expect(released).toEqual({});
    }
  }
  expect(methods).toEqual(["Login", "AcquireWriter", "ReleaseWriter", "AcquireWriter", "ReleaseWriter"]);
});
