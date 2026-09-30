# Deferred feedback

This is the canonical owner for independent, low-priority follow-up found
after an Epic closes. The roadmap owns task state and order. An entry here
does not promote a requirement, reopen an Epic, or authorize implementation.

| ID | Follow-up | Current impact | Re-entry condition |
| --- | --- | --- | --- |
| E6-FB-001 | Make `internal/files/watch_host_other.go` register the verified node rather than a pathname, or return `ErrWatchUnavailable` and use explicit refresh on hosts without a descriptor-faithful watcher. Test replacement of a watched path on that host. | A path swap can cause missed or spurious invalidations on non-Darwin hosts. Content reads still pass through the guarded accessor; the supported macOS watcher uses verified descriptors. | Before a non-Darwin host becomes a supported FileService delivery target. |
| E6-FB-002 | Add a deterministic assertion that a burst of file events produces one invalidation within the bounded coalescing interval in `internal/files/watch.go`. | The 100 ms coalescing behavior is implemented but lacks a direct timing assertion; node, scan, slot, and queue bounds have tests. | Before watcher-driven file-pane refresh is first mounted. |
| E6-FB-003 | Exercise canceled and unavailable watcher paths through `internal/delivery/api/file.go` and assert their Connect status and typed domain errors. | The mappings are implemented, and the watcher slot-limit mapping has an API test, but these remaining error branches lack handler-level coverage. | Before FileService API routes are mounted for the assembled product. |
| E7-FB-001 | Exercise FilePane continuation against a changing directory and deduplicate overlapping entries by identity, or reload from a fresh directory cursor. | Current E7 browser fixtures use stable directory lists. A live continuation can repeat entries when mutations shift directory offsets. | Before FileService API routes or watcher-driven directory refresh are mounted for authenticated delivery, or when overlapping pages are observed. |
| E7-FB-002 | Test root reuse, changed-root refusal and writer-active propagation through the exported `mountOperator` entry point. | Current browser fixtures mount operator components directly, so they do not exercise the public mounting boundary. | Before E8 mounts the authenticated operator entry point. |
| E7-FB-003 | Add delayed Prompt History original reads that settle after selection changes or refresh, including success and failure outcomes. | The original-read cleanup guards are implemented, but these orderings lack direct browser assertions. Conversation original-read races are covered separately. | Before Prompt History is first mounted in authenticated product assembly. |
