# Gul: Roadmap

| Field | Value |
|---|---|
| Role | Ordered implementation and acceptance plan |
| Product | Gul |
| Version | 0.1-grpc-interface-aligned |
| Last updated | 2026-08-23 |

## 1. Status and execution rules

Allowed executable Task states are `Planned`, `In Progress`, `In Review`, `Completed`, `Blocked`, and `Deferred`. `Retired` preserves a historical Task identity outside the executable DAG and the v0.1 completion count; a retired ID is never reused.

- Exactly zero or one Task may be `In Progress` or `In Review` globally; that Task occupies the single Active Task slot.
- A Task becomes `In Progress` only after all required predecessors and blocking ADRs are resolved.
- `In Review` retains the Active Task slot until evidence and independent review close.
- `Completed` requires implementation or documentation outputs, automated/manual evidence, synchronized SOT, and an implementation-memo record.
- `Blocked` names the external condition and remains in sequence.
- `Deferred` is outside v0.1 unless explicitly reactivated through SOT and ADR updates.
- No production implementation starts until the SOT rebaseline reaches `Completed`.
- A Task may not change the release tier of a requirement it owns; tier changes need an accepted ADR and Roadmap change in the same reviewable change.
- Release qualification requires every Task to be `Completed`, so `Planned` no longer passes the gate.

## 2. Active Task pointer

| Field | Value |
|---|---|
| Active Epic | E0 — Gul Rebaseline and Dependency Contract |
| Active Task | E0-T8 — Resolve pre-implementation technology ADRs and pins |
| Status | `In Progress` |
| Started | 2026-08-23 |
| Exit | Exact toolchain and platform pins, clean-host checks, standard commands, and accepted ADR-0017 are documented without app scaffolding. |
| Next | E0-T7 — Define the public gRPC contract inventory and fake-server fixtures |

Only this pointer and the E0-T8 detail identify active work.

## 3. Epic summary

| Group | Epic | Status | Outcome |
|---:|---|---|---|
| 0 | E0 — Gul Rebaseline and Dependency Contract | `In Progress` | Accepted SOT and pinned Dolgorae dependency boundary. |
| 1 | E1 — Core, Frontend, ConnectRPC, Persistence, and Desktop Shell | `Planned` | Headless core serving remote clients, plus a shell over the same core, with Gul-owned state only. |
| 2 | E2 — Dolgorae Runtime Provider Vertical Slice | `Planned` | Supervise the RPC server and create, submit, observe, render, and interrupt one Direct Run. |
| 3 | E3 — Workspace and Direct Session Presentation | `Planned` | Safe attachment, profiles, and presentation lifecycle. |
| 4 | E4 — Runtime Events, Interactions, Writer, Timeline, and Artifact UX | `Planned` | Two event layers, Controller interactions, closed actions, write continuation, timeline, and artifacts. |
| 5 | E5 — Runtime Reconnect and Recovery Presentation | `Planned` | Snapshot convergence without duplicate mutations. |
| 6 | E6 — File Explorer and Preview | `Planned` | Secure runtime-independent read-only workspace inspection. |
| 7 | E7 — Responsive Desktop, iPad, and iPhone UX | `Planned` | Shared accessible responsive interface. |
| 8 | E8 — Authentication, PWA, and Tailscale | `Planned` | Tailnet-only authenticated remote access. |
| 9 | E9 — Hardening, Packaging, and v0.1 Qualification | `Planned` | Reviewed, packaged, evidence-backed v0.1. |
| — | Deferred-Gorae — Gorae Managed Runtime Provider | `Deferred` | Future trusted managed-runtime integration. |
| — | Deferred-Handoff — Atomic writer handoff | `Deferred` | Unreachable while a Controller is scoped to one Run. |

The Order column groups product slices; executable order is defined solely by the predecessor DAG below.

### 3.1 Predecessor DAG

Every executable task has explicit predecessors. `—` means no predecessor. ADR entries are hard gates; only unresolved ADRs appear. The graph is acyclic and has 36 executable tasks. E0-T9 remains a retired historical identity and is not counted. E1 may build transport-independent foundations before E2-T0, but production E2 provider work and live smoke tests remain blocked until the accepted gRPC executable and Protobuf contract are pinned.

