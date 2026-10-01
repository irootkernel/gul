import {useEffect, useRef, useState} from "react";
import type {CheckCompatibilityRequest, CheckCompatibilityResponse, ListRuntimeProfilesResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {LaunchSelection} from "./launch-selection";
import {operatorError} from "./domain-errors";

export type RuntimeClient = {
  listRuntimeProfiles(request: object): Promise<ListRuntimeProfilesResponse>;
  checkCompatibility(request: CheckCompatibilityRequest): Promise<CheckCompatibilityResponse>;
};
export function RuntimeChoices({client}: {client: RuntimeClient}) {
  const [profiles, setProfiles] = useState<ListRuntimeProfilesResponse>();
  const [message, setMessage] = useState("");
  const request = useRef(0);
  useEffect(() => {
    let current = true;
    void client.listRuntimeProfiles({}).then(value => {if (current) setProfiles(value);})
      .catch(reason => {if (current) setMessage(operatorError(reason));});
    return () => {current = false; request.current++;};
  }, [client]);
  return <details><summary>Launch compatibility</summary>
    {profiles && <LaunchSelection profiles={profiles.profiles} policyNames={profiles.preprovisionedPolicyNames} sharedReadOnlyWarning={profiles.sharedReadonlyWarning}
      onCheck={choice => {const current = ++request.current; setMessage("");
        void client.checkCompatibility(choice).then(() => {if (current === request.current) setMessage("Configuration is compatible. Session creation requires the runtime integration.");})
          .catch(reason => {if (current === request.current) setMessage(operatorError(reason));});}} />}
    {message && <p role="status">{message}</p>}
  </details>;
}
