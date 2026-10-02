# Gul: Implementation Memo

| Field | Value |
|---|---|
| Role | Non-normative implementation observations, dependencies, risks, and handoff |
| Product | Gul |
| Version | 0.1-dolgorae-consumer-v1 |
| Last updated | 2026-10-02 |

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
`>=1.66.1` minimum and reproduces the checked descriptor. Buf is resolved
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
The pre-release Go 1.27.1 toolchain rebaseline retained this historical completion digest;
its generated-lock SHA-256 was
`96da1b0a5caeffac1bc9d387c4c4e4ee4159daf47e3877ed748d291be9bb5bc0`
after regenerating the Go header with host protoc 36.2.
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

## 1.5 E12 Epic closeout, 2026-09-22

E12 is Completed. Its sole first-release member, E12-T1, pins immutable TASK-053
commit `21aefe5b2a8dc6fb18a58338090348b23d2f0a4a` and reproduces the checked
descriptor, generated clients, 36-RPC inventory, 27-required/9-unavailable
consumer partition, operation and field-source maps, policy registries, and
contract fixtures. Retired E12-T2/T3 remain historical identities and are not
unfinished members.

The completion audit corrected the E0-T8 Bun floor in commit `bcfb543` to the
verified system minimum `>=1.4.2`. On that committed candidate, `make test-e2e`
and `make test` passed with frozen dependency materialization, contract
generation and drift checks, Go and TypeScript checks, SOT validation, breaking
fixtures, and toolchain boundary fixtures. A fresh six-role whole-Epic review
completed with full coverage, passing CI, and no Low-or-higher findings. No
completion criterion remains unmet or unverified.

This closeout establishes only the immutable consumer contract and checked
contract tooling. It does not claim a Gul product runtime, released-provider
compatibility, live credentials, or actual-provider acceptance. E1-T1 is the
next eligible Task but is not activated by this record. The shared consumer
rebaseline dossier remains because later Epics still reference it.

## 1.6 E1-T1 core foundation, 2026-09-23

E1-T1 adds the root Go product module and a delivery-independent core under
`internal/app`. The core owns a small lifecycle state machine and explicit
lifecycle, provider-readiness, persistence-readiness and authorization ports.
Missing dependencies receive fail-closed defaults: headless bootstrap performs
no external provider startup, product access is denied, and provider or storage
availability is never inferred.

Tests inject task-local ports to verify startup/shutdown, authorization-before-
availability ordering, typed unavailable results, failed startup, and rejection
before startup. This is foundation evidence only. It does not claim a listener,
Wails host, browser API, database repository, typed provider capability adapter,
production authentication, or real-provider behavior.

When a lifecycle port reports a successful start, a later caller-context
cancellation does not discard ownership of the running lifecycle; a subsequent
stop still cleans it up.

Epic validation added a repeated race-enabled fixture for a stop deadline while
product access is in flight. It verifies that failed drain restores the running
state, releases transition waiters, and permits a later clean stop.

## 1.7 E1-T2 shared frontend bundle, 2026-09-23

E1-T2 adds one root Bun package with exact React `19.2.7`, React DOM `19.2.7`,
React/React DOM types `19.3.0`, Bun types `1.4.2`, and TypeScript `7.0.2` pins. A Bun build produces one checked React bundle under
`internal/delivery/web/dist`; its manifest records the exact file sizes and
SHA-256 digests. The drift check rebuilds into a temporary directory and
compares the sorted output inventory and bytes. Generation builds into staging
and preserves the prior checked bundle if compilation fails.

`internal/delivery/web` embeds that bundle once; the Wails shell and browser
delivery both use its static handler. Tests compare the
bytes returned through both paths and verify SPA fallback, missing-asset 404s,
non-read method rejection, response headers, manifest and embedded-tree integrity,
exact package pins, type safety, and fail-closed delivered content. Browser source
typechecking excludes Bun globals. This task does not add a Wails host, listener,
ConnectRPC browser API, authentication bypass, product feature flow, or live
runtime assembly.
Epic validation added a double-rename failure fixture: if replacement and
restoration both fail, the error retains both causes and names the preserved
previous bundle. It also removed the unused `ShellAssets` export; tests compare
the embedded assets directly with bytes delivered through the shared handler.

## 1.8 E1-T3 typed browser and provider contracts, 2026-09-23

E1-T3 declares Gul-owned DirectSession history, original-item, execution-state,
specialist-result, and close-outcome messages plus bounded ArtifactPresentation
references in `api/proto`. The pinned Go and TypeScript generators produce
checked clients; `make api-check` compares regenerated inventory and bytes.
These declarations are not registered routes. E3-T3, E4-T5, and E5-T1 retain
their respective read and closure implementation responsibilities.

The consumer-side `contract/port` interface lists exactly the 27 required
public RPC methods without optional-method or generic-action exposure. Its
generated accepted-error catalog is derived from the pinned Dolgorae error
policy, and typed status/detail translation blocks unknown codes, malformed
details, unknown actions, and first-release WriteContinuation. Accepted
runtime and socket path errors use `RUNTIME_PATH_UNAVAILABLE` with
their typed actions; `UNSUPPORTED_PATH_ENCODING` is reserved for Gul's own
path-encoding validation. Gul page tokens
are random server-side handles binding account, session, query, projection and
captured snapshot while keeping provider cursor/head private. The store caps
live handles and expires unused mappings. The root module imports the checked
local contract module through a single exact replacement, and generated Gul
enums receive typed provider dispositions at the domain boundary. Shared
`api/proto/bounds.json` generates Go and TypeScript limits for pages, inline
originals, chunks, and close metadata. Contract fixtures
cover duplicate same-text history, exact Unicode originals, empty continuation,
page bounds, result/state classifications, source unavailability, close
classifications and errors.
This is schema/port evidence only; no provider adapter, authentication, live
history reads, close coordinator, or actual-provider acceptance is claimed.

E1-T3 completed with a committed six-role review covering the staged contract
and port implementation. All ten reported Low findings were resolved in a
bounded post-review correction and verified with focused fixtures, generated
drift checks, typechecking, and the full `make test` gate. The review predates
those corrections; they are not represented as provider-reviewed bytes. No
Medium-or-higher finding, deferred finding, enabled route, or live-provider
claim remains in this task scope.
Epic validation added an invalid-scope issue fixture covering empty account,
session, and snapshot IDs plus unsupported query and projection versions.

## 1.9 E1-T4 Gul-owned SQLite foundation, completed

The pinned `modernc.org/sqlite` `1.57.0` driver now backs an isolated storage
package requiring an owner-only directory and database file, with one writer,
up to four read-only connections, connection-local
foreign keys, `synchronous=FULL`, a five-second busy timeout, and WAL. Its first
transactional migration creates Gul-owned auth, presentation, cache, checkpoint,
delivery, and non-secret attempt records and rejects schema drift. Auth sessions
store only bearer-token digests; Controller bindings retain logical credential
keys, never carrier paths or capability bytes. A path helper checks ownership,
type, mode, and symlinks for each key component; the future provider adapter
must invoke it immediately before each authorized RPC. Caches become stale on restart.
The cache stores separate complete Run, Writer, and Interaction stamps; observation
checkpoints distinguish validated and projection-committed provider cursors. Attempt
records retain replay availability and role-tagged logical Controller references,
while canonical replay material remains outside SQLite.
Delivery sequence allocation uses `UPDATE ... RETURNING` in the same immediate
transaction as its journal insert. Backup checkpoints WAL and publishes an
owner-only `VACUUM INTO` image. Focused fixtures exercise these boundaries;
the package is not yet wired into production startup.
Epic validation added a held-WAL-reader fixture showing that a busy checkpoint
publishes no backup and succeeds after the reader releases its snapshot.

## 1.10 E1-T5 Wails shell foundation, completed

`internal/desktop` owns a small Wails v3 host and an injected lifecycle boundary
around the shared `internal/app` core. The desktop window uses the same checked
bundle handler as browser delivery and registers no separate Wails product
service. Isolated tests exercise start/stop ordering, failure cleanup, and
byte-identical shell assets, canceled-caller cleanup, concurrent shutdown, and
the macOS last-window termination setting. Window shutdown gives the shared
core a five-second stop budget independent of the window caller's context.
The Wails Go module is pinned
independently of the host CLI minimum. The serial `make test` facade, focused
desktop race test, SOT and boundary negative fixtures, and full native review
passed after correction; the final review-predating Low-only test delta passed
the serial facade again. The local SDK emitted a deployment-target linker
warning, so minimum-macOS live WebView behavior is not qualified here.
Authenticated singleton and verified attach remain E8-T3 work; no
provider-ready desktop behavior is claimed.

## 1.11 E1 epic closeout, 2026-09-23

E1-T1 through E1-T5 meet the roadmap's foundation completion criteria. The
core, checked bundle, typed contracts, isolated SQLite repositories, and Wails
shell passed the serial `make test` gate with Go race detection. All six review
roles covered the epic. The current Low findings were corrected and checked
locally; no current Medium-or-higher, completion, or unresolved Low gap remains.
E1 closes only the foundation. Later tasks still own live provider integration,
authenticated attach, enabled browser routes, and production storage wiring.
The shared consumer dossier remains for E13 and the other open epics.

## 1.12 E13-T1 stateful scenario provider, 2026-09-25

`contract/scenario` implements the pinned 27-method `port.PublicContractPort` as
a deterministic test provider. Drivers control the clock and reset, register a
Controller, inject faults before or after acceptance, fail individual streams,
append typed events, publish results, and advance aggregate close through
unknown-outcome recovery. Run operations produce accepted-input and terminal
timeline items. Tests call every required method through the frozen port and
cover replay, distinct same-text submissions, captured-head pagination, original
Unicode content, pending interactions, wrong Controller, durable event variants,
cursor resume, stream isolation, and close recovery. Drivers can advertise known
later methods without creating first-release Gul actions. The provider contains
no Gul UI or coordinator behavior and has no production wiring.
REQ-CONSUMER-002 is accepted only for this independently tested fake boundary;
E14 owns assembled Gul acceptance and E2/E9 own released-provider evidence.

`GOTOOLCHAIN=local go test -race ./scenario` passed in `contract/`; the serial
`make test` gate passed with SOT negative fixtures and contract drift checks.

Whole-Epic validation reopened E13-T1 to correct four fake-scoped findings:
stream-receive fault injection now has a one-shot assertion, malformed replay
bodies fail before identity hashing, limits above the pinned maximum return a
typed invalid request, and a plain Run is rejected as a non-session target.
The stable test-command guide and verified-behavior summary now name the
scenario suite. The correction does not change production wiring or the frozen
contract. E13-T1 returned to Completed after focused race and serial facade
verification; E13 closeout remains separate.

The next whole-Epic review found that `CloseRun` exposed its retained operation
ID pointer in a terminal replay response. `CloseRun` now copies that optional
value into its response and pending-error detail. The close test mutates both
returned values and confirms that the provider's retained ID does not change.

The third whole-Epic review found that colon-joined idempotency map keys could
alias distinct Controller/key or Interaction/key pairs. The scenario provider
now uses structured tuple keys, and tests prove both pairs create independent
operations while exact retries replay the corresponding result. The contract
module's `contract-check` and `test-unit` paths now run Go tests with the race
detector, including concurrent scenario streams. Four E13-T1 document headers
were brought current. These corrections remain fake-scoped; E13 closeout is
separate.

E4 prerequisite inspection reopened E13-T1 on 2026-09-27. Normal events now
capture their complete Run stamp after the event cursor and state revision
advance, so delayed reads and replay retain the original observation. Each
subscriber receives its requested supported projection; omitted settings use
`minimal` version 1 only as a documented fixture default. Acquire/release update
the workspace writer stamp from its state revision. Fresh Run, Interaction and
timeline snapshots read that same writer revision, including when another Run
changed the writer. Writer generation remains a separate field.
The initial correction left the workspace Writer stamp without Run components.
The ledger correction below supersedes that incomplete owner-scope assumption.
Regression tests cover all durable variants, delayed Interaction events,
projection selection and rejection, replay, returned-message isolation, repeated
writer acquisition/release, and reads across two Runs. The focused scenario race
suite and serial `make test` passed. Production wiring and the pinned contract
remain unchanged. E13-T1 returned to Completed after this verification.

A second E4 prerequisite check on 2026-09-27 found that the fixture's cursor
and revision meanings differed from SPEC-006 and the Projection revision
authority in the pinned producer revision. E13-T1 was reopened for this bounded
correction. Run snapshots now report the captured ledger head; events retain
their historical commit head. Event, accepted-Turn and timeline cursors share
canonical decimal Run-ledger sequences. Exclusive reads accept zero and
filtered gaps and reject malformed or beyond-head positions. Timeline pages
capture their own current head rather than implying one immutable multi-page
snapshot. Session-result token semantics remain separate and unchanged.
Interaction revisions record the ledger sequence of the view change, including
lifecycle transitions after the first Interaction. The fixture compares the
current lifecycle with the last committed lifecycle at each ledger update, so
pause, resume and Turn-driven transitions follow the same revision rule. Writer
reads use the current owner Run's stamp and policy, or the required ownerless
empty/zero Run stamp, unknown access, unverified policy and absent lane/assurance.
Heartbeat and stream-end metadata use the captured head without ledger writes.
Public-port tests cover these boundaries, historical replay, returned-message
isolation, newly appended timeline items, fresh owner reads and lifecycle
transitions before and after an Interaction exists. The earlier
ownerless-only Writer assertion and fixed-head timeline expectation were
replaced with the pinned semantics. The focused scenario race suite and serial
`make test` passed; E13-T1 returned to Completed. E4 implementation and production
wiring remain separate.

