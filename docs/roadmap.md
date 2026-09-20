# Gul: Roadmap

| Field | Value |
| --- | --- |
| Role | Sole authority for task identity, order, status, dependencies, and phase |
| Revision | 2026-09-20 consumer-contract rebaseline |
| Active Task | None |
| Next | E12-T1, after the external contract-ready gate; not activated by this plan |
| Required provider | Released Dolgorae v0.1.3 implementing `dolgorae.gul-consumer/v1` |

## 1. Status and execution rules

Allowed executable states are `Planned`, `In Progress`, `In Review`, `Completed`,
`Blocked`, and `Deferred`. `Retired` preserves a historical non-executable ID.
At most one Task is In Progress or In Review globally. This plan activates none.
A Task completes only after its scoped outputs, verification, documentation,
review, and ordinary completion commit requirements pass. No status implies live
credentials, staging, commit, publication, or installation authorization.

Required State and Current State remain separate. A pre-release Task can complete
with explicit fake-provider evidence only for its designated scope. It cannot
claim the corresponding live boundary passed. E9 owns assembled actual-provider
acceptance; earlier Tasks must not defer correctness within their own scope.

Completed E0 work is retained. Existing IDs are not recycled. E0-T1..T3 were
superseded; E0-T5/T6/T9 retired before implementation. Historical E10/E11 IDs
remain reserved. New E12 Tasks carry the consumer-contract rebaseline; IDs do
not prescribe execution order. E4-T4 moves to Deferred because write continuation
is not in this first-release profile, without changing its historical meaning.

### 1.1 Permanent identity registry and Active Task pointer

[task-identities.json](task-identities.json) is the canonical append-only Task ID
allocation registry. It records identity permanence and non-reuse only; this
roadmap remains the sole owner of status, phase, order, dependencies and activation.
`roadmapPresence=required` means the canonical Task row must remain, even when
Deferred or Retired. `reserved` preserves historically allocated, non-executable
IDs that must never be reused or inserted as new executable Tasks. No fixed total
Task count is an invariant. Allocate new IDs by appending registry entries and
matching roadmap rows in the same reviewed change.

The single Active Task header equals the exact In Progress/In Review row set:
None for zero rows, the one Task ID for one row, and never multiple rows. The
validator checks registry/row coverage, immutable committed registry entries and
historical roadmap allocations at first adoption, including retired identities.
In Git checkouts an unreadable baseline is a failure, not a silent waiver. Source
exports without immutable Git evidence must report that only current structure,
not historical nondeletion, was checked. Lifecycle changes do not delete IDs or
change their registry presence classification.

Run `bash scripts/check-sot.sh` and `node scripts/test-sot.mjs`. Negative fixtures
must reject pointer mismatch, removed retired rows, registry/row deletion and
reserved-ID reuse, while preserving DAG, phase, owner, duplicate and link checks.

## 2. External gates and phases

The canonical contract is Dolgorae `docs/specs/gul-consumer-v1.md`, contract ID
`dolgorae.gul-consumer/v1`. It is a producer authority, not a second Gul SOT.
E12-T1 imports its approved immutable lock revision into the consumer inventory.
Never use a floating sibling checkout or copied model prose as the contract pin.

| Gate | Authority and evidence | Blocks |
| --- | --- | --- |
| Contract ready | Dolgorae TASK-053 completed with checked additive wire, 27 required methods, exact descriptor/credential/policy hashes, clients, and fixtures | E12-T1 and implementation based on the new provider contract |
| Pre-release Gul ready | E12-T3: all required mock/core/UI/auth/files/history flows pass with explicit fake provenance | E2-T0 and live integration sequence |
| Provider released | Exact published Dolgorae v0.1.3 artifact and matching consumer lock after TASK-026 plus separate RC QA | E2-T0; not pre-release Tasks |
| Actual Gul accepted | E9-T1..T3 against the released artifact and actual Gul core/browser | Gul first-release qualification and actual-Gul BH1 only |

The producer sequence is fixed and MUST NOT be reordered: TASK-025 is Completed
at `57e6be8`; next is TASK-053, followed by TASK-047, TASK-048, TASK-049,
TASK-050, TASK-051, TASK-054, TASK-055, TASK-052, TASK-056, and TASK-026.
TASK-053 freezes checked wire, clients, fixtures, and the exact immutable revision.
TASK-054 completes timeline, TASK-055 supplies aggregate and result reads,
TASK-052 closes the whole session, TASK-056 proves old-client compatibility, and
TASK-026 performs final provider acceptance. Separate RC QA precedes release.

