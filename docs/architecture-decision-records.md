# Gul: Architecture Decision Records

| Field | Value |
|---|---|
| Role | Durable architecture decisions and supersession history |
| Product | Gul |
| Version | 0.1-grpc-interface-aligned |
| Last updated | 2026-08-23 |

## 1. Status model

- `Proposed`: not yet authoritative; implementation depending on it is blocked.
- `Accepted`: authoritative Required State decision.
- `Superseded`: historical decision replaced by a named ADR; its original meaning is not rewritten.
- `Rejected`: considered and declined.

An ADR records product architecture, not Current State. Requirement promotion remains governed by ADR-0015.

## 2. Index

| ADR | Title | Status | Replacement |
|---|---|---|---|
| ADR-0001 | Use Go and Wails v3 for the macOS host | Accepted | — |
| ADR-0002 | Keep the core independent of Wails | Accepted | — |
| ADR-0003 | Use Protobuf and ConnectRPC | Accepted | — |
| ADR-0004 | Use one Gul-managed Codex App Server | Superseded | ADR-0022, ADR-0023 |
| ADR-0005 | Map one Gul Session to one Codex thread | Superseded | ADR-0024 |
| ADR-0006 | Use SQLite as authority for runtime and application state | Superseded | ADR-0025 |
| ADR-0007 | Use a persistent Gul-owned workspace write lock | Superseded | ADR-0026 |
| ADR-0008 | Implement Gul-owned lock takeover | Superseded | ADR-0026 |
| ADR-0009 | Do not create automatic Git worktrees | Accepted, modified | — |
| ADR-0010 | Filter raw App Server output in Gul | Superseded | ADR-0023 |
| ADR-0011 | Use a guarded Go FileService | Accepted, modified | — |
| ADR-0012 | Share one responsive React frontend | Accepted | — |
| ADR-0013 | Use Tailscale Serve plus password authentication | Accepted | — |
| ADR-0014 | Use unary commands and streamed Gul events | Accepted, modified | — |
| ADR-0015 | Promote requirements only after task acceptance | Accepted | — |
| ADR-0016 | Select a Gul background-process policy | Superseded | ADR-0026 |
| ADR-0017 | Select SQLite driver and concurrency settings | Proposed | — |
| ADR-0018 | Select SVG preview policy | Proposed | — |
| ADR-0019 | Separate upstream cursors and Gul delivery sequences | Accepted, modified | — |
| ADR-0020 | Use bounded host Git for read-only artifact review | Accepted | — |
| ADR-0021 | Gul is an LLM-free remote operator interface | Accepted | — |
| ADR-0022 | Dolgorae is the sole v0.1 Direct Runtime Provider | Accepted | — |
| ADR-0023 | Gul never connects directly to Codex App Server | Accepted | — |
| ADR-0024 | A Direct Session maps to one Dolgorae Run | Accepted | — |
| ADR-0025 | SQLite owns presentation state and non-authoritative projections only | Accepted | — |
| ADR-0026 | Dolgorae owns writer, policy, assurance, and recovery authority | Accepted | — |
| ADR-0027 | Runtime Controller capabilities remain backend-only | Accepted | — |
| ADR-0028 | Runtime Providers are trusted built-in out-of-process adapters | Accepted | — |
| ADR-0029 | Use Dolgorae's public machine interface as initial transport | Superseded | ADR-0043 |
| ADR-0030 | Defer the Gorae Managed Runtime Provider from v0.1 | Accepted | — |
| ADR-0031 | Never bypass Gorae to control Gorae-owned Runs | Accepted | — |
| ADR-0032 | Model Direct and Managed runtime presentation separately | Accepted | — |
| ADR-0033 | Shared read-only work continues through a dedicated successor | Accepted | — |
| ADR-0034 | Store Controller capabilities as create-exclusive protected files | Superseded | ADR-0047 |
| ADR-0035 | Scope a Controller capability to one Run and defer atomic writer handoff | Accepted | — |
| ADR-0036 | Gate in-place write intent on provider access-policy transition support | Accepted | — |
| ADR-0037 | Decode provider responses under pinned schemas and fail closed on unknown decisive values | Accepted | — |
| ADR-0038 | Invalidate FileService from a bounded host filesystem watcher | Accepted | — |
| ADR-0039 | Separate the Wails shell from the core and loopback listener | Accepted | — |
| ADR-0040 | Register workspaces from a configured root allowlist as well as the host picker | Accepted | — |
| ADR-0041 | Use release tiers and require every Task complete at release | Accepted | — |
| ADR-0042 | Record the accepted product identity and its rename history | Accepted | — |
| ADR-0043 | Use Dolgorae local gRPC over a supervised private Unix socket | Accepted target; executable pin blocked | — |
| ADR-0044 | Derive a closed Gul action set and activate threadless writing through the first write Turn | Accepted | — |
| ADR-0045 | Require provider timeline and artifact capabilities in v0.1 | Accepted | — |
| ADR-0046 | Operate one shared core in desktop and headless modes | Accepted | — |
| ADR-0047 | Gul locally creates Dolgorae-schema Controller carriers under the Dolgorae-owned root | Accepted | — |
| ADR-0048 | Converge provider aggregates with ProjectionStamp before enabling mutations | Accepted | — |
| ADR-0049 | Retain operation-class-specific replay material instead of persisting prompts or secrets | Accepted | — |