## 1.13 E13 epic closeout, 2026-09-25

E13-T1 meets the roadmap's independent provider-scenario acceptance over the
frozen 27-method port. The serial `make test` gate passed with race-enabled
contract tests, contract drift checks, and SOT fixtures. Six independent review
roles covered the Epic and confirmed the final correction set. The remaining
Reset replay assertion was added and checked locally after that review. No
current Medium-or-higher finding or unresolved Low disposition remains.
REQ-CONSUMER-002 is complete for the fake provider only; E14 owns assembled
application acceptance, and E2/E9 own released-provider evidence. The shared
consumer dossier remains for the other open Epics.

## 1.14 E3-T1 Workspace attachment, 2026-09-25

`internal/workspace` registers a selected host directory or an allowlist-relative
directory by invoking the pinned `InspectWorkspace` port with no expected ID.
The returned canonical root must identify the same open directory and remain
inside the configured allowlist when browsing supplied the selection. SQLite
migration 2 stores the canonical root, provider ID, and filesystem identity
beside the subject-scoped presentation in one transaction. Revalidation sends
the stored root and expected ID, refuses moved or replaced directories, and
preserves typed provisioning and Profile Server blockers. A new explicit
registration re-inspects and atomically replaces the stale attachment while
retaining its Gul presentation identity and metadata. The pinned contract
adapter is isolated in `internal/workspace/contractprovider`. Local Workspace
presentation updates require an existing attachment, so they cannot create an
unattached entry. The browser-facing Workspace handler exposes only Gul IDs and
display metadata behind the injected trusted principal and core access gate;
it is not mounted as a production route. Host allowlist and picker failures use
an operator-directed unavailable error, while invalid browser paths retain one
selection error. The macOS directory picker is an isolated host adapter;
dismissing it returns an empty success result, while request cancellation remains
a cancellation error. Neither path initializes
a provider workspace, provisions a Profile or Policy, or creates a Git worktree.

Focused Go tests cover the scenario provider, picker aliases, relative browsing,
outside and private path denial, duplicate attachment, typed blockers, provider
identity changes, missing roots, reattachment, and migration from the prior schema.
Registration and revalidation fixtures also verify that workspace contents,
Git metadata, and the surrounding directory remain unchanged. `make test` and the
generated API drift check passed. This is fake-scoped evidence for REQ-WS-001,
REQ-WS-006/007, and REQ-WS-009..012; E6 still owns FileService use of the
verified root, E3-T4 owns launch Policy selection, E8 owns authenticated route
assembly, and E2/E9 own released-provider compatibility.

## 1.15 E3-T2 local presentation, 2026-09-26

`internal/presentation` exposes local Workspace and Direct Session changes over
the subject-scoped SQLite repository. Migration 3 stores favorites separately
from the earlier tables, preserving migration digests. Attached Workspace
rename, favorite and hide actions do not change canonical roots or provider
IDs. Direct Session rename, favorite and archive actions leave provider state
untouched. Repeated Direct Session discovery preserves existing local metadata.
Navigation accepts only visible attached Workspaces and sessions in the selected
Workspace. Hiding a Workspace clears its selection; archiving a session clears
the selected session. The local core access gate checks authorization and
persistence without requiring provider readiness. The generated browser
declarations and unmounted handlers expose these operations with typed request
errors; no production route is enabled.

SQLite reopen, separate subject, invalid name, hidden navigation and offline
provider fixtures passed with `make test`. This promotes REQ-WS-003 and
REQ-DIRECT-007 for local presentation only. E3-T3 owns provider-backed session
discovery, E8 owns authenticated route assembly, and E2/E9 retain live-provider
acceptance.

## 1.16 E3-T3 passive session reads, 2026-09-26

`internal/session` binds a Gul Direct Session to one accepted provider Primary
Run only after Workspace revalidation, trusted Controller-carrier resolution,
GetRun validation and authorized GetOrchestratedSession validation. Migration 4
persists the subject-scoped one-to-one binding and a non-authoritative copy of
Run configuration. Duplicate bind preserves the Gul ID and local presentation;
it refreshes configuration from GetRun. There is no local draft before an
accepted provider session and no draft cleanup path. Removing a Workspace Entry
atomically removes its Gul presentation, navigation and bindings without a
provider call.

The read path coalesces concurrent requests for one session, bounds the
authoritative refresh, and returns explicit fresh, stale or unavailable state.
Aggregate status, policy, revision, counts and recovery come only from
GetOrchestratedSession. Optional Specialist status uses public parent links as
an observation, with no child Controller or action. Member observations and
stale snapshots are capped separately at 256. Gul-owned state versions
are opaque and separate from provider revisions. The browser contract exposes
local session IDs and typed state, not provider Run IDs, carrier paths or raw
close operation IDs. Provider-declared unavailable or recovery-required
aggregates yield stale state only when this process has a prior fresh read;
otherwise the state is unavailable. The handler remains unmounted; E5-T1 owns
closure.
The adapter classifies transport and deadline failures as unavailable and
rejects semantic provider errors as invalid projections.
Session reads with missing local identifiers return `INVALID_REQUEST` with
`FIX_REQUEST` through the browser error contract.

Scenario-backed tests cover three Primary Runs across two Workspaces, wrong
Controller and non-session rejection, provider configuration changes, restart
readback, observer-only Specialists, unavailable counts, local removal and
subject isolation. This promotes fake-scoped REQ-WS-004/008,
REQ-DIRECT-001/015/019 and REQ-SESSION-003. E2/E9 retain released-provider
qualification and E8 owns authenticated route assembly.

The E3 review correction starts the five-second refresh deadline before the
local binding read, so a blocked context-aware repository read cannot hold a
session's refresh flight indefinitely. Focused tests check the five-second
binding deadline and that a timed-out read releases the refresh flight. Browser
tests also pin the observed-member lifecycle
and brokered composition mappings. The architecture's schema inventory now
includes migration 4's `primary_session_bindings` table.

## 1.17 E3-T4 Profile and launch selection, 2026-09-26

`internal/launch` reads the user-global ListProfiles and GetProfile projections
through the pinned provider port. It checks an explicit Profile, model, effort,
lane and assurance against the current selected Profile. It checks the Policy
name against trusted local preprovisioning; Dolgorae confirms it at StartRun.
Compatible projections require a runtime version and known capability
fields; malformed and transport responses fail closed. The result is a
prospective Direct Interactive configuration with an explicit orchestration
intent. It does not create a provider Run or credential carrier.
Accepted semantic errors from GetProfile retain `FIX_REQUEST` or
`USE_SUPPORTED_PROFILE`. Other semantic errors require operator repair.

The unmounted RuntimeService handler and React selector expose the catalog and
configuration check without enabling a route. The selector displays Profile
compatibility, runtime version, models, lanes, maximum assurance and capability
summary. Shared read-only needs an explicit lane choice, visible permanent-use
warning and acknowledgement in both the UI and API. Scenario and browser tests
cover the catalog's compatibility and capability fields, typed Profile errors,
unsupported choices and consent, promoting fake-scoped
REQ-DIRECT-003/009. E2-T3 owns fresh launch revalidation and StartRun; E8 owns
authenticated route assembly, and E9 retains released-provider acceptance.

## 1.18 E3 epic closeout, 2026-09-26

E3-T1 through E3-T4 satisfy their local and fake-provider acceptance criteria.
Workspace attachment, local presentation, passive Primary session reads and
prospective global Profile selection are implemented. The final cross-task audit
checked requirement ownership, persistence, provider and browser boundaries,
subject isolation, refresh concurrency and documentation against the completed
tasks. The six-role review and its correction assessment are complete.

The unmounted feature handlers return `UNAUTHORIZED` with `ABORT` for
authentication and authorization rejections. They preserve the ConnectRPC
status and expose only local error messages. Handler tests verify the typed
detail for missing composition, missing principals, resolver failures and
access denial.

Review corrections bound the entire refresh, including its local binding read,
to five seconds, pinned browser lifecycle and composition mappings, and completed
the schema inventory. The final Low findings added timeout recovery coverage and
reconciled the Current State promotion summary. Those last test and documentation
changes postdate the review and passed local verification. No confirmed
Medium-or-higher finding, pending Low disposition or unmet acceptance criterion
remains. The full `make test` gate, focused session/API race tests and SOT checks
passed on the corrected implementation; focused SOT checks also cover closeout.

E4-T3 is next. E5 owns session close, E8 owns authenticated route assembly, and
E2/E9 retain released-provider qualification. E3 does not establish mounted-route,
assembled-product or released-provider acceptance. The shared consumer dossier
remains for its other roadmap consumers; E3 now links to its canonical outcomes.

### E4-T1 event bridge

`internal/observation` adds separate Cursor and Sequence types, exhaustive
payload-free event mapping, per-Run checkpoint tracking, 64-entry queues and
50 ms notification coalescing. The required refresh port receives every
mandatory aggregate invalidation. A local slow consumer refreshes only its Run;
mutations use separate provider ports. The manager limits one shared provider
to eight streams with deterministic priority and 30-second ordinary demotion
hysteresis. The current components are unmounted; E5 retains reconnect and
aggregate convergence ownership.

SQLite migration 5 adds pending refresh metadata and Gul-only notification
references. Projection invalidations, committed cursor and delivery allocation
share a transaction; validated cursors are recorded separately. Fault tests
abort after projection writes and prove rollback, retained replay position and
no delivery. Reopen and independent local-delivery tests preserve the cursor
boundary. Migration tests cover every retained schema version.

Descriptor-driven generated-wire tests exercise all 15 durable variants,
malformed identities, unknown fields/enums, stamp drift and oversize envelopes.
Scenario-provider tests consume the default minimal stream. Bridge tests cover
coalescing, duplicate replay, finite-stream end/EOF ordering, startup failures,
metadata retention across demotion, stamp regression after reopen, binding
deletion during writer contention, crash uncertainty, queue
saturation, browser coalescing and actual stream-cap enforcement. Focused
race-enabled checks and serial `make test` pass. Completion review confirms the full T1 requirements and all
corrected behavior. The SOT negative
fixtures isolate their selected lifecycle state, so an active task or a changed
Next task does not invalidate the checker tests.

REQ-API-003 adds the unmounted generated ConnectRPC client-event stream,
subject/session-scoped replay, and a snapshot-required boundary after 256
retained notifications. HTTP streaming tests cover replay, subsequent delivery,
initial/overflow snapshot requests and unauthorized callers. REQ-WRITER-002
adds the shared fresh-policy/active-owner WRITE rule, explicit submission flag,
and an unmounted writer-status component. These requirements remain T1-owned;
T3 will consume the rule in its broader action evaluator. No live-provider or
assembled browser evidence is claimed.

### E4-T2 interaction cards and response handling

`internal/interaction` now separates observer pending summaries from fresh
Controller-authorized cards. The checked adapter classifies every advertised
kind and all four payload variants. It rejects unknown fields/enums, mismatched
IDs or stamps, missing decision context, unsafe paths and incompatible payloads.
The selected payload submessage uses the smaller provider limit and the exact
8 MiB local cap. File-change artifacts require matching metadata, ordered bounded
chunks, exact length and SHA-256 before the card exposes their diff.

The service normalizes approval, denial, cancellation and user answers in memory.
It checks raw response size before parsing, applies the smaller provider limit
and 64 KiB cap again before forwarding, and invokes ResolveInteraction once.
Neither bodies nor content-derived hashes are retained. Every attempt refreshes
authorized state: resolved finishes locally, pending requires explicit re-entry,
stale stays non-actionable, and an unreadable outcome remains unknown. Transport
assembly must preserve the pinned no-retry/no-hedging policy.

Non-live sessions receive independent observer-only Run and pending reads every
five seconds under a five-second deadline. The live manager and polling service
are connected by an observer component. The existing delivery transaction emits
only session/correlation invalidations; polling never advances event checkpoints.
The unmounted generated API exposes typed cards and a bounded response body. The
React form clears its fields before sending, clears its byte buffer afterward,
and performs no automatic replay or browser-storage writes.

Verification covers the full kind/variant matrix, actual 8 MiB payload and
64 KiB response boundaries alongside smaller provider limits,
Controller loss, stamp drift, path rejection, artifact corruption, response loss,
competing ConnectRPC clients, complete-question validation, internal revision
consistency, slow-Run polling and regression invalidation, rollback and cursor
independence. Canary tests check cleared response buffers, safe errors/results
and SQLite/WAL absence. Question IDs remain JSON keys but use separate DOM field
names. An isolated Chrome fixture verifies that `__proto__` and `reset` answers
reach one response callback, input fields and buffers are cleared, and browser
storage stays empty. These are fake-provider and isolated component checks.
The E13 scenario harness
is not used as evidence for fresh Controller detail stamps; typed wire fixtures
supply those complete projections. E5 retains recovery/convergence and E8 retains
authenticated product assembly; no live-provider acceptance is claimed.

E4-T2 is complete. Independent review confirmed the original acceptance criteria
and the corrections, including polling recovery at an unchanged stamp. The full
`make test` gate, focused race checks, component/type checks and isolated Chrome
verification passed. No unresolved finding or unmet criterion remains. E4-T3 is
next; the shared action evaluator and distinct interaction outcome presentation
remain in that task's scope.

### E4-T3 shared action eligibility and writer delegation

