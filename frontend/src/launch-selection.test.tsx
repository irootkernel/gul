import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {renderToStaticMarkup} from "react-dom/server";
import {LaunchAssurance, LaunchExecutionLane, RuntimeProfileChoiceSchema, RuntimeProfileCompatibility} from "../../api/generated/ts/gul/v1/gul_pb";
import {buildLaunchChoice, canCheckLaunch, LaunchSelection, ProfileCapabilities, selectLane, SharedReadOnlyNotice} from "./launch-selection";

const profile = create(RuntimeProfileChoiceSchema, {
  name: "profile", compatibility: RuntimeProfileCompatibility.COMPATIBLE, runtimeVersion: "runtime-1",
  models: [{modelId: "model", isDefault: true, supportedEfforts: ["medium"]}],
  supportedLanes: [LaunchExecutionLane.DEDICATED, LaunchExecutionLane.SHARED_READONLY],
  maximumAssurance: LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA,
});

test("launch selection remains disabled until an explicit supported choice is complete", () => {
  const html = renderToStaticMarkup(<LaunchSelection profiles={[profile]} policyNames={["named-policy"]} sharedReadOnlyWarning="Permanent shared read-only access" onCheck={() => {}} />);
  expect(html).toContain("Check configuration</button>");
  expect(html).toContain("disabled=\"\"");
  const selected = {profileName: "profile", modelId: "model", effort: "medium", lane: LaunchExecutionLane.DEDICATED,
    requiredAssurance: LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA, policyName: "named-policy", acknowledgeSharedReadonly: false};
  expect(canCheckLaunch(selected, [profile], ["named-policy"])).toBe(true);
  expect(canCheckLaunch({...selected, modelId: "other"}, [profile], ["named-policy"])).toBe(false);
  expect(canCheckLaunch({...selected, policyName: "other"}, [profile], ["named-policy"])).toBe(false);
  expect(canCheckLaunch({...selected, requiredAssurance: LaunchAssurance.STRONG_PROCESS_CONTAINMENT}, [profile], ["named-policy"])).toBe(false);
  expect(canCheckLaunch(selected, [{...profile, compatibility: RuntimeProfileCompatibility.UNVERIFIED}], ["named-policy"])).toBe(false);
});

test("shared read-only cannot be checked without explicit consent and a visible warning", () => {
  const selected = {profileName: "profile", modelId: "model", effort: "medium", lane: LaunchExecutionLane.SHARED_READONLY,
    requiredAssurance: LaunchAssurance.BEST_EFFORT_PERSONAL_ALPHA, policyName: "named-policy", acknowledgeSharedReadonly: false};
  expect(canCheckLaunch(selected, [profile], ["named-policy"])).toBe(false);
  expect(canCheckLaunch({...selected, acknowledgeSharedReadonly: true}, [profile], ["named-policy"])).toBe(true);
  expect(buildLaunchChoice(selected, [profile], ["named-policy"])).toBeUndefined();
  const acknowledged = {...selected, acknowledgeSharedReadonly: true};
  expect(buildLaunchChoice(acknowledged, [profile], ["named-policy"])?.acknowledgeSharedReadonly).toBe(true);
  expect(buildLaunchChoice(selectLane(acknowledged, LaunchExecutionLane.DEDICATED), [profile], ["named-policy"])?.acknowledgeSharedReadonly).toBe(false);
  expect(buildLaunchChoice(selectLane(acknowledged, LaunchExecutionLane.SHARED_READONLY), [profile], ["named-policy"])).toBeUndefined();
  expect(renderToStaticMarkup(<SharedReadOnlyNotice warning="Permanent shared read-only access" />)).toContain("Permanent shared read-only access");
});

test("selected Profile presents compatibility and capability summary", () => {
  const html = renderToStaticMarkup(<ProfileCapabilities profile={{...profile, featureFlags: ["safe_client_projection"],
    supportedInteractions: ["approval"], accessPolicyTransition: "unverified", backgroundExecution: "unavailable"}} />);
  expect(html).toContain("Compatibility: COMPATIBLE");
  expect(html).toContain("Runtime version: runtime-1");
  expect(html).toContain("Maximum assurance: BEST_EFFORT_PERSONAL_ALPHA");
  expect(html).toContain("Shared read-only");
  expect(html).toContain("Features: safe_client_projection");
  expect(html).toContain("Interactions: approval");
  expect(html).toContain("background execution: unavailable");
});