## 3. Retained decisions

### ADR-0001: Use Go and Wails v3 for the macOS host

**Status:** Accepted

Gul uses a Go application core hosted by Wails v3. Wails provides desktop lifecycle and WebView delivery, not domain authority. Exact versions are pinned by E0-T8. Under ADR-0039 the core and its loopback listener are built and accepted before the shell, so the desktop framework is not a precondition for any other capability.

### ADR-0002: Keep the core independent of Wails

**Status:** Accepted

Application services depend on ports rather than Wails bindings. ConnectRPC, Wails, tests, and future transports call the same core behavior.

### ADR-0003: Use Protobuf and ConnectRPC

**Status:** Accepted

Browser-facing APIs use versioned Protobuf messages and ConnectRPC-generated Go/TypeScript clients. Raw provider responses and credentials are never public API shapes.

### ADR-0009: Do not create automatic Git worktrees

**Status:** Accepted, modified

Gul does not create worktrees. Runtime Workspace identity and writer semantics belong to the Runtime Provider. Gul registers only an already initialized workspace, invokes runtime operations with its verified canonical path, and uses the returned Runtime Workspace ID/digest only to verify identity. A mismatch requires reattachment.

### ADR-0011: Use a guarded Go FileService

**Status:** Accepted, modified

Directory, text, image, and fixed Git revision reads occur outside LLM execution. Every request uses Workspace Entry ID plus relative path, resolves symlinks, proves containment, and applies bounds. The resolved `.dolgorae/**` subtree is denied before every read, listing, status, preview, image, diff, and aggregate operation.

### ADR-0012: Share one responsive React frontend

**Status:** Accepted

Wails, desktop browser, iPad PWA, and iPhone PWA use one React/TypeScript frontend with responsive layouts, Korean IME safety, and accessible controls.

### ADR-0013: Use Tailscale Serve plus password authentication

**Status:** Accepted

Gul binds loopback, uses one local password-authenticated account and server-side browser sessions, and exposes tailnet-only HTTPS through Tailscale Serve. Funnel is prohibited.

### ADR-0014: Use unary commands and streamed Gul events

**Status:** Accepted, modified

Mutations are unary, typed, and governed by each operation's accepted concurrency contract. Browser updates use server-streamed Gul client events. Runtime mutations are delegated through provider ports; upstream events are translated into safe Gul projections before delivery.

The upstream boundary uses unary gRPC for commands and snapshots, one logical server stream per live Run, and bounded artifact chunk reads. It has no universal bidirectional command bus, and stream connectivity never grants mutation authority.

### ADR-0015: Promote requirements only after task acceptance

**Status:** Accepted

Required State and Current State remain separate. Design, schema, or dependency evidence alone does not promote a requirement. At most one Roadmap task may be `In Progress` or `In Review`; zero is valid between tasks and at release closure.

### ADR-0019: Separate upstream cursors and Gul delivery sequences

**Status:** Accepted, modified

Dolgorae owns the upstream event cursor. Gul owns a monotonic client delivery sequence. Event records may contain both plus provider/runtime IDs and correlation ID, but neither value substitutes for the other. Gul validates and commits upstream projection state before allocating a delivery sequence. Retention bounds and SQLite allocation details remain part of ADR-0017, E5-T2, and E9-T2 validation.

### ADR-0020: Use bounded host Git for read-only artifact review

**Status:** Accepted

Gul may invoke the installed Git CLI directly, never through a shell, for bounded status and fixed `HEAD`/Working reads. APIs accept neither arbitrary refs nor repository paths, and a repository root outside the Workspace never expands visibility.

## 4. Superseded historical decisions

These records retain their original meaning solely to explain migration.

### ADR-0004: Use one Gul-managed Codex App Server

**Status:** Superseded

**Replacement:** ADR-0022 and ADR-0023

The former design ran one App Server child and hid it behind AppServerPool. Gul now delegates all App Server process and protocol work to Dolgorae.

### ADR-0005: Map one Gul Session to one Codex thread

**Status:** Superseded

**Replacement:** ADR-0024

The former durable identity was a Codex thread. DirectSession now references a Dolgorae Run ID; Dolgorae owns its thread.

### ADR-0006: Use SQLite as authority for runtime and application state

**Status:** Superseded

**Replacement:** ADR-0025

The former database owned Session, Turn, lock, and interaction state. Gul SQLite now owns only authentication, presentation, attachment, credential references, delivery, navigation, FileService state, and explicitly non-authoritative projections.

### ADR-0007: Use a persistent Gul-owned workspace write lock

**Status:** Superseded

