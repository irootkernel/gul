import { createHash } from "node:crypto";
import { readFile, readdir, stat } from "node:fs/promises";
import { join, relative, resolve } from "node:path";
import { fromJson } from "@bufbuild/protobuf";
import {
  GetOrchestratedSessionResponseSchema,
  ListOrchestratedSessionResultsResponseSchema,
} from "../generated/ts/dolgorae/public/v1/dolgorae_pb";
import { validateGoManifest } from "./validate-go-manifest.mjs";

const contractRoot = resolve(import.meta.dir, "..");
const generatedRoot = join(contractRoot, "generated");

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function equal(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

async function collectFiles(root) {
  const found = [];
  async function walk(directory) {
    for (const name of (await readdir(directory)).sort()) {
      const entryPath = join(directory, name);
      const info = await stat(entryPath);
      if (info.isDirectory()) await walk(entryPath);
      else if (name !== "generated-lock.json") found.push(entryPath);
    }
  }
  await walk(root);
  return found;
}

const supportedSchemaKeywords = new Set([
  "$comment", "$defs", "$id", "$ref", "$schema", "additionalProperties", "allOf", "anyOf",
  "const", "contentEncoding", "description", "else", "enum", "format", "if",
  "items", "maxItems", "maximum", "maxLength", "minItems", "minimum", "minLength",
  "minProperties", "not", "oneOf", "pattern", "properties", "required", "then",
  "title", "type", "uniqueItems", "x-maxUtf8Bytes",
]);

function assertSupportedSchema(schema, location = "$") {
  if (typeof schema === "boolean") return;
  for (const keyword of Object.keys(schema)) {
    if (!supportedSchemaKeywords.has(keyword)) throw new Error(`unsupported Machine schema keyword ${keyword} at ${location}`);
  }
  for (const [name, child] of Object.entries(schema.$defs ?? {})) assertSupportedSchema(child, `${location}.$defs.${name}`);
  for (const [name, child] of Object.entries(schema.properties ?? {})) assertSupportedSchema(child, `${location}.properties.${name}`);
  if (schema.items && typeof schema.items === "object") assertSupportedSchema(schema.items, `${location}.items`);
  if (schema.additionalProperties && typeof schema.additionalProperties === "object") assertSupportedSchema(schema.additionalProperties, `${location}.additionalProperties`);
  for (const keyword of ["allOf", "anyOf", "oneOf"]) {
    for (const [index, child] of (schema[keyword] ?? []).entries()) assertSupportedSchema(child, `${location}.${keyword}[${index}]`);
  }
  for (const keyword of ["not", "if", "then", "else"]) {
    if (schema[keyword]) assertSupportedSchema(schema[keyword], `${location}.${keyword}`);
  }
}

function localRef(root, reference) {
  if (!reference.startsWith("#/")) return null;
  return reference.slice(2).split("/").reduce((value, part) => value[part.replaceAll("~1", "/").replaceAll("~0", "~")], root);
}

function schemaValid(schema, value, root) {
  if (typeof schema === "boolean") return schema;
  if (schema.$ref) {
    const target = localRef(root, schema.$ref);
    return target ? schemaValid(target, value, root) : false;
  }
  if (schema.allOf && !schema.allOf.every((part) => schemaValid(part, value, root))) return false;
  if (schema.anyOf && !schema.anyOf.some((part) => schemaValid(part, value, root))) return false;
  if (schema.oneOf && schema.oneOf.filter((part) => schemaValid(part, value, root)).length !== 1) return false;
  if (schema.not && schemaValid(schema.not, value, root)) return false;
  if (schema.if) {
    const branch = schemaValid(schema.if, value, root) ? schema.then : schema.else;
    if (branch && !schemaValid(branch, value, root)) return false;
  }
  if (schema.const !== undefined && !equal(schema.const, value)) return false;
  if (schema.enum && !schema.enum.some((item) => equal(item, value))) return false;

  if (schema.type) {
    const types = Array.isArray(schema.type) ? schema.type : [schema.type];
    const matches = types.some((type) => {
      if (type === "null") return value === null;
      if (type === "array") return Array.isArray(value);
      if (type === "object") return value !== null && typeof value === "object" && !Array.isArray(value);
      if (type === "integer") return Number.isInteger(value);
      if (type === "number") return typeof value === "number" && Number.isFinite(value);
      return typeof value === type;
    });
    if (!matches) return false;
  }

  if (typeof value === "string") {
    if (schema.minLength !== undefined && value.length < schema.minLength) return false;
    if (schema.maxLength !== undefined && value.length > schema.maxLength) return false;
    if (schema.pattern && !new RegExp(schema.pattern, "u").test(value)) return false;
    if (schema.format === "uuid" && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)) return false;
    if (schema.format === "date-time" && (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) || Number.isNaN(Date.parse(value)))) return false;
    if (schema.contentEncoding === "base64" && !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) return false;
    if (schema["x-maxUtf8Bytes"] !== undefined && new TextEncoder().encode(value).length > schema["x-maxUtf8Bytes"]) return false;
  }
  if (typeof value === "number") {
    if (schema.minimum !== undefined && value < schema.minimum) return false;
    if (schema.maximum !== undefined && value > schema.maximum) return false;
  }
  if (Array.isArray(value)) {
    if (schema.minItems !== undefined && value.length < schema.minItems) return false;
    if (schema.maxItems !== undefined && value.length > schema.maxItems) return false;
    if (schema.uniqueItems && new Set(value.map((item) => JSON.stringify(item))).size !== value.length) return false;
    if (schema.items && !value.every((item) => schemaValid(schema.items, item, root))) return false;
  }
  if (value !== null && typeof value === "object" && !Array.isArray(value)) {
    if (schema.minProperties !== undefined && Object.keys(value).length < schema.minProperties) return false;
    if (schema.required && !schema.required.every((key) => Object.hasOwn(value, key))) return false;
    for (const [key, child] of Object.entries(schema.properties ?? {})) {
      if (Object.hasOwn(value, key) && !schemaValid(child, value[key], root)) return false;
    }
    if (schema.additionalProperties === false) {
      const allowed = new Set(Object.keys(schema.properties ?? {}));
      if (Object.keys(value).some((key) => !allowed.has(key))) return false;
    } else if (schema.additionalProperties && typeof schema.additionalProperties === "object") {
      const known = new Set(Object.keys(schema.properties ?? {}));
      for (const [key, child] of Object.entries(value)) {
        if (!known.has(key) && !schemaValid(schema.additionalProperties, child, root)) return false;
      }
    }
  }
  return true;
}