`internal/action` defines the closed 19-flag result and independently typed
provider and local inputs. The checked adapter validates field presence, enums,
identities, negotiated capabilities and compatible observations before
classification. It preserves the pinned producer's ownerless workspace-status Writer stamp and
other-owner revision domains. The authorized aggregate has an independent
revision and required action; stale aggregate observations disable every action.
OutcomeUnknown keeps ordinary mutations blocked while preserving an explicitly
advertised, fresh Reconcile/Recover instruction with required interruption consent.
That consent covers Primary and background work, member Runs, Spawns, approvals
and unfinished accepted tasks. Any matching pending local call blocks recovery,
including when another attempt has an unknown outcome.

The service resolves subject/session ownership, workspace identity and the
Controller carrier, then reads provider state and local invalidation/operation
metadata. Acquire and Release re-evaluate the same rule used for presentation
and invoke one provider RPC with the fresh Run revision. They return its accepted
Writer projection with actions disabled until another full read. Typed writer
conflict and unsupported-transition errors stay distinct; ambiguous results
remain unknown. No local writer authority, reservation or transfer queue is
stored. A threadless Run without direct-acquire support can activate first write
only through eligible SubmitTurn(WRITE) from its Unknown/Unverified initial policy.
Release preserves the full released-Run projection, including its unowned window.
This task supplies the Submit admission port,
while E2 owns Submit execution.

The Interaction service also requires the shared response flag. Components show
six distinct outcomes, clear response input/buffers, and block another response
on the same card after confirmation or an unknown outcome. The prompt composer
retains drafts during active Turns and sends only on an explicit action after
fresh eligibility. Interruption consent starts unchecked. Close, lifecycle and
adoption results are classifications for their owning coordinators, with no
placeholder mutation routes.

Focused race tests cover state/capability matrices, stamp domains, independent
aggregate actions, close confirmation, guarded API calls, SQLite operation
metadata, accepted provider projections, typed failures and single invocation.
`frontend/browser/verify-actions.py` exercises actual Chrome against explicit
component fakes: all six Interaction outcomes, reserved answer IDs, clearing,
empty browser storage, post-response admission, the unowned release window,
writer conflict, typed WRITE-submit rejection versus ambiguity, retained drafts
and explicit consent/submission. The fixture
is excluded from the product bundle. Generated-wire fixtures model the pinned
producer; they do not qualify a live provider or assembled product. E5 retains
mutation convergence and close coordination; E8 retains authenticated assembly.

E4-T3 completion: the original eleven requirements passed the third independent
completion assessment after the recovery consent and mixed-operation guards were
corrected. Focused race checks, serial `make test` and the actual Chrome fixture
passed. No unresolved finding or unmet criterion remains. E4-T5 is next; the
runtime and product-assembly boundaries above remain in force.

### E4-T5 bounded history, originals, results and artifacts

`internal/history` reconstructs the complete allowlisted timeline and a separate
accepted-human history. It validates chronology and the prefix before assigning
ordinals. Identical text in separate submissions retains separate IDs; repeated
reads and reconstructed caches retain each accepted item's ID. A changed original
or publication cannot overwrite an existing identity mapping. Page tokens bind
the subject, session, source binding, query and fixed snapshot. History reads at
most four provider pages per request, including empty continuable pages. Public
result reads preserve the provider publication head, source revision and capture
time and keep the Primary as artifact owner.

Originals are reread under current Controller authority and preserve exact UTF-8
and line endings. Previews stop at a complete UTF-8 prefix of at most 1 KiB.
Bodies above the 256 KiB browser threshold become Gul artifact references even
when the provider delivered them inline. Artifact retrieval respects both
negotiated bounds and the 64 MiB/256 KiB local size/chunk limits, validates every
chunk and verifies total length and SHA-256 before exposing bytes. References
are opaque; no artifact read resolves a filesystem path. The bounded cache keeps
metadata only, with explicit unavailable state after eviction until a fresh
traversal reconstructs the same IDs. Old page tokens expire on restart. The existing approval-diff adapter also
requires Artifact capabilities and applies negotiated total and chunk limits;
its 8 MiB card bound remains stricter than the general artifact limit. Missing
capabilities, smaller provider limits and cancellation are checked on that path.

The existing three history/result API methods and ArtifactPresentation handlers
are implemented with subject/session guards. `ListConversation` and
`GetConversationEntry` expose the complete timeline using Gul-only DTOs and closed
kind/status enums. All handlers remain unmounted. The renderer permits only
paragraphs, headings and fenced code; script, HTML, image and link syntax stays
inert text. Product assembly remains with E7/E8 and reconnect scheduling with E5.

Tests cover fixed traversal scope under appends, empty continuations, source/query
substitution, exact replay versus identical submissions, concurrent reconstruction,
SQLite reopen, closed/failed/interrupted history, public result paging, large
Unicode originals, negotiated limits, corrupt/truncated chunks, cancellation and
authorization. The scenario-backed test supplies the pinned 1 MiB inline-response
capability explicitly because the generic E13 capability fixture omits that field;
it does not claim that fixture is a complete negotiation response. Checked wire
fixtures cover all five timeline kinds and protocol corruption. The isolated
Chrome fixture checks inert artifact rendering without external resource loads.

E4-T5 completion: all seven owned requirements passed independent completion
review. Reconstruction now rejects changes to an existing timeline item's
Interaction reference, metadata or original source; typed Interaction events
must carry their permitted status and a nonempty safe title. Authorization
failures use the established `UNAUTHORIZED`/`ABORT` contract. Focused race checks,
serial `make test` and the isolated Chrome fixture passed. No unresolved finding
or unmet criterion remains. E4-wide validation precedes E5-T1.

### E4 epic closeout (2026-09-27)

E4-T1, E4-T2, E4-T3 and E4-T5 are complete. The final audit traced all 41 owned
requirements through implementation, behavior tests and canonical documentation.
Independent whole-epic review covered event-to-storage delivery, observer-to-card
refresh, shared action guards, protected response handling and both artifact
read paths. No unresolved finding or unmet completion criterion remains.

Serial `make test`, focused race checks and isolated Chrome Interaction/action/
artifact scenarios passed. These checks cover the pinned provider fakes and
reusable components. They do not qualify a live provider, authenticated product
route or real provider process restart. E4-T4 remains Deferred outside this epic.

The Current State ledger now includes the previously documented T1/T2 outcomes
as well as T3/T5. E4 links to these canonical outcomes; the shared consumer
dossier remains for the other epics. E5-T1 is next and owns whole-session close
coordination. E5 also owns reconnect convergence, E7 the integrated UI, E8 the
protected route assembly, and E2/E9 live-provider qualification.

### E5-T1 safe close and explicit recovery (2026-09-28)

`internal/sessionclose` coordinates whole-session Close through the Primary's
checked Controller binding. It reuses the action evaluator, persists a
non-secret attempt before transmission, and invokes the root once with the
fresh Run revision. Active owned work requires explicit interrupt consent.
The checked adapter applies the 10-second Close and 60-second Recover/Reconcile
limits. It accepts only complete typed responses and recognized pre-acceptance
rejections; transport loss remains unresolved.

SQLite retains request digests and Gul-owned attempt/operation references across
reopen. Concurrent copies of one request share one dispatch, and a changed
request with the same key fails. The provider operation ID stays in the backend.
An accepted or pending response cannot establish closure. Fresh aggregate, Run,
Writer, Interaction and timeline observations must agree, all owned work must
settle, and required artifact reads must succeed. Browser state reads reconcile
stored attempts without resending Close or starting recovery. History and
unrelated execution remain provider-owned.

Recovery consumes the immediate Run separately from the follow-up projections.
Each affected cache remains stale until its own authoritative read succeeds.
Missing credentials and uncertain policy block mutations without inventing
writer release or changing retained owner/generation. Provider status preserves
disconnected, incompatible, busy and degraded distinctions independently from
the retained execution snapshot.

`internal/recovery.Supervisor` applies jittered exponential restart delays,
the rolling five-start budget, a five-minute stable reset and terminal
incompatibility/exhaustion blockers through an injected gateway lifecycle.
It never changes durable Run state. Actual process ownership belongs to E2;
reconnect and startup convergence belong to E5-T2, and the general replay store
and operation policy belong to E5-T3. This includes a dispatcher that exits,
or cannot persist its result, between `Begin` and `Save`: its retained pending
blocker stays closed to conflicting mutations. E5-T2/T3 must identify that
orphan and make it available to authoritative observation without resending
the tokenless operation before product startup is enabled.

The checked scenario test connects the coordinator, SQLite, action/session
adapters and concrete refresher. The scenario now supplies explicit policy
support and reconciliation values and the correct threadless Run variant.
Negotiation fixtures provide the artifact and Interaction limits omitted by
its generic catalog.

Focused Go race checks and serial `make test` pass, including both Go modules,
frontend/API contracts, generation drift, migration, SOT and toolchain failure
fixtures. The final of three six-role completion reviews assessed all nine
requirements as met at this boundary. Review corrections preserve binding-query error classes,
concurrent WAIT receipts and bounded provider identities. The final local Low
pass moved the unchanged Gul identity predicate to the session owner and added
capability, browser health and post-transmission storage-failure checks; it
changed no product behavior. No blocking or undispositioned finding remains.
This evidence covers explicit fakes, reusable components and unmounted handlers. E7 owns the assembled session
UI; no live provider or authenticated product route is enabled. Release notes:
`not-enrolled`.

| Requirement | Accepted implementation and verification |
| --- | --- |
| REQ-RUNTIME-009 | Typed provider status in session/API and `ProviderStatus`; stale-retention, error mapping, schema and component tests |
| REQ-RUNTIME-012 | Checked capability and evaluator admission for explicit Recover/Reconcile; unsupported and missing-instruction tests |
| REQ-RUNTIME-018 | Injected gateway restart supervisor; fake-clock jitter, rolling budget, reset, cancellation and terminal-blocker tests |
| REQ-CTRL-008 | Backend credential resolution before admission; missing credential and foreign-subject tests never dispatch |
| REQ-WRITER-006 | Recovery invalidates freshness without fabricating owner/generation; ownerless/external Writer stamp and failure tests |
| REQ-REC-005 | Shared typed eligibility and checked identities, revisions, policy and compatibility; fail-closed adapter/service tests |
| REQ-REC-007 | Retained unknown attempts, independent outcome observation and distinct typed browser statuses; loss/cancellation/reopen tests |
| REQ-REC-010 | Concrete per-aggregate refresher and bounded artifact verification; independent failures, rollback and invalidation-race tests |
| REQ-SESSION-002 | Root-only Close, stable opaque correlation, explicit interrupt consent and whole-aggregate confirmation; checked scenario and API tests |

### E5-T2 provider and browser reconnection completion (2026-09-28)

Release notes: intentional no-note; this pre-release fake/component result is
unmounted and has no user-facing release surface.

The reconnect coordinator closes each subject/session gate before probing the
provider. It reads each bound Run, refreshes dependent snapshots, reads the Run
again, converges the aggregate stamps, and resumes observation. The second
read prevents convergence against a snapshot older than the refresh. A failed
probe, read, refresh, convergence check, or observation admission leaves a
visible blocker. The action service requires this gate and checks it again
before a writer mutation. Browser reconnect reads Gul presentation and current
execution state before replaying bounded delivery records. An initial or stale
read requests a full snapshot. These reads do not dispatch provider mutations.

SQLite marks Run, Writer, and Interaction caches stale on reopen. A provider
disconnect also invalidates their refresh state, records the subject's pending
operations as unresolved, and emits a Gul invalidation. The bound Run and its
committed upstream cursor remain intact. Action reads retain complete
per-aggregate stamps; the convergence transaction refuses an older stamp, an
outstanding event floor, or a snapshot head ahead of the artifact-verified
timeline head. A short timeline metadata read cannot clear pending full-timeline work.
The existing `internal/sessionclose.AggregateRefresher` must finish that work before action
eligibility returns. Its independent Writer and Interaction reads retain their
native stamps, including an ownerless Writer.

Live Runs require a successful Watch before their startup gate opens. A failed
Watch or checkpoint read leaves only that Run stale; healthy siblings can become ready. Runs outside
the eight-stream live window start the existing bounded unary Interaction
poller and report `polling`; their fresh snapshot and convergence still gate
actions. Terminal Runs allow final-state reads after convergence. Action
eligibility continues to deny terminal mutations. After observation admission,
provider recovery emits a second bounded Gul invalidation so a browser tail
that stayed connected during an outage can refresh quiet Runs. Failure to persist that
notification leaves the startup gate closed.

The stream manager can replace a finished subscription on the next supervised
update. A terminal marker performs final refresh and does not reconnect; a Run
that becomes terminal during admission receives a second checked final read. A
shutdown marker retains restarting status; the next subscription refreshes
authoritative aggregates before watching from the committed cursor. Typed
slow-consumer and other transport failures keep their existing per-Run
handling. The contract probe lives in the checked
`reconnect/contractprovider` adapter, shares the pinned descriptor with the
scenario fixture and checks protocol versions and required reconnect methods.
The new core and adapter join the existing action boundary in the foundation
check. These components remain unmounted. E7/E8 own authenticated browser and host
assembly, while E2/E9 own released-provider evidence.

