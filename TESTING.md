# Testing and developer commands

E0-T8 defines a serial command facade, E1-T1 extends it with the root Go product
module, E1-T2 adds the exact-pinned React bundle, and E1-T3 adds checked Gul
ConnectRPC declarations and the typed consumer-port error catalog. E1-T4 adds
isolated SQLite migration and repository fixtures. E1-T5 adds isolated Wails
shell lifecycle and shared-bundle tests without claiming authenticated attach.
The facade does not install host tools or claim assembled runtime readiness.
`make test-prepare` downloads only
declared Go and locked Bun dependencies. Generation targets rewrite only their
named checked outputs.

| Command | Contract |
|---|---|
| `make toolchain-check` | Compare the host with `toolchain/versions.env`; report an exact Go mismatch and every missing or below-minimum non-Go executable, then exit nonzero. |
| `make generate-contract` | Reproduce the pinned descriptor and deterministically rewrite only checked contract outputs. |
| `make contract-check` | Regenerate into a temporary directory; reject source/generated drift; race-test Go clients, the fake gRPC server, and the stateful `contract/scenario` provider; then check TypeScript clients, maps, and fixtures. |
| `make generate-frontend` | Install the frozen root Bun graph without lifecycle scripts, then build the exact-pinned React source into the checked shared browser/shell bundle. |
| `make frontend-check` | Install the frozen root Bun graph without lifecycle scripts, rebuild the frontend in a temporary directory, and reject any checked bundle drift. |
| `make generate-api` | Check the exact Go toolchain and available `gofmt`, then generate checked Gul Protobuf/ConnectRPC Go and TypeScript clients and the accepted provider-error catalog from pinned authorities. |
| `make api-check` | Regenerate Gul clients temporarily and reject schema, byte, inventory, or provider-error catalog drift. |
| `make test-prepare` | Validate both Go manifests, the root Bun manifest, exact Go, Bun, and shell syntax, then materialize the root and locked contract dependencies with Bun lifecycle scripts disabled. |
| `make test-unit` | Validate product manifests, typecheck and test the React foundation and generated Gul API clients, run browser-contract fixtures and race-enabled root and contract Go tests including `contract/scenario`, and run contract/facade fixtures. |
| `make test-int` | Recheck exact Go 1.27.1, product-module tidiness, frontend/API reproducibility, and SOT integrity, then run contract drift/schema/policy fixtures and whitespace checks. |
| `make test-e2e` | Run the complete checker fixture through its public command boundary. |
| `make test` | Run `test-prepare`, `test-unit`, `test-int`, and `test-e2e` serially. |

Go is fixed at exactly `1.27.1` until an explicit toolchain upgrade. Other host
executables use minimum-compatible versions: `wails3` `>=3.0.0-beta.8`, Node
`>=26.7.0`, Bun `>=1.4.2`, Buf `>=1.66.1`, protoc
`>=35.1`, Git `>=2.39.0`, and macOS `>=14.0.0` on arm64. A `wails` v2
installation is not a substitute. Newer non-Go host tools are accepted, but they still
must pass the build, test, and byte-for-byte descriptor checks. Language-client
generators and project dependencies remain exactly pinned in the project Go and
Bun manifests. PATH copies of those generators are not part of the host check.

`make test-prepare` checks the Bun minimum before dependency materialization.
The root Go module hosts the shared core; both Go modules use
`toolchain go1.27.1`, and Go commands run with `GOTOOLCHAIN=local` after the
live manifests and host compiler are checked. The root Bun manifest owns the
React bundle and uses only exact dependency versions; the nested contract Bun
graph remains separate. Later Tasks may extend the tests but must keep the
serial facade, deterministic generation, one-bundle delivery, and fail-closed
prerequisites.

The selected E4-T3/T5 browser check is `python3 frontend/browser/verify-actions.py`.
It requires existing Bun, `playwright-cli` and Chrome, builds an isolated component
fixture in a temporary directory, serves it on a loopback ephemeral port, and
closes its browser session and server afterward. It covers six Interaction
outcomes, writer changes and prompt/consent behavior against explicit fakes,
including typed WRITE-submit rejection and unknown outcomes with retained drafts.
It also checks inert artifact Markdown, path-shaped reference text, and the absence
of executable elements and external resource loads.
It installs nothing and remains separate from `make test` and live-provider
acceptance. The fixture is typechecked with the frontend test sources.

