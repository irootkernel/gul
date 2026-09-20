# Gul consumer rebaseline handoff

| Field | Value |
| --- | --- |
| Role | Temporary implementation dossier and cross-team handoff |
| Status authority | None. `docs/roadmap.md` owns Task state and order. |
| Consumer contract | `dolgorae.gul-consumer/v1` |
| Producer source | Dolgorae `docs/specs/gul-consumer-v1.md` |
| Target provider | Exact released Dolgorae v0.1.3 artifact |
| Updated | 2026-09-20 |

## 1. Scope

This dossier records the approved work envelope for the Gul planning rebaseline.
It does not claim generated-contract drift is resolved, start a Task, or provide
runtime evidence. The five canonical Gul documents remain authoritative in their
declared precedence order.

Allowed implementation sequence:

1. TASK-053 freezes the immutable producer wire, clients, fixtures, schema and
   policy hashes, and exact revision.
2. E12-T1 repins Gul to that immutable contract and regenerates checked consumer
   artifacts through the repository generator.
3. E12-T2 builds explicit stateful consumer fakes.
4. E12-T3 proves the assembled pre-release core and browser against those fakes.
5. E2 and E9 integrate and qualify only against the exact released v0.1.3
   artifact after producer RC QA.

No pre-release Task may depend directly or transitively on release-gated E2 or
E9. A fake is an explicit test dependency and never a production fallback.

## 2. Producer order and evidence boundary

TASK-025 is Completed at `57e6be8`. The provider order is fixed:

`TASK-053 -> TASK-047 -> TASK-048 -> TASK-049 -> TASK-050 -> TASK-051 -> TASK-054 -> TASK-055 -> TASK-052 -> TASK-056 -> TASK-026`

- TASK-053 freezes checked wire, clients, fixtures, and the immutable revision.
- TASK-054 completes `ListRunTimelineItems` semantics.
- TASK-055 completes `GetOrchestratedSession` and
  `ListOrchestratedSessionResults`.
- TASK-052 completes whole-session closure.
- TASK-056 proves old-client compatibility.
- TASK-026 performs final provider acceptance.

Gul does not reorder or complete those provider Tasks. Historical E0 evidence
continues to describe the old checked pin only. It does not establish the new
27-method profile or runtime implementation.

## 3. Required contract profile

The extended descriptor contains 36 methods. The first-release Gul profile
requires 27: the existing 24, complete `ListRunTimelineItems`, and two
Controller-authorized read-only OrchestrationService queries:

- `GetOrchestratedSession`
- `ListOrchestratedSessionResults`

Optional descriptor methods are not automatically supported actions.
`DeleteRun`, `CreateWriteContinuation`, and the other optional old methods do
not block the first release. E4-T4 remains Deferred and owns any future
continuation implementation.

## 3.1 Review closure checklist

- [ ] E12-T1 pins TASK-053's exact CloseRun progress/error/operation correlation and field-sourceability fixtures. Historical REQ-RUNTIME-022/E0-T7 evidence is not the new pin.
- [ ] E1-T3 declares ListPromptHistory, GetPromptHistoryItem, GetExecutionState, ListSpecialistResults and CloseOutcome with Gul-owned DTOs, IDs, tokens and bounds.
- [ ] E3-T3 completes passive aggregate reads and member status only. No temporary executable close route is installed.
- [ ] E4-T3 completes the shared close-eligibility and interrupt-confirmation evaluator.
- [ ] E4-T5 implements browser history/item/result reads, empty-but-continuable pages, complete original text, fixed traversal scope, token expiry and authorization.
- [ ] E5-T1 uses those completed predecessors for whole-session close, stable operation correlation and pending/confirmed/unknown/recovery handling. E7-T2 integrates UI; E2/E9 prove the real provider.
- [ ] Documentation checks reject mismatched Active Task pointers, removed retired/reserved permanent IDs, invalid dependencies/owners and cross-ledger duplicates. Negative fixtures never alter the live worktree.
- [ ] Check shell syntax and staged/unstaged whitespace. Run contract-check only with the pinned toolchain, or report the exact mismatch without rewriting generated output.

## 4. Product session contract

A Gul session is a Dolgorae Orchestrated Session. Its protected launch consists
of a parentless `direct_interactive` Primary root and an `interactive_client`
carrier with:

`orchestration_launch(use_case=dolgorae_orchestrated_session,specialist_policy_name)`

A low-level `direct_interactive` Run by itself is not a session. A fresh session
is a new launch and is never presented as lineage continuation.

The carrier root is
`~/.dolgorae/controller-carriers/gul/<installation-id>`. Workspace, global
Profile, and named Specialist Policy are host-preprovisioned. Gul has no
production CLI fallback, no Operator capability, and no policy editor. Dolgorae
decides whether a configured Policy name is valid.

