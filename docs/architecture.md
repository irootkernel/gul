# Gul: Architecture

| Field | Value |
|---|---|
| Role | Target and Current Architecture source of truth |
| Product | Gul |
| Version | 0.1-dolgorae-consumer-v1 |
| Status | Approved target rebaseline; current implementation through E7 and E8-T1 within their declared component scope |
| Last updated | 2026-09-30 |

## 1. Purpose and change control

This document translates `required-specs.md` and accepted ADRs into component, trust, data, API, event, recovery, and deployment boundaries. Required architecture is not implementation evidence. Part B remains the only Current Architecture statement.

The canonical producer contract is Dolgorae `docs/specs/gul-consumer-v1.md`, ID `dolgorae.gul-consumer/v1`. E12-T1 pins immutable TASK-053 commit `21aefe5b2a8dc6fb18a58338090348b23d2f0a4a`. Its first-release profile requires 27 methods, including full timeline and two read-only aggregate queries, from a 36-method descriptor; nine descriptor methods remain unavailable to first-release Gul actions. The checked consumer boundary is locked by dependency-lock SHA-256 `8f52ae66e126f37013d7842b2113fc509d21af4e4fc465cecdbeef7e21619f01` and current generated-lock SHA-256 `96da1b0a5caeffac1bc9d387c4c4e4ee4159daf47e3877ed748d291be9bb5bc0` after the Go 1.27.1 toolchain rebaseline. E12-T1's original generated-lock digest was `6284064e720e2220d6960c42faef6a4c13292ce1327f44e00388dc52b2e17d4a`. This is contract/tooling evidence only, not product runtime or live-provider acceptance. Continuation and Delete remain optional future functions.

# Part A. Target Architecture

## 2. Architectural goals

Gul is LLM-free. It provides one responsive local/remote interface for trusted local development runtimes; keeps the Go core independent from Wails; delegates all Codex runtime authority to Dolgorae; protects backend runtime capabilities; survives browser and provider reconnects without duplicate operations; and provides secure read-only workspace inspection without requiring an LLM Turn.

## 3. System topology and authority

```text
┌──────────────────── Gul clients ─────────────────────┐
│ Wails WebView │ Desktop Browser │ iPad PWA │ iPhone PWA │
└────────────────────────┬─────────────────────────────────┘
                         │ HTTPS + ConnectRPC
                         ▼
┌──────────────────── macOS host ──────────────────────────┐
│ Gul Wails application                                │
│                                                         │
│ Authentication │ presentation registry │ client events  │
│ Runtime Provider adapters │ credential store            │
│ read-only FileService │ diagnostics                     │
│                                                         │
│       ┌───────────────────┐                             │
│       │ Dolgorae Provider │                             │
│       └─────────┬─────────┘                             │
│                 │ gRPC, private Unix socket             │
│                 │ unary + per-Run server streams         │
│                 ▼                                       │
│       supervised Dolgorae public RPC server             │
│                 │                                       │
│                 ▼                                       │
│          Codex App Server                               │
│                                                         │
│       Future: Gorae Provider → Gorae                    │
└─────────────────────────────────────────────────────────┘
```

Gul supervises only the public Dolgorae RPC gateway process. It never crosses that boundary to use App Server stdio, sockets, JSON-RPC, schema generation, process supervision, worker sockets, internal state, or audit files. Restarting the gateway does not imply that durable Runs were destroyed. For a future Gorae-managed activity, Gul calls Gorae only; Gorae alone may operate its internal Dolgorae Runs.

A Gul Orchestrated Session owns presentation around one Primary Run binding.
Dolgorae's Broker owns Specialists and their credentials. Gul may display public
child Run observations, but obtains actual aggregate state from the authorized
GetOrchestratedSession query and permitted result references from
ListOrchestratedSessionResults. Parent provenance supports navigation only;
Gul never derives membership authority, creates a child Controller, or mutates
a child. Dolgorae orchestration is not deferred to a separate Gorae provider.

## 4. Layered component model

```text
Delivery
  Wails bindings, ConnectRPC handlers, authentication middleware,
  client-event streaming, frontend delivery

Application
  RuntimeService, WorkspacePresentationService, DirectSessionService,
  InteractionPresentationService, WriterActionService, FileService,
  ClientEventService, DiagnosticsService

Domain and ports
  RuntimeAttachment, WorkspaceEntry, DirectSession, RuntimeActivity,
  RuntimeProjection, ProviderReferences, ObservationCheckpoint,
  ControllerBindingReference, ProjectionConvergenceState,
  provider capability ports, credential-store port, replay-material port,
  clock, idempotency, and delivery ports

Infrastructure
  supervised Dolgorae gRPC adapter, diagnostic Machine CLI adapter,
  protected credential store, protected bounded replay store, SQLite repositories,
  filesystem guard, bounded Git adapter, Tailscale inspection, structured logs
```

Dependencies point inward. Provider adapters implement typed capability interfaces; they do not inject provider-specific wire payloads into browser or domain models. No unbounded `Execute(action, json)` interface is permitted.

## 5. Runtime Provider port

The runtime-neutral port expresses semantic responsibilities rather than CLI commands, REST endpoints, or generated RPC services:

```text
RuntimeCapability
  GetCapabilities
  InspectWorkspace
  ListProfiles, GetProfile

DirectRunCapability
  StartRun, ListRuns, GetRun, SubmitTurn, InterruptTurn
  PauseRun, ResumeRun, CloseRun

ObservationCapability
  WatchRunEvents, RefreshRunSnapshot

ControllerCapability
  VerifyController

ControllerInteractionCapability
  ListPendingInteractions, GetControllerInteraction, ResolveInteraction

WriterCapability
  GetWorkspaceWriterStatus, AcquireWriter, ReleaseWriter

RecoveryCapability
  RecoverRun, ReconcileRun

ArtifactCapability
  GetArtifact, ReadArtifactChunk

TimelineCapability
  ListRunTimelineItems

OrchestrationObservationCapability
  GetOrchestratedSession, ListOrchestratedSessionResults
```

Generated Protobuf messages exist only inside the adapter. Each accepted-version decoder performs strict message, identifier, enum, and typed-error validation and maps into Gul domain types. No generic `Execute(action, json)`, stdout envelope, exit-status semantic channel, or process-shaped event follower exists in the production port.

`RefreshRunSnapshot` is the semantic adapter operation for the accepted `RunService.GetRun` RPC; semantic names never imply an additional upstream endpoint. Observation remains available without mutation authority where the negotiated contract permits it. Controller-sensitive Interaction detail, Controller verification, and authorized mutations use their explicit capabilities. Artifact, complete Timeline, and the two Orchestration observation methods are required by the v0.1 consumer profile. Recovery operations appear only when advertised. DeleteRun and CreateWriteContinuation remain upstream future capabilities, not first-release Gul ports or routes.

Controller credential creation is not a Runtime Provider operation. Gul owns a local port:

```text
DolgoraeControllerCredentialStore
  Create
  Validate
  ResolveCarrierReference
  RemoveUnused
```

The store emits the accepted Dolgorae credential schema, while Dolgorae remains authoritative for credential meaning, binding, and authorization and verifies a supplied carrier before use.

Crash-safe exact replay for allocation operations is not a Runtime Provider responsibility. Gul owns a second local port:

```text
ProviderReplayStore
  PutCanonicalRequest
  GetCanonicalRequest
  DeleteResolved
  PurgeExpired
```

The first-release replay store retains bounded non-secret canonical StartRun material only, with a logical credential key and expected Controller ID. It excludes capability bytes, protected Interaction input, SubmitTurn prompts, image bytes and socket/carrier paths. Same-principal successor creation and continuation replay are deferred with E4-T4, not required local-store operations.

## 6. Dolgorae Provider

### 6.1 Supervised server lifecycle

The configured Dolgorae executable is resolved and identity-checked before use. Gul acquires its user/data-directory singleton lock, creates and validates the current-user-owned `0700` parent below `~/Library/Caches/Gul/runtime/`, chooses an unused absolute non-symlink socket pathname outside every Workspace, and starts the public RPC server without a shell. Dolgorae owns its gateway singleton record/lock, socket bind and chmod, stale-socket proof and unlink, and graceful socket cleanup. Gul never creates, chmods, or unlinks the socket node itself. The socket is never selected by a browser, stored in ordinary SQLite, exposed through Tailscale Serve, or replaced by TCP.

Startup has a 15-second overall deadline containing a 5-second readiness handshake. Readiness is semantic: after Dolgorae binds the socket, Gul verifies the resulting node's owner, socket type, restrictive mode, and non-symlink path; the channel connects; the server declares its API version and capabilities; and Gul accepts only a pinned supported version with all required capabilities. A collision or a reported active gateway fails closed. Without an accepted attach contract Gul does not connect to, replace, signal, or take over that gateway and maps the result to `RpcServerAlreadyRunning`.

After handshake Gul establishes one reusable channel, restores Direct Session bindings, refreshes Run, Writer, Interaction, Recovery, and timeline state, resumes event streams, then enables mutations. A replaced binary does not invalidate the already verified running child, but every later child start re-verifies the new file before execution.

On crash, restart delay is 1, 2, 4, 8, then 16 seconds with ±20 percent jitter and a 30-second cap. At most five starts are allowed per rolling 60 seconds; five stable minutes reset the budget. Protocol incompatibility and restart exhaustion stop retry. Runs remain durable upstream. Projections become stale and in-flight mutations become outcome-unknown until reconciliation.

Shutdown stops accepting mutations, cancels Run streams, drains unary operations for at most five seconds, closes the channel, requests graceful termination of the owned RPC child, and uses the remainder of the 10-second total budget before terminating that exact PID. Dolgorae performs socket cleanup; Gul only verifies post-exit state and reports an unsafe or stale result without unlinking it.

### 6.2 Compatibility and schema policy

The handshake selects an explicitly supported public API version and verifies the required Runtime, Run, Observation, Controller verification/interaction, Writer, Recovery where used, Artifact, Timeline, and OrchestrationObservation capabilities using the pinned consumer profile, not equality with the server's entire supported-method set. Generated decoders for that version are necessary but not sufficient evidence of semantic compatibility. Unknown required enum values, missing typed error details, and missing capabilities are blockers. Unknown optional data is retained only when the accepted version policy permits it; otherwise it is discarded at the adapter boundary and never persisted or forwarded.

Historical E0 evidence: the old Dolgorae contract at revision `85a8862f784cc57701751d81a9e03bf7c5722818` supplied its then-accepted service/method inventory and the typed Run, Turn, writer, policy, assurance, recovery, lineage, profile, event, Controller Interaction, Run configuration, required-action, Interaction-limit, path, timeline, and artifact projections required by Gul. Gate A is closed: its checked Protobuf source, descriptor, capabilities, mutation policy, error/action mapping, client policy, and conformance artifacts are internally consistent. The descriptor reproduces byte-for-byte with protoc 35.1 and the pinned Protobuf v32.1 `timestamp.proto` source-info input. Gate B pins the same source through dependency-lock SHA-256 `c4f91aa3e2add1093880684e5c96fdbb6239aef6a85261a0adf6b585e2db8863` and generated-lock SHA-256 `8a6a614a3a08c585f9a62f74095a0237d47feefba802e2dfa3ce13be5fbe0bf6`; the checked generator reproduces the descriptor, typed clients, exhaustive inventory/maps, policy fixtures, and descriptor-derived fake server.

Every decision input remains independent even when another projection appears to imply it. Gul never parses `effective_access`, `writer_state`, `recovery_status`, `writer_policy_confirmation`, `compatibility`, or `action` strings to reconstruct typed semantics. New Interaction Card mapping and action evaluation require E12-T1's TASK-053 contract pin; historical Gate B does not supply the new consumer contract. Gul never infers compatibility from the Codex binary, and the Machine CLI keeps a separate exact closed-schema conformance contract that cannot be selected by production dependency injection.

### 6.3 RPC use and mutation policy

Commands and snapshots are unary; Run events are server streaming; artifacts use the unary `ArtifactService.ReadArtifactChunk` RPC. There is no client-streaming or bidirectional command bus. Stream health is not authorization, and event backpressure is isolated from unary requests.

Transparent gRPC retries, retry middleware, and hedging are disabled for every mutation. Before transmission Gul persists a non-secret `ProviderOperationAttempt` carrying operation kind, target reference, idempotency identity where defined, deadline, state, reconciliation route, normalized-request digest, replay availability, and an optional logical replay-material reference. First-release deadlines are: StartRun 30 seconds; SubmitTurn and ResolveInteraction 20; Interrupt/Pause/Resume/Close 10; Acquire/Release 15; Recover/Reconcile 60. Deferred operations have no active route or deadline policy. Local credential and replay-material operations are not RPCs and use bounded protected-filesystem policies. Cancellation or a lost response does not prove failure.

| Semantic mutation | Deadline | Retry eligibility and reconciliation |
|---|---:|---|
| StartRun | 30s | Primary recovery is application-level replay of the exact canonical request from the protected replay store with the persisted key, Controller identity, and carrier. `ListRuns` matched by Controller ID is secondary; `GetRun`/`ReconcileRun` apply only after a Run ID is known. |
| SubmitTurn | 20s | No transparent retry. Exact replay is allowed only while the original normalized request bytes remain available in the same process and the accepted mutation policy permits it. After process restart Gul reconciles through `GetRun` and timeline; if acceptance cannot be proved or disproved, the operation remains `OutcomeUnknown` and is not automatically resubmitted. |
| InterruptTurn, PauseRun, ResumeRun | 10s | No automatic replay; reconcile Primary state through `GetRun`. |
| CloseRun | 10s | Whole-session close through the root; no automatic replay. Reconcile aggregate close intent through GetOrchestratedSession plus required Run/Writer/Interaction reads. |
| ResolveInteraction | 20s | Persist the Interaction ID, key, attempt state, and non-secret metadata, never the response body. Exclude the RPC from automatic retry; refresh pending/full Interaction state and require the protected value again only if it remains unresolved. |
| AcquireWriter, ReleaseWriter | 15s | No automatic replay; reconcile through `GetWorkspaceWriterStatus` plus `GetRun` when policy-derived actions are affected. |
| RecoverRun, ReconcileRun | 60s | No transparent replay. Root recovery accounts for retained aggregate-close intent; use returned Run plus fresh Session/Writer/Interaction/timeline reads as required. Do not interpret Primary recovery as proof that all owned work is settled. |

Every row also defines context cancellation, a browser-visible pending state before transmission, `OutcomeUnknown` after an ambiguous response, conflict blocking, and a final projection supplied only by a validated event, snapshot, timeline, or reconciliation result.

