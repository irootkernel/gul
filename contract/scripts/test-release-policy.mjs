import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {createHash} from "node:crypto";
import {releaseAdmission} from "./release-policy.mjs";

const read = name => JSON.parse(readFileSync(new URL(`../${name}`, import.meta.url)));
const lock = read("dependency-lock.json");
const profile = read("upstream/dolgorae-gul-consumer-v1.json");
const schemaBytes = readFileSync(new URL("../upstream/dolgorae-controller-credential-v1.schema.json", import.meta.url));
const schema = JSON.parse(schemaBytes);
const digest = createHash("sha256").update(schemaBytes).digest("hex");
const policy = read("admission-policy.json");
const derive = candidate => releaseAdmission(candidate, profile, schema, digest, policy);
const admitted = derive(lock);
assert.equal(admitted.operations.length, 27);
assert.equal(lock.release.advertised_capabilities.features.reader_writer_access, false);
assert.equal(lock.release.advertised_capabilities.features.brokered_independent_subagent_runs, false);
const action = name => admitted.actions.find(item => item.action === name).release_admitted;
assert.equal(action("first_dedicated_write"), true);
assert.equal(action("held_writer_submit"), true);
assert.equal(action("existing_reader_to_writer"), false);
assert.equal(action("threadless_acquire"), false);
assert.equal(action("shared_readonly_write"), false);
assert.equal(action("continuation"), false);
for (const name of ["create_direct_session", "submit_read", "release_writer", "pause_primary", "resume_primary", "interrupt_turn", "resolve_interaction", "recover_run", "reconcile_run", "close_session"]) {
  assert.equal(action(name), true);
}
for (const change of [
  value => { value.schema_version = 2; },
  value => { value.release.advertised_capabilities.controller_credential.schema_id = "dolgorae.controller-credential/v1"; },
  value => { value.release.advertised_capabilities.controller_credential.schema_sha256 = "0".repeat(64); },
  value => { value.release.advertised_capabilities.grpc_methods.pop(); },
  value => { value.release.advertised_capabilities.features.public_local_socket = false; },
  value => { value.release.advertised_capabilities.rpc_descriptor_sha256 = "0".repeat(64); },
  value => { value.release.advertised_capabilities.rpc_protocol_version = 2; },
  value => { delete value.release.advertised_capabilities.minimum_rpc_client_version; },
  value => { value.release.advertised_capabilities.minimum_rpc_client_version = 2; },
  value => { value.release.advertised_capabilities.maximum_rpc_client_version = 0; },
  value => { value.release.advertised_capabilities.supported_transports = []; },
  value => { value.release.advertised_capabilities.grpc_methods.push(value.release.advertised_capabilities.grpc_methods[0]); },
  value => { delete value.release.advertised_capabilities.lane_capabilities.dedicated; },
  value => { delete value.release.advertised_capabilities.features.brokered_independent_subagent_runs; },
  value => { value.release.executable.version = "0.1.2"; },
  value => { value.release.archive.sha256 = "invalid"; },
]) {
  const changed = structuredClone(lock);
  change(changed);
  assert.throws(() => derive(changed));
}
const noFirstWrite = structuredClone(lock);
noFirstWrite.release.advertised_capabilities.features.first_write_via_submit_turn = false;
assert.equal(derive(noFirstWrite).actions.find(item => item.action === "first_dedicated_write").release_admitted, false);
const laterMethod = structuredClone(lock);
laterMethod.release.advertised_capabilities.grpc_methods.push("RunService.DeleteRun");
assert.deepEqual(derive(laterMethod), admitted);
const noWriter = structuredClone(lock);
noWriter.release.advertised_capabilities.lane_capabilities.dedicated.writer_support = false;
for (const name of ["first_dedicated_write", "held_writer_submit", "release_writer"]) {
  assert.equal(derive(noWriter).actions.find(item => item.action === name).release_admitted, false);
}
const transition = structuredClone(lock);
transition.release.advertised_capabilities.access_policy_transition = "supported";
assert.equal(derive(transition).actions.find(item => item.action === "existing_reader_to_writer").release_admitted, true);
for (const name of policy.non_admission_features) {
  const changed = structuredClone(transition);
  changed.release.advertised_capabilities.features[name] = true;
  assert.deepEqual(derive(changed), derive(transition));
}
const emptyAction = structuredClone(policy);
emptyAction.conditional_actions.first_dedicated_write = {};
assert.equal(releaseAdmission(lock, profile, schema, digest, emptyAction).actions.find(item => item.action === "first_dedicated_write").release_admitted, false);
const changedTransitionPolicy = structuredClone(policy);
changedTransitionPolicy.conditional_actions.existing_reader_to_writer.required_profile_access_policy_transition = "unverified";
assert.throws(() => releaseAdmission(lock, profile, schema, digest, changedTransitionPolicy));
for (const change of [
  value => { value.contract_id = "wrong-contract"; },
  value => { delete value.method_required_features["WriterService.ReleaseWriter"]; },
  value => { value.method_required_features["RunService.DeleteRun"] = []; },
  value => {
    const condition = value.conditional_actions.first_dedicated_write;
    condition.lane_writer_suport = condition.lane_writer_support;
    delete condition.lane_writer_support;
  },
  value => { value.conditional_actions.first_dedicated_write.lane_writer_support = "true"; },
  value => { value.conditional_actions.first_dedicated_write.required_features = "first_write_via_submit_turn"; },
  value => { value.conditional_actions.shared_readonly_write.supported_by_release = "false"; },
]) {
  const changed = structuredClone(policy);
  change(changed);
  // An extra advertised RPC must not widen the required consumer policy.
  assert.throws(() => releaseAdmission(laterMethod, profile, schema, digest, changed));
}
assert.deepEqual(read("generated/policy/release-admission.v1.json"), admitted);
const closePending = {
  status: "FAILED_PRECONDITION",
  required_action: "REQUIRED_CLIENT_ACTION_REFRESH_SNAPSHOT",
  retry_classification: "RETRY_CLASSIFICATION_FORBIDDEN",
  recovery_classification: "RECOVERY_CLASSIFICATION_SNAPSHOT_REQUIRED",
  required_fields: ["run_id", "operation_id"],
};
assert.deepEqual(read("upstream/dolgorae-grpc-error-mapping-v1.json").method_overrides["RunService.SubmitTurn"].SESSION_CLOSE_IN_PROGRESS, closePending);
assert.deepEqual(read("generated/policy/error-action-map.v1.json").mapping.method_overrides["RunService.SubmitTurn"].SESSION_CLOSE_IN_PROGRESS, closePending);
console.log("release admission passed: required operations, first-write conditions, refusal, and drift");
