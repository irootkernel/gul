package storage

// These are Gul-owned records only. Provider state is represented by public
// identifiers, freshness stamps, and non-authoritative projections.
var schemaStatements = []string{
	`CREATE TABLE schema_migrations (
version INTEGER PRIMARY KEY,
digest TEXT NOT NULL,
applied_at TEXT NOT NULL
)`,
	`CREATE TABLE app_account (
subject_id TEXT PRIMARY KEY,
created_at TEXT NOT NULL
)`,
	`CREATE TABLE web_sessions (
token_sha256 TEXT PRIMARY KEY,
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
expires_at TEXT NOT NULL,
revoked_at TEXT
)`,
	`CREATE TABLE runtime_attachments (
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
runtime_id TEXT NOT NULL,
attached_at TEXT NOT NULL,
PRIMARY KEY(subject_id, runtime_id)
)`,
	`CREATE TABLE workspace_entries (
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
workspace_id TEXT NOT NULL,
display_name TEXT NOT NULL,
hidden INTEGER NOT NULL DEFAULT 0 CHECK(hidden IN (0, 1)),
PRIMARY KEY(subject_id, workspace_id)
)`,
	`CREATE TABLE direct_session_presentations (
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
session_id TEXT NOT NULL,
workspace_id TEXT NOT NULL,
display_name TEXT NOT NULL,
hidden INTEGER NOT NULL DEFAULT 0 CHECK(hidden IN (0, 1)),
PRIMARY KEY(subject_id, session_id)
)`,
	`CREATE TABLE controller_binding_references (
binding_id TEXT PRIMARY KEY,
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
credential_key TEXT NOT NULL,
expected_controller_id TEXT NOT NULL,
health TEXT NOT NULL CHECK(health IN ('unknown', 'healthy', 'blocked'))
)`,
	`CREATE TABLE navigation_state (
subject_id TEXT PRIMARY KEY REFERENCES app_account(subject_id) ON DELETE CASCADE,
workspace_id TEXT,
session_id TEXT
)`,
	`CREATE TABLE client_delivery_counter (
id INTEGER PRIMARY KEY CHECK(id = 1),
next_sequence INTEGER NOT NULL CHECK(next_sequence >= 0)
)`,
	`CREATE TABLE client_event_journal (
sequence INTEGER PRIMARY KEY CHECK(sequence > 0),
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
event_kind TEXT NOT NULL,
created_at TEXT NOT NULL
)`,
	`CREATE TABLE file_explorer_state (
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
workspace_id TEXT NOT NULL,
relative_path TEXT NOT NULL,
PRIMARY KEY(subject_id, workspace_id)
)`,
	`CREATE TABLE runtime_projection_cache (
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
session_id TEXT NOT NULL,
aggregate_kind TEXT NOT NULL CHECK(aggregate_kind IN ('run', 'writer', 'interaction')),
projection_stamp TEXT NOT NULL,
freshness TEXT NOT NULL CHECK(freshness IN ('fresh', 'stale', 'unavailable')),
invalidation_floor TEXT NOT NULL,
PRIMARY KEY(subject_id, session_id, aggregate_kind)
)`,
	`CREATE TABLE runtime_timeline_cache (
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
session_id TEXT NOT NULL,
captured_head_cursor TEXT NOT NULL,
PRIMARY KEY(subject_id, session_id)
)`,
	`CREATE TABLE observation_checkpoints (
provider_id TEXT NOT NULL,
runtime_object_id TEXT NOT NULL,
stream_kind TEXT NOT NULL,
last_validated_cursor TEXT NOT NULL,
last_committed_cursor TEXT NOT NULL,
PRIMARY KEY(provider_id, runtime_object_id, stream_kind)
)`,
	`CREATE TABLE provider_operation_attempts (
operation_id TEXT PRIMARY KEY,
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
operation_kind TEXT NOT NULL,
request_sha256 TEXT NOT NULL,
replay_key TEXT,
replay_available INTEGER NOT NULL CHECK(replay_available IN (0, 1)),
controller_references TEXT NOT NULL,
state TEXT NOT NULL CHECK(state IN ('pending', 'outcome_unknown', 'resolved')),
created_at TEXT NOT NULL
)`,
}

// Each unique key backs a related-attachment match in workspace registration;
// contention returns an existing entry only when all identity fields match.
var attachmentStatements = []string{
	`CREATE TABLE workspace_attachments (
subject_id TEXT NOT NULL,
workspace_id TEXT NOT NULL,
canonical_root TEXT NOT NULL,
provider_workspace_id TEXT NOT NULL,
file_device TEXT NOT NULL,
file_inode TEXT NOT NULL,
PRIMARY KEY(subject_id, workspace_id),
UNIQUE(subject_id, canonical_root),
UNIQUE(subject_id, provider_workspace_id),
FOREIGN KEY(subject_id, workspace_id) REFERENCES workspace_entries(subject_id, workspace_id) ON DELETE CASCADE
)`,
}