`StartRun` is the only first-release operation whose complete replay material is retained across a Gul restart. Before the first call, Gul canonicalizes the accepted semantic request, stores it in an exclusive owner-only replay file below `~/Library/Application Support/Gul/provider-replay/`, fsyncs the file and parent, stores its SHA-256 plus logical reference in `ProviderOperationAttempt`, and persists the operation attempt before network transmission. The canonical material includes every field in the provider idempotency identity but excludes Controller capability bytes and absolute carrier paths. It records the destination credential-store key and expected Controller ID. The fixed v0.1 maximum retention is 72 hours and host configuration may only shorten it. Gul purges expired envelopes at startup and at least once every six hours. A terminal authoritative result deletes the envelope immediately; expiry deletes only canonical request material, marks `replay_availability = expired`, preserves the non-secret attempt as `OutcomeUnknown`, attempts only documented secondary authoritative reconciliation, and never mints a new key or silently repeats the mutation. Deferred E4-T4 retains separate exact-replay requirements for a future `CreateWriteContinuation` implementation.

`SubmitTurn` deliberately has a different privacy and replay policy. Its prompt, ordered image material, and Turn body are not durably retained by the replay store. Same-process application replay may reuse the in-memory original request and the same key. The first-release in-memory cache holds at most four exact requests for at most ten minutes, clearing evicted or expired bytes without changing unresolved attempt state. After a Gul crash or restart, Gul first reconciles from `GetRun` and the provider timeline. If provider evidence does not prove whether the Turn was accepted, the attempt stays `OutcomeUnknown`, conflicting mutations remain blocked, and a new submit requires an explicit user action after non-acceptance is established. `ResolveInteraction` is stricter still: its protected body is never retained or replayed under any circumstance.

A lost response never mints a new idempotency key, credential, or Controller for the same semantic attempt. Tokenless mutations are never replayed merely because transport failed. An unresolved operation remains browser-visible and blocks conflicts until authoritative provider evidence supplies its final state.

### 6.4 Provider operation map

This inventory contains one row per consumer RPC semantic operation plus the Gul-owned adoption, action-evaluation, and delivery operations, with the exact owner, prerequisites, retry and reconciliation policy, projection effect, capability requirement, and verification state. It is the checked E12 consumer generation input. Credential-store lifecycle details remain in Section 8 and are not descriptor-facing consumer operations. `RefreshRunSnapshot` is a Gul semantic alias for `RunService.GetRun`, not an upstream method.

`Bounded read` is a Gul-owned deadline/retry class pinned under REQ-RUNTIME-015; it never authorizes a mutation retry. `Session Controller` means a currently valid carrier bound to the owning Direct Session and revalidated immediately before the call. Every RPC requires membership in `GetCapabilitiesResponse.supported_methods`; the capability column adds a typed feature requirement only where the public contract defines one.

#### 6.4.1 Checked E12 consumer operation map

This table records the immutable TASK-053 consumer profile pinned by E12-T1 and is the generator input. It describes the checked contract boundary, not product runtime implementation or live-provider acceptance.

<!-- contract-operation-map:start -->
| Gul semantic operation | Dolgorae RPC or local/Gul owner | Exact wire contract | Required Controller state | Required workspace state | Idempotency identity | Timeout class | Retry policy | Lost-response reconciliation | Projection effect | Capability requirement | Verification state |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `GetCapabilities` | `RuntimeService.GetCapabilities` | `GetCapabilitiesRequest` → `GetCapabilitiesResponse` | None | None | Read | Handshake read | Bounded repeat | Repeat with protocol zero and the same client range | Replace protocol/capability projection | Method is handshake authority | E12 pinned contract; not runtime/live evidence |
| `InspectWorkspace` | `RuntimeService.InspectWorkspace` | `InspectWorkspaceRequest` → `InspectWorkspaceResponse` | None | Host path; absent expected ID on attach, stored ID on revalidation | Read | Bounded read | Bounded repeat | Repeat same path/expected-ID mode | Replace workspace identity/inspection; no profile projection | Supported method | E12 pinned contract; not runtime/live evidence |
| `ListProfiles` | `RuntimeService.ListProfiles` | `ListProfilesRequest` → `ListProfilesResponse` | None | None; user-global Profile registry | Read | Bounded read | Bounded repeat | Repeat read | Replace profile summaries | Supported method | E12 pinned contract; not runtime/live evidence |
| `GetProfile` | `RuntimeService.GetProfile` | `GetProfileRequest` → `GetProfileResponse` | None | Global profile name; no `WorkspaceRef` | Read | Bounded read | Bounded repeat | Repeat read | Replace selected profile projection | Supported method | E12 pinned contract; not runtime/live evidence |
| `StartRun` | `RunService.StartRun` | `StartRunRequest` → `StartRunResponse` | Validated new carrier | Required `WorkspaceRef`; compatible profile | Required key + Controller + normalized request | 30s | No transparent retry; exact replay | Exact replay first; Controller-matched `ListRuns` second; `GetRun`/`ReconcileRun` after ID | Replace Run, binding, and configuration | `persistent_runs`, `controller_binding` | E12 pinned contract; not runtime/live evidence |
| `ListRuns` | `RunService.ListRuns` | `ListRunsRequest` → `ListRunsResponse` | None | Required `WorkspaceRef` | Read | Bounded read | Bounded repeat | Repeat read | Replace Run summaries | `persistent_runs` | E12 pinned contract; not runtime/live evidence |
| `GetRun` | `RunService.GetRun` | `GetRunRequest` → `GetRunResponse` | None | Required `RunRef`/`WorkspaceRef` | Read | Bounded read | Bounded repeat | Repeat read | Replace authoritative Run snapshot and configuration | `persistent_runs` | E12 pinned contract; not runtime/live evidence |
| `RefreshRunSnapshot` | `RunService.GetRun` | `GetRunRequest` → `GetRunResponse` | None | Required `RunRef`/`WorkspaceRef` | Read | Bounded read | Bounded repeat | Repeat read; repair cursor uncertainty | Same as `GetRun` | `persistent_runs` | E12 pinned contract; not runtime/live evidence |
| `SubmitTurn` | `RunService.SubmitTurn` | `SubmitTurnRequest` → `SubmitTurnAccepted` | Session Controller | Required `RunRef`/`WorkspaceRef` | Required key + normalized Turn request | 20s | No transparent retry; same-process replay only when original request bytes remain available | `GetRun` and timeline; after restart preserve `OutcomeUnknown` unless acceptance/non-acceptance is proved | Replace accepted Turn, Run, and Writer projections | `persistent_runs`; write uses writer features | E12 pinned contract; not runtime/live evidence |
| `InterruptTurn` | `RunService.InterruptTurn` | `InterruptTurnRequest` → `RunMutationResponse` | Session Controller | Required `RunRef`/`WorkspaceRef` | Tokenless expected revision | 10s | No automatic replay | `GetRun`; reconcile if outcome unknown | Replace Run projection | Supported method | E12 pinned contract; not runtime/live evidence |
| `PauseRun` | `RunService.PauseRun` | `PauseRunRequest` → `RunMutationResponse` | Session Controller | Required `RunRef`/`WorkspaceRef` | Tokenless expected revision | 10s | No automatic replay | `GetRun`; reconcile if required | Replace Run projection | Supported method | E12 pinned contract; not runtime/live evidence |
| `ResumeRun` | `RunService.ResumeRun` | `ResumeRunRequest` → `RunMutationResponse` | Session Controller | Required `RunRef`/`WorkspaceRef` | Tokenless expected revision | 10s | No automatic replay | `GetRun`; reconcile if required | Replace Run projection | Supported method | E12 pinned contract; not runtime/live evidence |
| `CloseRun` | `RunService.CloseRun` | `CloseRunRequest` → `RunMutationResponse` | Primary Session Controller | Primary `RunRef`/`WorkspaceRef` | Tokenless expected revision; explicit interrupt consent | 10s bounded call | No automatic replay | `GetOrchestratedSession` plus fresh Run/Writer/Interaction; unknown blocks success | Closing intent then confirmed whole-aggregate closure | Required consumer method | E12 pinned contract; not runtime/live evidence |
| `RecoverRun` | `RunService.RecoverRun` | `RecoverRunRequest` → `RunMutationResponse` | Session Controller | Required root `RunRef`/`WorkspaceRef` | Tokenless expected revision | 60s | No automatic replay | Fresh Run/Session/Writer/Interaction as required | Root recovery accounts for retained close intent; other projections remain separate | Supported method | E12 pinned contract; not runtime/live evidence |
| `ReconcileRun` | `RunService.ReconcileRun` | `ReconcileRunRequest` → `RunMutationResponse` | Session Controller | Required root `RunRef`/`WorkspaceRef` | Tokenless expected revision | 60s | No automatic replay | Fresh Run/Session before another reconcile | Resolve supported aggregate-close uncertainty without semantic replay | Supported method | E12 pinned contract; not runtime/live evidence |
| `WatchRunEvents` | `ObservationService.WatchRunEvents` | `WatchRunEventsRequest` → stream `RunEventEnvelope` | None | Required `RunRef`/`WorkspaceRef` | Exclusive `after_cursor` | Long-lived stream | Reason-specific reconnect | Snapshot repair, then last committed cursor | Variant-specific update/invalidate rules below | `event_replay` | E12 pinned contract; not runtime/live evidence |
| `ListRunTimelineItems` | `ObservationService.ListRunTimelineItems` | `ListRunTimelineItemsRequest` → `ListRunTimelineItemsResponse` | Session Controller | Required `RunRef`/`WorkspaceRef` | Timeline cursor | Bounded read | Bounded repeat | Repeat from committed timeline cursor | Merge validated timeline page | `controller_timeline` | E12 pinned contract; not runtime/live evidence |
| `ListPendingInteractions` | `InteractionService.ListPendingInteractions` | `ListPendingInteractionsRequest` → `ListPendingInteractionsResponse` | None | Required `RunRef`/`WorkspaceRef` | Read | Bounded read | Bounded repeat | Repeat read | Replace safe summaries | Advertised Interaction support | E12 pinned contract; not runtime/live evidence |
| `GetControllerInteraction` | `InteractionService.GetControllerInteraction` | `GetControllerInteractionRequest` → `GetControllerInteractionResponse` | Session Controller | Required `RunRef`/`WorkspaceRef` | Read | Bounded read | Repeat after carrier validation | Refresh summary, then repeat | Replace typed Controller-only detail | Advertised Interaction support | E12 pinned contract; not runtime/live evidence |
| `ResolveInteraction` | `InteractionService.ResolveInteraction` | `ResolveInteractionRequest` → `ResolveInteractionResponse` | Session Controller | Required `RunRef`/`WorkspaceRef` | Required key; response bytes never persisted | 20s | No automatic gRPC retry; no protected replay | Refresh summary/detail; finish if resolved, otherwise ask again | Apply receipt/status; invalidate summary/Run as needed | Advertised Interaction support and response limit | E12 pinned contract; not runtime/live evidence |
| `GetWorkspaceWriterStatus` | `WriterService.GetWorkspaceWriterStatus` | `GetWorkspaceWriterStatusRequest` → `GetWorkspaceWriterStatusResponse` | None | Required `WorkspaceRef` | Read | Bounded read | Bounded repeat | Repeat read | Replace Writer projection | `reader_writer_access`, `durable_writer_authority` | E12 pinned contract; not runtime/live evidence |
| `AcquireWriter` | `WriterService.AcquireWriter` | `AcquireWriterRequest` → `WriterState` | Eligible existing-thread Session Controller | Required `RunRef`/`WorkspaceRef` | Tokenless expected revision | 15s | No automatic replay | Writer status + `GetRun` | Replace Writer; invalidate Run-derived actions until fresh | Writer features; transition supported | E12 pinned contract; not runtime/live evidence |
| `ReleaseWriter` | `WriterService.ReleaseWriter` | `ReleaseWriterRequest` → `WriterState` | Writer-owning Session Controller | Required `RunRef`/`WorkspaceRef` | Tokenless expected revision | 15s | No automatic replay | Writer status + `GetRun` | Replace Writer; invalidate Run-derived actions until fresh | Writer features | E12 pinned contract; not runtime/live evidence |
| `VerifyController` | `ControllerService.VerifyController` | `VerifyControllerRequest` → `VerifyControllerResponse` | Host-selected candidate carrier | Required `RunRef`/`WorkspaceRef` | Verification read | Bounded read | Repeat after carrier revalidation | Revalidate carrier and repeat | Replace verification result only | `controller_binding` | E12 pinned contract; not runtime/live evidence |
| `GetOrchestratedSession` | `OrchestrationService.GetOrchestratedSession` | `GetOrchestratedSessionRequest` → `GetOrchestratedSessionResponse` | Primary Controller | Primary `RunRef`/`WorkspaceRef` | Read | Bounded read | Bounded repeat | Repeat authorized read; no repair | Replace typed aggregate revision/status/policy/counts/recovery | Required consumer method | E12 pinned contract; not runtime/live evidence |
| `ListOrchestratedSessionResults` | `OrchestrationService.ListOrchestratedSessionResults` | `ListOrchestratedSessionResultsRequest` → `ListOrchestratedSessionResultsResponse` | Primary Controller | Primary `RunRef`/`WorkspaceRef` | Opaque session/head-bound cursor | Bounded read | Same cursor repeat | Resume captured-head traversal, new traversal for new results | Merge published results; ArtifactRef owner is Primary RunRef | Required consumer method | E12 pinned contract; not runtime/live evidence |
| `GetArtifact` | `ArtifactService.GetArtifact` | `GetArtifactRequest` → `GetArtifactResponse` | Optional by visibility | Required `RunRef`/`WorkspaceRef` | Read | Artifact read | Bounded repeat | Refetch metadata | Replace artifact metadata/chunk bound | `artifact_retrieval` | E12 pinned contract; not runtime/live evidence |
| `ReadArtifactChunk` | `ArtifactService.ReadArtifactChunk` | `ReadArtifactChunkRequest` → `ReadArtifactChunkResponse` | Optional by visibility | Required `RunRef`/`WorkspaceRef` | Artifact ID + offset + length | Artifact read | Repeat same chunk after metadata/carrier validation | Refetch metadata; resume after length/digest validation | Append verified inert bytes; no authoritative aggregate | `artifact_retrieval` | E12 pinned contract; not runtime/live evidence |
| `AdoptController` workflow | Gul application orchestration | Host selection + local validation + `ControllerService.VerifyController` + atomic binding transaction | Host-selected candidate, then verified Controller | Existing Direct Session and Run | Gul-owned adoption identity | Bounded workflow | No blind replay | Restart from carrier validation and fresh reads | Atomically replace binding; refresh Run/Interaction/Writer | Controller plus local store | E12 pinned contract; not runtime/live evidence |
| `EvaluateActions` | Gul domain evaluator | Local pure evaluation | Binding health is typed input, never authority inferred from UI | Fresh typed Run/Writer/Interaction/timeline inputs with compatible stamps | Projection convergence tuple | Local | Safe reevaluation | Refresh required inputs | Closed Gul action/blocker set | N/A | E12 pinned contract; not runtime/live evidence |
| `AllocateDeliveryEvent` | Gul projection/event journal | Local transaction + ConnectRPC delivery | None; authorization-sensitive enrichment already completed | Validated safe projection | Gul `delivery_sequence` transaction | Local | Transaction retry only | Replay journal or coalesced snapshot | Browser-safe delivery event | N/A | E12 pinned contract; not runtime/live evidence |
<!-- contract-operation-map:end -->

