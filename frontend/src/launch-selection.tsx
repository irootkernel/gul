import {create} from "@bufbuild/protobuf";
import {useState, type FormEvent} from "react";
import {
  CheckCompatibilityRequestSchema,
  LaunchAssurance,
  LaunchExecutionLane,
  RuntimeProfileCompatibility,
  type CheckCompatibilityRequest,
  type RuntimeProfileChoice,
} from "../../api/generated/ts/gul/v1/gul_pb";

export type Selection = {
  profileName: string;
  modelId: string;
  effort: string;
  lane: LaunchExecutionLane;
  requiredAssurance: LaunchAssurance;
  policyName: string;
  acknowledgeSharedReadonly: boolean;
};

export function canCheckLaunch(selection: Selection, profiles: RuntimeProfileChoice[], policyNames: string[]): boolean {
  const profile = profiles.find(item => item.name === selection.profileName);
  const model = profile?.models.find(item => item.modelId === selection.modelId);
  return profile?.compatibility === RuntimeProfileCompatibility.COMPATIBLE &&
    Boolean(profile.runtimeVersion) && Boolean(model?.supportedEfforts.includes(selection.effort)) &&
    profile.supportedLanes.includes(selection.lane) &&
    selection.requiredAssurance > LaunchAssurance.UNSPECIFIED &&
    selection.requiredAssurance <= profile.maximumAssurance &&
    policyNames.includes(selection.policyName) &&
    (selection.lane !== LaunchExecutionLane.SHARED_READONLY || selection.acknowledgeSharedReadonly);
}

export function selectLane(selection: Selection, lane: LaunchExecutionLane): Selection {
  return {...selection, lane, acknowledgeSharedReadonly: false};
}

export function buildLaunchChoice(selection: Selection, profiles: RuntimeProfileChoice[], policyNames: string[]): CheckCompatibilityRequest | undefined {
  return canCheckLaunch(selection, profiles, policyNames) ? create(CheckCompatibilityRequestSchema, selection) : undefined;
}

export function SharedReadOnlyNotice({warning}: {warning: string}) {
  return <p role="alert">{warning}</p>;
}

export function ProfileCapabilities({profile}: {profile: RuntimeProfileChoice}) {
  return <section aria-label="Profile capabilities">
    <p>Compatibility: {RuntimeProfileCompatibility[profile.compatibility]}</p>
    <p>Runtime version: {profile.runtimeVersion || "unavailable"}</p>
    <p>Maximum assurance: {LaunchAssurance[profile.maximumAssurance]}</p>
    <p>Supported lanes: {profile.supportedLanes.map(lane => lane === LaunchExecutionLane.SHARED_READONLY ? "Shared read-only" : "Dedicated").join(", ")}</p>
    <p>Features: {profile.featureFlags.join(", ") || "none advertised"}</p>
    <p>Interactions: {profile.supportedInteractions.join(", ") || "none advertised"}</p>
    <p>Access policy transition: {profile.accessPolicyTransition || "unknown"}; background execution: {profile.backgroundExecution || "unknown"}; native subagents: {profile.nativeSubagentsEnabled ? "enabled" : "disabled"}</p>
  </section>;
}

type Props = {
  profiles: RuntimeProfileChoice[];
  policyNames: string[];
  sharedReadOnlyWarning: string;
  onCheck: (choice: CheckCompatibilityRequest) => void;
  onCreate?: (choice: CheckCompatibilityRequest) => void;
  creating?: boolean;
};

// Backend compatibility and creation each revalidate the explicit selection.
export function LaunchSelection({profiles, policyNames, sharedReadOnlyWarning, onCheck, onCreate, creating}: Props) {
  const [selection, setSelection] = useState<Selection>({profileName: "", modelId: "", effort: "",
    lane: LaunchExecutionLane.UNSPECIFIED, requiredAssurance: LaunchAssurance.UNSPECIFIED,
    policyName: "", acknowledgeSharedReadonly: false});
  const profile = profiles.find(item => item.name === selection.profileName);
  const model = profile?.models.find(item => item.modelId === selection.modelId);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const choice = buildLaunchChoice(selection, profiles, policyNames);
    if (choice) onCheck(choice);
  }

  return (
    <form onSubmit={submit} aria-label="Session configuration">
      <label>Runtime Profile
        <select value={selection.profileName} onChange={event => setSelection({...selection, profileName: event.target.value, modelId: "", effort: "", lane: LaunchExecutionLane.UNSPECIFIED, requiredAssurance: LaunchAssurance.UNSPECIFIED})}>
          <option value="">Choose a Profile</option>
          {profiles.map(item => <option key={item.name} value={item.name}>{item.name} ({RuntimeProfileCompatibility[item.compatibility]}, {item.runtimeVersion || "version unavailable"})</option>)}
        </select>
      </label>
      {profile && <ProfileCapabilities profile={profile} />}
      <label>Model
        <select value={selection.modelId} onChange={event => setSelection({...selection, modelId: event.target.value, effort: ""})}>
          <option value="">Choose a model</option>
          {profile?.models.map(item => <option key={item.modelId} value={item.modelId}>{item.modelId}</option>)}
        </select>
      </label>
      <label>Effort
        <select value={selection.effort} onChange={event => setSelection({...selection, effort: event.target.value})}>
          <option value="">Choose effort</option>
          {model?.supportedEfforts.map(value => <option key={value} value={value}>{value}</option>)}
        </select>
      </label>
      <label>Execution lane
        <select value={selection.lane} onChange={event => setSelection(selectLane(selection, Number(event.target.value) as LaunchExecutionLane))}>
          <option value={LaunchExecutionLane.UNSPECIFIED}>Choose a lane</option>
          {profile?.supportedLanes.map(lane => <option key={lane} value={lane}>{lane === LaunchExecutionLane.SHARED_READONLY ? "Shared read-only" : "Dedicated"}</option>)}
        </select>
      </label>
      <label>Required assurance
        <select value={selection.requiredAssurance} onChange={event => setSelection({...selection, requiredAssurance: Number(event.target.value) as LaunchAssurance})}>
          <option value={LaunchAssurance.UNSPECIFIED}>Choose assurance</option>
          {[LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA, LaunchAssurance.VERIFIED_THREAD_SCOPED_CONTROL, LaunchAssurance.STRONG_PROCESS_CONTAINMENT]
            .filter(value => value <= (profile?.maximumAssurance ?? LaunchAssurance.UNSPECIFIED))
            .map(value => <option key={value} value={value}>{LaunchAssurance[value]}</option>)}
        </select>
      </label>
      <label>Specialist Policy
        <select value={selection.policyName} onChange={event => setSelection({...selection, policyName: event.target.value})}>
          <option value="">Choose a Policy</option>
          {policyNames.map(name => <option key={name} value={name}>{name}</option>)}
        </select>
      </label>
      {selection.lane === LaunchExecutionLane.SHARED_READONLY && <div>
        <SharedReadOnlyNotice warning={sharedReadOnlyWarning} />
        <label><input type="checkbox" checked={selection.acknowledgeSharedReadonly} onChange={event => setSelection({...selection, acknowledgeSharedReadonly: event.target.checked})} />I understand this session will remain read-only.</label>
      </div>}
      <button type="submit" disabled={!canCheckLaunch(selection, profiles, policyNames)}>Check configuration</button>
      {onCreate && <button type="button" disabled={creating || !canCheckLaunch(selection, profiles, policyNames)} onClick={() => {
        const choice = buildLaunchChoice(selection, profiles, policyNames);
        if (choice) onCreate(choice);
      }}>Create session</button>}
    </form>
  );
}
