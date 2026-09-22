# Testing and developer commands

E0-T8 defines a serial, non-rewriting command facade. It does not install tools,
download dependencies, generate application code, or claim that the Gul runtime
exists.

| Command | Contract |
|---|---|
| `make toolchain-check` | Compare the host with `toolchain/versions.env`; report every missing or mismatched executable and exit nonzero. |
| `make generate-contract` | Reproduce the pinned descriptor and deterministically rewrite only checked contract outputs. |
| `make contract-check` | Regenerate into a temporary directory; reject source/generated drift; compile and test Go/TypeScript clients, fake gRPC server, maps, and fixtures. |
| `make test-prepare` | Validate pins and shell syntax, then materialize only locked Go/Bun contract dependencies with Bun lifecycle scripts disabled. |
| `make test-unit` | Compile generated Go clients, run descriptor-derived fake gRPC success/error tests, exercise the additive-breaking positive/negative regression, and exercise facade usage/fail-closed branches. |
| `make test-int` | Validate SOT integrity, then run contract drift/schema/policy fixtures and TypeScript clients through the public command facade before whitespace checks. |
| `make test-e2e` | Run the complete checker fixture through its public command boundary. |
| `make test` | Run `test-prepare`, `test-unit`, `test-int`, and `test-e2e` serially. |

The pinned Wails executable is `wails3` at exactly `v3.0.0-beta.8`. A `wails`
v2 installation is not a substitute. The host check also requires macOS 14 or
newer on arm64, Git `>=2.39.0,<3.0.0`, and the system `buf` from `PATH` at
`>=1.66.1,<2.0.0`. Buf lints, builds the descriptor used for byte comparison,
and checks the additive baseline. A different in-range Buf that emits different
descriptor bytes fails closed. Language-client generators remain exactly pinned.
The system `bun` resolved from `PATH` must be at least `0.3.14`; newer versions
are accepted and are used directly for contract generation and dependency work.

`make test-prepare` checks the Bun minimum before dependency materialization.
The nested contract Go/Bun graphs match the version manifest and exist only to
compile contract fixtures; they are not an application scaffold. Later Tasks may
extend the tests but must keep the serial facade, deterministic generation, and
fail-closed prerequisites.
