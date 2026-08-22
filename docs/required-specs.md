# Gul: Required Specifications

| Field | Value |
|---|---|
| Document role | Normative product and system requirements |
| Authority | Product requirements source of truth |
| Product | Gul (굴) |
| Subtitle | Remote operator interface for local development runtimes |
| Version | 0.1-grpc-interface-aligned |
| Status | Active; E0-T4 and E0-T8 completed, E0-T7 contract inventory and fixtures in progress |
| Last updated | 2026-08-23 |
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

For v0.1, the sole supported Runtime Provider is the **Dolgorae Direct Runtime Provider**. Every Direct Session is backed by exactly one Dolgorae Run. Dolgorae owns Codex App Server and all authoritative Run, thread, Turn, interaction, writer, policy, assurance, and recovery state.

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
| Direct Session | Gul presentation backed by exactly one Dolgorae Run. |
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
| Provider replay envelope | Owner-only bounded canonical request material retained only for crash-safe `StartRun` and `CreateWriteContinuation` exact replay. It contains no capability bytes, protected input, SubmitTurn prompt, or image bytes. |
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
11. Write-capable continuation from `shared_readonly` creates a dedicated successor with a distinct Controller and no initial writer authority.
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
35. Dolgorae timeline is authoritative for runtime conversation history; Gul timeline caches are presentation-only.
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

### 5.1 Host, authentication, and network

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-HOST-001 | Gul MUST provide a Go core, loopback delivery, and persistence that run without Wails and without an LLM runtime. | The core, listener, and persistence start and serve an authenticated remote client with no Wails process present. | E1-T1 |
| REQ-HOST-002 | One frontend bundle MUST serve both authenticated remote clients and the desktop shell. | The bundle is served over the authenticated listener and the shell reuses it with no second bundle or API surface. | E1-T2 |
| REQ-HOST-007 | Gul MUST ship a macOS Wails v3 desktop shell that starts the shared core in-process when absent or attaches to the verified existing `gul serve`, adding no domain authority or second runtime path. | Desktop smoke tests pass against the same API; capability inventory shows the shell adds only WebView delivery and host-native affordances. | E1-T5 |
| REQ-HOST-008 | Gul MUST provide production `gul serve` headless mode that starts the shared core, authenticated HTTPS/ConnectRPC service, Dolgorae supervision, event aggregation, and FileService without a Wails window. | Headless browser E2E passes with no Wails process. | E1-T1 |
| REQ-HOST-009 | Exactly one Gul core per user and data directory MUST own the runtime lock. Gul.app MUST attach to a verified healthy existing `gul serve`; a second headless invocation MUST exit without starting another core or Dolgorae server. | Desktop/headless contention and spoofed-owner tests pass. | E8-T3 |
| REQ-HOST-010 | Headless packaging MUST include user `launchd` operation, graceful upgrade/restart, host sleep/wake recovery, owned-process and protected-log locations, and actionable port-collision behavior. | Login, upgrade, sleep/wake, collision, and log-permission drills pass. | E8-T3 |
| REQ-HOST-003 | Gul APIs MUST use Protobuf-defined ConnectRPC services. | Generated Go and TypeScript clients compile without handwritten feature REST APIs. | E1-T3 |
| REQ-HOST-005 | Gul MUST pin and report its toolchain and target Runtime Provider compatibility ranges. | `toolchain/versions.env` is the single bootstrap authority; `make toolchain-check` reports exact, range, missing, and incompatible host tools without installing or rewriting anything; clean-host and mismatch fixtures pass. | E0-T8 |
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
| REQ-RUNTIME-001 | v0.1 MUST support the Dolgorae Direct Runtime Provider as its sole execution provider. | No alternative execution path or direct App Server path exists. | E2-T1 |
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
| REQ-RUNTIME-022 | The accepted Dolgorae contract's typed Run, Turn, writer, policy, assurance, recovery, lineage, profile, configuration, Interaction, and event projections MUST remain independent decision inputs and MUST NOT be inferred from another aggregate or free-form field. The inspected wire contract is semantically complete; production Interaction Card, event-convergence, and final action-evaluator implementation remain blocked only until Gate B pins one clean regenerated descriptor, client policy, and registry set. | Contract fixtures reject each missing, unknown, stale, stamp-incompatible, or string-only decisive field independently and prove that `effective_access`, `writer_state`, `recovery_status`, `writer_policy_confirmation`, `compatibility`, and `action` strings are never parsed for semantics. | E0-T7 |
| REQ-RUNTIME-023 | Gul v0.1 MUST NOT call or expose `ListProfileDiagnostics`, `SetDefaultEffort`, `ForkRun`, `VerifyRun`, or the Prepare/Commit/Cancel WriterHandoff RPCs. Their presence in the public descriptor MUST NOT block compatibility; future use requires a separate ADR and typed provider-port extension. | API and provider inventories contain no generic passthrough and no route to any intentionally unsupported RPC. | E1-T3 |