Focused component, persistence and race tests cover restart staleness, an
invalidation racing a read, convergence input rejection and rollback, gateway
loss, terminal streams, live and polling admission, transient recovery, browser
replay order and startup blockers. The first six-role review completed after
exact recovery of one rate-limited role. The second complete review found
terminal-gate, sibling-isolation, shared-poller, and test-coverage gaps. This
correction candidate addresses them. The third complete six-role review found
one missing service-level convergence denial test and low-severity gaps in
timeline progress, final-state admission, checkpoint isolation, persistence
errors, and failure-path tests. This candidate corrects those paths and keeps
the action gate closed until a verified timeline head matches the fresh Run.
The final six-role correction confirmation found no remaining Medium-or-higher
issue. Local settlement then classified `LocalState` persistence failures,
asserted that checkpoint-failed Runs never enter unary polling, preserved a
terminal marker during demotion, and synchronized the current-state handoff.
Focused Go tests and the serial `make test` gate pass on the settled candidate.
The E5-wide audit found that a concurrent disconnect could revoke the gate
while recovery was still waiting on observation, then let that older recovery
publish readiness. A generation guard now rejects the older result and holds
new recovery until disconnect invalidation finishes. A race test pauses
observation across the disconnect and verifies the superseded recovery fails
closed. The correction remains within the unmounted fake/component scope.

| Requirement | Candidate implementation and focused evidence |
| --- | --- |
| REQ-RUNTIME-019 | Compatibility probe, durable invalidation, authoritative refresh and committed-cursor stream resumption; storage and stream tests preserve Run identity and unresolved attempts |
| REQ-PROJ-001 | Reopen and disconnect mark provider caches stale; fresh independent reads replace cached stamps without treating an event as authority |
| REQ-PROJ-004 | Browser state precedes replay; monotonic Gul delivery and bounded snapshot fallback remain separate from provider cursors and mutations |
| REQ-PROJ-014 | Terminal, shutdown, typed slow-consumer, transport and heartbeat paths retain distinct stream states and refresh rules |
| REQ-PROJ-019 | Per-aggregate stamps, event floors and timeline head survive persistence; stale, advancing and fault cases keep action eligibility closed |
| REQ-REC-002 | Browser reconnection returns local presentation, current provider state and replay or snapshot fallback without a mutation port |
| REQ-REC-003 | Provider loss and browser-only interruption follow separate recovery paths; transient, stale and terminal fixtures preserve Gul state |
| REQ-REC-006 | Fail-closed action gate opens only after probe, full refresh, convergence and observation admission; failures expose blockers |

### E5-T3 mutation attempts and replay policy (2026-09-28)

Release notes: intentional no-note. The mutation services and checked adapters
remain unmounted and change no shipped product route.

Migration 7 stores target, deadline, reconciliation route and resolved provider
reference beside each non-secret attempt. Begin serializes overlapping pending
and unknown Controller effects before provider I/O. Database reopen changes
unfinished attempts to `outcome_unknown`; an unfinished Close also becomes
eligible for the existing read-only pending-observation path. Writer
Acquire/Release and protected Interaction Resolve now record an attempt before
the single provider call. Writer errors with uncertain transport retain an
unknown blocker. Interaction response bytes and their digest never enter the
attempt record; a later authorized resolved or stale card can settle an unknown
attempt for the same Controller binding and Interaction ID.

The StartRun coordinator writes a bounded canonical request to an exclusive
owner-only file before it records the attempt. The file contains semantic
request fields and a destination credential-store key, not carrier paths or
capability bytes. The checked adapter reconstructs paths through trusted
resolvers, uses the original idempotency key, and validates the returned Run.
Browser retries read the stored attempt without another call. Recovery repeats
the exact request while material remains available. Expiry deletes the file,
keeps the attempt unknown, and permits only a unique Controller-matched
ListRuns result to resolve it. The component supplies startup and six-hourly
purge; product startup must call it when E8 assembles the runtime.

SubmitTurn retains its normalized request only in memory. Recovery reads the
Run and timeline before any same-process exact-key replay. The checked public
projections do not expose the key that would identify an accepted Turn after
restart, so an absent page, stale cursor, event gap, or matching prompt text
cannot authorize a new submit. A fresh process leaves the attempt unknown
unless an exact provider result proves its outcome. Compatibility gates block
StartRun and SubmitTurn dispatch and recovery during version drift.

Focused persistence, adapter, browser retry, response-loss, process-exit,
retention, overlap, and history-gap tests pass, as does the serial `make test`
gate. Independent review is recorded with task closeout. This is fake/component
evidence only; no authenticated StartRun or SubmitTurn route, product
maintenance loop, or live Dolgorae proof is enabled.

| Requirement | Candidate implementation and focused evidence |
| --- | --- |
| REQ-DIRECT-018 | Architecture Section 6.3 records the mutation policy inventory; checked adapters disable transport retries and fault tests retain unknown attempts and authoritative final projections |
| REQ-DIRECT-020 | StartRun persists canonical request and non-secret attempt before dispatch, reuses an exact orphan file after pre-attempt crash, and replays the same key, Controller and carrier before Controller-matched ListRuns |
| REQ-REC-008 | Fault tests preserve one effect or an unresolved attempt across response loss, restart, browser retry, event gap and compatibility drift |
| REQ-REC-009 | Atomic attempt Begin and migration 7 retain non-secret identity, conflict state, target and reconciliation route before dispatch |
| REQ-REC-011 | Owner-only StartRun replay file, original-key reconstruction, expiry and secondary reconciliation; SubmitTurn memory-only and protected/tokenless no-replay policies |
| REQ-API-002 | Unary Writer browser retries cannot retransmit after a lost response; typed unknown state remains visible through the action boundary |
| REQ-PROMPT-005 | Run/timeline reads and unrelated history items never treat a missing page or matching text as proof that SubmitTurn was unaccepted |

The whole-Epic review found that storage rejected provider-advertised
Recover/Reconcile after an unrelated SubmitTurn or Writer attempt became
`outcome_unknown`. Explicit recovery now passes the overlap check only for an
exact Controller binding and ID. Pending attempts, unscoped records, ordinary
Close, and a second simultaneous recovery remain blocked. Reopen and
pending/unknown tests exercise the boundary without clearing the older
uncertainty. The public Run/timeline projections cannot settle an unknown
SubmitTurn after process exit; Close stays blocked until authoritative evidence
exists. The review also identified missing fault coverage for result
persistence; focused tests now verify that Writer and Interaction failures
after dispatch return typed uncertainty or unavailability without another
provider call. The SubmitTurn adapter rejects unknown protobuf fields before
dispatch, and execution-state reads fail closed if the opaque operation
reference cannot be persisted. The architecture inventory now names migration
7, and the Current State ledger distinguishes completed E5 coordination from
unmounted product assembly.

The independent whole-Epic review found two E5-T3 traceability omissions and
three bounded implementation gaps. The ledger and task evidence now include
REQ-DIRECT-018 and REQ-DIRECT-020. StartRun checks resolved Workspace/replay
containment and rejects a symlink anywhere in the replay root path. If a crash
leaves a replay file before the attempt transaction, the same canonical request
can reuse that file and still records the attempt before provider dispatch.
SubmitTurn now clears in-memory exact requests after ten minutes or when a
four-entry cap evicts them; unresolved non-secret attempts remain. Replay purge
uses the earlier of canonical request time and file modification time, so an
orphan created from an old request cannot outlive the 72-hour semantic limit.
Focused path, orphan, retention, and memory tests cover these corrections. A
failed Close response save remains fail-closed while storage is unavailable.
The same service retains the finished dispatch in memory and persists its
uncertainty before later observation, without sending the call again. Process
reopen converts an unfinished dispatch to an unknown attempt; elapsed time
alone cannot prove its call has ended.

A subsequent whole-Epic review identified a close-operation correlation gap:
terminal Close confirmation requires the provider operation reference returned
by that dispatch and a matching complete/abort intent. A foreign close after a
lost response leaves the original attempt unknown. Reconnect checks the full
pinned consumer method inventory and required Writer/artifact features before
opening the mutation gate. Writer Acquire/Release uses the declared 15-second
deadline; migration 8 records the pre-call Writer revision and generation with
the attempt and records when its dispatch ends. A later fresh, converged
Run/Writer read settles an unknown tokenless attempt only when the dispatch
has ended, the target owner's Writer revision and generation advance, and
the cached stamps remain current in the settlement transaction. Pre-v8
unknown Writer attempts have no baseline and stay unresolved. This records
the current authoritative Writer state without claiming that the original RPC
succeeded. Focused fault, migration, drift and projection tests cover these
boundaries.

### E5 epic closeout (2026-09-28)

E5-T1, E5-T2 and E5-T3 are complete. The final audit traced all 24 E5-owned
requirements through their fake/component implementations, tests and Current
State records. Independent whole-Epic review and correction confirmation found
no remaining blocker, medium-or-higher finding or unmet criterion on committed
candidate `32cf17f`.

Serial `make test`, focused Go race and contract checks, and source-of-truth
validation passed. These results cover checked adapters and isolated components;
the live Dolgorae provider, authenticated product assembly and production
startup remain with E2/E9, E8/E14 and their named later owners. The shared
consumer dossier remains for E6 and the other pending epics. E6-T1 is next.

### E5 replay retention correction (2026-09-29)

Cold validation found that an unrelated file in the owner-only replay directory
could stop the expiry pass before it reached old StartRun material. The replay
store now ignores names outside its canonical key format and uses the file's
modification time when malformed canonical material cannot be read. It still
deletes only expired canonical entries; an unrelated file remains untouched.
The replay regression test includes a Finder metadata file, an invalid key,
and malformed old canonical material alongside an expired request. This is a
fake/component correction to REQ-REC-011; production assembly remains outside
E5.

A further validation pass found two failure paths in E5-T3. After an accepted
`SubmitTurn` response, a failed outcome write now marks the attempt
`OutcomeUnknown`, allowing the same-process exact-key recovery path to run.
The `StartRun` maintenance loop reports a periodic purge error and retries
after one minute while retaining its six-hour regular cadence. A concurrent
deletion of an already resolved replay file no longer aborts the expiry scan.
Focused fault, maintenance-loop, and vanished-entry tests cover the corrected
paths. Concurrent same-key StartRun and SubmitTurn begins now have a storage
regression test across two connections, proving only one dispatch is admitted.
The host must provide the maintenance error reporter and handle an initial purge
error before admitting mutations when it assembles these components.

### E6-T1 verified-root FileService guard (2026-09-29)

The local FileService takes the subject from trusted delivery context and uses
the requested Workspace Entry ID to read its subject-scoped attachment. It
verifies the canonical root's stored filesystem identity on every access. It
resolves relative paths through root-anchored descriptors, permits contained
symlinks, and checks the opened node's current location before returning its
descriptor. Traversal, replacement, non-UTF8 input and resolved `.dolgorae/**`
aliases are denied before content reads. The unmounted ConnectRPC inspection
handler returns only node kind and size and uses the local access gate. The
provider is not queried for ordinary file access, so a disconnected runtime
does not prevent local root verification.

Isolated filesystem tests exercise private descendants, nested aliases,
in-root and escaping symlinks, root replacement, directory movement outside
the root or into `.dolgorae`, and subject isolation. API tests cover trusted
principal gating with an offline provider, indistinguishable unavailable-path
errors and the distinct non-UTF8 error. `REQ-FILE-001..003` are accepted at this
component boundary. The complete cross-surface `REQ-FILE-015` promotion awaits
E6-T2/T3 preview, Markdown, Git and watcher consumers; Submit-image integration
remains with its later owner. The shared accessor is the required entry point
for those consumers, and neither a live provider nor an authenticated product
route is claimed by this task.

### E6-T2 bounded file preview (2026-09-29)

The unmounted FileService API now lists live directories in pages of at most
100 entries through short-lived, bounded descriptor cursors. Each page reads
at most 101 raw names, including one lookahead. It reads bounded text, source,
Markdown and raster previews
through the T1 guarded descriptor. Invalid-byte entry names fail with the
typed unsupported-path error; private-root aliases never become browser
entries. Text has 256 KiB and 4,000-line limits with truncation indicated.
Raster bytes are capped at 4 MiB and checked for PNG, JPEG, GIF or WebP MIME
and dimensions before the browser receives an inert data image. Invalid,
binary or oversized content uses a controlled unavailable preview.

Markdown uses the established HTML-free renderer with a separate, explicit
raster-asset allowance for FileService content. Code-fence references are not
loaded. At most eight local images and
8 MiB aggregate asset bytes are read through the same root guard; external,
escaping and private references remain literal. Assets and Markdown are read
from the selected Working revision for this task. A writer can change Working
between reads, so this is not an atomic filesystem snapshot. E6-T3 adds fixed
`HEAD` comparison and refresh. ADR-0018 accepts escaped source-only SVG;
no SVG is rendered as an image or document.

Isolated filesystem and API tests cover large listing pages, private aliases,
invalid encodings, truncation, all four raster formats, oversize content and
offline provider independence. React tests check inert Markdown, source SVG
and source highlighting. The affected component requirements are
`REQ-FILE-004..008` and `REQ-FILE-012`; no assembled file pane or authenticated
route is claimed. The serial `make test` and generated API/frontend drift
checks verify this task candidate.

### E6-T3 refresh and fixed Git review (2026-09-29)

The unmounted FileService now exposes explicit refresh, bounded host file
watching, direct Git status, and fixed `HEAD` versus Working comparison.
Refresh invalidates retained directory cursors and increments a Workspace
revision without contacting Dolgorae. The watcher tracks at most 512 verified
nodes across at most eight subscriptions, scans at most 4,096 entries per
directory, coalesces changes every 100 ms, and holds one queued invalidation.
On macOS it registers only guarded descriptors with kqueue, avoiding implicit
child watches during directory registration. Resolved private paths do not
join the watch set or trigger updates. A watcher
failure is typed; explicit refresh remains available.

