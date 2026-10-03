import {useEffect, useRef, useState} from "react";
import type {CheckCompatibilityRequest, CheckCompatibilityResponse, ListRuntimeProfilesResponse, CreateSessionResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {LaunchSelection} from "./launch-selection";
import {operatorError} from "./domain-errors";
import {Code, ConnectError} from "@connectrpc/connect";

export type RuntimeClient = {
  listRuntimeProfiles(request: object): Promise<ListRuntimeProfilesResponse>;
  checkCompatibility(request: CheckCompatibilityRequest): Promise<CheckCompatibilityResponse>;
};
export type CreationClient = {
  createSession?(request: {workspaceId: string; attemptId: string; choice: CheckCompatibilityRequest}): Promise<CreateSessionResponse>;
  recoverCreation?(request: {workspaceId: string; attemptId: string}): Promise<CreateSessionResponse>;
};
export function RuntimeChoices({client, creator, workspaceId, pendingAttempts = [], onCreated}: {client: RuntimeClient; creator?: CreationClient; workspaceId?: string; pendingAttempts?: string[]; onCreated?: (sessionId: string) => void}) {
  const [profiles, setProfiles] = useState<ListRuntimeProfilesResponse>();
  const [message, setMessage] = useState("");
  const request = useRef(0);
  const [creating, setCreating] = useState(false);
  const [unresolved, setUnresolved] = useState(false);
  const pending = useRef<{workspaceId: string; attemptId: string} | undefined>(undefined);
  async function create(choice: CheckCompatibilityRequest) {
    if (!creator?.createSession || !workspaceId || pending.current) return;
    pending.current = {workspaceId, attemptId: crypto.randomUUID()};
    setCreating(true);
    try {
      const result = await creator.createSession({...pending.current, choice});
      if (result.outcomeUnknown || !result.sessionId) {setUnresolved(true); setMessage("Session creation is unresolved. Inspect its outcome before creating another session.");}
      else {pending.current = undefined; setMessage("Session created."); onCreated?.(result.sessionId);}
    } catch (reason) {
      if (reason instanceof ConnectError && (reason.code === Code.InvalidArgument || reason.code === Code.Unauthenticated || reason.code === Code.PermissionDenied)) pending.current = undefined;
      else setUnresolved(true);
      setMessage(operatorError(reason));
    }
    finally {setCreating(false);}
  }
  async function recover(attemptId?: string) {
    const request = attemptId && workspaceId ? {workspaceId, attemptId} : pending.current;
    if (!creator?.recoverCreation || !request || creating) return;
    setCreating(true);
    try {
      const result = await creator.recoverCreation(request);
      if (!result.outcomeUnknown && result.sessionId) {pending.current = undefined; setUnresolved(false); setMessage("Session created."); onCreated?.(result.sessionId);}
      else setMessage("Session creation remains unresolved.");
    } catch (reason) {
      if (reason instanceof ConnectError && reason.code === Code.NotFound && !attemptId) {pending.current = undefined; setUnresolved(false);}
      setMessage(operatorError(reason));
    }
    finally {setCreating(false);}
  }
  useEffect(() => {
    let current = true;
    void client.listRuntimeProfiles({}).then(value => {if (current) setProfiles(value);})
      .catch(reason => {if (current) setMessage(operatorError(reason));});
    return () => {current = false; request.current++;};
  }, [client]);
  return <details><summary>Launch compatibility</summary>
    {profiles && <LaunchSelection profiles={profiles.profiles} policyNames={profiles.preprovisionedPolicyNames} sharedReadOnlyWarning={profiles.sharedReadonlyWarning}
      {...(creator?.createSession && workspaceId ? {onCreate: (choice: CheckCompatibilityRequest) => void create(choice)} : {})} creating={creating || unresolved || pendingAttempts.length > 0}
      onCheck={choice => {const current = ++request.current; setMessage("");
        void client.checkCompatibility(choice).then(() => {if (current === request.current) setMessage("Configuration is compatible.");})
          .catch(reason => {if (current === request.current) setMessage(operatorError(reason));});}} />}
    {message && <p role="status">{message}</p>}
    {unresolved && creator?.recoverCreation && <button type="button" disabled={creating} onClick={() => void recover()}>Recover session creation</button>}
    {creator?.recoverCreation && pendingAttempts.map(attemptId => <button type="button" key={attemptId} disabled={creating} onClick={() => void recover(attemptId)}>Recover pending session creation</button>)}
  </details>;
}