### 5.3 Workspace Entries

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-WS-001 | Multiple local Workspace Entries MUST be registrable through either a host-controlled directory picker or bounded server-side browsing restricted to the configured workspace-root allowlist. | Both paths register the same verified canonical root; a browser cannot submit an arbitrary absolute path through either one, and registration is possible from a remote client without host access. | E3-T1 |
| REQ-WS-003 | Every Workspace Entry MUST have a user-visible display name independent of its directory basename. | Rename affects presentation only. | E3-T2 |
| REQ-WS-004 | Multiple provider workspaces and Direct Sessions in different workspaces MUST be usable concurrently. | Concurrent fixtures do not create Gul-owned writer arbitration. | E3-T3 |
| REQ-WS-006 | Runtime calls and FileService MUST use the Workspace Entry's verified canonical root; the provider ID/digest verifies it. | Missing, moved, or mismatched roots fail closed and require reattachment. | E3-T1 |
| REQ-WS-007 | Registration MUST inspect an already initialized Dolgorae workspace by passing the host-controlled absolute path with no expected Runtime Workspace ID, then store the returned verified canonical root and Runtime Workspace ID/digest. Every later revalidation MUST send the stored canonical path and expected ID, and every other workspace-scoped RPC MUST use a `WorkspaceRef` from that verified attachment. | Gul never invokes `init`; bootstrap omits the ID; profile and other workspace reads include `WorkspaceRef`; revalidation and identity-mismatch fixtures pass; neither browser path nor browser workspace ID is authoritative. | E3-T1 |
| REQ-WS-008 | Removing a Workspace Entry presentation MUST NOT close, delete, or otherwise mutate runtime activities. | UI removal and provider deletion remain distinct confirmed operations. | E3-T3 |
| REQ-WS-009 | v0.1 MUST accept only pre-initialized Dolgorae workspaces with at least one provisioned compatible Runtime Profile. | `workspace_not_provisioned` and `profile_missing` blockers are typed and user-visible. | E3-T1 |
| REQ-WS-010 | Canonicalization MUST collapse symlink, `..`, case-alias, and alternate-spelling paths into one verified workspace identity where the host filesystem does so. | Duplicate and move/mismatch tests fail closed or resolve to the same identity. | E3-T1 |
| REQ-WS-011 | Gul MUST NOT create an automatic Git worktree. | Inspection and tests show no worktree-creation operation. | E3-T1 |
| REQ-WS-012 | The workspace-root allowlist MUST be host-configured, canonically resolved at load, and MUST bound every registration browse; no listing, existence answer, or error MUST reveal or resolve a path outside it, and the reserved provider-private subtree MUST stay denied. | Absolute, traversal, symlink-escape, case-alias, and outside-root probes fail closed with an indistinguishable typed error; an empty or unresolvable allowlist disables server-side browsing without disabling the host picker. | E3-T1 |