const inventory = JSON.parse(await readFile(join(generatedRoot, "contract-inventory.v1.json"), "utf8"));
const operations = JSON.parse(await readFile(join(generatedRoot, "gul-operation-map.v1.json"), "utf8"));
const events = JSON.parse(await readFile(join(generatedRoot, "event-invalidation-map.v1.json"), "utf8"));
const fixtures = JSON.parse(await readFile(join(generatedRoot, "fixtures/consumer-policy.v1.json"), "utf8"));
const machineFixture = JSON.parse(await readFile(join(generatedRoot, "fixtures/machine-cli-comparison.v1.json"), "utf8"));
const capabilityMap = JSON.parse(await readFile(join(generatedRoot, "policy/capability-map.v1.json"), "utf8"));
const errorActionMap = JSON.parse(await readFile(join(generatedRoot, "policy/error-action-map.v1.json"), "utf8"));
const mutationMap = JSON.parse(await readFile(join(generatedRoot, "policy/mutation-map.v1.json"), "utf8"));
const identifierEnumMap = JSON.parse(await readFile(join(generatedRoot, "policy/identifier-enum-map.v1.json"), "utf8"));
const projectionReplayMap = JSON.parse(await readFile(join(generatedRoot, "policy/projection-replay-map.v1.json"), "utf8"));
const credentialBoundary = JSON.parse(await readFile(join(generatedRoot, "policy/credential-boundary.v1.json"), "utf8"));
const machineSchemaBytes = await readFile(join(contractRoot, "upstream/dolgorae-machine-v1.schema.json"));
const machineSchema = JSON.parse(machineSchemaBytes);
assertSupportedSchema(machineSchema);
const capabilities = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-capabilities-v1.schema.json"), "utf8"));
const clientPolicy = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-grpc-client-policy-v1.json"), "utf8"));
const mutations = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-rpc-mutation-policy-v1.json"), "utf8"));
const errorActions = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-grpc-error-mapping-v1.json"), "utf8"));
const descriptorMetadata = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-public-v1.descriptor.json"), "utf8"));
const conformance = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-grpc-conformance-v1.json"), "utf8"));
const verificationIndex = JSON.parse(await readFile(join(contractRoot, "upstream/verification-index-v1.json"), "utf8"));
const credentialSchemaBytes = await readFile(join(contractRoot, "upstream/dolgorae-controller-credential-v1.schema.json"));
const credentialSchema = JSON.parse(credentialSchemaBytes);
const consumerProfile = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-gul-consumer-v1.json"), "utf8"));
const providerFixtures = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-gul-consumer-v1.fixtures.json"), "utf8"));
const producerLock = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-gul-consumer-v1.lock.json"), "utf8"));
const dependencyLockBytes = await readFile(join(contractRoot, "dependency-lock.json"));
const dependencyLock = JSON.parse(dependencyLockBytes);
const generatedLockBytes = await readFile(join(generatedRoot, "generated-lock.json"));
const generatedLock = JSON.parse(generatedLockBytes);
const packageManifest = JSON.parse(await readFile(join(contractRoot, "package.json"), "utf8"));
const goManifest = await readFile(join(contractRoot, "go.mod"), "utf8");
const versionManifest = Object.fromEntries((await readFile(join(contractRoot, "../toolchain/versions.env"), "utf8"))
  .split("\n")
  .filter((line) => /^GUL_[A-Z0-9_]+=/.test(line))
  .map((line) => line.split("=", 2)));

