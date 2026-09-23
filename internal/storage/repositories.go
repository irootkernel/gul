package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"time"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

type AuthRepository struct{ store *Store }
type PresentationRepository struct{ store *Store }
type CacheRepository struct{ store *Store }
type AttemptRepository struct{ store *Store }
type DeliveryRepository struct{ store *Store }

func (s *Store) Auth() AuthRepository                 { return AuthRepository{s} }
func (s *Store) Presentation() PresentationRepository { return PresentationRepository{s} }
func (s *Store) Cache() CacheRepository               { return CacheRepository{s} }
func (s *Store) Attempts() AttemptRepository          { return AttemptRepository{s} }
func (s *Store) Delivery() DeliveryRepository         { return DeliveryRepository{s} }

func (r AuthRepository) CreateAccount(ctx context.Context, subjectID string, createdAt time.Time) error {
	if subjectID == "" || createdAt.IsZero() {
		return errors.New("invalid account")
	}
	_, err := r.store.writer.ExecContext(ctx, "INSERT INTO app_account(subject_id, created_at) VALUES (?, ?)", subjectID, timestamp(createdAt))
	return err
}

// Only a digest of the bearer token is retained; token creation belongs to E8.
func (r AuthRepository) IssueSession(ctx context.Context, tokenSHA256, subjectID string, expiresAt time.Time) error {
	if !sha256Hex.MatchString(tokenSHA256) || subjectID == "" || expiresAt.IsZero() {
		return errors.New("invalid web session")
	}
	_, err := r.store.writer.ExecContext(ctx, "INSERT INTO web_sessions(token_sha256, subject_id, expires_at) VALUES (?, ?, ?)", tokenSHA256, subjectID, timestamp(expiresAt))
	return err
}

func (r AuthRepository) SessionSubject(ctx context.Context, tokenSHA256 string, now time.Time) (string, error) {
	if !sha256Hex.MatchString(tokenSHA256) || now.IsZero() {
		return "", errors.New("invalid web session lookup")
	}
	var subjectID string
	err := r.store.reader.QueryRowContext(ctx, "SELECT subject_id FROM web_sessions WHERE token_sha256 = ? AND expires_at > ? AND revoked_at IS NULL", tokenSHA256, timestamp(now)).Scan(&subjectID)
	return subjectID, err
}

