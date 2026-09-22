# Testing and developer commands

E0-T8 defines a serial command facade, and E1-T1 extends it with the root Go
product module. It does not install host tools or claim assembled runtime
readiness. `make test-prepare` downloads only declared Go and locked contract
dependencies, and `make generate-contract` rewrites only checked contract output.

| Command | Contract |
|---|---|
| `make toolchain-check` | Compare the host with `toolchain/versions.env`; report an exact Go mismatch and every missing or below-minimum non-Go executable, then exit nonzero. |
| `make generate-contract` | Reproduce the pinned descriptor and deterministically rewrite only checked contract outputs. |
| `make contract-check` | Regenerate into a temporary directory; reject source/generated drift; compile and test Go/TypeScript clients, fake gRPC server, maps, and fixtures. |
| `make test-prepare` | Validate both Go manifests, the exact Go toolchain, Bun, and shell syntax, then materialize the root and locked contract dependencies with Bun lifecycle scripts disabled. |
| `make test-unit` | Validate both Go manifests and the exact Go toolchain, run the headless core lifecycle/fail-closed tests, compile generated Go clients, run descriptor-derived fake gRPC success/error tests, exercise the additive-breaking positive/negative regression, and exercise facade usage/fail-closed branches. |
| `make test-int` | Recheck exact Go 1.26.6, product-module tidiness, and SOT integrity, then run contract drift/schema/policy fixtures and TypeScript clients through the public command facade before whitespace checks. |
| `make test-e2e` | Run the complete checker fixture through its public command boundary. |
| `make test` | Run `test-prepare`, `test-unit`, `test-int`, and `test-e2e` serially. |

Go is fixed at exactly `1.26.6` until an explicit toolchain upgrade. Other host
executables use minimum-compatible versions: `wails3` `>=3.0.0-beta.8`, Node
`>=26.7.0`, Bun `>=1.4.2`, Buf `>=1.66.1`, protoc
`>=35.1`, Git `>=2.39.0`, and macOS `>=14.0.0` on arm64. A `wails` v2
installation is not a substitute. Newer non-Go host tools are accepted, but they still
must pass the build, test, and byte-for-byte descriptor checks. Language-client
generators and project dependencies remain exactly pinned in the project Go and
Bun manifests. PATH copies of those generators are not part of the host check.

`make test-prepare` checks the Bun minimum before dependency materialization.
The root Go module hosts the shared core; both Go modules use
`toolchain go1.26.6`, and Go commands run with `GOTOOLCHAIN=local` after the
live manifests and host compiler are checked. The nested contract Go/Bun graphs match the version manifest
and compile contract fixtures. Later Tasks may extend the tests but must keep the
serial facade, deterministic generation, and fail-closed prerequisites.
