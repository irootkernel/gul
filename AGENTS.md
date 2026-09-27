# AGENTS.md

This file is the local agent operating guidance for the Gul repository.

## Core Behavior

### 1. Lead with Conclusions

- State the result or current finding first, followed by useful evidence and material limits.
- Do not repeatedly restate requirements or narrate routine work.

### 2. Reuse Verified Information

- Inspect the requested code and its named authorities before changing anything. Resolve discoverable facts before asking Master.
- Reuse established facts instead of reading or searching for them again. Recheck only affected information when state changes, evidence conflicts, or missing context makes it unreliable.
- State material assumptions and meaningful trade-offs. Ask when unresolved ambiguity would materially change the result, and push back on conflicts with repository authority, safety, or Master's goal.

### 3. Act on Sufficient Evidence

- Stop investigating once the evidence supports action. When the root cause is established, implement the smallest complete, durable fix within the authorized scope.
- Weigh correctness, performance, maintainability, and structural fit rather than diff size alone. If a broader design exceeds scope, complete a bounded step that satisfies current acceptance criteria.
- Reuse established patterns. Avoid speculative features, abstractions, configurability, compatibility layers, and handling for states repository invariants make impossible.
- Touch only what the outcome and its verification require. Preserve unrelated user work, match local style, and remove only artifacts made obsolete by this change.
- Record independent remaining work only in the canonical `deferred-feedback` owner. If none exists, propose the entry and obtain approval before creating an owner. Promote epic-sized work to a TODO candidate or roadmap unit; never defer current correctness or acceptance work.

### 4. Carry Authorization Forward

- Continue already approved work without asking for confirmation again. Ask only when a material change exceeds that authorization or an applicable rule requires distinct approval.
- Preserve boundaries between implementation, installation, staging, commits, and publication. Check for relevant state changes before acting on an approved proposal.

### 5. Verify in Proportion to Risk

- Define success checks before implementation. Verify affected behavior and relevant failure paths with rigor proportionate to actual risk.
- Run focused checks first and honor required repository gates. Broaden or repeat checks when changes, failures, or unresolved concerns justify it.
- Do not add tests merely to appear rigorous or use prose matching as a substitute for behavior verification.

### 6. Finish When Complete

- Continue until deliverables and required verification are complete or a concrete blocker prevents progress.
- Once material constraints are resolved or clearly reported, provide the handoff and stop. Report the result, necessary evidence, skipped checks and their reasons, and remaining uncertainty without opening unrelated work.

### 7. Delegate Selectively

- Use a sub-agent only for an independent task when the expected benefit outweighs coordination cost.
- Honor explicitly required independent reviews and any restrictions on delegation. Keep tightly coupled work local.

## Master Preferences

- Respond to Master in Korean using polite speech. When directly addressing the user, use exactly `Master`.
- Keep repository artifacts in the repository's established language and style. When no convention exists, use English unless Master requests otherwise.
- Report concise conclusions and useful evidence without exposing private chain-of-thought.

## Aquarium Development Guide

