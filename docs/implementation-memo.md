# Gul: Implementation Memo

| Field | Value |
|---|---|
| Role | Non-normative implementation observations, dependencies, risks, and handoff |
| Product | Gul |
| Version | 0.1-dolgorae-consumer-v1 |
| Last updated | 2026-09-22 |

## 1. Boundary

This memo records current investigation and execution context. It does not override Required Specifications, accepted ADRs, Architecture, or Roadmap. An observation here is not Current State and does not freeze an external protocol.

## 1.1 Approved consumer rebaseline, 2026-09-20

This amendment adopts ADR-0050/0051 and producer-owned `dolgorae.gul-consumer/v1`
requirements. It does not regenerate artifacts, implement a component, complete
E12, or establish live compatibility. The revised roadmap separates pre-release
fake-based work from post-release adapter/integration work. No Task is active.

| New boundary | Current authority or remaining owner |
| --- | --- |
| Immutable producer lock | **Available:** Dolgorae TASK-053 completed at `21aefe5b2a8dc6fb18a58338090348b23d2f0a4a` with 27 required methods, complete timeline, two additive aggregate reads, exact hashes and bounded fixtures |
| New Gul consumer pin | **Accepted by E12-T1:** immutable TASK-053 inputs and regenerated consumer outputs |
| Consumer-realistic fakes | E13-T1, replacing retired E12-T2: independently tested history/session/result/admission/failure scenarios |
| Pre-release readiness | E14-T1, replacing retired E12-T3: assembled authenticated core/browser with fakes and explicit remaining live edges |
| Released-provider integration | E2/E9: exact released v0.1.3 artifact, actual UDS/carriers/RPC and browser evidence |
| Future FSM display | Deferred-Podway: inspect the source contract before implementation IDs; read-only end to end |

Provider status at this planning snapshot: TASK-025 is Completed at `57e6be8`,
and TASK-053 is Completed at immutable commit
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a`. The contract-ready gate is therefore
satisfied. The remaining producer order is 047, 048, 049, 050, 051, 054, 055,
052, 056, 026. TASK-054 owns complete timeline, TASK-055 owns aggregate/result
reads, TASK-052 owns whole-session close, TASK-056 owns old-client compatibility,
and TASK-026 owns final acceptance. This proves the checked producer contract,
not Gul adoption, runtime implementation, or released-provider compatibility.

The product uses drafts and explicit send after an active Turn, with no queue,
steering, auto-send or auto-interrupt; approval answers remain available. Session
close covers the whole owned aggregate through the root and Broker. Gul neither
controls children nor infers closed success while effects remain unknown.
Accepted original prompts and safe results remain readable after closure.

The older assumption/risk/evidence tables below describe their original E0
baseline unless retained by ADR-0050. Old Gate A/B hashes, the 34-method inventory,
mandatory first-release continuation, and the child-navigation prohibition are
not new consumer authority. E0 remains completed history; E12 owns the new pin.
Retained security, replay, convergence, FileService and transport principles
remain applicable. A plan or fake never advertises actual runtime support.

## 1.2 Review refinements, 2026-09-21

The producer TASK-053 lock is available at immutable commit
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a`; E12-T1 subsequently accepted its
Gul consumer pin. This revision declares browser
ListPromptHistory/GetPromptHistoryItem/GetExecutionState/ListSpecialistResults
contracts, bounded Gul tokens and references, and CloseOutcome distinctions.
Provider cursors and close operation IDs remain backend correlations.
E1-T3 owns browser contract fixtures, E3-T3 passive execution-state reads, E4-T5
history/result reads, E4-T3 the shared evaluator and E5-T1 the complete close
coordinator. Real evidence remains in E2/E9; no earlier Task provides a temporary
unsafe close route. REQ-RUNTIME-022 retains its historical typed-state scope;
REQ-CONSUMER-001/E12-T1 alone own the new pin.

The documentation checker must match the Active Task header to active rows and
preserve permanent retired/reserved IDs through the canonical registry and
historical baseline, without a fixed total count. Negative tests cover pointer
mismatch and identity loss alongside existing DAG/phase/owner/link checks.
These checks prove document structure, not runtime or generated-contract readiness.
The host currently has Buf 1.69.0, which satisfies the current
`>=1.66.1,<2.0.0` range and reproduces the checked descriptor. Buf is resolved
from the system `PATH` for lint, descriptor construction, and additive checks;
different descriptor bytes fail closed. Language-client tools remain exactly pinned. No tool installation
is implied, and tool availability is not wire or semantic acceptance.

Architecture Section 6.4 now carries one checked E12 operation map. Contract
generation rejects RPCs absent from the descriptor and derives the required and
unavailable sets from the pinned consumer profile rather than treating descriptor
presence as runtime support.

## 1.3 Whole-Epic execution reorganization, 2026-09-21

ADR-0052 responds to Master's request to drive development through one complete
Aquarium `epic-handler` invocation at a time. The earlier Task DAG was valid,
but E12's pin preceded E1 while its fake and final-readiness Tasks required E1
and later feature Epics. That prevented whole-Epic execution. E8 also appeared
in separate early and late passes.

The roadmap now groups each required Epic's Tasks contiguously and displays the
Epics in execution order. E12 retains only contract pinning. E13-T1 replaces
unstarted E12-T2 for stateful provider scenarios; E14-T1 replaces E12-T3 for
assembled pre-release acceptance. Both old IDs remain Retired with unchanged
registry entries. E8 delivers authentication and packaging together after UI.
No existing E0 completion or product feature is changed. The contract-ready gate
is recorded as satisfied by TASK-053; the released-provider gate remains pending.
No Task is activated and no provider checkout is modified.

Completion boundaries now distinguish E1's isolated foundation tests, E13's
provider scenario tests, each feature Epic's behavior, E8 authentication and
packaging, and E14 assembly. Required Specifications move full authenticated
host/bundle/API acceptance to E14, shell attachment and PWA qualification to E8,
actual headless provider assembly to E2, and browser mutation retry behavior to
E5. REQ-CONSUMER-004 names assembled fake-application acceptance separately from
the provider fake harness. Early production access remains denied, rather than
using a temporary authentication bypass to finish an Epic.

## 1.4 E12-T1 contract pin completion, 2026-09-22

E12-T1 imports immutable TASK-053 commit
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a` and records dependency-lock SHA-256
`8f52ae66e126f37013d7842b2113fc509d21af4e4fc465cecdbeef7e21619f01` plus
generated-lock SHA-256
`6284064e720e2220d6960c42faef6a4c13292ce1327f44e00388dc52b2e17d4a`.
The generator reproduces the 8-service, 36-method descriptor with Buf 1.69,
checks additive compatibility against the pre-TASK-053 descriptor, and emits
typed Go/TypeScript clients, an all-method transport fake, 31 semantic operations,
and explicit 27-required/9-unavailable policy maps. Validation covers global
Profile requests without `WorkspaceRef`, the fixed carrier root, no Operator or
production CLI fallback, CloseRun operation correlation and forbidden retry,
field sourceability, producer-lock correlation, and exact generated hashes.
This promotes REQ-CONSUMER-001 as contract/tooling Current State only. No Gul
product runtime, stateful scenario fake, released executable, or live provider
compatibility is claimed.

The current E4 scope excludes Deferred E4-T4. Required release completion also
excludes Retired IDs, and the old generic successor E2E wording now names the
first-release unsupported-continuation check instead. Each consumer Epic has a
separate link to the shared dossier so its closeout cannot delete another Epic's
remaining acceptance context. The final consumer owns dossier deletion.

Verification for E12-T1 includes contract regeneration and drift comparison,
Go and TypeScript compilation, all-method fake transport tests, descriptor
breaking fixtures, contract/schema/policy validation, SOT checks and their
negative fixtures, toolchain fixtures, whitespace checks, and the serial
`make test` facade. These checks certify the generated contract boundary only.
Product implementation and live integration remain absent. TASK-053 completion
is immutable producer evidence, not an inference from Gul's local tests.

## 2. Current development snapshot

| Area | State |
|---|---|
| Five Gul SOT documents | E0-T4 completed the consumer alignment and Gate A reproduction; E0-T8 completed toolchain/ADR alignment; E0-T7 completed Gate B |
| Toolchain and developer-command artifacts | E0-T8 accepted one pin manifest and read-only host checks; E0-T7 adds checked contract generation/drift delegates; no installer or application scaffold |
| Contract boundary | E12-T1 pins TASK-053 and regenerates checked clients/maps/fake transport for 36 known, 27 required, and 9 unavailable methods |
| Production source | None |
| Wails host/frontend | Not implemented |
| ConnectRPC schema/services | Not implemented |
| Gul SQLite schema | Not implemented |
| Dolgorae RPC supervisor/provider | Not implemented |
| Controller credential store | Caller-owned mechanism selected by ADR-0047; not implemented |
| FileService/auth/PWA/Tailscale integration | Not implemented |
| Current State promotions | REQ-HOST-005, REQ-RUNTIME-011, REQ-RUNTIME-022, and REQ-CONSUMER-001; no product runtime behavior |

The repository contains documentation, pre-implementation bootstrap validation, and a checked provider-contract boundary candidate. No application source or live runtime behavior has completed review.

## 3. Historical E0 assumption snapshot

This table records the earlier E0 design context, not current contract readiness.
Any Accepted, published, complete or Gate A/B wording in this historical table
refers only to that earlier scope. Sections 1.1 and 2 record the current state:
TASK-053 publication is complete at
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a`; E12-T1 has accepted that contract
boundary. Current product rules are in Required Specifications and ADR-0050/0051,
not these archived assumptions.