### 5.4 Direct Sessions and provider references

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-DIRECT-001 | A workspace MUST support multiple Direct Sessions, each mapped one-to-one to a Dolgorae Run ID. | Three independent Runs can be listed and reopened without a Codex thread field. | E3-T3 |
| REQ-DIRECT-002 | Direct Run creation MUST default to `direct_interactive`, `interactive`, `dedicated`, and `best_effort_personal_alpha`. | Outbound semantic request and accepted projection match the defaults. | E2-T3 |
| REQ-DIRECT-003 | Permanent `shared_readonly` creation MUST require an explicit user choice and warning. | UI and API tests prevent accidental selection. | E3-T4 |
| REQ-DIRECT-004 | Submit MUST include Direct Session ID, prompt, an explicit closed `write_intent` enum of `READ` or `WRITE`, optional validated image references, optional effort override, and the persisted idempotency identity required by the accepted operation policy. | Read/write routing and duplicate handling match the provider contract; unknown intent values fail before provider invocation. | E2-T3 |
| REQ-DIRECT-005 | Submit and Interrupt MUST delegate acceptance and lifecycle authority to Dolgorae. | Gul never advances a Turn state independently. | E2-T3 |
| REQ-DIRECT-006 | Pause, Resume, Close, Delete, CreateWriteContinuation, Recover, and Reconcile MUST be exposed only when the provider advertises the corresponding typed capability and Gul's closed action evaluator permits the action. | Presentation archive never invokes a runtime action; unsupported or locally unauthorized actions are absent. | E3-T3 |
| REQ-DIRECT-007 | Rename, favorite, hide/archive, and navigation changes MUST remain Gul-local presentation actions. | Provider state is unchanged. | E3-T2 |
| REQ-DIRECT-008 | Runtime conversation history MUST be reconstructed from the Controller-safe Dolgorae timeline and current Run snapshot and MAY be cached only as a non-authoritative presentation projection. | Open, reconnect, provider restart, and Gul restart merge timeline items by validated identity and chronology without duplication. | E4-T5 |
| REQ-DIRECT-009 | Runtime Profile selection MUST display compatibility, models, lanes, maximum assurance, capability summary, and runtime version. | Selection is blocked when the requested configuration is unsupported. | E3-T4 |
| REQ-DIRECT-010 | A write continuation from `shared_readonly`, or from a dedicated reader whose in-place transition is unavailable or unverified, MUST use `CreateWriteContinuation` to create a distinct threadless dedicated Run, Direct Session, and Controller binding without changing the source. | Source Run and writer remain unchanged; immutable lineage points to the exact current source terminal Turn; no separate Acquire is offered before the first write Turn. | E4-T4 |
| REQ-DIRECT-011 | Each image reference MUST contain Workspace Entry ID, normalized relative path, and a closed detail token. | Absolute, traversal, symlink-escape, oversized, unsupported, and `.dolgorae/**` references fail before provider invocation. | E2-T3 |
| REQ-DIRECT-012 | `runtime_run_id`, `runtime_turn_id`, `interaction_request_id`, `handoff_id`, and `artifact_id` MUST be typed opaque provider references, never Codex-owned authority fields. | Required operation identifiers exist; serialization proves that possessing an ID grants no authorization. | E2-T3 |
| REQ-DIRECT-013 | A write-intent submission rejected for upstream writer conflict MUST produce a distinct typed `writer_busy` presentation. | UI names safe owner/blocker information and never reports write acceptance. | E4-T3 |
| REQ-DIRECT-014 | Where access-policy transition is unavailable or unverified, a Direct Session's access character MUST be fixed after its first Turn and write continuation MUST use `CreateWriteContinuation`; an upstream transition rejection MUST surface as a typed blocker naming that action. | Unsupported and unverified fixtures block in-place write intent, offer only continuation, and preserve the source session; supported existing-thread fixtures retain per-submission intent. | E4-T4 |
| REQ-DIRECT-015 | A profile tool MAY use a trusted external broker to operate a provider-advertised independent subagent Run, but Gul MUST treat only the resulting parent-Turn tool/result projection as Direct Session chat content. Gul MUST NOT create a Direct Session, Runtime Activity, Controller Binding, mutation action, or authoritative lifecycle record for that child. | A brokered-child fixture renders the bounded parent result while Gul persistence/API inventories contain no child Run or credential reference and no child navigation or mutation action appears. | E2-T3 |
| REQ-DIRECT-016 | A threadless dedicated Run with `threadless_acquire_write=false` MUST activate its first writer only through `SubmitTurn(write_intent=WRITE)`; Gul MUST NOT expose or invoke `AcquireWriter` first. | First-write acceptance confirms effective policy and writer state; an explicit threadless Acquire attempt is rejected before provider invocation. | E4-T3 |
| REQ-DIRECT-017 | Before the first `CreateWriteContinuation` RPC Gul MUST persist one idempotency key, source Run, exact current source terminal Turn, destination Controller ID, destination credential key, reason, and normalized request identity. After a lost response it MUST replay the exact same request and use ListRuns plus the unique destination Controller ID only as secondary confirmation; ReconcileRun is valid only after the destination Run ID is known and the provider requires it. | Lost-response fixtures never mint a second credential or key, use no nonexistent lineage lookup, and converge on the provider-returned original destination Run without changing the source. | E5-T3 |
| REQ-DIRECT-018 | Every provider mutation MUST have a persisted local policy defining deadline, cancellation, retry eligibility, ambiguous-response handling, reconciliation, browser pending state, and final projection source. | The operation-policy inventory is exhaustive and transparent gRPC mutation retries are disabled. | E5-T3 |
| REQ-DIRECT-019 | `RunConfigurationProjection` MUST be authoritative for profile, purpose/label, model, current default effort, required capabilities, parent provenance, and instructions identity. Gul MUST refresh Direct Session presentation from it after StartRun, GetRun/snapshot refresh, Gul restart, and Dolgorae restart; the original request and local cache MUST NOT override it. | Reconnect and restart fixtures replace stale local values with the provider configuration and never require the original request as authority. | E3-T3 |
| REQ-DIRECT-020 | Before `StartRun`, Gul MUST persist its idempotency key, Controller ID, credential-store reference, Workspace ID, normalized semantic request identity, and attempt state. A lost response MUST first replay the exact request with the same key, Controller, and carrier; Controller-matched `ListRuns` is secondary, and `GetRun`/`ReconcileRun` apply only after a Run ID is known. | Fault injection returns the original Run and never mints a new key or credential or requires an unknown Run ID. | E5-T3 |