The dependency path is also governed by five named gates:

| Gate | Closure condition | Downstream owner |
|---|---|---|
| A — Clean Contract Publication | Dolgorae publishes the semantically complete inspected public gRPC contract as one clean accepted revision and deterministically regenerates its proto/descriptor, capability, event/client-policy, credential, mutation, error/action, and conformance artifacts with the pinned toolchain. | Closed external prerequisite for E0-T7 |
| B — Gul Contract Pinning | Gul pins the source revision, proto/descriptor, capability, event, credential, mutation, and error/action digests; verifies the complete concrete operation map; and regenerates clients, fake provider, fixtures, and compatibility matrices. | E0-T7/E2-T0 |
| C — Read-intent, no-workspace-write Vertical Slice | Supervision, protocol-zero handshake, Workspace inspection/profile reads, local credential creation, StartRun/GetRun, `SubmitTurn(READ)`, Run events, timeline, and unary artifact reads pass against the pinned fake and compatible runtime. | E2-T1..T3/E4-T1/E4-T5 |
| D — Writer and Continuation | Typed evaluator, valid existing-thread Acquire/Release, both continuation branches, same-principal credential, and exact-request ambiguous replay pass. | E4-T3/E4-T4/E5-T3 |
| E — Interaction and Recovery | Safe summaries/full fetch, one-shot protected input, timeline/artifact, event recovery, gateway/Gul restart, and outcome reconciliation pass. | E4-T1/E4-T2/E4-T5/E5 |

Gate A closed at Dolgorae revision `85a8862f784cc57701751d81a9e03bf7c5722818`. Its primary proto and checked descriptor are pinned by SHA-256, and the descriptor reproduces byte-for-byte with `protoc 35.1` plus the source-info input `google/protobuf/timestamp.proto` from Protobuf `v32.1` commit `7fcfd66022455635fa29af92987cdc0967efd4f3` (SHA-256 `14052c6042c1dd2d0b50245f2812eaab6eaf82db0b6e8ce483eae527f73b6ee8`). Buf 1.66.1 lint and all 96 checked JSON artifacts pass. Gate B remains Gul-owned E0-T7 work. The missing executable is an E2-T0 blocker and no longer blocks E0 completion.

| Task | Required predecessors |
|---|---|
| E0-T4 | — |
| E0-T8 | E0-T4 |
| E0-T7 | E0-T8 |
| E1-T1 | E0-T4, E0-T8 |
| E1-T2 | E1-T1 |
| E1-T3 | E1-T1, E0-T7 |
| E1-T4 | E1-T1, E0-T8, ADR-0017 Accepted |
| E1-T5 | E1-T1, E1-T2, E0-T8 |
| E2-T0 | E0-T7 |
| E2-T1 | E2-T0, E1-T3 |
| E2-T2 | E2-T1, E1-T4 |
| E2-T3 | E2-T1, E2-T2, E3-T1 |
| E3-T1 | E0-T7, E1-T3 |
| E3-T2 | E3-T1, E1-T4 |
| E3-T3 | E3-T1, E2-T1 |
| E3-T4 | E3-T1, E2-T1 |
| E4-T1 | E2-T3, E1-T4 |
| E4-T2 | E4-T1, E2-T2 |
| E4-T3 | E4-T1, E2-T2 |
| E4-T4 | E4-T3, E2-T2 |
| E4-T5 | E4-T1, E4-T2, E2-T3 |
| E5-T1 | E4-T1, E2-T2 |
| E5-T2 | E5-T1, E4-T1, E4-T5, E1-T4 |
| E5-T3 | E5-T2, E2-T3 |
| E6-T1 | E3-T1 |
| E6-T2 | E6-T1, ADR-0018 Accepted |
| E6-T3 | E6-T1 |
| E7-T1 | E1-T2 |
| E7-T2 | E7-T1, E4-T2, E4-T3, E4-T5, E5-T1 |
| E7-T3 | E7-T1 |
| E8-T1 | E1-T4 |
| E8-T2 | E8-T1, E1-T3 |
| E8-T3 | E8-T2, E1-T2 |
| E9-T1 | E2-T3, E4-T4, E4-T5, E5-T3, E6-T3, E8-T3 |
| E9-T2 | E9-T1, E8-T3 |
| E9-T3 | E9-T1, E9-T2, E7-T3 |

