import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, stat, writeFile } from "node:fs/promises";
import { join, relative, resolve } from "node:path";
import { fromBinary } from "@bufbuild/protobuf";
import { FileDescriptorSetSchema } from "@bufbuild/protobuf/wkt";
import { releaseAdmission } from "./release-policy.mjs";

const contractRoot = resolve(import.meta.dir, "..");
const repoRoot = resolve(contractRoot, "..");
const outputRoot = resolve(process.argv[2] ?? join(contractRoot, "generated"));
const lock = JSON.parse(await readFile(join(contractRoot, "dependency-lock.json"), "utf8"));

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function stable(value) {
  return `${JSON.stringify(value, null, 2)}\n`;
}

async function writeJson(path, value) {
  await mkdir(resolve(path, ".."), { recursive: true });
  await writeFile(path, stable(value));
}

async function validateInputs() {
  const lockedPaths = lock.files.map((entry) => entry.path).sort();
  if (lockedPaths.length !== new Set(lockedPaths).size || lockedPaths.some((path) => !path.startsWith("upstream/"))) {
    throw new Error("dependency lock paths must be unique upstream files");
  }
  const upstreamPaths = (await collectFiles(join(contractRoot, "upstream")))
    .map((path) => relative(contractRoot, path))
    .sort();
  if (JSON.stringify(lockedPaths) !== JSON.stringify(upstreamPaths)) {
    throw new Error("dependency lock does not exhaustively cover contract/upstream");
  }
  for (const entry of lock.files) {
    const bytes = await readFile(join(contractRoot, entry.path));
    const actual = sha256(bytes);
    if (actual !== entry.sha256) {
      throw new Error(`input digest mismatch for ${entry.path}: expected ${entry.sha256}, found ${actual}`);
    }
  }
}

function publicDescriptor(set) {
  const file = set.file.find((candidate) => candidate.name === "dolgorae/public/v1/dolgorae.proto");
  if (!file) throw new Error("public descriptor file is missing");
  return file;
}

function descriptorInventory(file) {
  const services = file.service.map((service) => ({
    name: service.name,
    full_name: `${file.package}.${service.name}`,
    methods: service.method.map((method) => ({
      name: method.name,
      full_name: `${file.package}.${service.name}.${method.name}`,
      input_type: method.inputType.replace(/^\./, ""),
      output_type: method.outputType.replace(/^\./, ""),
      client_streaming: method.clientStreaming,
      server_streaming: method.serverStreaming,
    })),
  }));

  const messages = [];
  const enums = [];
  function walkMessage(message, prefix) {
    const fullName = `${prefix}.${message.name}`;
    messages.push({
      name: fullName,
      fields: message.field.map((field) => ({
        name: field.name,
        number: field.number,
        label: field.label,
        type: field.type,
        type_name: field.typeName?.replace(/^\./, "") ?? "",
        oneof_index: Object.hasOwn(field, "oneofIndex") ? field.oneofIndex : null,
        proto3_optional: field.proto3Optional,
      })),
      oneofs: message.oneofDecl.map((oneof) => oneof.name),
    });
    for (const nested of message.nestedType) walkMessage(nested, fullName);
    for (const nestedEnum of message.enumType) walkEnum(nestedEnum, fullName);
  }
  function walkEnum(enumType, prefix) {
    enums.push({
      name: `${prefix}.${enumType.name}`,
      values: enumType.value.map((value) => ({ name: value.name, number: value.number })),
    });
  }
  for (const message of file.messageType) walkMessage(message, file.package);
  for (const enumType of file.enumType) walkEnum(enumType, file.package);

  return { services, messages, enums };
}

function parseMarkdownTable(text, start, end) {
  const startIndex = text.indexOf(start);
  const endIndex = text.indexOf(end, startIndex + start.length);
  if (startIndex < 0 || endIndex < 0 || endIndex <= startIndex) throw new Error(`missing Markdown table boundary ${start}..${end}`);
  const section = text.slice(startIndex + start.length, endIndex);
  const lines = section.split("\n").filter((line) => line.startsWith("|"));
  if (lines.length < 3) throw new Error(`incomplete Markdown table section ${start}`);
  const headings = lines[0].split("|").slice(1, -1).map((value) => value.trim());
  return lines.slice(2).map((line) => {
    const values = line.split("|").slice(1, -1).map((value) => value.trim());
    if (values.length !== headings.length) throw new Error(`malformed Markdown table row: ${line}`);
    return Object.fromEntries(headings.map((heading, index) => [heading, values[index]]));
  });
}