### 5.5 Controller credentials and interactions

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-CTRL-001 | Every Direct Session MUST reference exactly one backend-only Controller Binding to its Run; its capability material MAY be in a typed unhealthy state. | Browser contracts contain health and references, never capability bytes. | E2-T2 |
| REQ-CTRL-002 | Gul-owned Controller capabilities MUST be stored as create-exclusive owner-only files below `~/Library/Application Support/Dolgorae/controller-carriers/gul/<gul-installation-id>/`, outside ordinary plaintext SQLite and every Workspace/FileService path. Parent directories MUST be `0700`, files `0600`, and every relevant component non-symlinked and current-user-owned. | Exclusive-create, wrong-mode/owner/type, symlink, containment, fsync, and replacement-race fixtures fail closed; loss produces a typed blocker and reset remains provider-external. | E2-T2 |
| REQ-CTRL-003 | Controller capability bytes MUST reach Dolgorae only through the accepted protected capability file carrier in an authorized RPC request. Credentials MUST NOT appear in gRPC metadata; the carrier path is backend-only and revalidated immediately before every authorized call. | Bytes and carrier paths never appear in browser payloads, metadata, argv, environment, or stdin; owner, mode, type, symlink, and containment checks precede each call. | E2-T2 |
| REQ-CTRL-004 | Logs, errors, diagnostics, events, prompts, workspaces, cookies, URLs, and browser storage MUST exclude capability bytes and digests. | Redaction and canary-secret tests pass. | E9-T2 |
| REQ-CTRL-005 | Before StartRun or CreateWriteContinuation, Gul MUST locally create and validate a new credential carrier through `DolgoraeControllerCredentialStore`, then supply its derived carrier reference for Dolgorae verification and binding. `CreateControllerCredential` MUST NOT exist in the Runtime Provider port. | Provider inventory contains only `VerifyController`; local-store and provider fakes prove that Dolgorae neither creates nor implicitly invents a Gul credential and the source credential cannot authorize the destination. | E2-T2 |
| REQ-CTRL-006 | Controller credential storage MUST be outside every Workspace root and every path FileService can resolve. | Startup refuses an unsafe store location and path-replacement tests fail closed. | E2-T2 |
| REQ-CTRL-007 | Gul MUST never create, read, store, present, or use Dolgorae Operator capability material or invoke an operator-gated operation. | API, process, storage, and diagnostics inventories contain no Operator carrier or operation. | E9-T2 |
| REQ-CTRL-008 | A missing or invalid Controller capability MUST be a typed blocker that persists until an externally provisioned capability is adopted under REQ-CTRL-009. | No in-product reset is claimed; affected mutations fail closed before verified adoption. | E5-T1 |
| REQ-CTRL-009 | Gul MUST support adopting an externally provisioned Controller capability for an existing Direct Session only through host-controlled selection, local path/owner/mode/type validation, side-effect-free `VerifyController`, and atomic binding replacement followed by a fresh Run including recovery/configuration, Writer status, pending/Controller Interactions, and timeline as needed. | Adoption succeeds for a matching Run and fails closed for arbitrary browser paths, mismatched Runs, malformed carriers, symlinks, and ambiguous verification without an Operator capability. | E2-T2 |
| REQ-CTRL-010 | Gul security policy MUST bind one distinct Controller capability to each Direct Session/Run pair and MUST NOT reuse it across Direct Sessions. | Storage, StartRun, and continuation fixtures prove distinct bindings; the documentation does not claim this as a universal Dolgorae invariant. | E2-T2 |
| REQ-CTRL-011 | Ordinary SQLite MUST store only a logical relative credential key, protected binding reference, public metadata, and health, never credential bytes, an absolute/unvalidated carrier path, or the supervised RPC socket path. Absolute carrier paths MUST be derived under the trusted root and fully revalidated immediately before every authorized RPC. | Schema and canary inspection pass; traversal, alternate-root, symlink, owner, type, and mode replacement fixtures fail closed. | E1-T4 |
| REQ-CTRL-012 | The local credential store MUST emit schema version 1 with a UUIDv7 `controller_id`, `kind=interactive_client`, stable trusted-local Gul installation ID, stable trusted-local account `subject_id`, and 32 `crypto/rand` bytes encoded as unpadded base64url; it MUST use exclusive create with no overwrite, fsync the file and parent, and clear capability buffers where practical. | Exact-schema and negative fixtures verify UUID version, 32-byte decoding, encoding, trusted ID provenance, durability ordering, collision refusal, and secret absence from logs and SQLite. | E2-T2 |
| REQ-CTRL-013 | A continuation credential MUST have a new Controller ID, new random capability, and generation 1 while preserving the source `kind`, `subject_id`, and stable Gul `instance_id` individually. It MUST additionally preserve the normalized principal: `(kind, subject_id)` when subject ID exists, otherwise `(kind, instance_id)`. A matching subject MUST NOT permit a different installation identity. | Same-principal successor fixtures accept only identical kind/subject/instance metadata, reject cross-principal and changed-instance transfer, and preserve the same destination credential identity across retries. | E2-T2 |
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
| REQ-WRITER-008 | The action evaluator MUST return only `CanSubmitRead`, `CanSubmitWrite`, `CanAcquireWriter`, `CanReleaseWriter`, `CanCreateWriteContinuation`, `CanInterrupt`, `CanResolveInteraction`, `CanRecover`, `CanReconcile`, `CanAdoptController`, `RequiresOperatorAction`, `RequiresFreshSnapshot`, `BlockedByOutcomeUnknown`, `BlockedByCredentialState`, `BlockedByBackgroundExecution`, and `BlockedByProviderCompatibility`. `REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION` MUST map to continuation; shared-readonly or unsupported-transition write failures MUST NOT offer another source write. Threadless first write maps to `CanSubmitWrite`, valid existing-thread acquisition to `CanAcquireWriter`, and outcome/recovery states block conflicts. The evaluator's typed inputs remain exhaustive and missing/unknown/string-only values fail closed. | State-matrix tests cover every provider condition and prove every button/endpoint uses the same result; any “Create successor” label aliases only `CanCreateWriteContinuation`. | E4-T3 |

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
| REQ-REC-010 | `RecoverRun` and `ReconcileRun` MUST consume the `RunProjection` in `RunMutationResponse` and perform explicit follow-up reads for Writer, Interaction, timeline, or artifact aggregates when required; they MUST NOT be modeled as atomically returning all projections. | Recovery fixtures prove the immediate response updates only the Run and each other aggregate remains stale until its authoritative read succeeds. | E5-T1 |
| REQ-REC-011 | Gul MUST retain crash-safe canonical request material only for `StartRun` and `CreateWriteContinuation` in an owner-only bounded replay store outside every Workspace. The attempt MUST reference it by logical key and retain role-tagged logical Controller references: StartRun's destination credential-store key/Controller ID, and continuation's source binding/Controller ID plus destination credential-store key/Controller ID. No absolute carrier path or capability may be persisted. Material MUST be deleted after authoritative terminal resolution or the 72-hour maximum retention, with startup and at-least-six-hourly expiry purge. On expiry Gul MUST mark replay unavailable, preserve the non-secret attempt as `OutcomeUnknown`, use only documented secondary reconciliation, and MUST NOT mint a replacement key or silently repeat the mutation. `SubmitTurn` MAY replay only while its exact request remains in process memory; after restart it MUST reconcile without prompt/image replay and preserve `OutcomeUnknown` when acceptance cannot be proved. `ResolveInteraction` protected bytes and tokenless mutations MUST never be automatically replayed. | Crash, restart, retention, canary, role-reference, and lost-response tests prove exact allocation replay, exact source/destination carrier reconstruction, absence of path/capability/prompt/image/secret persistence, no duplicate mutation, and fail-closed unresolved behavior when replay material is unavailable or expired. | E5-T3 |

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
| REQ-FILE-008 | SVG MUST NOT execute active content in the application origin. | The accepted ADR policy blocks script, external resources, and active links; source-only rendering is required until ADR-0018 is accepted. | E6-T2 |
| REQ-FILE-009 | File changes MUST be reflected without a full application reload. | Explicit refresh invalidates affected nodes and previews and satisfies this requirement on its own; the REQ-FILE-016 watcher is the Recommended-tier enhancement over it. | E6-T3 |
| REQ-FILE-010 | File preview MUST remain read-only. | No save, rename, delete, upload, or drag-and-drop mutation exists. | E6-T3 |
| REQ-FILE-011 | FileService MUST expose Git status per file and accessible aggregate status for ancestors. | `MODIFIED`, `ADDED`, `UNTRACKED`, `DELETED`, `RENAMED`, `CONFLICTED`, and `CLEAN`, including combined staged/unstaged state, use color plus a non-color indicator; provider-private paths contribute neither direct nor aggregate status. | E6-T3 |
| REQ-FILE-012 | Markdown MUST support a safe rendered view with contained workspace-relative raster images from the selected fixed revision. | Raw HTML and automatic external loads are blocked; image paths pass all FileService and private-root guards. | E6-T2 |
| REQ-FILE-013 | Changed text, Markdown, and supported raster images MUST support fixed `HEAD` versus Working comparison. | Desktop uses side-by-side and small screens use an explicit revision switch; added, deleted, renamed, and missing-asset cases are represented without fabrication. | E6-T3 |
| REQ-FILE-014 | Browsing MUST remain usable when Git comparison is unavailable. | Non-Git, unborn `HEAD`, and unavailable Git return typed degradation without breaking current preview. | E6-T3 |
| REQ-FILE-015 | `.dolgorae` MUST appear only as one non-expandable provider-managed denied node; denial MUST be evaluated on the fully resolved real path after symlink resolution and before any read, listing, status, or provider hand-off. | Descendants and symlink or nested-symlink aliases resolving into them are unavailable to listing, preview, Markdown assets, Submit images, Git status/diff, and all ancestor aggregates. | E6-T1 |
| REQ-FILE-016 | File invalidation MUST use a bounded host filesystem watcher scoped to the verified canonical root and MUST NOT depend on any upstream event projection. | The watcher is bounded in queue depth, coalescing interval, and watched-node count; `.dolgorae/**` events are discarded at the resolved path; FileService invalidation works while the provider is unavailable. | E6-T3 |
| REQ-UI-001 | Desktop and wide-tablet layout MUST use workspace/activity, conversation, and file panes. | Responsive layout tests pass. | E7-T1 |
| REQ-UI-002 | Small screens MUST use Sessions, Chat, and Files top-level navigation. | iPhone tests preserve context. | E7-T1 |
| REQ-UI-003 | Navigation MUST display provider connectivity, activity, writer, policy, assurance, and interaction state without implying local authority. | State combinations remain unambiguous. | E7-T2 |
| REQ-UI-004 | The file pane MUST switch between explorer and preview while retaining navigation context. | Back navigation restores directory and selection. | E7-T1 |
| REQ-UI-006 | Interaction cards MUST take visual priority over passive progress. | Required action is reachable without hidden scrolling on supported mobile layouts. | E7-T2 |
| REQ-UI-007 | The PWA MUST preserve safe presentation navigation across ordinary refresh. | Runtime state is revalidated and stale projection is never promoted. | E7-T1 |
| REQ-UI-008 | Primary flows MUST be keyboard accessible and use semantic controls. | Automated and manual accessibility checks pass. | E7-T3 |
| REQ-UI-009 | Typed blockers including unprovisioned workspace, missing profile, lost Controller, denied provider root, and incompatible provider MUST be presented distinctly. | Every blocker has a safe explanation and allowed next action. | E7-T2 |
| REQ-UI-010 | Blockers requiring action outside Gul, including provider-side migration, an unavailable or unverifiable provider server, and Controller reset, MUST be a distinct operator-action-required class naming the external procedure, and unresolved-outcome states MUST be visually distinct from ordinary errors. | Fixtures for each condition present no in-product retry, name the documented external step, and never render as a transient failure. | E7-T2 |