## 4. Detailed roadmap

### E0 — Gul Rebaseline and Dependency Contract

**Status:** `In Progress`

#### E0-T4 — Rebaseline the five Gul SOT documents

**Status:** `Completed`

Rewrite Required Specifications, Architecture, ADRs, Roadmap, and Implementation Memo around the LLM-free Gul boundary. Preserve historical IDs through explicit disposition, retain Current State as unimplemented, and run terminology, ID, status, link, and credential-boundary checks.

A product-owner revision replaces the production CLI transport with the new public local gRPC target and adds ADR-0043 through ADR-0049. The 2026-08-19 correction removes the nonexistent credential-creation RPC, moves carrier creation to Gul under the Dolgorae-owned root, fixes InspectWorkspace bootstrap, socket ownership, typed-projection gating, continuation reconciliation, protected input, and event validation, and establishes Gates A–E. The final alignment records the concrete public RPC/type inventory, provider-authoritative Run configuration, the exact upstream event invalidation matrix, per-aggregate ProjectionStamp convergence, crash-safe StartRun/continuation replay material, process-local-only SubmitTurn replay, Interaction limits and receipts, stream-end rules, and explicit v0.1 exclusions without changing the accepted topology.

The earlier decisions remain except where explicitly corrected: ADR-0034 is superseded by ADR-0047; ADR-0035 is a Gul one-credential-per-Run policy rather than a universal provider invariant; ADR-0036 uses `CreateWriteContinuation` and first-write activation; ADR-0037 enforces typed Protobuf Run events and typed Controller Interaction payloads under pinned descriptors; and ADR-0039 includes production headless ownership and desktop attachment.

The Dolgorae brokered-independent-subagent review further narrows the non-goal
to Gul-owned orchestration. REQ-DIRECT-015 permits only a bounded child result in
the parent chat and forbids every Gul child aggregate, credential, navigation,
and mutation surface. The Section 9 gate must cover that boundary before review.

**Acceptance:** all five documents meet the prompt completion criteria; exactly one active Task exists; every former requirement/ADR/Roadmap area has a disposition; no production file is introduced; independent review may move the Task to `In Review` and then `Completed`.

#### E0-T7 — Define the Dolgorae public gRPC contract inventory and fake-server fixtures

**Status:** `Planned`

After Gate A, pin the already identified versioned Protobuf services/RPCs and the final identifiers, enums, typed error/action details, Run configuration, Controller Interaction payload, byte limits, typed event form, exact aggregate-invalidation matrix, client policy, carrier schema, timeline/artifacts, capability set, mutation policies, and Run-stream contract. Generate clients, a fake local gRPC server, fixtures, operation/error/capability/enum/event-invalidation maps, projection-stamp convergence fixtures, replay-policy fixtures, and an independent exact-schema Machine CLI conformance fixture from the accepted descriptor without depending on private worker sockets, state, audit files, or guessed RPC names.

**Acceptance:** Gate B records the clean source revision and all required digests; every semantic operation row resolves byte-for-byte to the accepted descriptor or its local/Gul owner; supported-version, optional-field, capability, projection, exact event-invalidation, error/action, idempotency, replay-material, reconciliation, Interaction-limit, and unsupported-RPC matrices are exhaustive; generated artifacts are reproducible; production DI has no CLI fallback.

#### E0-T8 — Resolve pre-implementation technology ADRs and pins

**Status:** `In Progress`

Pin Go, Wails, Node, package manager, TypeScript, React, Protobuf/Buf, ConnectRPC, SQLite choice, Git bounds, and macOS targets. Accept ADR-0017; ADR-0018 may remain scheduled for E6.

**Acceptance:** clean-host version checks and standard commands are documented. This Task is not gated by Dolgorae executable availability.

#### E0-T9 — Pin an accepted Dolgorae executable contract

**Status:** `Retired`

This identity is preserved but removed from the executable DAG. Its complete responsibility moved to E2-T0 so missing external runtime software cannot prevent the documentation, toolchain, and contract-fixture epic from closing.

The ID MUST NOT be reused. Historical references must identify E2-T0 as the current owner.

