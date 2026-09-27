# Gul: Roadmap

| Field | Value |
| --- | --- |
| Role | Sole authority for task identity, order, status, dependencies, and phase |
| Revision | 2026-09-27 E4-T2 completion |
| Active Task | None |
| Next | E4-T3 |
| Required provider | Released Dolgorae v0.1.3 implementing `dolgorae.gul-consumer/v1` |

## 1. Status and execution rules

Allowed executable states are `Planned`, `In Progress`, `In Review`, `Completed`,
`Blocked`, and `Deferred`. `Retired` preserves a historical non-executable ID.
At most one Task is In Progress or In Review globally. No Task is active.
A Task completes only after its scoped outputs, verification, documentation,
review, and ordinary completion commit requirements pass. No status implies live
credentials, staging, commit, publication, or installation authorization.

Required State and Current State remain separate. A pre-release Task can complete
with explicit fake-provider evidence only for its designated scope. It cannot
claim the corresponding live boundary passed. E9 owns assembled actual-provider
acceptance; earlier Tasks must not defer correctness within their own scope.

### 1.1 Whole-Epic execution contract

Execute one complete Epic at a time in Section 3 order. The required sequence is:

`E0 (Completed) -> E12 -> E1 -> E13 -> E3 -> E4 -> E5 -> E6 -> E7 -> E8 -> E14 -> E2 -> E9`

Epic numbers are permanent identities, not priority numbers. Section 4 groups
all first-release member Tasks by Epic in that same order. Within an Epic, use
Task predecessors and then displayed row order. Do not start an Epic, execute a
few Tasks from another Epic, and return to finish the first one.

Before invoking `$aquarium:epic-handler`, every earlier required Epic must be
Completed, including its member verification and Epic closeout, and the selected
Epic's external gates must be satisfied. This ordering prerequisite applies in
addition to the specific technical predecessors in the Task tables. Within an
Epic, unfinished member predecessors determine order rather than blocking entry.
An unavailable external gate leaves the Epic Planned and the active slot empty.
This planning amendment is not permission to execute or commit any member Task.

First-release Epic membership includes only non-Deferred, non-Retired Tasks.
E4-T4 is not a member of the current E4 delivery scope. Retired E12-T2/T3 are not
unfinished E12 work. An Epic can complete its current scope without activating
those historical or deferred identities; promotion needs a new approved plan.
One member Task remains one task-owned completion commit; an Epic closeout is
separate workflow bookkeeping, not permission to squash unrelated Task work.

Each pending Epic links to the shared implementation dossier in Section 3. Its
closeout removes only that Epic's link after durable information is promoted.
Retain the dossier while another Epic still references it. Only the last consumer
closeout may delete it under the approved workflow envelope.

### 1.2 Permanent identity registry and Active Task pointer

[task-identities.json](task-identities.json) is the canonical append-only Task ID
allocation registry. It records identity permanence and non-reuse only; this
roadmap remains the sole owner of status, phase, order, dependencies and activation.
`roadmapPresence=required` means the canonical Task row must remain, even when
Deferred or Retired. `reserved` preserves historically allocated, non-executable
IDs that must never be reused or inserted as new executable Tasks. No fixed total
Task count is an invariant. Allocate new IDs with registry entries and matching
roadmap rows in the same reviewed change; keep existing registry entries intact.

The single Active Task header equals the exact In Progress/In Review row set:
None for zero rows, the one Task ID for one row, and never multiple rows. The
validator checks registry/row coverage, immutable committed registry entries and
historical roadmap allocations at first adoption, including retired identities.
In Git checkouts an unreadable baseline is a failure, not a silent waiver. Source
exports without immutable Git evidence must report that only current structure,
not historical nondeletion, was checked. Lifecycle changes do not delete IDs or
change their registry presence classification.

Run `bash scripts/check-sot.sh` and `node scripts/test-sot.mjs`. Checks cover
pointer/identity integrity, DAG order, contiguous first-release Epic blocks,
Epic-summary order, phases, owners, duplicates and links. Passing these structural
checks is not evidence that a product Task or an external gate is complete.