Gul may display provider-observed Specialists and published results. It owns no
child credential, control surface, or membership authority. It does not infer
membership from links or parse model text as a result.

## 5. Prompt History acceptance

Prompt History is required for the first release and is a separate user-only
projection of accepted Primary input. Verification covers:

- exact stable provider order, accepted identity, and timestamp;
- original Unicode text, including Korean, multiline input, and CRLF;
- preview, full expansion, and navigation to the matching conversation;
- complete traversal across pages and concurrent append boundaries;
- browser, Gul, gateway, provider, and session-close recovery;
- one identity for a retried request, with no duplicate history item;
- distinct identities for separate new requests with the same text;
- retention when the associated task is interrupted or fails;
- Controller-authorized Artifact reads for long accepted input.

The Dolgorae timeline is authority. Gul's cache is non-authoritative. History
storage does not authorize automatic mutation replay and excludes secret
answers, reasoning, and raw tool payloads. Unknown acceptance is never resent
automatically.

During an active Primary Turn, a fresh general prompt remains a draft. Sending
requires an explicit user action after actual terminal evidence and a fresh
eligibility check. There is no prompt queue, steering, auto-send, or
auto-interrupt. Existing Interaction replies remain available.

## 6. Whole-session close acceptance

Close uses root `CloseRun`; the Broker settles owned Specialists. Gul never
loops over child mutations. If owned work is active, the user must provide
explicit interrupt intent and `interrupt=false` is rejected.

The UI distinguishes closing, unknown outcome, and confirmed closed. A mutation
receipt alone is not confirmed closure. `GetOrchestratedSession` must account
for all owned work before success appears. Close retains Prompt History, results,
and workspace changes. It does not close unrelated sessions or stop a healthy
shared Profile. Browser close and hide are local presentation actions.

Pause and Interrupt remain Primary-scoped. Neither is an aggregate pause.

## 7. Stateful fake acceptance

E12-T2 fakes model behavior rather than universal success. Required scenarios
include:

- stable prompt identities, same-text new requests, and request retry;
- timeline pagination, captured heads, CRLF, Korean, and long-input artifacts;
- active-Turn draft rejection and fresh post-terminal admission;
- Interaction replies while the Primary waits;
- aggregate status, Specialist observations, and published result pages;
- closing, confirmed closed, and unresolved close outcomes;
- wrong Controller, stale revision, foreign cursor, and unknown required enum;
- reconnect, duplicate events, slow consumers, and projection convergence;
- optional method presence without a corresponding Gul action.

E12-T3 uses actual Gul core and browser code with these fakes. It is not a
screenshot-only proof and does not claim real credentials, UDS lifecycle,
supervision, or native binding behavior.

## 8. Released-provider acceptance

E2 repins the exact released artifact, then implements and verifies the real
adapter, credentials, process supervision, UDS ownership, compatibility
handshake, and native bindings. E9 runs actual-provider fault, security, browser,
restart, artifact, history, result, and close scenarios. A floating worktree or
provider-side test campaign cannot substitute for this evidence.

## 9. Deferred Podway observation

Podway observation is post-v0.1.3 and read-only end to end. A future accepted
contract may expose the full pinned graph, active node set, per-loop iteration,
stable node-execution counts, execution identity, source revision, and freshness.
Execution identity remains distinct from session identity.

Gul will not connect directly to Podway or read its files. It will expose no FSM
edit, jump, skip, force-complete, retry-node, or reset-count endpoint. A user may
request change only through an ordinary general prompt. The executing LLM
decides under Podway rules, and the diagram changes only after actual Podway
state changes. Missing Podway observation cannot block existing functionality.

## 10. Completion checklist

- [ ] E12-T1 pins the immutable TASK-053 revision and regenerates, without
      hand-editing generated outputs.
- [ ] The consumer matrix distinguishes 27 required methods from all 36 methods.
- [ ] Global Profile reads contain no Workspace reference.
- [ ] The fixed carrier-root contract and no-Operator boundary pass fixtures.
- [ ] E12-T2 stateful fake scenarios pass.
- [ ] E12-T3 assembled pre-release browser proof passes.
- [ ] No pre-release Task depends on E2 or E9.
- [ ] E4-T4 remains Deferred and no first-release action offers continuation.
- [ ] E2 pins the exact released v0.1.3 artifact after RC QA.
- [ ] E9 records actual-provider acceptance separately from fake evidence.
- [ ] All canonical documents, requirement owners, DAG phases, and local links
      pass `scripts/check-sot.sh`.