### 5.9 APIs, diagnostics, quality, and release

| ID | Requirement | Acceptance | Owner |
|---|---|---|---|
| REQ-API-001 | Application services MUST be declared in Protobuf and implemented with ConnectRPC. | Auth, Runtime, WorkspacePresentation, DirectSession, InteractionPresentation, WriterAction, File, ClientEvent, and Diagnostics clients are generated and versioned. | E1-T3 |
| REQ-API-002 | Retryable state-changing operations MUST use unary RPCs with typed provider-aware concurrency semantics. | Browser retries never create duplicate provider operations. | E1-T3 |
| REQ-API-003 | Client updates MUST use server-streaming RPCs with a Gul delivery sequence distinct from upstream cursors. | Replay and snapshot fallback converge in order. | E4-T1 |
| REQ-API-004 | Unknown, expired, stale, unauthorized, conflict, unavailable, and recovery-required conditions MUST have stable Gul error codes. | Clients never parse human text or raw provider errors. | E1-T3 |
| REQ-API-005 | Public APIs MUST NOT expose raw provider envelopes, capability material, unrestricted absolute paths, unvalidated or unauthorized artifact content, or arbitrary Git revisions. | Contract scans and negative serialization tests pass. | E1-T3 |
| REQ-API-006 | Every provider outcome MUST map to a stable Gul error code plus a closed provider action class; an unmapped provider error code or required-action enum MUST fail closed as a typed blocker rather than a generic failure. | The mapping is generated from the accepted upstream error/action contract, coverage fixtures fail on an unmapped value, and clients never parse provider message or action text. | E1-T3 |
| REQ-API-007 | The public domain error set MUST distinguish `TransportUnavailable`, `DeadlineExceeded`, `ProtocolIncompatible`, `ControllerMismatch`, `ControllerCarrierInvalid`, `WriteContinuationControllerInvalid`, `WriterConflict`, `ThreadlessRequiresWriteTurn`, `InteractionStale`, `InteractionAlreadyResolved`, `RecoveryRequired`, `OutcomeUnknown`, `SlowConsumer`, `ArtifactUnavailable`, `UnsupportedPathEncoding`, `RunStateConflict`, `RpcServerAlreadyRunning`, and `OperatorActionRequired`. | Generated ConnectRPC contracts and mapping tests cover every value and expose no human-message or action-string parsing. | E1-T3 |
| REQ-API-008 | Browser contracts MUST expose neither Dolgorae socket/carrier paths nor private worker IDs, raw Controller credentials, App Server transport details, raw upstream events, or generated upstream messages. | Descriptor inspection and negative serialization fixtures prove absence. | E1-T3 |
| REQ-OBS-001 | Structured logs MUST use Gul/provider/runtime-reference/correlation fields and exclude secrets and hidden content. | Canary-secret and bounded-output tests pass. | E9-T2 |
| REQ-OBS-002 | Diagnostic output MUST be bounded and redacted. | Oversized provider output, secret canaries, prompts, and local sensitive paths remain absent. | E9-T2 |
| REQ-OBS-003 | The user MUST be able to view a diagnostic summary reporting Gul/toolchain/database/Tailscale/provider version, compatibility, capabilities, reachability, checkpoint state, safe errors, and Controller binding health. | No App Server process ownership, capability material, or Operator-gated diagnostic operation is present. | E9-T2 |
| REQ-QA-001 | Core presentation, path, projection, cursor, and adapter behavior MUST have deterministic unit tests. | Tests require no real LLM. | E9-T1 |
| REQ-QA-003 | Browser E2E MUST cover auth, inspection, Direct Session, read/write submit, final response, interaction, writer actions, successor, reconnect, and files. | Supported flow matrix passes. | E9-T3 |
| REQ-QA-004 | Manual checks MUST cover Wails, macOS browser, iPhone Safari/PWA, and iPad Safari/PWA. | Results and suspension limitations are recorded. | E9-T3 |
| REQ-QA-005 | Release docs MUST cover installation, Dolgorae compatibility, Tailscale Serve, backup, fail-closed Controller loss, external terminal reset followed by verified adoption, and the prohibition on hard links into `.dolgorae`. | Clean-host and credential-loss documentation dry runs succeed without claiming in-product reset and deployment guidance covers the hard-link residual. | E9-T3 |
| REQ-QA-006 | Release acceptance MUST require every Release-tier requirement to be in Current State with evidence, every Recommended-tier requirement to be promoted or explicitly recorded as accepted-incomplete, and every Roadmap Task to be `Completed`. | Qualification fails while any Release-tier item remains unpromoted, while a Recommended-tier item has no recorded disposition, or while any Task is not `Completed`. | E9-T3 |
| REQ-QA-007 | Dolgorae integration MUST have a fake public gRPC server plus opt-in smoke tests against an accepted compatible executable, and a separate exact-schema Machine CLI conformance harness. | Private interfaces are never used; production DI contains no CLI fallback; live smoke tests remain blocked until E2-T0 completes. | E9-T1 |
| REQ-QA-008 | Provider conformance MUST map every port/API operation, required identifier, enum, error/action detail, projection, event form, and carrier schema to the accepted public contract. | Checked inventories and positive/negative fixtures cover every mapping and are regenerated from the accepted descriptor/schema digests. | E9-T1 |
| REQ-QA-009 | Security tests MUST cover `.dolgorae/**` across every surface, Controller-capability and protected-interaction-input canaries across process/browser/storage paths, and Controller-loss recovery without Operator possession. | All escape and recovery drills fail closed. | E9-T2 |
| REQ-QA-010 | Integration qualification MUST cover the canonical local-gRPC matrix, including every non-conflicting prior scenario and every numbered scenario in the 2026-08-19 corrective revision: local credential ownership/schema, Workspace bootstrap, socket ownership, typed projection blocking, exact continuation replay, protected-input recovery, event schema modes, error additions, and no CLI fallback. | Every scenario is individually traceable to an automated or explicitly manual owner and recorded evidence; none is silently omitted. | E9-T1 |