Git commands use bounded output and a five-second timeout under a repository
root contained within the verified Workspace. Status preserves direct and
ancestor changes, including staged and unstaged flags. The comparison pins one
`HEAD` commit for text, Markdown assets, and supported raster previews, then
reads Working through the existing guarded accessor. Added, deleted, renamed,
and missing sides remain explicit. Non-Git, unborn `HEAD`, and unavailable Git
return typed states with the current Working preview. The read-only UI
component shows side-by-side revisions on wide screens, a revision switch on
small screens, and status labels alongside color.

Filesystem, Git, API, and React fixtures cover private filtering, offline
refresh, watcher limits, combined status, revision assets, and degraded Git.
`REQ-FILE-009..011`, `REQ-FILE-013..014`, and `REQ-FILE-016` are accepted at
the unmounted component boundary. Complete cross-surface `REQ-FILE-015` and
assembled file-pane behavior remain with their designated integration owners.
E2-T3 owns the Submit-image handoff and final REQ-FILE-015 promotion; E6-T1
supplied its shared local guard.

### E6 epic closeout (2026-09-29)

E6-T1, E6-T2 and E6-T3 are complete. Final whole-epic audit and independent
review found no remaining task or seam defect on committed candidate
`85b75da`. Serial `make test`, focused Go race tests and source-of-truth checks
passed. The audit corrected REQ-FILE-015 ownership to E2-T3 because its full
acceptance includes Submit images; the shared E6 guard is complete, but that
cross-surface requirement remains Required State. E6's accepted Current State
covers the unmounted FileService component. E7/E8/E14 retain assembled product
and authentication work, E2 retains live provider integration, and the shared
consumer dossier remains available to those pending epics.

### E7-T1 responsive panes and file navigation (2026-09-29)

The checked React bundle contains an operator component with a desktop
workspace/session pane, conversation pane, and read-only file pane. Small
screens use Sessions, Chat, and Files controls. Workspace and session selection
use the existing navigation and Direct Session presentation methods. File
location is kept per Workspace while switching panes or Workspaces. The file
pane uses bounded ListDirectory pages, ReadPreview, GetGitStatus,
CompareFixedRevisions, and explicit RefreshFiles calls through injected typed
clients. A provider-managed denied node cannot be opened. Git status failure
does not suppress a Working preview. A writer-active warning marks previews as
potentially intermediate. The foundation entry remains fail-closed until E8
provides authenticated clients; E14 owns assembled application acceptance.

The Chrome fixture checks desktop and narrow layouts, session selection,
page continuation, retry, and retention after preview, stale-page rejection,
preview and fixed-revision comparison, denied-node behavior, Workspace return,
navigation-write serialization, filtered and empty presentation states, and error recovery.
A Safari screenshot on an iPhone 17 Pro simulator confirms
the small-screen layout and native controls render. The serial `make test`,
frontend bundle regeneration and reproducibility check passed. This evidence
does not establish iPhone Safari interaction behavior, authenticated delivery,
or released-provider compatibility. E7-T2 adds current activity, conversation,
Prompt History, and approval controls; E7-T3 adds browser-scoped keyboard and
Korean input checks on supported layouts.
`REQ-UI-001`, `REQ-UI-002`, and `REQ-UI-004` are accepted at this fake-client
browser boundary. The Chrome fixture supplies their behavior assertions;
`make test` supplies typechecking, bundle reproducibility, and existing
foundation coverage.

### E7-T2 activity, conversation and Prompt History (2026-09-29)

The injected-client operator view now reads GetExecutionState, GetActionState,
ListPending/GetCard, ListConversation/GetConversationEntry and
ListPromptHistory/GetPromptHistoryItem. The selected session's provider,
activity, writer, policy, assurance and request count appear in navigation and
the mobile header. Pending Interaction cards precede activity and conversation.
When eligibility alone is unavailable, navigation retains the provider,
activity, policy and pending count from successful reads and marks writer and
assurance unavailable. Loaded card details stay visible during a stale or
blocked state, with response controls disabled.
The separate Prompt History keeps provider item identity, ordinal and time,
opens the exact full original, and links to its conversation Turn. Inline and
artifact-backed originals render as inert text. Artifact reads accept Gul's
supported text media types, cap content at 64 MiB, and check
metadata, every chunk and the full SHA-256 digest before decoding. Conversation
refresh drops locally expanded content and rereads provider pages. The panels
bound page metadata and inline originals before rendering. Linked Turns outside
the loaded page are fetched by ID. Command streams appear in Interaction cards,
outside the normal conversation.

CloseRuntime uses a fresh typed eligibility snapshot and explicit interruption
consent. A returned pending status stays pending; a confirmed mutation receipt
does not claim whole-session closure without a fresh provider projection. An
ambiguous response or transport failure blocks another close request in the
current view. Typed external-action blockers name the provider-side step and
remove in-product retry controls. A definitive rejection with a typed external
blocker also holds the close control until the view is reopened. Interruption
consent refreshes eligibility without clearing pending cards or activity.
Writer changes retain the E4-T3 one-shot
component behavior. There is no declared SubmitTurn browser RPC, so the
operator view does not report or simulate prompt submission; E2 owns that
boundary and E7-T3 exercises the existing draft component's input behavior.

`python3 frontend/browser/verify-activity.py` passed in real Chrome against
explicit typed fakes. It covers mobile status and Interaction priority,
chronological conversation across refresh and pagination, linked Turns outside
the loaded page, Markdown artifact originals and digest failure, same-text
Prompt History items, close receipts and transport failure, and typed external
blockers. Focused unit tests check typed error mapping and artifact integrity,
including a multi-chunk original, rejected metadata and full-body digest drift.
Provider-page tests cover item, token, preview and serialized metadata limits.
The serial
`make test` and checked bundle verify compilation and existing gates; browser
behavior remains a separate selected check. This is fake-client presentation
evidence, not authenticated assembly, live provider, or supported-device
interaction qualification.

### E7-T3 Korean input and keyboard access (2026-09-30)

The prompt draft and user-input Interaction card now reject submission while
Korean composition is active and while the Enter that committed it remains
pressed. Their contents stay intact until a later explicit submission. The
guard handles both Enter-before-compositionend and compositionend-before-Enter
event orders. It expires when keyup is lost. Input blur releases a guard stranded
by a missing composition-end event, so a later pointer or keyboard button action
can submit. A deliberate Enter within 250 ms of a non-Enter composition commit
may be treated as its commit key and require a second Enter. The draft still
has no mounted SubmitTurn route in the operator view.

Selecting a session focuses the Chat heading. Prompt History links move focus
to the matching conversation Turn, including an entry fetched outside the
loaded page, or to its read error. Reopening the same Turn moves focus again;
refreshing the conversation leaves focus on Refresh; a linked Turn fetched while
the panel is hidden receives focus when the panel reopens. If the operator
focuses another control while a Turn read is pending, the result leaves that
control focused. File navigation focuses
the new path, preview Back control, or restored selected file and clears an old
comparison error on returning to the explorer. Repeated history,
response, and user-input controls have distinct accessible names, and keyboard
focus is visibly outlined.

`python3 frontend/browser/verify-accessibility.py` passed in real Chrome. Its
injected-client fixtures exercised both synthetic commit orders at desktop
size and the Chrome-order commit plus explicit Korean submission at desktop,
tablet, and phone sizes. They checked repeated Turn links and destination
focus at all three sizes; unloaded, failed, and hidden-during-fetch Turn links
at desktop size; and a delayed Turn read while an answer input retained focus.
They also checked submission after lost keyup or composition end, distinct
labels, and visible focus on the workspace recovery button. The existing E4
action and E7-T1/T2 browser checks passed after the change. A macOS Safari
accessibility inspection found the named prompt field and button; entering
Korean text into the field and activating
Send cleared the draft. Safari also focused Chat after session selection and
the linked Turn after the first history navigation. Its accessibility input
did not exercise native composition events. Native iPad and iPhone browser
behavior remains for E9's actual-device qualification. The serial `make test`
covers guard state cases, typecheck, the checked bundle, SOT, and foundation
tests; the selected real-browser check is separate.

### E7 validation corrections (2026-09-30)

The whole-Epic review confirmed a file-pane race: a comparison failure could
arrive after Back and show a preview-specific alert in the explorer. A late
RefreshFiles failure could also appear after navigation. Compare and refresh
now admit results only while the initiating location remains current, including
when the operator leaves and reopens the same preview. File continuation keeps
its existing directory scope. The file browser check delays both failures past
navigation and confirms the old alerts do not appear in the new context.

Successful reselection of the current session now preserves its navigation
activity summary. The failed-selection path already preserved that summary.
The selected-session browser check covers both cases. ListPending still reports
the complete provider request count, while GetCard fan-out is limited to the
first 100 summaries; an oversized list shows a partial-availability notice.
The activity browser check verifies the limit. The checked bundle's
`mountOperator` entry now accepts the writer-active signal and passes it to the
file pane, and the delivery test checks for the warning in that bundle.

Focused tests admit provider pages, linked Turn previews, and inline originals
at their exact size limits. An artifact at the 64 MiB metadata limit proceeds
to a chunk read; the test stops before reading the full body. The corrected
source passed `make test` and the real Chrome file, activity, accessibility,
and action checks. These corrections were checked locally after the static
whole-Epic review; the review covered the preceding committed three-task
range. Authenticated assembly, live providers, and native device composition
remain outside E7's acceptance scope.

### E7 epic closeout (2026-09-30)

E7-T1, E7-T2, and E7-T3 are complete. The whole-Epic audit and independent
review covered their committed range at the injected-client and browser
boundary. The audit found one Low file-pane race. Review findings identified
that race and five other Low corrections; the six issues were resolved in
commit `02a71fe`. The review preceded those corrected bytes. Local
verification of the correction passed `make test` and the real Chrome file,
activity, accessibility, and action checks. The accepted E7 requirements have
no remaining unmet criterion or open validation finding at this scope.

The checked bundle still mounts the fail-closed foundation until E8 provides
authenticated assembly. E2 owns released-provider integration, E9 owns native
device and live-provider qualification, and E14 owns assembled pre-release
acceptance. The shared consumer dossier remains linked from those pending
Epics. E8-T1 is the next queued Task.

### E8-T1 account and password completion (2026-09-30)

`internal/auth` accepts a host-issued, service-bound, one-use setup grant valid
for ten minutes. It rejects missing, forged, foreign, expired and reused grants.
The trusted local app is the approved setup surface. E8-T2/T3 must carry this
permission through the protected native bootstrap; loopback, Origin and a
browser boolean are insufficient authority.

Passwords use the exact policy in ADR-0013: at least 15 Unicode characters,
at most 1024 UTF-8 bytes, and no normalization or trimming. Argon2id uses
version 19, 19 MiB, two iterations, one lane, a random 16-byte salt and a
32-byte key. `api/proto/bounds.json` generates the password character and byte
limits for Go and TypeScript; error and form copy use those constants.
`golang.org/x/crypto` is pinned at `v0.57.0`; its dependency raises
`golang.org/x/sys` to `v0.48.0`. The strict decoder refuses malformed hashes
and different costs before invoking Argon2. Verification returns the durable
subject rather than accepting a browser identity. The service serializes hash
work and returns safe fixed errors for dependency failures.

SQLite migration 9 preserves prior migration bytes and adds `password_account`
with a singleton key and a unique account reference. First setup uses one
immediate transaction for subject and credential insertion. It retains the
sole pre-existing foundation subject, creates a UUID only when no subject
exists, and refuses multiple legacy subjects. `CreateAccount` also refuses a
second subject; subject-isolation fixtures deliberately seed adversarial rows
through test-only SQL. Password hashing does not hold the writer transaction.
Storage receives the encoded hash only.

The injected `FirstRunSetup` form has no username or second-account flow and
omits controls for remote views. It clears the uncontrolled password input
before calling the client, suppresses duplicate pending requests, sanitizes
errors, and uses the existing IME guard. The browser flag affects presentation
only; it cannot authorize the server. Closed local failure categories distinguish
unavailable account data and an already-configured account without displaying
an exception. E8-T2 must map the service's stable categories to these errors,
clear converted password buffers, and exclude passwords from every log.
A failed or cancelled setup call can race a durable commit; check saved account
state before offering another setup attempt. The form remains separate from the
checked fail-closed entry until E8-T2/T3 mount authenticated delivery.

Focused `go test -race ./internal/auth ./internal/storage` and the real Chrome
`python3 frontend/browser/verify-setup.py` passed. Tests cover independent salts,
strict hash decoding, password byte/character bounds, denied setup grants,
concurrent setup within and across services, atomic rollback, retained identity,
reopen and owner-only backup, damaged data refusal, input clearing and generic
errors. A fixed canonical Argon2id answer and an independently derived random-salt
answer pin the actual hash and verification costs. Regression tests also deny a
grant that expires during hashing and sanitize failures during grant issuance.
The browser check verifies both password bounds, closed error categories,
composition protection, failure focus recovery and no browser storage. The serial
`make test` passed after adapting Session and Workspace
fixtures to deny registration by an unregistered subject. It covers both Go
modules, frontend typechecks and unit tests, manifests, checked-output drift,
SOT and the public toolchain-checker fixture. That last fixture is tooling E2E,
not authenticated-product E2E. Six-role static completion review confirmed the
component criteria and the focus correction. Final comment and documentation
corrections passed SOT and whitespace checks.
E8-T1 is complete within this component scope; no authenticated route, native
HTTPS or deployment acceptance is claimed. Release notes are not enrolled.