The map above contains first-release operations only. Every verification label identifies the E12 checked contract and explicitly denies runtime or live-provider evidence. Generated files consume this table and must not be hand-edited. Deferred continuation contracts remain with E4-T4 and its requirements, not inside the first-release map.

### 6.5 Live subscription window

One shared channel carries at most eight logical `WatchRunEvents` streams. Admission order is pending Interaction/recovery or active Turn, user-visible Run, recent activity, then stable Run ID. Safety-relevant candidates preempt immediately; ordinary demotion uses 30-second hysteresis. Non-live Runs receive snapshot and pending-Interaction polling with a hard 10-second maximum discovery delay.

Each `RunSubscriptionState` tracks `run_id`, `last_validated_upstream_cursor`, `last_committed_projection_cursor`, `stream_generation`, connection state, last heartbeat, last snapshot refresh, and reconnect attempts. Stream loss never changes Run lifecycle. If the validated cursor is ahead of the committed cursor, Gul first refreshes authoritative snapshots, then resumes from the accepted cursor boundary. Rejected, rewound, or unresolvable cursors also force snapshot refresh. Per-stream bounded queues and projection coalescing turn overflow into `SlowConsumer` for that stream without delaying mutations or other Runs.

Stream termination is classified by wire form. A `RunEventStreamEnd(RUN_TERMINAL)` triggers final Run/timeline/artifact refresh as needed and closes without reconnecting indefinitely. `RunEventStreamEnd(SERVER_SHUTDOWN)` marks the provider unavailable or restarting without failing the Run, reconnects the gateway, and resumes each live Run from its last committed upstream cursor. `SLOW_CONSUMER` is not a `RunEventStreamEnd` enum value in v1: the server closes only that stream with gRPC `RESOURCE_EXHAUSTED` plus typed `SLOW_CONSUMER`; Gul refreshes the required snapshots and resumes from its committed cursor without affecting unary mutations or other Run streams. Other transport failure produces stale/disconnected presentation state, never `RunFailed`, and follows the bounded connection policy.

Dolgorae's event invalidation contract is a mandatory minimum. Gul may add local presentation invalidations, such as FileService or profile refresh, but may never remove an upstream-required aggregate invalidation or required read. Each typed event uses the following exhaustive rule. “Disable only” means the event may remove an unsafe action immediately but cannot enable a mutation until every upstream-required aggregate is fresh at or beyond the event's `ProjectionStamp` and the complete stamps are compatible.

<!-- contract-event-map:start -->
| Envelope/variant | Complete update | Upstream invalidation and required refresh | Gul-local additional effect | Action-evaluator safety |
|---|---|---|---|---|
| `RunStateChanged` | None | Invalidate Run; also invalidate Writer when lifecycle can affect authority. Fetch `GetRun` and conditionally `GetWorkspaceWriterStatus`. | Refresh profile only when a separate generation/capability signal requires it. | Disable affected Run actions until Run/Writer stamps converge. |
| `TurnStateChanged` | Turn status only | Invalidate Run and timeline. Fetch `GetRun` and page timeline to a compatible captured head. | Update transient Turn presentation from the event hint. | Never enables a mutation by itself. |
| `FinalResponseAvailable` | Final-response value | Invalidate Run and timeline. Fetch `GetRun` and page timeline. | Resolve inline data or verified artifact chunks. | No writer action; final state comes from refreshed aggregates. |
| `InteractionOpened` | Interaction identity and kind only | Invalidate Interaction and Run pending count. Fetch `ListPendingInteractions`, `GetRun`, and Controller detail only with a healthy binding. | Raise Interaction priority in the live-window scheduler. | Disable conflicting actions until summary/detail and Run stamps converge. |
| `InteractionResolved` | Interaction outcome only | Invalidate Interaction, Run pending count, and timeline. Fetch `ListPendingInteractions`, `GetRun`, and timeline. | Remove or update the browser card only after authoritative reads. | Disable response action until refreshed. |
| `WriterStateChanged` | Writer generation hint only | Invalidate Writer and Run. Fetch `GetWorkspaceWriterStatus` and `GetRun`. | None. | Disable all writer actions until both refresh and stamps converge. |
| `RecoveryRequired` | Recovery notification only | Invalidate Run and Writer. Fetch `GetRun` and `GetWorkspaceWriterStatus`. | Raise unresolved/recovery presentation priority. | Immediately block conflicting mutations. |
| `RuntimeErrorOccurred` | Diagnostic only | Invalidate Run when the event's Run revision advances beyond the cached stamp; fetch `GetRun` in that case. | Append bounded safe error presentation. | Never marks the Run failed or enables an action by itself. |
| `GenerationChanged` | Generation or epoch only | Invalidate Run, Writer, and Interaction. Fetch `GetRun`, `GetWorkspaceWriterStatus`, and `ListPendingInteractions` plus authorized detail as required. | Re-handshake and refresh Profile when server/profile generation may have changed. | Block affected mutations until every required aggregate refreshes. |
| `WorkspaceChanges` | Workspace-change observation | Invalidate timeline and page it to a compatible captured head. | Invalidate affected FileService/Git presentation paths and perform bounded local refresh. | No authority change. |
| `CommandStarted` | Command observation | Invalidate timeline and page it. | Update bounded operational presentation. | No authority change. |
| `CommandCompleted` | Command observation | Invalidate timeline and page it. | Complete bounded operational presentation. | No authority change. |
| `UsageReported` | Usage observation | Invalidate timeline and page it. | Update bounded usage presentation. | No authority change. |
| `DiagnosticReported` | Diagnostic observation | Invalidate timeline and page it. | Append bounded safe diagnostic presentation. | No authority change. |
| `ReasoningSuppressed` | Suppression metadata | Invalidate timeline and page it. | Record only bounded suppression metadata, never content. | No authority change. |
| `RunEventHeartbeat` | Connection health and advisory durable head only | No semantic aggregate update; fetch snapshots only on gap, rewind, or commit uncertainty. | Update connection health. | No action change. |
| `RunEventStreamEnd(RUN_TERMINAL)` | Subscription terminal marker only | Fetch final Run and required timeline/artifact state. | Close the subscription without endless reconnect. | Use refreshed final state only. |
| `RunEventStreamEnd(SERVER_SHUTDOWN)` | Provider connection interruption only | Reconnect, re-handshake, refresh snapshots, and resume from the committed cursor. | Mark provider unavailable/restarting. | Block mutations while unavailable. |
| typed gRPC `SLOW_CONSUMER` | No semantic update | Refresh the affected Run's required aggregates and resume from its committed cursor. | Terminate only that subscription. | No cross-Run or unary effect. |
| other transport failure | No semantic update | Apply bounded reconnect and snapshot policy. | Mark connection stale/disconnected only. | Block only actions requiring freshness. |
<!-- contract-event-map:end -->

Run, Writer, and Interaction compatibility is established by the complete provider `ProjectionStamp`; timeline compatibility is established by `captured_head_cursor`. A snapshot with a later stamp supersedes an earlier invalidation. Equal stamps are mutually compatible. If state continues to advance while Gul is refreshing, the relevant action remains disabled rather than combining revisions. No partial event enables a mutation while any required aggregate is stale or incompatible.

The capability handshake starts with `GetCapabilities.context.protocol_version = 0` plus Gul's minimum and maximum protocol versions. It validates Dolgorae and protocol versions, descriptor and credential-schema digests, supported methods, required features, event projection version, Interaction payload contract, client policy, and artifact/Interaction bounds. Every later `RequestContext.protocol_version` equals the negotiated version. A missing or incompatible required field fails closed.

The public descriptor also contains `RuntimeService.ListProfileDiagnostics`, `RunService.SetDefaultEffort`, `RunService.ForkRun`, `RunService.VerifyRun`, and `WriterService.PrepareWriterHandoff`, `CommitWriterHandoff`, and `CancelWriterHandoff`. Gul v0.1 intentionally calls none of them. Their presence does not block compatibility, they are never exposed through a generic passthrough, and future use requires a separate Gul ADR and typed provider-port extension.

### 6.6 Direct Interactive defaults

Run creation requests these semantic values unless the user explicitly selects a supported alternative:

```text
control_mode       = direct_interactive
purpose            = interactive
execution_lane     = dedicated
required_assurance = best_effort_personal_alpha
```

Before StartRun, Gul locally creates and validates a credential carrier under the Dolgorae-owned Gul carrier root, then supplies its derived carrier reference to Dolgorae. Dolgorae verifies and binds the carrier; it does not create the credential or implicitly invent a Gul binding.

E3-T4 provides an unmounted launch selector and authenticated browser handler over
the user-global Profile registry. It displays compatibility, runtime version,
models and efforts, supported lanes, maximum assurance, and capability summary.
The selected model, effort, lane and assurance are checked against a fresh
`GetProfile` projection. The Policy name is checked against trusted local
preprovisioning; Dolgorae confirms its validity at StartRun. The resulting
prospective configuration includes both choices. The shared read-only lane
requires a separate choice and acknowledgement of its permanent effect.
E2-T3 owns revalidation and the
actual carrier and StartRun mutation; E8 owns route registration.

`shared_readonly` is permanent. The first release blocks a later write request with a typed unsupported-transition result and preserves the source. E4-T4 owns any future `CreateWriteContinuation` flow. Starting a fresh Orchestrated Session is a new launch and is never labeled or recorded as lineage continuation.

An existing-thread `dedicated` Run offers in-place write only while the selected profile reports transition support. When unsupported or unverified, the first release blocks the write without offering continuation. An upstream transition rejection is a typed blocker, never a retryable failure.

## 7. Domain model

### 7.1 RuntimeAttachment

```text
RuntimeAttachment
  id
  provider_kind
  accepted_executable_reference
  expected_version_range
  supported_rpc_api_versions
  binary_identity
  health
  capability_snapshot
  last_compatible_at
```

Health and capabilities are cached projections. A fresh handshake overrides them. The socket path and channel are supervisor-owned ephemeral state and never attachment fields.

### 7.2 WorkspaceEntry

```text
WorkspaceEntry
  id
  display_name
  favorite
  recent_order
  runtime_attachment_id
  runtime_workspace_id
  verified_canonical_root_snapshot
  selected_runtime_profile
  navigation_state
  file_explorer_state
```

`verified_canonical_root_snapshot` is the runtime workspace address and the FileService root. `runtime_workspace_id` verifies provider identity but never replaces the canonical path; a missing, moved, non-UTF8, or mismatched root fails closed and requires reattachment or the typed unsupported-path presentation.

### 7.3 DirectSession

```text
DirectSession
  gul_session_id
  workspace_entry_id
  runtime_provider_id
  runtime_run_id
  controller_binding_reference
  display_name
  presentation_lifecycle
  local_navigation_state
  predecessor_session_id?
```

There is no `codex_thread_id`, `codex_turn_id`, or Gul-owned Turn state machine. Runtime lifecycle shown in the UI is a RuntimeProjection.

The accepted `RunConfigurationProjection` is authoritative for profile, purpose and purpose label, model, current default effort, required capabilities, parent provenance, and instructions identity. Gul updates Direct Session presentation from it after `StartRun`, every `GetRun`/snapshot refresh, Gul restart, and Dolgorae restart. Any local copy is only a cache; the original request never becomes reconnect authority.

`runtime_run_id`, `runtime_turn_id`, `interaction_request_id`, `handoff_id`, and `artifact_id` are typed opaque provider references that convey no authority. Optional references travel in a closed `ProviderReferences` group on validated projections and operation inputs.

### 7.4 RuntimeActivity

`RuntimeActivity` is a discriminated navigation projection. The initial kind is `dolgorae_orchestrated_session`, with one retained DirectSession Primary binding and observer-only child views. SessionProjection records the authorized provider's aggregate revision/status/policy/counts independently of Run ProjectionStamp. A local cache is not membership authority. Future Gorae and Podway variants remain optional; workflow execution identity must not be equated with Session or Run identity.

An independently brokered subagent used by a profile tool is not a
`RuntimeActivity`. Its bounded result appears only inside the owning Direct
Session's parent-Turn chat projection. Child identity does not create a Gul
Controller binding or mutation surface.

### 7.5 ControllerBinding

The database stores only a reference and health metadata:

```text
ControllerBindingReference
  id
  direct_session_id
  credential_store_key
  controller_public_metadata
  binding_health
```

Secret capability bytes reside in create-exclusive files below the capability-advertised `~/.dolgorae/controller-carriers/gul/<installation-id>/` root, outside every Workspace root and every FileService-resolvable path (ADR-0047). SQLite stores only a logical relative key. The store derives the absolute path and immediately before every authorized RPC proves root containment, current-user ownership, `0700` parent modes, `0600` regular-file mode, and absence of symlinks in every relevant component.

`Create` writes credential schema version 1 with a UUIDv7 `controller_id`, `kind=interactive_client`, stable trusted-local `instance_id` and `subject_id`, 32 crypto-random bytes encoded as unpadded base64url, and explicit orchestration_launch with the selected preprovisioned Policy name. It validates the actual advertised schema digest and the `~/.dolgorae/controller-carriers/gul/<installation-id>/` root policy. Creation is exclusive with no overwrite, fsyncs the file and parent, and clears capability buffers where practical. Installation and account identifiers originate in trusted local setup, never a browser field. Gul supplies the validated carrier to StartRun and Dolgorae verifies it before binding. Same-principal successor creation is deferred with E4-T4 and REQ-CTRL-013; it is not a first-release credential-store operation. Neither bytes nor path enter gRPC metadata or ConnectRPC. One distinct credential per Direct Session/Run is Gul policy, not a universal Dolgorae invariant.