| ID | Historical assumption | Status at the recorded baseline |
|---|---|---|
| ASM-010 | Gul is a single-user, LLM-free remote operator interface. | Required Specifications; Accepted |
| ASM-011 | Dolgorae is the sole execution provider; the first-release product is its Orchestrated Session consumer profile. | ADR-0050; Approved Required State |
| ASM-012 | Dolgorae owns all Codex runtime, interaction, writer, policy, assurance, and recovery authority. | ADR-0023/0026; Accepted target |
| ASM-013 | Every Direct Session references one Dolgorae Run, not a Codex thread. | ADR-0024; Accepted target |
| ASM-014 | Gul owns UI, auth, FileService, presentation state, and client delivery. | Required Specifications; Accepted target |
| ASM-015 | Runtime Providers are trusted built-in out-of-process adapters. | ADR-0028; Accepted target |
| ASM-016 | Gorae is a future provider and is deferred from v0.1. | ADR-0030; Accepted target |
| ASM-017 | Local gRPC is the accepted production target, but an accepted pinned executable and Protobuf contract are required before provider implementation. | ADR-0043/E0-T7/E2-T0; contract published, executable unverified |
| ASM-018 | Gul never possesses an Operator capability; Controller reset is external and Gul performs only verified Controller adoption. | ADR-0027; Accepted target |
| ASM-019 | Runtime calls use a verified canonical workspace path; Runtime Workspace ID/digest only verifies identity. | ADR-0009; Accepted target |
| ASM-020 | Gul policy assigns one distinct Controller credential to each Direct Session/Run pair; this is not asserted as a universal Dolgorae invariant. | ADR-0035; Accepted target |
| ASM-021 | A threadless Run with `threadless_acquire_write=false` activates its first writer through SubmitTurn(WRITE); unsupported or unverified existing-thread transition uses CreateWriteContinuation. | ADR-0036/0044; Accepted target |
| ASM-022 | Observation reads (activity list, status, pending summaries, client-safe events and writer status) need no runtime capability, so state stays visible while a Controller binding is unhealthy. | Upstream contract observation; Unverified dependency |
| ASM-023 | One shared gRPC channel carries at most eight logical per-Run streams; all other Runs are polled within ten seconds. | ADR-0043, REQ-PROJ-012/013; Accepted target |
| ASM-024 | File freshness comes from a bounded host filesystem watcher, so no Gul feature requires the upstream operational event projection. | ADR-0038; Accepted target |
| ASM-025 | Gul locally creates exact-schema Controller carriers below Dolgorae's Gul carrier root, stores only logical keys, and revalidates the derived path before every authorized RPC; Dolgorae verifies/binds but does not create them. | ADR-0047; Accepted target |
| ASM-026 | Workspace registration works from a remote client through a host-configured root allowlist, so nothing except the shell itself depends on the desktop shell. | ADR-0039/0040; Accepted target |
| ASM-027 | Requirements carry a release tier, and release qualification requires every Task `Completed`; the single-active-task rule is retained. | ADR-0041; Accepted target |
| ASM-028 | A profile tool may use a trusted external broker for an independent subagent, but Gul receives only the parent-Turn result and owns no child activity, credential, lifecycle, or mutation surface. | REQ-DIRECT-015/ADR-0021; Accepted target |
| ASM-029 | Timeline and Artifact capabilities are required for v0.1; local history caches and artifact references are not runtime authority or filesystem paths. | ADR-0045; Accepted target |
| ASM-030 | `gul serve` and Gul.app share one user-wide core; Gul.app attaches to a verified existing headless core. | ADR-0046; Accepted target |
| ASM-031 | Machine CLI is explicit diagnostics/conformance only and is never a production fallback. | ADR-0043; Accepted target |
| ASM-032 | Gul owns socket-parent validation and path selection, while Dolgorae owns gateway singleton state, socket bind/chmod, stale proof/unlink, and cleanup. | ADR-0043; Accepted target |
| ASM-033 | The accepted public gRPC wire contract contains the complete semantic interface required by Gul; Gate A is closed at Dolgorae `85a8862f784cc57701751d81a9e03bf7c5722818`, and E0-T7's checked source pin, clients, maps, fixtures, and fake server close Gate B. | REQ-RUNTIME-022/Gates A-B; Gate B closed |
| ASM-034 | Dolgorae's event invalidation matrix is a mandatory minimum; Gul may add local invalidation but cannot omit a provider-required Run, Writer, Interaction, or timeline refresh. | ADR-0048/REQ-PROJ-018; Accepted target |
| ASM-035 | Mutation enablement requires fresh compatible Run, Writer, and Interaction ProjectionStamps plus a compatible timeline captured head. | ADR-0048/REQ-PROJ-019; Accepted target |
| ASM-036 | Crash-safe replay material is retained only for StartRun and CreateWriteContinuation; SubmitTurn prompts/images and protected Interaction input are never durably retained for replay. | ADR-0049/REQ-REC-011; Accepted target |
| ASM-037 | Unresolved `StartRun` and `CreateWriteContinuation` replay envelopes have a fixed 72-hour v0.1 maximum retention, configurable only downward; startup and at-least-six-hourly purge delete expired canonical material but preserve the non-secret attempt as `OutcomeUnknown`. | ADR-0049/REQ-REC-011; Accepted target |
| ASM-038 | Allocation replay stores role-tagged logical Controller references only: `StartRun` uses a destination credential-store key/Controller ID, while continuation uses separate source binding/Controller and destination credential-store/Controller references; absolute carrier paths and capabilities are never persisted. | ADR-0049/REQ-REC-011; Accepted target |

Superseded assumptions about one Gul-managed App Server, Session-to-thread mapping, local WriteLock, local App Server recovery and raw App Server request handling are retained only in Section 11 as historical context.

## 4. Historical external dependency observations

These observations are retained from the 2026-08-23 E0 inspection. They do not
describe today's producer checkout or satisfy the new consumer contract. In
particular, the earlier pre-implementation status and completeness claims below
must not be used to start E12 or certify v0.1.3 compatibility.

### 4.1 Dolgorae at the E0 snapshot

The source inspected for E0 was repository `git@github-irootkernel:irootkernel/dolgorae.git`, with clean accepted remote-main revision `85a8862f784cc57701751d81a9e03bf7c5722818` inspected on 2026-08-23. The local sibling `../dolgorae` is read-only context and its unrelated dirty/ahead state is not evidence. The accepted revision supplies the complete public RPC inventory, exact request/response types, typed Run/events, `RunConfigurationProjection`, typed Controller Interaction payload, Interaction byte limits, continuation required action, opaque-path handling, and normative event aggregate semantics required by Gul.