## 2. External gates and phases

The canonical contract is Dolgorae `docs/specs/gul-consumer-v1.md`, contract ID
`dolgorae.gul-consumer/v1`. It is a producer authority, not a second Gul SOT.
E12-T1 imports its approved immutable lock revision into the consumer inventory.
Never use a floating sibling checkout or copied model prose as the contract pin.

| Gate | Authority and evidence | Blocks |
| --- | --- | --- |
| Contract ready | **Satisfied:** Dolgorae TASK-053 completed at `21aefe5b2a8dc6fb18a58338090348b23d2f0a4a` with checked additive wire, 27 required methods, exact descriptor/credential/policy hashes, clients, and fixtures | E12 and implementation based on the new provider contract |
| Pre-release Gul ready | E14-T1: all required mock/core/UI/auth/files/history flows pass with explicit fake provenance; E14 closes | E2 and live integration sequence |
| Provider released | Exact published Dolgorae v0.1.3 artifact and matching consumer lock after TASK-026 plus separate RC QA | E2; not pre-release Epics |
| Actual Gul accepted | E9-T1..T3 against the released artifact and actual Gul core/browser | Gul first-release qualification and actual-Gul BH1 only |

The producer sequence is fixed and MUST NOT be reordered: TASK-025 was Completed
at `57e6be8`; TASK-053 was then Completed at immutable commit
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a`. The remaining producer sequence is
TASK-047, TASK-048, TASK-049, TASK-050, TASK-051, TASK-054, TASK-055, TASK-052,
TASK-056, and TASK-026. TASK-053 froze checked wire, clients, fixtures, and the
exact immutable revision. TASK-054 completes timeline, TASK-055 supplies
aggregate and result reads, TASK-052 closes the whole session, TASK-056 proves
old-client compatibility, and TASK-026 performs final provider acceptance.
Separate RC QA precedes release. E12 must pin the immutable TASK-053 commit, not
the current mutable producer worktree; later external gates require fresh evidence.

Dolgorae release does not wait for Gul. Gul does not use an unreleased checkout
as a production integration substitute. Contract stubs may exist before the
provider implements them; neither stubs nor fakes advertise live readiness.

Phases:

- Historical: E0 accepted tooling and old contract fixtures; retained retired IDs.
- Pre-release: E12, E1, E13, E3, E4, E5, E6, E7, E8, E14, in that order, using
  the new pin and explicit test dependencies; no real Dolgorae lifecycle or
  Controller operations.
- Post-release: E2 and E9 actual adapter, gateway, credentials, and integration.
- Deferred: write continuation, separate Gorae integration, and read-only Podway.

## 3. Epic summary in execution order

Every row below is one complete `epic-handler` target except historical E0 and
explicitly Deferred rows. The shared dossier is an execution reference, not a
second owner of membership, order or status.

| Epic | Status | Scope | Dossier |
| --- | --- | --- | --- |
| E0 | Completed | Historical rebaseline, bootstrap toolchain, and old generated contract fixtures | None |
| E12 | Completed | Immutable consumer contract pin only; E12-T1 | [Roadmap](#e12-pin-the-immutable-consumer-contract); [Memo](implementation-memo.md#15-e12-epic-closeout-2026-09-22) |
| E1 | Completed | Core, typed APIs, persistence, one frontend and desktop-shell foundation | [Roadmap](#e1-build-the-application-foundation); [Memo](implementation-memo.md#111-e1-epic-closeout-2026-09-23) |
| E13 | Completed | Stateful consumer fake provider and reusable scenario harness; E13-T1 | [Roadmap](#e13-supply-stateful-consumer-fakes); [Memo](implementation-memo.md#113-e13-epic-closeout-2026-09-25) |
| E3 | Completed | Workspace/session presentation and global Profile selection | [Roadmap](#e3-complete-workspace-and-session-presentation); [Memo](implementation-memo.md#118-e3-epic-closeout-2026-09-26) |
| E4 | In Progress | Events, interactions, writer evaluator, complete timeline and artifacts; excludes E4-T4 | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| E5 | Planned | Safe whole-session close, projection convergence and ambiguous-outcome handling | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| E6 | Planned | Read-only FileService and bounded preview/review | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| E7 | Planned | Responsive UI, mandatory Prompt History, accessibility and IME | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| E8 | Planned | Authentication, browser protection and headless/PWA/tailnet packaging together | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| E14 | Planned | Assembled pre-release application acceptance and live handoff; E14-T1 | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| E2 | Planned | Released provider qualification and real integration | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| E9 | Planned | Actual-provider fault/security/E2E and Gul release qualification | [Shared](todo/GUL-CONSUMER-REBASELINE.md) |
| Deferred-Gorae | Deferred | Separate future managed provider, not Dolgorae orchestration | None |
| Deferred-Podway | Deferred | Read-only FSM graph/execution visualization after v0.1.3 | None |
| Deferred-Handoff | Deferred | Atomic writer handoff; no implicit current scope | None |

## 4. Epic execution units and complete predecessor DAG

All technical predecessors in the tables must complete, in addition to the
whole-Epic ordering rule in Section 1.1. Each Epic below states the result that
can be verified using completed earlier Epics and its own members. Later Epic
features are not hidden completion prerequisites. Unimplemented routes remain
unavailable rather than returning placeholder success.

### E0: Historical foundation, already Completed

No new execution. Preserve accepted evidence and historical identities.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E0-T4 | Historical | Completed | None | Original five-document rebaseline; historical evidence retained |
| E0-T8 | Historical | Completed | E0-T4 | Pinned bootstrap toolchain and read-only checks |
| E0-T7 | Historical | Completed | E0-T8 | Old contract pin/generated clients/maps/fakes; not the new consumer profile |
| E0-T5 | Historical | Retired | None | Historical identity retained; never executable and never reusable |
| E0-T6 | Historical | Retired | None | Historical identity retained; never executable and never reusable |
| E0-T9 | Historical | Retired | None | Historical executable-pin identity retained; responsibility moved to E2-T0 |

### E12: Pin the immutable consumer contract

Entry: E0 Completed and Contract ready. Completion: the new lock, generated
clients, inventories and contract fixtures reproduce exactly. No Gul application,
stateful domain fake or assembled browser flow is required to finish E12.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E12-T1 | Pre-release | Completed | E0-T7; Contract ready | Pinned immutable TASK-053 lock, regenerated sources/clients/maps, and passed reproducibility plus schema/hash checks |

### E1: Build the application foundation

Entry: E12 Completed. Completion: the shared core, browser contracts, SQLite,
bundle and shell compile and pass foundation tests. Use isolated injected ports
and test principals; unavailable product routes fail closed. E1 does not require
E13's scenario fake, E3..E8's features, or actual provider processes. An injected
test principal is never a production authentication bypass. Product access stays
denied until E8 installs real authentication. Full authenticated host/bundle/API
assembly is owned by E14; actual headless provider integration is owned by E2.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E1-T1 | Pre-release | Completed | E12-T1 | Shared headless-capable core, lifecycle/provider/persistence/authorization ports and deny-by-default composition; isolated bootstrap tests without real provider startup |
| E1-T2 | Pre-release | Completed | E1-T1 | One React bundle and delivery path for shell/browser; foundation build and delivery tests, not completed feature flows |
| E1-T3 | Pre-release | Completed | E1-T1, E12-T1 | Typed ConnectRPC/domain ports, explicit browser history/item/state/result reads and CloseOutcome, Gul-owned pagination/references, operation/error fixtures; no generic passthrough or premature coordinator |
| E1-T4 | Pre-release | Completed | E1-T1 | Gul-only SQLite, auth/presentation/cache/attempt repositories; migration and transaction tests |
| E1-T5 | Pre-release | Completed | E1-T2, E1-T3, E1-T4 | Wails shell reuses the core/bundle/API; isolated shell smoke and lifecycle ports, no second authority; authenticated singleton/attach qualification belongs to E8-T3 |

### E13: Supply stateful consumer fakes

Entry: E1 Completed. Completion: a deterministic provider scenario harness over
the frozen ports, independently testable without future application features.
Model provider behavior, not Gul UI or coordinators. Feature Epics consume the
harness; E14 later proves their assembled application. This replaces retired
E12-T2 without changing that historical ID's meaning.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E13-T1 | Pre-release | Completed | E1-T3, E1-T4, E12-T1 | Stateful scenario fakes for all 27 methods and failure/recovery boundaries, with deterministic driver assertions and reusable reset/clock/fault controls; never production fallback |

### E3: Complete workspace and session presentation

Entry: E13 Completed. Completion: canonical workspace attachment, local metadata,
passive aggregate reads and explicit launch configuration against fakes. Session
creation/submit is not a real-provider claim, and close remains unavailable until
E5. Result/history implementation is owned by E4, not an E3 audit prerequisite.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E3-T1 | Pre-release | Completed | E1-T3, E13-T1 | Host/allowlisted Workspace selection and canonical attachment via fake provider; containment/identity tests |
| E3-T2 | Pre-release | Completed | E3-T1, E1-T4 | Workspace and Direct Session rename/favorite/hide/archive/navigation presentation without runtime mutations |
| E3-T3 | Pre-release | Completed | E3-T1, E13-T1 | Passive Orchestrated Session model, Primary binding, GetExecutionState implementation and observer-only member status; no close coordinator or executable placeholder |
| E3-T4 | Pre-release | Completed | E3-T1, E13-T1 | Global Profile/model/lane selection, preprovisioned Policy name and explicit launch configuration |

### E4: Complete events, interactions, history and action evaluation

Entry: E3 Completed. Completion: typed event/projection paths, safe Interaction
handling, shared eligibility evaluator, complete prompt/history/result/artifact
reads against fakes. Close eligibility is testable without implementing E5's
coordinator. Current membership is T1, T2, T3, T5 only; T4 remains Deferred.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E4-T1 | Pre-release | Completed | E3-T3, E1-T4, E13-T1 | Event bridge, separate cursors, coalescing, all typed invalidation variants and duplicate/slow-consumer fixtures |
| E4-T2 | Pre-release | Completed | E4-T1, E13-T1 | Controller interaction cards, safe fetch, approval/denial, protected-response absence and competing clients |
| E4-T3 | Pre-release | Planned | E4-T1, E3-T4, E13-T1 | Shared evaluator including aggregate-close eligibility and explicit interrupt confirmation, writer projection, no active-Turn queue/steering and unavailable-continuation blockers; coordinator remains E5-T1 |
| E4-T5 | Pre-release | Planned | E4-T1, E4-T2, E13-T1 | ListPromptHistory/GetPromptHistoryItem/ListSpecialistResults implementation, bounded Gul paging and full timeline, stable identity/ordinal coverage, public result discovery and verified artifacts |

### E5: Complete safe close and recovery

Entry: E4 Completed. Completion: whole-session CloseRuntime, typed ambiguous
outcomes, reconnect/convergence and operation-specific replay policy against
fakes. All close reads and eligibility inputs already exist; no child commands,
optimistic closure or history-authorized resend is permitted.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E5-T1 | Pre-release | Planned | E4-T1, E4-T3, E4-T5, E13-T1 | Complete CloseRuntime coordinator using prior evaluator and aggregate reads, provider-to-Gul operation correlation, pending/confirmed/unknown/recovery behavior and typed blockers; no blind retry |
| E5-T2 | Pre-release | Planned | E5-T1, E4-T5, E1-T4 | Provider/browser reconnect and stamp convergence using controlled fakes, no success inferred from disconnect |
| E5-T3 | Pre-release | Planned | E5-T2, E13-T1 | Mutation-attempt/replay policy fault tests; history cache never authorizes resend |

### E6: Complete read-only files and review

Entry: E5 Completed. Completion: contained FileService, bounded previews and
refresh/Git review, independently of a live provider. Use isolated filesystem
roots. Source-only SVG is the safe default; no new policy decision is required
unless the implementation proposes a different accepted rendering policy.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E6-T1 | Pre-release | Planned | E3-T1 | Verified-root FileService guards and all private-subtree/escape denial tests using isolated roots |
| E6-T2 | Pre-release | Planned | E6-T1 | Bounded directory/text/raster/Markdown preview and SVG-safe fallback |
| E6-T3 | Pre-release | Planned | E6-T2 | Refresh, bounded Git review, typed degradation without runtime dependency |

### E7: Complete the usable responsive interface

Entry: E6 Completed. Completion: responsive layouts, separate user-only Prompt
History, conversation/approval/status integration, Korean IME and accessibility.
Use completed domain features and the explicit test harness. Authentication and
remote/PWA installation packaging belong to E8, not E7's scoped UI acceptance.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E7-T1 | Pre-release | Planned | E1-T2, E3-T3, E6-T3 | Desktop panes/mobile navigation, actual file-pane integration and preserved presentation context |
| E7-T2 | Pre-release | Planned | E7-T1, E4-T2, E4-T3, E4-T5, E5-T1 | Separate user-only Prompt History section, full original, Turn navigation, current status and approval priority |
| E7-T3 | Pre-release | Planned | E7-T2 | Korean IME, keyboard and accessibility on supported layouts |

### E8: Complete authentication and deployment packaging

Entry: E7 Completed. Execute T1, T2 and T3 together, not in separate early/late
passes. Completion: actual account/cookie/CSRF protection gates the assembled
application, with shell/headless singleton, verified attachment, PWA and tailnet
packaging tests. No remote-ready claim is made before those protections pass.
Deployment tests use isolated fixtures and do not authorize host installation or
live Tailscale changes. Actual-provider deployment qualification remains E9.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E8-T1 | Pre-release | Planned | E1-T4 | One-account setup and protected password storage |
| E8-T2 | Pre-release | Planned | E8-T1, E1-T3 | Cookie sessions, revocation, CSRF/Origin and rate-limit tests across completed feature routes |
| E8-T3 | Pre-release | Planned | E8-T2, E1-T5, E7-T3 | Authenticated shell/headless singleton and verified attach, launchd/PWA/Serve packaging and negative deployment fixtures, no real provider-ready claim |

### E14: Accept the assembled pre-release application

Entry: E8 Completed. Completion: actual Gul core, shell/browser delivery and all
first-release feature routes pass together with explicit fakes, authentication,
files and restart behavior. This is application integration acceptance, not just
fake-harness tests or screenshots. List every remaining live boundary for E2/E9.
This replaces retired E12-T3 and does not require a released provider to complete.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E14-T1 | Pre-release | Planned | E1-T5, E13-T1, E5-T3, E6-T3, E7-T3, E8-T3 | End-to-end fake-provider readiness across authenticated core/bundle/APIs, workspace/session/history/approval/result/close/files/reconnect; all unverified live boundaries listed |

### E2: Integrate the released provider

Entry: E14 Completed and Provider released. Completion: released executable
qualification, real gateway/UDS/carriers, and the full actual-provider vertical
slice. This includes production headless assembly using the already completed
core, authentication, files and UI; it does not rebuild those Epics.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E2-T0 | Post-release | Planned | E14-T1; Provider released | Pin and qualify exact released executable/contract/capabilities; no silent installation or replaced production binary |
| E2-T1 | Post-release | Planned | E2-T0, E1-T3 | Real gateway supervision, private UDS ownership, shared channel, handshake and process isolation |
| E2-T2 | Post-release | Planned | E2-T1, E1-T4 | Actual exclusive carriers under advertised fixed-home root, adoption and authorization, no Operator or child credential |
| E2-T3 | Post-release | Planned | E2-T2, E3-T4, E4-T5, E8-T3, E14-T1 | Actual session start, sequential read/write submit, approval, timeline/history, public session/results and aggregate close vertical slice through the authenticated headless core/browser |

### E9: Qualify actual Gul and the first release

Entry: E2 Completed. Completion: actual-provider fault, security and supported-
device/deployment qualification. Producer tests and pre-release fakes cannot
substitute for actual Gul evidence. Deferred capabilities remain excluded.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E9-T1 | Post-release | Planned | E2-T3, E5-T3, E6-T3, E8-T3 | Actual Gul/generated-client integration, restart and mutation-loss faults, bounded stream/artifact/history tests |
| E9-T2 | Post-release | Planned | E9-T1 | Actual security/credential/path/secret canaries, diagnostics and multi-browser authorization review |
| E9-T3 | Post-release | Planned | E9-T2, E7-T3 | Actual supported-device/headless packaging and first-release qualification; truthful provider/consumer evidence |

## 5. Task acceptance details

The shared dossier supplies scenario checklists and cross-team handoff. The
roadmap tables and Required Specifications own scope and acceptance, respectively.
E1 builds reusable foundations; E13 tests provider behavior independently;
feature Epics prove their own complete scoped behaviors; E14 verifies assembly.
A later integration Epic is not permission to defer a current feature defect.

### E12-T1: Pin the approved consumer boundary

Use the immutable TASK-053 producer revision and lock, not merely a version
string. Re-pin protocol source/descriptor, credential schema, method and feature
profile, errors, mutation policies, and fixtures. Update the generator's former
34-method assumption for the extended descriptor while separating 27 required
runtime methods from the known full inventory. Verify Profile reads have no
WorkspaceRef and the carrier root is the advertised fixed Dolgorae home. Do not
hand-edit generated files. Regenerate and run drift checks with the approved
existing toolchain or a separately justified pin update. A changed dependency
must not silently reinstall tools or alter a user runtime.

### E13-T1: Build consumer-realistic fakes

The fake provider must model durable accepted identity, repeated same-text new
submissions, pagination/captured head, actual versus unresolved closure, pending
approval, public result publication, and wrong-controller/foreign-cursor errors.
Include all known event variants, slow consumers, optional later methods and
unknown required values. Test these through the frozen ports with deterministic
scenario drivers; do not require E3..E8 implementation or duplicate their domain
logic inside the fake. Explicit test/development injection cannot become an
automatic production fallback when the real provider fails.

### Session reads and closure ownership

E1-T3 freezes the Gul browser contracts and schema fixtures; it does not claim a
live provider or completed application coordinator. E3-T3 implements passive
session state and GetExecutionState against the explicit fake. E4-T3 completes
the shared action evaluator. E4-T5 implements paginated history, original-content
and result reads. E5-T1 then completes REQ-SESSION-002 and enables CloseRuntime
with typed pending/confirmed/unknown outcomes. Required post-close reads already
exist. No earlier Task provides an unsafe temporary close path. E7-T2 integrates
the UI; E2/E9 retain all actual-provider proof.

### E14-T1: Prove the pre-release application

Exercise actual Gul core/browser code against fakes, not screenshot-only UI.
Verify a session's prompt sequence and original bodies after refresh and Gul
restart; submit while busy must stay a draft or typed rejection, never a queued
accepted entry. Confirm an Interaction can be answered while the Turn waits.
Close shows pending until authoritative aggregate confirmation, never calls
children, and retains history. Missing provider permits authenticated
presentation/file work where safe but blocks runtime mutations. Prove real
account/session protection on every assembled API and verify test-only injection
is absent from production fallback. Inventory remaining live boundaries for
E2/E9 and do not label them passed by inference.

### E2/E9: Qualify the released provider, not a development substitute

Build the actual adapter only after release pinning. Reuse the existing domain
ports, action evaluator, and presentation. Run the same consumer scenarios with
real UDS/gateway/credential semantics and isolated permitted upstream fakes;
actual live Codex scenarios need separate explicit authorization and must be
reported distinctly. Test large user-input and Specialist artifacts discovered
through public methods, multi-page history, gateway/Worker/Gul/browser restart,
and aggregate-close races. These tasks establish actual Gul compatibility;
producer tests alone cannot complete them.

## 6. Historical evidence and Task migration

E0-T7 pinned Dolgorae revision `85a8862f784cc57701751d81a9e03bf7c5722818` and the
old 34-method descriptor/fake-server fixtures. That remains accepted historical
Current State for its original scope. It does not prove the new 27-method
consumer profile, two new observer methods, fixed-home credential semantics,
Prompt History UI, or any product runtime. E12-T1 owns replacement consumer
artifacts; do not reset E0 to incomplete or count old success as new pinning.
The accepted E12 consumer boundary pins TASK-053 commit
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a` with dependency-lock SHA-256
`8f52ae66e126f37013d7842b2113fc509d21af4e4fc465cecdbeef7e21619f01` and
generated-lock SHA-256
`6284064e720e2220d6960c42faef6a4c13292ce1327f44e00388dc52b2e17d4a`.
The current generated-lock SHA-256 after the Go 1.27.1 toolchain rebaseline is
`96da1b0a5caeffac1bc9d387c4c4e4ee4159daf47e3877ed748d291be9bb5bc0`;
the E12-T1 completion digest above remains historical.
It is contract/tooling Current State only; product runtime and live-provider
evidence remain with their later owners.
The historical pin remains bound to dependency-lock SHA-256
`c4f91aa3e2add1093880684e5c96fdbb6239aef6a85261a0adf6b585e2db8863` and
generated-lock SHA-256
`8a6a614a3a08c585f9a62f74095a0237d47feefba802e2dfa3ce13be5fbe0bf6`.