if (inventory.service_count !== 8 || inventory.method_count !== 36) throw new Error("public inventory must contain 8 services and 36 methods");
const expectedPackages = {
  "@bufbuild/protobuf": versionManifest.GUL_PROTOBUF_ES_VERSION,
  "@connectrpc/connect": versionManifest.GUL_CONNECT_ES_VERSION,
  "@connectrpc/connect-web": versionManifest.GUL_CONNECT_WEB_VERSION,
  "@bufbuild/protoc-gen-es": versionManifest.GUL_PROTOC_GEN_ES_VERSION,
  typescript: versionManifest.GUL_TYPESCRIPT_VERSION,
};
for (const [name, expected] of Object.entries(expectedPackages)) {
  const actual = packageManifest.dependencies?.[name] ?? packageManifest.devDependencies?.[name];
  if (actual !== expected) throw new Error(`${name} must match toolchain authority ${expected}, found ${actual}`);
}
if (packageManifest.dependencies?.["@connectrpc/protoc-gen-connect-es"] || packageManifest.devDependencies?.["@connectrpc/protoc-gen-connect-es"] || generatedLock.files.some((entry) => entry.path.endsWith("_connect.ts"))) throw new Error("Connect-ES v1 generator artifacts must not coexist with Protobuf-ES v2 service descriptors");
validateGoManifest(goManifest, versionManifest);
const descriptorMethods = inventory.services.flatMap((service) => service.methods.map((method) => `${service.name}.${method.name}`)).sort();
const capabilityMethods = [...capabilities.properties.grpc_methods.items.enum].sort();
if (!equal(descriptorMethods, capabilityMethods)) throw new Error("descriptor and capability method inventories differ");
const requiredMethods = [...consumerProfile.required_methods].sort();
const unavailableMethods = [...consumerProfile.unavailable_until_later_tasks].sort();
if (consumerProfile.descriptor_method_count !== 36 || consumerProfile.required_method_count !== 27 || requiredMethods.length !== 27 || unavailableMethods.length !== 9) throw new Error("consumer profile method counts drifted");
if (requiredMethods.some((method) => unavailableMethods.includes(method)) || !equal([...requiredMethods, ...unavailableMethods].sort(), descriptorMethods)) throw new Error("consumer required and unavailable methods must partition the descriptor inventory");
if (!equal([...capabilityMap.known_methods].sort(), descriptorMethods)) throw new Error("generated capability known-method inventory differs from the descriptor");
if (!equal([...capabilityMap.required_methods].sort(), requiredMethods) || !equal([...capabilityMap.unavailable_methods].sort(), unavailableMethods)) throw new Error("generated capability consumer profile drifted");
if (!equal(capabilityMap.stage_requirements, capabilities.properties.grpc_methods["x-stageRequirements"])) throw new Error("generated capability stage requirements drifted");

if (operations.operations.length !== 31) throw new Error(`expected 31 Gul semantic operations, found ${operations.operations.length}`);
if (new Set(operations.operations.map((entry) => entry.operation)).size !== operations.operations.length) throw new Error("duplicate Gul semantic operation");
const operationMethods = [...new Set(operations.operations.filter((entry) => entry.owner.includes("Service.")).map((entry) => entry.owner))].sort();
if (!equal(operationMethods, requiredMethods)) throw new Error("operation-map RPC owners must exactly match the required consumer methods");
if (events.events.length !== 20) throw new Error(`expected 20 event invalidation rows, found ${events.events.length}`);
if (new Set(events.events.map((entry) => entry.variant)).size !== events.events.length) throw new Error("duplicate event invalidation variant");
const durableEvent = inventory.messages.find((message) => message.name.endsWith(".DurableRunEvent"));
const durableOneof = durableEvent.oneofs.indexOf("event");
const durableVariants = durableEvent.fields
  .filter((field) => field.oneof_index === durableOneof)
  .map((field) => field.name.split("_").map((part) => `${part[0].toUpperCase()}${part.slice(1)}`).join(""))
  .sort();