Architecture 6.4 records every semantic-to-RPC mapping including `RefreshRunSnapshot` → `RunService.GetRun`. Gate A is closed by the clean revision and deterministic checked-artifact evidence. The descriptor reproduces byte-for-byte with protoc 35.1 plus official Protobuf v32.1 commit `7fcfd66022455635fa29af92987cdc0967efd4f3`'s `google/protobuf/timestamp.proto` (SHA-256 `14052c6042c1dd2d0b50245f2812eaab6eaf82db0b6e8ce483eae527f73b6ee8`); Buf 1.66.1 lint passes. E0-T7/Gate B pins those sources and digests, generates Go/TypeScript clients, calls all 34 RPCs through a descriptor-derived fake server, and freezes 36-operation, 20-event, capability, identifier/enum, error/action, mutation, replay, convergence, limit, and unsupported-RPC policy evidence. E2-T0 separately pins a reproducible executable/release plus live evidence.

Upstream delivery status in the inspected snapshot remains pre-implementation: the tree contains documentation, protocol artifacts, and validator tooling but no product source. Gul's E2 vertical slice depends on the upstream runtime and the corrected public interface. Gate B fixtures are now checked locally, but provider implementation/live evidence still wait for an accepted compatible executable under E2-T0. This is concurrent development across two repositories, not a version-skew workaround.

The older CLI contract remains machine-checkable and supplies the exact closed-schema conformance baseline. It is not mechanically reinterpreted as gRPC. E0-T7 generates the gRPC inventory only from the revised accepted Protobuf artifacts, keeps a valid/invalid exact-schema CLI comparison fixture separate, and forbids production CLI fallback.

The later Dolgorae SOT revision standardizes a CLI-first brokered independent-
subagent composition while leaving MCP as a future thin adapter. Gul does not
consume that child CLI surface directly. Its only v0.1 obligation is to preserve
and render bounded tool/result material already present in the parent Run's safe
projection without creating child-owned domain or credential state.

The upstream project was comprehensively renamed from Gomchi to Dolgorae at commit `79950870963d5740ff08c59b8d92a8091a556488`. The current sibling checkout is `../dolgorae`; legacy checkout names are historical evidence only. This repository aligns its `gul` repository and checkout names with the authoritative Gul product identity. Directory names alone are still not identity evidence: scripts, bundle IDs, persistence directories, documentation scanners, and release artifacts must use explicit product and repository provenance.

### 4.2 Wails, ConnectRPC, Tailscale, and Git

The retained direction is Wails v3 with a Go core, generated ConnectRPC clients, one responsive frontend, Tailscale Serve to a loopback listener, and bounded read-only host Git operations. Versions and changed upstream behavior require E0-T8 verification before implementation.

## 5. Historical E0 dependency ledger