func (r AuthRepository) RevokeSession(ctx context.Context, tokenSHA256 string, at time.Time) error {
	if !sha256Hex.MatchString(tokenSHA256) || at.IsZero() {
		return errors.New("invalid web session revocation")
	}
	result, err := r.store.writer.ExecContext(ctx, "UPDATE web_sessions SET revoked_at = ? WHERE token_sha256 = ? AND revoked_at IS NULL", timestamp(at), tokenSHA256)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

type WorkspaceEntry struct {
	SubjectID   string
	WorkspaceID string
	DisplayName string
	Hidden      bool
}

type DirectSessionPresentation struct {
	SubjectID   string
	SessionID   string
	WorkspaceID string
	DisplayName string
	Hidden      bool
}

func (r PresentationRepository) PutWorkspace(ctx context.Context, entry WorkspaceEntry) error {
	if entry.SubjectID == "" || entry.WorkspaceID == "" || entry.DisplayName == "" {
		return errors.New("invalid workspace entry")
	}
	_, err := r.store.writer.ExecContext(ctx, `INSERT INTO workspace_entries(subject_id, workspace_id, display_name, hidden)
VALUES (?, ?, ?, ?) ON CONFLICT(subject_id, workspace_id) DO UPDATE SET display_name = excluded.display_name, hidden = excluded.hidden`,
		entry.SubjectID, entry.WorkspaceID, entry.DisplayName, entry.Hidden)
	return err
}

func (r PresentationRepository) Workspace(ctx context.Context, subjectID, workspaceID string) (WorkspaceEntry, error) {
	var entry WorkspaceEntry
	var hidden bool
	err := r.store.reader.QueryRowContext(ctx, "SELECT display_name, hidden FROM workspace_entries WHERE subject_id = ? AND workspace_id = ?", subjectID, workspaceID).Scan(&entry.DisplayName, &hidden)
	entry.SubjectID, entry.WorkspaceID, entry.Hidden = subjectID, workspaceID, hidden
	return entry, err
}

func (r PresentationRepository) PutDirectSession(ctx context.Context, entry DirectSessionPresentation) error {
	if entry.SubjectID == "" || entry.SessionID == "" || entry.WorkspaceID == "" || entry.DisplayName == "" {
		return errors.New("invalid Direct Session presentation")
	}
	_, err := r.store.writer.ExecContext(ctx, `INSERT INTO direct_session_presentations(subject_id, session_id, workspace_id, display_name, hidden)
VALUES (?, ?, ?, ?, ?) ON CONFLICT(subject_id, session_id) DO UPDATE SET workspace_id = excluded.workspace_id, display_name = excluded.display_name, hidden = excluded.hidden`,
		entry.SubjectID, entry.SessionID, entry.WorkspaceID, entry.DisplayName, entry.Hidden)
	return err
}

func (r PresentationRepository) DirectSession(ctx context.Context, subjectID, sessionID string) (DirectSessionPresentation, error) {
	entry := DirectSessionPresentation{SubjectID: subjectID, SessionID: sessionID}
	err := r.store.reader.QueryRowContext(ctx, "SELECT workspace_id, display_name, hidden FROM direct_session_presentations WHERE subject_id = ? AND session_id = ?", subjectID, sessionID).
		Scan(&entry.WorkspaceID, &entry.DisplayName, &entry.Hidden)
	return entry, err
}

type BindingReference struct {
	BindingID            string
	SubjectID            string
	CredentialKey        string
	ExpectedControllerID string
	Health               string
}

func validLogicalKey(key string) bool {
	return fs.ValidPath(key) && path.Clean(key) == key && key != "." && !strings.ContainsRune(key, '\\')
}

func (r PresentationRepository) PutBinding(ctx context.Context, binding BindingReference) error {
	if binding.BindingID == "" || binding.SubjectID == "" || !validLogicalKey(binding.CredentialKey) ||
		binding.ExpectedControllerID == "" || (binding.Health != "unknown" && binding.Health != "healthy" && binding.Health != "blocked") {
		return errors.New("invalid controller binding reference")
	}
	result, err := r.store.writer.ExecContext(ctx, `INSERT INTO controller_binding_references(binding_id, subject_id, credential_key, expected_controller_id, health)
VALUES (?, ?, ?, ?, ?) ON CONFLICT(binding_id) DO UPDATE SET credential_key = excluded.credential_key, expected_controller_id = excluded.expected_controller_id, health = excluded.health WHERE subject_id = excluded.subject_id`,
		binding.BindingID, binding.SubjectID, binding.CredentialKey, binding.ExpectedControllerID, binding.Health)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

type ProjectionStamp struct {
	CapturedHeadCursor       string
	RunStateRevision         uint64
	WriterStateRevision      uint64
	InteractionStateRevision uint64
}

type ProjectionCacheEntry struct {
	SubjectID         string
	SessionID         string
	AggregateKind     string
	Stamp             ProjectionStamp
	Freshness         string
	InvalidationFloor ProjectionStamp
}

// UpstreamCursor is an opaque provider cursor, never a Gul delivery number.
type UpstreamCursor struct{ Value string }

func (r CacheRepository) PutProjectionStamp(ctx context.Context, entry ProjectionCacheEntry) error {
	if entry.SubjectID == "" || entry.SessionID == "" ||
		(entry.AggregateKind != "run" && entry.AggregateKind != "writer" && entry.AggregateKind != "interaction") ||
		entry.Stamp.CapturedHeadCursor == "" || entry.InvalidationFloor.CapturedHeadCursor == "" ||
		(entry.Freshness != "fresh" && entry.Freshness != "stale" && entry.Freshness != "unavailable") {
		return errors.New("invalid projection stamp")
	}
	stamp, err := json.Marshal(entry.Stamp)
	if err != nil {
		return err
	}
	floor, err := json.Marshal(entry.InvalidationFloor)
	if err != nil {
		return err
	}
	_, err = r.store.writer.ExecContext(ctx, `INSERT INTO runtime_projection_cache(subject_id, session_id, aggregate_kind, projection_stamp, freshness, invalidation_floor)
VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(subject_id, session_id, aggregate_kind) DO UPDATE SET projection_stamp = excluded.projection_stamp, freshness = excluded.freshness, invalidation_floor = excluded.invalidation_floor`,
		entry.SubjectID, entry.SessionID, entry.AggregateKind, string(stamp), entry.Freshness, string(floor))
	return err
}

func (r CacheRepository) ProjectionStamp(ctx context.Context, subjectID, sessionID, aggregateKind string) (ProjectionCacheEntry, error) {
	entry := ProjectionCacheEntry{SubjectID: subjectID, SessionID: sessionID, AggregateKind: aggregateKind}
	var stamp, floor string
	err := r.store.reader.QueryRowContext(ctx, "SELECT projection_stamp, freshness, invalidation_floor FROM runtime_projection_cache WHERE subject_id = ? AND session_id = ? AND aggregate_kind = ?", subjectID, sessionID, aggregateKind).
		Scan(&stamp, &entry.Freshness, &floor)
	if err != nil {
		return ProjectionCacheEntry{}, err
	}
	if err := json.Unmarshal([]byte(stamp), &entry.Stamp); err != nil {
		return ProjectionCacheEntry{}, err
	}
	if err := json.Unmarshal([]byte(floor), &entry.InvalidationFloor); err != nil {
		return ProjectionCacheEntry{}, err
	}
	return entry, nil
}

func (r CacheRepository) PutTimelineHead(ctx context.Context, subjectID, sessionID string, cursor UpstreamCursor) error {
	if subjectID == "" || sessionID == "" || cursor.Value == "" {
		return errors.New("invalid timeline head")
	}
	_, err := r.store.writer.ExecContext(ctx, `INSERT INTO runtime_timeline_cache(subject_id, session_id, captured_head_cursor)
VALUES (?, ?, ?) ON CONFLICT(subject_id, session_id) DO UPDATE SET captured_head_cursor = excluded.captured_head_cursor`,
		subjectID, sessionID, cursor.Value)
	return err
}

func (r CacheRepository) TimelineHead(ctx context.Context, subjectID, sessionID string) (UpstreamCursor, error) {
	var cursor UpstreamCursor
	err := r.store.reader.QueryRowContext(ctx, "SELECT captured_head_cursor FROM runtime_timeline_cache WHERE subject_id = ? AND session_id = ?", subjectID, sessionID).Scan(&cursor.Value)
	return cursor, err
}

type ObservationCheckpoint struct {
	ProviderID          string
	RuntimeObjectID     string
	StreamKind          string
	LastValidatedCursor string
	LastCommittedCursor string
}

func (r CacheRepository) PutCheckpoint(ctx context.Context, checkpoint ObservationCheckpoint) error {
	if checkpoint.ProviderID == "" || checkpoint.RuntimeObjectID == "" || checkpoint.StreamKind == "" || checkpoint.LastValidatedCursor == "" {
		return errors.New("invalid observation checkpoint")
	}
	_, err := r.store.writer.ExecContext(ctx, `INSERT INTO observation_checkpoints(provider_id, runtime_object_id, stream_kind, last_validated_cursor, last_committed_cursor)
VALUES (?, ?, ?, ?, ?) ON CONFLICT(provider_id, runtime_object_id, stream_kind) DO UPDATE SET last_validated_cursor = excluded.last_validated_cursor, last_committed_cursor = excluded.last_committed_cursor`,
		checkpoint.ProviderID, checkpoint.RuntimeObjectID, checkpoint.StreamKind, checkpoint.LastValidatedCursor, checkpoint.LastCommittedCursor)
	return err
}

func (r CacheRepository) Checkpoint(ctx context.Context, providerID, runtimeObjectID, streamKind string) (ObservationCheckpoint, error) {
	checkpoint := ObservationCheckpoint{ProviderID: providerID, RuntimeObjectID: runtimeObjectID, StreamKind: streamKind}
	err := r.store.reader.QueryRowContext(ctx, "SELECT last_validated_cursor, last_committed_cursor FROM observation_checkpoints WHERE provider_id = ? AND runtime_object_id = ? AND stream_kind = ?", providerID, runtimeObjectID, streamKind).
		Scan(&checkpoint.LastValidatedCursor, &checkpoint.LastCommittedCursor)
	return checkpoint, err
}

type OperationAttempt struct {
	OperationID          string
	SubjectID            string
	Kind                 string
	RequestSHA256        string
	ReplayKey            string
	ReplayAvailable      bool
	ControllerReferences []ControllerReference
	State                string
	CreatedAt            time.Time
}

type ControllerReference struct {
	Role                 string
	ExpectedControllerID string
	CredentialKey        string
	BindingID            string
}

func validateAttempt(attempt OperationAttempt) error {
	if attempt.OperationID == "" || attempt.SubjectID == "" || attempt.Kind == "" ||
		(attempt.Kind != "StartRun" && attempt.Kind != "SubmitTurn" && attempt.Kind != "ResolveInteraction" && attempt.Kind != "CloseRun") ||
		!sha256Hex.MatchString(attempt.RequestSHA256) || attempt.CreatedAt.IsZero() ||
		attempt.State != "pending" ||
		(attempt.ReplayKey != "" && (attempt.Kind != "StartRun" || !validLogicalKey(attempt.ReplayKey))) ||
		(attempt.ReplayAvailable != (attempt.ReplayKey != "")) ||
		(attempt.ReplayAvailable && len(attempt.ControllerReferences) == 0) {
		return errors.New("invalid provider operation attempt")
	}
	seen := make(map[string]bool, len(attempt.ControllerReferences))
	for _, reference := range attempt.ControllerReferences {
		if (reference.Role != "source" && reference.Role != "destination") || seen[reference.Role] ||
			reference.ExpectedControllerID == "" || (reference.CredentialKey == "") == (reference.BindingID == "") ||
			(reference.CredentialKey != "" && !validLogicalKey(reference.CredentialKey)) ||
			(reference.BindingID != "" && !validLogicalKey(reference.BindingID)) {
			return errors.New("invalid Controller reference")
		}
		seen[reference.Role] = true
	}
	return nil
}

func (r AttemptRepository) Record(ctx context.Context, attempt OperationAttempt) error {
	if err := validateAttempt(attempt); err != nil {
		return err
	}
	references, err := json.Marshal(attempt.ControllerReferences)
	if err != nil {
		return err
	}
	_, err = r.store.writer.ExecContext(ctx, `INSERT INTO provider_operation_attempts
(operation_id, subject_id, operation_kind, request_sha256, replay_key, replay_available, controller_references, state, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, attempt.OperationID, attempt.SubjectID, attempt.Kind, attempt.RequestSHA256,
		nullIfEmpty(attempt.ReplayKey), attempt.ReplayAvailable, string(references), attempt.State, timestamp(attempt.CreatedAt))
	return err
}

func (r AttemptRepository) Get(ctx context.Context, operationID string) (OperationAttempt, error) {
	var attempt OperationAttempt
	var replayKey sql.NullString
	var references string
	var createdAt string
	err := r.store.reader.QueryRowContext(ctx, `SELECT subject_id, operation_kind, request_sha256, replay_key, replay_available, controller_references, state, created_at
FROM provider_operation_attempts WHERE operation_id = ?`, operationID).
		Scan(&attempt.SubjectID, &attempt.Kind, &attempt.RequestSHA256, &replayKey, &attempt.ReplayAvailable, &references, &attempt.State, &createdAt)
	if err != nil {
		return OperationAttempt{}, err
	}
	attempt.OperationID = operationID
	attempt.ReplayKey = replayKey.String
	if err := json.Unmarshal([]byte(references), &attempt.ControllerReferences); err != nil {
		return OperationAttempt{}, err
	}
	attempt.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	return attempt, err
}

func (r AttemptRepository) MarkOutcomeUnknown(ctx context.Context, operationID string) error {
	result, err := r.store.writer.ExecContext(ctx, "UPDATE provider_operation_attempts SET state = 'outcome_unknown' WHERE operation_id = ? AND state = 'pending'", operationID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// ExpireReplay removes replay availability without resolving an unknown outcome.
// The coordinator removes the external replay file before clearing this reference.
func (r AttemptRepository) ExpireReplay(ctx context.Context, operationID string) error {
	if operationID == "" {
		return errors.New("invalid operation ID")
	}
	result, err := r.store.writer.ExecContext(ctx, "UPDATE provider_operation_attempts SET replay_key = NULL, replay_available = 0 WHERE operation_id = ? AND replay_available = 1 AND state IN ('pending', 'outcome_unknown')", operationID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkResolved clears the logical replay reference. The coordinator must remove
// its external replay file before calling this method.
func (r AttemptRepository) MarkResolved(ctx context.Context, operationID string) error {
	if operationID == "" {
		return errors.New("invalid operation ID")
	}
	result, err := r.store.writer.ExecContext(ctx, "UPDATE provider_operation_attempts SET state = 'resolved', replay_key = NULL, replay_available = 0 WHERE operation_id = ? AND state IN ('pending', 'outcome_unknown')", operationID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r DeliveryRepository) Append(ctx context.Context, subjectID, eventKind string, at time.Time) (int64, error) {
	if subjectID == "" || eventKind == "" || at.IsZero() {
		return 0, errors.New("invalid delivery record")
	}
	conn, err := r.store.writer.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return 0, err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var sequence int64
	if err := conn.QueryRowContext(ctx, "UPDATE client_delivery_counter SET next_sequence = next_sequence + 1 WHERE id = 1 RETURNING next_sequence").Scan(&sequence); err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO client_event_journal(sequence, subject_id, event_kind, created_at) VALUES (?, ?, ?, ?)", sequence, subjectID, eventKind, timestamp(at)); err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return 0, fmt.Errorf("commit delivery %d: %w", sequence, err)
	}
	return sequence, nil
}

// Fixed-width UTC timestamps retain chronological ordering in SQLite TEXT comparisons.
func timestamp(at time.Time) string { return at.UTC().Format("2006-01-02T15:04:05.000000000Z") }

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