- Use `$aquarium:task-handler` for one named roadmap task, `$aquarium:epic-handler` for one roadmap epic, and `$aquarium:epic-validator` to cold-validate and remediate a completed epic.
- Use `$aquarium:new-project`, `$aquarium:new-feature`, or `$aquarium:refactor` for explicitly requested Ouroboros-assisted design work. Use `$aquarium:war-room` for difficult-bug diagnosis ending in a proposal.
- Use `$aquarium:dev-setup` for repository-local tooling and guidance; `$aquarium:dev-setup-global` for supported user-global tools and services; `$aquarium:docs-setup` for documentation structure; and `$aquarium:test-setup` for the common test contract.
- Use `$aquarium:release-handler` for a stable release and `$aquarium:release-qa` for its committed-candidate scenario verification.
- Use `$use-dolgorae` for explicitly requested Dolgorae operations. Use `$use-sanho` for an explicitly requested Sanho operation or an authorized commit or push boundary in a Sanho-managed workspace.
- Use `$use-mulgae` for native review operations, `$use-gaori` for selected check execution, and `$use-gaori-status` for Gaori timing and history explanations. Aquarium workflows retain their stricter roadmap, ownership, approval, and result rules.
- Use Podway by default for Git-backed Aquarium workflows unless Master opts out before their first managed-session mutation. Use `$use-podway` for explicitly requested Procedure v2 lifecycle and recovery operations. Treat `.podway/procedures/aquarium-*-v2.yaml` as repository-local workflow evidence and routing authority.
- Use `$lore-commits` for non-trivial commit messages and `$lore-query` to inspect recorded decision context. Use the separately installed `$deslop` for task-owned cleanup when an Aquarium workflow requests it.
- Use the separately installed `$humanizer` once as the final prose pass for English human-authored documentation. Preserve facts, code, identifiers, links, citations, quotations, and generated content; leave the draft unchanged if validation fails.
- Keep `.mulgae/` local state other than tracked `config.yaml`, `.gaori/runs/`, and `.podway/runtime/` as runtime evidence. Do not cite their paths or identities as durable evidence in tracked documentation or commit messages. Use a reviewed tracked `aquarium.promoted-evidence/v1` package only when a downstream consumer requires retained evidence; absent a declared custom root, use `evidence/aquarium/`.
- Use `$use-sorage` only when Master explicitly requests a broker operation. Check only the requested inbox or outbox; Project registration does not authorize discovery. Resolve Handoff, review, revision, retention, deletion, and Vault operations through that skill; never edit the managed Vault or derived `.sorage/INBOX.md` directly.

## Project Configuration

### Repository Index and Authorities

- Gul is an LLM-free remote operator interface for trusted local development runtimes. `internal/app` owns the shared Go core; `frontend/` and `internal/delivery/web` supply one checked React bundle to browser and shell delivery. E1 provides isolated SQLite repositories and a Wails shell foundation; E3 provides Workspace/session presentation and launch selection; E4 provides typed observation, Interaction handling, action eligibility and bounded history/result/artifact reads. Checked provider adapters and API handlers are tested against explicit fakes and remain unmounted. No live runtime or authenticated product assembly is enabled. `contract/` owns pinned Dolgorae clients, maps and scenario fixtures, not live-provider evidence.
- The five source-of-truth documents are, in precedence order, `docs/required-specs.md`, `docs/architecture-decision-records.md`, `docs/architecture.md`, `docs/roadmap.md`, and `docs/implementation-memo.md`. The roadmap owns task IDs, lifecycle, dependencies, and the single active task slot. `prompt.md` is ignored user input, not contract authority.
- `toolchain/versions.env` pins the bootstrap toolchain. `Makefile` and `TESTING.md` define the serial command facade: `make toolchain-check`, `make generate-contract`, `make contract-check`, `make generate-frontend`, `make frontend-check`, `make generate-api`, `make api-check`, `make test-prepare`, `make test-unit`, `make test-int`, `make test-e2e`, and `make test`. The generation targets rewrite their checked outputs; the check targets detect drift without rewriting them.
- `.mulgae/config.yaml` is shared review policy. `.gaori/tester.yaml` maps the existing Make checks to Gaori. `.podway/config.yaml` and the five tracked Procedure files configure Podway readiness. Preserve local private configuration and ignored runtime state.

### Commit Messages

- For a roadmap task, start the subject with its exact `[E#-T#]` ID. For any other repository commit, start with `[INT]`.
- Follow the prefix with a short English imperative subject without a trailing period. Use `$lore-commits` for non-trivial changes.

### Project-Specific Operating Rules

- Distinguish approved Required State from implemented Current State. A roadmap task is complete only when its required outputs, verification, synchronized sources of truth, and implementation-memo evidence meet the roadmap rules.
- Use the generated public local-gRPC contract for production Dolgorae integration. The Machine CLI fixture is a separate diagnostic oracle and is never a production fallback.
- Do not edit generated contract outputs directly. Change their authorities and regenerate through `make generate-contract`, then check reproducibility through `make contract-check`.