function plain(value) {
  return value.replaceAll("`", "").trim();
}

function operationMap(architecture, inventory) {
  const rows = parseMarkdownTable(architecture, "<!-- contract-operation-map:start -->", "<!-- contract-operation-map:end -->");
  const methods = new Map();
  for (const service of inventory.services) {
    for (const method of service.methods) methods.set(`${service.name}.${method.name}`, method);
  }
  const operations = rows.map((row) => {
    const operation = plain(row["Gul semantic operation"]);
    const owner = plain(row["Dolgorae RPC or local/Gul owner"]);
    const wire = plain(row["Exact wire contract"]);
    const upstream = methods.get(owner);
    if (owner.includes("Service.") && !upstream) throw new Error(`${operation} resolves to unknown RPC ${owner}`);
    if (upstream) {
      const expected = `${upstream.input_type.split(".").at(-1)} → ${upstream.server_streaming ? "stream " : ""}${upstream.output_type.split(".").at(-1)}`;
      if (wire !== expected) throw new Error(`${operation} wire mismatch: expected ${expected}, found ${wire}`);
    }
    return {
      operation,
      owner,
      wire,
      controller: plain(row["Required Controller state"]),
      workspace: plain(row["Required workspace state"]),
      idempotency: plain(row["Idempotency identity"]),
      timeout: plain(row["Timeout class"]),
      retry: plain(row["Retry policy"]),
      reconciliation: plain(row["Lost-response reconciliation"]),
      projection: plain(row["Projection effect"]),
      capability: plain(row["Capability requirement"]),
      verification: plain(row["Verification state"]),
    };
  });
  return { schema_version: 1, operations };
}

function eventMap(architecture) {
  const rows = parseMarkdownTable(architecture, "<!-- contract-event-map:start -->", "<!-- contract-event-map:end -->");
  return {
    schema_version: 1,
    exhaustive: true,
    events: rows.map((row) => ({
      variant: plain(row["Envelope/variant"]),
      update: plain(row["Complete update"]),
      invalidation_and_refresh: plain(row["Upstream invalidation and required refresh"]),
      local_effect: plain(row["Gul-local additional effect"]),
      action_safety: plain(row["Action-evaluator safety"]),
    })),
  };
}

function goFakeServer(inventory) {
  const methods = inventory.services.flatMap((service) => service.methods.map((method) => ({ service, method })));
  const bodies = methods.map(({ method }) => {
    const input = method.input_type.split(".").at(-1);
    const output = method.output_type.split(".").at(-1);
    if (method.server_streaming) {
      return `func (s *Server) ${method.name}(context.Context, *connect.Request[v1.${input}], *connect.ServerStream[v1.${output}]) error {\n\tif s.Error != nil {\n\t\treturn s.Error\n\t}\n\treturn nil\n}`;
    }
    return `func (s *Server) ${method.name}(context.Context, *connect.Request[v1.${input}]) (*connect.Response[v1.${output}], error) {\n\tif s.Error != nil {\n\t\treturn nil, s.Error\n\t}\n\treturn connect.NewResponse(&v1.${output}{}), nil\n}`;
  }).join("\n\n");
  const mounts = inventory.services.map((service) => `\tpattern, handler = connectv1.New${service.name}Handler(server)\n\tmux.Handle(pattern, handler)`).join("\n");
  return `// Code generated by contract/scripts/build-contract.mjs. DO NOT EDIT.\npackage fake\n\nimport (\n\t"context"\n\t"net/http"\n\n\t"connectrpc.com/connect"\n\tv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"\n\tconnectv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1/dolgoraev1connect"\n)\n\n// Server returns typed empty success messages for every known descriptor RPC,\n// or Error when a test needs to exercise typed failure transport.\ntype Server struct {\n\tError *connect.Error\n}\n\nfunc NewHandler() http.Handler {\n\treturn NewHandlerWithServer(&Server{})\n}\n\nfunc NewHandlerWithServer(server *Server) http.Handler {\n\tmux := http.NewServeMux()\n\tvar pattern string\n\tvar handler http.Handler\n${mounts}\n\treturn mux\n}\n\n${bodies}\n`;
}