const mappedDurableVariants = events.events.slice(0, durableVariants.length).map((entry) => entry.variant).sort();
if (!equal(durableVariants, mappedDurableVariants)) throw new Error("typed durable event oneof and invalidation map differ");
const expectedPolicyEvents = [
  "RunEventHeartbeat",
  "RunEventStreamEnd(RUN_TERMINAL)",
  "RunEventStreamEnd(SERVER_SHUTDOWN)",
  "typed gRPC SLOW_CONSUMER",
  "other transport failure",
];
if (!equal(events.events.slice(durableVariants.length).map((entry) => entry.variant), expectedPolicyEvents)) throw new Error("non-durable event policy rows drifted");
if (!equal(Object.keys(clientPolicy.stream_end_policy), ["RUN_TERMINAL", "SERVER_SHUTDOWN", "SLOW_CONSUMER", "TRANSPORT_FAILURE"])) throw new Error("stream-end policy inventory drifted");

const serviceConfigMutations = clientPolicy.grpc_service_config.methodConfig[0].name.map((entry) => `${entry.service.split(".").at(-1)}.${entry.method}`).sort();
const mutationMethods = mutations.mutations.map((entry) => entry.rpc).sort();
if (!equal(serviceConfigMutations, mutationMethods)) throw new Error("client no-retry list and mutation registry differ");
if (clientPolicy.mutation_retry_policy.transparent_retries !== "disabled" || mutations.transparent_grpc_retries !== false) throw new Error("transparent retries are not disabled");
if (mutationMap.transparent_grpc_retries !== false || !equal(mutationMap.upstream, mutations.mutations)) throw new Error("generated mutation policy drifted");
const expectedOperationIdempotency = operations.operations.map((entry) => ({
  operation: entry.operation,
  owner: entry.owner,
  idempotency: entry.idempotency,
  retry: entry.retry,
  reconciliation: entry.reconciliation,
}));
if (!equal(mutationMap.gul_operation_idempotency, expectedOperationIdempotency)) throw new Error("generated operation idempotency policy drifted");
if (errorActionMap.fail_closed_on_unknown_detail !== true || !equal(errorActionMap.mapping, errorActions)) throw new Error("generated error action policy drifted");
if (fixtures.production_cli_fallback !== false || machineFixture.production_fallback !== false) throw new Error("production CLI fallback must remain false");
if (!equal([...fixtures.required_rpcs].sort(), requiredMethods) || !equal([...fixtures.unavailable_rpcs].sort(), unavailableMethods) || !equal(fixtures.producer_case_catalog, providerFixtures)) throw new Error("generated consumer-profile catalog drifted");
if (fixtures.protocol_handshake.minimum !== descriptorMetadata.minimum_client_protocol_version || fixtures.protocol_handshake.maximum !== descriptorMetadata.maximum_client_protocol_version) throw new Error("protocol handshake bounds drifted from descriptor metadata");
if (fixtures.protocol_handshake.cases.find((entry) => entry.outcome === "compatible")?.server !== descriptorMetadata.protocol_version) throw new Error("protocol handshake accepted version drifted");
if (fixtures.interaction_limits.effective_response_bytes !== Math.min(fixtures.interaction_limits.provider_response_bytes, fixtures.interaction_limits.gul_response_bytes)) throw new Error("effective Interaction response limit drifted");
if (fixtures.interaction_limits.effective_safe_payload_bytes !== Math.min(fixtures.interaction_limits.provider_safe_payload_bytes, fixtures.interaction_limits.gul_safe_payload_bytes)) throw new Error("effective Interaction payload limit drifted");

if (conformance.schema_version !== 1 || conformance.contract_cases.length !== 25 || conformance.contract_negative_fixtures.length !== 12 || conformance.negative_fixtures.length !== 6) throw new Error("gRPC conformance inventory drifted");
const verificationSections = Object.values(verificationIndex.sections);
if (verificationIndex.schema_version !== 1 || verificationSections.length !== 15 || verificationSections.some((section) => !Array.isArray(section.tasks) || !Array.isArray(section.catches))) throw new Error("verification index structure drifted");
const credentialCapability = credentialSchema.properties.capability;
const credentialKinds = credentialSchema.properties.kind.enum;
if (credentialSchema.properties.schema_version.const !== 1 || credentialSchema.additionalProperties !== false || !credentialSchema.required.includes("capability") || credentialCapability.contentEncoding !== "base64url" || credentialCapability.pattern !== "^[A-Za-z0-9_-]{43}$" || credentialCapability["x-canonicalDecodedBytes"] !== 32 || credentialKinds.some((kind) => /operator|admin/i.test(kind))) throw new Error("controller credential schema structure drifted");

