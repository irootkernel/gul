# Gul: Required Specifications

| Field | Value |
|---|---|
| Document role | Normative product and system requirements |
| Authority | Product requirements source of truth |
| Product | Gul (굴) |
| Subtitle | Remote operator interface for local development runtimes |
| Version | 0.1-dolgorae-consumer-v1 |
| Status | Approved Required State; E1-T1 through E1-T5 foundations completed without assembled product acceptance |
| Last updated | 2026-09-29 |
| Target | macOS host; modern desktop, iPad, and iPhone browsers |

## 1. Purpose, authority, and lifecycle

Gul separates **Required State**, approved target behavior without accepted implementation evidence, from **Current State**, behavior implemented, reviewed, and accepted with evidence. Deferred requirements are recorded separately and are not part of v0.1 release acceptance.

The source-of-truth order is:

1. `required-specs.md`
2. `architecture-decision-records.md`
3. `architecture.md`
4. `roadmap.md`
5. `implementation-memo.md`

A requirement ID is permanent and MUST NOT be reused for another meaning. IDs inherited from the former baseline retain their historical family and meaning; any superseded ID disappears from the active ledger. New IDs in an existing family MUST be greater than the highest number ever used in that family. `RUNTIME`, `DIRECT`, `CTRL`, `INTERACT`, `WRITER`, `PROJ`, and `GORAE` are rebaseline families with no earlier product meaning.

A Required State requirement moves to Current State only after its owning task reaches `Completed`, every acceptance criterion passes, Current Architecture and ADRs agree, and evidence is recorded in the implementation memo. Partial work remains Required State. At most one Roadmap task may be `In Progress` or `In Review` globally; zero is valid.

## 2. Product definition and identity

Gul is a single-user, LLM-free remote operator interface that runs on a macOS host and provides secure browser access to trusted local software-development runtimes over a Tailscale tailnet.

For v0.1, the sole execution provider is the **Dolgorae Runtime Provider** implementing `dolgorae.gul-consumer/v1` in released Dolgorae v0.1.3. A Gul Orchestrated Session has one Primary Run and provider-owned optional Specialist Runs. The retained internal term Direct Session denotes its one-to-one Primary Run binding, not the whole hierarchy. Explicit Orchestration Launch Intent creates the aggregate; control_mode alone does not. Dolgorae owns Codex App Server, execution, Broker membership, policy, writer, recovery, accepted history, and whole-session closure.

Dolgorae `docs/specs/gul-consumer-v1.md` is the shared consumer authority. E12-T1 pins its TASK-053 immutable checked revision before implementation; Gul never edits a second copy independently. That pin is accepted contract/tooling Current State, not product runtime or live-provider evidence. Pre-release Tasks use explicit contract-derived fakes. E2/E9 alone qualify the real adapter against the released artifact. Actual Gul does not gate the provider release.

Gul owns remote UI, authentication, Runtime Provider integration, presentation metadata, browser event delivery, and a runtime-independent read-only FileService. Gul does not contain an LLM and never starts, connects to, supervises, or interprets Codex App Server directly.

| Item | Value |
|---|---|
| Product | Gul |
| Application | `Gul.app` |
| Executable/helper | `gul` |
| Repository identity | `gul` |
| Bundle identifier | `xyz.rootkernel.gul` |
| Application data | `~/Library/Application Support/Gul/` |
| Logs | `~/Library/Logs/Gul/` |
| Cache | `~/Library/Caches/Gul/` |

This identity and its two historical predecessors are recorded in ADR-0042. Historical names carry no authority and do not appear in active normative text. `prompt.md` is user-owned, excluded from version control, and an incoming revision request rather than a contract authority; accepted changes are promoted into these five documents, which then govern implementation.

## 3. Definitions

| Term | Definition |
|---|---|
| Runtime Provider | A trusted, built-in Gul adapter to an out-of-process local runtime, never an arbitrary library, JavaScript extension, or marketplace plugin. |
| Runtime Attachment | Gul configuration identifying provider kind, accepted executable, expected RPC/API versions, binary identity, health, and capability snapshot. The supervised socket is ephemeral runtime state, not attachment data. |
| Workspace Entry | Gul presentation metadata linked to a provider-owned Runtime Workspace ID/digest and a verified canonical root. |
| Direct Session | Retained internal Primary Run binding for a Gul Orchestrated Session; not authority over its Specialists. |
| Orchestrated Session | Provider aggregate created by explicit launch intent; one Primary plus optional Broker-owned Specialists. |
| Prompt History | User-only view of accepted Primary timeline input, preserving original text and provider order. |
| Runtime Activity | Closed provider-defined navigation projection, such as a Direct Session or a future Gorae Mission. |
| Controller Binding | Backend-only association between a Direct Session and its Dolgorae Controller capability. |
| Provider reference | Opaque provider-scoped Run, Turn, interaction, handoff, or artifact identifier. Possession conveys no authority. |
| Runtime Projection | Non-authoritative cached/UI representation of provider-owned state. |
| Observation checkpoint | Gul reconnect metadata containing validated and projection-committed upstream cursors, keyed by provider, runtime object, and stream kind. |
| Client Delivery Sequence | Gul-owned monotonic sequence used to replay browser-facing events. |
| Interaction Summary | Redacted observer-safe pending-interaction description. |
| Controller Interaction | Full normalized interaction fields available only through the bound Controller backend. |
| Interaction Card | Gul-owned browser-facing contract for one interaction kind, populated from a Controller Interaction under a per-kind allowlist. |
| Protected interaction input | A user-supplied secret answer to an interaction the provider marks protected. |
| Access policy transition | Provider-reported support state for changing an existing runtime thread's effective access policy in place. |
| Threadless Run | A Run with no accepted first Turn and therefore no writer authority; when `threadless_acquire_write=false`, its first write is activated only by `SubmitTurn(write_intent=WRITE)`. |
| Live subscription window | The deterministic bounded set of at most eight Runs holding logical `WatchRunEvents` streams over the shared gRPC channel. |
| Runtime operation attempt | Gul-owned, non-authoritative record of a provider mutation identity, pending state, timeout, replay availability, and reconciliation status. It contains no credential or protected input. |
| Projection stamp | Dolgorae-provided complete revision tuple containing captured head cursor plus Run, Writer, and Interaction revisions for compatibility and invalidation checks. |
| Projection convergence state | Gul-owned cache metadata recording per-aggregate stamps, timeline captured head, freshness, and invalidated aggregates; it never grants authority. |
| Provider replay envelope | Owner-only bounded canonical request material retained for crash-safe `StartRun` exact replay in the first release. Continuation replay is deferred with E4-T4. It contains no capability bytes, protected input, SubmitTurn prompt, or image bytes. |
| Runtime timeline | Dolgorae-owned Controller-safe chronology used as the authoritative source for conversation reconstruction. |
| Provider action class | Closed Gul classification of a provider outcome as retryable, operator-action-required, user-choice-required, ambiguous-outcome, or blocked. |
| Presentation action | Gul-local rename, favorite, archive/hide, or navigation operation. |
| Runtime action | Provider-delegated Run, Turn, interaction, writer, or recovery operation. |

## 4. Product invariants

1. Gul does not contain or invoke an LLM.
2. Gul does not start or connect directly to Codex App Server.
3. Every Direct Session is backed by exactly one Dolgorae Run.
4. Gul never treats a Codex-owned thread or Turn ID as runtime authority.
5. Dolgorae is authoritative for Run, thread, Turn, interaction, writer, policy, assurance, and recovery state.
6. Gul SQLite is authoritative only for Gul-owned state.
7. A cached projection never overrides a provider snapshot.
8. Controller capability bytes never enter a browser, prompt, workspace, log, event, diagnostics, URL, cookie, ordinary SQLite field, process argv, environment, or stdin.
9. Gul displays `WRITE` only when upstream writer authority is active and effective write policy is verified; a write operation additionally requires explicit write intent.
10. A `shared_readonly` Direct Session cannot be promoted in place.
11. The first release never promotes `shared_readonly` in place or offers write continuation. A later accepted continuation must use a distinct Controller and a threadless dedicated successor.
12. Gul does not directly take writer authority from another Controller.
13. Gorae-managed state is mutated only through a future Gorae Provider.
14. Gul never bypasses Gorae to mutate Gorae-owned Dolgorae Runs.
15. Reasoning and hidden chain-of-thought are never displayed or persisted by Gul.
16. Browser clients never receive unrestricted local paths or runtime capabilities.
17. Browser reconnect never duplicates a runtime mutation.
18. File browsing is independent of LLM execution.
19. Provider failure never causes Gul to invent success or release writer authority.
20. v0.1 supports only explicitly trusted, built-in Runtime Provider adapters.
21. FileService never exposes the reserved provider-private `.dolgorae/**` subtree through any browsing, preview, Git, Markdown-asset, or Submit-image surface.
22. Controller credential storage is outside every Workspace and every path FileService can resolve.
23. Gul never creates, reads, stores, presents, or uses a Dolgorae Operator capability and never invokes an operator-gated operation.
24. Runtime workspace addressing uses the verified canonical path; the returned Runtime Workspace ID/digest verifies identity and never replaces path verification.
25. Gul's security policy binds one distinct Controller credential to each Direct Session/Run pair and never reuses it across Direct Sessions; this is not asserted as a universal Dolgorae invariant.
26. Gul never offers an in-place read/write access change unless the provider reports access-policy transition as supported.
27. An unmapped provider error code or an unknown value in a decision-critical closed enum fails closed as a typed blocker.
28. Protected interaction input is never journaled, cached, logged, returned to a browser, or stored outside the single provider hand-off.
29. A required user action is never withheld because a Run falls outside the live subscription window.
30. Workspace registration never resolves or reveals a path outside the host-configured workspace-root allowlist.
31. Production runtime operations use only the negotiated Dolgorae local gRPC API over a Gul-managed user-private Unix socket; Machine CLI failure or gRPC incompatibility never triggers a silent production fallback.
32. Browser clients never receive a Dolgorae socket path, Controller carrier path, private worker identifier, raw upstream event, or App Server transport detail.
33. A successful event-stream connection grants no mutation authority, and a slow event consumer cannot block unary mutations.
34. A threadless Run never acquires writer authority through a separate Acquire action when `threadless_acquire_write=false`; its first write is one explicit write Turn.
35. Dolgorae timeline is authoritative for runtime conversation history and the mandatory user-only Prompt History section; Gul caches are presentation-only.
36. Artifact references are opaque provider references, not local filesystem paths, unless an accepted contract explicitly states otherwise.
37. Gul never performs lossy conversion of a non-UTF8 provider path or exposes its raw bytes to a browser or log.
38. Gul creates its own Controller credential carriers locally; Dolgorae verifies and binds them but does not create them for Gul.
39. Dolgorae owns RPC gateway socket bind, mode, stale cleanup, and unlink; Gul never unlinks the socket node.
40. Gul never derives decision-critical provider state by parsing human-readable or weakly typed strings.
41. Protected Interaction input is sent only in one bounded `ResolveInteraction` request body and is never materialized as a carrier file.
42. An upstream event invalidation is a mandatory minimum; Gul may add local invalidation but never omit a provider-required aggregate refresh.
43. A partial event may disable a mutation immediately but never enables one until every required Run, Writer, Interaction, and timeline input is fresh and stamp-compatible.
44. Crash-safe exact replay is limited to operations whose canonical non-secret request material was retained before transmission; SubmitTurn prompts and protected Interaction input are never durably retained for replay.
45. Replayable allocation operations retain only role-tagged logical Controller references and expected public Controller IDs; they never persist an absolute carrier path or capability, and StartRun does not pretend that a destination binding already exists.
46. An unresolved replay envelope has a fixed 72-hour v0.1 maximum retention configurable only downward; expiry removes replay material and preserves the non-secret attempt as `OutcomeUnknown` until authoritative reconciliation or explicit operator handling.
47. No fresh ordinary prompt is queued, auto-sent, auto-interrupted, or injected into an active Primary Turn; drafts wait for explicit send after eligibility. Current Interaction answers remain available.
48. Session Close targets the whole owned aggregate through the root and Broker. Gul never loops over child mutations or claims final closure while effects remain unknown.
49. Podway observations are future and strictly read-only. No Gul API/backend/UI edits FSM, jumps/skips/forces/reexecutes nodes, or resets counts; changes require ordinary prompts judged by the executing LLM.

## 5. Required State ledger

Every requirement below carries a release tier.

- **Release tier** is the default. A Release-tier requirement must reach Current State with evidence before v0.1 qualifies.
- **Recommended tier** is a named exception. A Recommended-tier requirement is still Required State and is still implemented when its owning task runs, but v0.1 may qualify with it explicitly recorded as accepted-incomplete rather than promoted.

A requirement is Release tier unless it appears in the Recommended list below. Moving any requirement into or out of that list needs an accepted ADR and Roadmap change in the same reviewable change; a task may never silently demote its own requirement. Deferred State, in Section 6, is separate from both tiers and does not participate in v0.1 acceptance at all.

The initial Recommended list is limited to workspace-review presentation extras whose absence removes convenience without weakening safety, authority, containment, or the core operate-a-Run loop, and each already has a Release-tier degradation path:

| ID | Recommended because | Release-tier fallback |
|---|---|---|
| REQ-FILE-011 | Per-file Git status is review convenience, not a safety property. | REQ-FILE-014 requires browsing to stay usable without Git. |
| REQ-FILE-012 | Rendered Markdown is presentation over the same guarded bytes. | REQ-FILE-005 source rendering. |
| REQ-FILE-013 | Fixed-revision comparison is review convenience. | REQ-FILE-014 typed degradation. |
| REQ-FILE-016 | The watcher is an enhancement over explicit invalidation. | REQ-FILE-009 is satisfiable by explicit refresh alone. |

Every other requirement, including all authority, credential, path-containment, interaction, writer, recovery, authentication, network, and qualification requirements, is Release tier.

### 5.0 Consumer release profile and phase rules

The first-release profile requires 27 methods from the producer contract,
including complete ListRunTimelineItems and two read-only OrchestrationService
queries. Do not use the known descriptor inventory as the runtime supported set.
The complete descriptor's optional later methods are not prerequisites.

REQ-DIRECT-010, REQ-DIRECT-017 and REQ-CTRL-013 are defined only in the Deferred
State ledger, owned by E4-T4. Shared safety requirements remain in the active
ledger and describe the first release without requiring continuation work. Do not offer a successor action when its RPC is absent;
show a typed unsupported transition and preserve the source. REQ-DIRECT-014
still prohibits unsafe in-place write conversion. No blanket safety waiver is
created. DeleteRun is optional and not a first-release acceptance action;
presentation hiding and provider deletion remain distinct.