**Replacement:** ADR-0026

The former WriteLock aggregate and `workspace_write_locks` table are removed. Dolgorae is authoritative.

### ADR-0008: Implement Gul-owned lock takeover

**Status:** Superseded

**Replacement:** ADR-0026

The former two-step local takeover transaction is removed. Gul may present Dolgorae's same-controller prepare/commit/cancel handoff and cannot take over from another Controller.

### ADR-0010: Filter raw App Server output in Gul

**Status:** Superseded

**Replacement:** ADR-0023

The former adapter received raw App Server traffic and filtered reasoning. Dolgorae now provides client-safe projections. Gul retains a defensive allowlist but is not the primary raw-protocol filter.

### ADR-0016: Select a Gul background-process policy

**Status:** Superseded

**Replacement:** ADR-0026

Background containment, process census, and quiescence belong to Dolgorae. Gul displays achieved assurance and blockers only.

## 5. New runtime, security, and presentation-boundary decisions

### ADR-0021: Gul is an LLM-free remote operator interface

**Status:** Accepted

Gul contains no LLM and does not own independent-agent orchestration. It owns remote interaction and presentation, not development-runtime execution. A Direct Session profile tool may use an external trusted broker, but Gul renders only the bounded result in the parent chat and creates no child RuntimeActivity, Controller binding, lifecycle, or mutation action. This limits secrets, lifecycle authority, and recovery complexity in the remotely exposed process.

### ADR-0022: Dolgorae is the sole v0.1 Direct Runtime Provider

**Status:** Accepted

All Direct Interactive Codex execution uses Dolgorae. A supported profile and compatible public protocol are prerequisites. No fallback direct-to-Codex path exists.

### ADR-0023: Gul never connects directly to Codex App Server

**Status:** Accepted

Gul does not start, supervise, connect to, parse, correlate, schema-check, recover, or diagnose App Server directly. Any safe App Server-derived information arrives through Dolgorae projections.

### ADR-0024: A Direct Session maps to one Dolgorae Run

**Status:** Accepted

`runtime_run_id` is the authoritative runtime reference. Gul may own display and navigation lifecycle, but Run/Turn lifecycle remains upstream. Codex thread and turn IDs are absent from Gul domain and public APIs.

### ADR-0025: SQLite owns presentation state and non-authoritative projections only

**Status:** Accepted

Gul persists account/session data, attachments, Workspace Entries, Direct Session presentation, credential references, navigation, delivery journal, FileService state, one observation-checkpoint store, and projection cache. Only the checkpoint store contains upstream cursors, keyed by provider, runtime object, and stream kind. It does not persist authoritative Runs, Turns, writer state, pending interactions, native subagents, or recovery state.

### ADR-0026: Dolgorae owns writer, policy, assurance, and recovery authority

**Status:** Accepted

Writer actions are typed provider commands. Gul displays `WRITE` only for active authority plus verified effective write policy. Ambiguity fails closed. Cross-controller takeover is prohibited; same-controller handoff would follow Dolgorae, but ADR-0035 defers it because no two Gul-owned Runs share a Controller, so v0.1 transfers the writer through Release then Acquire. Background and App Server recovery policy are upstream concerns.

### ADR-0027: Runtime Controller capabilities remain backend-only

**Status:** Accepted

The browser never receives capability bytes or carrier paths. Gul stores only a trusted backend binding reference and typed health in ordinary SQLite. Capability bytes are prohibited in gRPC metadata, argv, environment, stdin, URLs, cookies, prompts, workspaces, browser storage, events, logs, and diagnostics. An authorized gRPC request may carry the backend-resolved protected-file carrier path in its typed body after immediate revalidation. The store is outside every Workspace and FileService-resolvable path. Gul never possesses an Operator capability. ADR-0047 selects caller-owned create-exclusive files under the Dolgorae carrier root.

### ADR-0028: Runtime Providers are trusted built-in out-of-process adapters

**Status:** Accepted

v0.1 providers are compiled Gul adapters to trusted local processes. Arbitrary in-process plugins, JavaScript UI extensions, marketplace packages, and untyped generic actions are out of scope.

### ADR-0029: Use Dolgorae's public machine interface as initial transport

**Status:** Superseded

**Replacement:** ADR-0043

The initial target used Dolgorae's machine-oriented CLI with JSON request/response and one event-follow process per live Run. That design is retained only as history. It made subprocess lifetime, stdout framing, exit status, and process termination part of normal provider semantics. ADR-0043 replaces it with the public local gRPC server. The Machine CLI remains an explicit diagnostic and conformance surface and is never a production fallback.

### ADR-0030: Defer the Gorae Managed Runtime Provider from v0.1

**Status:** Accepted

Architecture includes a typed provider extension point, but v0.1 does not implement Gorae Mission, Task, Workflow, Finding, Evidence, Artifact, or Closure presentation.

### ADR-0031: Never bypass Gorae to control Gorae-owned Runs

**Status:** Accepted