Controller adoption is a Gul application workflow, not a credential-store capability. A host-controlled selector supplies a protected carrier reference; the application calls the store's existing validation/resolution operations, invokes side-effect-free provider `VerifyController` against the session's `runtime_run_id`, atomically replaces the binding, and refreshes the Run including its RecoveryProjection, Writer status, pending Interactions, and timeline as needed. It never accepts capability bytes or an arbitrary absolute path through a browser/API message.

### 7.6 RuntimeProjection

`RuntimeProjection` is a composed Gul presentation view over independently validated provider aggregates. It is not one upstream snapshot and is never treated as an authorization token.

```text
RuntimeProjection
  provider_id
  runtime_object_id
  observed_at
  freshness
  observation_mode
  run_lifecycle
  thread_presence
  active_turn
  pending_interaction
  control_mode
  execution_lane
  final_response
  interaction_summary
  writer_authority
  writer_generation
  effective_policy
  policy_verification
  profile_capabilities
  profile_compatibility
  access_policy_transition_support
  background_execution
  requested_assurance
  achieved_assurance
  recovery_state
  required_recovery_action
  lineage
  run_configuration
  run_projection_stamp
  writer_projection_stamp
  interaction_projection_stamp
  timeline_captured_head_cursor
  invalidated_aggregates
  gul_action_set
```

Each independently fetched provider aggregate is stored as a cache record with explicit convergence metadata:

```text
ValidatedAggregate<T>
  value
  projection_stamp
  observed_at
  freshness
  invalidated_by_cursor?
  required_at_least_stamp?

ProjectionConvergenceState
  run_stamp?
  writer_stamp?
  interaction_stamp?
  timeline_captured_head_cursor?
  invalidated_aggregates
  last_invalidation_cursor?
```

`ProjectionStamp` retains the provider's `captured_head_cursor`, `run_state_revision`, `writer_state_revision`, and `interaction_state_revision`. An event invalidation records the minimum stamp that a later snapshot must satisfy. A later stamp supersedes the invalidation, equal complete stamps are mutually compatible, and continuously advancing stamps keep the affected action disabled. Timeline freshness is checked separately through `captured_head_cursor`. On Gul or gateway restart every provider aggregate is marked stale until refreshed, even when cached stamps are retained for diagnostics and safe replay positioning.

`gul_action_set` is derived locally from fresh typed inputs and is reevaluated at every mutation endpoint. It may disable an action from a partial event immediately, but it may enable one only when all required aggregates are fresh and their complete stamps are compatible under the provider contract. Upstream cursors live only in `ObservationCheckpoint(provider_id, runtime_object_id, stream_kind, last_validated_cursor, last_committed_cursor)`; DirectSession records contain no cursor.

Additional infrastructure-facing domain records are:

```text
RunSubscriptionState
  run_id, last_validated_upstream_cursor, last_committed_projection_cursor
  stream_generation, connection_state, last_heartbeat_at
  last_snapshot_refresh_at, reconnect_attempt_count

ProviderOperationAttempt
  operation_id, semantic_operation, target_reference
  idempotency_key?, controller_reference_set?
  normalized_request_sha256?
  replay_material_reference?, replay_availability
  deadline, state, reconciliation_operation, created_at, updated_at

ProviderReplayEnvelope
  operation_id, operation_kind
  canonical_request_schema_version
  canonical_request_sha256
  canonical_request_material
  controller_references
  created_at, expires_at

ReplayControllerReference
  role
  expected_controller_id
  credential_store_key?
  direct_session_binding_id?
```

`ProviderOperationAttempt` and `ProviderReplayEnvelope` are Gul coordination state, not runtime authority. The first-release replay envelope is allowed only for `StartRun`, is stored outside SQLite as an exclusive owner-only bounded file, and contains no Controller capability bytes, protected Interaction input, `SubmitTurn` prompt, image bytes, carrier path, or socket path. Its role-tagged Controller references contain only expected public Controller IDs and logical credential-store or Direct Session binding keys. The operation row stores only the replay envelope's logical reference, digest, and equivalent non-secret reference metadata. A resolved envelope is deleted immediately. An unresolved envelope expires after at most 72 hours in v0.1, may be configured to expire sooner, and is purged at startup and at least every six hours; expiry removes canonical material and replay availability while the non-secret attempt remains visible as `OutcomeUnknown` until authoritative evidence or explicit operator handling resolves it.

The single closed evaluator consumes an `ActionEvaluationInput` composed of typed provider state plus Gul-owned state:

```text
ActionEvaluationInput
  run_lifecycle, thread_presence, active_turn, pending_interaction
  control_mode, execution_lane
  writer_authority, writer_generation
  effective_policy, policy_verification
  profile_capabilities, profile_compatibility
  access_policy_transition_support
  background_execution, requested_assurance, achieved_assurance
  recovery_state, required_recovery_action, lineage
  run_projection_stamp, writer_projection_stamp, interaction_projection_stamp
  timeline_captured_head_cursor, invalidated_aggregates
  controller_binding_health
  direct_session_ownership
  unresolved_mutation_state
  provider_compatibility_state
  orchestrated_session_projection, session_revision, session_freshness
  aggregate_close_intent, owned_work_counts, explicit_interrupt_consent
```

Every field is independently required even when another aggregate appears to imply it. Missing, unknown, string-only, stale, or stamp-incompatible decision input yields `BlockedByProviderCompatibility` or `RequiresFreshSnapshot`; it is never inferred from writer, policy, lifecycle, or diagnostic text. Both UI rendering and every mutation endpoint evaluate this same structure. Aggregate revision has its own domain and is not compared for equality with Run stamps. Session-wide action eligibility requires a fresh authorized aggregate observation; the provider still rechecks actual state at mutation admission.

The E4-T3 implementation uses `internal/action.Input` and the pure `Evaluate`
function for this contract. The checked adapter reads Run, workspace Writer,
Profile, pending Interaction, timeline head and authorized root aggregate facts.
Ownerless workspace-status Writer stamps contain only the workspace Writer revision; another
owner's Run and Interaction revisions remain in that owner's domain. The local
repository adds Controller health, subject ownership, invalidation floor and
unresolved operation metadata. Aggregate required actions remain independent
from its recovery classification.

`WriterActionService` exposes the shared result and performs guarded Acquire or
Release once with a freshly read Run revision. Accepted responses display the
provider projection and require a new complete snapshot before further actions.
Release returns the released Run policy, lane, assurance and full stamp even when
the owner is absent; it has a separate validation boundary from workspace status.
The initial threadless Unknown/Unverified policy permits the first eligible Turn
without claiming verified WRITE before its accepted provider response.
The Interaction service uses the same evaluator before response admission.
`RequireSubmit` is the admission port for later Submit assembly. Primary
lifecycle, Controller adoption and session-close flags are classifications;
E2/E5 own their execution, and E8 owns authenticated product route assembly.
The isolated Writer panel, prompt draft and Interaction components consume only
these Gul flags and typed blockers. They do not mount a runtime route.

## 8. Direct Session flows

### 8.1 Workspace registration

1. An existing path is selected either through the Wails directory picker on the host or through bounded server-side browsing restricted to the host-configured workspace-root allowlist. The allowlist is canonically resolved when configuration loads; a browse request carries an allowlist entry plus a relative path, never an absolute path.
2. Gul validates the path locally and proves containment beneath the selected allowlist root when the request came from browsing.
3. On first attachment Gul calls `InspectWorkspace` with that host-controlled absolute path and no expected Runtime Workspace ID. The provider inspects an already initialized workspace with a provisioned compatible Runtime Profile and returns its canonical root and Runtime Workspace ID/digest.
4. Gul verifies the returned root and identity, stores WorkspaceEntry metadata, and supplies the stored canonical path plus expected Runtime Workspace ID on every later revalidation. Every later workspace-scoped call uses a `WorkspaceRef` built from that attachment. It returns typed `workspace_not_provisioned`, `profile_missing`, or identity-mismatch blockers as applicable. Gul never invokes `init`, provisions a profile, or trusts a browser-supplied provider path or workspace ID.
5. FileService uses that verified snapshot for subsequent relative-path access.

Remote clients cannot supply an absolute path. Registration browsing never resolves or reveals anything outside the allowlist: outside-root, traversal, and symlink-escape probes return one indistinguishable typed error. Case-variant provider-private paths stay denied; in-root case aliases resolve under REQ-WS-010. An empty or unresolvable allowlist disables server-side browsing while leaving the host picker available.

### 8.2 Submit

The browser sends Gul Session ID, prompt, an explicit closed `write_intent` of `READ` or `WRITE`, optional image references, and optional effort override. The backend authenticates and applies CSRF/Origin checks, resolves DirectSession, refreshes the closed action evaluation, persists/reuses the server-owned operation identity, revalidates the Controller carrier, validates local images through the same resolved-path private-root guard, and invokes typed `SubmitTurn`. A write intent never substitutes for provider writer authority, except that the provider-defined first write Turn is the activation mechanism for an eligible threadless Run.

The normalized Turn request remains in memory only for the lifetime of the active operation. If the response is lost while the same Gul process still holds the exact bytes, application replay may use the original idempotency key under the accepted provider policy. Gul does not persist the prompt or image bytes for crash-safe replay. After process restart it reconciles through `GetRun` and timeline, preserves `OutcomeUnknown` when acceptance cannot be proved, and never automatically submits a reconstructed or new Turn.

### 8.2.1 Human prompt admission and history

While a Primary Turn is active, the composer keeps a draft without creating a
provider queue item. Explicit send requires authoritative terminal evidence and
fresh action/session eligibility. No automatic send on idle, auto-interrupt, or
steering is allowed. The backend and provider recheck busy admission to prevent
multi-client races. Exact replay of an accepted identity is not a fresh submit.
Explicit interruption has its own confirmation/unknown-outcome path. Current
Interaction answers remain possible while the Turn is waiting.

Prompt History is a separate projection of Primary USER_INPUT_ACCEPTED timeline
items. Stable Run/item/cursor/Turn identity drives ordering and deduplication;
matching text does not. Show ordinal, provider time, original preview/full body,
and Turn navigation. Long inputs use authorized user-input artifacts. Rejected,
pending, draft, and unknown attempts are not silently inserted into accepted
history. Full pagination and restart/close restore the same original history.
The timeline cache is not a resend queue and never authorizes mutation replay.

### 8.3 Interaction

```text
Dolgorae interaction summary            observer-safe; carries no decision context
  → Gul Controller-only fetch of ControllerInteraction
  → per-kind allowlist projection into an Interaction Card
  → Gul event bridge
  → browser card and user response
  → Gul Controller-only resolve
  → Dolgorae authoritative result
```

The stream event is only a notification. The observer summary carries only the accepted safe fields: interaction/request ID, created time, nullable expiry and resolution times, kind, status, Controller kind, protected-input marker, and the final typed user-escalation boolean. It may not contain Controller-sensitive decision material. Any human-readable escalation explanation is a non-authoritative Gul presentation derived from typed state, never a substitute for that boolean. The backend fetches `ControllerInteraction` through the session's binding, merges both shapes by interaction/request ID, and projects a Gul-owned **Interaction Card** typed per kind under a strict allowlist. `expires_at=null` means no upstream expiry. The browser never receives either upstream message directly.

New card mapping uses the E12-T1-pinned TASK-053 typed Protobuf `ControllerInteraction.payload` oneof and client policy. Generated `CommandApprovalInteraction`, `FileChangeApprovalInteraction`, `UserInputInteraction`, and `UnsupportedInteraction` variants are mapped exhaustively; unknown required variants and kind/variant mismatches fail closed. The Protobuf envelope, projection stamp, maximum safe payload size, and per-kind field allowlist are validated before domain mapping. Gul never parses arbitrary JSON based only on `summary.kind`, and each card exposes only fields allowed for that kind.

Only the bound backend may fetch `ControllerInteraction`. Advertised kinds are classified supported or unsupported; unknown or unclassified kinds fail closed as visible blockers and are never dropped or auto-resolved. For Gorae-managed work, Gul receives only Gorae-level escalation and cannot resolve the internal Dolgorae interaction.

An interaction the provider marks protected requests secret material from the user. That answer is a second secret class alongside Controller capabilities: it is size-checked in memory before parsing and before forwarding, then travels browser → backend → one `ResolveInteraction` RPC body bounded by `min(maximum_response_bytes, Gul's 64 KiB cap)`, never through a file carrier or gRPC metadata. Safe payload decoding is independently bounded by `min(maximum_safe_payload_bytes, GUL_MAX_SAFE_INTERACTION_PAYLOAD_BYTES)`, where the Gul v0.1 local cap is exactly 8 MiB. Rejected or accepted values never enter SQLite, a retry queue, the client event journal, projection cache, logs, metrics, traces, diagnostics, error text, or browser storage; transient buffers are cleared where practical. The Interaction ID, idempotency key, attempt state, and non-secret metadata may be persisted, but response JSON and secret-derived values may not. Protected-response RPCs are excluded from automatic gRPC retries. After a lost response Gul refreshes summary and authorized full Interaction state; if resolved it finishes locally, if pending it asks the user for the value again, and if neither can be proven it preserves unresolved state.

### 8.4 Writer

Writer display includes authority state/generation, effective access/verification, lane, requested/achieved assurance, owner reference, blocker, and Gul-derived actions. `WRITE` is derived only from:

```text
writer_authority.state = active
AND effective_policy.access = write
AND effective_policy.verification = verified
```

Acquire and release are upstream commands. Writer status is an observation read, so writer state stays visible even when a Controller binding is missing or unhealthy. Acquire is offered only for an eligible existing-thread Run; it is absent for a threadless Run when `threadless_acquire_write=false`.

The first-release closed evaluator emits only `CanSubmitRead`, `CanSubmitWrite`, `CanAcquireWriter`, `CanReleaseWriter`, `CanInterrupt`, `CanResolveInteraction`, `CanRecover`, `CanReconcile`, `CanAdoptController`, `CanPausePrimary`, `CanResumePrimary`, `CanRequestSessionClose`, `RequiresCloseConfirmation`, `RequiresOperatorAction`, `RequiresFreshSnapshot`, `BlockedByOutcomeUnknown`, `BlockedByCredentialState`, `BlockedByBackgroundExecution`, and `BlockedByProviderCompatibility`. These are Gul-owned decisions, not upstream RPC enums. Close request eligibility never asserts that closure is already complete; active owned work requires explicit interrupt confirmation. A continuation-required state maps to a typed unsupported blocker, not a successor action. Inputs are the typed Run projection, workspace-writer projection, profile capabilities/compatibility, effective access and verification, transition support, lane, writer authority and generation, requested/achieved assurance, background execution, recovery/action state, lineage, Controller-binding health, Gul session ownership, and unresolved operation state. Unknown or string-only decision state fails closed.