const profileRequestNames = ["ListProfilesRequest", "GetProfileRequest"];
for (const name of profileRequestNames) {
  const message = inventory.messages.find((entry) => entry.name.endsWith(`.${name}`));
  if (!message || message.fields.some((field) => field.name === "workspace" || field.type_name.endsWith(".WorkspaceRef"))) throw new Error(`${name} must not carry WorkspaceRef`);
}
if (consumerProfile.close_run.transparent_retry !== "forbidden" || consumerProfile.close_run.transport_loss_follow_up !== "OrchestrationService.GetOrchestratedSession") throw new Error("CloseRun retry and reconciliation policy drifted");
const closeFixtures = new Map(providerFixtures.close_outcomes.map((entry) => [entry.id, entry]));
if (closeFixtures.get("intent_before_response_loss")?.client_action !== "observe_without_resubmit" || closeFixtures.get("concurrent_compatible_close")?.outcome !== "same_operation" || closeFixtures.get("concurrent_mismatched_interrupt")?.outcome !== "typed_conflict_no_new_operation" || closeFixtures.get("unknown_child_effect")?.retry !== "RETRY_CLASSIFICATION_FORBIDDEN") throw new Error("CloseRun operation-correlation fixtures drifted");
if (providerFixtures.method_coverage.length !== 27 || !equal(providerFixtures.method_coverage.map((entry) => entry.method).sort(), requiredMethods) || providerFixtures.method_coverage.some((entry) => !entry.positive || !entry.negative || entry.positive === entry.negative)) throw new Error("producer case labels do not match the required consumer profile");

const fixtureIds = new Set([
  ...providerFixtures.positive.map((entry) => entry.id),
  ...providerFixtures.negative.map((entry) => entry.id),
  ...providerFixtures.close_outcomes.map((entry) => entry.id),
  ...Object.keys(providerFixtures.fixture_groups),
]);
const sessionResponse = inventory.messages.find((entry) => entry.name === "dolgorae.public.v1.GetOrchestratedSessionResponse");
const sessionProjection = inventory.messages.find((entry) => entry.name === "dolgorae.public.v1.OrchestratedSessionProjection");
const resultsResponse = inventory.messages.find((entry) => entry.name === "dolgorae.public.v1.ListOrchestratedSessionResultsResponse");
const resultProjection = inventory.messages.find((entry) => entry.name === "dolgorae.public.v1.OrchestratedSessionResult");
if (!sessionResponse || !sessionProjection || !resultsResponse || !resultProjection || sessionResponse.fields.find((field) => field.name === "session")?.type_name !== sessionProjection.name || resultsResponse.fields.find((field) => field.name === "items")?.type_name !== resultProjection.name) throw new Error("aggregate response or projection messages are missing from the descriptor inventory");
const expectedFieldSources = [
  ...sessionResponse.fields.filter((field) => field.name !== "session").map((field) => `session.${field.name}`),
  ...sessionProjection.fields.map((field) => `session.${field.name}`),
  ...resultsResponse.fields.filter((field) => field.name !== "items").map((field) => `results.${field.name}`),
  ...resultProjection.fields.map((field) => `results.items.${field.name}`),
].sort();
const actualFieldSources = consumerProfile.field_sources.map((entry) => entry.field).sort();
if (!equal(actualFieldSources, expectedFieldSources) || new Set(actualFieldSources).size !== actualFieldSources.length || consumerProfile.field_sources.some((entry) => !entry.source || !fixtureIds.has(entry.fixture) || !entry.owner)) throw new Error("consumer field-sourceability inventory is incomplete or does not resolve to pinned fixtures");

const sessionFixture = providerFixtures.positive.find((entry) => entry.id === "session_active");
const resultsFixture = providerFixtures.positive.find((entry) => entry.id === "results_page_one");
if (sessionFixture?.grpc_message !== "dolgorae.public.v1.GetOrchestratedSessionResponse" || resultsFixture?.grpc_message !== "dolgorae.public.v1.ListOrchestratedSessionResultsResponse") throw new Error("aggregate fixture message identities drifted");
fromJson(GetOrchestratedSessionResponseSchema, sessionFixture.grpc_json, { ignoreUnknownFields: false });
fromJson(ListOrchestratedSessionResultsResponseSchema, resultsFixture.grpc_json, { ignoreUnknownFields: false });
if (conformance.freeze_policy?.status !== "blocked_runtime_evidence" || !Array.isArray(conformance.freeze_policy.runtime_evidence) || conformance.freeze_policy.runtime_evidence.length !== 0) throw new Error("producer runtime-evidence freeze boundary drifted");
if (mutations.mutations.length !== 18) throw new Error(`expected 18 mutation policies, found ${mutations.mutations.length}`);