Owner IDs identify responsibility, while the roadmap's phase determines whether
completion evidence is mock-scoped or live. Pre-release completion cannot claim
real credentials, gateway, or provider acceptance; E2/E9 qualify those boundaries.
Historical E0 evidence is retained only for its old pin. New contract pinning,
provider scenario fakes, and assembled pre-release readiness belong to E12-T1,
E13-T1, and E14-T1, respectively. Former E12-T2/T3 are Retired, not incomplete
members of E12. The roadmap orders complete Epics; no pre-release Epic requires
a later Epic's implementation to satisfy its own acceptance.

Foundation Tasks prove their named ports, schemas, repositories, bundle and shell
against isolated test dependencies. They do not promote a requirement whose
acceptance needs the assembled application. The complete requirements retain
these explicit acceptance owners:

E1-T1/T2 are task evidence, not partial promotion of E14-owned REQ-HOST-001/002.

| Required outcome | Foundation or feature contribution | Complete requirement owner |
| --- | --- | --- |
| Authenticated runtime-independent core and shared bundle | E1-T1/T2 foundation and E8 authentication | REQ-HOST-001/002: E14-T1 |
| Authenticated Wails singleton/verified attach | E1-T5 shell and E8 packaging | REQ-HOST-007: E8-T3 |
| Production headless runtime assembly | E1 core, E6 files, E8 authentication/packaging, E2 adapter | REQ-HOST-008: E2-T3 |
| Complete declared application services | E1-T3 contracts and each feature's service implementation | REQ-API-001: E14-T1 |
| No duplicate state-changing operation on browser retry | E1-T3 concurrency contract and E5 attempt/replay implementation | REQ-API-002: E5-T3 |
| Installed PWA navigation across refresh | E7-T1 navigation and E8 PWA packaging | REQ-UI-007: E8-T3 |

Until E8 supplies real account/session protection, production product access MUST
remain denied. Test principals and fake dependencies are explicit isolated test
injections, never production bypasses. E13-T1 proves provider scenario behavior
without requiring Gul feature implementations; E14-T1 proves those completed
features together. This splits evidence ownership, not product safety or release
scope. E2/E9 still own actual-provider proof.

### 5.1 Host, authentication, and network

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-HOST-001 | Gul MUST provide a Go core, loopback delivery, and persistence that run without Wails and without an LLM runtime. | The assembled core, listener, and persistence serve an authenticated client with no Wails process or real provider present; E1 supplies the foundation and E8 supplies authentication. | E14-T1 |
| REQ-HOST-002 | One frontend bundle MUST serve both authenticated remote clients and the desktop shell. | The bundle is served over the authenticated listener and the shell reuses it with no second bundle or API surface. | E14-T1 |
| REQ-HOST-007 | Gul MUST ship a macOS Wails v3 desktop shell that starts the shared core in-process when absent or attaches to the verified existing `gul serve`, adding no domain authority or second runtime path. | Authenticated desktop and singleton/verified-attach tests pass against the same API; E1-T5 supplies the shell foundation. Capability inventory shows only WebView delivery and host-native affordances. | E8-T3 |
| REQ-HOST-008 | Gul MUST provide production `gul serve` headless mode that starts the shared core, authenticated HTTPS/ConnectRPC service, Dolgorae supervision, event aggregation, and FileService without a Wails window. | Headless browser E2E passes with the released provider and no Wails process; E1's foundation or E14's fake evidence alone cannot promote this requirement. | E2-T3 |
| REQ-HOST-009 | Exactly one Gul core per user and data directory MUST own the runtime lock. Gul.app MUST attach to a verified healthy existing `gul serve`; a second headless invocation MUST exit without starting another core or Dolgorae server. | Desktop/headless contention and spoofed-owner tests pass. | E8-T3 |
| REQ-HOST-010 | Headless packaging MUST include user `launchd` operation, graceful upgrade/restart, host sleep/wake recovery, owned-process and protected-log locations, and actionable port-collision behavior. | Login, upgrade, sleep/wake, collision, and log-permission drills pass. | E8-T3 |
| REQ-HOST-003 | Gul APIs MUST use Protobuf-defined ConnectRPC services. | Generated Go and TypeScript clients compile without handwritten feature REST APIs. | E1-T3 |
| REQ-HOST-005 | Gul MUST pin Go, project dependencies, and active generators exactly; enforce minimum versions without upper bounds for other host executables; and report target Runtime Provider compatibility. | `toolchain/versions.env` is the single bootstrap authority; `make toolchain-check` enforces the exact Go version and minimum versions for other host executables while project manifests pin dependencies and active generators exactly. Missing and incompatible host fixtures, project-pin validation, and generated-output drift checks pass without installing or rewriting tools. | E0-T8 |
| REQ-HOST-006 | Runtime diagnostics MUST fail closed or display a blocker for unsupported or unverifiable dependencies. | Missing, changed, and incompatible dependency fixtures never enable runtime mutations. | E9-T2 |
| REQ-AUTH-001 | Gul MUST support exactly one local password-authenticated account. | No username, team, or second-account creation surface exists. | E8-T1 |
| REQ-AUTH-002 | Password material MUST use an approved salted password hash and never be persisted or logged in plaintext. | Security tests and source review pass. | E8-T1 |
| REQ-AUTH-003 | Login MUST create a revocable server-side session represented by a `Secure`, `HttpOnly`, `SameSite=Strict` cookie. | Cookie, expiry, logout, and revocation tests pass. | E8-T2 |
| REQ-AUTH-004 | Login and mutations MUST resist brute force, CSRF, and cross-origin abuse. | Rate-limit, Origin, CSRF, and negative tests pass. | E8-T2 |
| REQ-NET-001 | Remote access MUST remain tailnet-only through Tailscale Serve; Funnel is prohibited. | Deployment inspection finds no public exposure. | E8-T3 |
| REQ-NET-002 | HTTP and ConnectRPC listeners MUST bind to loopback by default. | Tests reject accidental non-loopback binding. | E8-T3 |
| REQ-NET-003 | Remote browser access MUST use HTTPS. | PWA and ConnectRPC operate without mixed content through Tailscale Serve. | E8-T3 |
| REQ-NET-004 | Startup and diagnostics MUST verify that Tailscale Serve targets the authenticated loopback listener and that Funnel is disabled; the Dolgorae Unix socket MUST never be a Serve target. | Misrouted Serve and Funnel fixtures block remote-ready status. | E8-T3 |

### 5.2 Runtime Provider and attachment

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-RUNTIME-001 | v0.1 MUST support the released Dolgorae Orchestrated Session consumer profile as its sole execution provider. | No direct App Server path or production fake/CLI fallback exists; actual qualification follows release pinning. | E2-T1 |
| REQ-RUNTIME-002 | Providers MUST be trusted, built-in, typed, and out-of-process. | No dynamic library, arbitrary JavaScript extension, or generic `Execute(action,json)` port exists. | E1-T3 |
| REQ-RUNTIME-003 | Gul MUST discover the configured Dolgorae executable and validate binary identity, version, public RPC API version, and required capabilities before enabling runtime mutations. | Compatible, incompatible, missing, replaced, and changed-binary fixtures pass. | E2-T1 |
| REQ-RUNTIME-004 | Gul MUST expose provider health, compatibility, capabilities, profiles, lanes, assurance, models, and relevant native-subagent summaries without reinterpreting Codex. | UI values match provider projections. | E2-T1 |
| REQ-RUNTIME-005 | Production runtime operations MUST use only Dolgorae's accepted public local gRPC API over a Gul-managed Unix domain socket. | No private worker socket, state file, audit file, operator-gated stream, App Server socket, TCP endpoint, REST endpoint, or production CLI invocation is accessed. | E2-T1 |
| REQ-RUNTIME-006 | Gul MUST supervise one Dolgorae public RPC server without a shell, use one reusable gRPC channel, and apply explicit startup, readiness, shutdown, request, stream, memory, and restart bounds. | Lifecycle, timeout, cancellation, restart-rate, socket-collision, and process-ownership tests pass. | E2-T1 |
| REQ-RUNTIME-007 | Every mutation MUST follow its provider operation's concurrency contract. | Idempotency keys are used only where accepted; generation fencing is used where required; tokenless operations are never blindly retried after an ambiguous result. | E2-T3 |
| REQ-RUNTIME-008 | Protocol operations and wire shapes MUST be enabled only after an accepted executable/release contract is verified. | The dependency ledger identifies release, binary, schema, and probe evidence. | E2-T0 |
| REQ-RUNTIME-009 | Gul MUST display disconnected, incompatible, busy, and degraded provider states without fabricating runtime state. | Failure fixtures preserve the last projection as explicitly stale. | E5-T1 |
| REQ-RUNTIME-011 | A versioned gRPC service, operation, identifier, enum, typed-error detail, projection, carrier, timeline, artifact, and capability inventory plus fake-server fixtures MUST exist before production provider work. It MUST map every semantic port operation to an exact RPC request/response type, a local credential-store operation, or a Gul-only projection operation; record Controller/workspace prerequisites, idempotency, timeout class, retry, lost-response reconciliation, projection update/invalidation, and capability requirement; and retain the exact Machine CLI schema separately. | Every operation has exactly one accepted owner/mapping, all RPC rows resolve to the pinned descriptor, supported-version and required-capability matrices are complete, mutation policies are exhaustive, and generated drift checks fail on upstream change. | E0-T7 |
| REQ-RUNTIME-012 | Recover and Reconcile MAY be exposed only through a typed RecoveryCapability advertised by the provider. | Unsupported operations are absent; supported actions use provider authorization and results. | E5-T1 |
| REQ-RUNTIME-013 | Gul MUST read and surface the provider's access-policy-transition support state per selected Runtime Profile. | Supported, unsupported, and unverified fixtures produce distinct projections; no state is inferred from a successful read turn. | E2-T1 |
| REQ-RUNTIME-014 | The gRPC adapter MUST call `GetCapabilities` with `RequestContext.protocol_version=0` and Gul's minimum/maximum range, then use the negotiated version in every later request. It MUST validate versions, descriptor and credential digests, supported methods, required features, event projection, Interaction payload contract, and artifact/Interaction bounds; use generated decoders; reject unknown required enum values; and preserve unknown optional data only when the accepted policy permits it. Successful wire decoding alone MUST NOT imply semantic compatibility. | Protocol-zero, accepted/rejected range, digest drift, missing method/feature/bound, unknown-required-enum, allowed-optional-data, and disallowed-optional-data fixtures produce the specified result. | E2-T1 |
| REQ-RUNTIME-015 | Provider reads MUST be coalesced and rate-bounded through single-flight per operation key plus per-provider concurrency and refresh budgets; mutation RPCs MUST remain independent from event-stream consumer speed. | Startup, reconnect, foreground-resume, multi-client, and slow-consumer bursts remain within budgets and do not delay unrelated unary mutations. | E2-T1 |
| REQ-RUNTIME-016 | The supervised socket pathname MUST be absolute under `~/Library/Caches/Gul/runtime/`, beneath owner-only non-symlink directories outside every Workspace, and MUST never be selected by a browser or remote caller. Gul owns parent creation/validation and unused-path selection; Dolgorae owns gateway singleton state, bind/chmod, stale proof/unlink, and graceful cleanup. Gul MUST never unlink the socket. | Wrong-owner, permissive-mode, symlink, workspace-contained, stale, live-collision, TCP-fallback, browser-supplied path, and attempted Gul-unlink fixtures fail closed; an active gateway without an accepted attach contract yields `RpcServerAlreadyRunning`. | E2-T1 |
| REQ-RUNTIME-017 | Startup MUST complete within 15 seconds, including a 5-second readiness handshake; graceful shutdown MUST complete within 10 seconds and drain unary operations for at most 5 seconds. | Boundary-time fixtures prove mutation remains disabled before readiness and only the owned child can be terminated after grace expiry. | E2-T1 |
| REQ-RUNTIME-018 | Crash restart MUST use jittered exponential backoff of 1, 2, 4, 8, and 16 seconds, capped at 30 seconds and five starts per rolling 60 seconds; five stable minutes reset the budget. Protocol incompatibility and budget exhaustion MUST stop automatic restart. | Clock-controlled restart fixtures verify rate, jitter bounds, reset, and terminal blockers. | E5-T1 |
| REQ-RUNTIME-019 | A public RPC server restart MUST NOT mark a durable Dolgorae Run failed. Gul MUST re-handshake, refresh snapshots and timeline, and resume Run streams; mutations in flight at disconnect become unresolved until reconciled. | Gateway-restart fixtures preserve Run identity and recover projections without duplicate mutations. | E5-T2 |
| REQ-RUNTIME-020 | Production dependency injection MUST register only the gRPC provider. The Machine CLI adapter MAY be used only by explicit diagnostics, operator procedures, development fixtures, emergency inspection, and CLI-versus-gRPC conformance tests. | Removing or breaking gRPC produces a compatibility blocker and never invokes a CLI mutation or event follower. | E9-T1 |
| REQ-RUNTIME-021 | gRPC status messages MUST NOT be parsed. gRPC status codes plus accepted typed Dolgorae error details MUST map exhaustively into Gul domain errors, and missing or malformed details MUST fail closed. | Mapping coverage includes transport, deadline, protocol, Controller, writer, threadless, interaction, recovery, outcome, slow-consumer, artifact, path, Run-conflict, and operator-action cases. | E1-T3 |
| REQ-RUNTIME-022 | The accepted Dolgorae contract's typed Run, Turn, writer, policy, assurance, recovery, lineage, profile, configuration, Interaction, and event projections MUST remain independent decision inputs and MUST NOT be inferred from another aggregate or free-form field. E0-T7 owns only the historical typed-state contract and fixture invariant at its original pin. New producer-contract publication, repinning and regenerated evidence are owned exclusively by REQ-CONSUMER-001/E12-T1; this historical requirement does not certify the new contract. | Contract fixtures reject each missing, unknown, stale, stamp-incompatible, or string-only decisive field independently and prove that `effective_access`, `writer_state`, `recovery_status`, `writer_policy_confirmation`, `compatibility`, and `action` strings are never parsed for semantics. | E0-T7 |
| REQ-RUNTIME-023 | Gul v0.1 MUST NOT call or expose `ListProfileDiagnostics`, `SetDefaultEffort`, `ForkRun`, `VerifyRun`, or the Prepare/Commit/Cancel WriterHandoff RPCs. Their presence in the public descriptor MUST NOT block compatibility; future use requires a separate ADR and typed provider-port extension. | API and provider inventories contain no generic passthrough and no route to any intentionally unsupported RPC. | E1-T3 |