## 6. Deferred State ledger

Deferred requirements do not participate in v0.1 acceptance unless explicitly promoted through an ADR and Roadmap change.

| ID | Deferred requirement | Owner |
|---|---|---|
| REQ-GORAE-001 | A future Gorae Provider remains a trusted built-in out-of-process adapter. | Deferred-Gorae |
| REQ-GORAE-002 | Gul calls only Gorae public operations and never mutates Gorae-owned Dolgorae Runs directly. | Deferred-Gorae |
| REQ-GORAE-003 | Managed interactions expose only Gorae-defined summaries or escalations; Gul never resolves low-level Dolgorae interactions. | Deferred-Gorae |
| REQ-WRITER-004 | Same-controller handoff MAY be offered only when Dolgorae reports eligibility and MUST use its prepare/commit/cancel protocol. | Deferred-Handoff |

REQ-WRITER-004 retains its original ID and meaning and is deferred, not superseded. Under ADR-0035 Gul policy binds a distinct Controller to every Direct Session/Run pair, so no two Gul-owned Runs share a Controller and the same-controller handoff precondition is unreachable in v0.1. Reactivation requires an accepted Controller-scope change. `Deferred-Handoff` is a deferred owner label, not a Roadmap task.

## 7. Current State ledger

No Gul product behavior has completed implementation review. The Current State ledger is empty.