if (dependencyLock.source.revision !== "21aefe5b2a8dc6fb18a58338090348b23d2f0a4a" || inventory.source_revision !== dependencyLock.source.revision || generatedLock.source_revision !== dependencyLock.source.revision) throw new Error("TASK-053 completion revision is not pinned consistently");
const expectedProducerTaskInputRevision = "a963f3601b1e7036432501b6d8946c8c62c1466d";
if (dependencyLock.source.producer_task_input_revision !== expectedProducerTaskInputRevision || producerLock.source_revision.task_input_commit !== expectedProducerTaskInputRevision || consumerProfile.baseline_commit !== expectedProducerTaskInputRevision || expectedProducerTaskInputRevision === dependencyLock.source.revision) throw new Error("producer task-input identity is not pinned distinctly from the completion revision");
const dependencyByPath = new Map(dependencyLock.files.map((entry) => [entry.path, entry.sha256]));
for (const artifact of producerLock.artifacts.filter((entry) => !entry.path.startsWith("generated/"))) {
  const relativePath = artifact.path.startsWith("baselines/") ? `upstream/${artifact.path}` : `upstream/${artifact.path}`;
  const bytes = await readFile(join(contractRoot, relativePath));
  if (sha256(bytes) !== artifact.sha256 || dependencyByPath.get(relativePath) !== artifact.sha256) throw new Error(`producer lock correlation failed for ${artifact.path}`);
}
if (descriptorMetadata.buf_breaking?.result !== "compatible_additive" || descriptorMetadata.buf_breaking?.baseline_sha256 !== dependencyByPath.get("upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb")) throw new Error("additive descriptor baseline evidence drifted");
const credentialArtifact = producerLock.artifacts.find((entry) => entry.path === "dolgorae-controller-credential-v1.schema.json");
const credentialProperties = capabilities.properties.controller_credential.properties;
if (!credentialArtifact || credentialArtifact.sha256 !== sha256(credentialSchemaBytes) || credentialProperties.schema_sha256.const !== credentialArtifact.sha256) throw new Error("credential schema digest drifted from producer lock or capabilities");
const credentialProjection = conformance.adapter_parity_fixtures.find((entry) => entry.id === "credential_carrier_projection")?.grpc_json?.controllerCarrier;
if (!credentialProjection || credentialProjection.capabilityByteLength !== credentialProperties.capability_byte_length.const || credentialProjection.parentDirectoryMode !== Number.parseInt(credentialProperties.parent_directory_mode.const, 8) || credentialProjection.credentialFileMode !== Number.parseInt(credentialProperties.file_mode.const, 8) || credentialProjection.symlinksForbidden !== true || credentialProjection.createExclusiveRequired !== true) throw new Error("credential carrier conformance projection drifted");
const architectureText = await readFile(join(contractRoot, "../docs/architecture.md"), "utf8");
if (!architectureText.includes("~/.dolgorae/controller-carriers/gul/<installation-id>") || !architectureText.includes("no production fake/CLI fallback") || !architectureText.includes("never stores or uses an Operator capability")) throw new Error("carrier-root or production authority boundary is missing from architecture");
if (operations.operations.some((entry) => entry.verification !== "E12 pinned contract; not runtime/live evidence")) throw new Error("every E12 operation must retain the contract-only verification boundary");
const requiredSpecsText = await readFile(join(contractRoot, "../docs/required-specs.md"), "utf8");
if (!requiredSpecsText.includes("E12-T1 has accepted the immutable consumer contract and generated tooling") || !requiredSpecsText.includes("It does not\npromote E14-owned REQ-HOST-001/002 or implement a listener, Wails host, browser\nAPI, persistence repository, provider adapter, product route, or production\nauthentication.") || !requiredSpecsText.includes("| REQ-CONSUMER-001 | Immutable TASK-053 consumer source")) throw new Error("REQ-CONSUMER-001 contract-only Current State drifted");

if (machineFixture.schema_sha256 !== sha256(machineSchemaBytes)) throw new Error("Machine CLI fixture schema digest drifted");
if (!schemaValid(machineSchema, machineFixture.valid, machineSchema)) throw new Error("valid Machine CLI comparison fixture failed exact schema validation");
if (schemaValid(machineSchema, machineFixture.invalid, machineSchema)) throw new Error("invalid Machine CLI comparison fixture unexpectedly passed schema validation");
const schemaNegativeCases = [
  [{ type: "string", pattern: "^ok$" }, "bad"],
  [{ type: "string", format: "uuid" }, "not-a-uuid"],
  [{ type: "string", format: "date-time" }, "2026-99-99"],
  [{ type: "string", contentEncoding: "base64" }, "%%%"],
  [{ type: "string", "x-maxUtf8Bytes": 2 }, "굴"],
  [{ type: "string", enum: ["allowed"] }, "denied"],
  [{ type: "array", uniqueItems: true }, ["duplicate", "duplicate"]],
];
for (const [schema, value] of schemaNegativeCases) {
  if (schemaValid(schema, value, schema)) throw new Error(`Machine schema negative self-test unexpectedly passed: ${JSON.stringify(schema)}`);
}
let unsupportedKeywordRejected = false;
try {
  assertSupportedSchema({ type: "string", unsupportedKeyword: true });
} catch {
  unsupportedKeywordRejected = true;
}
if (!unsupportedKeywordRejected) throw new Error("Machine schema keyword audit did not fail closed");