### 5.3 Workspace Entries

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-WS-001 | Multiple local Workspace Entries MUST be registrable through either a host-controlled directory picker or bounded server-side browsing restricted to the configured workspace-root allowlist. | Both paths register the same verified canonical root; a browser cannot submit an arbitrary absolute path through either one, and registration is possible from a remote client without host access. | E3-T1 |
| REQ-WS-003 | Every Workspace Entry MUST have a user-visible display name independent of its directory basename. | Rename affects presentation only. | E3-T2 |
| REQ-WS-004 | Multiple provider workspaces and Direct Sessions in different workspaces MUST be usable concurrently. | Concurrent fixtures do not create Gul-owned writer arbitration. | E3-T3 |
| REQ-WS-006 | Runtime calls and FileService MUST use the Workspace Entry's verified canonical root; the provider ID/digest verifies it. | Missing, moved, or mismatched roots fail closed and require reattachment. | E3-T1 |
| REQ-WS-007 | Registration MUST inspect an already initialized Dolgorae workspace by passing the host-controlled absolute path with no expected Runtime Workspace ID, then store the returned verified canonical root and Runtime Workspace ID/digest. Every later revalidation MUST send the stored canonical path and expected ID, and every other workspace-scoped RPC MUST use a `WorkspaceRef` from that verified attachment. | Gul never invokes `init`; bootstrap omits the ID; subsequent workspace reads include `WorkspaceRef`, while user-global ListProfiles/GetProfile requests do not; revalidation and identity-mismatch fixtures pass; neither browser path nor browser workspace ID is authoritative. | E3-T1 |
| REQ-WS-008 | Removing a Workspace Entry presentation MUST NOT close, delete, or otherwise mutate runtime activities. | UI removal and provider deletion remain distinct confirmed operations. | E3-T3 |
| REQ-WS-009 | v0.1 MUST accept pre-initialized workspaces with a compatible user-global Profile and preprovisioned named Specialist Policy. | Missing workspace/profile/policy or unavailable Profile Server produces typed provider blockers; Gul does not provision or repair them. | E3-T1 |
| REQ-WS-010 | Canonicalization MUST collapse symlink, `..`, case-alias, and alternate-spelling paths into one verified workspace identity where the host filesystem does so. | Duplicate and move/mismatch tests fail closed or resolve to the same identity. | E3-T1 |
| REQ-WS-011 | Gul MUST NOT create an automatic Git worktree. | Inspection and tests show no worktree-creation operation. | E3-T1 |
| REQ-WS-012 | The workspace-root allowlist MUST be host-configured, canonically resolved at load, and MUST bound every registration browse; no listing, existence answer, or error MUST reveal or resolve a path outside it, and the reserved provider-private subtree MUST stay denied. | Absolute, traversal, symlink-escape, case-variant private-subtree, and outside-root probes fail closed with an indistinguishable typed error; an empty or unresolvable allowlist disables server-side browsing without disabling the host picker. In-root case aliases resolve under REQ-WS-010. | E3-T1 |

### 5.4 Direct Sessions and provider references

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-DIRECT-001 | A workspace MUST support multiple Direct Sessions, each mapped one-to-one to a Dolgorae Run ID. | Three independent Runs can be listed and reopened without a Codex thread field. | E3-T3 |
| REQ-DIRECT-002 | Orchestrated Session creation MUST use a parentless `direct_interactive` Primary root with a protected `interactive_client` carrier and explicit `orchestration_launch(use_case=dolgorae_orchestrated_session,specialist_policy_name)`. A low-level `direct_interactive` Run without accepted launch intent is not a Gul session. | Outbound semantic request and accepted projection match every required field; missing or invalid launch intent fails without creating or labeling a session. | E2-T3 |
| REQ-DIRECT-003 | Permanent `shared_readonly` creation MUST require an explicit user choice and warning. | UI and API tests prevent accidental selection. | E3-T4 |
| REQ-DIRECT-004 | Submit MUST include Direct Session ID, prompt, an explicit closed `write_intent` enum of `READ` or `WRITE`, optional validated image references, optional effort override, and the persisted idempotency identity required by the accepted operation policy. | Read/write routing and duplicate handling match the provider contract; unknown intent values fail before provider invocation. | E2-T3 |
| REQ-DIRECT-005 | Submit and Interrupt MUST delegate acceptance and lifecycle authority to Dolgorae. | Gul never advances a Turn state independently. | E2-T3 |
| REQ-DIRECT-006 | The shared action evaluator MUST distinguish Primary Pause/Resume/Interrupt, aggregate-aware root Recover/Reconcile and whole-session Close. Every action requires advertised capability and fresh typed eligibility; active owned work requires explicit interrupt confirmation. DeleteRun and CreateWriteContinuation are absent from the first-release action set. | E4-T3 classifies all actions and blockers without executing unsafe placeholders. Close coordination belongs to REQ-SESSION-002/E5-T1; browser close/hide is local, and Primary Pause/Interrupt is never aggregate pause. | E4-T3 |
| REQ-DIRECT-007 | Rename, favorite, hide/archive, and navigation changes MUST remain Gul-local presentation actions. | Provider state is unchanged. | E3-T2 |
| REQ-DIRECT-008 | Runtime conversation history MUST be reconstructed from the Controller-safe Dolgorae timeline and current Run snapshot and MAY be cached only as a non-authoritative presentation projection. | Open, reconnect, provider restart, and Gul restart merge timeline items by validated identity and chronology without duplication. | E4-T5 |
| REQ-DIRECT-009 | Runtime Profile selection MUST display compatibility, models, lanes, maximum assurance, capability summary, and runtime version. | Selection is blocked when the requested configuration is unsupported. | E3-T4 |
| REQ-DIRECT-011 | Each image reference MUST contain Workspace Entry ID, normalized relative path, and a closed detail token. | Absolute, traversal, symlink-escape, oversized, unsupported, and `.dolgorae/**` references fail before provider invocation. | E2-T3 |
| REQ-DIRECT-012 | `runtime_run_id`, `runtime_turn_id`, `interaction_request_id`, `handoff_id`, and `artifact_id` MUST be typed opaque provider references, never Codex-owned authority fields. | Required operation identifiers exist; serialization proves that possessing an ID grants no authorization. | E2-T3 |
| REQ-DIRECT-013 | A write-intent submission rejected for upstream writer conflict MUST produce a distinct typed `writer_busy` presentation. | UI names safe owner/blocker information and never reports write acceptance. | E4-T3 |
| REQ-DIRECT-014 | Where access-policy transition is unavailable or unverified, a Direct Session's access character MUST be fixed after its first Turn. The first release MUST surface a typed unsupported-transition blocker and preserve the source without offering continuation. | Unsupported and unverified fixtures offer neither source promotion nor continuation; supported existing-thread fixtures retain per-submission intent. Future continuation belongs to deferred E4-T4. | E4-T3 |
| REQ-DIRECT-015 | Gul MAY show provider-owned Specialists as observer-only projections beneath a session using public Run parent references and authorized aggregate reads. It MUST NOT create child Controllers, mutate children, or reconstruct authoritative membership from parent links. | E3-T3 displays observable member status without child controls and uses GetOrchestratedSession for aggregate facts. Result discovery/rendering is separately owned by REQ-SESSION-004/E4-T5 and is not an E3-T3 acceptance prerequisite. | E3-T3 |
| REQ-DIRECT-016 | A threadless dedicated Run with `threadless_acquire_write=false` MUST activate its first writer only through `SubmitTurn(write_intent=WRITE)`; Gul MUST NOT expose or invoke `AcquireWriter` first. | First-write acceptance confirms effective policy and writer state; an explicit threadless Acquire attempt is rejected before provider invocation. | E4-T3 |
| REQ-DIRECT-018 | Every provider mutation MUST have a persisted local policy defining deadline, cancellation, retry eligibility, ambiguous-response handling, reconciliation, browser pending state, and final projection source. | The operation-policy inventory is exhaustive and transparent gRPC mutation retries are disabled. | E5-T3 |
| REQ-DIRECT-019 | `RunConfigurationProjection` MUST be authoritative for profile, purpose/label, model, current default effort, required capabilities, parent provenance, and instructions identity. Gul MUST refresh Direct Session presentation from it after StartRun, GetRun/snapshot refresh, Gul restart, and Dolgorae restart; the original request and local cache MUST NOT override it. | Reconnect and restart fixtures replace stale local values with the provider configuration and never require the original request as authority. | E3-T3 |
| REQ-DIRECT-020 | Before `StartRun`, Gul MUST persist its idempotency key, Controller ID, credential-store reference, Workspace ID, normalized semantic request identity, and attempt state. A lost response MUST first replay the exact request with the same key, Controller, and carrier; Controller-matched `ListRuns` is secondary, and `GetRun`/`ReconcileRun` apply only after a Run ID is known. | Fault injection returns the original Run and never mints a new key or credential or requires an unknown Run ID. | E5-T3 |

### 5.5 Controller credentials and interactions

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-CTRL-001 | Every Direct Session MUST reference exactly one backend-only Controller Binding to its Run; its capability material MAY be in a typed unhealthy state. | Browser contracts contain health and references, never capability bytes. | E2-T2 |
| REQ-CTRL-002 | Gul-owned Controller capabilities MUST be stored as create-exclusive owner-only files below the capability-advertised `~/.dolgorae/controller-carriers/gul/<installation-id>/`, outside ordinary plaintext SQLite and every Workspace/FileService path. Parent directories MUST be `0700`, files `0600`, and every relevant component non-symlinked and current-user-owned. | Exclusive-create, wrong-mode/owner/type, symlink, containment, fsync, and replacement-race fixtures fail closed; loss produces a typed blocker and reset remains provider-external. | E2-T2 |
| REQ-CTRL-003 | Controller capability bytes MUST reach Dolgorae only through the accepted protected capability file carrier in an authorized RPC request. Credentials MUST NOT appear in gRPC metadata; the carrier path is backend-only and revalidated immediately before every authorized call. | Bytes and carrier paths never appear in browser payloads, metadata, argv, environment, or stdin; owner, mode, type, symlink, and containment checks precede each call. | E2-T2 |
| REQ-CTRL-004 | Logs, errors, diagnostics, events, prompts, workspaces, cookies, URLs, and browser storage MUST exclude capability bytes and digests. | Redaction and canary-secret tests pass. | E9-T2 |
| REQ-CTRL-005 | Before StartRun, Gul MUST locally create and validate a new credential carrier through `DolgoraeControllerCredentialStore`, then supply its derived carrier reference for Dolgorae verification and binding. `CreateControllerCredential` MUST NOT exist in the Runtime Provider port. | Provider inventory contains only `VerifyController`; local-store and provider fakes prove that Dolgorae neither creates nor implicitly invents a Gul credential and the source credential cannot authorize the destination. | E2-T2 |
| REQ-CTRL-006 | Controller credential storage MUST be outside every Workspace root and every path FileService can resolve. | Startup refuses an unsafe store location and path-replacement tests fail closed. | E2-T2 |
| REQ-CTRL-007 | Gul MUST never create, read, store, present, or use Dolgorae Operator capability material or invoke an operator-gated operation. | API, process, storage, and diagnostics inventories contain no Operator carrier or operation. | E9-T2 |
| REQ-CTRL-008 | A missing or invalid Controller capability MUST be a typed blocker that persists until an externally provisioned capability is adopted under REQ-CTRL-009. | No in-product reset is claimed; affected mutations fail closed before verified adoption. | E5-T1 |
| REQ-CTRL-009 | Gul MUST support adopting an externally provisioned Controller capability for an existing Direct Session only through host-controlled selection, local path/owner/mode/type validation, side-effect-free `VerifyController`, and atomic binding replacement followed by a fresh Run including recovery/configuration, Writer status, pending/Controller Interactions, and timeline as needed. | Adoption succeeds for a matching Run and fails closed for arbitrary browser paths, mismatched Runs, malformed carriers, symlinks, and ambiguous verification without an Operator capability. | E2-T2 |
| REQ-CTRL-010 | Gul security policy MUST bind one distinct Controller capability to each Direct Session/Run pair and MUST NOT reuse it across Direct Sessions. | Storage and StartRun fixtures prove distinct bindings; continuation-specific checks remain deferred with E4-T4. The documentation does not claim this as a universal Dolgorae invariant. | E2-T2 |
| REQ-CTRL-011 | Ordinary SQLite MUST store only a logical relative credential key, protected binding reference, public metadata, and health, never credential bytes, an absolute/unvalidated carrier path, or the supervised RPC socket path. Absolute carrier paths MUST be derived under the trusted root and fully revalidated immediately before every authorized RPC. | Schema and canary inspection pass; traversal, alternate-root, symlink, owner, type, and mode replacement fixtures fail closed. | E1-T4 |
| REQ-CTRL-012 | The local credential store MUST emit schema version 1 with a UUIDv7 `controller_id`, `kind=interactive_client`, stable trusted-local Gul installation ID, stable trusted-local account `subject_id`, and 32 `crypto/rand` bytes encoded as unpadded base64url; it MUST use exclusive create with no overwrite, fsync the file and parent, and clear capability buffers where practical. | Exact-schema and negative fixtures verify UUID version, 32-byte decoding, encoding, trusted ID provenance, durability ordering, collision refusal, and secret absence from logs and SQLite. | E2-T2 |
| REQ-INTERACT-001 | Direct Run Interaction Summaries MUST be delivered through Gul client events as pending-state notifications; the actionable payload is the Interaction Card defined by REQ-INTERACT-009. | Reconnect restores the current summary and card without duplicating resolution. | E4-T2 |
| REQ-INTERACT-002 | The backend MAY fetch a Controller Interaction only through the Direct Session's valid Controller Binding. | Observer, lost-binding, and wrong-controller attempts fail closed regardless of known IDs. | E4-T2 |
| REQ-INTERACT-003 | The browser MUST receive only fields required to answer or approve an interaction. | Contract allowlist excludes secrets, unrestricted paths, and raw runtime payloads. | E4-T2 |
| REQ-INTERACT-004 | Resolve MUST forward a normalized response using the provider's concurrency contract; its immediate response is a resolution status/receipt, not a complete refreshed Interaction. Gul MUST refresh the summary, authorized detail, or Run snapshot when required, and Dolgorae determines stale, resolved, or invalid state. | Concurrent clients converge on the provider receipt and subsequent authoritative reads without treating the receipt as a full Interaction. | E4-T2 |
| REQ-INTERACT-005 | Approval, denial, answer, expiration, cancellation, and provider failure MUST be visually distinct. | Component and browser tests cover every state. | E4-T3 |
| REQ-INTERACT-006 | Multiple connected clients MUST converge without Gul claiming authoritative exactly-once resolution. | The first upstream-accepted response wins and later clients receive the provider result. | E4-T2 |
| REQ-INTERACT-007 | Each interaction presented for user action MUST carry the provider-supplied minimum decision context for its kind, including affected command or path, reason, scope, and risk where supplied. | Per-kind contract fixtures fail when a required decision-context field is absent. | E4-T2 |
| REQ-INTERACT-008 | Every advertised provider interaction kind MUST be explicitly classified as supported or unsupported; an unclassified or unknown kind MUST fail closed and surface a blocker, never be dropped or auto-resolved. | Coverage fixtures fail on an unclassified advertised kind, and injected unknown kinds produce a typed blocker. | E4-T2 |
| REQ-INTERACT-009 | The browser-facing Interaction Card MUST be a distinct Gul-owned typed contract per interaction kind, derived by merging the observer summary and Controller Interaction with the same ID under a per-kind allowlist. The typed `ControllerInteraction.payload` oneof MUST map command approval, file-change approval, user input, and unsupported variants exhaustively after envelope/stamp, kind/variant, and effective size-limit validation. Arbitrary JSON selected only by `summary.kind` is forbidden. | Per-kind fixtures reject unknown variants, stamp drift, kind mismatch, oversize payloads, and fields outside the card allowlist; the stream summary alone never satisfies a Controller-sensitive card. | E4-T2 |
| REQ-INTERACT-010 | Protected interaction input MUST be validated in memory and sent only in one `ResolveInteraction` RPC body bounded by `min(maximum_response_bytes, 64 KiB)`, never a file carrier or metadata. Gul MAY persist only Interaction ID, idempotency key, attempt state, and non-secret metadata; response JSON, protected input, and secret-derived values MUST be excluded from delivery records, retry queues, cache, SQLite, logs, metrics, traces, diagnostics, error text, and browser storage. Protected-response RPCs MUST be excluded from automatic gRPC retries and protected input MUST never be automatically replayed. | Canary-secret tests prove absence from every store/surface; oversize values fail before parsing and invocation; lost-response tests refresh summary/full state, finish if resolved, request the value again only if pending, and otherwise preserve unresolved state. | E4-T2 |
| REQ-INTERACT-011 | A Run event MAY notify Gul that an Interaction opened but MUST NOT be treated as the complete Controller-scoped payload. | Tests require a Controller-authorized fetch before presenting protected or decision-sensitive fields. | E4-T2 |
| REQ-INTERACT-012 | Incoming Controller payloads and outgoing responses MUST each use the smaller of the provider-advertised and Gul-local byte limit, validated before parsing and before forwarding without including rejected bytes in diagnostics. Gul v0.1 MUST set `GUL_MAX_SAFE_INTERACTION_PAYLOAD_BYTES` to 8 MiB and the local protected-response cap to 64 KiB. | Boundary and oversize fixtures independently exercise `maximum_safe_payload_bytes`, the exact 8 MiB local safe-payload cap, `maximum_response_bytes`, and the 64 KiB local response cap. | E4-T2 |

