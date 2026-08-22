# Dolgorae public contract fixture

This directory is E0-T7's checked consumer boundary. `dependency-lock.json`
binds every imported public input to Dolgorae revision
`85a8862f784cc57701751d81a9e03bf7c5722818` and the descriptor's exact
Protobuf v32.1 WKT source. Nothing here is executable-release or live-runtime
evidence.

`generate.sh` reproduces the descriptor, typed Go and TypeScript clients,
descriptor-derived fake gRPC server, inventory, policy maps, and conformance
fixtures. `check.sh` regenerates into a temporary directory and rejects drift.
The nested Go and Bun dependency graphs exist only to compile and test these
contract fixtures; they are not a Gul application scaffold.

The Machine CLI schema remains a separate diagnostic/comparison oracle.
Production dependency injection must use the public local-gRPC adapter only and
must never fall back to CLI after a gRPC failure.
