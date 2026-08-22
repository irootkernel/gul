import { createHash } from "node:crypto";
import { readFile, readdir, stat } from "node:fs/promises";
import { join, relative, resolve } from "node:path";

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
  "$defs", "$id", "$ref", "$schema", "additionalProperties", "allOf", "anyOf",
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
const credentialSchema = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-controller-credential-v1.schema.json"), "utf8"));
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

if (inventory.service_count !== 7 || inventory.method_count !== 34) throw new Error("public inventory must contain 7 services and 34 methods");
const expectedPackages = {
  "@bufbuild/protobuf": versionManifest.GUL_PROTOBUF_ES_VERSION,
  "@connectrpc/connect": versionManifest.GUL_CONNECT_ES_VERSION,
  "@connectrpc/connect-web": versionManifest.GUL_CONNECT_WEB_VERSION,
  "@bufbuild/protoc-gen-es": versionManifest.GUL_PROTOC_GEN_ES_VERSION,
  "@connectrpc/protoc-gen-connect-es": versionManifest.GUL_PROTOC_GEN_CONNECT_ES_VERSION,
  typescript: versionManifest.GUL_TYPESCRIPT_VERSION,
};
for (const [name, expected] of Object.entries(expectedPackages)) {
  const actual = packageManifest.dependencies?.[name] ?? packageManifest.devDependencies?.[name];
  if (actual !== expected) throw new Error(`${name} must match toolchain authority ${expected}, found ${actual}`);
}
for (const [module, expected] of [["connectrpc.com/connect", versionManifest.GUL_CONNECT_GO_VERSION], ["google.golang.org/protobuf", versionManifest.GUL_PROTOBUF_GO_VERSION]]) {
  if (!goManifest.includes(`${module} v${expected}`)) throw new Error(`${module} must match toolchain authority ${expected}`);
}
const descriptorMethods = inventory.services.flatMap((service) => service.methods.map((method) => `${service.name}.${method.name}`)).sort();
const capabilityMethods = [...capabilities.properties.grpc_methods.items.enum].sort();
if (!equal(descriptorMethods, capabilityMethods)) throw new Error("descriptor and capability method inventories differ");
if (!equal([...capabilityMap.supported_methods].sort(), descriptorMethods)) throw new Error("generated capability map and descriptor inventories differ");
if (!equal(capabilityMap.stage_requirements, capabilities.properties.grpc_methods["x-stageRequirements"])) throw new Error("generated capability stage requirements drifted");

if (operations.operations.length !== 36) throw new Error(`expected 36 Gul semantic operations, found ${operations.operations.length}`);
if (new Set(operations.operations.map((entry) => entry.operation)).size !== operations.operations.length) throw new Error("duplicate Gul semantic operation");
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
if (fixtures.protocol_handshake.minimum !== descriptorMetadata.minimum_client_protocol_version || fixtures.protocol_handshake.maximum !== descriptorMetadata.maximum_client_protocol_version) throw new Error("protocol handshake bounds drifted from descriptor metadata");
if (fixtures.protocol_handshake.cases.find((entry) => entry.outcome === "compatible")?.server !== descriptorMetadata.protocol_version) throw new Error("protocol handshake accepted version drifted");
if (fixtures.interaction_limits.effective_response_bytes !== Math.min(fixtures.interaction_limits.provider_response_bytes, fixtures.interaction_limits.gul_response_bytes)) throw new Error("effective Interaction response limit drifted");
if (fixtures.interaction_limits.effective_safe_payload_bytes !== Math.min(fixtures.interaction_limits.provider_safe_payload_bytes, fixtures.interaction_limits.gul_safe_payload_bytes)) throw new Error("effective Interaction payload limit drifted");

if (conformance.schema_version !== 1 || conformance.contract_cases.length !== 24 || conformance.contract_negative_fixtures.length !== 12 || conformance.negative_fixtures.length !== 6) throw new Error("gRPC conformance inventory drifted");
const verificationSections = Object.values(verificationIndex.sections);
if (verificationIndex.schema_version !== 1 || verificationSections.length !== 15 || verificationSections.some((section) => !Array.isArray(section.tasks) || !Array.isArray(section.catches))) throw new Error("verification index structure drifted");
if (credentialSchema.properties.schema_version.const !== 1 || credentialSchema.additionalProperties !== false || !credentialSchema.required.includes("capability")) throw new Error("controller credential schema structure drifted");

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
// lock digest; these four documents are the Gate B lifecycle and design records.
for (const document of ["architecture-decision-records.md", "architecture.md", "implementation-memo.md", "roadmap.md"]) {
  const text = await readFile(join(contractRoot, "../docs", document), "utf8");
  if (!text.includes(dependencyLockDigest) || !text.includes(generatedLockDigest)) throw new Error(`${document} does not carry the current Gate B lock digests`);
}

console.log(`contract validation passed: ${inventory.method_count} RPCs, ${operations.operations.length} operations, ${events.events.length} event variants, ${generatedLock.files.length} generated files`);