E12-T1 adopted the producer lock at Dolgorae TASK-053 completion commit
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a`. The live dependency and generated
lock paths now carry the E12 digests recorded in section 1.4. The E0 digests in
this historical table remain provenance records only and are not the contents
of those live paths.

Gate A's earlier source and checked-artifact digests are recorded below. E0-T7 records them in `contract/dependency-lock.json` (SHA-256 `c4f91aa3e2add1093880684e5c96fdbb6239aef6a85261a0adf6b585e2db8863`) and records every generated output in `contract/generated/generated-lock.json` (SHA-256 `8a6a614a3a08c585f9a62f74095a0237d47feefba802e2dfa3ce13be5fbe0bf6`).

| Historical dependency field | E0 recorded value, not the new consumer pin | Original evidence requirement | Original owner |
|---|---|---|---|
| Dolgorae SOT source | `git@github-irootkernel:irootkernel/dolgorae.git` remote `main` at `85a8862f784cc57701751d81a9e03bf7c5722818`; clean Gate A source | Pin with the Gul revision and reject source drift | E0-T7 |
| Dolgorae executable | Unset; E2-T0 waits for the exact released v0.1.3 artifact after TASK-026 and RC QA | Resolved absolute executable and provenance | E2-T0 |
| Dolgorae semantic version | Unset | Accepted release/version output | E2-T0 |
| Public gRPC proto/descriptor digest | Proto `bdb9916026f82725c1f0592a4d00a280dca43ef8fc968e3503cbf61059d09163`; descriptor `22e605dddc26c145ab6c682955fa4bfcf078b8356d38e94982e118b948965318` | Accepted protocol range, generated inventory, and pinned reproduction check | E0-T7/E2-T0 |
| Capability schema digest | `99393b430f014eec861cdba57258594f21cb65d2f3b1c9dfc23a3c0051e067e7` | Required capability/version/limit matrix | E0-T7 |
| Event projection/type digest | Typed event `oneof` is in the accepted descriptor; client policy `8f8563bd6265c2940805c107b756d198e3d168379f291e4a74202db557c4333e` | Exact invalidation map, cursor/replay/heartbeat/termination fixtures; Gul cap is eight streams | E0-T7/E2-T0 |
| Credential schema digest | `6e888023fa6f12964afbd2867832944307dc626cad3fe6c6bc517768ab98b84f` | Exact local-store fixtures | E0-T7/E2-T2 |
| Mutation registry digest | `d734044d2e1d8c814e36e803717d5f022e9497c87d2a8cf73285ee750d483a9b` | Mutation/idempotency/timeout/reconciliation matrix | E0-T7 |
| Error/action registry digest | `854a101e279950777b8989b0c314d745300fffa3ae499608a15ada236a62f256` | Exhaustive typed error and required-action mapping | E0-T7 |
| gRPC conformance digest | `c81e34927e37254f98400f2aa678aac912ec28e131c669829ab2ab2c50ac5cd6` | Generated fixture reproduction | E0-T7 |
| Verification index digest | `dca027e165b95010ab96b771ac6f957120a68316a7b7867dab71cbfb86123948` | Complete artifact membership check | E0-T7 |
| Machine CLI contract | Existing v1 JSON schemas only | Exact closed-schema diagnostic/conformance fixtures, never production fallback | E0-T7 |
| Binary digest/identity policy | Unset | ADR-backed identity/change handling | E2-T0 |
| Supported capability contract | Accepted artifacts include the complete method list and all required typed state, Run configuration, typed Interaction payload, Interaction limits, continuation action, timeline/artifact bounds, and opaque-path contract | Operations, IDs, typed projections, Interaction/action/limit contracts, exact event invalidation, replay policy, timeline/artifact bounds, and unsupported-RPC matrix | E0-T7/E2-T0 |
| Public transport | Accepted target: supervised local gRPC over private UDS | Pinned release/descriptor/lifecycle evidence; private, TCP, REST, and operator interfaces excluded | E0-T7/E2-T0 |
| Gul adapter version | Unset | Versioned adapter contract | E2-T1 |
| Last successful compatibility probe | None | Timestamp, environment, result, redacted evidence | E2-T1 |
| Go/Wails/Node/Bun/TypeScript/React | Go `1.26.6`; Wails `3.0.0-beta.8` (`wails3`); Node `26.7.0`; system Bun `>=1.4.2`; TypeScript `7.0.2`; React/DOM `19.2.7` | `toolchain/versions.env`; exact checks except Bun's minimum-only system policy; clean-host fixtures | E0-T8 |
| Protobuf/Buf/ConnectRPC | System Buf `>=1.66.1,<2.0.0`; protoc `35.1`; protoc-gen-go/protobuf-go `1.36.12`; connect-go/protoc-gen-connect-go `1.20.0`; Connect-ES/Web `2.1.2`; Protobuf-ES/protoc-gen-es `2.14.0`; historical protoc-gen-connect-es `1.7.0` | Historical E0-T8 manifest; Buf lint must pass and generated dependency manifests must match before use | E0-T8 |
| Host platform | macOS `>=14.0.0`, `arm64`; Git `>=2.39.0,<3.0.0` | Range checks; no installation or mutation | E0-T8 |
| SQLite driver/settings | modernc.org/sqlite `1.57.0`; WAL; foreign keys; `synchronous=FULL`; 5-second busy timeout; one writer and at most four read-only connections; immediate transactional sequence allocation; checkpoint plus `VACUUM INTO` backup | Accepted ADR-0017 and future E1-T4 fault/race/backup tests | E0-T8 |

## 6. Historical E0 decision register

This records decisions and questions at E0. It is not the current execution gate
list. The current contract-ready and released-provider gates are in the roadmap;
ADR-0050 resolves the new input/closure/scope decisions. Source-only SVG remains
the safe first-release default unless a later accepted ADR changes it.

| Decision | Status | Required action | Blocking scope |
|---|---|---|---|
| Dolgorae executable and live compatibility | Historical E0 contract evidence exists; the new immutable contract starts at TASK-053 and the exact release follows TASK-026 plus RC QA | Complete the pre-release Epic sequence through E14 before E2-T0 pins the released executable, lifecycle, version, binary identity, and smoke evidence. | E2-T0 release-gated; does not block pre-release work |
| Gorae release scope | Deferred by ADR-0030 | Explicit SOT/ADR approval required to enter v0.1. | Deferred epic |
| Local draft Direct Session behavior | Unspecified | Decide whether a presentation may exist before Run creation and define cleanup/idempotency. | E3-T3 |
| SVG preview | ADR-0018 Proposed | Select source-only, rasterization, or isolated sanitization. | Active SVG preview |

No unresolved item is silently decided by this memo. Source-only SVG remains the safe fallback, and explicit refresh remains the Release-tier behavior beneath the Recommended-tier watcher.

Decisions closed in the historical E0 round: ADR-0017 selects modernc SQLite with bounded WAL concurrency, immediate transactional delivery-sequence allocation, and checkpointed `VACUUM INTO` backup. ADR-0043 assigns socket lifecycle ownership while retaining supervised local gRPC and diagnostic-only CLI; ADR-0044 requires the typed-state evaluator and threadless first-write flow; ADR-0045 requires timeline and Artifact capabilities; ADR-0046 selects one shared headless/desktop core; ADR-0047 assigns exact-schema credential creation/storage to Gul and verification/binding to Dolgorae. Gate A clean publication and reproduction are complete; E0-T7 supplies Gate B's checked consumer maps, replay/convergence fixtures, generated clients/fake server, and digest manifests, while executable/version evidence remains E2-T0 work.

## 7. Historical E0 risk register

The probabilities, producer status and mitigation owners below are historical,
not a fresh assessment. Current mitigation ownership follows E12/E13/E14/E2/E9
and the revised roadmap. Continuation-only scenarios are deferred with E4-T4; none of
these archived rows adds a first-release requirement.

| ID | Risk | Likelihood | Impact | Mitigation/owner |
|---|---|---:|---:|---|
| RISK-010 | Dolgorae public gRPC Protobuf or semantics change before Gul implementation. | High | High | Generate inventory/fake server, pin accepted descriptor/executable, enforce handshake and drift gates; E0-T7/E2-T0/E2-T1. |
| RISK-011 | Dolgorae is unavailable, busy, or incompatible. | Medium | High | Stale/disconnected projections, fail-closed mutations, recovery UX; E5. |
| RISK-012 | Controller capability leaks through storage, argv, environment, stdin, logs, events, Workspace paths, or browser contracts. | Medium | Critical | ADR-0047 exact-schema local store, logical keys, pre-invocation path revalidation, external reset plus verified adoption, canary scans; E2-T2/E9-T2. |
| RISK-013 | Supervised RPC server, socket, channel, or logical Run streams leak, collide, flap, or exceed host capacity. | Medium | High | Gul-parent/Dolgorae-socket ownership split, active-gateway refusal, five-per-minute restart rate, eight-stream cap, cancellation and 10-second polling; E2-T1/E4-T1. |
| RISK-015 | Upstream cursor and delivery sequence are conflated. | Medium | High | Distinct types and fields, replay/fault tests; E4/E5. |
| RISK-016 | Write continuation loses lineage, duplicates a destination, rotates credential identity after an ambiguous response, or mutates the source. | Medium | High | Persist key/credential identity, exact terminal Turn, source immutability, reconciliation tests; E4-T4/E5-T3. |
| RISK-017 | Provider state or stale UI is mistaken for Gul authorization and enables an invalid writer action. | Low | Critical | One exhaustive closed evaluator in every button and endpoint; no provider-authoritative allowed-actions list; E4-T3. |
| RISK-018 | Future Gorae scope contaminates Direct Session abstractions. | Medium | High | Typed RuntimeActivity variants and deferred epic; ADR-0030/0032. |
| RISK-019 | Wails v3 changes affect host delivery. | Medium | Medium | ADR-0039 keeps the core and loopback listener free of Wails and confines the shell to E1-T5, so churn no longer gates the graph; pin and smoke-test Wails; E0-T8/E1-T5. |
| RISK-020 | iOS PWA suspension loses event continuity. | High | Medium | Snapshot fallback, separate delivery replay, manual devices; E5/E7/E9. |
| RISK-021 | Verified filesystem root becomes stale or is replaced. | Medium | High | Revalidation, containment proof, fail closed; E3/E6. |
| RISK-022 | `.dolgorae` data leaks through symlink aliases, Markdown/images, or ancestor Git aggregation. | Medium | Critical | Fully resolved denial at every surface, negative tests, hard-link deployment prohibition; E6/E9. |
| RISK-023 | Dolgorae's retained `gomchi` checkout name, an upstream obsolete client name, or a mutable request in `prompt.md` is mistaken for current product identity or SOT. | Medium | High | ADR-0042 records identity history; promote accepted request changes into all five SOT documents; bind dependency evidence to URL/path/commit and check bundle, binary, persistence, docs, and release names; E0-T7/E0-T8/E9-T3. |
| RISK-024 | Dolgorae's implementation programme is unstarted, so Gul's sole provider does not exist as software and E2 through E5 cannot produce live evidence for an extended period. | High | High | Generate inventory and fakes from upstream checked artifacts, pin the commit pair, keep fake and live evidence classes distinct, and sequence work that does not need the executable; E0-T7/E2-T0. |
| RISK-025 | Gul offers Acquire before the first write on a threadless Run or mishandles unsupported transition. | Medium | High | ADR-0044 action matrix, explicit threadless rejection, SubmitTurn(WRITE), CreateWriteContinuation branch; E4-T3/T4. |
| RISK-026 | Wire decoding succeeds despite an unsupported API, missing capability, unknown required enum, or malformed typed error. | High | High | Handshake allowlist, generated-version decoders, required-capability matrix, typed blocker and drift checks; E0-T7/E2-T1/E9-T1. |
| RISK-027 | Protected interaction input supplied by the user leaks through the delivery journal, projection cache, logs, diagnostics, or browser storage. | Medium | Critical | Second-secret rules, one bounded ResolveInteraction body, no carrier/metadata/retry queue, state refresh after response loss, canary tests; E4-T2/E9-T2. |
| RISK-028 | Eight-stream saturation or a slow consumer hides a pending interaction or blocks mutations. | Medium | High | Deterministic priority/hysteresis, 10-second polling guarantee, bounded per-stream queues, unary isolation; E4-T1/E4-T2. |
| RISK-029 | Writer transfer through Release then Acquire loses the writer to a competing acquisition in the unowned window, and users read it as a Gul defect. | Medium | Medium | Present the unowned window explicitly, report the provider result, add no Gul-side queue or reservation; E4-T3. |
| RISK-030 | A Controller capability at rest on disk is read by another process, a backup, or a sync client because it is protected by file ownership and mode rather than the keychain. | Low | Critical | Dolgorae-owned Gul carrier subtree outside every Workspace/FileService path, owner-only modes, pre-invocation revalidation, documented backup guidance and exclusion from sync locations; accepted residual of ADR-0047; E2-T2/E9-T2/E9-T3. |
| RISK-031 | Allowlist-bounded registration browsing discloses host layout or resolves outside its roots through traversal, symlink, or case-alias probes. | Medium | High | Canonical resolution at load, containment proof per browse, one indistinguishable typed error for every outside-root outcome, provider-private denial, negative path fixtures; ADR-0040; E3-T1/E9-T2. |
| RISK-032 | Recommended-tier requirements silently become permanent omissions and v0.1 ships thinner than intended. | Medium | Medium | Tier changes require an accepted ADR and Roadmap change, the release gate demands a recorded disposition for every Recommended item, and each has a named Release-tier fallback; ADR-0041; E9-T3. |
| RISK-033 | Gul unlinks a stale, symlinked, foreign, or live socket, or the socket is remotely exposed. | Low | Critical | Gul never unlinks; Dolgorae owns stale proof/cleanup; Gul validates parent and resulting node, refuses active gateways, and prohibits TCP/Serve exposure; ADR-0043/E2-T1/E9-T2. |
| RISK-034 | A lost mutation response is retried without the exact original semantic request and creates a duplicate or conflicting operation. | Medium | Critical | Disable transparent retries; persist first-release crash-safe replay envelopes only for StartRun; keep SubmitTurn replay process-local; defer continuation replay to E4-T4; stable keys/credentials, semantic reconciliation, and outcome-unknown blocking; ADR-0049/E5-T3. |
| RISK-035 | Missing timeline or Artifact capability produces fabricated history or truncated success. | Medium | High | Required handshake capabilities, authoritative timeline merge, verified bounded artifact reads; E4-T5/E9-T1. |
| RISK-036 | Machine CLI silently becomes a divergent production fallback. | Medium | Critical | Separate adapter/DI graph, explicit diagnostic entrypoints, failure tests proving a gRPC blocker; ADR-0043/E9-T1. |
| RISK-037 | Gul implements against an unpinned revision or weakens a typed provider rule while adapting it. | High | Critical | Gate A/B require one clean regenerated checked set; exact typed field and event-invalidation inventory, fail-closed compatibility tests, and no diagnostic-string parsing; E0-T7/E4-T1/E4-T3. |
| RISK-038 | Workspace registration creates an ID bootstrap cycle or trusts a browser-supplied provider identity. | Medium | High | First InspectWorkspace has no expected ID; later calls use the stored returned ID; browser path/ID remains non-authoritative; E3-T1. |
| RISK-039 | Continuation response loss creates a second destination, loses canonical request material, or relies on a nonexistent lineage lookup. | Medium | Critical | Persist owner-only canonical replay envelope and same-principal credential first, exact-key replay, Controller-ID secondary lookup, delayed ReconcileRun, retention/cleanup tests; ADR-0049/E4-T4/E5-T3. |
| RISK-040 | Gul accepts the typed event variant but omits one of Dolgorae's required aggregate invalidations or combines incompatible projection revisions. | High | Critical | ADR-0048, exact generated event-invalidation map, per-aggregate ProjectionStamp storage, convergence gating, and descriptor/client-policy drift tests; E0-T7/E4-T1/E5-T2. |
| RISK-041 | Gul cannot reproduce an exact allocation request after restart, retains replay material indefinitely, or persists user prompts/secrets to make replay possible. | Medium | Critical | ADR-0049 first-release protected replay store for StartRun only, terminal-result deletion, fixed 72-hour maximum expiry, process-local SubmitTurn replay, no protected-response replay, deferred continuation replay in E4-T4, canary and crash tests; E1-T4/E5-T3. |

## 8. E0-T4 completion record

### E0-T4: Rebaseline the five Gul SOT documents

**Status:** `Completed`
**Started:** 2026-08-17

#### Objective

Replace the obsolete direct-Codex control-plane architecture with the Gul→Dolgorae Runtime Provider boundary while preserving historical traceability and Current State truth.

#### Required outputs

- Revised `required-specs.md`, `architecture.md`, `architecture-decision-records.md`, `roadmap.md`, and `implementation-memo.md`.
- Requirement, ADR, Roadmap, domain, persistence, and API disposition coverage.
- Revision summary and open-decision list inside the SOT set.
- Validation evidence for links, IDs, statuses, terminology, and credential boundaries.

#### Constraints

- Do not create production implementation.
- Do not promote any requirement to Current State.
- Do not modify files outside the five SOT documents for this Task.
- Preserve old IDs and meanings through explicit supersession rather than reuse.
- Treat the concrete RPC/type names in Architecture 6.4 as accepted contract input, but do not add generated code before E0-T7; do not claim live compatibility before E2-T0.

#### Current progress

The 2026-08-19 correction retains supervised local gRPC and the authenticated ConnectRPC browser boundary while aligning the SOT with the reviewed public interface. ADR-0047 replaces the old provider-created credential assumption; ADR-0043/0044 cover Dolgorae-owned socket cleanup and the corrected action set; ADR-0048/0049 now close aggregate convergence and operation-class replay persistence.

Required State now includes a caller-owned exact-schema credential store under the Dolgorae carrier root, bootstrap-safe InspectWorkspace and workspace-scoped `WorkspaceRef`, the complete concrete RPC/type map, provider-authoritative Run configuration, protected crash-safe StartRun/continuation replay envelopes, process-local-only SubmitTurn replay, receipt-based Interaction refresh, typed Controller Interaction mapping with an exact 8 MiB Gul safe-payload cap, one-shot protected input, exact upstream event invalidation plus per-aggregate ProjectionStamp convergence, distinct artifact wire/presentation bounds, protocol-zero handshake, explicit unsupported RPCs, and production `gul serve`. Machine CLI remains explicit diagnostics/conformance only.

Gate A is closed at clean upstream revision `85a8862f784cc57701751d81a9e03bf7c5722818`. All recorded artifact hashes match, Buf 1.66.1 lint and 96 JSON parses pass, and the descriptor reproduces byte-for-byte after pinning its previously implicit Protobuf v32.1 WKT source-info input. Gate B remains open for Gul consumer-map generation and pinning. E2-T0 owns the missing executable/live evidence, and no runtime requirement is promoted to Current State.

#### Exit sequence

1. Rerun the Section 9 and Section 9.1 gate pack against the local-gRPC documents; all earlier evidence predates this change.
2. Extend the gate pack to check ADR-0029/0034 supersession, 36-task DAG/owners, contiguous invariants, semantic/local-store/API agreement, socket ownership, exact event-invalidation parity, ProjectionStamp convergence, replay-envelope privacy/retention, lifecycle/stream/artifact bounds, forbidden production CLI fallback, and the canonical integration matrix.
3. Complete a fresh independent read-only review and move E0-T4 to `In Review` only when all blocking findings are closed.
4. Correct any blocking finding within the five SOT documents.
5. Complete only after review finds no blocking inconsistency.
6. Activate E0-T8 only in the reviewed completion transition; E0-T7 remains planned until E0-T8 completes.

## 9. Verification ledger

Rows dated before 2026-08-23 are historical. E0-T4 lifecycle evidence remains recorded; E0-T8 uses the 2026-08-23 command, fixture, SOT, and independent-review evidence below.

| Check | Command/approach | Result |
|---|---|---|
| Markdown links | Local Markdown-link scan; no unresolved local targets | Passed 2026-08-19 corrective rerun |
| Requirement ID definitions | 182 Required State definitions; 182 unique; 4 Deferred definitions; no Deferred ID also appears in the Required ledger | Passed 2026-08-19 final-alignment rerun |
| Requirement family monotonicity | Every added ID exceeds the highest number ever used in its family; vacated draft IDs remain unused | Passed, post-review rerun |
| Requirement supersession | Former staged baseline contains 107 IDs; all 107 are covered by the migration appendix | Passed 2026-08-17 |
| Requirement ownership | Every Required owner label resolves to one of 36 Roadmap DAG tasks; Deferred owners resolve to `Deferred-Gorae` or `Deferred-Handoff` | Passed 2026-08-19 corrective rerun |
| Requirement ownership coverage | Only E0-T4, the documentation Task, owns no requirement | Passed 2026-08-19 corrective rerun |
| Release tier assignment | Four Recommended-tier IDs are named in Section 5 with a Release-tier fallback each; every other requirement is Release tier by default | Passed, post-review rerun |
| Product invariants | Invariants number contiguously 1 through 46 | Passed 2026-08-19 corrective rerun |
| ADR index/body | 49 index IDs and 49 body IDs/statuses match; only ADR-0018 remains `Proposed`; ADR-0017 is Accepted; ADR-0034 is superseded by ADR-0047 | Passed 2026-08-23 E0-T8 rerun |
| Roadmap active Task | 36 executable Tasks plus retired E0-T9: all three E0 Tasks `Completed`, E2-T0 `Blocked`, and zero Active Tasks | Passed 2026-08-23 completion transition |
| Bootstrap pins | One data-only manifest fixes all approved executable, dependency, Git, macOS, and architecture values; manifest validation rejects missing or malformed authority | Passed `make test` 2026-08-23 |
| Clean-host and drift behavior | Exact clean-host fixture passes; mismatched and missing Wails fail precisely; the current host reports four real readiness differences without installation or substitution | Passed fixture; current-host check intentionally exits 1 with four blockers |
| Standard commands | Serial `make test` invokes checked generation/drift validation, Go fake-server tests, TypeScript checking, SOT checks, and toolchain fixtures without rewriting tracked output | Passed 2026-08-23 E0-T7 candidate |
| Independent E0-T8 review | Full logic/maintainability/documentation/testing review plus logic/documentation lifecycle delta; complete coverage; CI pass; publication committed | Runs `r_01a02b4a-038b-74c8-950d-ab766eda7036` and `r_01a02b52-2423-79f2-9c3a-da129769792c`; zero findings |
| ADR gates in the DAG | Only unresolved ADRs appear as predecessors; ADR-0034 is superseded and accepted ADR-0047 owns credential storage | Passed 2026-08-19 corrective rerun |
| Roadmap status vocabulary and DAG | Every detailed Task has an allowed status and one acyclic predecessor row with known dependencies | Passed 2026-08-19 corrective rerun |
| Concrete RPC/type map | All 36 semantic operation rows resolve to an exact pinned RPC request/response type, local credential-store owner, or Gul projection owner; all 34 public RPCs are inventoried | Passed generated validation 2026-08-23 |
| Gate B source and generation | Dependency lock pins the exact Dolgorae revision and every imported source hash; generated lock covers 16 outputs; fresh generation is byte-identical | Passed `contract/check.sh` 2026-08-23 |
| Generated clients and fake server | Exact Go and TypeScript generators match the E0-T8 authority; a descriptor-derived ConnectRPC fake implements and is called through all seven services and 34 RPCs | Passed Go tests and TypeScript check 2026-08-23 |
| Consumer policy fixtures | Capability, identifiers/enums, error/action, mutation/no-retry, replay, convergence, 20-row invalidation, limits, optional-field, stream-end, unsupported-RPC, and separate Machine CLI valid/invalid cases are checked | Passed contract validator 2026-08-23 |
| Independent E0-T7 review | Full review identified derivation/test gaps; the final correction review confirmed all prior accepted findings closed and found no medium-or-higher defect. Its two low follow-ups (facade wording and streaming/handshake coverage) were corrected and re-tested after the review; the two-review budget is exhausted. | Runs `r_01a02b6d-83e7-72f0-91a0-bfcc9e2a0226` and `r_01a02b79-f303-785a-8600-c67caa4007f3`; diagnostic-only failed delta `r_01a02b79-3a1c-70a4-8176-3bba9893347f` has no publication authority |
| Port and API agreement | Semantic provider port excludes credential creation and unsupported passthrough; local credential-store, ConnectRPC APIs, closed actions/errors, timeline/artifact, and diagnostic-only CLI boundary agree | Passed documentation gate 2026-08-19 |
| Product terminology | Historical product names remain only in ADR-0042 and explicit implementation-memo provenance/risk text; Gul identity fields are present in all five documents | Passed 2026-08-19 final-alignment rerun |
| Product identity provenance | ADR-0042 records accepted identity history and `prompt.md`'s non-authoritative status; the dependency ledger identifies the current `../dolgorae` checkout and exact origin | Passed 2026-08-19 final-alignment rerun |
| Direct App Server ownership | Active text contains only prohibitions/delegation; ownership appears only in superseded history | Passed 2026-08-17 |
| Local writer authority | No active WriteLock aggregate/table/state machine; writer mutations delegate upstream | Passed 2026-08-17 |
| Codex thread fields | Thread/turn field names appear only in prohibition and migration context | Passed 2026-08-17 |
| SQLite authority | Logical schema contains only logical credential keys, Gul-owned state, non-authoritative caches, and non-secret operation attempts; no absolute socket/carrier path is ordinary data | Passed documentation gate 2026-08-19 |
| Credential leakage | APIs/data/invocation/diagnostics prohibit browser, metadata, SQLite plaintext, argv, environment, stdin, log, metric, trace, event, URL, cookie, and Workspace-path exposure | Passed documentation gate 2026-08-19 |
| Corrective contract scan | No active provider credential-creation RPC, Gul socket unlink, protected-input carrier, separate successor action, lineage lookup, free-form state authorization, or CLI fallback remains | Passed 2026-08-19; superseded/rejected/negative-test mentions retained explicitly |
| Positive invariants | LLM-free, caller-owned credentials, Dolgorae-owned socket cleanup, typed-projection blocking, supervised gRPC, no CLI fallback, no Operator, threadless first write, timeline/artifact authority, headless mode, and distinct sequencing are present | Passed 2026-08-19 |
| Integration matrix | 42 canonical cases plus Section 9.6's 32 final-interface cases cover the retained and final-alignment scenarios without claiming execution | Passed documentation coverage 2026-08-19; execution remains future work |
| Upstream checked-artifact reproduction | Accepted revision and nine artifact hashes match; Buf 1.66.1 lint and 96 JSON parses pass; protoc 35.1 plus Protobuf v32.1 WKT reproduces descriptor SHA-256 `22e605dddc26c145ab6c682955fa4bfcf078b8356d38e94982e118b948965318` byte-for-byte | Passed 2026-08-23 Gate A rerun |
| Diff hygiene | `git diff --check -- docs` plus untracked-file whitespace scan | Passed 2026-08-23 rerun |

### 9.1 Boundary gate pack

The current boundary pack must verify unique permanent requirement IDs and valid active and deferred owners; the dynamic acyclic DAG with at most one active Task; ADR index/body equality; concrete RPC/type and provider/local-store/API/error/action agreement; caller-owned credential schema/root/principal rules; Dolgorae-owned socket bind/unlink; bootstrap plus global Profile reads; provider-authoritative configuration; exact StartRun replay; deferred continuation exclusion; protected Interaction recovery and byte limits; typed event invalidation; no browser secret/path fields; no CLI fallback; threadless first write; bounded streams and independent cursors; complete Prompt History; aggregate/result reads and whole-session close; artifact bounds; headless/Tailscale/FileService boundaries; release tiers; and the canonical first-release integration matrix.

### 9.2 Prompt completion criteria

| # | Evidence | Result |
|---:|---|---|
| 1 | All five titles and identity fields use Gul consistently, and historical names appear only inside ADR-0042 as history. Mutable `prompt.md` revisions are promoted into the five SOT documents before they can govern implementation. | Pass |
| 2 | Product definition and invariant 1 are LLM-free. | Pass |
| 3 | ADR-0023 and active architecture prohibit App Server ownership. | Pass |
| 4 | Codex thread fields are prohibited outside migration history. | Pass |
| 5 | DirectSession maps one-to-one to a Dolgorae Run. | Pass |
| 6 | Dolgorae owns writer authority; no local lock aggregate exists. | Pass |
| 7 | Dolgorae owns interaction lifecycle; Gul presents/forwards only. | Pass |
| 8 | SQLite owns only Gul state, one checkpoint store, and non-authoritative cache. | Pass |
| 9 | Dolgorae is the sole v0.1 Direct Runtime Provider. | Pass |
| 10 | Controller capabilities are backend-only with a protected file/FD carrier. | Pass |
| 11 | Direct Interactive defaults, profile selection, Submit, interaction, writer, and lifecycle are represented. | Pass |
| 12 | Shared-readonly continuation creates a threadless dedicated Run and activates writing through the first write Turn without Acquire. | Pass |
| 13 | Gorae is a deferred trusted built-in out-of-process provider. | Pass |
| 14 | ADR-0031 prohibits bypassing Gorae authority. | Pass |
| 15 | Observation checkpoints and client delivery sequences are distinct. | Pass |
| 16 | Guarded read-only FileService remains in scope. | Pass |
| 18 | The 107-row requirement appendix and ADR migration matrix explicitly supersede obsolete artifacts. | Pass |
| 19 | Exactly one task, E0-T4, occupies the Active Task slot. | Pass |
| 20 | All E1-E9 implementation tasks remain `Planned`; no production source exists. | Pass |
| 21 | Current State says implementation and verified runtime behavior are absent. | Pass |
| 22 | Cross-document IDs, owners, decisions, DAG, authority, and security gates agree. | Pass |

These twenty-two criteria come from the originating brief. The local-gRPC revision is additionally governed by the following acceptance map.

### 9.3 Local-gRPC acceptance map

| # | Authoritative answer location |
|---:|---|
| 1 | Architecture 6.1 and ADR-0043 define RPC supervision. |
| 2 | Architecture 6.1/13 and REQ-RUNTIME-016 define socket validation. |
| 3 | Architecture 6.2 and REQ-RUNTIME-003/014 define version/capability negotiation. |
| 4 | Architecture 5/6.3/6.4 identifies unary commands/snapshots, local credential operations, timeline, and artifact-chunk calls. |
| 5 | Architecture 5/6.5 identifies `WatchRunEvents` as server streaming. |
| 6 | ADR-0043 and REQ-PROJ-012 cap live Run streams at eight. |
| 7 | Architecture 6.5 and REQ-PROJ-013 guarantee non-live polling within ten seconds. |
| 8 | Architecture 6.5/10.3 and REQ-PROJ-014/015 define per-Run resume and snapshot repair. |
| 9 | ADR-0019, Architecture 10, and REQ-PROJ-002/016 separate sequencing domains. |
| 10 | Architecture 6.3 and REQ-DIRECT-018 disable transparent mutation retries and define per-operation policies. |
| 11 | Architecture 6.3/15 and REQ-REC-007/009 define outcome-unknown reconciliation. |
| 12 | ADR-0044, Architecture 6.6/8.4, and REQ-DIRECT-016 define threadless first write. |
| 13 | Architecture 8.4 and REQ-WRITER-003/008 restrict Acquire to eligible existing-thread Runs. |
| 14 | Architecture 6.6 and REQ-DIRECT-010/017 define CreateWriteContinuation. |
| 15 | ADR-0047, Architecture 7.5/8.6, and REQ-CTRL-002..013 define credential create/store/verify/adopt. |
| 16 | ADR-0045, Architecture 8.7, and REQ-DIRECT-008 define timeline reconstruction. |
| 17 | Architecture 8.3 and REQ-INTERACT-009..011 define enrichment, nullable expiry, and redaction. |
| 18 | ADR-0045, Architecture 8.8, and REQ-OUT-008..010 define artifact download and verification. |
| 19 | Architecture 9 and REQ-RUNTIME-021/REQ-API-007 define error mapping. |
| 20 | ADR-0046, Architecture 17, and REQ-HOST-008..010/REQ-NET-004 define headless remote operation. |
| 21 | ADR-0043, Architecture 6.1/15, and REQ-RUNTIME-019 define RPC gateway restart behavior. |
| 22 | ADR-0043 and REQ-RUNTIME-020 prohibit silent Machine CLI fallback because it would split mutation, credential, ordering, timeout, error, and history semantics. |
| 23 | Required Specifications invariants 5/6/35 and Architecture 3/11 separate Dolgorae authority from Gul presentation, authentication, delivery, operation coordination, and FileService state. |

### 9.4 Required local-gRPC integration matrix

1. Start the supervised Dolgorae RPC server on a private Unix socket.
2. Reject unsafe, symlinked, foreign-owned, permissive, workspace-controlled, and colliding socket paths.
3. Complete the version and required-capability handshake before enabling mutations.
4. Start a Run through unary gRPC.
5. Reconcile an ambiguous StartRun response without duplicating the Run.
6. Submit a read Turn.
7. Submit the first write Turn to a threadless Run without Acquire.
8. Reject explicit AcquireWriter on a threadless Run when `threadless_acquire_write=false`.
9. Watch multiple Runs over one shared channel.
10. Reconnect one failed Run stream without affecting other streams or unary calls.
11. Resume from the last validated upstream cursor, using snapshot repair when it is ahead of projection commit.
12. Distinguish stream transport loss from Run failure.
13. Handle slow-consumer termination without blocking mutations.
14. Refresh an authoritative snapshot after cursor uncertainty.
15. Create a shared-readonly write continuation.
16. Create a dedicated-reader write continuation when transition is unavailable or unverified.
17. Replay/reconcile continuation with the exact same idempotency key, request identity, destination credential, and source terminal Turn; use destination Controller ID only as secondary confirmation.
18. Detect a wrong Controller.
19. Adopt a reset Controller after host-controlled selection and side-effect-free verification.
20. Receive and resolve every supported Interaction kind, including nullable expiry and the required typed boolean `user_escalation_required`; reject a missing or wrongly typed escalation requirement.
21. Prove protected-input secrecy across storage, logs, metrics, traces, delivery, and browser state.
22. Download and verify a large final-response Artifact by length and digest.
23. Restore prompt and response history from the provider timeline.
24. Reject unsupported protocol/API versions and missing required capabilities.
25. Reject unknown required enum values according to the compatibility policy.
26. Preserve Gul delivery sequencing independently from Dolgorae cursors.
27. Restart the Dolgorae RPC gateway while durable Runs remain available.
28. Restart Gul and reconstruct snapshots, timeline, operations, and subscriptions.
29. Run `gul serve` without Wails and attach Gul.app to the verified existing core.
30. Reconnect iPad/iPhone clients after network loss and host sleep/wake.
31. Prove no socket path, Controller carrier/secret, private worker ID, or App Server detail appears in browser payloads.
32. Prove `.dolgorae/**` remains inaccessible through every FileService, Git, image, Markdown, and watcher surface.
33. Break or remove gRPC and prove production reports a compatibility blocker without invoking Machine CLI fallback.
34. Locally create an exact schema-v1 Controller credential and verify UUIDv7, stable installation/account identities, 32-byte capability, and unpadded base64url.
35. Enforce exclusive `0600` creation beneath `0700` parents, file/parent fsync, logical-key storage, and rejection of symlinked, replaced, foreign, non-regular, or out-of-root carriers.
36. Create a distinct same-principal continuation credential that preserves source kind, subject ID, and stable Gul instance ID independently; reject a changed instance even when subject ID matches, and preserve the exact destination identity across a lost-response replay.
37. Bootstrap InspectWorkspace without an expected workspace ID, then revalidate with the stored returned ID while rejecting browser-authoritative path/ID input.
38. Map the complete typed evaluator input contract and independently reject absent, unknown, or string-only Run lifecycle, thread presence, active Turn, pending Interaction, control mode, lane, writer, policy, profile capability/compatibility, transition, background, assurance, recovery state/required action, lineage, Controller health, ownership, unresolved-operation, and provider compatibility state.
39. Validate the accepted typed-Protobuf event form exhaustively, including projection stamp, identity, binding, monotonicity, and variant-specific invalidation.
40. Prove Dolgorae, not Gul, binds/chmods/unlinks the socket and that an existing gateway yields `RpcServerAlreadyRunning` without attach or takeover.
41. Map `WriteContinuationControllerInvalid`, `RpcServerAlreadyRunning`, and every required action from typed details without parsing human text.
42. Exhaustively vary threadless/existing-thread, active/idle Turn, pending/no Interaction, every control mode, profile compatibility, required recovery action, and verified/missing lineage; acquire writer authority only for an eligible existing-thread dedicated Run and reject unsupported, unverified, conflicting, recovery-required, outcome-unknown, or lineage-uncertain variants through the same closed evaluator used by UI and endpoints.

### 9.5 2026-08-19 corrective acceptance map

| # | Required evidence and current disposition |
|---:|---|
| 1 | Architecture 6.4.1 records the checked E12 first-release operation set with exact RPC or Gul owner, prerequisites, retry, reconciliation, projection, capability, and verification fields. The generated map verifies 31 semantic operations against the accepted descriptor or explicit Gul ownership. |
| 2 | Runtime Provider no longer contains `CreateControllerCredential`; ADR-0047 assigns creation to Gul. |
| 3 | ADR-0047 and REQ-CTRL-002/012 define exact local credential creation under the Dolgorae carrier root. |
| 4 | REQ-CTRL-013 defines distinct same-principal continuation credentials and independently preserves source kind, subject ID, and stable Gul instance ID. |
| 5 | REQ-WS-007 removes the workspace-ID bootstrap cycle. |
| 6 | REQ-RUNTIME-022 and ADR-0044 prohibit free-form state parsing. |
| 7 | REQ-RUNTIME-022/REQ-WRITER-008 retain independent typed evaluator inputs and narrow Gate A to clean publication of the candidate Interaction/configuration/action/limit contract; Gate B performs Gul pinning. |
| 8 | REQ-DIRECT-016 retains `SubmitTurn(WRITE)` as threadless first-write activation. |
| 9 | REQ-DIRECT-017 requires exact same-key/request replay after continuation response loss. |
| 10 | Architecture 6.3 and REQ-DIRECT-017 contain no lineage lookup operation. |
| 11 | ADR-0043 and REQ-RUNTIME-016 assign bind/chmod/stale cleanup/unlink to Dolgorae. |
| 12 | REQ-INTERACT-010 uses one bounded RPC body with no carrier or automatic replay. |
| 13 | ADR-0037 and REQ-PROJ-016 pin typed Protobuf Run events plus exhaustive invalidation rules; checked JSON is not used for v1 events. |
| 14 | REQ-INTERACT-009 pins the candidate typed Controller Interaction oneof and per-kind allowlists. |
| 15 | ADR-0045 and REQ-DIRECT-008 retain provider-backed timeline reconstruction. |
| 16 | ADR-0045 and REQ-OUT-008..010 retain required Artifact access. |
| 17 | REQ-RUNTIME-021/REQ-API-006/007 require typed error/action mapping without diagnostic-text parsing. |
| 18 | The accepted Gate A revision contains every requested semantic addition and its checked artifact set has matching pinned digests and a byte-identical descriptor reproduction recipe. |
| 19 | E0-T7 supplies descriptor-generated clients, an all-RPC fake provider, and checked compatibility/policy fixtures without a production provider or CLI fallback. |
| 20 | REQ-RUNTIME-020 and matrix case 33 prove production refuses incompatible gRPC without CLI fallback. |

### 9.6 Final interface-alignment test map

These cases are normative future acceptance owned by the Roadmap tasks named in the requirements; their presence here is not implementation evidence.

| # | Required case |
|---:|---|
| 1 | Every Gul semantic operation resolves to one exact accepted RPC/type pair or local/Gul owner. |
| 2 | The accepted operation map contains no unresolved RPC placeholder. |
| 3 | Initial `InspectWorkspace` omits expected Workspace ID; revalidation supplies the stored path and ID. |
| 4 | `ListProfiles` and `GetProfile` carry no `WorkspaceRef` and read the user-global Profile registry. |
| 5 | StartRun/GetRun configuration replaces Direct Session presentation state. |
| 6 | Exact same-key StartRun replay returns the original Run and Controller binding. |
| 7 | StartRun recovery never requires `GetRun` before a Run ID is known. |
| 8 | Optional continuation RPC presence does not create a first-release Gul route. |
| 9 | Shared-readonly or unsupported-transition write failure exposes a typed unsupported blocker, not source write resubmission. |
| 10 | A threadless dedicated Run exposes first `SubmitTurn(WRITE)`, not Acquire. |
| 11 | Protected Interaction response bytes are absent from every durable and diagnostic surface. |
| 12 | Lost protected-response results reconcile without automatic secret replay. |
| 13 | Controller Interaction kind/payload mismatch and unknown variants fail closed. |
| 14 | Each typed Interaction variant exposes only its permitted card fields. |
| 15 | Provider and Gul Interaction request/payload bounds combine with `min`. |
| 16 | Provider inline wire bounds and Gul's 256 KiB browser threshold remain distinct. |
| 17 | `RUN_TERMINAL` completes final processing without endless reconnect. |
| 18 | `SERVER_SHUTDOWN` resumes each live Run from its committed upstream cursor. |
| 19 | Partial Writer events cannot enable mutation before the required Run refresh. |
| 20 | Upstream cursor and Gul delivery sequence remain independent across reconnect. |
| 21 | The nine consumer-profile methods unavailable until later tasks have no passthrough route. |
| 22 | Descriptor, registry, or required capability drift blocks production startup. |
| 23 | Production failure never falls back to Machine CLI. |
| 24 | Every Dolgorae durable event variant produces exactly the provider-required Run, Writer, Interaction, or timeline invalidation; Gul-local FileService/profile effects are additive only. |
| 25 | Run, Writer, and Interaction caches retain complete ProjectionStamps and timeline retains captured head; an event invalidation cannot be cleared by an older or incompatible snapshot. |
| 26 | A partial event can immediately disable an action but cannot enable it until every required aggregate stamp converges. |
| 27 | StartRun survives Gul process loss by reusing the same protected canonical replay envelope, key, Controller identity, and carrier. |
| 28 | CloseRun response loss is reconciled through `GetOrchestratedSession` with stable operation correlation and no automatic resubmission. |
| 29 | SubmitTurn exact replay is possible only while the original request remains in memory; after restart no prompt/image is persisted or automatically replayed and unresolved acceptance remains `OutcomeUnknown`. |
| 30 | ResolveInteraction response bytes are never retained, and a lost result is reconciled without secret replay. |
| 31 | Incoming Controller safe payloads enforce `min(provider limit, 8 MiB)` and outgoing protected responses enforce `min(provider limit, 64 KiB)`. |
| 32 | The clean Dolgorae revision reproduces all pinned proto, descriptor, capability, credential, mutation, error/action, client-policy, and conformance digests before Gate B closes. |

## 10. Standard command contract

The root Makefile provides the non-rewriting serial facade `toolchain-check`, `generate-contract`, `contract-check`, `test-prepare`, `test-unit`, `test-int`, `test-e2e`, and `test`. `test` invokes the four test phases serially. E0-T7's checked delegates require exact generator versions, verify imported source hashes, reproduce the descriptor and all tracked outputs, run the all-RPC fake-server tests and TypeScript checker, and validate generated policies plus the separate Machine CLI fixture. Later Tasks extend these stable targets for frontend/browser E2E and opt-in pinned-runtime smoke tests. Real provider tests remain opt-in and must never reveal Controller capabilities, carrier/socket paths, or protected input.

The current host check is honest evidence, not an installation workflow: it
matches Go, Node, Bun, Buf, protoc, protoc-gen-es, Git, macOS and architecture,
and reports missing Wails v3 and two older PATH generators. System Bun 1.4.2
satisfies the `>=1.4.2` minimum and is used directly with the tracked text
`bun.lock` and frozen contract pipeline. The clean-host fixture proves the exact pins and supported ranges; mismatch and
missing-command fixtures prove fail-closed diagnostics. E0-T8 does not install
or silently substitute those tools.

### E0-T8 completion record

E0-T8 completed on 2026-08-23 after serial tests and independent review. It promotes REQ-HOST-005 for the bootstrap pin/reporting surface and accepts ADR-0017. The accepted artifacts are `toolchain/versions.env`, the root Make facade, `TESTING.md`, and the read-only scripts under `scripts/`. This promotion proves the bootstrap contract only: the current host still needs Wails v3 and three generator upgrades, no application dependency manifest exists, and no runtime behavior is claimed.

### E0-T7 completion record

E0-T7 completed on 2026-08-23 with Dolgorae revision `85a8862f784cc57701751d81a9e03bf7c5722818`, dependency-lock SHA-256 `c4f91aa3e2add1093880684e5c96fdbb6239aef6a85261a0adf6b585e2db8863`, generated-lock SHA-256 `8a6a614a3a08c585f9a62f74095a0237d47feefba802e2dfa3ce13be5fbe0bf6`, generated clients, and a fake server covering the historical 34-RPC descriptor. E12-T1 later replaced the live lock and generated paths, so those paths are not evidence bytes for the E0 pin. The historical completion promotes REQ-RUNTIME-011 and REQ-RUNTIME-022 within its original contract/tooling scope only; E2-T0 still owns the compatible executable and live smoke boundary.

## 11. Superseded design note

The initial documentation assumed Gul would manage one Codex App Server, map Session to thread, store Turn/interaction state and a persistent workspace writer lock in SQLite, filter raw App Server events, manage background-process safety, and reconcile App Server state after failure. ADR-0004 through ADR-0008, ADR-0010, and ADR-0016 preserve that history as Superseded. None of those assumptions may guide production work.

## 12. Handoff

E12-T1 is complete and E12 is in completion review. The TASK-053 consumer lock and generated contract tooling are authoritative for new implementation, while historical E0 evidence remains scoped to its original pin. E1-T1 becomes eligible only after E12 closeout; E2-T0 remains blocked on an accepted compatible Dolgorae executable and live smoke evidence. No product runtime or next Task is activated automatically.