When Gorae integration exists, Gul invokes only Gorae's public API for managed activities. Low-level Dolgorae interactions, writer release, and Run mutation remain behind Gorae policy.

### ADR-0032: Model Direct and Managed runtime presentation separately

**Status:** Accepted

DirectSession is not a universal runtime abstraction. RuntimeActivity uses closed typed variants so a future managed hierarchy is not forced into Session/Turn semantics. ControllerInteraction is available only for Direct Sessions controlled by Gul; managed activities expose summaries/escalations.

### ADR-0033: Shared read-only work continues through a dedicated successor

**Status:** Accepted

A `shared_readonly` Run is immutable in lane and cannot become writable. Gul creates a lineage-linked threadless dedicated continuation, stores its distinct Controller binding, creates a new DirectSession without initial writer authority, and keeps the source read-only. When `threadless_acquire_write=false`, writing begins with one explicit `SubmitTurn(write_intent=WRITE)`; a separate Acquire is invalid.

### ADR-0035: Scope a Controller capability to one Run and defer atomic writer handoff

**Status:** Accepted

Gul's security policy assigns one distinct Controller capability to each Direct Session/Run pair and never reuses it across Direct Sessions. This is a Gul blast-radius decision, not a claimed universal Dolgorae invariant. Consequently no two Gul-owned Runs share a Controller, and same-controller writer handoff is unreachable in v0.1.

Writer transfer between Gul Direct Sessions therefore uses provider Release followed by a separate provider Acquire. Gul presents the intervening unowned window honestly and implements no queue, reservation, or retry loop that would imply an atomic transfer. REQ-WRITER-004 keeps its ID and meaning and moves to Deferred State under owner `Deferred-Handoff`.

Rejected alternative: sharing one Controller per Workspace to unlock handoff. It would widen the blast radius of a leaked or lost capability from one Run to every session in that Workspace, and the atomicity it buys is worth little in a single-operator product.

### ADR-0036: Gate in-place write intent on provider access-policy transition support

**Status:** Accepted

Per-submission `write_intent` assumes one Run can serve both read and write turns. That depends on the provider's access-policy transition support, which is reported per profile as supported, unsupported, or unverified and which the provider may refuse for an existing thread.

Gul reads that state and branches: when it is supported for an existing thread, per-submission write intent stands; otherwise the session's access character is fixed after its first Turn and write continuation uses `CreateWriteContinuation`. A new destination is threadless and activates writing through its first `SubmitTurn(WRITE)`, not Acquire. An upstream transition rejection is a typed blocker naming continuation, never a retryable error.

### ADR-0037: Decode provider responses under pinned schemas and fail closed on unknown decisive values

**Status:** Accepted

Gul starts `GetCapabilities` with `RequestContext.protocol_version=0` and its minimum/maximum accepted range, selects only an explicitly supported version and required capability set, and sends that negotiated version in every later request. It validates the Dolgorae and protocol versions, descriptor and credential-schema digests, supported-method inventory, required features, event projection version, Interaction payload contract, and artifact/Interaction bounds before using the generated decoder. Successful wire decoding is not semantic compatibility. Unknown required enum values and malformed typed error details fail closed. Unknown optional data may survive only when the accepted Protobuf compatibility policy explicitly permits it; because Gul is not a transparent proxy, unrecognized data is otherwise discarded at the adapter boundary and never persisted or forwarded.

Run events follow the exact accepted v1 typed Protobuf contract. Each durable-event `oneof` is decoded with generated messages, validated projection stamp and identities, exhaustive variant handling, and an explicit full-update/partial-update/invalidate/follow-up/action-safety rule. Generated envelope decoding alone does not establish binding, monotonicity, or action safety.

Controller Interaction payloads follow the same fail-closed rule. The candidate typed `ControllerInteraction.payload` oneof is mapped exhaustively across command approval, file-change approval, user input, and unsupported variants; envelope/stamp, kind/variant, provider/local size, and per-kind allowlist validation precede every Interaction Card. Production card mapping remains blocked until Gate A publishes that exact form in a clean regenerated artifact set and Gate B pins it for Gul.

The operation, identifier, enum, error, and projection inventory is generated from the accepted upstream contract artifacts rather than transcribed, and a drift check fails when upstream changes. Every provider outcome maps to a stable Gul error code plus a closed provider action class of retryable, operator-action-required, user-choice-required, ambiguous-outcome, or blocked, so clients branch on classification instead of provider text. `SLOW_CONSUMER` is a typed gRPC stream error, not a `RunEventStreamEnd` enum value; `RUN_TERMINAL`, `SERVER_SHUTDOWN`, slow-consumer error, and other transport failure have distinct recovery rules.

### ADR-0038: Invalidate FileService from a bounded host filesystem watcher

**Status:** Accepted

**Replaces the open decision:** file-change refresh

FileService invalidation uses a bounded host filesystem watcher scoped to the verified canonical root, with explicit refresh retained as the fallback. Gul already holds direct read access to that root, so file freshness never requires the broader upstream operational event projection, and `minimal` remains the default projection with no Gul feature depending on `operational`.