### 5.6 Writer-state presentation and actions

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-WRITER-001 | Writer UI MUST use provider projections for authority state, generation, effective access/verification, lane, assurance, owner, and blocker, while Gul derives actions through its closed action evaluator. | No Gul writer-lock row, provider-supplied authoritative `allowed_actions`, or local authority state machine exists. | E4-T3 |
| REQ-WRITER-002 | `WRITE` MUST be displayed only for active writer authority plus verified effective write policy; an individual write operation also requires `write_intent=write`. | Other combinations display read-only or blocked and submit without the upstream write flag. | E4-T1 |
| REQ-WRITER-003 | Acquire and Release MUST delegate to Dolgorae only when the closed evaluator permits them and MUST display its accepted projection. Acquire is invalid for a threadless Run when `threadless_acquire_write=false`. | Local transactions do not grant or release authority; threadless acquisition is rejected before RPC. | E4-T3 |
| REQ-WRITER-005 | When another Controller owns the writer, Gul MUST show external management and MUST NOT offer direct takeover; when that Controller belongs to another Gul Direct Session, Gul MAY offer navigation to that session where the provider allows Release from it. | Only provider-allowed inspect, open, or request-release actions appear; a Controller outside Gul yields no mutation action. | E4-T3 |
| REQ-WRITER-006 | Provider failure or ambiguous policy MUST fail closed without altering cached owner or generation. | Recovery fixtures never invent release or write access. | E5-T1 |
| REQ-WRITER-007 | Writer transfer between Gul Direct Sessions MUST use provider Release followed by a separate provider Acquire; Gul MUST present the intervening unowned window and MUST NOT describe the pair as an atomic transfer. | Release/acquire fixtures show an explicit unowned state, a competing acquisition is reported from the provider result, and no Gul-side queue or reservation exists. | E4-T3 |
| REQ-WRITER-008 | The first-release action evaluator MUST return only `CanSubmitRead`, `CanSubmitWrite`, `CanAcquireWriter`, `CanReleaseWriter`, `CanInterrupt`, `CanResolveInteraction`, `CanRecover`, `CanReconcile`, `CanAdoptController`, `CanPausePrimary`, `CanResumePrimary`, `CanRequestSessionClose`, `RequiresCloseConfirmation`, `RequiresOperatorAction`, `RequiresFreshSnapshot`, `BlockedByOutcomeUnknown`, `BlockedByCredentialState`, `BlockedByBackgroundExecution`, and `BlockedByProviderCompatibility`. Session actions require fresh aggregate state and its independent revision; active owned work requires explicit interrupt confirmation. These Gul-owned decisions do not grant authority or prove closure. A continuation-required provider state maps to a typed unsupported blocker, not an action. Threadless first write maps to `CanSubmitWrite`, valid existing-thread acquisition to `CanAcquireWriter`, and outcome/recovery states block conflicts. The evaluator's typed inputs remain exhaustive and missing, unknown, or string-only values fail closed. | State-matrix tests cover every provider condition and prove every button and endpoint uses the same result; no successor or continuation action is rendered. | E4-T3 |

### 5.7 Events, persistence, and recovery

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-PROJ-001 | Provider snapshots are authoritative; cached Runtime Projections MUST be marked stale and replaced on reconnect. | Conflicting cache never overrides upstream. | E5-T2 |
| REQ-PROJ-002 | Dolgorae Cursor and Gul Client Delivery Sequence MUST be independent typed sequencing domains. Upstream cursor wire/storage representation MUST follow the accepted gRPC contract without conversion into a Gul delivery number. | Serialization and migration tests reject ambiguous use and prove that reconnect in either domain does not advance the other. | E4-T1 |
| REQ-PROJ-003 | Event bridges MUST preserve provider ID, runtime object ID, upstream cursor, delivery sequence, and correlation ID. | Reconnect tests preserve identity and order. | E4-T1 |
| REQ-PROJ-004 | Gul client events MUST use a monotonic replay sequence and snapshot fallback. | Suspended clients converge without replaying mutations. | E5-T2 |
| REQ-PROJ-005 | Gul SQLite MAY own accounts/sessions, attachments, Workspace Entries, Direct Session presentation, credential references, navigation, client delivery, FileService state, observation checkpoints, and projection cache. | No authoritative Run, Turn, interaction, writer, policy, or recovery aggregate exists. | E1-T4 |
| REQ-PROJ-009 | The observation-checkpoint store MUST be the sole durable upstream-cursor location, keyed by provider, runtime object, and stream kind, and MUST distinguish last validated from last projection-committed cursor. | Direct Session records and projection cache contain no upstream cursor; crash tests never skip an uncommitted event. | E4-T1 |
| REQ-PROJ-011 | `minimal` MUST be the default upstream event projection; broader operational projection requires an accepted capability and explicit need. | Reasoning and provider-private fields remain absent under every projection; no Gul feature requires the operational projection for v0.1. | E4-T1 |
| REQ-PROJ-012 | Gul MUST maintain at most eight logical `WatchRunEvents` streams over one reusable channel and MUST expose each Run's observation mode. Admission priority is pending Interaction/recovery or active Turn, then user-visible Run, then recent activity, with stable Run-ID tie-breaking and 30-second ordinary demotion hysteresis. | Cap-saturation fixtures admit, preempt, demote, and promote deterministically; stream count never exceeds eight. | E4-T1 |
| REQ-PROJ-013 | Runs outside the live window MUST be polled through bounded unary snapshots and pending-Interaction reads with a maximum discovery delay of 10 seconds. | Every non-live Run is checked within 10 seconds; polling needs no mutation authority and cannot indefinitely hide an Interaction. | E4-T2 |
| REQ-PROJ-014 | Stream termination MUST be reason-specific. `RUN_TERMINAL` performs required final Run/timeline/artifact work and closes without endless reconnect; `SERVER_SHUTDOWN` marks provider connectivity unavailable/restarting and resumes live Runs from committed cursors; typed gRPC `SLOW_CONSUMER` refreshes/resumes only that Run; other transport failure produces stale/disconnected state. None alone marks the Run failed. | Independent terminal, shutdown, slow-consumer, transport, heartbeat, cursor, and snapshot fixtures produce the correct state without affecting unrelated streams or mutations. | E5-T2 |
| REQ-PROJ-015 | Each live subscription MUST track Run ID, last validated cursor, last committed projection cursor, stream generation, connection state, last heartbeat, last snapshot refresh, and reconnect attempts. | Lifecycle tests assert every field across connect, event, cancellation, reconnect, snapshot replacement, and demotion. | E4-T1 |
| REQ-PROJ-016 | Every upstream event MUST pass the accepted typed Protobuf `oneof` contract through generated decoding, projection-stamp and identity validation, exhaustive variants, Run/Direct Session binding, domain mapping, authorization-sensitive enrichment, and safe projection commit before Gul delivery allocation. | Malformed, mismatched, unknown, non-monotonic, and unbound event fixtures fail; no browser event is emitted before projection commit and no raw upstream message is forwarded. | E4-T1 |
| REQ-PROJ-017 | A bounded event consumer MUST coalesce projection notifications and isolate a slow Run or browser consumer from unary mutation RPCs. Upstream `SLOW_CONSUMER` is a typed `RESOURCE_EXHAUSTED` stream error, not a `RunEventStreamEnd` enum. | Queue saturation terminates/refreshes only the affected stream from its committed cursor and preserves other stream and mutation latency budgets. | E4-T1 |
| REQ-PROJ-018 | Every event variant, heartbeat, and stream-end form MUST implement the accepted Dolgorae aggregate semantics exactly. The provider's invalidation and required-refresh set is a mandatory minimum; Gul MAY add local presentation refreshes but MUST NOT omit an upstream-required Run, Writer, Interaction, or timeline refresh. A partial event MUST NOT enable mutation while another required projection is stale or stamp-incompatible. | Descriptor/client-policy-driven coverage fails on an unclassified or weakened variant; every Dolgorae matrix row is reproduced exactly and Gul-only FileService/profile effects are proven additive. | E4-T1 |
| REQ-PROJ-019 | Gul MUST retain the complete `ProjectionStamp` separately for Run, Writer, and Interaction caches plus `captured_head_cursor` for timeline, record event invalidation floors, and converge all aggregates before action enablement. A later stamp supersedes an earlier invalidation, equal complete stamps are compatible, and continuously advancing state keeps the action disabled. | Persistence, restart, race, and fault fixtures prove no action combines incompatible revisions; stale aggregate and timeline metadata survive long enough to force refresh and never become authority. | E5-T2 |
| REQ-DATA-002 | Database migrations MUST be ordered, transactional where possible, and tested from empty state and every retained version. | Migration-up, failure-recovery, and schema-drift tests pass. | E1-T4 |
| REQ-DATA-004 | Sensitive and cached content retention MUST be minimized and configurable. Provider replay material MUST be bounded, owner-only, excluded from ordinary SQLite/logging/browser surfaces, deleted immediately after terminal resolution, and retained unresolved for at most 72 hours in v0.1 with configuration permitted only to shorten that limit. `PurgeExpired` MUST run at startup and at least every six hours. Expiry MUST mark replay unavailable without coercing the operation to success or failure. | Startup and periodic-purge tests remove resolved and expired replay files, preserve the non-secret unresolved attempt as `OutcomeUnknown`, and prove that credentials, absolute carrier paths, protected input, SubmitTurn prompts/images, and hidden reasoning are absent. | E9-T2 |
| REQ-REC-002 | Browser reconnect MUST recover presentation and fresh provider projections without duplicate runtime operations. | Network interruption tests show no duplicate submit or resolution. | E5-T2 |
| REQ-REC-003 | macOS sleep/wake and temporary tailnet loss MUST not corrupt Gul-owned state. | Reconnection tests distinguish transient, stale, and terminal conditions. | E5-T2 |
| REQ-REC-005 | Gul MUST fail closed when provider compatibility, identity, credential binding, policy, or state is uncertain. | Unsafe operations remain blocked until provider-authoritative recovery or external remediation succeeds. | E5-T1 |
| REQ-REC-006 | Startup MUST probe provider compatibility, load authoritative snapshots, resume observation, and display blockers before enabling any runtime mutation. | Ordering tests prove no mutation becomes available before the startup gate completes successfully. | E5-T2 |
| REQ-REC-007 | An ambiguous mutation outcome MUST be presented as an unresolved outcome until an authoritative snapshot resolves it; Gul MUST NOT display success, failure, or writer release for an unresolved outcome. | Outcome-unknown, blocked-unknown, recovery-required, and unverified-background fixtures render a distinct unresolved state and block dependent mutations. | E5-T1 |
| REQ-REC-008 | Absence of duplicate runtime mutation MUST be proven by fault injection across process exit, lost response, stale cursor, event gap, browser retry, and version drift, reconciling through each operation's accepted concurrency contract rather than blind replay. | Every injected failure yields exactly one upstream effect or a recorded unresolved outcome; idempotent operations reuse their key and tokenless operations are reconciled from an authoritative snapshot instead of retried. | E5-T3 |
| REQ-REC-009 | Gul MUST persist non-secret runtime-operation attempts before transmission, including normalized-request digest, replay availability, and an optional logical replay-material reference, show browser-visible pending or outcome-unknown state, and block conflicting mutations until provider evidence resolves them. | Restart and lost-response tests preserve the same operation identity and replay classification; final projection comes only from event, snapshot, timeline, exact provider replay, or reconciliation evidence. | E5-T3 |
| REQ-REC-010 | `RecoverRun` and `ReconcileRun` MUST consume the `RunProjection` in `RunMutationResponse` and perform explicit follow-up reads for Writer, Interaction, timeline, artifacts and Orchestrated Session state when required; root recovery follows the provider's aggregate-close semantics and MUST NOT be modeled as atomically returning all projections. | Recovery fixtures prove the immediate response updates only the Run and each other aggregate remains stale until its authoritative read succeeds. | E5-T1 |
| REQ-REC-011 | In the first release Gul MUST retain crash-safe canonical request material only for `StartRun` in an owner-only bounded replay store outside every Workspace. The attempt MUST reference it by logical key and retain the destination credential-store key and expected Controller ID. Continuation-specific material and exact replay belong to deferred E4-T4. No absolute carrier path or capability may be persisted. Material MUST be deleted after authoritative terminal resolution or the 72-hour maximum retention, with startup and at-least-six-hourly expiry purge. On expiry Gul MUST mark replay unavailable, preserve the non-secret attempt as `OutcomeUnknown`, use only documented secondary reconciliation, and MUST NOT mint a replacement key or silently repeat the mutation. `SubmitTurn` MAY replay only while its exact request remains in process memory; after restart it MUST reconcile without prompt/image replay and preserve `OutcomeUnknown` when acceptance cannot be proved. `ResolveInteraction` protected bytes and tokenless mutations MUST never be automatically replayed. | Crash, restart, retention, canary, role-reference, and lost-response tests prove exact allocation replay, exact StartRun carrier reconstruction, absence of path/capability/prompt/image/secret persistence in replay material, no duplicate mutation, and fail-closed unresolved behavior when replay material is unavailable or expired. | E5-T3 |

