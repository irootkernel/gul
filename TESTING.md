# Testing and developer commands

E0-T8 defines a serial, non-rewriting command facade. It does not install tools,
download dependencies, generate application code, or claim that the Gul runtime
exists.

| Command | Contract |
|---|---|
| `make toolchain-check` | Compare the host with `toolchain/versions.env`; report every missing or mismatched executable and exit nonzero. |
| `make generate-contract` | Delegate to E0-T7's checked generator; fail closed while that executable is absent. |
| `make contract-check` | Delegate to E0-T7's drift check; fail closed while that executable is absent. |
| `make test-prepare` | Validate the pin manifest and shell syntax without changing the tree. |
| `make test-unit` | Prove the pre-E0-T7 contract commands fail closed with actionable prerequisites. |
| `make test-int` | Validate SOT Task/requirement/ADR integrity and reject whitespace errors in the task diff. |
| `make test-e2e` | Run the complete checker fixture through its public command boundary. |
| `make test` | Run `test-prepare`, `test-unit`, `test-int`, and `test-e2e` serially. |

The pinned Wails executable is `wails3` at exactly `v3.0.0-beta.8`. A `wails`
v2 installation is not a substitute. The host check also requires macOS 14 or
newer on arm64 and Git `>=2.39.0,<3.0.0`.

The version manifest also pins application dependencies that do not exist in the
repository yet. Their future `go.mod` and `package.json` declarations must match
the manifest. E0-T7 must install the two contract delegates named by the command
facade alongside its task-owned generated artifacts and fixtures. Later Tasks may
extend the tests but must keep the serial facade and fail-closed prerequisites.