function goFakeTest(inventory) {
  const blocks = inventory.services.map((service) => {
    const calls = service.methods.map((method) => {
      const input = method.input_type.split(".").at(-1);
      if (method.server_streaming) {
        return `\tstream, err := client.${method.name}(ctx, connect.NewRequest(&v1.${input}{}))\n\tif err != nil { t.Fatalf("${service.name}.${method.name}: %v", err) }\n\tfor stream.Receive() {}\n\tif err := stream.Err(); err != nil { t.Fatalf("${service.name}.${method.name} stream: %v", err) }`;
      }
      return `\tif _, err := client.${method.name}(ctx, connect.NewRequest(&v1.${input}{})); err != nil { t.Fatalf("${service.name}.${method.name}: %v", err) }`;
    }).join("\n");
    return `func Test${service.name}(t *testing.T) {\n\tt.Parallel()\n\tserver := newGRPCServer(t)\n\tclient := connectv1.New${service.name}Client(server.Client(), server.URL, connect.WithGRPC())\n\tctx := context.Background()\n${calls}\n}`;
  }).join("\n\n");
  return `// Code generated by contract/scripts/build-contract.mjs. DO NOT EDIT.\npackage fake\n\nimport (\n\t"context"\n\t"errors"\n\t"net/http"\n\t"net/http/httptest"\n\t"testing"\n\n\t"connectrpc.com/connect"\n\tv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"\n\tconnectv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1/dolgoraev1connect"\n)\n\nfunc newGRPCServer(t *testing.T) *httptest.Server {\n\tt.Helper()\n\treturn newGRPCServerWithHandler(t, NewHandler())\n}\n\nfunc newGRPCServerWithHandler(t *testing.T, handler http.Handler) *httptest.Server {\n\tt.Helper()\n\tserver := httptest.NewUnstartedServer(handler)\n\tserver.EnableHTTP2 = true\n\tserver.StartTLS()\n\tt.Cleanup(server.Close)\n\treturn server\n}\n\nfunc TestTypedErrorDetailRoundTrip(t *testing.T) {\n\trpcError := connect.NewError(connect.CodeFailedPrecondition, errors.New("typed fixture"))\n\tdetail, err := connect.NewErrorDetail(&v1.DolgoraeErrorDetail{})\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\trpcError.AddDetail(detail)\n\tserver := newGRPCServerWithHandler(t, NewHandlerWithServer(&Server{Error: rpcError}))\n\tclient := connectv1.NewRuntimeServiceClient(server.Client(), server.URL, connect.WithGRPC())\n\t_, callErr := client.GetCapabilities(context.Background(), connect.NewRequest(&v1.GetCapabilitiesRequest{}))\n\tvar connectErr *connect.Error\n\tif !errors.As(callErr, &connectErr) {\n\t\tt.Fatalf("expected connect error, got %v", callErr)\n\t}\n\tdetails := connectErr.Details()\n\tif len(details) != 1 {\n\t\tt.Fatalf("expected one typed detail, got %d", len(details))\n\t}\n\tvalue, err := details[0].Value()\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\tif _, ok := value.(*v1.DolgoraeErrorDetail); !ok {\n\t\tt.Fatalf("unexpected typed detail %T", value)\n\t}\n}\n\n${blocks}\n`;
}

function consumerFixtures(operations, events, clientPolicy, mutations, descriptorMetadata, consumerProfile, providerFixtures) {
  return {
    schema_version: 1,
    protocol_handshake: {
      initial_protocol_version: 0,
      minimum: descriptorMetadata.minimum_client_protocol_version,
      maximum: descriptorMetadata.maximum_client_protocol_version,
      cases: [
        { name: "accepted-version", server: descriptorMetadata.protocol_version, outcome: "compatible" },
        { name: "below-range", server: descriptorMetadata.minimum_client_protocol_version - 1, outcome: "ProtocolIncompatible" },
        { name: "above-range", server: descriptorMetadata.maximum_client_protocol_version + 1, outcome: "ProtocolIncompatible" }
      ]
    },
    interaction_limits: {
      provider_response_bytes: clientPolicy.interaction_policy.maximum_response_bytes,
      gul_response_bytes: 65536,
      effective_response_bytes: Math.min(clientPolicy.interaction_policy.maximum_response_bytes, 65536),
      provider_safe_payload_bytes: clientPolicy.interaction_policy.maximum_safe_payload_bytes,
      gul_safe_payload_bytes: 8388608,
      effective_safe_payload_bytes: Math.min(clientPolicy.interaction_policy.maximum_safe_payload_bytes, 8388608),
      protected_response_persistence: "forbidden",
      protected_response_replay: "forbidden"
    },
    replay_material: {
      StartRun: "protected_durable_exact_request",
      SubmitTurn: "process_memory_only",
      ResolveInteraction: "never_persist_or_replay"
    },
    projection_convergence: [
      { name: "equal-complete-stamps", run: 7, writer: 7, interaction: 7, floor: 7, enables: true },
      { name: "writer-behind-floor", run: 8, writer: 7, interaction: 8, floor: 8, enables: false },
      { name: "partial-event-disable-only", run: 9, writer: null, interaction: 9, floor: 9, enables: false },
      { name: "timeline-head-behind", run: 10, writer: 10, interaction: 10, floor: 10, timeline_head: 9, enables: false }
    ],
    stream_end: clientPolicy.stream_end_policy,
    event_invalidation_variants: events.events.map((entry) => entry.variant),
    required_rpcs: consumerProfile.required_methods,
    unavailable_rpcs: consumerProfile.unavailable_until_later_tasks,
    producer_case_catalog: providerFixtures,
    production_cli_fallback: false,
    unknown_required_enum: "reject",
    unknown_optional_field: "preserve_only_when_allowlisted"
  };
}