Dolgorae release does not wait for Gul. Gul does not use an unreleased checkout
as a production integration substitute. Contract stubs may exist before the
provider implements them; neither stubs nor fakes advertise live readiness.
Missing an external prerequisite leaves work Planned without occupying the
single active slot. Do not activate a blocked live task while mock work remains.

Phases:

- Historical: E0 accepted tooling and old contract fixtures.
- Pre-release: E12 and applicable E1/E3/E4/E5/E6/E7/E8 work against the new pin
  and explicit fake provider; no real Dolgorae lifecycle or Controller operations.
- Post-release: E2 and E9 actual adapter, gateway, credentials, and integration.
- Deferred: write continuation, separate Gorae integration, and read-only Podway.

## 3. Epic summary

| Epic | Status | Scope |
| --- | --- | --- |
| E0 | Completed | Historical rebaseline, bootstrap toolchain, and old generated contract fixtures |
| E12 | Planned | New consumer pin, realistic fakes, and pre-release readiness |
| E1 | Planned | Core, one frontend, typed APIs, persistence, desktop shell |
| E3 | Planned | Workspace/session presentation and global Profile selection |
| E4 | Planned | Events, interactions, writer presentation, complete timeline and artifacts; continuation deferred |
| E5 | Planned | Projection convergence and ambiguous-outcome presentation against fakes |
| E6 | Planned | Read-only FileService and bounded preview/review |
| E7 | Planned | Responsive UI, prompt-history section, accessibility and IME |
| E8 | Planned | Authentication, browser protection, headless/PWA/tailnet packaging |
| E2 | Planned | Released provider qualification and real integration |
| E9 | Planned | Actual-provider fault/security/E2E and Gul release qualification |
| Deferred-Gorae | Deferred | Separate future managed provider, not Dolgorae orchestration |
| Deferred-Podway | Deferred | Read-only FSM graph/execution visualization after v0.1.3 |
| Deferred-Handoff | Deferred | Atomic writer handoff; no implicit current scope |

## 4. Complete predecessor DAG

External gates are named separately from Task IDs. All dependencies below must
complete. Ready Tasks are executed in the table order unless an approved roadmap
amendment changes it; no dependency can be bypassed. E4-T4 is outside the active
first-release DAG and no required first-release Task depends on it.