Because every Gul Direct Session holds its own Controller (ADR-0035), any other session that owns the writer is a different Controller upstream, and the provider's same-controller handoff precondition never holds. Writer transfer is therefore Release in the owning session followed by a separate Acquire in the target session. Gul presents the intervening unowned window, keeps no queue or reservation, and reports a competing acquisition from the provider result. When the owning Controller is one of Gul's own sessions, Gul may offer navigation to it where the provider allows Release from there; when the owner is outside Gul, no mutation action is offered.

### 8.5 Presentation versus runtime lifecycle

Rename, favorite, archive/hide, panel state, browser close, and navigation are local. Pause, resume, and interrupt are Primary-scoped provider operations. Close is a root provider operation over the whole owned aggregate. Delete and successor are absent from the first release. Hiding or removing a presentation record never implicitly mutates the Run.

Recover and Reconcile are provider operations exposed only through an advertised RecoveryCapability.

### 8.5.1 Whole-session close and read-only results

The E5-T1 close coordinator uses the Primary Controller and root CloseRun only,
after E4-T3 supplies eligibility/confirmation and E3-T3 supplies aggregate reads.
A declared browser route is unavailable until that coordinator is complete.
Without interrupt intent,
active owned work is a typed busy rejection. With explicit confirmation, the
Broker records closing intent, stops new admissions, and handles all owned
Specialist work. Gul displays closing/pending until GetOrchestratedSession and
fresh Run projections prove closure. Unknown work is not reported closed.
Never loop over children or stop shared Profile Servers. History/results/files
are retained; hide/navigation/browser exit is not close. Pause/Interrupt is
Primary-scoped and is not presented as an aggregate-wide pause.

ListOrchestratedSessionResults provides typed stable publication records and
permitted Primary-owned ArtifactRef values, using bounded captured-head pages.
GetArtifact/ReadArtifactChunk use that Primary RunRef and its Controller. Do not
parse model links, read private stores, or fabricate a Primary final response
for a Specialist result. Reads do not acknowledge Primary delivery or reexecute
work. Session revision and Run stamps are independent observations; neither
is synthesized from the other's counter.

Refresh aggregate and result snapshots on opening/reconnect, relevant Run
notifications, and a coalesced bounded schedule while owned work or closure is
in progress. Aggregate-only changes need not emit a Primary Run event; event-only
refresh would miss them. Stale aggregate state disables affected actions and
never becomes an empty count. Root Recover/Reconcile accounts for retained
aggregate-close intent under the provider contract, followed by fresh separate
Run/Session/Writer/Interaction reads. It does not auto-resume paused work.

### 8.6 Externally reset Controller adoption

An operator uses a local terminal outside Gul to reset/provision a Controller capability into an approved protected carrier. A browser may request adoption mode but cannot submit a path. Gul.app uses the host-controlled picker; headless mode requires an explicit host-local console workflow that supplies the selection directly to the core and is unavailable through ConnectRPC. Gul validates path, owner, mode, regular-file type, symlink absence, and credential-root policy, calls side-effect-free `VerifyController`, atomically replaces the binding, then refreshes the Run including recovery state, Writer status, pending Interactions, and timeline as needed. Malformed carriers, mismatched Runs, and ambiguous verification fail closed; Gul never receives or uses the Operator capability.

### 8.7 Timeline reconstruction

Opening or reconnecting a Direct Session performs `GetRun`, then `ListRunTimelineItems` after the stored timeline checkpoint, validates Run and Turn identities, merges provider chronology into a non-authoritative presentation cache, and publishes one coalesced browser snapshot. Unknown or non-approved timeline kinds are rejected or redacted according to the accepted safe-timeline inventory. Missing TimelineCapability is a compatibility blocker.

The E4-T5 reader also exposes bounded `ListConversation` pages and
`GetConversationEntry` originals through the unmounted DirectSession handler.
Conversation entries use Gul IDs and closed kinds/statuses for accepted human
input, final responses, Interaction openings/resolutions and Turn termination.
A fresh traversal rebuilds the validated prefix; stable metadata-derived IDs let
reconnect reads merge without text-based deduplication. A timeline older than
the authorized Run snapshot is unavailable rather than fresh.

### 8.8 Artifact presentation

Artifact references remain opaque. Dolgorae's inline-final-response and maximum-artifact values are provider wire capabilities; Gul's 256 KiB inline-browser threshold, preferred 256 KiB chunk size, and 64 MiB artifact cap are local presentation/safety limits. A 300 KiB inline provider response is therefore not a protocol violation: Gul may turn it into a bounded browser presentation object. Effective artifact size is `min(provider maximum, 64 MiB)` and each unary chunk request is no larger than both the advertised maximum and Gul's preferred size. Gul verifies exact total length plus SHA-256 before presentation. Safe Markdown is rendered under the Gul allowlist; content is never executed, automatically opened, or interpreted as a local filesystem path.

## 9. Application API

All browser APIs are Protobuf-defined ConnectRPC. Mutations are unary, authenticated, CSRF/Origin protected, typed, and governed by each operation's accepted concurrency contract. Client events are server streamed.

```text
AuthService
  FirstRunSetup, Login, Logout, GetSession

RuntimeService
  ListProviders, GetProviderHealth, GetProviderCapabilities,
  ListRuntimeProfiles, CheckCompatibility

WorkspacePresentationService
  ListWorkspaces, RenameWorkspace, SetWorkspaceFavorite,
  SetWorkspaceHidden, GetNavigation, SetNavigation,
  RegisterFromHostSelection,
  ListRegistrableRoots, BrowseRegistrableRoot, RegisterFromAllowlistPath,
  RevalidateWorkspace, RemoveWorkspaceEntry; Get is planned

DirectSessionService
  GetDirectSessionPresentation, ListDirectSessions, RenameDirectSession,
  SetDirectSessionFavorite, SetDirectSessionArchived,
  ListPromptHistory, GetPromptHistoryItem, GetExecutionState,
  ListSpecialistResults,
  Submit, Interrupt, PauseRuntime, ResumeRuntime, CloseRuntime,
  Recover, Reconcile, BeginControllerAdoption; Create is planned

InteractionPresentationService
  ListPending, GetCard, Resolve

WriterActionService
  GetActionState, AcquireWriter, ReleaseWriter

ArtifactPresentationService
  GetMetadata, ReadChunk

FileService
  InspectPath, ListDirectory, ReadPreview, RefreshFiles,
  GetGitStatus, CompareFixedRevisions, WatchFileChanges

ClientEventService
  Subscribe(after_delivery_sequence)

DiagnosticsService
  GetSummary
```

No API returns raw Dolgorae messages, socket or carrier paths, Controller capabilities, private worker identifiers, App Server transport details, arbitrary absolute paths, unclassified artifacts, or arbitrary Git revisions. `BeginControllerAdoption` creates a host-local selection workflow; the browser never submits a carrier path or capability bytes.

`ListRegistrableRoots` and `BrowseRegistrableRoot` expose only allowlist entry identifiers plus relative names, never absolute paths, and `RegisterFromAllowlistPath` accepts an allowlist entry identifier plus a relative path. `InteractionPresentationService.GetCard` returns the allowlisted card, never `ControllerInteraction`. Delete and WriteContinuation routes are absent in the first release. Artifact endpoints return only validated chunks. Writer handoff RPCs are absent.

Every error response carries a stable Gul code and closed action class. The domain distinguishes `TransportUnavailable`, `DeadlineExceeded`, `ProtocolIncompatible`, `ControllerMismatch`, `ControllerCarrierInvalid`, `WriteContinuationControllerInvalid`, `WriterConflict`, `ThreadlessRequiresWriteTurn`, `InteractionStale`, `InteractionAlreadyResolved`, `RecoveryRequired`, `OutcomeUnknown`, `SlowConsumer`, `ArtifactUnavailable`, `UnsupportedPathEncoding`, `RunStateConflict`, `RpcServerAlreadyRunning`, `OperatorActionRequired`, `InvalidPageToken`, and `PageTokenExpired`. Mapping uses gRPC status plus typed Dolgorae details and a typed provider-required-action enum; it never parses a human-readable status message or action string.

### 9.1 Browser read contracts

E1-T3 declares these Protobuf-defined Gul application types and their contract
fixtures. They are not aliases for generated Dolgorae messages. Session/item/view
IDs below are Gul-owned opaque references backed by presentation mappings, not
raw provider IDs or credentials. Get remains presentation metadata; it does not
implicitly return complete history or all results.

| Operation | Gul request | Gul response and implementation owner |
| --- | --- | --- |
| DirectSessionService.ListPromptHistory | session_id, optional page_token, page_size | snapshot_id, ordered PromptHistoryItem summaries, optional next_page_token, traversal_complete, freshness and observed_at. E4-T5. |
| DirectSessionService.GetPromptHistoryItem | session_id, prompt_item_id | Stable item identity, ordinal, accepted_at, conversation_entry_id and a typed original-content value: exact inline UTF-8 or authorized Gul artifact reference. E4-T5. |
| DirectSessionService.GetExecutionState | session_id | Gul ExecutionState containing mapped lifecycle/composition/approval policy, safe policy identity, named counts, optional bounded observer-only member status with synthetic Gul references and a truncation flag, close progress and recovery classification, optional Gul close_operation_ref, typed provider_state, state_version, freshness and observed_at. E3-T3 and E5-T1. |
| DirectSessionService.ListDirectSessions | workspace_id | Subject-scoped Gul Direct Session presentation references for locally bound Primary Runs. E3-T3. |
| DirectSessionService.ListSpecialistResults | session_id, optional page_token, page_size | snapshot_id, ordered SpecialistResult summaries, optional next_page_token, traversal_complete, freshness and observed_at. E4-T5. |

PromptHistoryItem contains prompt_item_id, one-based ordinal, accepted_at,
exact-original preview, preview_truncated and conversation_entry_id. It contains
only accepted human input. Ordinals count validated accepted human items from the
start of the retained history, not ledger gaps, local send attempts or timestamps.
The backend tracks validated prefix coverage with its checkpoint. It must not
assign an ordinal from a partial suffix or deduplicate by text. Stable mappings
survive ordinary reconnect and cache rebuild; loss of a valid mapping requires
explicit reattachment or unavailable state, not reassignment to another item.

SpecialistResult contains a Gul result_id and specialist_view_id, safe role label,
publication time/order, typed format, byte_length, SHA-256 and a Gul artifact
reference. Its provider task/result identity and Primary artifact owner remain
backend mappings. GetMetadata/ReadChunk in ArtifactPresentationService revalidate
those mappings, current authorization, size and integrity; a browser must never
supply a raw provider RunRef or artifact path to obtain bytes. Reading never
acknowledges private result delivery.

Pages default to 50 items and accept 1..100, with at most 256 KiB of encoded Gul
metadata per page. A preview is at most 1 KiB of a complete UTF-8 prefix, marked
truncated when shorter than the original. Full originals use the existing 256 KiB
browser-inline threshold and verified artifact reads for larger bodies; previews
are never substituted for full content. All provider and local size limits still
apply independently. An item that cannot fit the metadata contract is a typed
limit error, never silently omitted.

### 9.2 Paging, freshness and authorization boundary

A Gul page_token is a bounded opaque handle (maximum 4 KiB), bound to the current
Gul account, session, query kind, projection version, captured snapshot scope and
scan position. The backend retains or authenticates its mapping to provider
cursors. It never forwards the raw provider cursor, publication head, Run stamp,
Controller generation or private identity as a browser token. Each page request
rechecks the authenticated session, binding and applicable provider authority;
the token itself grants no access. Cross-session/query substitution is rejected.

The first history page starts from the validated history prefix and captures an
upper head. Later provider pages can report later heads; Gul keeps the original
upper watermark for that browser traversal and leaves new items for a fresh
traversal. The result list uses the provider's fixed publication-head traversal.
The snapshot_id is a Gul handle for this scope, not a provider cursor in disguise.
The backend may read at most four provider pages per browser request, within the
existing deadline/concurrency budget. A filtered page with no human items may
return an empty items array and a continuation token; it is not end of history.
traversal_complete is true only when the captured scope was fully scanned.

Expired, evicted or restart-invalidated token mappings return PageTokenExpired
and require a fresh traversal. Malformed or wrong-query tokens return
InvalidPageToken; neither case silently restarts pagination or asserts empty
history. Gul may retain valid mappings across restart, but provider-backed
reconstruction and stable item identity do not depend on token survival.
Unknown timeline types and source corruption follow the pinned fail-closed
policy; they are not skipped to fabricate a complete page.

GetExecutionState performs a bounded coalesced authoritative refresh. It returns
an explicit FRESH, STALE or UNAVAILABLE classification; cached authorized data
may accompany STALE but cannot enable a mutation. Unavailable counts are absent,
not zero. state_version is Gul-owned change metadata and cannot replace or be
numerically compared with the independent backend aggregate revision and Run
ProjectionStamps. Every mutation re-evaluates fresh decision inputs regardless
of a browser's displayed state_version. On provider reconnect, refresh execution
state and start or continue valid history/result traversals separately.

ClientEventService supplies Gul-owned invalidation or presentation notifications.
It is not a replacement for paginated reads and must not imply that the complete
history or result collection is present in one delivery replay.

### 9.3 Whole-session close application contract and ownership

CloseRuntime accepts the Gul session_id and explicit interrupt choice under the
existing authenticated mutation-attempt contract. E1-T3 defines CloseOutcome
and its fixture shapes. E4-T3 supplies the shared eligibility/confirmation
classification; E5-T1 owns the complete coordinator and enables the route only
when both that evaluator and E3-T3's execution-state read are available.
E3-T3 may render passive state but cannot install a temporary direct close route.

CloseOutcome distinguishes REJECTED, IN_PROGRESS, CONFIRMED, OUTCOME_UNKNOWN and
RECOVERY_REQUIRED. It carries a Gul close_attempt_id allocated before dispatch,
an optional close_operation_ref when the provider's durable operation is known,
and typed next-observation/recovery information. REJECTED must additionally carry
a Gul-owned rejection object with the existing stable domain error code and
closed action classification, distinguishing authorization, stale revision, busy
and invalid-target outcomes without exposing provider text or private reasons.
Requests rejected before Gul authentication or session authorization use the
existing typed ConnectRPC error and disclose no close attempt or session state.
The reference maps to the
provider operation_id only in the backend; a local attempt ID is not proof of
provider acceptance. GetExecutionState exposes the same Gul close operation
reference after it is discovered through the root query, including response loss.