The watcher is bounded in watched-node count, queue depth, and coalescing interval; events whose fully resolved real path lies within `.dolgorae/**` are discarded before any listing, status, or preview effect. Invalidation continues to work while the provider is unavailable, preserving FileService independence from LLM execution.

### ADR-0034: Store Controller capabilities as create-exclusive protected files

**Status:** Superseded

Controller capabilities live in create-exclusive owner-only files under a credential root inside application support, outside every Workspace root and every path FileService can resolve. That root is created owner-only, and startup refuses to run if it resolves inside a Workspace or under a FileService-reachable path.

This matches the provider carrier contract exactly. Gul prepares a protected destination, asks Dolgorae to create a new carrier there, verifies the returned location and public metadata, and supplies that carrier in the subsequent Run or continuation request. Every authorized call revalidates the carrier immediately before using it. Credential bytes and carrier paths never appear in browser payloads or gRPC metadata.

Rules the mechanism must satisfy: creation is exclusive and fails on an existing path; replacement is atomic within the same directory; the path is revalidated for owner, mode, file type, and containment immediately before each invocation; a symlinked, foreign-owned, or non-regular path fails closed; loss produces a typed blocker recoverable only through external reset plus verified adoption; successor and lineage runs receive their own file; buffers holding capability bytes are zeroized after use where practical; and diagnostics report only binding health.

Backup and restore are the operator's responsibility over that credential root, documented with the release notes. The accepted residual risk is that a capability sits at rest on disk protected by file ownership, file mode, and full-disk encryption rather than by the system keychain.

Rejected alternatives. Keychain as the primary store was rejected because every carrier-based provider call would have to re-materialize the secret into a temporary file, adding a disk-landing and cleanup-failure path, and because a freshly minted credential arrives in the protected carrier first. Plaintext ordinary SQLite and any location inside a Workspace or FileService-resolvable path were never options.

This decision is retained as history. Its protected-file direction remains, but its claims that Dolgorae creates Gul's credential and that Gul chooses a generic application-support root are replaced by ADR-0047.

### ADR-0039: Separate the Wails shell from the core and loopback listener

**Status:** Accepted

The Go core, ports, configuration, lifecycle, persistence, authenticated loopback HTTP/ConnectRPC listener, Dolgorae supervisor, event aggregation, and FileService run as production `gul serve` without Wails. The desktop shell is a separate task that either starts the same core in-process or attaches to the verified existing user-wide core, then adds WebView delivery and host-native affordances.

One core per user and data directory owns the singleton lock, loopback port, supervised Dolgorae child, and socket runtime. A second `gul serve` exits with an already-running result. Gul.app verifies and attaches to a healthy existing core instead of starting a second runtime. A loopback port occupied by an unverified process is a blocker, never an invitation to choose another port silently.

### ADR-0040: Register workspaces from a configured root allowlist as well as the host picker

**Status:** Accepted

Workspace registration accepts two paths: the host-controlled directory picker, and bounded server-side browsing restricted to a host-configured workspace-root allowlist. A remote client can therefore register a workspace without physical host access.

The original guarantee is unchanged: a browser still cannot submit an arbitrary absolute path. The allowlist is host-configured rather than client-supplied, is canonically resolved when loaded, and bounds every browse. No listing, existence answer, or error may reveal or resolve a path outside it, outside-root probes fail closed with an indistinguishable typed error, and the reserved provider-private subtree stays denied. An empty or unresolvable allowlist disables server-side browsing while leaving the host picker available.

This also removes registration's dependency on the desktop shell, which is what makes ADR-0039 useful rather than merely a reordering.

### ADR-0041: Use release tiers and require every Task complete at release

**Status:** Accepted

Requirements carry a release tier. Release tier is the default and must reach Current State with evidence before v0.1 qualifies. Recommended tier is a named exception that is still implemented when its owning task runs but may be recorded as accepted-incomplete at release. Moving a requirement between tiers needs an accepted ADR and Roadmap change in the same reviewable change, so no task can quietly demote its own requirement. The initial Recommended set is limited to workspace-review presentation extras that already have a Release-tier degradation path.

The release gate additionally requires every Roadmap Task to be `Completed`. The former gate only excluded active and blocked Tasks, so a `Planned` Task passed it. Combined with a Task that owned no requirement, that made it possible to skip the fault-injection proof of no duplicate mutation and still qualify. That Task now owns a requirement, and `Planned` no longer passes.

The single-active-task rule is deliberately retained. Tiers address the risk that an all-or-nothing ledger never closes while the upstream provider is unavailable; they are not a licence to run epics concurrently.

### ADR-0042: Record the accepted product identity and its rename history

**Status:** Accepted

The accepted product identity is Gul, with application `Gul.app`, helper `gul`, repository `gul`, bundle identifier `xyz.rootkernel.gul`, and the `Gul` application-support, log, and cache directories named in Required Specifications. This record exists because the identity changed twice without a decision record, which left the source-of-truth set unable to explain its own name.