const requiredIdentifiers = ["RequestContext", "ResponseContext", "WorkspaceRef", "RunRef", "ControllerCarrierRef", "ProjectionStamp"];
for (const identifier of requiredIdentifiers) {
  if (!identifierEnumMap.identifiers.some((entry) => entry.name.endsWith(`.${identifier}`))) throw new Error(`generated identifier map omits ${identifier}`);
}
if (!equal(identifierEnumMap.enums, inventory.enums)) throw new Error("generated enum map and descriptor inventory differ");
if (!equal(projectionReplayMap.event_invalidation, events.events)) throw new Error("generated projection invalidation policy drifted");
if (!equal(projectionReplayMap.convergence, fixtures.projection_convergence)) throw new Error("generated projection convergence policy drifted");
if (!equal(projectionReplayMap.replay_material, fixtures.replay_material) || !equal(projectionReplayMap.stream_end, fixtures.stream_end)) throw new Error("generated replay policy drifted");
const replayOperations = Object.keys(projectionReplayMap.replay_material);
if (!equal([...replayOperations].sort(), ["ResolveInteraction", "StartRun", "SubmitTurn"]) || replayOperations.some((operation) => unavailableMethods.some((rpc) => rpc.endsWith(`.${operation}`))) || replayOperations.some((operation) => !operations.operations.some((entry) => entry.operation === operation))) throw new Error("Gul replay policy contains an unavailable or unknown operation");
const credentialValue = (name) => credentialProperties[name].const;
const carrierRoot = capabilities.properties.controller_carrier_root.const.replace(/^home\//, "~/");
const clientDescendant = credentialValue("client_descendant_pattern").replace("<client>", "gul").replace(/\/$/, "");
const expectedCredentialBoundary = {
  schema_version: 1,
  schema_id: credentialValue("schema_id"),
  credential_schema_version: credentialValue("schema_version"),
  credential_schema_sha256: credentialValue("schema_sha256"),
  accepted_kinds: credentialValue("accepted_kinds"),
  capability_byte_length: credentialValue("capability_byte_length"),
  capability_encoding: credentialValue("capability_encoding"),
  carrier_root: `${carrierRoot}/${clientDescendant}`,
  parent_directory_mode: credentialValue("parent_directory_mode"),
  file_mode: credentialValue("file_mode"),
  same_uid: credentialValue("same_uid"),
  regular_file: credentialValue("regular_file"),
  symlinks: credentialValue("symlinks"),
  create_exclusive: credentialValue("create_exclusive"),
  maximum_file_bytes: credentialValue("maximum_file_bytes"),
  client_descendant_pattern: clientDescendant,
  normalized_principal: credentialValue("normalized_principal"),
  initial_generation: credentialValue("initial_generation"),
  operator_capability: "forbidden",
  overwrite: false,
};
if (!equal(credentialBoundary, expectedCredentialBoundary)) throw new Error("generated credential boundary drifted");

for (const entry of generatedLock.files) {
  const bytes = await readFile(join(generatedRoot, entry.path));
  if (sha256(bytes) !== entry.sha256) throw new Error(`generated digest mismatch for ${entry.path}`);
}
if (generatedLock.files.length !== new Set(generatedLock.files.map((entry) => entry.path)).size) throw new Error("duplicate generated lock entry");
const generatedPaths = (await collectFiles(generatedRoot)).map((entryPath) => relative(generatedRoot, entryPath)).sort();
if (!equal(generatedLock.files.map((entry) => entry.path).sort(), generatedPaths)) throw new Error("generated lock does not exhaustively cover generated outputs");
const upstreamPaths = (await collectFiles(join(contractRoot, "upstream"))).map((entryPath) => relative(contractRoot, entryPath)).sort();
if (!equal(dependencyLock.files.map((entry) => entry.path).sort(), upstreamPaths)) throw new Error("dependency lock does not exhaustively cover upstream inputs");

const dependencyLockDigest = sha256(dependencyLockBytes);
const generatedLockDigest = sha256(generatedLockBytes);
// required-specs.md names promoted requirements but deliberately carries no
// lock digest; these four documents are the E12 lifecycle and design records.
for (const document of ["architecture-decision-records.md", "architecture.md", "implementation-memo.md", "roadmap.md"]) {
  const text = await readFile(join(contractRoot, "../docs", document), "utf8");
  if (!text.includes(dependencyLockDigest) || !text.includes(generatedLockDigest)) throw new Error(`${document} does not carry the current E12 lock digests`);
}
const implementationMemoText = await readFile(join(contractRoot, "../docs/implementation-memo.md"), "utf8");
const testingText = await readFile(join(contractRoot, "../TESTING.md"), "utf8");
const adrText = await readFile(join(contractRoot, "../docs/architecture-decision-records.md"), "utf8");
const markdownCode = (value) => `\`${value}\``;
const hostMinimums = [
  ["Wails", versionManifest.GUL_WAILS_MIN_VERSION],
  ["Node", versionManifest.GUL_NODE_MIN_VERSION],
  ["Bun", versionManifest.GUL_BUN_MIN_VERSION],
  ["Buf", versionManifest.GUL_BUF_MIN_VERSION],
  ["protoc", versionManifest.GUL_PROTOC_MIN_VERSION],
  ["Git", versionManifest.GUL_GIT_MIN_VERSION],
  ["macOS", versionManifest.GUL_MACOS_MIN_VERSION],
];
for (const [name, minimum] of hostMinimums) {
  const minimumCode = markdownCode(`>=${minimum}`);
  if (!architectureText.includes(minimumCode) || !testingText.includes(minimumCode) || !implementationMemoText.includes(minimumCode)) {
    throw new Error(`${name} minimum ${minimum} is not synchronized across the toolchain documentation`);
  }
}
const goExactCode = markdownCode(versionManifest.GUL_GO_VERSION);
if (!architectureText.includes(`Go exactly ${goExactCode}`) || !testingText.includes(`Go is fixed at exactly ${goExactCode}`) || !implementationMemoText.includes(`Host Go exactly ${goExactCode}`)) throw new Error(`Go exact pin ${versionManifest.GUL_GO_VERSION} is not synchronized across the toolchain documentation`);
if (!architectureText.includes("Non-Go host checks do not impose upper bounds") || !testingText.includes("Newer non-Go host tools are accepted") || !testingText.includes("PATH copies of those generators are not part of the host check") || !implementationMemoText.includes("exact generator and library")) throw new Error("host-version and project-pin policy drifted across documentation");
if (!requiredSpecsText.includes("enforces the exact Go version and minimum versions for other host executables") || !requiredSpecsText.includes("Exact Go toolchain with minimum-compatible non-Go host tools")) throw new Error("REQ-HOST-005 drifted from the host-version and project-pin policy");
const adr0053Start = adrText.indexOf("\n### ADR-0053:");
const adr0054Start = adrText.indexOf("\n### ADR-0054:", adr0053Start);
const proposedStart = adrText.indexOf("## 6. Proposed decisions", adr0054Start);
if (adr0053Start < 0 || adr0054Start < 0 || proposedStart < 0) throw new Error("toolchain ADR boundaries are missing");
const adr0053Text = adrText.slice(adr0053Start, adr0054Start);
const adr0054Text = adrText.slice(adr0054Start, proposedStart);
const adr0053Compact = adr0053Text.replace(/\s+/g, " ");
if (!adr0053Compact.includes("**Status:** Accepted, modified") || adr0053Compact.includes(">=1.66.1,<2.0.0") || !adr0053Compact.includes("former `<2.0.0` ceiling") || !adr0053Compact.includes("protoc `35.1` as a host minimum")) throw new Error("ADR-0053 does not carry its ADR-0054 amendment");
for (const [, minimum] of hostMinimums) {
  if (!adr0054Text.includes(markdownCode(minimum))) throw new Error(`ADR-0054 is missing host minimum ${minimum}`);
}
if (!adr0054Text.includes(`Go remains exactly pinned at ${goExactCode}`) || !adr0054Text.includes("minimum versions with no upper bound") || !adr0054Text.includes("Project Go and Bun manifests retain exact dependency and code-generator pins")) throw new Error("ADR-0054 policy text drifted");
if (implementationMemoText.includes("E12-T1 must still reproduce") || implementationMemoText.includes("Existing checked/generated files are intentionally unchanged")) throw new Error("implementation memo still describes TASK-053 adoption as outstanding");

console.log(`contract validation passed: ${inventory.method_count} RPCs, ${operations.operations.length} operations, ${events.events.length} event variants, ${generatedLock.files.length} generated files`);