SESSION_CLOSE_IN_PROGRESS maps to IN_PROGRESS, not a generic error or CONFIRMED.
Only provider-confirmed whole-session settlement permits CONFIRMED. Transport
loss without accepted evidence is OUTCOME_UNKNOWN. No automatic tokenless retry
or repeated child commands follow either case. Read GetOrchestratedSession via
the adapter, then required Run/Writer/Interaction projections; use authorized
root recovery only when indicated. E5-T2/T3 test reconnect and fault behavior,
E7-T2 integrates the UI, and E2/E9 supply actual released-provider proof.

## 10. Event and reconnect architecture

### 10.1 Upstream events

Dolgorae owns upstream cursors and events such as Run/Turn state, final response reference, interaction-open notification, writer state, recovery blocker, and runtime error. Gul stores last validated and last projection-committed cursors solely as reconnect metadata. Each v1 typed-Protobuf event uses the accepted generated decoder, validates its projection stamp and identities, and exhaustively maps the closed `oneof` under the Section 6.5 invalidation matrix. Run/Direct Session binding validation, authorization-sensitive enrichment, and safe projection commit precede every Gul delivery allocation; successful envelope decoding alone is insufficient.

### 10.2 Client events

Gul owns browser-facing events such as presentation changes, projection updates, interaction card changes, file invalidation, and runtime disconnect. Each browser record has a global monotonic `delivery_sequence`, Gul-owned session/item/operation references when applicable, correlation ID, allowed payload kind and creation time. Provider/runtime IDs and upstream cursors needed for correlation remain in backend checkpoint metadata and are not serialized into the browser record.

### 10.3 Reconnect

On browser reconnect, Gul authenticates, returns fresh Gul presentation state plus the latest validated provider snapshot, replays retained client events after the supplied delivery sequence, or requests a full snapshot when retention is insufficient. Event replay never invokes a provider mutation.

On provider reconnect, Gul re-establishes the supervised channel, handshakes compatibility, marks every provider aggregate stale, loads authoritative Run, Writer, Interaction, and timeline snapshots with their stamps, converges them under Section 6.5, resumes each Run from its safe cursor boundary, then emits coalesced Gul events. One Run stream may reconnect without disturbing other streams. No mutation is re-enabled until every required aggregate has a compatible fresh stamp.

The upstream cursor uses the accepted Protobuf representation and never becomes a Gul delivery number. A filtered sequence gap is not by itself a fault. A provider-rejected, rewound, unresolvable, or validated-but-uncommitted cursor triggers authoritative snapshot refresh before resumption. Stream transport failure is presented as stale/disconnected observation, not `RunFailed`.

## 11. Persistence

The current versioned schema contains these Gul-owned logical tables:

```text
app_account
password_account
web_sessions
runtime_attachments
workspace_entries
workspace_attachments
workspace_favorites
direct_session_presentations
direct_session_favorites
primary_session_bindings
controller_binding_references
navigation_state
client_delivery_counter
client_event_journal
file_explorer_state
runtime_projection_cache
runtime_timeline_cache
observation_checkpoints
observation_checkpoint_stamps
observation_refreshes
client_projection_notifications
provider_operation_attempts
session_close_attempts
session_close_operations
mutation_attempt_details
writer_attempt_details
schema_migrations
```

`workspace_attachments` was added by migration 2. Migration 3 adds the two
favorite tables, and migration 4 adds `primary_session_bindings`. Migration 5
adds `observation_checkpoint_stamps`, `observation_refreshes`, and
`client_projection_notifications`. Migration 6 adds `session_close_attempts`
and `session_close_operations`. Migration 7 adds `mutation_attempt_details`,
and migration 8 adds `writer_attempt_details` with pre-call Writer revision,
generation and dispatch completion. Migration 9 adds `password_account`.
The remaining tables were created by the initial migration.

Prohibited authoritative tables/aggregates include Codex threads, Turns, workspace writer locks, writer generations, pending runtime interactions, native subagents, background processes, and runtime recovery state. A projection table is named and documented as a cache.

Database transactions cover Gul-owned presentation, auth, delivery, non-authoritative projection/timeline cache, projection-stamp and invalidation metadata, and mutation-attempt coordination only. Provider calls never occur while a SQLite transaction is held. `provider_operation_attempts` contains operation identity, normalized-request digest, replay availability, logical replay-material reference, role-tagged logical Controller references, and reconciliation state, not authoritative provider outcome, prompt, protected input, credential bytes, carrier path, socket path, or canonical request bytes.

ADR-0017 pins modernc SQLite `1.57.0`. Every connection enables foreign keys, sets `synchronous=FULL`, and uses a 5-second busy timeout; the database uses WAL mode. One writer connection serializes short write transactions; a separate read-only pool is capped at four connections. Delivery sequence allocation uses `UPDATE ... RETURNING` inside the same `BEGIN IMMEDIATE` transaction as the journal insert. Backup checkpoints WAL, uses `VACUUM INTO` on the destination filesystem, fsyncs the new file and parent, and publishes by atomic no-replace rename; direct copying of the live database is prohibited.

First-release crash-safe canonical request material for unresolved `StartRun` attempts lives in a separate protected `ProviderReplayStore` below `~/Library/Application Support/Gul/provider-replay/`. Its parent is `0700`, files are exclusive `0600`, all path components are non-symlinked and current-user-owned, material is bounded and versioned, file and parent are fsynced, and files are deleted on terminal resolution or after the fixed 72-hour v0.1 maximum retention, configurable only downward. `PurgeExpired` runs at startup and at least every six hours. Expiry removes replay availability but does not convert an unresolved operation into success or failure. The store never accepts `SubmitTurn` prompts/images or `ResolveInteraction` response bytes, and it never stores an absolute credential path or capability; exact carrier resolution uses the role-tagged logical Controller references.

`runtime_projection_cache` stores each aggregate's complete `ProjectionStamp`, freshness, and invalidation floor; its rows are marked stale at startup before mutation enablement. `runtime_timeline_cache` stores `captured_head_cursor` as non-authoritative metadata; timeline freshness is checked separately through that cursor.

`client_event_journal` stores interaction-card change notifications, never protected interaction input and never a `ControllerInteraction` payload. A protected interaction is re-presented from a fresh provider snapshot on reconnect rather than replayed from the journal.

## 12. FileService and Git boundary

FileService remains a Gul-owned read-only boundary and does not route ordinary reads through Dolgorae. It resolves Workspace Entry, loads its verified root, normalizes a relative path, safely resolves symlinks, proves containment, denies any fully resolved path within `.dolgorae/**`, applies MIME/size/line/image bounds, and performs no mutation or arbitrary command. `.dolgorae` appears only as one non-expandable provider-managed node. Missing, moved, or unverifiable roots fail closed. A provider path containing opaque non-UTF8 bytes yields `ERROR_CODE_UNSUPPORTED_PATH_ENCODING`; Gul neither performs lossy conversion nor logs raw bytes or exposes a manipulable browser path. The UI warns that previews may observe intermediate state while a writer is active.

The bounded host Git adapter may provide status and compare only fixed `HEAD` and Working revisions. It is shell-free and cannot accept arbitrary refs, object IDs, repository paths, or absolute paths. A repository root outside the Workspace never expands visibility. Non-Git workspaces retain normal browsing.

ADR-0018 selects escaped source-only SVG preview. SVG never enters an image or
document renderer in the application origin.

File invalidation uses the ADR-0038 bounded host filesystem watcher scoped to the verified canonical root, with explicit refresh retained as the fallback. The watcher is bounded in watched-node count, queue depth, and coalescing interval, and it discards any event whose fully resolved real path lies within `.dolgorae/**` before that event can affect listing, status, preview, or aggregate state. Because Gul reads the workspace directly, file freshness never requires the upstream operational event projection, so `minimal` stays the default and FileService keeps working while the provider is unavailable.

## 13. Authentication and security

Gul has one local account, server-side browser sessions, password hashing, rate limiting, Origin/CSRF protection, loopback HTTPS/ConnectRPC, and Tailscale Serve HTTPS. Funnel is forbidden. The browser never connects directly to Dolgorae and never receives its Unix socket, Controller carrier, private worker identifier, or App Server detail.

ADR-0013 defines host-authorized setup in the local Gul app, bounded Argon2id
hashing, exact UTF-8 password policy and fixed seven-day sessions. The native
shell pins Gul's protected local certificate without adding system trust; remote
browsers use Tailscale Serve's tailnet certificate. Neither network origin nor a
loopback source authorizes first setup. A native setup grant is distinct from a
browser session and from every Dolgorae Controller capability.

The RPC socket root is `~/Library/Caches/Gul/runtime/`, owner-only and outside Workspaces. Every relevant component must be absolute, current-user-owned, restrictive, and non-symlinked. The path is supervisor state, not a user-controlled SQLite field. No TCP fallback or Tailscale exposure exists.

Controller capability rules are stricter than ordinary application secrets:

- never browser-visible or cookie/URL-addressable;
- never stored as plaintext in ordinary SQLite;
- never included in prompt, workspace, event, log, diagnostics, argv, environment, stdin, browser storage, URL, or cookie;
- loaded only for the owning Direct Session and required operation;
- passed only as a path to an owner-only regular protected file outside every Workspace and FileService-resolvable path, revalidated immediately before each authorized RPC and never included in gRPC metadata;
- replaced with a distinct protected capability on successor creation;
- represented in diagnostics only by safe binding health.

Protected interaction input is a second secret class with its own rules:

- accepted from the browser only for one pending protected interaction;
- never written to the client event journal, projection cache, SQLite, logs, metrics, traces, diagnostics, error text, or browser storage;
- forwarded only in one bounded `ResolveInteraction` request body, never a file carrier, gRPC metadata, retry queue, argv, environment, or ordinary stdin;
- discarded after the resolve attempt, whatever the provider result;
- never replayed; a reconnect re-presents the pending card instead.

Canonical replay material is a separate short-lived sensitive class, not a credential or protected Interaction secret. In the first release it is permitted only for `StartRun`, is stored in the owner-only replay store outside every Workspace, is referenced only by a logical key, and is absent from browser APIs, logs, metrics, traces, diagnostics, and the delivery journal. It may contain bounded Controller instructions or handoff text required by the provider idempotency identity, but it never contains capability bytes, carrier/socket paths, a `SubmitTurn` prompt or image, or a protected Interaction answer. Role-tagged Controller references contain only expected Controller IDs and trusted logical store/binding keys. Material is removed immediately on authoritative terminal resolution or by the 72-hour maximum-retention purge; expiry leaves the non-secret attempt unresolved and fail-closed.

Gul never stores or uses an Operator capability and never invokes operator-gated reset. If a Controller is lost, mutations remain blocked until an operator uses the explicit local Machine CLI procedure outside Gul and a host-controlled picker starts verified adoption. The browser cannot provide an arbitrary path.

The credential-storage mechanism is accepted in ADR-0047. Its residual risk is explicit: a capability sits at rest on disk protected by file ownership, file mode, and full-disk encryption rather than by the system keychain, and backup and restore of the credential root are the operator's responsibility.

## 14. Diagnostics and observability

Diagnostics include Gul mode/owner, Wails, Go, frontend, database migration, singleton lock, loopback listener, Tailscale state, supervised RPC child health/restart budget, accepted binary identity, API compatibility, capabilities, channel reachability, per-Run observation mode/checkpoint state, safe errors, and Controller binding health. They do not include socket/carrier paths, App Server process health unless exposed safely, raw stderr, unrestricted paths, capability material, prompts, or reasoning.

Logs are structured, bounded, retained by policy, and keyed by Gul/provider/runtime references plus correlation IDs under `~/Library/Logs/Gul/`. A supervised-server stream is accepted only if it can be bounded and redacted under the pinned contract; otherwise the release is incompatible. Provider raw payloads and opaque path bytes are not ordinary log fields.

## 15. Recovery and failure policy

Startup order is: acquire the singleton lock; load configuration and validate the Dolgorae-owned Gul credential subtree; open/migrate SQLite; start the authenticated loopback listener; discover and verify Dolgorae; allocate and validate the private socket parent and unused pathname; start the RPC server; verify the Dolgorae-created socket; complete readiness and compatibility handshake; establish the shared channel; load Workspace/Direct Session presentation; verify Controller bindings; fetch authoritative Runs including recovery/configuration, Writer status, pending/Controller Interactions, and timeline; resume Run observation; then enable runtime mutations only for compatible healthy bindings.

RPC gateway exit marks every provider aggregate stale/disconnected but does not change Run, Turn, interaction, writer, or policy state. Gul applies its bounded restart policy, re-handshakes, reconstructs Run/Writer/Interaction/timeline aggregates with complete stamps, and converges them before re-enabling mutations. Ambiguous `StartRun` results use the persisted replay envelope and exact operation identity; ambiguous `SubmitTurn` after process loss reconciles without prompt replay; protected Interaction input is never replayed. Tokenless operations use fresh authoritative reads. Browser/Tailscale loss affects only delivery. Gul never restarts or reconciles App Server. Controller loss is not reset in product; verified adoption after an external terminal reset is the only recovery path.

Two failure classes are presented distinctly from ordinary errors:

- **Unresolved outcome.** When the provider reports an unknown turn outcome, writer authority blocked as unknown, a required recovery, or unverified background execution, Gul shows an unresolved state until an authoritative snapshot resolves it. It never renders success, failure, or writer release for an unresolved outcome, and dependent mutations stay blocked.
- **Operator action required.** Provider-side migration, an unavailable or unverifiable provider server, and Controller reset are operator-gated upstream. Gul cannot perform them, so it names the documented external procedure and offers no in-product retry.

## 15.1 Pre-release and actual-provider development

ADR-0052 organizes development into complete Epics in roadmap order. E12-T1 pins
the new producer lock. E1 builds the shared core, typed ports, SQLite, bundle and
shell foundation, using isolated test dependencies without requiring later
features. E13-T1 then supplies stateful deterministic provider fakes over those
ports. Its scenario drivers verify provider behavior without requiring Gul UI or
coordinators; it does not duplicate their application logic.

Workspace/session, event/history/approval, close/recovery, files and UI Epics
complete their own scoped behaviors using that harness. E8 delivers account and
cookie protection together with shell/headless/PWA/tailnet packaging. Product
access stays denied before real Gul authentication is installed; test principals
are isolated injections, never production bypasses.