The E7-T1 browser check is `python3 frontend/browser/verify-files.py`. It
requires existing Bun, `playwright-cli`, and Chrome. The check builds the
responsive operator component against explicit typed clients in a temporary
loopback fixture and exercises desktop panes, iPhone-size
navigation, session selection, file browsing, preview, degraded Git status,
pagination and retry, comparison, workspace return, navigation-write isolation,
explicit refresh, delayed compare and refresh failures after navigation, and
representative client failures. It is separate from
`make test` and does not qualify authenticated delivery or a live provider.

The E7-T2 browser check is `python3 frontend/browser/verify-activity.py`.
It requires existing Bun, `playwright-cli`, and Chrome.
It builds an isolated typed-client fixture and checks mobile navigation status,
pending Interaction priority, chronological conversation across refresh and
pagination, accepted-user Prompt History with inline and artifact originals,
linked Turns outside the loaded page, close receipts and transport failure,
read degradation, bounded pending-card fan-out, same-session status retention,
typed external blockers, per-entry original-response errors, suppression of
superseded original success and failure, and successful retry after a visible
original error in real Chrome.
`make test` typechecks the fixture and runs focused artifact integrity,
provider page limit and typed blocker unit tests. The browser check remains
separate from `make test`
and does not qualify authenticated delivery or a live provider.

The E7-T3 browser check is `python3 frontend/browser/verify-accessibility.py`.
It requires existing Bun, `playwright-cli`, and Chrome. It builds isolated
typed-client fixtures and checks Korean composition in both commit event
orders, explicit pointer and keyboard submission after lost keyup or composition
end, keyboard focus after session, Turn, and file navigation, distinct accessible controls,
and visible focus at desktop,
tablet, and phone sizes. It is separate from
`make test`; actual supported-device and assembled-product acceptance remain
with E9.

The E8-T1 component check is `python3 frontend/browser/verify-setup.py`. It uses
existing Bun, `playwright-cli`, and Chrome in an isolated temporary loopback
fixture. It checks remote setup refusal, exact Unicode input, password bounds,
clearing before a pending request, duplicate suppression, sanitized failures and
keyboard focus recovery,
Korean composition, and absence of browser storage. It uses a fake callback,
creates no real account, and closes its browser and server. Run it separately
from `make test`; the serial facade covers the auth service and SQLite race,
rollback, migration, reopen and backup tests. Authenticated transport and native
bootstrap are E8-T2/T3 checks.

The E8-T2 component check is `python3 frontend/browser/verify-auth.py`. It
checks setup-to-login, sanitized login and rate-limit failures, password
clearing, pending duplicate suppression, focus/IME, removal of product content
on expiry/logout and retry after uncertain logout. It uses injected auth in
Chrome and creates no account or deployment. Bun transport tests separately
exercise typed Protobuf encoding, CSRF headers, setup-response reconciliation
and cancellation. The serial Go checks use real isolated TLS ConnectRPC and
SQLite fixtures for cookies, all declared feature-route guards, durable sessions,
rate bounds and active event/file stream revocation. Host/native/Serve assembly
remains E8-T3-owned and assembled application acceptance remains E14-owned.