### E8-T2 authentication implementation (2026-09-30)

The candidate adds AuthService FirstRunSetup, Login, Logout and GetSession to
the Gul-owned Protobuf contract and regenerates its Go/Connect and TypeScript
outputs. Password fields are bounded UTF-8 bytes, cleared on every handler exit.
The service returns only fixed errors and closed browser session state, never a
bearer, browser-selected subject or repository diagnostic.

`internal/auth/session.go` creates random 256-bit bearers, retains their SHA-256
digests and uses a fixed seven-day expiry. SQLite's existing web_sessions table
needs no new migration. Lookup requires the sole saved password subject and a
valid bounded hash. Reopen preserves sessions; a revoked or expired row denies
access. Context binding limits concurrent calls, bounds their lifetime, cancels
calls during local revocation, and checks durable state every 250 ms for idle
streams. Storage failures close streams too. Expiry never extends on reads.

`internal/delivery/api/auth.go` owns exact HTTPS Origin/Host agreement, the
custom browser header, CSRF, protected cookies and fixed process-local rate
windows recorded in ADR-0013. It ignores forwarded identity, denies missing or
duplicate cookies, and requires a separate native credential for setup on the
loopback-IP origin. The host-only grant API supersedes the prior transport
secret. No RPC can mint this permission. `routes.go` registers AuthService and
only supplied completed services behind this boundary; copied handlers share
the server resolver even when a caller supplied a foreign one. Core authorization
requires that same bound context. Missing components remain absent or deny access.

The checked frontend exports `mountAuthenticated` and `createBrowserAuth`.
Password fields are uncontrolled, clear before submission and never enter React
state or browser storage. The typed same-origin HTTPS transport keeps CSRF and
bootstrap authority in memory, clears converted UTF-8 buffers, and checks saved
setup state after a failed reply before offering setup again. Login errors use
fixed categories; the existing IME and failure-focus behavior apply. Expiry,
unauthenticated feature errors and logout remove product content and abort
active feature calls. Server cancellation or a denied feature call rechecks saved session state,
refreshing a rotated cookie/CSRF pair without replaying the failed action. Auth
RPCs have a ten-second deadline. Logout removes protected content and aborts
feature calls before awaiting revocation; an uncertain reply offers a retry and
retains only the CSRF needed to revoke the saved cookie.

Focused Go race tests cover cookie flags, fixed expiry, rotation, logout, reopen,
hash-only rows, ambiguous/damaged account refusal, every declared feature route's
anonymous/Origin/CSRF rejection, auth and feature body limits, shared setup/login
rate windows, local-only setup and
active event/file stream revocation. The first idle-file-stream test waited for
response headers before producing an event, which deadlocked this no-initial-item
stream; the fixture now starts the call concurrently and produces a bounded
invalidation before asserting idle revocation. Bun transport tests and
`python3 frontend/browser/verify-auth.py` cover exact Unicode, setup reconciliation,
CSRF forwarding and rotation recovery, real idle expiry, converted-buffer
clearing, bounded stalled logout, no early product render, pending duplicate
suppression, sanitized failures, focus, Korean IME, expiry and uncertain logout
retry. Chrome uses injected auth; Go uses isolated TLS RPC and SQLite fixtures.

The initial serial `make test` passed before review. Review remediation adds
CSRF rotation recovery, bounded auth deadlines, immediate logout clearing and
regression assertions for request limits, idle expiry and password buffers.
The corrected candidate passed serial `make test` and the Chrome auth check.
The second review confirmed those corrections and identified a setup-failure
readback that incorrectly switched an unconfigured account to sign-in, plus a
missing negative test for the native loopback-IP guard. The setup gate now
retains its form and failure focus after unconfigured readback; exhausted setup
attempts use a fixed wait message. Tests distinguish the IP guard from generic
URL rejection and check ignored forwarded headers and malformed native
authority. The Current topology now distinguishes isolated route registration
from production host mounting, including its SOT validation fixture. These
corrections passed focused Go race, Bun and Chrome checks, serial `make test`,
and the third corrected-target review. That review established three remaining
required evidence gaps: missing/duplicate CSRF and malformed cookie rejection
at the HTTP boundary, browser stream-error session readback, and damaged or
unavailable account state refusing setup. The correction adds tests on the
existing API and browser transport test surfaces. Focused Go race and Bun checks
pass; the frozen evidence correction passed restricted confirmation and serial
`make test`. A service inventory label and already-configured setup instruction received wording-only
corrections. The serial gate includes both Go
modules with race detection, Bun/type checks,
reproducible contract/API/frontend checks, SOT and the toolchain-checker fixture.
That fixture is tooling E2E. Updated review-state documentation also passed
SOT and whitespace checks. E8-T2 is complete within its authenticated
route/component scope. E8-T3 must create the shared HTTPS host,
validate loopback and Serve origins, deliver bootstrap authority without URL,
log or storage exposure, supply the same boundary to core authorization and all
routes, and mount the checked auth entry. The default entry remains fail-closed.
No authenticated host, Tailscale, native installation, real provider or assembled
product acceptance is claimed. E14 and E9 retain their acceptance boundaries.
Canonical follow-up entries E8-FB-001 through E8-FB-008 retain independent
availability, maintenance and optional UX risks.
No current correctness or required acceptance is deferred. Release notes are
not enrolled.

## 2. Current development snapshot

| Area | State |
|---|---|
| Five Gul SOT documents | E0-T4 completed the consumer alignment and Gate A reproduction; E0-T8 completed toolchain/ADR alignment; E0-T7 completed Gate B |
| Toolchain and developer-command artifacts | E0-T8 accepted one pin manifest and read-only host checks; E0-T7 adds checked contract generation/drift delegates; E1-T1 adds the root Go module; E1-T2 adds root Bun pin validation and checked frontend generation/drift commands; E1-T3 adds checked Gul API/error-catalog generation; no installer |
| Contract boundary | E12-T1 pins TASK-053 and regenerates checked clients/maps/fake transport for 36 known, 27 required, and 9 unavailable methods; E13-T1 adds an explicit stateful scenario provider over the 27-method port |
| Production source | E1 shared core, bundle, API declarations, isolated SQLite and Wails shell; E3 adds Workspace attachment and presentation; E4 adds typed observation, Interactions, actions and bounded history/result/artifact reads; E5 adds safe close, reconnect and operation-specific replay; E6 adds verified-root local FileService, bounded previews, refresh, watcher and Git review; E7-T1/T2/T3 add the responsive operator, files, session presentation, composition guard, and keyboard focus through injected clients; E8-T1 adds isolated local auth, bounded Argon2id storage and the injected first-run setup form; E8-T2 adds durable cookie sessions, protected route assembly and the browser auth gate within isolated delivery scope; E8-T3 mounts the authenticated loopback HTTPS host against offline runtime ports; E14-T1 adds checked adapter assembly and authenticated browser/native acceptance with explicit stateful fakes; released-provider assembly remains E2/E9-owned |
| Wails host/frontend | One checked React bundle includes the E7-T1/T2/T3 operator components and fail-closed foundation; E8-T3 implements verified authenticated Wails attachment and native cancellation; E14-T1 exercises E3-T4 launch selection through checked clients; live provider assembly remains E2/E9-owned |
| ConnectRPC schema/services | Gul AuthService and completed feature declarations and generated clients exist; E8-T2 adds protected route assembly; E8-T3 mounts eight feature handlers behind the shared browser boundary; E14-T1 adds the ninth Diagnostics handler and checked runtime assembly |
| Gul SQLite schema | Gul-owned version 9 schema with Workspace attachment, favorites, Primary binding, event metadata, session-close attempts, mutation-attempt details, Writer reconciliation baselines and a singleton password record; E8-T3 owns protected host startup and ordered shutdown integration |
| Dolgorae RPC supervisor/provider | Bounded restart policy implemented against an injected lifecycle; production process ownership and live provider remain pending |
| Controller credential store | Caller-owned mechanism selected by ADR-0047; not implemented |
| FileService/auth/PWA/Tailscale integration | E8-T3 mounts the E6 FileService and E7 checked operator entry behind authentication, with network-only PWA delivery and read-only Tailscale admission; live deployment and provider qualification remain E2/E9-owned |
| Current State promotions | Prior E0/E12/E1/E13/E3/E4/E5 entries plus component-scoped REQ-FILE-001..014 and REQ-FILE-016; E7-T1 fake-client REQ-UI-001/002/004, E7-T2 fake-client REQ-OUT-001/002/004/007, REQ-UI-003/006/009/010 and REQ-PROMPT-001, and E7-T3 browser/component-scoped REQ-OUT-006 and REQ-UI-008; E8-T1 account/service/component-scoped REQ-AUTH-001/002, E8-T2 authenticated-route REQ-AUTH-002/003/004, and E8-T3 isolated host/delivery-scoped REQ-HOST-007/009/010, REQ-NET-001..004 and REQ-UI-007; E14-T1 assembled fake-provider REQ-HOST-001/002, REQ-API-001 and REQ-CONSUMER-004; cross-surface REQ-FILE-015 and released-provider qualification remain pending |

The repository contains a mounted authenticated HTTPS host, nine feature handlers
including E14-T1 Diagnostics, protected Gul-owned SQLite lifecycle,
verified native attachment, and one checked browser/PWA bundle. E8-T3 completion
review accepts its eight-handler delivery scope; E14-T1 and E14 accept the
assembled fake-provider scope. Default runtime ports fail closed as unavailable.
Released provider wiring, Controller credentials and live deployment
qualification remain E2/E9-owned.

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
| Go/Wails/Node/Bun/TypeScript/React | Host Go exactly `1.27.1`; Wails `>=3.0.0-beta.8` (`wails3`); Node `>=26.7.0`; Bun `>=1.4.2`; project TypeScript `7.0.2`; React/DOM `19.2.7` | `toolchain/versions.env`; exact Go and non-Go host minimum checks; exact project manifests; clean-host fixtures | E0-T8; ADR-0054 |
| Protobuf/Buf/ConnectRPC | Host Buf `>=1.66.1`; protoc `>=35.1`; project protoc-gen-go/protobuf-go `1.36.12`; connect-go/protoc-gen-connect-go `1.20.0`; Connect-ES/Web `2.1.2`; Protobuf-ES/protoc-gen-es `2.14.0`; historical protoc-gen-connect-es `1.7.0` | Host minimum checks; exact project manifests; Buf lint and generated-output drift checks | E0-T8; ADR-0054 |
| Host platform | macOS `>=14.0.0`, exact `arm64`; Git `>=2.39.0` | Minimum checks for macOS and Git; exact architecture check; no installation or mutation | E0-T8; ADR-0054 |
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
| Local draft Direct Session behavior | Resolved by E3-T3 | No Gul draft before an accepted provider session; repeat binding reuses the Gul ID and preserves local presentation. | E3-T3 |
| SVG preview | ADR-0018 Accepted | Escaped source-only rendering is the first-release policy. | None |

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
| RISK-025 | Gul offers Acquire before the first write on a threadless Run or mishandles unsupported transition. | Medium | High | ADR-0044 action matrix, explicit threadless Acquire rejection, first SubmitTurn(WRITE), and a typed unsupported-transition blocker; E4-T3. Future continuation remains with Deferred E4-T4. |
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
| ADR index/body | 54 index IDs and 54 body IDs/statuses match; ADR-0018 accepts source-only SVG and ADR-0054 defines the host-version policy | E6-T2 source review |
| Roadmap active Task | 36 executable Tasks plus retired E0-T9: all three E0 Tasks `Completed`, E2-T0 `Blocked`, and zero Active Tasks | Passed 2026-08-23 completion transition |
| Bootstrap pins | One data-only manifest records the exact Go version, minimum versions for other host tools, and exact project dependency and generator versions; manifest validation rejects missing or malformed authority | Passed `make test` 2026-09-22 |
| Clean-host and drift behavior | Minimum-version fixtures accept newer hosts and reject old or missing Wails; project validation and generation still reject pin or output drift | Passed fixture and current-host check 2026-09-22 |
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
| 19 | No Task occupies the Active Task slot. | Pass |
| 20 | E1 and E1-T1 through E1-T5 are `Completed`, while later implementation tasks retain their roadmap state. | Pass |
| 21 | Current State records the reviewed core task evidence without promoting incomplete assembled-product requirements. | Pass |
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

The root Makefile provides the serial facade `toolchain-check`, `generate-contract`, `contract-check`, `generate-frontend`, `frontend-check`, `generate-api`, `api-check`, `test-prepare`, `test-unit`, `test-int`, `test-e2e`, and `test`. Generation rewrites only its named checked outputs; `contract-check`, `frontend-check`, and `api-check` reject drift without rewriting them. `test-prepare` validates the Go and product manifests, the Make recipes, shell syntax, exact Go, and Bun minimum. It then downloads the root Go graph, installs the root locked Bun graph, downloads the contract Go graph, and installs the contract locked Bun graph; both Bun installs disable lifecycle scripts. `test-unit` repeats the manifest and Make-recipe checks, typechecks the frontend and generated API clients, tests the frontend/API contracts, runs both Go suites, and executes the contract fixtures. `test-int` checks the exact Go toolchain again before module tidiness, frontend bundle drift, Gul API drift, SOT validation, contract drift, and whitespace checks. The checking targets do not rewrite tracked files, and `test` invokes the four test phases serially. E0-T7's checked delegates require exact generator versions, verify imported source hashes, reproduce the descriptor and all tracked outputs, run the all-RPC fake-server tests and TypeScript checker, and validate generated policies plus the separate Machine CLI fixture. Later Tasks extend these stable targets for frontend/browser E2E and opt-in pinned-runtime smoke tests. Real provider tests remain opt-in and must never reveal Controller capabilities, carrier/socket paths, or protected input.