| Task | Phase | Status | Predecessors | Owned result and verification |
| --- | --- | --- | --- | --- |
| E0-T4 | Historical | Completed | None | Original five-document rebaseline; historical evidence retained |
| E0-T8 | Historical | Completed | E0-T4 | Pinned bootstrap toolchain and read-only checks |
| E0-T7 | Historical | Completed | E0-T8 | Old contract pin/generated clients/maps/fakes; not the new consumer profile |
| E0-T5 | Historical | Retired | None | Historical identity retained; never executable and never reusable |
| E0-T6 | Historical | Retired | None | Historical identity retained; never executable and never reusable |
| E0-T9 | Historical | Retired | None | Historical executable-pin identity retained; responsibility moved to E2-T0 |
| E12-T1 | Pre-release | Planned | E0-T7; Contract ready | Pin new immutable consumer lock, update sources/generator/clients/maps; exact reproducibility and schema/hash checks |
| E1-T1 | Pre-release | Planned | E12-T1 | Shared headless-capable core, lifecycle and provider ports; no mandatory real-provider startup for fake tests |
| E1-T2 | Pre-release | Planned | E1-T1 | One React bundle and delivery path for shell and browser |
| E1-T3 | Pre-release | Planned | E1-T1, E12-T1 | Typed ConnectRPC/domain ports, explicit browser history/item/state/result reads and CloseOutcome, Gul-owned pagination and references, operation/error contract fixtures; no generic passthrough |
| E1-T4 | Pre-release | Planned | E1-T1 | Gul-only SQLite, auth/presentation/cache/attempt repositories; migration and transaction tests |
| E12-T2 | Pre-release | Planned | E1-T3, E1-T4, E12-T1 | Stateful scenario fakes for all 27 methods and failure/recovery boundaries, never production fallback |
| E1-T5 | Pre-release | Planned | E1-T2, E1-T3 | Wails shell reuses the core/bundle/API, no second authority |
| E8-T1 | Pre-release | Planned | E1-T4 | One-account setup and protected password storage |
| E8-T2 | Pre-release | Planned | E8-T1, E1-T3 | Cookie sessions, revocation, CSRF/Origin and rate-limit tests |
| E3-T1 | Pre-release | Planned | E1-T3, E12-T2 | Host/allowlisted Workspace selection and canonical attachment via fake provider; containment/identity tests |
| E3-T2 | Pre-release | Planned | E3-T1, E1-T4 | Rename/favorite/hide presentation without runtime mutations |
| E3-T3 | Pre-release | Planned | E3-T1, E12-T2 | Passive Orchestrated Session model, Primary binding, GetExecutionState implementation and observer-only member status; no close coordinator or executable placeholder |
| E3-T4 | Pre-release | Planned | E3-T1, E12-T2 | Global Profile/model/lane selection, preprovisioned Policy name and explicit launch configuration |
| E4-T1 | Pre-release | Planned | E3-T3, E1-T4, E12-T2 | Event bridge, separate cursors, coalescing, all typed invalidation variants and duplicate/slow-consumer fixtures |
| E4-T2 | Pre-release | Planned | E4-T1, E12-T2 | Controller interaction cards, safe fetch, approval/denial, protected-response absence and competing clients |
| E4-T3 | Pre-release | Planned | E4-T1, E3-T4, E12-T2 | Shared evaluator including aggregate-close eligibility and explicit interrupt confirmation, writer projection, no active-Turn queue/steering and unavailable-continuation blockers; coordinator remains E5-T1 |
| E4-T5 | Pre-release | Planned | E4-T1, E4-T2, E12-T2 | ListPromptHistory/GetPromptHistoryItem/ListSpecialistResults implementation, bounded Gul paging and full timeline, stable identity/ordinal coverage, public result discovery and verified artifacts |
| E5-T1 | Pre-release | Planned | E4-T1, E4-T3, E4-T5, E12-T2 | Complete CloseRuntime coordinator using prior evaluator and aggregate reads, provider-to-Gul operation correlation, pending/confirmed/unknown/recovery behavior and typed blockers; no blind retry |
| E5-T2 | Pre-release | Planned | E5-T1, E4-T5, E1-T4 | Provider/browser reconnect and stamp convergence using controlled fakes, no success inferred from disconnect |
| E5-T3 | Pre-release | Planned | E5-T2, E12-T2 | Mutation-attempt/replay policy fault tests; history cache never authorizes resend |
| E6-T1 | Pre-release | Planned | E3-T1 | Verified-root FileService guards and all private-subtree/escape denial tests using isolated roots |
| E6-T2 | Pre-release | Planned | E6-T1 | Bounded directory/text/raster/Markdown preview and SVG-safe fallback |
| E6-T3 | Pre-release | Planned | E6-T2 | Refresh, bounded Git review, typed degradation without runtime dependency |
| E7-T1 | Pre-release | Planned | E1-T2, E3-T3 | Desktop panes/mobile navigation and preserved presentation context |
| E7-T2 | Pre-release | Planned | E7-T1, E4-T2, E4-T3, E4-T5, E5-T1 | Separate user-only Prompt History section, full original, Turn navigation, current status and approval priority |
| E7-T3 | Pre-release | Planned | E7-T2 | Korean IME, keyboard and accessibility on supported layouts |
| E8-T3 | Pre-release | Planned | E8-T2, E1-T5, E7-T1 | Headless singleton/launchd/PWA/Serve packaging and negative deployment fixtures, no real provider-ready claim |
| E12-T3 | Pre-release | Planned | E1-T5, E5-T3, E6-T3, E7-T3, E8-T3 | End-to-end fake-provider readiness across auth/workspace/session/history/approval/result/close/reconnect; all unverified live boundaries listed |
| E2-T0 | Post-release | Planned | E12-T3; Provider released | Pin and qualify exact released executable/contract/capabilities; no silent installation or replaced production binary |
| E2-T1 | Post-release | Planned | E2-T0, E1-T3 | Real gateway supervision, private UDS ownership, shared channel, handshake and process isolation |
| E2-T2 | Post-release | Planned | E2-T1, E1-T4 | Actual exclusive carriers under advertised fixed-home root, adoption and authorization, no Operator or child credential |
| E2-T3 | Post-release | Planned | E2-T2, E3-T4, E4-T5 | Actual session start, sequential read/write submit, approval, timeline/history, public session/results and aggregate close vertical slice |
| E9-T1 | Post-release | Planned | E2-T3, E5-T3, E6-T3, E8-T3 | Actual Gul/generated-client integration, restart and mutation-loss faults, bounded stream/artifact/history tests |
| E9-T2 | Post-release | Planned | E9-T1 | Actual security/credential/path/secret canaries, diagnostics and multi-browser authorization review |
| E9-T3 | Post-release | Planned | E9-T2, E7-T3 | Actual supported-device/headless packaging and first-release qualification; truthful provider/consumer evidence |
| E4-T4 | Deferred | Deferred | E4-T3; separately accepted future WriteContinuation capability | Lineage continuation with exact replay and distinct same-principal Controller; not required by the first release |