E14-T1 verifies the assembled authenticated core/bundle/API and feature flows
against explicit fakes, then records the unverified live boundaries. E12-T2/T3
are retained as Retired identities, replaced by E13-T1/E14-T1. Tests exercise
actual application code, not screenshots or universal-success stubs. Fakes are
unavailable to production dependency fallback. E2-T0 then pins the released
v0.1.3 artifact; E2/E9 qualify real gateway, carrier, RPC, production headless
assembly, multi-browser and restart/close boundaries through the same ports.
A foundation Task never claims complete authenticated or live assembly evidence.

## 15.2 Future read-only Podway observations

A later optional Dolgorae observer surface supplies the pinned graph definition,
workflow execution identity, active node set, per-loop iteration and stable node
execution counts with source revision/freshness. Gul renders observations only;
reconnect or duplicate messages cannot increment counts. Multiple/nested loops
and simultaneous active nodes remain distinguishable. Missing information is
unavailable/stale, not zero or completed. No Gul API/backend/UI can edit FSM,
jump/skip/force/reexecute nodes, or reset counters. Changes are ordinary prompts
judged by the executing LLM; only actual Podway state updates the graph. Feature
absence never blocks existing session, history, approval, or close flows.

## 16. Test architecture

The first-release integration matrix covers the 27-method consumer profile: protocol-zero handshake; workspace bootstrap and global Profile reads; authoritative Run and Session observations; exact StartRun replay; process-local SubmitTurn retry without history-based resend; complete Timeline and original prompt history; public result discovery then verified artifact reads; sequential input and current Interaction replies; whole-session close and retained-intent recovery; independent revisions, event cursors, reconnect and slow consumers; typed secret-safe errors; and no production fake/CLI fallback. E13-T1 verifies the provider scenario harness independently; E14-T1 executes assembled authenticated core/browser flows with those explicit fakes. E2/E9 verify the real released provider. Delete, continuation and same-principal successor tests are deferred, not hidden first-release gates. Historical 2026-08-19 cases apply only when consistent with this scope.

Supporting suites include deterministic domain/action-matrix/error-mapping/serialization tests; SQLite migration, operation-attempt, checkpoint, timeline-cache, and delivery replay tests; generated fake gRPC server fixtures; exact closed-schema Machine CLI conformance; socket ownership/symlink/collision/process tests; compatibility and optional-field policy fixtures; Controller and protected-input canaries; and manual Wails/macOS/Tailscale/launchd qualification. Real Dolgorae smoke tests stay opt-in until E2-T0 pins a compatible release.

No test requires Gul to parse raw App Server events or own runtime state.

## 17. Packaging

The package identity is `Gul.app`, bundle identifier `xyz.rootkernel.gul`, and helper `gul` with production `gul serve`. Application Support, Logs, and Cache use the `Gul` directories defined in Required Specifications; Controller carriers live below the advertised `~/.dolgorae/controller-carriers/gul/<installation-id>/` root and the socket runtime parent lives under Gul Cache. A user `launchd` agent may own the headless core at login. Gul.app attaches to that verified core or starts the same core in-process while holding the same lock. Graceful upgrade drains the core and supervised child, replaces binaries externally, re-verifies them, and reconstructs state. Dolgorae remains an external dependency and is not silently bundled.

### 17.1 Bootstrap toolchain and command contract

`toolchain/versions.env` is the single E0-T8 bootstrap authority. The supported host is macOS `>=14.0.0` on `arm64`, with Go exactly `1.27.1`, Wails `>=3.0.0-beta.8` through `wails3`, Node `>=26.7.0`, Bun `>=1.4.2`, Buf `>=1.66.1`, protoc `>=35.1`, and Git `>=2.39.0`. Non-Go host checks do not impose upper bounds. Project manifests pin protoc-gen-go `1.36.12`, protoc-gen-connect-go `1.20.0`, and protoc-gen-es `2.14.0` exactly. Protobuf-ES v2 emits the TypeScript message schemas and Connect service descriptors together; the incompatible Connect-ES v1 generator is not part of the toolchain. Buf lints, constructs the byte-compared descriptor, and checks additive compatibility. A newer accepted host tool that changes generated or descriptor bytes fails the reproducibility checks. PATH copies of language-client generators are not used as project evidence.

Project dependency manifests must match TypeScript `7.0.2`, React/React DOM `19.2.7`, React/React DOM types `19.3.0`, Bun types `1.4.2`, protobuf-go `1.36.12`, connect-go `1.20.0`, Connect-ES/Connect-Web `2.1.2`, Protobuf-ES `2.14.0`, modernc SQLite `1.57.0`, and the Wails Go module `v3.0.0-beta.24`. The Wails module pin is separate from the `wails3` host CLI minimum. The read-only `make toolchain-check` reports a Go pin mismatch and missing or below-minimum host tools; it never installs or substitutes a dependency. Project validation reports dependency or generator pin drift. Wails v2 does not satisfy the Wails v3 beta minimum.

The serial command facade is `toolchain-check`, `generate-contract`, `contract-check`, `generate-frontend`, `frontend-check`, `generate-api`, `api-check`, `test-prepare`, `test-unit`, `test-int`, `test-e2e`, and `test`. E0-T7 supplies checked contract generation and drift delegates. E1-T2 supplies checked frontend generation and bundle drift delegates. E1-T3 supplies checked Gul API generation, drift detection and provider-error catalog coverage. These commands fail closed on their source, version, generated-output, schema, or policy boundaries. No bootstrap command scaffolds the application or claims runtime behavior.

## 18. Artifact migration matrix

| Former artifact | Disposition | Replacement/owner |
|---|---|---|
| App Server supervisor / AppServerPool / JSON-RPC adapter | Remove | Dolgorae; Gul supervises only its public local gRPC gateway. |
| Per-request CLI process / per-Run event follower process | Supersede | One supervised Dolgorae RPC server, one reusable channel, unary calls, and logical per-Run streams. |
| Workspace aggregate as canonical runtime identity | Modify | WorkspaceEntry verified canonical path as runtime address, with provider ID/digest as identity verifier. |
| Session with `codex_thread_id` | Supersede | DirectSession with `runtime_run_id`. |
| Turn aggregate/state machine | Remove | RuntimeProjection of Dolgorae state. |
| WriteLock / WriteLockTakeoverRequest | Remove | Writer projection and WriterActionService delegation. |
| InteractiveRequest authoritative aggregate | Remove | InteractionSummary/ControllerInteraction projections. |
| ApplicationEvent as sole authority | Modify | Upstream event plus Gul client event layers. |
| `workspace_write_locks`, `turns`, `interactive_requests` tables | Remove | Dolgorae authority; no equivalent authoritative Gul table. |
| Session/Turn/WriteLock/InteractiveRequest RPCs | Replace | DirectSession, InteractionPresentation, and WriterAction services. |
| App Server recovery/process diagnostics | Remove | Provider compatibility, snapshot recovery, and safe provider diagnostics. |
| Background-process census/policy | Remove | Dolgorae assurance and blocker projection. |
| Writer handoff prepare/commit/cancel port and RPCs | Defer | ADR-0035; eligible existing-thread transfer uses Release then Acquire, while threadless first write never Acquires. |
| Public `GetControllerInteraction` RPC | Replace | `InteractionPresentationService.GetCard` returning a per-kind allowlisted Interaction Card. |
| Provider `allowed_actions` as Gul authority | Remove | Closed Gul action evaluator over provider and local state. |
| Local prompt/final cache as runtime history | Modify | Provider timeline is authoritative; cache is presentation-only. |

# Part B. Current Architecture

## 19. Current snapshot

**Snapshot date:** 2026-09-30 (E8-T1 completed)

**Roadmap point:** E0 is `Completed`, E12 is `Completed`, E1 is `Completed`, and E13 is `Completed`; E1-T1 is `Completed` and E1-T2/T3/T4/T5 are `Completed`. E3 and E3-T1/T2/T3/T4 are `Completed`. E4 and E4-T1/T2/T3/T5 are `Completed`. E4-T4 remains Deferred outside its epic. E5 and E5-T1/T2/T3 are `Completed`. E6 and E6-T1/T2/T3 are `Completed`. E7 and E7-T1/T2/T3 are `Completed`. E8 is `In Progress`; E8-T1 is `Completed` and E8-T2/T3 are `Planned`. Former E12-T2/T3 remain Retired. E14 owns pre-release application acceptance. No live-provider or assembled-application acceptance is implied.

**Maturity:** delivery-independent Go core, shared React bundle, declared but disabled Gul API, typed provider ports and explicit scenario harness, isolated SQLite repositories with fake-scoped Workspace attachment, local presentation and passive session reads, and a Wails shell foundation. E4 adds typed observation, Interaction cards, action eligibility and bounded history/result/artifact reads. E5 adds whole-session close, reconnect convergence and operation-specific recovery against fakes. E6 adds verified-root local FileService inspection, bounded previews, refresh, watcher and Git review through unmounted typed APIs. E7-T1 adds three responsive panes, workspace/session navigation and read-only FileService presentation through injected typed clients. E7-T2 adds provider-projected activity, prioritized Interaction cards, conversation and accepted-user Prompt History, and guarded whole-session close to that injected-client view. E7-T3 adds composition-safe input and keyboard focus behavior to the same component scope. E8-T1 adds isolated local account setup, bounded password hashing and the injected first-run form. Authenticated product assembly and live-provider qualification remain pending.

### 19.1 Implemented components

`internal/auth` implements the E8-T1 local setup permission, bounded Argon2id
hashing and saved-subject password verification. SQLite migration 9 adds the
singleton password record. Setup commits the account and hash atomically,
retains the sole foundation subject, refuses ambiguous data, and cannot add a
second password account. Password hashing runs before the write transaction;
the service serializes hash work and sanitizes repository failures.

`frontend/src/first-run-setup.tsx` supplies an injected local setup form with
no username. Remote views have no setup controls. The password stays out of
React state and browser storage, clears before submission, and is never shown
in an error. The E7 composition guard protects this form too. These components
are exercised by isolated Go/SQLite and Chrome checks. The checked entry still
mounts the fail-closed foundation. E8-T2 owns AuthService transport and session
protection; E8-T3 owns native bootstrap, HTTPS and authenticated host assembly.

`internal/app` implements the shared delivery-independent core lifecycle and
explicit lifecycle, provider, persistence and authorization ports. Missing
dependencies are composed with fail-closed defaults: startup performs no
external provider work, product access is denied, and provider/persistence
availability is not implied. The core does not import Wails; the desktop
foundation hosts it, while assembled headless and authenticated delivery remain
future work.

`api/proto` declares Gul-owned Runtime, WorkspacePresentation, DirectSession,
ArtifactPresentation and FileService browser
contracts. Its `bounds.json` is the shared authority for generated Go and
TypeScript page/content limits. Generated clients are checked for drift. The
declarations are not registered as routes. `contract/port` defines the exact
27-method typed consumer interface and a closed provider-error translation
catalog; the root Go module consumes this checked local module through one
explicit local replacement. `contract/scenario` supplies a stateful test
provider over that port without production wiring. `internal/domain` maps
provider dispositions to browser enums and holds bounded
account/session/query-bound opaque page tokens.
Accepted runtime and socket path errors map to `RUNTIME_PATH_UNAVAILABLE` with
their typed recovery action; `UNSUPPORTED_PATH_ENCODING` remains a distinct
Gul path-encoding error, not a label for an unsafe provider socket.

`frontend` builds one exact-pinned React/TypeScript source tree into the checked
`internal/delivery/web/dist` bundle. `internal/delivery/web` exposes that one
embedded filesystem to browser delivery and the Wails shell, preventing a
second frontend build or delivery-specific domain surface.

`internal/workspace` accepts a trusted host directory selection or a relative
path beneath a host-loaded canonical allowlist. It rejects symlink traversal,
outside roots, and provider-private paths before inspection, then stores the
provider's verified canonical path and ID with local presentation metadata.
Revalidation checks the original filesystem identity and expected provider ID.
An explicit registration can reattach a moved or replaced workspace after
inspection while retaining its Gul presentation ID and metadata. The pinned
inspection adapter lives in `internal/workspace/contractprovider`, keeping the
Workspace service and SQLite adapter independent of generated provider types.
`internal/delivery/api` exposes typed Runtime, Workspace and Direct Session presentation
handlers for explicit authenticated composition; no production route is
registered. Their authentication and authorization rejections preserve the
ConnectRPC status and include `UNAUTHORIZED` with the closed `ABORT` action.
The macOS shell
has a directory-picker adapter, but no attached product binding. Dismissing the
host picker returns a typed cancelled registration result with no Workspace.

`internal/files` reads a subject-scoped saved Workspace attachment without
calling the provider. It checks the canonical root's stored filesystem identity
on each request, then walks relative paths through root-anchored descriptors.
It rechecks the opened node's current location before returning its descriptor.
Symlinks resolving outside the root and every resolved `.dolgorae/**` path fail
with the same unavailable-path result as a missing target. The typed
`InspectPath`, `ListDirectory`, and `ReadPreview` handlers use the local access
gate. Directory pages return at most 100 entries and retain at most 128
short-lived directory cursors. Invalid tokens receive `ERROR_CODE_INVALID_PAGE_TOKEN`;
expired or refresh-invalidated tokens receive `ERROR_CODE_PAGE_TOKEN_EXPIRED` and
must restart from the first page. Unsupported
byte encodings return a typed error without an alias. Text is bounded to
256 KiB and 4,000 lines. Raster previews require checked dimensions, a known
MIME type, and a 4 MiB limit. Markdown embeds at most eight contained raster
assets through the same guarded accessor. SVG is escaped source only. Explicit
refresh closes retained pages and advances a Workspace revision. A bounded host
watcher tracks verified nodes and coalesces public changes independently of the
provider. Direct Git commands report contained direct and ancestor status and
compare fixed `HEAD` with current Working previews. Typed Git degradation
preserves Working preview. No FileService route is mounted yet. E7 supplies
the injected-client file pane; E8/E14 own authenticated delivery and assembled
application acceptance.

`internal/presentation` changes only subject-scoped Gul metadata. Attached
Workspace names, favorites and hidden state are independent of their directory
paths. Direct Session names, favorites and archive state retain the existing
provider reference without invoking it. Navigation points only to a visible
attached Workspace and, when selected, its visible Direct Session. Hiding a
Workspace clears its selection; archiving a session clears that session from
navigation. The typed handlers use the core's local access gate, so an offline
provider does not block authenticated presentation edits. They remain unmounted.

