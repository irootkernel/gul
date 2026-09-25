# Dolgorae public contract fixture

This directory is E12-T1's checked consumer boundary. `dependency-lock.json`
binds every imported public input to immutable Dolgorae TASK-053 commit
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a`. The descriptor records 36 known
methods while the consumer profile separately identifies 27 required and nine
unavailable methods. Nothing here is executable-release, product-runtime, or
live-provider evidence.

The imported producer lock names task-input revision
`a963f3601b1e7036432501b6d8946c8c62c1466d` because its immutable identity is
the recorded Git blobs plus artifact digests. Gul's dependency lock names the
TASK-053 completion commit that contains that producer lock and correlates the
task-input revision explicitly.

`generate.sh` reproduces the Buf descriptor, proves additive compatibility
against the pre-TASK-053 baseline, and generates typed Go and TypeScript clients,
descriptor-derived fake gRPC server, inventory, policy maps, and conformance
fixtures. `check.sh` regenerates into a temporary directory and rejects drift.
The nested Go and Bun dependency graphs exist only to compile and test these
contract fixtures; they are not a Gul application scaffold.

`port/` defines Gul's checked 27-method consumer interface and translates
accepted typed provider errors into browser-safe codes and actions. Its
`error_catalog_gen.go` is generated from the pinned upstream error policy by
`port/generate-errors.mjs`; run root `make generate-api` to regenerate it and
`make api-check` to verify drift.

`scenario/` implements the 27-method port as a stateful test provider. Drivers
can reset its state, advance its clock, register an orchestration-capable
Controller, append typed events, drive accepted-input and terminal timeline
items through Run operations, and inject faults before or after mutation
acceptance. Scenarios cover accepted identity, pagination, interactions,
results, session close, recovery, and stream isolation without later Gul
features. Drivers may advertise known later methods without adding Gul actions.
Gul has no production adapter or failure fallback to this provider.

Protobuf-ES v2 generates TypeScript message schemas and Connect service
descriptors in the same checked module. The incompatible Connect-ES v1 plugin
and its unchecked `_connect.ts` output are deliberately absent. The imported
`method_coverage` rows are immutable producer case labels, not a claim that Gul
executed one request body per label. Gul strictly decodes the two supplied
aggregate JSON bodies, proves descriptor-complete field sourceability, and
keeps runtime-pending cases outside contract evidence.

The Machine CLI schema remains a separate diagnostic/comparison oracle.
Production dependency injection must use the public local-gRPC adapter only and
must never fall back to CLI after a gRPC failure.