Draft-only E0-T5 and E0-T6 are also retired and MUST NOT be reused. E0 execution consists of E0-T4, E0-T8, and E0-T7 in that order.

### E1 — Core, Frontend, ConnectRPC, Persistence, and Desktop Shell

**Status:** `Planned`

#### E1-T1 — Build the transport-independent core and loopback delivery

**Status:** `Planned`

Create production `gul serve` with the Go core ports, lifecycle, singleton lock, authenticated loopback HTTP/ConnectRPC listener, structured configuration, workspace-root allowlist, Dolgorae-owned Gul credential-subtree configuration, provider-supervisor port, FileService, event aggregation, and empty diagnostics, without an LLM or Wails dependency. The core must serve an authenticated client headlessly.

#### E1-T2 — Establish the shared React frontend and delivery

**Status:** `Planned`

Serve one React/TypeScript bundle to Wails and authenticated browser delivery with initial responsive shell and error boundaries.

#### E1-T3 — Define ConnectRPC contracts and typed provider ports

**Status:** `Planned`

Generate Auth, Runtime, WorkspacePresentation, DirectSession, InteractionPresentation, WriterAction, ArtifactPresentation, File, ClientEvent, and Diagnostics clients. Ensure no upstream generated message, socket/carrier path, private worker ID, credential field, or Controller-scoped interaction payload exists. Define the closed action set and exhaustive stable domain errors; an unmapped typed detail fails closed.

#### E1-T4 — Implement Gul-owned SQLite persistence

**Status:** `Planned`

Implement migrations/repositories only for account/session, attachments, Workspace Entries, Direct Session presentation, trusted credential references, navigation, delivery journal, FileService state, observation checkpoints, per-aggregate projection stamps/invalidation metadata, non-authoritative projection/timeline caches, and non-secret provider-operation attempts with logical replay references. Model role-tagged replay Controller references so StartRun uses a destination credential-store key rather than a nonexistent binding, while continuation keeps separate source binding and destination credential references. Implement the separate protected replay store for bounded StartRun/continuation canonical material with immediate terminal-result deletion, startup and at-least-six-hourly purge, and a fixed 72-hour v0.1 maximum retention configurable only downward. Socket, carrier, and replay-file paths are not ordinary SQLite data.

#### E1-T5 — Build the Wails desktop shell

**Status:** `Planned`

Create `Gul.app` as a shell that acquires the shared lock and starts the core in-process when absent, or verifies and attaches to an existing `gul serve`. Add WebView delivery and host-controlled directory/credential pickers without a second runtime path.

**Epic acceptance:** the core serves an authenticated remote client with no shell process; the shell then reuses the same frontend bundle and API; schema inspection finds no authoritative Run, Turn, writer, pending interaction, or recovery table.

### E2 — Dolgorae Runtime Provider Vertical Slice

**Status:** `Planned`

#### E2-T0 — Qualify an accepted Dolgorae executable contract

**Status:** `Blocked`

**Design Gate impact:** `Not required` — this Task validates an already-approved contract and executable compatibility without creating a product, UX, or architecture design decision.

Resolve an accepted executable/release against the Gate B descriptor, API/capability versions, `serve` lifecycle and Dolgorae-owned socket cleanup, compatible profiles, binary identity policy, concrete RPCs, stream/cursor behavior, typed projections/errors/actions, timeline/artifact contracts, and caller-owned protected Controller carriers.

**External blocker:** Dolgorae TASK-020 has not produced an accepted compatible executable/release. This blocks E2 provider implementation and runtime smoke tests but does not block E0 or transport-independent E1 work.

**Acceptance:** the dependency ledger names a reproducible binary, release, Protobuf/schema, lifecycle probe, CLI comparison, and live gRPC smoke evidence; no private or operator-gated interface is required.

#### E2-T1 — Implement RPC supervision, compatibility, and shared channel

**Status:** `Planned`

Implement shell-free binary discovery; Gul-owned private parent/unused-path validation; Dolgorae-owned socket bind/chmod/stale cleanup; active-gateway refusal without takeover; supervised server startup/readiness/shutdown/restart; one reusable gRPC channel; negotiated API/capability checks; generated DTO mapping; strict enum and typed-error policy; per-profile transition state; cancellation; safe diagnostics; and read coalescing. Enforce 15/5/10-second lifecycle budgets and the accepted restart-rate policy. Register no production CLI fallback.