### 5.8 Output, files, and responsive UI

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-OUT-001 | The UI MUST show execution progress based on provider projection. | Spinner never claims locally inferred execution. | E7-T2 |
| REQ-OUT-002 | Gul MUST deliver and safely cache final assistant responses from provider projections. | A completed response renders after refresh without becoming authoritative local runtime state. | E7-T2 |
| REQ-OUT-003 | Reasoning, thinking, hidden chain-of-thought, and reasoning summaries MUST NOT be delivered, persisted, or logged. | Provider fixtures and Gul allowlists reject these fields. | E9-T2 |
| REQ-OUT-004 | Normal conversation MUST omit command streams and raw diffs except fields needed for an interaction. | File review remains available only through FileService. | E7-T2 |
| REQ-OUT-005 | Errors MUST be bounded, redacted, and free of unrestricted paths or runtime secrets. | Security fixtures pass. | E9-T2 |
| REQ-OUT-006 | Prompt input MUST be Korean IME-safe. | macOS, iPad, and iPhone composition tests pass. | E7-T3 |
| REQ-OUT-007 | Browser-visible prompts and final responses MUST appear in chronological provider order. | Snapshot/reconnect tests preserve order without duplication. | E7-T2 |
| REQ-OUT-008 | ArtifactCapability MUST be a required v0.1 capability for oversized final responses, large approval diffs, Controller-only review material, and replay. | Missing capability is a compatibility blocker rather than a truncated-success presentation. | E4-T5 |
| REQ-OUT-009 | Artifact reads MUST use unary chunks bounded by both the provider maximum and Gul's preferred 256 KiB size, enforce `min(provider maximum artifact size, Gul's 64 MiB cap)`, and validate metadata, authorization, exact byte length, and SHA-256. The 256 KiB inline threshold is Gul browser presentation policy, not a provider wire limit; a larger provider-inline response MAY be converted to a bounded presentation object. | Provider/Gul bound combinations, 300 KiB inline input, truncation, digest mismatch, oversize, cancellation, stale reference, and authorization fixtures produce the specified behavior. | E4-T5 |
| REQ-OUT-010 | Artifact content MUST use safe Markdown rendering where applicable and MUST never be executed or treated as a local path merely because its reference resembles one. | Script, external-resource, path-shaped-ID, and active-content fixtures remain inert. | E4-T5 |
| REQ-FILE-001 | FileService MUST browse independently of any LLM Turn. | Browse operations emit no runtime mutation. | E6-T1 |
| REQ-FILE-002 | File APIs MUST accept only Workspace Entry ID and normalized relative path. | Absolute and traversal paths are rejected. | E6-T1 |
| REQ-FILE-003 | Every access MUST resolve symlinks safely and prove containment beneath the verified root. | Symlink escape, nested symlink, replacement-race, and missing-target tests pass. | E6-T1 |
| REQ-FILE-004 | Directory listing MUST be lazy and bounded. | Large-tree tests remain responsive. | E6-T2 |
| REQ-FILE-005 | Text and source files MUST render read-only with syntax highlighting where identifiable. A provider path containing non-UTF8 bytes MUST produce `NonUtf8PathUnsupported` without lossy conversion, manipulable browser path, or raw-byte logging. | UTF-8 renders; binary and unsupported encodings use a controlled fallback; invalid-byte path fixtures expose no replacement-character alias. | E6-T2 |
| REQ-FILE-006 | Large text MUST be bounded or paged with truncation indicated. | Byte/line limits prevent server or browser exhaustion. | E6-T2 |
| REQ-FILE-007 | Supported raster images MUST render only after MIME and size validation. | PNG, JPEG, WebP, and GIF fixtures pass. | E6-T2 |
| REQ-FILE-008 | SVG MUST NOT execute active content in the application origin. | ADR-0018 selects escaped source-only rendering; script, external resources, and active links remain inert. | E6-T2 |
| REQ-FILE-009 | File changes MUST be reflected without a full application reload. | Explicit refresh invalidates affected nodes and previews and satisfies this requirement on its own; the REQ-FILE-016 watcher is the Recommended-tier enhancement over it. | E6-T3 |
| REQ-FILE-010 | File preview MUST remain read-only. | No save, rename, delete, upload, or drag-and-drop mutation exists. | E6-T3 |
| REQ-FILE-011 | FileService MUST expose Git status per file and accessible aggregate status for ancestors. | `MODIFIED`, `ADDED`, `UNTRACKED`, `DELETED`, `RENAMED`, `CONFLICTED`, and `CLEAN`, including combined staged/unstaged state, use color plus a non-color indicator; provider-private paths contribute neither direct nor aggregate status. | E6-T3 |
| REQ-FILE-012 | Markdown MUST support a safe rendered view with contained workspace-relative raster images from the selected fixed revision. | Raw HTML and automatic external loads are blocked; image paths pass all FileService and private-root guards. | E6-T2 |
| REQ-FILE-013 | Changed text, Markdown, and supported raster images MUST support fixed `HEAD` versus Working comparison. | Desktop uses side-by-side and small screens use an explicit revision switch; added, deleted, renamed, and missing-asset cases are represented without fabrication. | E6-T3 |
| REQ-FILE-014 | Browsing MUST remain usable when Git comparison is unavailable. | Non-Git, unborn `HEAD`, and unavailable Git return typed degradation without breaking current preview. | E6-T3 |
| REQ-FILE-015 | `.dolgorae` MUST appear only as one non-expandable provider-managed denied node; denial MUST be evaluated on the fully resolved real path after symlink resolution and before any read, listing, status, or provider hand-off. | Descendants and symlink or nested-symlink aliases resolving into them are unavailable to listing, preview, Markdown assets, Submit images, Git status/diff, and all ancestor aggregates. | E2-T3 |
| REQ-FILE-016 | File invalidation MUST use a bounded host filesystem watcher scoped to the verified canonical root and MUST NOT depend on any upstream event projection. | The watcher is bounded in queue depth, coalescing interval, and watched-node count; `.dolgorae/**` events are discarded at the resolved path; FileService invalidation works while the provider is unavailable. | E6-T3 |
| REQ-UI-001 | Desktop and wide-tablet layout MUST use workspace/activity, conversation, and file panes. | Responsive layout tests pass. | E7-T1 |
| REQ-UI-002 | Small screens MUST use Sessions, Chat, and Files top-level navigation. | iPhone tests preserve context. | E7-T1 |
| REQ-UI-003 | Navigation MUST display provider connectivity, activity, writer, policy, assurance, and interaction state without implying local authority. | State combinations remain unambiguous. | E7-T2 |
| REQ-UI-004 | The file pane MUST switch between explorer and preview while retaining navigation context. | Back navigation restores directory and selection. | E7-T1 |
| REQ-UI-006 | Interaction cards MUST take visual priority over passive progress. | Required action is reachable without hidden scrolling on supported mobile layouts. | E7-T2 |
| REQ-UI-007 | The PWA MUST preserve safe presentation navigation across ordinary refresh. | The installed PWA preserves E7-T1's navigation state; runtime state is revalidated and stale projection is never promoted. | E8-T3 |
| REQ-UI-008 | Primary flows MUST be keyboard accessible and use semantic controls. | Automated and manual accessibility checks pass. | E7-T3 |
| REQ-UI-009 | Typed blockers including unprovisioned workspace, missing profile, lost Controller, denied provider root, and incompatible provider MUST be presented distinctly. | Every blocker has a safe explanation and allowed next action. | E7-T2 |
| REQ-UI-010 | Blockers requiring action outside Gul, including provider-side migration, an unavailable or unverifiable provider server, and Controller reset, MUST be a distinct operator-action-required class naming the external procedure, and unresolved-outcome states MUST be visually distinct from ordinary errors. | Fixtures for each condition present no in-product retry, name the documented external step, and never render as a transient failure. | E7-T2 |

### 5.9 APIs, diagnostics, quality, and release

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-API-001 | Application services MUST be declared in Protobuf and implemented with ConnectRPC. | Auth, Runtime, WorkspacePresentation, DirectSession, InteractionPresentation, WriterAction, File, ClientEvent, and Diagnostics clients are generated and versioned. E1-T3 proves the declarations and transport contracts; E14 verifies first-release service wiring across completed features with fake-provider diagnostics, while E9-T2 owns actual-provider diagnostic/security qualification. | E14-T1 |
| REQ-API-002 | Retryable state-changing operations MUST use unary RPCs with typed provider-aware concurrency semantics. | Browser retry fault tests prove no duplicate provider effect using the E1-T3 contract and completed attempt/replay implementation; actual-provider proof remains E2/E9. | E5-T3 |
| REQ-API-003 | Client updates MUST use server-streaming RPCs with a Gul delivery sequence distinct from upstream cursors. | Replay and snapshot fallback converge in order. | E4-T1 |
| REQ-API-004 | Unknown, expired, stale, unauthorized, conflict, unavailable, and recovery-required conditions MUST have stable Gul error codes. | Clients never parse human text or raw provider errors. | E1-T3 |
| REQ-API-005 | Public APIs MUST NOT expose raw provider envelopes, capability material, unrestricted absolute paths, unvalidated or unauthorized artifact content, or arbitrary Git revisions. | Contract scans and negative serialization tests pass. | E1-T3 |
| REQ-API-006 | Every provider outcome MUST map to a stable Gul error code plus a closed provider action class; an unmapped provider error code or required-action enum MUST fail closed as a typed blocker rather than a generic failure. | The mapping is generated from the accepted upstream error/action contract, coverage fixtures fail on an unmapped value, and clients never parse provider message or action text. | E1-T3 |
| REQ-API-007 | The public domain error set MUST distinguish `TransportUnavailable`, `DeadlineExceeded`, `ProtocolIncompatible`, `ControllerMismatch`, `ControllerCarrierInvalid`, `WriteContinuationControllerInvalid`, `WriterConflict`, `ThreadlessRequiresWriteTurn`, `InteractionStale`, `InteractionAlreadyResolved`, `RecoveryRequired`, `OutcomeUnknown`, `SlowConsumer`, `ArtifactUnavailable`, `UnsupportedPathEncoding`, `RunStateConflict`, `RpcServerAlreadyRunning`, `OperatorActionRequired`, `InvalidPageToken`, and `PageTokenExpired`. | Generated ConnectRPC contracts and mapping tests cover every value and expose no human-message or action-string parsing. | E1-T3 |
| REQ-API-008 | Browser contracts MUST expose neither Dolgorae socket/carrier paths nor private worker IDs, raw Controller credentials, App Server transport details, raw upstream events, or generated upstream messages. | Descriptor inspection and negative serialization fixtures prove absence. | E1-T3 |
| REQ-OBS-001 | Structured logs MUST use Gul/provider/runtime-reference/correlation fields and exclude secrets and hidden content. | Canary-secret and bounded-output tests pass. | E9-T2 |
| REQ-OBS-002 | Diagnostic output MUST be bounded and redacted. | Oversized provider output, secret canaries, prompts, and local sensitive paths remain absent. | E9-T2 |
| REQ-OBS-003 | The user MUST be able to view a diagnostic summary reporting Gul/toolchain/database/Tailscale/provider version, compatibility, capabilities, reachability, checkpoint state, safe errors, and Controller binding health. | No App Server process ownership, capability material, or Operator-gated diagnostic operation is present. | E9-T2 |
| REQ-QA-001 | Core presentation, path, projection, cursor, and adapter behavior MUST have deterministic unit tests. | Tests require no real LLM. | E9-T1 |
| REQ-QA-003 | Browser E2E MUST cover auth, inspection, Direct Session, read/write submit, final response, interaction, writer actions, unsupported-continuation rejection, reconnect, and files. | The first-release flow matrix passes; actual successor creation remains Deferred with E4-T4. | E9-T3 |
| REQ-QA-004 | Manual checks MUST cover Wails, macOS browser, iPhone Safari/PWA, and iPad Safari/PWA. | Results and suspension limitations are recorded. | E9-T3 |
| REQ-QA-005 | Release docs MUST cover installation, Dolgorae compatibility, Tailscale Serve, backup, fail-closed Controller loss, external terminal reset followed by verified adoption, and the prohibition on hard links into `.dolgorae`. | Clean-host and credential-loss documentation dry runs succeed without claiming in-product reset and deployment guidance covers the hard-link residual. | E9-T3 |
| REQ-QA-006 | Release acceptance MUST require every Release-tier requirement to be in Current State with evidence, every Recommended-tier requirement to be promoted or explicitly recorded as accepted-incomplete, and every first-release Roadmap Task and Epic to be `Completed`. | E9-T3 verifies guards that reject an unpromoted Release-tier item, an undispositioned Recommended-tier item or an incomplete required Task/Epic. Deferred and Retired Tasks are excluded, not marked passed. The final release declaration follows E9 closeout; E9-T3 does not require its own Epic to have closed before it can test these guards. | E9-T3 |
| REQ-QA-007 | Dolgorae integration MUST have a fake public gRPC server plus opt-in smoke tests against an accepted compatible executable, and a separate exact-schema Machine CLI conformance harness. | Private interfaces are never used; production DI contains no CLI fallback; live smoke tests remain blocked until E2-T0 completes. | E9-T1 |
| REQ-QA-008 | Provider conformance MUST map every port/API operation, required identifier, enum, error/action detail, projection, event form, and carrier schema to the accepted public contract. | Checked inventories and positive/negative fixtures cover every mapping and are regenerated from the accepted descriptor/schema digests. | E9-T1 |
| REQ-QA-009 | Security tests MUST cover `.dolgorae/**` across every surface, Controller-capability and protected-interaction-input canaries across process/browser/storage paths, and Controller-loss recovery without Operator possession. | All escape and recovery drills fail closed. | E9-T2 |
| REQ-QA-010 | Integration qualification MUST cover the canonical first-release local-gRPC matrix, including local credential ownership/schema, Workspace bootstrap, socket ownership, typed projection blocking, protected-input recovery, event schema modes, error additions, whole-session closure, complete Prompt History, aggregate/result reads, and no CLI fallback. Historical continuation cases remain traceable to deferred E4-T4 and are excluded from first-release acceptance. | Every applicable scenario is individually traceable to an automated or explicitly manual owner and recorded evidence; deferred cases are named rather than silently treated as passed. | E9-T1 |

### 5.10 Accepted prompt and session requirements

Browser API contracts are explicit Gul-owned types. E1-T3 owns declaration and
schema-level fixtures; E3-T3 implements execution-state reads, E4-T5 implements
history/result reads, E4-T3 provides the action evaluator, and E5-T1 owns the
safe close coordinator. A declared route may remain explicitly unavailable until
its owner and predecessors complete. It must never use placeholder mutation
success. Pre-release acceptance is fake-scoped; E2/E9 own real-provider proof.

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-CONSUMER-001 | Pin TASK-053's immutable producer contract before new contract-based implementation, including exact wire, credential-schema digest, 27 required methods, error and close-outcome mappings, client/mutation policies, bounds, field-sourceability fixtures and independent typed-state inputs. | Regenerated consumer artifacts reproduce the new lock and required fixtures; old E0 evidence is not new pinning or implementation acceptance. | E12-T1 |
| REQ-CONSUMER-002 | Pre-release implementation MUST use explicit stateful contract-derived fakes and MUST never enable a fake as production failure fallback. | Deterministic scenario drivers prove the 27-method provider behavior, failure/recovery boundaries and explicit injection over frozen ports without depending on future Gul features. REQ-CONSUMER-004 owns assembled application acceptance. | E13-T1 |
| REQ-CONSUMER-003 | Actual adapter qualification MUST use the exact released Dolgorae v0.1.3 artifact and matching consumer lock. | E2-T0 records released build identity, hashes, capabilities, and real gateway evidence; no unreleased substitution. | E2-T0 |
| REQ-CONSUMER-004 | Before released-provider integration, the actual Gul core/browser MUST pass the assembled first-release scenarios against the explicit stateful fake, with real Gul account/session protection and no production fake fallback. | Authentication, workspace/session flows, history/originals, approvals, results, close, files and restart/reconnect pass together. Remaining real-provider, credential, UDS and deployment evidence is listed for E2/E9, not inferred passed. | E14-T1 |
| REQ-PROMPT-001 | Each session MUST expose a separate user-only Prompt History section from accepted Primary timeline items. | UI displays ordered original previews, ordinal, provider time, expandable full original, and linked Turn/chat; AI/internal Specialist prompts are excluded. | E7-T2 |
| REQ-PROMPT-002 | Original accepted prompt history MUST survive pagination, browser/Gul/provider restart and session closure. | Complete timeline and long-input artifact fixtures reconstruct exact Unicode/line endings without text-based deduplication. | E4-T5 |
| REQ-PROMPT-003 | Distinct accepted submissions with identical text MUST remain distinct; exact replay/reconnect MUST not duplicate one submission. | Identity/cursor tests, multi-client races, failed/interrupted Turn history retention, and append-during-pagination tests pass. | E4-T5 |
| REQ-PROMPT-004 | Fresh ordinary prompts during an active Turn MUST remain drafts or receive a typed pre-acceptance rejection, with no queue/steering/auto-send/auto-interrupt. | Explicit send only after terminal evidence and fresh eligibility; exact accepted replay and current Interaction answers remain distinct. | E4-T3 |
| REQ-PROMPT-005 | Pending/rejected/unknown attempts MUST be distinct from accepted history; history is never automatic replay authority. | Loss and crash tests do not infer non-acceptance from one missing page or match an attempt by prompt text alone. | E5-T3 |
| REQ-SESSION-001 | Creation MUST use an interactive-client carrier with explicit orchestration_launch and named Policy, plus parentless direct_interactive StartRun and explicit settings. | Missing/invalid launch intent never silently falls back to an unrelated low-level Run. | E2-T3 |
| REQ-SESSION-002 | Session Close MUST coordinate whole-aggregate closure through root CloseRun using the completed E4-T3 evaluator, explicit interrupt confirmation and E3-T3 aggregate reads. Map SESSION_CLOSE_IN_PROGRESS to pending, correlate retained provider operation identity through Gul-owned references, and never retry an ambiguous tokenless close automatically. | Fake-backed pending/closing/confirmed/unknown/recovery cases preserve all owned-effect distinctions, history and unrelated execution without child calls. E3-T3 builds passive session/read models only; E5-T1 enables the complete safe coordinator. E7-T2 integrates UI and E2/E9 supply actual-provider proof. | E5-T1 |
| REQ-SESSION-003 | GetOrchestratedSession MUST be the authority for aggregate status/revision/policy/counts/recovery, not reconstructed Run links. | Wrong Controller and non-session roots fail; absent/stale evidence never becomes a fabricated empty or completed session. | E3-T3 |
| REQ-SESSION-004 | Specialist results MUST be discovered through ListOrchestratedSessionResults before metadata/chunk reads under its explicit Primary owner. | Stable bounded pages survive restart and close; no private ID injection, child credential, model-text parsing, or fabricated Primary response. | E4-T5 |
| REQ-API-009 | DirectSessionService MUST declare typed ListPromptHistory and GetPromptHistoryItem operations over Gul session/item IDs, bounded pages and Gul-owned opaque tokens. Preserve accepted-input identity/order and lossless full content or a validated ArtifactPresentation reference. | Contract fixtures define empty-but-continuable pages, snapshot scope, explicit expiration, duplicate/new-same-text inputs, ordinal coverage, long input and authorization. E4-T5 implements reads and E7-T2 presents them. | E1-T3 |
| REQ-API-010 | DirectSessionService MUST declare GetExecutionState and a typed CloseRuntime outcome boundary, with Gul-owned state/operation references, freshness and distinct pending/confirmed/unknown/recovery classifications. | Browser types contain no raw provider projection/cursor/operation ID. Contract fixtures preserve producer close semantics, include stable typed rejection code/action for REJECTED, and reject optimistic closure or pre-auth state disclosure; E3-T3 implements reads and E5-T1 coordinates closure. | E1-T3 |
| REQ-API-011 | DirectSessionService MUST declare ListSpecialistResults with bounded snapshot-scoped Gul pagination, result/view IDs and validated ArtifactPresentation references. | Foreign-session/query tokens and raw provider cursor injection fail. Collected results remain discoverable; source-unavailable is distinct from an empty page. E4-T5 implements the read path. | E1-T3 |

## 6. Deferred State ledger

Deferred requirements do not participate in v0.1 acceptance unless explicitly promoted through an ADR and Roadmap change.

| ID | Deferred requirement | Owner |
|---|---|---|
| REQ-GORAE-001 | A future Gorae Provider remains a trusted built-in out-of-process adapter. | Deferred-Gorae |
| REQ-GORAE-002 | Gul calls only Gorae public operations and never mutates Gorae-owned Dolgorae Runs directly. | Deferred-Gorae |
| REQ-GORAE-003 | Managed interactions expose only Gorae-defined summaries or escalations; Gul never resolves low-level Dolgorae interactions. | Deferred-Gorae |
| REQ-WRITER-004 | Same-controller handoff MAY be offered only when Dolgorae reports eligibility and MUST use its prepare/commit/cancel protocol. | Deferred-Handoff |
| REQ-DIRECT-010 | A supported write continuation from shared-readonly or an unsupported-transition reader MUST create a distinct threadless dedicated Run and Controller without changing the source. Preserve immutable lineage to the exact current terminal source Turn; no separate Acquire before its first write. | E4-T4 |
| REQ-DIRECT-017 | Before CreateWriteContinuation persist the exact key, source Run/terminal Turn, destination Controller/key, reason and normalized identity. Lost-response recovery replays that same request; Controller-matched ListRuns is secondary and Reconcile applies only after the destination Run is known. Never mint a replacement credential/key or mutate the source. | E4-T4 |
| REQ-CTRL-013 | A successor uses a fresh Controller/capability and generation 1, preserving source kind, subject_id and installation instance_id individually and the normalized principal. Reject cross-principal or changed-installation transfer and retain destination identity across retries. | E4-T4 |
| REQ-PODWAY-001 | Render the full pinned FSM graph, active node set, per-loop iteration, and stable node execution count from optional Dolgorae observations. | Deferred-Podway |
| REQ-PODWAY-002 | Separate workflow execution from Session/Run identity; preserve counts and freshness across duplicates, reconnect, nested loops, and parallel nodes. | Deferred-Podway |
| REQ-PODWAY-003 | No Gul frontend/backend/API may edit FSM or jump/skip/force/reexecute nodes or reset counts. Such requests are ordinary prompts judged by the running LLM; actual Podway state alone updates the graph. | Deferred-Podway |
| REQ-PODWAY-004 | Missing future observation capability MUST NOT block chat, history, approvals, or close. | Deferred-Podway |

REQ-WRITER-004 retains its original ID and meaning and is deferred, not superseded. Under ADR-0035 Gul policy binds a distinct Controller to every Direct Session/Run pair, so no two Gul-owned Runs share a Controller and the same-controller handoff precondition is unreachable in v0.1. Reactivation requires an accepted Controller-scope change. `Deferred-Handoff` is a deferred owner label, not a Roadmap task.

## 7. Current State ledger

E1-T1 through E1-T5 establish the delivery-independent Go core, one checked
React bundle, declared but disabled browser API, isolated SQLite repositories,
and a Wails shell foundation using the shared asset-delivery boundary. They do
not promote E14-owned REQ-HOST-001/002 or implement a listener, enabled browser
API, production persistence lifecycle, provider adapter, product route, or
production authentication. The Wails shell has no authenticated attach.
E12-T1 has accepted the immutable consumer contract and generated tooling
boundary; later Tasks still own the remaining product behavior. Former
E12-T2/T3 are Retired without implementation evidence.
E13-T1 adds an independently tested scenario provider over the frozen port;
assembled application behavior and a live provider remain future work.
E3-T1 adds fake-scoped Workspace registration and revalidation through an
unmounted authenticated handler; it does not enable a product route or qualify
the released provider.
E3-T2 adds subject-scoped Workspace and Direct Session display metadata and
navigation with local-only mutations. Its typed handlers remain unmounted.
E3-T3 adds fake-scoped Primary bindings and passive aggregate reads. It does
not create Runs, register a product route, or enable whole-session close.
E3-T4 adds fake-scoped global Profile launch selection, unmounted catalog and
configuration handlers, and the shared read-only consent boundary. It creates
no Run or Controller carrier.
E4 adds typed event delivery, Controller Interaction handling, shared action
eligibility and bounded history/result/artifact reads. Its 41 requirements are
accepted against checked provider fakes and isolated browser components; live
provider qualification and authenticated product assembly remain with
their named later owners. E5-T1 adds whole-session close, explicit recovery,
typed provider health and bounded restart policy against injected components.
Its nine requirements are accepted at the same fake/component boundary.
E5-T2 adds provider and browser reconnect coordination, startup convergence,
and stamp gating against controlled fakes. E5-T3 adds operation-specific replay
and uncertainty handling against checked provider fakes. Both remain unmounted;
assembled UI and live-provider qualification remain future work.
E6-T1 adds a local, read-only FileService guard over subject-scoped verified
Workspace attachments and one unmounted typed inspection handler. Its accepted
scope is isolated root access; preview, Git, watcher and assembled product
surfaces remain with E6-T2/T3 and later integration owners.
E6-T2 adds bounded listing, text and raster preview, safe Markdown image
embedding, and source-only SVG at the same unmounted component boundary.
Working-tree reads may observe intermediate writer state; fixed `HEAD`
comparison, invalidation, and assembled file-pane navigation remain with
E6-T3 and E7.
E6-T3 adds explicit refresh, a bounded host watcher, and fixed `HEAD` versus
Working Git review through unmounted FileService handlers. Git status, private
path denial, and typed degradation are accepted at this component boundary;
E7/E8/E14 own the assembled pane and authenticated delivery. E6-T1 established
the guard used by E6-T2/T3. E2-T3 owns the Submit-image handoff and final
cross-surface promotion of REQ-FILE-015, which remains Required State.

| Requirement | Accepted Current State | Evidence |
|---|---|---|
| REQ-HOST-005 | Exact Go toolchain with minimum-compatible non-Go host tools, exact project dependency and generator pins, and read-only host reporting | E0-T8; ADR-0054; `toolchain/versions.env`; checker fixtures; serial `make test` |
| REQ-RUNTIME-011 | Versioned public gRPC inventory, exact semantic-operation ownership, generated clients, descriptor-derived fake server, exhaustive maps, and separate Machine CLI fixture | E0-T7 historical digests recorded in the implementation memo; current live lock paths are E12-T1 evidence; contract validator; fake-server tests |
| REQ-RUNTIME-022 | Independently typed projection inputs, public enum/event inventory, convergence and fail-closed compatibility fixtures | E0-T7 historical digests recorded in the implementation memo; current live policy maps are E12-T1 evidence; conformance pin |
| REQ-CONSUMER-001 | Immutable TASK-053 consumer source, 36-method descriptor inventory, explicit 27-required/9-unavailable profile, generated clients/maps/fake transport, and close/sourceability fixtures; no runtime-support claim | E12-T1; dependency/generated locks; additive descriptor check; contract validator |
| REQ-CONSUMER-002 | Explicit deterministic stateful provider implementing all 27 required port methods with reset, clock, fault and stream controls; no production injection or fallback | E13-T1; `contract/scenario` port assertions and scenario tests; `make test` |
| REQ-WS-001 | Host picker and allowlist-relative remote registration reach one canonical attachment; no browser absolute-path input | E3-T1; `internal/workspace` and `internal/delivery/api` tests; `make test` |
| REQ-WS-003 | Attached Workspaces retain editable display names independent of directory basenames; favorite and hidden state persist locally | E3-T2; `internal/storage` and `internal/delivery/api` presentation tests; `make test` |
| REQ-WS-004 | Three independent Primary Runs remain separately bound and readable across two Workspaces without a Gul writer arbiter | E3-T3 scenario-backed concurrent session test; `make test` |
| REQ-WS-006 | Attachment revalidation verifies stored canonical root, inode, and provider identity before scoped use | E3-T1; moved-root and changed-ID fixtures; `make test` |
| REQ-WS-007 | Bootstrap inspection omits expected provider ID; subsequent revalidation sends the stored path and ID; no initialization call | E3-T1; recording fake provider fixture; `make test` |
| REQ-WS-008 | Removing a Workspace Entry deletes only Gul presentation, navigation and local bindings; provider Runs remain untouched | E3-T3 local removal and API tests; `make test` |
| REQ-WS-009 | Registration accepts a compatible inspected workspace and returns typed unavailable-workspace, profile, and Profile Server blockers without provisioning | E3-T1 fake-scoped blocker fixtures; E3-T4 fake-scoped launch Policy selection; `make test` |
| REQ-WS-010 | Symlink aliases collapse to one attachment; moved or replaced identity fails closed | E3-T1 canonicalization and revalidation fixtures; `make test` |
| REQ-WS-011 | Registration invokes inspection only and creates no Git worktree | E3-T1; inspected provider port and registration call graph; workspace and Git-metadata non-mutation fixture; `make test` |
| REQ-WS-012 | Host-loaded canonical allowlist bounds browsing; outside, traversal, symlink, and private-subtree probes fail with one typed selection error | E3-T1 containment fixtures; `make test` |
| REQ-FILE-001 | Local FileService inspection reads a saved verified Workspace root without a Turn or provider call; component scope, unmounted | E6-T1; isolated filesystem and API tests; serial `make test`; implementation memo |
| REQ-FILE-002 | The typed inspection API accepts only a Workspace Entry ID and normalized relative path, with no browser absolute path or provider identity; component scope, unmounted | E6-T1; generated API drift and negative request tests; serial `make test`; implementation memo |
| REQ-FILE-003 | The shared guarded accessor rechecks the root identity, resolves in-root symlinks through anchored descriptors, and rejects escape or replacement before reading; component scope, unmounted | E6-T1; isolated root, nested symlink, alias, replacement and API tests; serial `make test`; implementation memo |
| REQ-FILE-004 | Directory listing reads one bounded page and exposes continuation without materializing the tree; component scope, unmounted | E6-T2; large-directory page fixture; serial `make test`; implementation memo |
| REQ-FILE-005 | UTF-8 source preview is inert and highlights known keywords; binary text falls back and invalid-byte directory names return a typed error without an alias; component scope, unmounted | E6-T2; Go and React preview fixtures; serial `make test`; implementation memo |
| REQ-FILE-006 | Text preview caps bytes at 256 KiB and lines at 4,000 with a visible truncation marker; component scope, unmounted | E6-T2; over-limit fixtures; serial `make test`; implementation memo |
| REQ-FILE-007 | PNG, JPEG, WebP and GIF previews pass MIME, byte and dimension checks before inert data-image rendering; component scope, unmounted | E6-T2; image fixtures and oversize fallback; serial `make test`; implementation memo |
| REQ-FILE-008 | ADR-0018 source-only SVG is escaped in the preview component; component scope, unmounted | E6-T2; active SVG fixture; serial `make test`; implementation memo |
| REQ-FILE-012 | Markdown is rendered through an HTML-free allowlist; at most eight same-working-revision raster assets pass the guarded FileService accessor and external or private references remain literal; component scope, unmounted | E6-T2; Markdown asset and external/private fixtures; serial `make test`; implementation memo |
| REQ-FILE-009 | Explicit refresh invalidates retained directory pages and increments a Workspace revision without a provider call; component scope, unmounted | E6-T3; cursor invalidation test; serial `make test`; implementation memo |
| REQ-FILE-010 | FileService review APIs expose reads and invalidation only; no file mutation API exists; component scope, unmounted | E6-T3; API surface review; serial `make test`; implementation memo |
| REQ-FILE-011 | Contained Git status reports direct and ancestor changes, staged/unstaged flags, and color plus text labels; private paths are filtered; component scope, unmounted | E6-T3; Git fixture and React status test; serial `make test`; implementation memo |
| REQ-FILE-013 | Fixed `HEAD` and Working text, Markdown, and raster revisions render side by side or through an explicit narrow-screen switch, including missing and rename cases; component scope, unmounted | E6-T3; revision fixture and React comparison test; serial `make test`; implementation memo |
| REQ-FILE-014 | Non-Git, unborn `HEAD`, and unavailable Git return typed degradation while retaining Working preview; component scope, unmounted | E6-T3; degradation fixture; serial `make test`; implementation memo |
| REQ-FILE-016 | Root-scoped host watcher bounds node count, scan count, queue depth, and coalescing interval; private events do not invalidate; component scope, unmounted | E6-T3; watcher isolation and limit tests; serial `make test`; implementation memo |
| REQ-DIRECT-001 | Each locally accepted Primary Run has one persisted subject-scoped Direct Session binding; three Runs list and reopen without a Codex thread field | E3-T3 scenario-backed binding and SQLite reopen test; `make test` |
| REQ-DIRECT-003 | Permanent shared read-only launch requires an explicit lane choice, visible warning and acknowledgement before configuration check | E3-T4 React selection, authenticated handler and domain tests; `make test` |
| REQ-DIRECT-007 | Direct Session name, favorite, archive and last navigation selection are subject-scoped SQLite presentation; offline actions invoke no provider method | E3-T2; restart, isolation and offline handler tests; `make test` |
| REQ-DIRECT-009 | Global Profile catalog projects compatibility, models, lanes, maximum assurance, capability summary and runtime version; unsupported choices are blocked | E3-T4 pinned-contract adapter, browser validator and UI tests; `make test` |
| REQ-DIRECT-015 | Specialist parent links provide typed observer-only status; aggregate counts and state come only from authorized GetOrchestratedSession | E3-T3 scenario Specialist and wrong-Controller fixtures; `make test` |
| REQ-DIRECT-019 | Checked GetRun configuration refreshes the local Primary projection on bind, rediscovery and reopen, including profile, purpose, model, effort, capabilities and instruction identity | E3-T3 provider-overrides-cache and reopen fixtures; `make test` |
| REQ-SESSION-003 | Authorized GetOrchestratedSession supplies aggregate identity, revision, policy, counts and recovery; non-session roots and unavailable reads cannot become an empty or completed aggregate | E3-T3 scenario-backed provider adapter and browser read tests; `make test` |
| REQ-DIRECT-006 | Shared typed Pause, Resume, Interrupt, Recover, Reconcile and aggregate Close eligibility with explicit owned-work interruption consent; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-DIRECT-013 | First WRITE admission and typed writer conflict remain distinct from ambiguous outcomes; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-DIRECT-014 | Unsupported or unverified transitions preserve fixed source access and return an unsupported blocker; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-DIRECT-016 | Threadless direct Acquire is rejected before RPC; the exact initial policy admits an eligible first SubmitTurn WRITE; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-WRITER-001 | Writer presentation uses checked provider authority, generation, policy, lane, assurance, owner and blocker projections; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-WRITER-003 | Acquire and Release re-evaluate shared backend eligibility, invoke once and display the accepted projection behind a fresh-read barrier; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-WRITER-005 | External ownership is visible with no takeover or local authority transfer; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-WRITER-007 | Release and later Acquire remain separate with an explicit unowned window and provider-reported competition; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-WRITER-008 | Exactly 19 flags use fresh typed facts and independent aggregate state; pending local calls block recovery even alongside unknown attempts; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-INTERACT-005 | Six Interaction outcomes have distinct component and browser presentations; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-PROMPT-004 | Active-Turn prompts remain drafts and require explicit submission after fresh eligibility; interruption consent begins unchecked; fake/component scope, unmounted | E4-T3; `internal/action`, storage/API and React tests; serial `make test`; Chrome action fixture; implementation memo |
| REQ-DIRECT-008 | Controller-authorized timeline and fresh Run snapshots reconstruct all five kinds with stable Gul identities and a bounded non-authoritative metadata cache; fake/component scope, unmounted | E4-T5; `internal/history`, checked provider/API and renderer tests; serial `make test`; Chrome artifact fixture; implementation memo |
| REQ-OUT-008 | Required timeline and artifact capabilities reject missing or incompatible negotiation before history reads; fake/component scope, unmounted | E4-T5; `internal/history`, checked provider/API and renderer tests; serial `make test`; Chrome artifact fixture; implementation memo |
| REQ-OUT-009 | Negotiated and local artifact bounds, exact metadata/chunk/length/SHA-256 verification and authorized rereads precede byte delivery; fake/component scope, unmounted | E4-T5; `internal/history`, checked provider/API and renderer tests; serial `make test`; Chrome artifact fixture; implementation memo |
| REQ-OUT-010 | Allowlisted Markdown renders active syntax and path-shaped strings as inert text without external resource loads; fake/component scope, unmounted | E4-T5; `internal/history`, checked provider/API and renderer tests; serial `make test`; Chrome artifact fixture; implementation memo |
| REQ-PROMPT-002 | Accepted human originals retain Unicode and line endings across bounded pages, replaced service instances, SQLite reopen and session closure; fake/component scope, unmounted | E4-T5; `internal/history`, checked provider/API and renderer tests; serial `make test`; Chrome artifact fixture; implementation memo |
| REQ-PROMPT-003 | Separate identical submissions retain distinct stable IDs; exact replay and concurrent reconstruction preserve one entry per accepted item; fake/component scope, unmounted | E4-T5; `internal/history`, checked provider/API and renderer tests; serial `make test`; Chrome artifact fixture; implementation memo |
| REQ-SESSION-004 | Public result discovery preserves fixed publication scope, Primary artifact ownership and stable result/artifact references after reopen and closure; fake/component scope, unmounted | E4-T5; `internal/history`, checked provider/API and renderer tests; serial `make test`; Chrome artifact fixture; implementation memo |
| REQ-WRITER-002 | WRITE presentation requires active authority and verified effective write policy; each write requires explicit intent; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-002 | Provider cursors and Gul delivery sequences use separate types and independent replay boundaries; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-003 | Bound provider/Run identity, cursor, delivery sequence and correlation survive event commit and replay; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-009 | Observation checkpoints distinguish validated and projection-committed cursors; rollback and reopen preserve replay safety; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-011 | The checked event adapter requests minimal projection and emits payload-free allowlisted invalidations; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-012 | One shared provider admits at most eight streams with deterministic priority, stable ties and demotion hysteresis; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-015 | Subscription metadata tracks generation, connection, heartbeat, snapshot refresh and reconnect attempts across lifecycle changes; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-016 | Generated typed envelopes pass identity, stamp, variant and binding checks before transactional invalidation and delivery allocation; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-017 | Bounded per-Run queues, coalescing and typed slow-consumer handling isolate streams and unary mutations; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-018 | Exhaustive generated event mapping preserves every mandatory aggregate invalidation and refresh; stale projections cannot grant actions; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-API-003 | Unmounted authenticated ConnectRPC streaming uses independent Gul delivery replay and bounded snapshot fallback; fake/component scope, unmounted | E4-T1; observation, storage, event API and writer presentation behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-001 | Observer pending summaries become payload-free client invalidations; fresh authorized reads supply actionable cards; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-002 | Current subject/session/Workspace and Controller identity/generation guard every sensitive card read; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-003 | Typed card allowlists exclude raw provider payloads, private paths and unrelated sensitive fields; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-004 | Normalized one-shot resolution treats receipts as status and refreshes authoritative state before presenting the outcome; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-006 | Competing clients converge through provider acceptance and subsequent reads without local exactly-once claims; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-007 | Each supported card requires its typed command/path, reason, scope and decision context; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-008 | Every advertised Interaction kind is classified; unknown or mismatched variants fail closed; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-009 | Gul cards merge matching observer and Controller projections after typed payload, stamp and size validation; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-010 | Protected responses remain bounded in-memory one-shot bodies; response loss triggers fresh reads and explicit re-entry, with no persistence or replay; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-011 | Event summaries never substitute for a fresh Controller-authorized decision payload; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-INTERACT-012 | Selected payloads and outgoing responses honor negotiated limits plus exact local 8 MiB and 64 KiB caps; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-PROJ-013 | Independent bounded observer polling covers every non-live Run and publishes invalidations without advancing event checkpoints; fake/component scope, unmounted | E4-T2; interaction, checked provider, storage, API and React behavior and race tests; serial `make test`; implementation memo |
| REQ-RUNTIME-009 | Typed provider health preserves disconnected, incompatible, busy and degraded states with explicit stale snapshots; fake/component scope, unmounted | E5-T1; session, checked provider, API and ProviderStatus tests; serial `make test`; implementation memo |
| REQ-RUNTIME-012 | Recover/Reconcile require advertised methods and a matching typed provider instruction; fake/component scope, unmounted | E5-T1; capability, evaluator and close-coordinator tests; serial `make test`; implementation memo |
| REQ-RUNTIME-018 | Injected supervisor enforces jittered backoff, rolling start budget, stable reset and terminal blockers; fake/component scope, unmounted | E5-T1; fake-clock restart tests; serial `make test`; implementation memo |
| REQ-CTRL-008 | Missing or invalid backend Controller binding remains a typed blocker before dispatch; fake/component scope, unmounted | E5-T1; credential and authorization tests; serial `make test`; implementation memo |
| REQ-WRITER-006 | Independent Writer refresh preserves retained owner/generation and native stamp on uncertainty; fake/component scope, unmounted | E5-T1; Writer adapter and refresh-cache failure tests; serial `make test`; implementation memo |
| REQ-REC-005 | Uncertain compatibility, identity, credential, policy or state blocks mutation admission; fake/component scope, unmounted | E5-T1; checked adapter and evaluator failure tests; serial `make test`; implementation memo |
| REQ-REC-007 | Retained unknown attempts resolve only through agreeing authoritative observations without retransmission; fake/component scope, unmounted | E5-T1; loss, cancellation, persistence-failure and reopen tests; serial `make test`; implementation memo |
| REQ-REC-010 | Recovery consumes only the immediate Run; each dependent aggregate requires its own read and bounded artifact verification; fake/component scope, unmounted | E5-T1; refresh, transaction rollback and invalidation-race tests; serial `make test`; implementation memo |
| REQ-SESSION-002 | Root-only whole-session close requires interrupt consent, stable opaque correlation and whole-aggregate confirmation; fake/component scope, unmounted | E5-T1; checked scenario, concurrent attempt, storage and API tests; serial `make test`; implementation memo |
| REQ-RUNTIME-019 | Reconnect keeps durable Run identity, resumes from committed cursors and leaves in-flight mutations unresolved; fake/component scope, unmounted | E5-T2; controlled restart, stream, storage and convergence tests; serial `make test`; implementation memo |
| REQ-PROJ-001 | Provider aggregate caches become stale on reopen or loss and require fresh independent reads; fake/component scope, unmounted | E5-T2; storage reopen and reconnect tests; serial `make test`; implementation memo |
| REQ-PROJ-004 | Browser delivery replay or snapshot fallback follows current state reads without invoking provider mutations; fake/component scope, unmounted | E5-T2; browser replay and snapshot tests; serial `make test`; implementation memo |
| REQ-PROJ-014 | Terminal, shutdown, slow-consumer and transport stream ends retain distinct recovery paths; fake/component scope, unmounted | E5-T2; stream lifecycle and sibling-isolation tests; serial `make test`; implementation memo |
| REQ-PROJ-019 | Persisted aggregate stamps, invalidation floors and verified timeline heads gate action convergence; fake/component scope, unmounted | E5-T2; persistence, race and rollback tests; serial `make test`; implementation memo |
| REQ-REC-002 | Browser reconnection refreshes presentation and bounded delivery without replaying a provider mutation; fake/component scope, unmounted | E5-T2; reconnect and delivery tests; serial `make test`; implementation memo |
| REQ-REC-003 | Transient connectivity loss keeps Run identity and distinguishes stale from terminal state; fake/component scope, unmounted | E5-T2; startup and recovery fixtures; serial `make test`; implementation memo |
| REQ-REC-006 | Startup mutation gate requires compatibility, snapshots, convergence and observation admission; fake/component scope, unmounted | E5-T2; failure-path and admission tests; serial `make test`; implementation memo |
| REQ-DIRECT-018 | Mutation policy inventory defines deadlines, retry, uncertainty, reconciliation and final projection; fake/component scope, unmounted | E5-T3; architecture Section 6.3, checked adapter and mutation fault tests; serial `make test`; implementation memo |
| REQ-DIRECT-020 | StartRun persists its key, Controller and credential reference, Workspace and semantic identity before dispatch; exact replay and Controller-matched secondary reconciliation; fake/component scope, unmounted | E5-T3; orphan-file, response-loss, restart and checked adapter tests; serial `make test`; implementation memo |
| REQ-REC-008 | Faulted mutation paths preserve one upstream effect or an unresolved attempt across loss, restart, browser retry, event gaps and compatibility drift; fake/component scope, unmounted | E5-T3; mutation, storage and API fault tests; serial `make test`; implementation memo |
| REQ-REC-009 | Non-secret attempts persist before provider calls and block conflicting effects until verified resolution; fake/component scope, unmounted | E5-T3; migration, atomic Begin, reopen and protected-response tests; serial `make test`; implementation memo |
| REQ-REC-011 | StartRun uses bounded owner-only exact replay, expiry and Controller-matched reconciliation; SubmitTurn remains memory-only and protected/tokenless operations do not replay; fake/component scope, unmounted | E5-T3; replay, adapter, retention, canary and restart tests; serial `make test`; implementation memo |
| REQ-API-002 | Unary Writer browser retry cannot send again after an uncertain tokenless response; fake/component scope, unmounted | E5-T3; browser API loss and retry test; serial `make test`; implementation memo |
| REQ-PROMPT-005 | Pending and unknown SubmitTurn attempts remain separate from accepted history; missing pages and matching text never authorize resend; fake/component scope, unmounted | E5-T3; checked adapter and process-exit tests; serial `make test`; implementation memo |

## 8. Explicit v0.1 non-goals and limitations

Gul v0.1 does not provide an LLM; direct App Server integration; Codex thread/Turn authority; a local writer lock; background-process containment; Gul-owned independent-agent orchestration; Gorae Mission/Task/Workflow semantics; arbitrary plugins; a marketplace; a generic multi-provider LLM abstraction; public internet exposure; multi-user accounts; native mobile apps; automatic Git worktrees; browser file editing; a browser PTY; reasoning display; atomic writer handoff between Direct Sessions; a Gul-side writer queue or reservation; or parallel writers in one Workspace. A brokered child remains provider-owned and observer-only in Gul under REQ-DIRECT-015. Human prompt queue/steering, direct Podway manipulation, and first-release write continuation/delete are excluded.

Gul never initializes a Dolgorae workspace, provisions a Runtime Profile, holds an Operator capability, or invokes operator-gated diagnostics. A lost Controller capability cannot be reset inside Gul v0.1; recovery requires an external terminal reset followed by in-product verified adoption under REQ-CTRL-009. The `.dolgorae` provider-private subtree, including user-tracked provider policy files, is intentionally unavailable through Gul FileService and Git review.

## 9. Requirement migration and supersession appendix

Each former requirement has exactly one disposition. `Modify` retains its stable ID and user-visible intent. `Supersede` retires the ID and names a fresh replacement or external owner.

| Old ID | Disposition | Destination or owner |
|---|---|---|
| REQ-HOST-001 | Modify | REQ-HOST-001 |
| REQ-HOST-002 | Keep | REQ-HOST-002 |
| REQ-HOST-003 | Keep | REQ-HOST-003 |
| REQ-HOST-004 | Supersede | Dolgorae; REQ-RUNTIME-005 |
| REQ-HOST-005 | Modify | REQ-HOST-005 and REQ-HOST-006 |
| REQ-AUTH-001 | Keep | REQ-AUTH-001 |
| REQ-AUTH-002 | Keep | REQ-AUTH-002 |
| REQ-AUTH-003 | Keep | REQ-AUTH-003 |
| REQ-AUTH-004 | Keep | REQ-AUTH-004 |
| REQ-NET-001 | Keep | REQ-NET-001 |
| REQ-NET-002 | Keep | REQ-NET-002 |
| REQ-NET-003 | Keep | REQ-NET-003 |
| REQ-WS-001 | Modify | REQ-WS-001 and REQ-WS-007 |
| REQ-WS-002 | Supersede | REQ-WS-006, REQ-WS-007, REQ-WS-010 |
| REQ-WS-003 | Modify | REQ-WS-003 |
| REQ-WS-004 | Modify | REQ-WS-004 |
| REQ-WS-005 | Supersede | REQ-WS-008 |
| REQ-SES-001 | Supersede | REQ-DIRECT-001 |
| REQ-SES-002 | Supersede | REQ-DIRECT-001; Dolgorae owns the thread |
| REQ-SES-003 | Supersede | REQ-DIRECT-008, REQ-OUT-002, REQ-OUT-007 |
| REQ-SES-004 | Supersede | Dolgorae; REQ-DIRECT-005 |
| REQ-SES-005 | Supersede | REQ-DIRECT-006 and REQ-DIRECT-007 |
| REQ-SES-006 | Supersede | Dolgorae; REQ-WRITER-001 |
| REQ-TURN-001 | Supersede | Dolgorae; REQ-DIRECT-004 |
| REQ-TURN-002 | Supersede | Dolgorae; REQ-DIRECT-004 and REQ-WRITER-002 |
| REQ-TURN-003 | Supersede | Dolgorae; REQ-WRITER-001 and REQ-WRITER-003 |
| REQ-TURN-004 | Supersede | Dolgorae; REQ-DIRECT-010 |
| REQ-TURN-005 | Supersede | Dolgorae; REQ-DIRECT-005 |
| REQ-TURN-006 | Supersede | Dolgorae; REQ-PROJ-001 |
| REQ-TURN-007 | Supersede | Dolgorae assurance; REQ-RUNTIME-004 |
| REQ-LOCK-001 | Supersede | Dolgorae; REQ-WRITER-001 |
| REQ-LOCK-002 | Supersede | Dolgorae; REQ-WRITER-001 |
| REQ-LOCK-003 | Supersede | Dolgorae; REQ-WRITER-006 |
| REQ-LOCK-004 | Supersede | Dolgorae; REQ-WRITER-003 |
| REQ-LOCK-005 | Supersede | Dolgorae; REQ-WRITER-003 |
| REQ-LOCK-006 | Supersede | Dolgorae; REQ-DIRECT-013 |
| REQ-LOCK-007 | Supersede | Dolgorae; REQ-WRITER-004 |
| REQ-LOCK-008 | Supersede | Dolgorae; REQ-WRITER-004 |
| REQ-LOCK-009 | Supersede | Dolgorae; REQ-WRITER-004 |
| REQ-LOCK-010 | Supersede | Dolgorae; REQ-WRITER-004 |
| REQ-LOCK-011 | Supersede | Dolgorae; REQ-WRITER-004 |
| REQ-LOCK-012 | Supersede | REQ-WS-011 |
| REQ-CODEX-001 | Supersede | Dolgorae; REQ-RUNTIME-001 and REQ-RUNTIME-005 |
| REQ-CODEX-002 | Supersede | Dolgorae; REQ-RUNTIME-008 and REQ-QA-008 |
| REQ-CODEX-003 | Supersede | Dolgorae; REQ-RUNTIME-005 |
| REQ-CODEX-004 | Supersede | Dolgorae; REQ-RUNTIME-005 |
| REQ-CODEX-005 | Supersede | Dolgorae; REQ-DIRECT-005 and REQ-INTERACT-004 |
| REQ-CODEX-006 | Supersede | Dolgorae; REQ-WRITER-006 |
| REQ-INT-001 | Supersede | REQ-INTERACT-001 and REQ-INTERACT-004 |
| REQ-INT-002 | Supersede | REQ-INTERACT-002 through REQ-INTERACT-004 |
| REQ-INT-003 | Supersede | REQ-INTERACT-002 through REQ-INTERACT-004 |
| REQ-INT-004 | Supersede | REQ-INTERACT-002 through REQ-INTERACT-004 |
| REQ-INT-005 | Supersede | REQ-PROJ-001 and REQ-INTERACT-001 |
| REQ-INT-006 | Supersede | REQ-INTERACT-006 |
| REQ-INT-007 | Supersede | Dolgorae; REQ-WRITER-001 |
| REQ-INT-008 | Supersede | REQ-INTERACT-005 |
| REQ-OUT-001 | Modify | REQ-OUT-001 |
| REQ-OUT-002 | Modify | REQ-OUT-002 |
| REQ-OUT-003 | Modify | REQ-OUT-003 |
| REQ-OUT-004 | Keep | REQ-OUT-004 |
| REQ-OUT-005 | Keep | REQ-OUT-005 |
| REQ-OUT-006 | Keep | REQ-OUT-006 |
| REQ-OUT-007 | Modify | REQ-OUT-007 |
| REQ-FILE-001 | Modify | REQ-FILE-001 |
| REQ-FILE-002 | Modify | REQ-FILE-002 |
| REQ-FILE-003 | Modify | REQ-FILE-003 |
| REQ-FILE-004 | Keep | REQ-FILE-004 |
| REQ-FILE-005 | Keep | REQ-FILE-005 |
| REQ-FILE-006 | Keep | REQ-FILE-006 |
| REQ-FILE-007 | Keep | REQ-FILE-007 |
| REQ-FILE-008 | Keep | REQ-FILE-008 |
| REQ-FILE-009 | Keep | REQ-FILE-009 |
| REQ-FILE-010 | Keep | REQ-FILE-010 |
| REQ-FILE-011 | Modify | REQ-FILE-011 and REQ-FILE-015 |
| REQ-FILE-012 | Modify | REQ-FILE-012 and REQ-FILE-015 |
| REQ-FILE-013 | Modify | REQ-FILE-013 and REQ-FILE-015 |
| REQ-FILE-014 | Keep | REQ-FILE-014 |
| REQ-UI-001 | Modify | REQ-UI-001 |
| REQ-UI-002 | Keep | REQ-UI-002 |
| REQ-UI-003 | Modify | REQ-UI-003 |
| REQ-UI-004 | Keep | REQ-UI-004 |
| REQ-UI-005 | Supersede | REQ-UI-009 and REQ-WRITER-003 through REQ-WRITER-005 |
| REQ-UI-006 | Keep | REQ-UI-006 |
| REQ-UI-007 | Modify | REQ-UI-007 |
| REQ-UI-008 | Keep | REQ-UI-008 |
| REQ-API-001 | Modify | REQ-API-001 and REQ-API-005 |
| REQ-API-002 | Modify | REQ-API-002 |
| REQ-API-003 | Modify | REQ-API-003 |
| REQ-API-004 | Modify | REQ-API-004 |
| REQ-DATA-001 | Supersede | REQ-PROJ-005 |
| REQ-DATA-002 | Keep | REQ-DATA-002 |
| REQ-DATA-003 | Supersede | Dolgorae; REQ-RUNTIME-007 |
| REQ-DATA-004 | Modify | REQ-DATA-004 |
| REQ-REC-001 | Supersede | REQ-REC-005, REQ-REC-006, REQ-PROJ-001, REQ-RUNTIME-012 |
| REQ-REC-002 | Modify | REQ-REC-002 |
| REQ-REC-003 | Modify | REQ-REC-003 |
| REQ-REC-004 | Supersede | Dolgorae; REQ-WRITER-006 |
| REQ-REC-005 | Modify | REQ-REC-005 |
| REQ-OBS-001 | Modify | REQ-OBS-001 |
| REQ-OBS-002 | Modify | REQ-OBS-002 |
| REQ-OBS-003 | Modify | REQ-OBS-003 |
| REQ-QA-001 | Modify | REQ-QA-001 |
| REQ-QA-002 | Supersede | REQ-QA-007 and REQ-QA-008 |
| REQ-QA-003 | Modify | REQ-QA-003 |
| REQ-QA-004 | Keep | REQ-QA-004 |
| REQ-QA-005 | Modify | REQ-QA-005 |
| REQ-QA-006 | Modify | REQ-QA-006 |

### 9.1 Retired-to-new family summary

| Retired family/ID | Fresh replacement |
|---|---|
| REQ-SES-* | REQ-DIRECT-* |
| REQ-TURN-* | REQ-DIRECT-*, REQ-WRITER-*, REQ-PROJ-* |
| REQ-LOCK-001..012 | REQ-WRITER-* and REQ-WS-011 |
| REQ-CODEX-* | REQ-RUNTIME-*, REQ-DIRECT-*, REQ-INTERACT-* |
| REQ-INT-* | REQ-INTERACT-* |
| REQ-WS-002/005 | REQ-WS-006..010 |
| REQ-UI-005 | REQ-UI-009 and REQ-WRITER-* |
| REQ-QA-002 | REQ-QA-007/008 |

Draft-only IDs `REQ-RUNTIME-010`, `REQ-PROJ-006..008`, and `REQ-PROJ-010` are intentionally vacated and MUST NOT be reused. Hard links whose inode is also reachable beneath `.dolgorae` cannot be identified portably from an alternate allowed path and are an accepted v0.1 residual limitation governed by REQ-QA-005.

## 10. Global acceptance

v0.1 qualifies only when all non-deferred first-release Tasks and Release-tier requirements pass with phase-appropriate evidence, Recommended exceptions are explicit, and Deferred work is excluded from the completion denominator. All five SOT documents must agree. E2/E9 must verify actual Gul against the released Dolgorae v0.1.3 artifact and matching consumer contract: authentication, Workspace inspection, global Profile/Policy selection, gateway/carrier safety, explicit Orchestrated Session creation, sequential read and threadless write submission, Interaction round trip, ordered original Prompt History over pages/restarts/close, public aggregate/result discovery, verified large artifacts, writer safety, whole-session-close reconciliation, browser/provider restart, headless operation, and contained read-only browsing. No fake-only or producer-only campaign establishes actual Gul acceptance. Missing WriteContinuation/Delete/Podway functionality does not block this release; missing prompt history or required session semantics does.
