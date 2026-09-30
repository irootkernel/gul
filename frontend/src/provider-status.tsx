import {Freshness, ProviderState} from "../../api/generated/ts/gul/v1/gul_pb";

// Provider observation health never grants mutation authority or changes the
// displayed Run lifecycle. Authenticated product composition remains with E8/E14.
export function ProviderStatus({state, freshness}: {state: ProviderState; freshness: Freshness}) {
  const label = state === ProviderState.READY ? "Connected"
    : state === ProviderState.DISCONNECTED ? "Disconnected"
    : state === ProviderState.INCOMPATIBLE ? "Incompatible"
    : state === ProviderState.BUSY ? "Busy"
    : state === ProviderState.DEGRADED ? "Degraded" : "Status unavailable";
  const observation = freshness === Freshness.FRESH ? "Current provider snapshot"
    : freshness === Freshness.STALE ? "Last known snapshot is stale" : "No current snapshot";
  return <div role="status" aria-label="Provider status"><span>{label}</span><p>{observation}</p></div>;
}
