// Release admission is derived from the pinned published binary, never from a
// broad feature flag or a successful read. Run/Profile state still gates actions.
export function releaseAdmission(lock, profile, schema, schemaDigest, policy) {
  const release = lock.release;
  const caps = release?.advertised_capabilities;
  const fail = (reason) => { throw new Error(`release admission: ${reason}`); };
  if (lock.schema_version !== 1) fail("lock schema version");
  if (release?.version !== "0.1.3" || release.tag !== "v0.1.3" ||
      release.source_revision !== lock.source.revision ||
      !/^[a-f0-9]{40}$/.test(release.source_revision) ||
      !/^[a-f0-9]{64}$/.test(release.archive?.sha256 ?? "") ||
      !/^[a-f0-9]{64}$/.test(release.executable?.sha256 ?? "") ||
      release.executable.version !== release.version ||
      caps?.dolgorae_version?.replace(/^v/, "") !== release.version) fail("identity mismatch");
  const credential = caps.controller_credential;
  if (credential?.schema_id !== schema.$id || credential.schema_version !== 1 ||
      credential.schema_sha256 !== schemaDigest) fail("credential schema mismatch");
  const descriptor = lock.files.find(file => file.path === "upstream/dolgorae-public-v1.descriptor.pb");
  if (caps.rpc_protocol_version !== 1 || !Number.isInteger(caps.minimum_rpc_client_version) ||
      !Number.isInteger(caps.maximum_rpc_client_version) || caps.minimum_rpc_client_version > 1 ||
      caps.maximum_rpc_client_version < 1 || caps.rpc_descriptor_sha256 !== descriptor?.sha256 ||
      !caps.supported_transports?.includes("local_grpc")) fail("transport mismatch");
  const sorted = values => JSON.stringify([...values].sort());
  if (policy.schema_version !== 1 || policy.contract_id !== profile.contract_id ||
      sorted(Object.keys(policy.method_required_features)) !== sorted(profile.required_methods)) fail("method policy coverage");
  if (!Array.isArray(caps.grpc_methods)) fail("missing methods");
  const methods = new Set(caps.grpc_methods);
  if (methods.size !== caps.grpc_methods.length) fail("duplicate method");
  const featureSupported = name => {
    if (typeof caps.features?.[name] !== "boolean") fail(`missing feature ${name}`);
    return caps.features[name];
  };
  for (const name of policy.transport_required_features) {
    if (!featureSupported(name)) fail(`required transport feature ${name}`);
  }
  for (const name of policy.non_admission_features) featureSupported(name);
  const operations = Object.entries(policy.method_required_features).map(([method, features]) => {
    if (!methods.has(method) || !features.every(featureSupported)) fail(`required operation ${method}`);
    return {method, required_features: features, release_admitted: true};
  });
  const string = value => typeof value === "string" && value.length > 0;
  const boolean = value => typeof value === "boolean";
  const conditionTypes = {
    lane: string, lane_writer_support: boolean,
    required_features: value => Array.isArray(value) && value.every(string),
    requires: string, required_method: string,
    required_profile_access_policy_transition: string,
    supported_by_release: boolean, supported_by_gul: boolean,
  };
  const actions = Object.entries(policy.conditional_actions).map(([action, condition]) => {
    if (!condition || typeof condition !== "object" || Array.isArray(condition)) fail(`invalid condition ${action}`);
    for (const [key, value] of Object.entries(condition)) {
      if (!Object.hasOwn(conditionTypes, key)) fail(`unknown condition ${action}.${key}`);
      if (!conditionTypes[key](value)) fail(`invalid condition ${action}.${key}`);
    }
    const evidence = condition.required_features?.length || condition.lane_writer_support !== undefined ||
      condition.required_method || condition.required_profile_access_policy_transition;
    let admitted = Boolean(evidence) && condition.supported_by_release !== false && condition.supported_by_gul !== false;
    if (condition.required_method) admitted = methods.has(condition.required_method) && admitted;
    if (condition.required_features) admitted = condition.required_features.every(featureSupported) && admitted;
    if (condition.lane_writer_support !== undefined) {
      const lane = caps.lane_capabilities?.[condition.lane];
      if (typeof lane?.writer_support !== "boolean") fail(`missing lane ${condition.lane}`);
      admitted = lane.writer_support === condition.lane_writer_support && admitted;
    }
    if (condition.required_profile_access_policy_transition) {
      if (condition.required_profile_access_policy_transition !== "supported") fail("unknown transition policy");
      admitted = caps.access_policy_transition === condition.required_profile_access_policy_transition && admitted;
    }
    return {action, release_admitted: admitted, condition};
  });
  return {schema_version: 1, source_revision: release.source_revision,
    version: release.version, credential_schema_id: schema.$id,
    credential_schema_sha256: schemaDigest, operations, actions,
    non_admission_features: policy.non_admission_features};
}