The current host check requires Go exactly `1.27.1`, requires architecture to
match `arm64` exactly, and reports Wails, Node, Bun, Buf, protoc, Git, and macOS
against minimum versions. It does not inspect PATH copies of project
generators. Project Go and Bun manifests provide the exact generator and library
pins, while contract generation proves that their output remains reproducible.
Missing-command and below-minimum fixtures prove fail-closed diagnostics. The
checker does not install or silently substitute tools.

### E0-T8 completion record

E0-T8 completed on 2026-08-23 after serial tests and independent review. It promotes REQ-HOST-005 for the bootstrap pin/reporting surface and accepts ADR-0017. ADR-0054 keeps Go exactly pinned, changes other host executable checks to minimum versions, and leaves exact project pins and generated-output validation intact. The accepted artifacts are `toolchain/versions.env`, the root Make facade, `TESTING.md`, and the read-only scripts under `scripts/`. This promotion proves the bootstrap contract only; no product runtime behavior is claimed.

### E0-T7 completion record

E0-T7 completed on 2026-08-23 with Dolgorae revision `85a8862f784cc57701751d81a9e03bf7c5722818`, dependency-lock SHA-256 `c4f91aa3e2add1093880684e5c96fdbb6239aef6a85261a0adf6b585e2db8863`, generated-lock SHA-256 `8a6a614a3a08c585f9a62f74095a0237d47feefba802e2dfa3ce13be5fbe0bf6`, generated clients, and a fake server covering the historical 34-RPC descriptor. E12-T1 later replaced the live lock and generated paths, so those paths are not evidence bytes for the E0 pin. The historical completion promotes REQ-RUNTIME-011 and REQ-RUNTIME-022 within its original contract/tooling scope only; E2-T0 still owns the compatible executable and live smoke boundary.

## 11. Superseded design note

The initial documentation assumed Gul would manage one Codex App Server, map Session to thread, store Turn/interaction state and a persistent workspace writer lock in SQLite, filter raw App Server events, manage background-process safety, and reconcile App Server state after failure. ADR-0004 through ADR-0008, ADR-0010, and ADR-0016 preserve that history as Superseded. None of those assumptions may guide production work.

## 12. Handoff

E12, E1, E13, E3, E4, E5, E6, E7 and E8 are complete. E8-T1/T2/T3
are accepted within their account/password, authenticated route and isolated
host/delivery scopes. E14-T1 assembled fake-provider acceptance is complete.
E14 is Completed; E2-T0 follows after the Provider released gate. The
TASK-053 consumer lock and generated contract tooling remain authoritative. E6 provides guarded local FileService
inspection, bounded preview, refresh and Git review over E3's saved Workspace
attachment. E2-T3 retains the Submit-image handoff and final REQ-FILE-015
acceptance. E2-T0 remains blocked on an accepted compatible Dolgorae executable
and live smoke evidence. No product route or live-provider behavior is activated
automatically.

### E8-T3 authenticated host implementation (2026-09-30)

`internal/host` owns one core, protected SQLite and certificate state, a stable
owner lock and the shared loopback HTTPS listener. The fixed port never falls
back after collision. The desktop verifies the held lock, private owner record,
TLS leaf and a fresh challenge before opening the Wails view; a second headless
invocation stops with an already-running result. Closing an attached desktop
window leaves the headless owner running. A failed owned lifecycle stop retains
the lock until retry succeeds. Restart preserves the certificate and durable
account/session state; it does not silently replace an expired certificate.

The local native exchange has separate private host authority. Ordinary local
or remote HTTP requests cannot mint setup grants. Wails pins the exact loopback
leaf and adds the setup permission at document start only in its verified main
frame. The frontend consumes and deletes that field. Password setup, login,
logout and feature calls use the existing shared authentication boundary.

The default checked entry now starts same-origin authentication and typed
operator clients. Missing runtime services remain unavailable. The production
host never chooses a scenario fake or the Machine CLI. Full feature assembly
and released-provider qualification remain E14 and E2/E9 responsibilities.

PWA metadata, icons and a network-only service worker are included in the checked
bundle. The worker stores no responses. Refresh restores only a presentation tab;
authentication and selected Workspace/Session are read again from the server.
The browser handler admits only same-origin Connect, worker, manifest and image
resources under CSP, with explicit manifest and PNG content types.

Focused host race fixtures exercise real TLS account setup/login, forged native
requests, owner-record and certificate mismatch, stale ownership, port collision,
unsafe permissions and symlinks, lifecycle-stop failure and restart. The opt-in
real Wails/WebKit race check passes for the certificate pin and native field
across two same-origin navigations using an isolated probe. Chrome checks of the
actual host and checked entry pass login, safe refresh with fresh auth/navigation,
network-only storage, offline refusal and reconnect, logout and forged setup
denial. Chrome and the isolated host close successfully. Serial `make test` passes.
The separately approved installed-PWA check also passes actual installation,
standalone launch, safe tab refresh with fresh authentication/navigation,
network-only storage, logout and uninstall in a temporary Chrome profile. It
sets Chrome's app display mode explicitly and leaves no Gul app shim. Completion
source review now accepts the isolated host/delivery scope. These fixtures do not qualify live Tailscale,
launchd loading, system sleep, real providers or supported devices.

Completion review identified certificate-startup and CLI lifecycle coverage gaps.
Focused tests now reject expired and damaged certificate bytes without replacing
them, preserve a headless owner after attached-window close, drain a desktop-owned
host, and check dispatch and headless cancellation. Diagnostics no longer create a
missing state directory. Host, Serve and launchd share one tailnet endpoint parser;
invalid endpoints fail at startup. The misroute and Unix-socket fixture files are
valid JSON and reach policy validation. Tailscale completeness now requires the
checked 1.102.4 source revision and matching CLI/daemon versions. Other listener
failures retain their underlying cause rather than reporting a port collision,
and clean stop followed by wait returns no error. These corrections have completed corrected-target source assessment.

The E7-FB-002 entry-point follow-up is covered by a bounded isolated Bun test: all
public mounts reuse one React root, reject a changed or missing root, and carry
writer activity through the operator mount. The feedback entry is removed.
Global single-account rate windows retain the approved brute-force bound; origin
splitting would weaken that bound. Sustained deployed availability remains the
E8-FB-001 re-entry condition. Host composition and startup visibility follow-ups
are retained in the canonical deferred-feedback owner.

Direct inspection also found that post-start failure cleanup discarded a failed
core-stop result. That path now retains the lock and store until explicit Stop
succeeds, including when no HTTP server remains. The fixture refuses publication
of unsafe owner metadata, fails the first stop, blocks another owner, then permits
a verified restart after cleanup succeeds. Contract validation now tracks the
roadmap host state instead of requiring the retired unauthenticated-shell claim.

Authenticated FileService mounting also activated E6-FB-003 and E7-FB-001.
Connect transport fixtures now cover watcher setup cancellation and unavailable
setup/stream errors: cancellation has no DomainError, while unavailable delivery
carries SOURCE_UNAVAILABLE. A canceled-context handler test uses the real file
service. The unavailable-watcher fixtures inject explicit errors; they do not
claim a live watcher outage or provider qualification. FilePane now merges pages
by name within the current Workspace/directory, preserving order and replacing
metadata from overlapping entries. The real Chrome fixture injects an overlapping
continuation after a failed page, verifies one row per name, and retains the
stale-response and context-switch checks. Both feedback entries are removed.
E6-FB-002 retains its watcher-driven refresh re-entry condition. E7-FB-003 is
settled by direct browser assertions for delayed Prompt History original success
and failure after another selection or refresh; superseded responses cannot
replace the current original or restore an obsolete error.

The second completion review identified desktop signal handling, initial-load
failure, forwarded local-Host routing and a stale Current State summary. The
native window now receives cancellation, bounds initial loading to 30 seconds,
and returns from the AppKit event loop so Go reports errors and drains an owned
core. Isolated real Wails checks cover SIGTERM, trust-install failure before any
HTTPS request, an incomplete HTTPS load, blocked popup loading and two pinned
navigations. Host restart checks preserve a session cookie and password verification.

Qualified Tailscale Serve overwrites forwarding headers while retaining inbound
Host. The outer host gate denies a proxy-marked request claiming the local-only
Host; those headers grant no authority. A real isolated TLS proxy fixture proves
forged loopback Host/Origin cannot reach assets, auth or native bootstrap, with or
without a selected remote endpoint, while direct local access remains available.
The architecture summary now describes the mounted authenticated host and its
Gul database without promoting live-provider acceptance.

Boundary checks cover host, deployment, auth and the isolated browser Go helper;
auth retains its generated password-bounds dependency. Additional checks cover
writer response presence, overlapping file metadata and order, writable log
parents, unexpected listener failure, verified-owner tailnet diagnostics, and the
unavailable-runtime documentation boundary. Diagnose distinguishes missing state,
invalid input and unsafe existing state without creating directories. Concurrent
inspection can return a temporary 503; observer lock contention remains bounded
and fail closed under E8-FB-011. Corrected-target source review accepts this scope.

### E8-T3 scoped completion (2026-10-01)

Completion source assessment accepts the shared authenticated host, isolated
launchd/Serve packaging and checked browser/PWA delivery. The final confirmation
also verifies that the Current architecture consistently describes the mounted
File, presentation, Direct Session, Runtime, ClientEvent, WriterAction,
Interaction and Artifact handlers, and host-opened persistence. It preserves
the separate full-application and live-provider acceptance boundaries.

Ownership is published before HTTP serving, so a failed owner-record write
cannot admit a handler against persistence being closed. Failed lifecycle stop
retains the stable lock and store until retry succeeds; a direct account read
now verifies store retention during that interval. A deterministic hosted active
HTTP-handler drain assertion remains future regression coverage under E8-FB-013,
before drain-order changes or deployed upgrade/restart qualification.

Serial `make test`, focused host race tests and SOT negative fixtures pass.
The unchanged production bytes retain the actual Wails/WebKit pin, SIGTERM,
trust-failure, initial-load deadline and popup results, and the actual installed
Chrome PWA refresh, fresh authentication/navigation, logout and uninstall results.
File continuation checks cover first-seen order and latest metadata; Prompt
History checks cover delayed success and failure after selection or refresh.

REQ-HOST-007/009/010, REQ-NET-001..004 and REQ-UI-007 are promoted only within
these approved isolated host and delivery scopes. No live provider, launchd
loading, login/upgrade/sleep/wake drill, live Tailscale exposure or supported-device
qualification is inferred. Existing ADRs remain applicable without a new decision.
Canonical E8 feedback retains explicit future re-entry conditions; no current
correctness or acceptance work is deferred.

### E8 Epic closeout (2026-10-01)

The committed E8-T1/T2/T3 outcomes pass the whole-Epic requirement and seam
audit and the six-role completion source assessment. Accepted scope covers
REQ-AUTH-001..004, REQ-HOST-007/009/010, REQ-NET-001..004 and REQ-UI-007:
one protected account, authenticated sessions and feature routes, the shared
loopback HTTPS core, verified native attachment, isolated deployment packaging
and checked browser/PWA delivery.

Serial `make test` passes on the committed candidate. Separate native and
browser evidence covers Wails/WebKit pinning, process cancellation, trust and
load failures, popup denial, installed standalone PWA refresh with fresh
authentication/navigation, logout and uninstall. File continuation and Prompt
History race evidence covers the fired E6/E7 integration follow-ups.

All current acceptance criteria are met and every admitted Low disposition is
complete. Canonical feedback retains future re-entry conditions without
deferring current correctness. No release, installation, launchd loading, live
Tailscale exposure, real-provider or supported-device qualification is implied.
Full fake application acceptance remains E14-owned; released-provider and live
deployment qualification remain E2/E9-owned.

E8 is Completed, no Task is active, and E14-T1 is next. The shared consumer
dossier remains for E14, E2 and E9; only E8's roadmap link is replaced by its
canonical outcomes.

### E14-T1 assembled acceptance (2026-10-01)

The checked adapters now run together through the real shared host, authenticated
Connect routes and SQLite. The native shell and Chrome use the same checked
bundle. The requirement mapping below records assembled fake-provider
acceptance for the four E14-T1-owned requirements.

| Requirement | Implemented E14-T1 scope and evidence |
| --- | --- |
| REQ-HOST-001 | The real shared TLS listener, account/session boundary and SQLite operate without Wails or a real provider; `internal/host` tests also cover the unavailable-runtime production configuration. |
| REQ-HOST-002 | Chrome and isolated Wails/WebKit use the same checked bundle, authenticated Connect routes and host persistence. The native assembled check covers login, session selection, exact original and local file reading. |
| REQ-API-001 | Generated Auth, Runtime, Workspace, Direct Session, Artifact, Interaction, Writer, File, ClientEvent and Diagnostics services share the protection boundary. Direct Session adds bounded text Submit; Diagnostics returns only provider/persistence availability. |
| REQ-CONSUMER-004 | Real assembled HTTP and Chrome checks cover sequential read/write admission, distinct duplicate prompts, busy draft/rejection, approval during a waiting Turn, verified originals/results, restart, disconnect/reconnect, root-only aggregate close and safe local files. Unknown Submit survives restart without replay. |