`internal/session` persists one Gul Direct Session binding per accepted provider
Primary Run. Its trusted bind hook requires a revalidated Workspace and a
backend-resolved Controller carrier. The checked adapter reads `GetRun`,
authorized `GetOrchestratedSession`, and optional `ListRuns` parent links. It
rejects a wrong Controller or non-session root and refreshes provider-owned
configuration on bind and reopen. Public
Run parent links yield optional Specialist status observations only; they do
not supply aggregate membership or counts. The browser read caps observations
at 256 and reports truncation; the service retains at most 256 stale snapshots.
The stale cache is process-local; after a restart an offline provider yields
unavailable state until a new authoritative observation succeeds.
Oversized provider snapshots fail validation before persistence. A coalesced
refresh is bounded independently of any one caller, so cancelling one browser
read does not cancel another caller's read of the same session.
The unmounted Direct Session handler
lists local bindings and maps aggregate reads to Gul-owned browser types.
Unavailable reads omit counts and close references. Workspace removal clears
only local presentation, navigation and binding rows.

`internal/launch` reads the global Profile catalog through the pinned
ListProfiles/GetProfile adapter and checks explicit launch choices against a
fresh selected Profile. It checks the Policy name against trusted local
preprovisioning. The unmounted Runtime handler returns a bounded browser
catalog and prospective configuration, including the shared read-only warning.
Accepted semantic errors from GetProfile retain `FIX_REQUEST` or
`USE_SUPPORTED_PROFILE`. Other semantic errors require operator repair.
It creates no Run or Controller carrier.

`internal/observation` translates the pinned typed event envelopes into
payload-free invalidations. Its per-Run bridge keeps validated and committed
provider cursors separate from browser delivery sequences, coalesces updates,
and calls a required refresh port with the complete invalidation set. SQLite
commits invalidation floors and the checkpoint before allocating delivery in the
same `BEGIN IMMEDIATE` transaction, which revalidates the binding under the
writer lock. A checkpoint companion retains the committed comparison stamp;
a failed transaction leaves both stamp and cursor unchanged.
The shared manager admits at most eight streams with safety priority and
30-second ordinary demotion hysteresis. Browser notifications retain only Gul
session/correlation references; slow clients receive the latest invalidation.
The unmounted ClientEvent handler uses generated ConnectRPC server streaming,
bounded journal replay, and snapshot-required notifications when replay exceeds
256 entries. It checks the subject and session on each read. The shared writer
rule displays WRITE only for fresh active owner authority with verified write
policy; only an explicit write intent sets the upstream write flag. The writer
status component renders the backend mode in `SessionDetail`'s injected-client
presentation. These paths are tested against explicit fakes; the ClientEvent
handler remains unmounted. Authenticated product assembly remains with E8 and
E14.
E5 owns reconnect coordination and fresh aggregate convergence.

`internal/interaction` provides observer summaries, Controller-authorized typed
cards and one-shot response handling. The adapter checks current Controller
identity/generation, matching complete stamps, the selected typed payload size,
kind/variant agreement and per-kind decision context. Card paths are relative
to the bound Workspace; opaque or escaping paths block the card. File-change
artifacts require negotiated retrieval support, Controller visibility and integrity
capabilities. Reads honor the smaller provider/card total bound and the minimum
negotiated, artifact-specific and 256 KiB chunk bound before becoming actionable.
Response bytes use the smaller negotiated limit and 64 KiB, are normalized only
in memory, sent once and cleared where practical. A fresh authorized read after
every submission distinguishes resolved, stale, pending re-entry and unknown
outcomes. No response body or content-derived value enters persistence.

`internal/action` evaluates the closed 19-action set from typed provider facts,
local coordination metadata and a fresh independent session aggregate. Recovery
and Close share the same owned-work consent rule, including background work,
member Runs, Spawns, pending approvals and accepted unfinished tasks. A pending
local call blocks recovery even when another matching attempt has an unknown
outcome. Backend Acquire and Release re-evaluate these facts before one provider
call; accepted projections require a fresh read before another mutation.
The generated WriterAction handler remains unmounted. E7 composes the writer
panel and interrupt consent into the injected-client operator view and verifies
the separate prompt draft component in browser fixtures. E2 owns Submit/Interrupt
execution and E5 supplies Close coordination at their checked adapter boundaries.
E8/E14 own authenticated assembly and application acceptance.

The observer connects live-window selection to per-session unary polling at a
five-second interval with a five-second total deadline. Independent workers
prevent a slow Run from starving other Runs. Polling needs no Controller; it
reads Run and pending Interaction snapshots and emits coalesced invalidations
through the existing Gul journal without moving provider event checkpoints.
Read failures invalidate the visible projection rather than enabling actions.
The generated InteractionPresentation handler remains unmounted; E7 composes
the React cards into the injected-client operator view. E8 owns authenticated
registration and E2/E9 own live transport qualification.

`internal/history` supplies Controller-authorized conversation, prompt and public
Specialist-result reads through the checked contract adapter. Page tokens bind
subject, session, source binding, query and captured scope. History scans at most
four provider pages per request; an empty page can still carry a continuation.
Result traversals retain the provider publication head, revision and capture time.
Validated human-prefix counts supply one-based ordinals. Stable IDs derive from
non-secret source identity and chronology, never from the text.

The bounded metadata cache stores no original bodies. Each original read fetches
its source again; each artifact read checks authorization, negotiated bounds,
length and SHA-256 before returning bytes. The service permits two concurrent
reads with a five-second deadline. Tokens expire after fifteen minutes and on
restart. Missing item mappings return unavailable until an authorized traversal
rebuilds them; reconstruction preserves the same item IDs. SQLite stores only
the existing subject-scoped session binding. The generated history/conversation
and ArtifactPresentation handlers remain unmounted. The shared Markdown renderer
allows paragraphs, headings and fenced code; HTML, links and images stay literal.
E5 supplies reconnect scheduling and E7 supplies the integrated conversation
and history UI through injected clients. E8 owns authenticated route registration.
These components use explicit fake evidence, not live RPCs.

`internal/storage` requires an owner-only database directory and file, then
opens the pinned SQLite driver with one writer and at most four read-only
connections. Versioned migration creates only Gul-owned records;
auth sessions retain token digests, Controller bindings retain logical keys,
and operation attempts retain non-secret references. A credential-path helper
revalidates those keys under a selected owner-only root immediately before its
caller uses them; no provider RPC is wired yet. Cache stamps become stale
on reopen. Delivery sequence allocation and journal insertion share one
immediate transaction. Backup checkpoints WAL and publishes an owner-only
`VACUUM INTO` image without copying the live file. This repository remains
unwired to production startup after E1-T4 completion.
Its second migration adds a subject-scoped Workspace attachment record; local
presentation and verified provider reference commit in one transaction.
Migration 3 adds subject-scoped favorite tables. Direct Session discovery
inserts require a bounded display name and preserve existing display metadata,
so later provider refreshes cannot undo a local rename or archive choice.
Migration 4 adds one-to-one subject-scoped Primary bindings and a local copy of
the latest provider Run configuration. The copy is presentation data; GetRun
remains authoritative on every fresh read and after a restart.
Migration 5 adds checkpoint stamps, pending refresh metadata, and Gul-only
notification references. Projection updates, checkpoint advancement, and
notification delivery commit in one immediate transaction.

E5-T1 adds `internal/sessionclose` and migration 6. A Close request
passes the shared action evaluator and records its attempt atomically before
the checked adapter sends one root mutation. Duplicate requests share a stable
attempt; opaque operation references survive a database reopen. The coordinator
distinguishes rejection, progress, uncertainty, required recovery and confirmed
whole-session closure. Independent reads must agree on the Run and aggregate
revision, all owned work must be settled, and the Primary must hold no Writer
authority before closure is confirmed. Completed Specialist history remains
available. State reads never resend a mutation.

Explicit Recover/Reconcile use the provider's advertised methods and typed
eligibility. A recovery response supplies only the Run. `AggregateRefresher`
invalidates dependent caches, reads Session/Writer/Interaction/timeline
independently and verifies referenced final and timeline artifacts within the
existing bounds. A failed artifact read retains the prior timeline head so a
later refresh cannot skip it. Recovering an idle session resolves that recovery
attempt without claiming that the session closed or resolving an older close.

Provider health is a separate typed `GetExecutionState` field. Disconnected,
incompatible, busy and degraded observations retain the last snapshot as stale;
identity and credential failures remain blocked. The isolated `ProviderStatus`
component displays health and snapshot freshness without deriving actions.
`internal/recovery.Supervisor` uses an injected gateway lifecycle, clock and
random source for the bounded restart policy. Product route registration,
process startup and live-provider evidence remain with their later owners.

E5-T2 adds `internal/reconnect` with a checked compatibility
probe, a process-local action gate, independent per-Run refresh and convergence,
and live or unary-poll observation admission. SQLite keeps Run, Writer,
Interaction, timeline and delivery state separate; provider loss marks
projections stale and retains the committed cursor and unresolved attempts.
The reconnect snapshot reader uses the direct provider read port before the
action gate opens; routing those reads through gated action admission would
prevent startup convergence.
An in-flight recovery cannot publish readiness after a newer provider-loss
notification revokes its gate; a replacement recovery waits for disconnect
invalidation to finish.
Only `internal/sessionclose.AggregateRefresher` advances the artifact-verified timeline
head. Browser reconnect reads presentation, execution state and bounded Gul
delivery without sending a provider mutation. These components remain
unmounted pending later product assembly.

E5-T3 adds a non-secret mutation-attempt detail table in migration 7 and a
StartRun-only protected replay store. StartRun stores a bounded canonical
semantic request under a logical key in an owner-only file before recording
the matching attempt and dispatching the checked gRPC request. Workspace and
carrier paths are resolved again for each call. A repeated browser request
reads its existing attempt; an unresolved exact-key replay uses the retained
file, while an expired or missing file permits only Controller-matched ListRuns
reconciliation. The coordinator deletes resolved or expired files and exposes
startup and six-hourly purge operations. The host must provide an error
reporter, handle an initial purge failure before mutation admission, and keep
periodic maintenance running through reported errors with one-minute retries
when it assembles the product.

SubmitTurn retains its exact request only in process memory. Its checked
adapter reads Run and timeline on uncertainty, but their current projections
do not identify a SubmitTurn idempotency key; an absent page or matching text
therefore leaves the attempt unknown after restart. Writer Acquire/Release and
protected Interaction Resolve now persist non-secret attempt metadata before
their existing single provider call. Unknown attempts block conflicts. A
later fresh, converged Run/Writer read can settle a completed Writer call only
when its revision and generation advanced beyond the recorded pre-call state
and its owner matches the requested effect. Reconciliation rechecks the cache
and attempt under one transaction; old attempts without a pre-call baseline
remain unknown. A later fresh Interaction card can settle a matching protected-response attempt.
Database reopen also turns unfinished Close and other pending attempts into
observable uncertainty. These services and adapters remain unmounted. E8 owns
authenticated product routes and startup assembly, E14 owns assembled application
acceptance, and E2/E9 own live provider evidence.

`internal/desktop` starts and stops the same core through its lifecycle boundary,
then runs a Wails v3 window over the checked bundle's existing asset handler.
The shell registers no Wails services or product API bindings. Its injected
window host permits isolated lifecycle and asset smoke tests without opening a
real WebView. Window shutdown gives the shared core a five-second bounded stop
context, including when the window caller's context is canceled. Authenticated
singleton and verified attach remain E8-T3 work.

### 19.2 Verified runtime behavior

Isolated tests verify headless core startup/shutdown, injected lifecycle and
access ordering, denial before availability probes, typed unavailable states,
and failed-start behavior. Frontend tests, package checks, the bundle drift check,
and Go delivery tests verify fail-closed markup, exact dependency pins,
byte-reproducible checked output, manifest hashes, SPA fallback, and identical
browser/shell bytes. Gul schema and port fixtures verify generated client drift,
the 27-method inventory, provider error/action coverage, page-token scope,
bounded metadata, and CloseOutcome distinctions. The stateful `contract/scenario`
tests verify deterministic replay, captured pages, faults, streams, and close
recovery through the frozen port. These checks start no real
provider and do not establish a listener, authenticated client, Wails host, enabled browser API,
assembled persistence lifecycle, or live compatibility. Isolated SQLite tests
exercise migration, schema drift, connection settings, typed records, rollback,
sequence allocation, cache staleness, and backup publication.
Workspace tests cover picker and remote registration, canonical aliases, denied
private/escaping paths, typed provider blockers, moved roots, and changed IDs.
Session tests cover three Runs across two Workspaces, wrong-Controller and
non-session rejection, provider configuration refresh, typed Specialist status,
coalesced passive reads, stale/unavailable state, subject isolation and reopen.
Launch tests cover the scenario Profile catalog, unsupported choices, malformed
projections, consent, browser errors and UI readiness.

### 19.3 Existing artifacts

The root Go and Bun manifests, `internal/app` core, React source, checked bundle,
and shared delivery adapter now accompany the five SOT documents, E0-T8
toolchain authority, and E12-T1 TASK-053 contract artifacts. The serial Make
facade validates both Go manifests, the root package pins, exact Go 1.27.1, and
the Bun minimum before running frontend, core, contract, SOT and drift gates.
Historical E0-T7 facts remain scoped to their original digests. The Wails shell
has no authenticated attach or enabled product route; no assembled database
service, live provider adapter, or live provider behavior exists.

### 19.4 Current topology and data

```text
Shared Go core composition and lifecycle exist without assembled delivery.
One checked React bundle and shared browser/shell asset delivery exist.
An isolated Wails shell foundation reuses the shared core and checked bundle; authenticated attach is not enabled.
ConnectRPC services are declared and generated but not registered.
Gul-only SQLite schema and repositories exist in isolated tests; no production database lifecycle is enabled.
No Runtime Provider adapter, RPC supervisor, Controller credential store, timeline adapter, or Artifact adapter exists in the assembled product. Isolated Workspace, session, launch, observation, Interaction, action and history/artifact adapters use the pinned provider port with explicit fakes.
```

### 19.5 Security posture

No Gul service is running or exposed. The static foundation contains no product
controls or enabled API, and default core composition denies product access before
probing provider or persistence availability. Production authentication,
transport protection and durable security behavior remain Required State.

## 20. Promotion format

Each completed task records date, implemented components/types/APIs, security and recovery boundaries, verification evidence, accepted limitations, promoted requirement IDs, and ADRs. Required behavior is never described as Current merely because its design or dependency contract exists.