E0-T9's executable qualification responsibility remains E2-T0. Retired E0-T5,
E0-T6, E0-T9 and superseded E0-T1..T3 must never be reused. Former E10/E11 Task
identities remain reserved even though their functions moved to E4/E5/E9.

The 2026-09-21 reorganization moves two unstarted responsibilities out of E12.
Keep both old IDs and registry entries permanently; neither old Task is marked
Completed and neither is an executable predecessor or current requirement owner.
New E13-T1 and E14-T1 retain the respective deliverables under new Epic ownership.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E12-T2 | Historical | Retired | None | Former consumer-realistic fake task; unimplemented responsibility moved intact to E13-T1 |
| E12-T3 | Historical | Retired | None | Former assembled pre-release readiness task; unimplemented responsibility moved intact to E14-T1 |

## 7. Deferred work

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E4-T4 | Deferred | Deferred | E4-T3; separately accepted future WriteContinuation capability | Lineage continuation with exact replay and distinct same-principal Controller; not a member of current E4 or required by the first release |

### Deferred-Podway: Read-only FSM observation

After v0.1.3, inspect the accepted Podway-to-Dolgorae contract before allocating
implementation IDs. Dolgorae supplies a pinned full graph and workflow execution
snapshot, active node set, per-loop iteration and stable node execution counts.
Gul renders source revisions/freshness and never computes authoritative counts
by incrementing on event arrival. Multiple workflows in one session are distinct.

No frontend or backend API edits the FSM, moves/jumps/skips a node, force-completes
or reruns a node, or resets counts. Only ordinary prompts can request such
changes, and the running LLM decides under Podway rules. Actual Podway state,
not LLM consent, updates the diagram. Feature absence cannot block ordinary
chat, approval, close, or prompt history. This future work is outside first-release
acceptance and has no assigned provider/Gul version yet.

### Deferred-Gorae and Deferred-Handoff

A separate Gorae provider does not own Dolgorae's current Orchestrated Sessions.
Future Gorae activities remain typed and independently authorized. Atomic writer
handoff remains excluded; existing Release then Acquire is not atomic. Deferred
WriteContinuation retains E4-T4 and cannot silently become a release prerequisite.

## 8. Release qualification

The first Gul release requires all non-Deferred, non-Retired executable rows
above and their Epics to be Completed with phase-appropriate evidence and E9's
actual-provider acceptance. Every Release-tier requirement must pass; Recommended
items retain explicit accepted-incomplete treatment. Deferred and Retired Tasks
are excluded from the completion denominator. No Task remains active in the
released scope. Actual Gul evidence is identified separately from historical
fixtures, pre-release mock evidence, Dolgorae provider acceptance and publication.