The fake uses a real-time clock for fresh projections while retaining the
existing deterministic constructor for contract scenarios. It preserves one
Thread across sequential Turns, exact original bytes and distinct accepted
identities. First dedicated write publishes typed writer authority and verified
write policy. Waiting Interaction projections carry current revision/stamp and
terminal Turn events include their identity.

The assembled host creates checked adapters before core startup. Reconnect
reads raw provider snapshots to avoid a circular admission dependency; it checks
workspace/carrier authority, refreshes aggregate floors and joins observation
before store shutdown. Repeated provider loss publishes one invalidation until
recovery; failed persistence invalidation remains retryable. Sync reads have a
20-second cycle deadline. Pending-close UI reads refresh the projection without
sending another mutation.

Focused Go race checks and real Chrome/native checks pass. Serial `make test` also passes, including API/frontend/contract drift, foundation isolation and SOT checks. Gaori reports command exit 0; its degraded extractor does not change the passing command status. The first six-role static assessment found missing direct assertions for offline
mutation refusal, the WRITE effect and actual no-replay calls, plus a duplicated
Submit bound and unqualified phase/API wording. Corrections add provider-call
counters, typed writer assertions, cancellable subject-scoped binding lookup,
generated bound use and explicit E2-T3 extension ownership. The updated browser
campaign checks unknown draft retention, size feedback and automatic event
reconnection, and holds close pending through a poll interval. The corrected
focused race suites, complete Chrome campaign and actual native race suite pass.
The serial `make test` facade also passes, with degraded Gaori extraction recorded
separately. The second six-role assessment confirmed those corrections and identified a snapshot-required content catch-up gap, pre-attempt persistence misclassification and missing direct fake-authority regression assertions. The candidate now refreshes all content panels on rejoin, distinguishes typed persistence failure from unknown effects and tests startup policy, stable thread identity, WRITE retention, lane refusal and cross-run writer conflict. Focused checks also cover production Submit refusal, ambiguous binding lookup and online Diagnostics. The Chrome campaign holds rejoin across an actual result publication and verifies all three content reads resume. Revised contract comments and host timing descriptions match the assembly. Unchanged historical handler comments remain explicitly owned by E8-FB-012. The corrected focused Go race checks and full real Chrome campaign pass. Chrome verifies neutral pending-eligibility status, all three content reads after rejoin, missed-publication visibility and full result integrity. The actual native race suite also passes. Latest serial `make test` passes with command exit 0 and degraded Gaori extraction recorded separately. The third assessment confirmed the earlier corrections and found a gate closure between durable admission and provider dispatch could retain a never-dispatched Submit as unknown. The correction resolves fresh pending attempts as rejected, preserves prior unknown attempts during blocked recovery, propagates rejection-persistence failure and prevents durable rejection retries from appearing accepted. Focused race checks prove restart releases the rejected attempt's mutation slot, exact retries never dispatch, unknown recovery retains its possible prior effect and storage failure remains fail closed. Assembled checks also cover offline-start cleanup and actual JSON transport limits. The fourth six-role assessment confirmed the runtime corrections. Its documentary command attribution was corrected and checked locally; remaining independent Low observations retain their canonical future-work owners. A lifecycle negative fixture depended on the repository task remaining incomplete. The fixture now explicitly sets E14-T1 to Planned within its isolated copy, preserving the same premature-promotion rejection assertion across actual lifecycle transitions.
E14-T1 is Completed for the assembled fake-provider scope. Its completion
projection passed the required SOT and serial gates and the bounded correction
confirmation. E14 is Completed after whole-Epic validation and closeout;
real-provider and live deployment qualification remain E2/E9-owned.

| Remaining qualification | Canonical owner |
| --- | --- |
| Exact released executable and runtime capabilities, install identity and immutable version pin | E2-T0 |
| Public local gRPC/UDS permissions, dial/reconnect, no Machine CLI fallback | E2-T1 |
| Real Controller/carrier issuance and fixed-home credential validation | E2-T2 |
| Actual session creation, live sequential read/write, guarded image handoff, provider/Worker/Gul/browser restart and large artifacts | E2-T3 |
| Supported browser/device layouts and touch, live diagnostics/security, deployed authentication and stream revocation | E9-T1/T2 |
| Installed launchd login/upgrade/sleep/wake and actual Tailscale Serve exposure/reachability | E2/E9 deployment qualification |

The real provider and live runtime are absent from E14. Fake approval, mutation
and result evidence does not qualify those remaining boundaries. The shared
consumer dossier remains for E2 and E9 until their respective closeouts. Existing ADRs apply; this task introduces no installation or release
operation.

### E14 Epic closeout (2026-10-01)

E14 is Completed for the assembled authenticated application against explicit
stateful fakes. REQ-HOST-001/002, REQ-API-001 and REQ-CONSUMER-004 cover the shared
core, protected generated APIs, one browser/native bundle and integrated
history, approval, results, close, files and recovery behavior. The committed
implementation and its actual scoped checks support the whole-Epic acceptance;
released-provider qualification remains E2/E9-owned. Independent regression
coverage and diagnostic wording follow-ups are owned by E14-FB-001 through
E14-FB-015 in `deferred-feedback.md`, with explicit re-entry conditions.

No Task is active. E2-T0 is next after the Provider released gate. The shared
consumer dossier remains for E2/E9; E14's roadmap link now points to its
canonical outcomes. Live transport, credentials, session creation, image/effort
handoff, supported devices and deployment remain unqualified by this fake
acceptance. No installation or upstream publication is implied.

### E2 integration plan amendment (2026-10-02)

The approved E2 plan retains E2-T0 through E2-T3, their predecessors and
Post-release phase. E2-T0 covers published-artifact qualification, release
contract drift and an operation-specific capability admission matrix. E2-T1
implements that matrix in the real generated gRPC adapter and ordinary host
startup. E2-T2 supplies actual protected credential creation and verification;
E2-T3 proves authenticated session creation, sequential read/write, approval,
history, artifacts and whole-session close through Gul.

Acceptance now explicitly compares the credential schema ID with the pinned
schema's `$id` and distinguishes `reader_writer_access` output from lane and
write-intent inputs. These checks must resolve provider support without changing
advertised values. Source-tag diagnostics cannot satisfy the published-artifact
gate, and a successful handshake/read cannot establish write acceptance.
E9 retains fault, security, supported-device and deployment qualification.
Existing architecture decisions apply. All E2 Tasks remain Planned, no Task is
active, and implemented Current State is unchanged. This amendment authorizes
planning only; it does not qualify a provider, install a binary or implement a
Task. The shared consumer dossier continues to link to the roadmap's acceptance
details.

### E2-T0 release qualification (2026-10-02)

E2-T0 release qualification (2026-10-02): the published v0.1.3 source is
`07dc31331d03aae9ed7c0c862a0cbe8a5184024e`. The current dependency-lock SHA-256
is `223d2a72d7bd281dba0abdc8b1e966d792492546c3be63a67e593c94eed0c05a` and generated-lock SHA-256 is
`8d02e824c36795221ee6d1b3fc7e2b43943d13701aa76d019c3b6aea30b26a6d`. The earlier E12 pin remains historical evidence.
Actual production assembly, credentials and session actions remain E2-T1/T2/T3.

The published archive `dolgorae-v0.1.3-aarch64-apple-darwin.tar.gz` has SHA-256
`91fff11625ec546730d7bc18c68c51f5b367a6623700b09661a2b9589ce35bd2`; its executable
has SHA-256 `8154564ffaa3014bef235aed6a8cc0da017cdc1501267fa9c85aa927227bc824`.
The published release is stable, identifies the exact source above, and records
TASK-026 acceptance plus separate release-candidate QA. The installed binary
and neighboring dirty producer tree were not used for qualification.

The exact tag was exported into a separate temporary tree. With isolated HOME,
Cargo home and build target, `rustup run 1.97.1 cargo build --locked --bin dolgorae`
passed. Its binary SHA-256 is
`0e7ae137ba699eb63d67c4319839939513ca9b7a7476e6714ddeeabe26431d03` and its Machine
capabilities equal the published binary's snapshot. This is supplemental source
provenance; the source-built executable does not satisfy the release gate.

The opt-in published-artifact test checks the archive digest, copies the binary
into its private directory and verifies that copy before execution. It uses a
minimal child environment and generated public gRPC clients over a private Unix
socket in an isolated HOME/workspace. It passes version negotiation
with a protocol-zero handshake and fresh request UUIDs, negotiated ListProfiles,
27 required methods, feature parity, exact credential schema ID/version/digest,
dedicated/shared lane writer support, and typed unsupported-protocol refusal.
No Profile, carrier, account or live Codex runtime is configured by this test.
The fixture initializes its own home/workspace using the public CLI; production
operations retain the public-gRPC-only boundary.

Release inputs change descriptor metadata, error/action mapping, producer lock
and verification index. The additional frozen TASK-053 descriptor baseline is
imported and locked. Wire proto/descriptor, credential schema, consumer profile,
client and mutation policy bytes are unchanged. Regeneration adds SubmitTurn's
`SESSION_CLOSE_IN_PROGRESS` action mapping and the release-admission map.
`admission-policy.json` owns the operation conditions. Transport admission keeps
all 27 methods separate from conditional Writer actions, preserves false broad
flags, admits dedicated first WRITE with fresh typed guards, and refuses
threadless Acquire, shared-readonly WRITE and continuation. Existing-reader
transition remains blocked until the selected Profile proves supported.

E2-T1 must implement the admission policy in the production probe and transport;
E2-T2 owns carrier creation and verification. E2-T3 supplies action and assembled
browser acceptance against this published executable. Handshake/read evidence
alone supplies no live write acceptance. E9 keeps fault/security/device and
deployment qualification. Release notes are not enrolled. The shared dossier
remains for E2/E9. `make contract-check`, admission-policy negatives and the
serial `make test` facade passed. Corrected-target completion source review
confirms the release-qualification scope. E2-T0 is Completed; E2-T1 is next.

### E2-T1 production gateway and host integration (2026-10-02)

`internal/gateway` implements the frozen 27-method port with the generated
public local-gRPC clients. All clients share one Unix HTTP/2 transport. The
handshake uses protocol zero and the supported range; subsequent requests use
the negotiated protocol and a fresh UUID without modifying caller messages.
The probe compares the exact schema `$id` and digests and consumes the generated
operation admission matrix. The accepted event projection inventory must include
`MINIMAL`, which the observation adapter requests. Dedicated writer support stays
independent from
`reader_writer_access=false`; the selected Profile supplies transition support.
Threadless Acquire remains unavailable. Existing-reader Acquire requires
supported Profile transition, while a verified held writer can submit READ and
Release without it. Typed Submit close errors pass through
the wire adapter to the existing safe domain mapper. Unknown optional bytes are
discarded before projection use, while unknown typed enums and stale response
contexts block the call.

The host singleton precedes provider work. Gul verifies a private executable
copy against the embedded published digest, creates protected runtime parents
below the selected user's cache and chooses an unused socket outside configured
and saved Workspace roots. It launches only `serve`, without a shell. The
provider owns socket bind, chmod, stale proof and unlink. A bounded serve
readiness envelope supplies safe failure codes; socket ownership and the gRPC
handshake decide admission. Source selection is rechecked at every later start.
Startup, shutdown and restart limits follow §6.1. Transient restart readiness or
transport failure retries within the same backoff and rolling budget. Semantic,
identity, socket and collision rejection remains terminal. An earlier failed
startup must settle its owned child before the next attempt.
Coalesced reads have four
workers, 16 starts per second and a 250ms per-key floor; mutations and streams
have independent eight-slot bounds. The transport retains no read-result cache.

Ordinary headless/native host startup assembles the completed services without
`Config.Assemble`. Missing or incompatible providers keep local authentication,
Workspace presentation and FileService available. A provider loss disables
runtime actions and does not mark durable Runs failed. Construction uses the
accepted handshake contract without another network probe; provider loss at
that boundary preserves local host services while live runtime gates stay closed.
The diagnostic API and
operator view expose health, release identity, unsafe socket cleanup and
independent capability flags.
Trusted executable, root and policy flags also survive launchd rendering.
Production never invokes a Machine CLI operation to bootstrap or replace RPCs.
The opt-in tests initialize only their private provider HOME/workspace.

Published-artifact qualification checks live startup, protocol/schema/feature
refusal, active-gateway collision, owned-child shutdown, crash re-handshake and
replacement rejection. Ordinary authenticated host tests cover actual profile
and diagnostic reads followed by transport loss, preserved local navigation/
files and rejected Submit. Offline transport fixtures cover request context
isolation, single-flight, cancellation, typed errors and optional-data handling.
Regression checks cover held-writer READ/Release without transition support,
transient restart recovery and budget/cancellation boundaries, transport versus
semantic handshake rejection, and authenticated local routes after actual
published-provider loss between handshake and service construction. Missing or
OPERATIONAL-only projection inventories are rejected in both isolated and actual
released-capability tests.
Serial `make test` and the published provider/gateway/host race checks passed.
The exact Rust-tag build remains supplemental T0 evidence. Real carrier creation
and session actions remain E2-T2/T3-owned; E9 keeps device/fault/security and
deployment acceptance. No installation or upstream publication is implied.

E2-T1 is Completed for the production gateway and host integration scope.
Real protected carriers and actual session-action qualification remain
E2-T2/T3-owned. E2 remains In Progress, with E2-T2 next.