var presentationStatements = []string{
	`CREATE TABLE workspace_favorites (
subject_id TEXT NOT NULL,
workspace_id TEXT NOT NULL,
PRIMARY KEY(subject_id, workspace_id),
FOREIGN KEY(subject_id, workspace_id) REFERENCES workspace_attachments(subject_id, workspace_id) ON DELETE CASCADE
)`,
	`CREATE TABLE direct_session_favorites (
subject_id TEXT NOT NULL,
session_id TEXT NOT NULL,
PRIMARY KEY(subject_id, session_id),
FOREIGN KEY(subject_id, session_id) REFERENCES direct_session_presentations(subject_id, session_id) ON DELETE CASCADE
)`,
}

var sessionStatements = []string{
	`CREATE TABLE primary_session_bindings (
subject_id TEXT NOT NULL,
session_id TEXT NOT NULL,
workspace_id TEXT NOT NULL,
run_id TEXT NOT NULL,
controller_binding_id TEXT NOT NULL,
provider_session_id TEXT NOT NULL,
configuration_json TEXT NOT NULL,
observed_at TEXT,
PRIMARY KEY(subject_id, session_id),
UNIQUE(subject_id, workspace_id, run_id),
FOREIGN KEY(subject_id, session_id) REFERENCES direct_session_presentations(subject_id, session_id) ON DELETE CASCADE,
FOREIGN KEY(subject_id, workspace_id) REFERENCES workspace_attachments(subject_id, workspace_id) ON DELETE CASCADE
)`,
}

var observationStatements = []string{
	`CREATE TABLE observation_checkpoint_stamps (
provider_id TEXT NOT NULL,
runtime_object_id TEXT NOT NULL,
stream_kind TEXT NOT NULL,
stamp TEXT NOT NULL,
PRIMARY KEY(provider_id, runtime_object_id, stream_kind),
FOREIGN KEY(provider_id, runtime_object_id, stream_kind) REFERENCES observation_checkpoints(provider_id, runtime_object_id, stream_kind) ON DELETE CASCADE
)`,
	`CREATE TABLE observation_refreshes (
subject_id TEXT NOT NULL REFERENCES app_account(subject_id) ON DELETE CASCADE,
session_id TEXT NOT NULL,
refresh_mask INTEGER NOT NULL,
PRIMARY KEY(subject_id, session_id),
FOREIGN KEY(subject_id, session_id) REFERENCES direct_session_presentations(subject_id, session_id) ON DELETE CASCADE
)`,
	`CREATE TABLE client_projection_notifications (
sequence INTEGER PRIMARY KEY REFERENCES client_event_journal(sequence) ON DELETE CASCADE,
session_id TEXT NOT NULL,
correlation_id TEXT NOT NULL
)`,
}

var closeStatements = []string{
	`CREATE TABLE session_close_attempts (
attempt_id TEXT PRIMARY KEY REFERENCES provider_operation_attempts(operation_id) ON DELETE CASCADE,
subject_id TEXT NOT NULL,
session_id TEXT NOT NULL,
request_id TEXT NOT NULL,
request_sha256 TEXT NOT NULL,
operation_kind TEXT NOT NULL CHECK(operation_kind IN ('CloseRun', 'RecoverRun', 'ReconcileRun')),
interrupt INTEGER NOT NULL CHECK(interrupt IN (0, 1)),
created_at TEXT NOT NULL,
dispatch_finished INTEGER NOT NULL CHECK(dispatch_finished IN (0, 1)),
outcome_status TEXT NOT NULL CHECK(outcome_status IN ('rejected', 'in_progress', 'confirmed', 'outcome_unknown', 'recovery_required')),
operation_ref TEXT NOT NULL,
code TEXT NOT NULL,
next_action TEXT NOT NULL,
UNIQUE(subject_id, session_id, request_id),
FOREIGN KEY(subject_id, session_id) REFERENCES primary_session_bindings(subject_id, session_id) ON DELETE CASCADE
)`,
	`CREATE TABLE session_close_operations (
subject_id TEXT NOT NULL,
session_id TEXT NOT NULL,
controller_binding_id TEXT NOT NULL,
provider_operation_id TEXT NOT NULL,
operation_ref TEXT NOT NULL UNIQUE,
PRIMARY KEY(subject_id, session_id, controller_binding_id, provider_operation_id),
FOREIGN KEY(subject_id, session_id) REFERENCES primary_session_bindings(subject_id, session_id) ON DELETE CASCADE
)`,
}