The rename chain, recorded here as history only:

| Identity | Role | Disposition |
|---|---|---|
| Kkotge (꽃게) | Working identity of the superseded direct-Codex documentation baseline | Historical |
| Garibi (가리비) | Interim identity used by the originating rebaseline brief | Historical |
| Gul (굴) | Identity accepted for the 0.1-rebaseline | Current |

The identity was set by the product owner. Historical names carry no authority and MUST NOT appear in active normative text; this ADR and explicit provenance notes in the implementation memo are the only places they may appear, and only as history. The terminology gate is therefore refined rather than relaxed: a historical-name scan must return zero hits outside this ADR and those provenance notes.

`prompt.md` is user-owned input, is excluded from version control, and is not itself a contract authority. It may be revised with product-owner corrections; accepted effects must be promoted into all five source-of-truth documents, which then govern implementation.

Scope limit, decided deliberately: this record covers the Gul repository only. The upstream Dolgorae contract still refers to this class of interactive client by an obsolete name. No upstream change is requested as part of Gul's rebaseline. The E0-T7 contract inventory carries that as an annotation, and the annotation must never be treated as current identity evidence.

### ADR-0043: Use Dolgorae local gRPC over a supervised private Unix socket

**Status:** Accepted target; executable pin blocked

Gul supervises one process conceptually equivalent to `dolgorae serve --socket <private-path>`, connects through one reusable standard gRPC channel over that socket, uses unary RPCs for commands and snapshots, and maintains one logical server stream per live Run. No TCP, REST, remote, or production CLI fallback exists. Generated upstream messages stop at the adapter boundary.

The socket pathname lives below `~/Library/Caches/Gul/runtime/` in `0700` current-user-owned non-symlink directories outside every Workspace. Gul creates and validates the parent and chooses an unused absolute pathname. Dolgorae owns its gateway singleton record/lock, bind and `0600` chmod, stale proof and unlink, and graceful cleanup. Gul verifies the resulting socket but never creates, chmods, or unlinks the node. An active gateway without an accepted attach contract fails as `RpcServerAlreadyRunning`; Gul never takes over or silently attaches. The path is never stored in ordinary SQLite or exposed through ConnectRPC or Tailscale Serve.

Startup has a 15-second total budget and a 5-second readiness budget. Shutdown has a 10-second total budget with at most five seconds of unary drain. Crash restart uses 1/2/4/8/16-second exponential delay, a 30-second cap, ±20% jitter, and at most five starts per rolling 60 seconds; five stable minutes reset the budget. Protocol incompatibility or restart exhaustion stops retry. Restarting the RPC gateway does not destroy Runs; Gul re-handshakes, refreshes authoritative state, resumes streams, and marks in-flight mutations outcome-unknown.

The Machine CLI remains available through an explicit diagnostic/conformance adapter only. Its supported schema is validated as a closed exact contract. It is not registered as a production Runtime Provider and is never invoked automatically after gRPC failure.

The accepted contract at Dolgorae revision `85a8862f784cc57701751d81a9e03bf7c5722818` provides the concrete Runtime, Run, Observation, Interaction, Writer, Controller, and Artifact method inventory, including unary artifact chunks and typed Run streams. Gate A closes publication and deterministic descriptor reproduction, while E0-T7 still owns Gul's Gate B pin and generated fixtures. E2-T0 remains blocked until an executable, API version, capability set, typed-error contract, and live smoke evidence are pinned; this ADR is not evidence that runtime software is accepted or implemented.

### ADR-0044: Derive a closed Gul action set and activate threadless writing through the first write Turn

**Status:** Accepted

Dolgorae projections are authoritative state inputs, but they do not provide one Gul-authoritative `allowed_actions` list because they cannot know Gul session ownership, local credential health, browser operation state, or upstream compatibility. A single domain evaluator consumes those inputs and emits only the closed Gul action and blocker variants. Every button and mutation endpoint invokes the same evaluator immediately before action.

The evaluator's explicit typed input contract contains Run lifecycle, thread presence, active Turn, pending Interaction, control mode, execution lane, writer authority/generation, effective access and policy verification, profile capabilities and compatibility, access-policy transition support, background execution, requested/achieved assurance, recovery state and required recovery action, lineage, Controller binding health, Gul Direct Session ownership, unresolved mutation state, and provider compatibility state. Each is independently required even when another aggregate appears to imply it. It does not parse decision-critical strings; a missing, unknown, or string-only input produces `BlockedByProviderCompatibility`. `CanCreateSuccessor` is not a domain action; “Create successor” may be only a presentation label for `CanCreateWriteContinuation`.

The final typed `REQUIRED_CLIENT_ACTION_CREATE_WRITE_CONTINUATION` maps only to `CanCreateWriteContinuation`. A shared-readonly write or unsupported access transition offers continuation, never another write on the source Run. A threadless dedicated Run requiring its first write offers `SubmitTurn(WRITE)`; valid existing-thread acquisition offers `AcquireWriter`; outcome unknown and recovery-required states block conflicts and offer only their typed reconciliation/recovery actions.