#### E2-T2 — Implement protected Controller bindings

**Status:** `Planned`

Implement `DolgoraeControllerCredentialStore` locally with exact schema-v1 generation, trusted installation/account IDs, UUIDv7 and 32-byte capability creation, exclusive `0600` files below `0700` Dolgorae-owned carrier directories, file/parent fsync, logical SQLite keys, pre-RPC validation, cleanup of unused carriers, and same-principal successor creation. A successor preserves source kind, subject ID, and stable Gul instance ID independently while receiving a new Controller ID/capability at generation 1; matching subject with a changed instance is rejected. Supply carriers for provider verification; cover exact destination identity across retries, symlink/owner/mode/type/containment refusal, no metadata leakage, and host-controlled side-effect-free adoption as a Gul application workflow after external reset.

#### E2-T3 — Complete one Direct Interactive vertical slice

**Status:** `Planned`

Inspect an initialized workspace using first-call path bootstrap and `WorkspaceRef` revalidation/profile reads, create a local credential carrier, persist a protected StartRun replay envelope and StartRun with exact-replay recovery, GetRun, submit read and threadless first-write Turns through unary gRPC, observe a safe Run stream, render the final response, and interrupt an active Turn. Refresh presentation from the provider Run configuration rather than the original request. Persist operation identity before each mutation; keep SubmitTurn request material in memory only and reconcile post-restart ambiguity without prompt replay.

Also render a fake parent-Turn result produced by a profile tool's trusted
brokered child while proving that no child Direct Session, Runtime Activity,
Controller Binding, lifecycle record, or mutation API is created.

**Epic acceptance:** the fake gRPC server and pinned compatible Dolgorae pass; one shared channel handles unary calls and multiple logical streams; Gul never accesses App Server, private Dolgorae interfaces, or the CLI as fallback.

### E3 — Workspace and Direct Session Presentation

**Status:** `Planned`

#### E3-T1 — Implement host selection and provider Workspace attachment

**Status:** `Planned`

Use either the host picker or bounded server-side browsing inside the configured workspace-root allowlist, then local checks and provider InspectWorkspace with the host-controlled absolute path and no expected ID. Store the returned canonical root and Runtime Workspace ID/digest, revalidate with both stored values, and derive every later workspace-scoped `WorkspaceRef` from that attachment. Gul never invokes init, provisions a profile, or trusts a browser-supplied absolute path/provider ID.

#### E3-T2 — Implement Workspace Entry presentation

**Status:** `Planned`

Add aliases, favorites, recent order, selected provider/profile, navigation, and safe presentation removal without runtime deletion.

#### E3-T3 — Implement Direct Session presentation lifecycle

**Status:** `Planned`

Create/list/get/rename/archive Direct Sessions backed by Run IDs; consume provider-authoritative Run configuration on StartRun/GetRun/restarts; distinguish provider pause/resume/close/delete receipts from local presentation actions; and expose runtime mutations only when the provider advertises the typed capability and Gul's closed evaluator permits them.

#### E3-T4 — Implement profile, model, effort, and lane selection

**Status:** `Planned`

Display provider-advertised compatibility, model, lane, assurance, version, and capability summaries. Warn before permanent shared-readonly creation.

**Epic acceptance:** no Codex thread field or Gul-owned Turn lifecycle exists; presentation actions do not silently mutate Runs.

### E4 — Runtime Events, Interactions, Writer, Timeline, and Artifact UX

**Status:** `Planned`

#### E4-T1 — Build the upstream-to-client event bridge

**Status:** `Planned`

Keep validated and projection-committed upstream cursors solely in observation checkpoints and a distinct Gul delivery sequence. Implement the validation→binding→mapping→enrichment→projection commit→delivery pipeline with bounded queues. Persist each Run, Writer, and Interaction `ProjectionStamp`, timeline `captured_head_cursor`, and invalidation floor as non-authoritative convergence metadata.