The E8-T3 host tests use isolated protected directories, actual loopback TLS,
SQLite and the shared Connect routes. They check contention, verified native
attachment, bootstrap refusal, certificate mismatch, unsafe state, port collision
and graceful restart. They also reject expired or damaged persisted certificates
without rewriting them and require a qualified CLI/daemon version before using
Tailscale snapshot completeness. CLI tests cover dispatch, read-only diagnostics,
headless cancellation and both owned and attached desktop close paths without
opening a native window. Deployment fixtures validate Serve/Funnel configurations
and render a user launchd agent without loading it. The opt-in native check
`GUL_RUN_NATIVE_WAILS_TEST=1 GOTOOLCHAIN=local go test -race ./internal/desktop -count=1`
opens a real Wails/WebKit window against an isolated HTTPS probe to test the
pin and document-start bootstrap across same-origin navigation. It does not
qualify live-provider behavior. The assembled native check also opens the checked operator bundle against the real isolated host with an explicitly injected fake and verifies login, session selection, exact prompt original and safe local files.
`python3 frontend/browser/verify-delivery.py` builds the real isolated host and
opens the checked entry in Chrome, with a temporary profile and a fixture-scoped
certificate pin. The default run checks login, refresh, offline refusal and
reconnect, then closes Chrome and the host. It skips actual PWA installation.
`GUL_RUN_INSTALLED_PWA_TEST=1 python3 frontend/browser/verify-delivery.py`
additionally uses a temporary persistent Chrome profile and privileged CDP pipe
to install, launch and uninstall the PWA. On macOS this can create an app shim
outside the temporary profile and requires explicit approval for those host
effects before execution. That installed-PWA gate must pass before T3 is
accepted; manifest and service-worker readiness alone do not satisfy it.
Both checks are separate from `make test`; live Tailscale, launchd installation,
system trust and actual-provider qualification remain outside these checks.

FileService Connect transport tests inject canceled watcher setup and unavailable
setup/stream failures and check status plus DomainError presence. The real service
also receives an already-canceled context. The Chrome file fixture returns an
overlapping continuation after a retry and verifies unique rows while retaining
stale-page and Workspace-context tests. These isolated checks settle the mounted
FileService re-entry obligations without claiming live provider qualification.

## Assembled application acceptance

`go test -race ./test/acceptance/fixture` runs the real TLS/core/SQLite and
checked-adapter flows without a native window or real provider. It covers
sequential read and first-write admission, busy rejection, long/duplicate prompt
history and originals, waiting approval, Specialist results, root-only close,
restart, all declared feature-service guards, offline local presentation and blocked Submit/
writer/close requests. Provider-call counters prove WRITE intent and no Submit
replay after restart; a blocked offline Submit creates no durable attempt. Direct fake-port checks pin unknown/unverified startup, stable thread identity across WRITE then READ, retained WRITE policy, unsupported-lane refusal and cross-run writer conflict. Other fixture assertions cover ambiguous binding lookup, typed pre-dispatch persistence failure and both Diagnostics availability branches. The production-composition Submit refusal is asserted by `go test -race ./internal/host`. Additional assembled checks verify offline startup releases its listener and singleton lock, the maximum escaped-text JSON request is accepted, and an oversized wire request is refused before provider dispatch. The Submit race suites, `go test -race ./internal/mutation ./internal/mutation/contractprovider`, cover no-effect gate closure, durable rejection across restart, retained unknown replay and typed rejection-persistence failure. Fixture setup uses the protected native permission flow.

`python3 frontend/browser/verify-acceptance.py` builds the explicit acceptance
driver and uses Chrome with a temporary profile and that fixture's certificate
pin. It exercises runtime-profile/model/effort/lane/policy compatibility selection through the checked bundle, busy draft preservation, original/result
integrity, restart, provider/stream loss, offline files and aggregate close. It
also faults one browser event connection, holds rejoin while a real result is published, and asserts automatic reconnection plus content-panel catch-up,
keeps close pending across a polling interval, and checks an isolated unknown-
outcome session for draft retention, size guidance and no replay. The
driver refuses to run without `GUL_RUN_ASSEMBLED_ACCEPTANCE=1`; controls use stdin,
never a product HTTP endpoint. The script sets the opt-in for its own child only.

These checks supplement `make test`. The native opt-in above includes
`TestNativeAssembledCheckedApplication`; it drives the real WebKit DOM through a
test-only native probe and reports over child stdout. It adds no Wails API bridge.
The fixtures close their temporary hosts, windows and browser profiles.
Released executable/UDS integration, real Controller/carrier creation, image
handoff, live runtimes, launchd/Serve deployment and supported devices need E2/E9
qualification.