For a threadless dedicated Run with `threadless_acquire_write=false`, `AcquireWriter` is unavailable. `SubmitTurn(write_intent=WRITE)` performs the first-write activation and only the accepted projection may confirm effective write policy and writer authority. Acquire remains meaningful only for an eligible existing-thread Run whose current projections and capabilities permit the transition.

### ADR-0045: Require provider timeline and artifact capabilities in v0.1

**Status:** Accepted

The Controller-safe Dolgorae timeline is the authoritative source for runtime conversation reconstruction. Gul may cache approved items for presentation but always reopens from a Run snapshot and timeline cursor, validates Run and Turn identity, merges provider chronology, and publishes one coalesced browser snapshot. Missing timeline capability is a compatibility blocker.

ArtifactCapability is required for large final responses, approval diffs, Controller-only review artifacts, and reconnect. Dolgorae's inline response and maximum-artifact bounds are provider wire capabilities; Gul's 256 KiB browser-inline threshold, preferred 256 KiB unary chunk size, and 64 MiB artifact cap are local presentation/safety limits. Effective download size is the smaller provider/Gul maximum. A provider-inline response above 256 KiB is not a protocol error and may be converted into a bounded browser presentation object. Every artifact still receives expected-length and SHA-256 verification, cancellation, authorization-sensitive classification, and inert safe rendering; its reference is opaque and never treated as a local path by shape alone.

### ADR-0046: Operate one shared core in desktop and headless modes

**Status:** Accepted

`gul serve` is a production mode, not a test harness. It starts the same Go core, authenticated loopback service, ConnectRPC routes, Dolgorae supervisor, event aggregation, persistence, and FileService used by Gul.app. A user `launchd` agent may own it at login. Gul.app attaches to an already running verified core; otherwise it may start that core in-process while holding the same singleton lock.

Tailscale Serve is verified to target only the authenticated loopback listener, Funnel is prohibited, and the Dolgorae socket is never remotely exposed. Sleep/wake, browser reconnect, graceful upgrade, log ownership, and port collision share one lifecycle in both modes.

### ADR-0047: Gul locally creates Dolgorae-schema Controller carriers under the Dolgorae-owned root

**Status:** Accepted

The public Dolgorae API verifies Controller carriers but does not create them for Gul. Gul therefore owns `DolgoraeControllerCredentialStore` with `Create`, `Validate`, `ResolveCarrierReference`, `RemoveUnused`, and a same-principal successor operation. Dolgorae remains authoritative for credential meaning, binding, and authorization and verifies the supplied carrier before StartRun, CreateWriteContinuation, or another authorized call.

The store writes schema version 1 beneath `~/Library/Application Support/Dolgorae/controller-carriers/gul/<gul-installation-id>/`. A credential contains a UUIDv7 Controller ID, `interactive_client` kind, stable trusted-local installation ID, stable trusted-local account subject ID, and 32 cryptographically random bytes encoded as unpadded base64url. Files are exclusive-create `0600`, parents are `0700`, no overwrite or symlink is permitted, file and parent are fsynced, and secret buffers are cleared where practical. Ordinary SQLite stores only a logical relative key; every authorized call derives and revalidates the absolute carrier for containment, owner, type, mode, and symlink absence.

A continuation receives a distinct Controller ID and capability at generation 1 while preserving the source `kind`, `subject_id`, and stable Gul `instance_id` individually. It also preserves the normalized principal `(kind, subject_id)` when subject ID exists, otherwise `(kind, instance_id)`; a matching subject never permits a different installation identity. Its credential identity and idempotency key are persisted before the first call and retained across ambiguous responses. One credential per Direct Session/Run remains a Gul security policy rather than a universal Dolgorae invariant. Operator capabilities remain prohibited. External Controller adoption is a Gul application workflow—not a credential-store capability—and requires host-controlled selection, local carrier validation/resolution, side-effect-free provider `VerifyController`, atomic binding replacement, and fresh Run including recovery/configuration, Writer, Interaction, and timeline reads as needed.

Rejected alternatives are a provider `CreateControllerCredential` RPC that does not exist, browser-selected carrier paths, generic Gul application-support storage, plaintext SQLite, workspace storage, and creating a fresh destination credential after an ambiguous continuation response.

### ADR-0048: Converge provider aggregates with ProjectionStamp before enabling mutations

**Status:** Accepted

Dolgorae Run events are notifications or partial projections, not replacements for every affected aggregate. Gul therefore treats the upstream event invalidation matrix as a mandatory minimum. Gul may add local presentation invalidations, but it may not remove a provider-required Run, Writer, Interaction, or timeline invalidation or required refresh.