Maintain at most eight Run streams over one channel using the accepted priority and 30-second demotion hysteresis. Poll every other Run within 10 seconds. Classify `RUN_TERMINAL`, `SERVER_SHUTDOWN`, typed gRPC `SLOW_CONSUMER`, and other transport failure separately; reconnect only when appropriate. Reproduce the Dolgorae per-variant invalidation matrix exactly, add only local presentation invalidations, and prevent any partial event from enabling a mutation before all required aggregate stamps converge.

#### E4-T2 — Implement Controller interaction presentation

**Status:** `Planned`

List expanded safe summaries, treat stream events as notifications only, fetch full fields through the valid backend Controller binding, merge by ID, and map the typed `ControllerInteraction.payload` oneof exhaustively into per-kind allowlisted Interaction Cards. Enforce `min(provider maximum_safe_payload_bytes, 8 MiB)` for incoming safe payloads and `min(provider maximum_response_bytes, 64 KiB)` for outgoing protected responses before mapping/forwarding. Treat ResolveInteraction success as a receipt/status, persist only its key and non-secret metadata, keep protected bytes out of every store, exclude protected responses from automatic retry, and refresh Interaction/Run state after a lost response.

Also handle protected interaction input as user-supplied secret material excluded from every Gul store and surface, and poll activities outside the live subscription window for pending interactions through capability-free observer reads.

#### E4-T3 — Implement writer state and provider actions

**Status:** `Planned`

After Gate B pins the clean continuation required-action mapping and projection contract, render the typed writer, policy, assurance, lane, background/recovery, lineage, compatibility, owner, and blocker inputs. Implement the exhaustive closed evaluator in every UI/action endpoint without parsing strings: shared/unsupported-transition writes map to `CanCreateWriteContinuation`, threadless first write to `SubmitTurn(WRITE)`, eligible existing-thread acquisition to `CanAcquireWriter`, and outcome/recovery states block conflicts. Matrix tests vary every independent input and verify partial events cannot enable actions.

#### E4-T4 — Implement lineage continuation and shared-to-dedicated successor

**Status:** `Planned`

Use CreateWriteContinuation for a shared-readonly source and for a dedicated reader whose transition is unavailable or unverified. Persist the key, source Run/terminal Turn, destination Controller ID/logical credential key, reason, normalized request digest, and an owner-only canonical replay envelope before the first call. Create a new same-principal local credential with source kind, subject ID, and stable Gul instance ID preserved independently, preserve source state and immutable lineage, and make the destination threadless/dedicated. Reject a changed instance even when subject ID matches. A lost response replays the exact same request; ListRuns plus destination Controller ID is secondary confirmation, and ReconcileRun is unavailable until the destination Run ID is known.

#### E4-T5 — Implement provider timeline and Artifact presentation

**Status:** `Planned`

Reconstruct Direct Session history from GetRun plus Controller-safe timeline items after the stored checkpoint, validate Run/Turn identity and safe item kinds, merge provider chronology into a non-authoritative cache, and publish one coalesced browser snapshot. Implement authorization-sensitive Artifact metadata and unary chunks bounded by provider and Gul limits, a 64 MiB Gul total cap, a distinct 256 KiB browser-inline threshold, length/SHA-256 verification, safe Markdown, and inert opaque references. A larger provider-inline response is converted for presentation rather than rejected as a wire violation.

**Epic acceptance:** no local writer aggregate or authoritative interaction table exists; all races resolve according to provider results.

### E5 — Runtime Reconnect and Recovery Presentation

**Status:** `Planned`

#### E5-T1 — Handle unavailable, incompatible, and ambiguous provider state

**Status:** `Planned`

Show typed blockers for transport, protocol, Controller, writer, Run conflict, artifact, path encoding, recovery, outcome-unknown, and operator action. Mark projections stale, fail closed for writes until verified adoption/reconciliation, never invent Turn completion or writer release, and stop restart/retry on protocol incompatibility.

Also implement the unresolved-outcome state for unknown turn outcomes, writer authority blocked as unknown, required recovery, and unverified background execution: no success, failure, or release is rendered until an authoritative snapshot resolves it, and dependent mutations stay blocked.

#### E5-T2 — Implement provider and browser convergence

**Status:** `Planned`

Re-handshake compatibility; mark all provider aggregates stale; fetch Runs including recovery/configuration, Writer status, pending/Controller Interactions, and timeline with complete stamps; converge every required aggregate and timeline head; resume each Run stream from its safe cursor boundary; replay Gul events or return a snapshot; and handle RPC gateway restart, Gul restart, sleep/wake, and mobile suspension.

