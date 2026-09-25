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
| `make contract-check` | Regenerate into a temporary directory; reject source/generated drift; compile and test Go/TypeScript clients, fake gRPC server, maps, fixtures, and the stateful `contract/scenario` provider. |
| `make generate-frontend` | Install the frozen root Bun graph without lifecycle scripts, then build the exact-pinned React source into the checked shared browser/shell bundle. |
| `make frontend-check` | Install the frozen root Bun graph without lifecycle scripts, rebuild the frontend in a temporary directory, and reject any checked bundle drift. |
| `make generate-api` | Check the exact Go toolchain and available `gofmt`, then generate checked Gul Protobuf/ConnectRPC Go and TypeScript clients and the accepted provider-error catalog from pinned authorities. |
| `make api-check` | Regenerate Gul clients temporarily and reject schema, byte, inventory, or provider-error catalog drift. |
| `make test-prepare` | Validate both Go manifests, the root Bun manifest, exact Go, Bun, and shell syntax, then materialize the root and locked contract dependencies with Bun lifecycle scripts disabled. |
| `make test-unit` | Validate product manifests, typecheck and test the React foundation and generated Gul API clients, run browser-contract fixtures and race-enabled shared bundle/core/storage tests, compile generated Go clients, and run contract/facade fixtures including `contract/scenario`. |
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