Every cached Run, Writer, and Interaction aggregate retains the complete provider `ProjectionStamp`; timeline pages retain `captured_head_cursor`. An event records the minimum stamp or captured head that a later read must satisfy. A later stamp supersedes an earlier invalidation, equal complete stamps are compatible, and continually advancing state keeps the affected action disabled instead of combining revisions. A partial event may disable an action immediately but may never enable one until all required aggregates are fresh and compatible.

`RuntimeProjection` is therefore a Gul-composed presentation over independently validated aggregates. The action evaluator consumes the aggregate stamps, timeline head, and invalidation set in addition to the semantic provider state. On Gul or gateway restart all provider aggregates become stale until refreshed. Upstream cursor checkpoints remain separate from aggregate stamps and from Gul delivery sequences.

### ADR-0049: Retain operation-class-specific replay material instead of persisting prompts or secrets

**Status:** Accepted

Gul must survive a lost allocation response without minting a second Run, but it must not turn every user request into durable retry data. `StartRun` and `CreateWriteContinuation` therefore receive crash-safe exact replay. Before transmission Gul stores their bounded canonical non-secret request material in an exclusive owner-only `ProviderReplayStore`, persists its digest and logical reference with the operation attempt, and reuses the same idempotency key and Controller identity after restart. Exact carrier reconstruction uses role-tagged logical references rather than a single binding: StartRun records its destination credential-store key and expected Controller ID; continuation records the source Direct Session binding/source Controller ID plus the destination credential-store key/expected destination Controller ID. No absolute carrier path or capability is persisted. The replay file is removed immediately after authoritative terminal resolution. An unresolved replay file has a fixed v0.1 maximum retention of 72 hours, configurable only downward. Startup and at-least-six-hourly purges delete expired canonical material, mark replay unavailable, preserve the non-secret operation attempt as `OutcomeUnknown`, and use only the documented secondary authoritative reconciliation without minting a replacement key or silently repeating the mutation.

The replay store may contain bounded Controller instructions or handoff text that is part of the provider idempotency identity. It never contains Controller capability bytes, carrier or socket paths, protected Interaction input, a `SubmitTurn` prompt, or image bytes. It is outside every Workspace and browser/file-service surface and is absent from logs, metrics, traces, diagnostics, and the client event journal.

`SubmitTurn` supports application replay only while the original normalized request remains available in the same process. Gul does not durably retain prompts or images for replay. After process restart it reconciles through `GetRun` and the provider timeline and preserves `OutcomeUnknown` if acceptance cannot be proved. `ResolveInteraction` response bytes are never retained or replayed. Tokenless mutations are reconciled through authoritative reads rather than transport retry. Replay expiry never fabricates success, failure, or permission to issue a semantically new allocation request.

## 6. Proposed decisions

### ADR-0017: Select SQLite driver and concurrency settings

**Status:** Proposed

**Deadline:** E1-T4

Choose between maintained pure-Go and CGO drivers using macOS packaging, cancellation, transaction correctness, backup, and race stability. Also decide WAL, busy timeout, connection limits, and event-sequence allocation. Runtime authority is not a factor because SQLite owns Gul state only.

### ADR-0018: Select SVG preview policy

**Status:** Proposed

**Deadline:** E6-T2

Options are source-only, trusted host rasterization, or sanitized isolated rendering. Direct same-origin rendering is rejected without evidence. Source-only is the required fallback.

## 7. ADR migration matrix

| Old area | New disposition |
|---|---|
| ADR-0001..0003, 0012, 0013, 0015, 0017, 0018, 0020 | Retained with Gul naming or minor boundary clarification. |
| ADR-0004..0008, 0010, 0016 | Explicitly superseded; historical meaning preserved. |
| ADR-0009, 0011, 0014, 0019 | Modified for provider-owned workspace identity, private-root denial, writer state, and two event layers. |
| ADR-0021..0028, 0030..0033 | Accepted runtime-boundary decisions; ADR-0029 is superseded by ADR-0043. |
| ADR-0034 | Superseded by ADR-0047; the protected-file direction remains but credential creation and root ownership changed. |
| ADR-0035..0038 | Rebaseline-review decisions closing gaps found against the upstream public contract: Controller scope and deferred handoff, access-transition gating, forward-compatible decoding, and watcher-based file invalidation. ADR-0038 resolves the former file-change-refresh open decision. |
| ADR-0039..0041 | Rebaseline-review decisions on delivery order and governance: the shell is separated from the core and listener, workspace registration gains a configured root allowlist, and release tiers plus a complete-task release gate replace the former all-or-nothing ledger with a gate that `Planned` cannot pass. |
| ADR-0042 | New record for a change that had none: the product identity and its two prior names. Scoped to this repository; the upstream contract's obsolete client name stays an inventory annotation. |
| ADR-0043..0046 | Accepted local-gRPC transport, closed action evaluation, required timeline/artifact, and shared headless/desktop lifecycle decisions. Exact upstream executable/release qualification remains E2-T0. |
| ADR-0047..0049 | Accepted caller-owned Controller carrier, per-aggregate projection convergence, and operation-class-specific replay-material decisions. |