## 5. Task execution details

Every table row is one coherent task-owned completion unit with focused tests,
SOT synchronization, independent review, and the repository's commit convention.
The temporary [consumer rebaseline dossier](todo/GUL-CONSUMER-REBASELINE.md)
contains acceptance checklists and cross-team handoff; it does not own status.

### Task ownership for session reads and closure

E1-T3 freezes the Gul browser contracts and schema fixtures; it does not claim a
live provider or completed application coordinator. E3-T3 implements passive
session state and GetExecutionState against the explicit fake. E4-T3 completes
the shared action evaluator. E4-T5 implements paginated history, original-content
and result reads. E5-T1 then completes REQ-SESSION-002 and enables CloseRuntime
with typed pending/confirmed/unknown outcomes. It depends on E4-T5 as well as the
evaluator, so required post-close reads already exist. No earlier Task provides
an unsafe temporary close path. E7-T2 integrates the UI; E2/E9 retain all actual-
provider proof. Each Task can satisfy its own acceptance using completed
predecessors, without a dependency back from E3-T3 to E5-T1.

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

### E12-T2: Build consumer-realistic fakes

The fake provider must model durable accepted identity, repeated same-text new
submissions, pagination/captured head, actual versus unresolved closure, pending
approval, public result publication, and wrong-controller/foreign-cursor errors.
Include all known event variants, slow consumers, optional later methods and
unknown required values. Explicit test/development injection cannot become an
automatic production fallback when the real provider fails.

### E12-T3: Prove the pre-release application

Exercise actual Gul core/browser code against fakes, not screenshot-only UI.
Verify a session's prompt sequence and original bodies after refresh and Gul
restart; submit while busy must stay a draft or typed rejection, never a queued
accepted entry. Confirm an Interaction can be answered while the Turn waits.
Close shows pending until authoritative aggregate confirmation, never calls
children, and retains history. Missing provider permits presentation/file work
where safe but blocks runtime mutations. Inventory the remaining live boundaries
for E2/E9 and do not label them passed by inference.

### E2/E9: Qualify the released provider, not a development substitute

Build the actual adapter only after release pinning. Reuse the existing domain
ports, action evaluator, and presentation. Run the same consumer scenarios with
real UDS/gateway/credential semantics and isolated permitted upstream fakes;
actual live Codex scenarios need separate explicit authorization and must be
reported distinctly. Test large user-input and Specialist artifacts discovered
through public methods, multi-page history, gateway/Worker/Gul/browser restart,
and aggregate-close races. These tasks establish actual Gul compatibility;
producer tests alone cannot complete them.

## 6. Historical contract evidence

E0-T7 pinned Dolgorae revision `85a8862f784cc57701751d81a9e03bf7c5722818` and the
old 34-method descriptor/fake-server fixtures. That remains accepted historical
Current State for its original scope. It does not prove the new 27-method
consumer profile, two new observer methods, fixed-home credential semantics,
Prompt History UI, or any product runtime. E12-T1 owns replacement consumer
artifacts; do not reset E0 to incomplete or count old success as new pinning.

E0-T9's executable qualification responsibility remains E2-T0. Retired E0-T5,
E0-T6, E0-T9 and superseded E0-T1..T3 must never be reused. Former E10/E11 Task
identities remain historical even though their functions moved to E4/E5/E9.

## 7. Deferred work

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

The first Gul release requires all non-deferred executable rows above to be
Completed with their own phase-appropriate evidence and E9's actual-provider
acceptance. Every Release-tier requirement must pass; Recommended items retain
explicit accepted-incomplete treatment. Deferred Tasks/requirements are excluded
from the completion denominator. No Task remains active in the released scope.
Actual Gul evidence is identified separately from historical fixtures,
pre-release mock evidence, Dolgorae provider acceptance, and release publication.