## 8. Explicit v0.1 non-goals and limitations

Gul v0.1 does not provide an LLM; direct App Server integration; Codex thread/Turn authority; a local writer lock; background-process containment; Gul-owned independent-agent orchestration; Gorae Mission/Task/Workflow semantics; arbitrary plugins; a marketplace; a generic multi-provider LLM abstraction; public internet exposure; multi-user accounts; native mobile apps; automatic Git worktrees; browser file editing; a browser PTY; reasoning display; atomic writer handoff between Direct Sessions; a Gul-side writer queue or reservation; or parallel writers in one Workspace. A profile tool's brokered child remains external to Gul under REQ-DIRECT-015.

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

v0.1 is acceptable only when every Release-tier requirement is promoted with evidence, every Recommended-tier requirement is promoted or explicitly recorded as accepted-incomplete, all Deferred State items remain explicitly deferred or are separately approved, every Roadmap Task is `Completed` so no task remains in `Planned`, `In Progress`, `In Review`, or `Blocked`, all five SOT documents agree, an accepted Dolgorae executable and public gRPC contract are pinned, projection convergence and replay-material handling pass review, Controller handling passes review, supported-device tests pass, and a clean host completes authentication, pre-provisioned workspace inspection, supervised RPC startup, Direct Run creation, read submission, threadless first-write submission without Acquire, one interaction, final-response and large-artifact display, timeline reconstruction, writer presentation, existing-thread Release followed by eligible Acquire in a second Direct Session, write continuation, gateway/Gul/browser reconnect, headless service operation, and secure read-only file browsing without any production CLI fallback.