#### E5-T3 — Prove no duplicate mutation across failure boundaries

**Status:** `Planned`

Persist non-secret operation attempts before transmission and protected replay envelopes only for StartRun and CreateWriteContinuation. Persist only role-tagged logical Controller references, proving that StartRun resolves its destination credential without a pre-existing binding and that continuation resolves distinct source and destination carriers. Fault-inject gateway exit, Gul process exit, lost response, deadlines, stale/uncertain cursors, browser retry, replay-material loss, startup/periodic purge, 72-hour expiry, and version drift. Exercise exact allocation replay, same-process-only SubmitTurn replay, no protected-response replay, stable idempotency/credential identities, blocked conflicting mutations, and final projection from provider evidence rather than blind gRPC replay.

**Epic acceptance:** recovery contains no App Server connection or local runtime-state reconciliation.

### E6 — File Explorer and Preview

**Status:** `Planned`

#### E6-T1 — Implement verified-root FileService guards

**Status:** `Planned`

Resolve Workspace Entry, validate relative path, prevent traversal/symlink escape, prove containment, deny the fully resolved `.dolgorae/**` subtree before every surface, suppress its Git aggregate contribution, and enforce file/type bounds.

#### E6-T2 — Implement directory, text, image, and SVG-safe preview

**Status:** `Planned`

Provide lazy browsing, bounded source/text and raster preview. Resolve ADR-0018 before active SVG rendering; source-only is fallback.

#### E6-T3 — Implement refresh and bounded Git review

**Status:** `Planned`

Provide the ADR-0038 bounded host filesystem watcher with explicit refresh fallback, fixed `HEAD`/Working status/comparison, and safe non-Git degradation without file mutation. The watcher is bounded in watched nodes, queue depth, and coalescing interval, discards `.dolgorae/**` events at the resolved real path, and requires no upstream event projection.

**Epic acceptance:** FileService works while Dolgorae is unavailable and never creates an LLM Turn.

### E7 — Responsive Desktop, iPad, and iPhone UX

**Status:** `Planned`

#### E7-T1 — Build desktop panes and mobile navigation

**Status:** `Planned`

Implement three-pane desktop/wide-tablet layout and Sessions/Chat/Files small-screen navigation with safe state retention.

#### E7-T2 — Integrate runtime projection and interaction priority

**Status:** `Planned`

Display provider connectivity, lifecycle, observation mode, writer/policy/assurance, final responses, and interaction cards without implying local runtime authority. Present operator-action-required blockers naming the documented external procedure, and render unresolved outcomes distinctly from ordinary errors with no in-product retry.

#### E7-T3 — Complete Korean IME and accessibility

**Status:** `Planned`

Test composition, newline/submit, touch/keyboard navigation, semantic controls, focus, contrast, and reduced motion.

### E8 — Authentication, PWA, and Tailscale

**Status:** `Planned`

#### E8-T1 — Implement first-run account and password storage

**Status:** `Planned`

Create exactly one local account with approved hashing and recovery documentation.

#### E8-T2 — Implement browser sessions and request protection

**Status:** `Planned`

Add secure cookies, logout/revocation, rate limiting, Origin/CSRF enforcement, and authenticated streaming.

#### E8-T3 — Package headless service, PWA, and Tailscale Serve deployment

**Status:** `Planned`

Package `gul serve` and a user `launchd` agent; enforce one core per user/data directory; let Gul.app attach to a verified existing core; define owned process/log locations, actionable port collision, graceful upgrade/restart, sleep/wake recovery, and browser reconnection. Deliver tailnet-only HTTPS to the authenticated loopback listener and verify Funnel and Dolgorae-socket exposure are absent.

### E9 — Hardening, Packaging, and v0.1 Qualification

**Status:** `Planned`

#### E9-T1 — Run deterministic, contract, concurrency, and fault suites

**Status:** `Planned`