function credentialBoundary(capabilities) {
  const properties = capabilities.properties.controller_credential.properties;
  const value = (name) => properties[name].const;
  const carrierRoot = capabilities.properties.controller_carrier_root.const.replace(/^home\//, "~/");
  const clientDescendant = value("client_descendant_pattern").replace("<client>", "gul").replace(/\/$/, "");
  return {
    schema_version: 1,
    schema_id: value("schema_id"),
    credential_schema_version: value("schema_version"),
    credential_schema_sha256: value("schema_sha256"),
    accepted_kinds: value("accepted_kinds"),
    capability_byte_length: value("capability_byte_length"),
    capability_encoding: value("capability_encoding"),
    carrier_root: `${carrierRoot}/${clientDescendant}`,
    parent_directory_mode: value("parent_directory_mode"),
    file_mode: value("file_mode"),
    same_uid: value("same_uid"),
    regular_file: value("regular_file"),
    symlinks: value("symlinks"),
    create_exclusive: value("create_exclusive"),
    maximum_file_bytes: value("maximum_file_bytes"),
    client_descendant_pattern: clientDescendant,
    normalized_principal: value("normalized_principal"),
    initial_generation: value("initial_generation"),
    operator_capability: "forbidden",
    overwrite: false,
  };
}

async function collectFiles(root) {
  const found = [];
  async function walk(directory) {
    for (const name of (await readdir(directory)).sort()) {
      const path = join(directory, name);
      const info = await stat(path);
      if (info.isDirectory()) await walk(path);
      else if (name !== "generated-lock.json") found.push(path);
    }
  }
  await walk(root);
  return found;
}

await validateInputs();
const descriptorBytes = await readFile(join(contractRoot, "upstream/dolgorae-public-v1.descriptor.pb"));
const set = fromBinary(FileDescriptorSetSchema, descriptorBytes);
const descriptor = publicDescriptor(set);
const inventoryData = descriptorInventory(descriptor);
const inventory = {
  schema_version: 1,
  source_revision: lock.source.revision,
  source_descriptor_sha256: sha256(descriptorBytes),
  package: descriptor.package,
  service_count: inventoryData.services.length,
  method_count: inventoryData.services.reduce((count, service) => count + service.methods.length, 0),
  message_count: inventoryData.messages.length,
  enum_count: inventoryData.enums.length,
  ...inventoryData,
};
if (inventory.service_count !== 8 || inventory.method_count !== 36) {
  throw new Error(`expected 8 services and 36 public methods, found ${inventory.service_count} services and ${inventory.method_count} methods`);
}

const architecture = await readFile(join(repoRoot, "docs/architecture.md"), "utf8");
const operations = operationMap(architecture, inventory);
const events = eventMap(architecture);
const clientPolicy = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-grpc-client-policy-v1.json"), "utf8"));
const mutations = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-rpc-mutation-policy-v1.json"), "utf8"));
const descriptorMetadata = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-public-v1.descriptor.json"), "utf8"));
const capabilities = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-capabilities-v1.schema.json"), "utf8"));
const errorActions = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-grpc-error-mapping-v1.json"), "utf8"));
const consumerProfile = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-gul-consumer-v1.json"), "utf8"));
const providerFixtures = JSON.parse(await readFile(join(contractRoot, "upstream/dolgorae-gul-consumer-v1.fixtures.json"), "utf8"));
const fixtures = consumerFixtures(operations, events, clientPolicy, mutations, descriptorMetadata, consumerProfile, providerFixtures);

if (!process.argv.includes("--lock-only")) {
  const credentialBytes = await readFile(join(contractRoot, "upstream/dolgorae-controller-credential-v1.schema.json"));
  const admissionPolicy = JSON.parse(await readFile(join(contractRoot, "admission-policy.json"), "utf8"));
  await writeJson(join(outputRoot, "policy/release-admission.v1.json"),
    releaseAdmission(lock, consumerProfile, JSON.parse(credentialBytes), sha256(credentialBytes), admissionPolicy));
  await writeJson(join(outputRoot, "contract-inventory.v1.json"), inventory);
  await writeJson(join(outputRoot, "gul-operation-map.v1.json"), operations);
  await writeJson(join(outputRoot, "event-invalidation-map.v1.json"), events);
  await writeJson(join(outputRoot, "fixtures/consumer-policy.v1.json"), fixtures);
  await writeJson(join(outputRoot, "policy/capability-map.v1.json"), {
    schema_version: 1,
    known_methods: capabilities.properties.grpc_methods.items.enum,
    required_methods: consumerProfile.required_methods,
    unavailable_methods: consumerProfile.unavailable_until_later_tasks,
    stage_requirements: capabilities.properties.grpc_methods["x-stageRequirements"],
    gul_operation_requirements: operations.operations.map((entry) => ({ operation: entry.operation, capability: entry.capability }))
  });
  await writeJson(join(outputRoot, "policy/error-action-map.v1.json"), {
    schema_version: 1,
    fail_closed_on_unknown_detail: true,
    mapping: errorActions
  });
  await writeJson(join(outputRoot, "policy/mutation-map.v1.json"), {
    schema_version: 1,
    transparent_grpc_retries: false,
    upstream: mutations.mutations,
    gul_operation_idempotency: operations.operations.map((entry) => ({
      operation: entry.operation,
      owner: entry.owner,
      idempotency: entry.idempotency,
      retry: entry.retry,
      reconciliation: entry.reconciliation,
    })),
  });
  await writeJson(join(outputRoot, "policy/identifier-enum-map.v1.json"), {
    schema_version: 1,
    identifiers: inventory.messages.filter((message) => [".RequestContext", ".ResponseContext", ".WorkspaceRef", ".RunRef", ".ControllerCarrierRef", ".ProjectionStamp"].some((suffix) => message.name.endsWith(suffix))),
    enums: inventory.enums
  });
  await writeJson(join(outputRoot, "policy/projection-replay-map.v1.json"), {
    schema_version: 1,
    event_invalidation: events.events,
    convergence: fixtures.projection_convergence,
    replay_material: fixtures.replay_material,
    stream_end: fixtures.stream_end
  });
  await writeJson(join(outputRoot, "policy/credential-boundary.v1.json"), credentialBoundary(capabilities));
  await writeJson(join(outputRoot, "fixtures/machine-cli-comparison.v1.json"), {
    schema_version: 1,
    schema_sha256: lock.files.find((entry) => entry.path.endsWith("dolgorae-machine-v1.schema.json")).sha256,
    production_fallback: false,
    valid: {
      schema_version: 1,
      ok: true,
      command: "help",
      invocation_id: "019d0000-0000-7000-8000-000000000001",
      data: { text: "diagnostic-only comparison fixture" },
    },
    invalid: {
      schema_version: 1,
      ok: true,
      command: "help",
      invocation_id: "019d0000-0000-7000-8000-000000000001",
      data: { text: "diagnostic-only comparison fixture" },
      private_worker_socket: "/forbidden",
    },
  });
  await mkdir(join(outputRoot, "go/fake"), { recursive: true });
  await writeFile(join(outputRoot, "go/fake/server.go"), goFakeServer(inventory));
  await writeFile(join(outputRoot, "go/fake/server_test.go"), goFakeTest(inventory));
}

if (process.argv.includes("--lock-only")) {
  const files = await collectFiles(outputRoot);
  await writeJson(join(outputRoot, "generated-lock.json"), {
    schema_version: 1,
    source_revision: lock.source.revision,
    files: await Promise.all(files.map(async (path) => ({
      path: relative(outputRoot, path),
      sha256: sha256(await readFile(path))
    })))
  });
}
