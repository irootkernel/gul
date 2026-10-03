# Dolgorae public contract fixture

This directory contains the checked consumer boundary. `dependency-lock.json`
binds every imported public input to the published Dolgorae v0.1.3 source
`07dc31331d03aae9ed7c0c862a0cbe8a5184024e` and records the published archive,
executable digest and advertised capabilities. The unchanged descriptor records
36 known methods; the consumer profile identifies 27 required and nine
unavailable methods. E12-T1's original TASK-053 completion at
`21aefe5b2a8dc6fb18a58338090348b23d2f0a4a` remains historical contract evidence.
Production assembly and action acceptance belong to E2-T1 through E2-T3.

The imported producer lock names task-input revision
`a963f3601b1e7036432501b6d8946c8c62c1466d` because its immutable identity is
the recorded Git blobs plus artifact digests. Gul's dependency lock names the
released source and retains the TASK-053 completion identity separately from
the producer's task-input revision.

`admission-policy.json` is Gul's operation-specific policy authority. Generation
derives `generated/policy/release-admission.v1.json` from it and the published
capability snapshot. Transport admission requires the 27 methods and their
relevant features. `reader_writer_access=false` and
`brokered_independent_subagent_runs=false` remain advertised facts and do not
decide every action. A dedicated threadless first WRITE requires lane writer
support, `first_write_via_submit_turn`, durable authority and fresh typed state;
it uses SubmitTurn without AcquireWriter. A declared UNKNOWN/UNVERIFIED
best-effort policy can admit READ and same-Run ACTIVE Writer WRITE without
claiming verified access. Release uses fresh held-writer authority and its
ownerless response preserves the workspace-only projection. Existing-reader
acquisition requires
verified Profile transition support. The release advertises that transition as
unverified. Shared-readonly WRITE, threadless Acquire and Gul continuation are
unavailable. Every release-admitted action still requires current Run, Profile,
Controller, Writer and Session guards at execution.

The release changes descriptor metadata, the producer lock, verification index
and SubmitTurn's `SESSION_CLOSE_IN_PROGRESS` action mapping. Proto, descriptor
bytes, credential schema, consumer profile and client/mutation policies are
unchanged. Imported inputs retain producer bytes and exact per-file digests.

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

Durable scenario events retain the stamp captured at their commit, including on
replay. Each subscription selects `minimal` or `operational` version 1 without
changing another subscriber's event. An omitted profile defaults to
`minimal`; an omitted version defaults to 1 and preserves an explicit profile.
These are fixture conveniences, not guarantees of the released provider's
handling of omitted fields. Unknown profiles and unsupported versions
are rejected. Run, Interaction and timeline snapshots observe the current
workspace writer state revision, independently of writer generation. The
Writer stamp uses its owner Run's captured boundary when owned; an ownerless
Writer has empty/zero Run components and unknown/unverified policy with no lane
or assurance. Fresh reads never change the retained accepted responses. These
tests do not establish Gul's cross-aggregate convergence or live-provider
acceptance.

The pinned producer's SPEC-006 and Projection revision authority also govern
cursor semantics. Event and timeline cursors are canonical decimal strings in
one Run ledger. A snapshot head represents the current ledger revision,
including records omitted from the event projection. Interaction revision is
the ledger sequence of its latest view change, including lifecycle transitions
after the first Interaction. Exclusive resume accepts zero
and filtered gaps; malformed or beyond-head cursors fail. Each timeline page
captures its own current head, so later pages can include intervening appends.
Session-result page tokens retain their separate opaque snapshot scope.
Heartbeats and stream ends report the captured head without advancing it.

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