Run deterministic domain/action/error, generated gRPC contract, exact Machine CLI conformance, credential-store, migration/operation-attempt, socket/path-ownership, mutation-fault, event-schema, stream, timeline/artifact, headless, and browser suites. Cover the canonical matrix with every non-conflicting prior case and every numbered 2026-08-19 corrective scenario, including local credential ownership, bootstrap inspection, typed-projection blocking, exact continuation replay, one-shot protected input, durable gateway restart, secrecy, `.dolgorae` denial, and no production CLI fallback.

#### E9-T2 — Complete security and diagnostic review

**Status:** `Planned`

Threat-model browser, loopback, supervised RPC process/channel/socket, Machine CLI diagnostic boundary, filesystem, credential store, artifacts, protected input, and logs. Verify client allowlists, no metadata/path leakage, and bounded diagnostics.

#### E9-T3 — Package and qualify v0.1

**Status:** `Planned`

Produce reproducible `Gul.app`, `gul serve`, and user `launchd` configuration; operator docs covering RPC/CLI roles and credential-root backup/restore; clean-host dry run; Wails/browser/iPad/iPhone sleep/network evidence; and promote requirements only after review. Qualification requires every Release-tier requirement promoted, every Recommended-tier requirement promoted or explicitly recorded as accepted-incomplete, and every Task `Completed`.

### Deferred-Gorae — Gorae Managed Runtime Provider

**Status:** `Deferred`

Potential future tasks cover Gorae compatibility, Project/Mission/Task tree, high-level approvals, Findings/Artifacts, managed writer-release requests, and Gorae events. The provider is trusted and out-of-process; no plugin marketplace is introduced and Gul never controls Gorae-owned Dolgorae Runs directly.

### Deferred-Handoff — Atomic Writer Handoff

**Status:** `Deferred`

Owns deferred REQ-WRITER-004. Gul policy binds one distinct Controller to each Direct Session/Run pair, so no two Gul-owned Runs share a Controller and the handoff precondition is unreachable. Eligible existing-thread transfer uses Release followed by a separately evaluated Acquire; threadless first write never uses Acquire. Reactivation requires an accepted Controller-scope change. `Deferred-Handoff` is a deferred owner label, not an executable Task.

## 5. Former Roadmap disposition

`Replaced` below is historical disposition, not an allowed active Task status.

| Former area | Disposition | New owner |
|---|---|---|
| E0-T1..T3 governance/toolchain/App Server feasibility | Replaced | E0-T4, E0-T8, E0-T7, and E2-T0 split SOT rebaseline, toolchain pins, contract inventory, and executable proof. |
| E0-T5..T6 and E0-T9 | Retired before execution | E0-T7/E0-T8 retain contract/toolchain work; E2-T0 owns executable proof; old IDs are non-reusable. |
| E1-T1..T4 host/API/SQLite/frontend | Modify | E1-T1..T4 with Gul-only persistence and provider ports. |
| E2-T1..T5 App Server vertical slice | Replaced | E2-T1..T3 Dolgorae Provider vertical slice. |
| E3-T1..T4 workspace/session-thread | Modify/Replace | E3-T1..T4 Workspace Entry and Direct Session presentation. |
| E4-T1..T5 persistent WriteLock | Replaced | E4-T3 writer projection/actions; Dolgorae owns authority. |
| E5-T1..T4 raw App Server interactions | Replaced | E4-T2 Controller interaction presentation. |
| E6-T1..T4 raw output/Turn controls | Modify/Replace | E4 event allowlist plus E7 projection UI; runtime control upstream. |
| E7-T1..T7 Git-aware artifacts | Modify | E6 FileService with provider-verified root. |
| E8-T1..T3 responsive UX | Modify | E7-T1..T3. |
| E9-T1..T4 auth/PWA/Tailscale | Modify | E8-T1..T3 and E9 security review. |
| E10-T1 client replay | Modify | E4-T1 and E5-T2 with two cursors. |
| E10-T2 App Server reconciliation | Replaced | E5 provider snapshot recovery. |
| E10-T3..T5 sleep/diagnostics/hardening | Modify | E5 and E9. |
| E11-T1..T3 qualification | Modify | E9-T1..T3. |

## 6. Maintenance

Any task change updates the Active Task pointer, epic summary, detailed status, requirement ownership, ADR deadlines, and implementation memo in one reviewable change. Production work that crosses the Provider, credential, filesystem, or remote-auth boundary requires an accepted ADR and explicit evidence before completion.
