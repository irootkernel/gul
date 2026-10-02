package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
)

var controllerStoreStatements = []string{
	`CREATE TABLE app_installation (
singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
installation_id TEXT NOT NULL
)`,
	`CREATE TABLE controller_credential_metadata (
binding_id TEXT PRIMARY KEY REFERENCES controller_binding_references(binding_id) ON DELETE CASCADE,
generation TEXT NOT NULL,
instance_id TEXT NOT NULL,
file_identity TEXT NOT NULL,
removing INTEGER NOT NULL DEFAULT 0 CHECK(removing IN (0,1)),
allocation_started INTEGER NOT NULL DEFAULT 0 CHECK(allocation_started IN (0,1)),
allocation_key TEXT NOT NULL DEFAULT '',
allocation_workspace TEXT NOT NULL DEFAULT ''
)`,
}

// FileIdentity is non-secret local metadata. Replacing a carrier requires
// verified adoption rather than silently retargeting its logical reference.
type FileIdentity struct {
	Device, Inode  uint64
	Size, Modified int64
}

type ControllerCredential struct {
	BindingReference
	Generation                         uint64
	InstanceID                         string
	FileIdentity                       FileIdentity
	Removing, AllocationStarted        bool
	AllocationKey, AllocationWorkspace string
}

func (r PresentationRepository) InstallationID(ctx context.Context) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	if _, err = r.store.writer.ExecContext(ctx, `INSERT OR IGNORE INTO app_installation(singleton,installation_id) VALUES(1,?)`, id.String()); err != nil {
		return "", err
	}
	var value string
	err = r.store.reader.QueryRowContext(ctx, `SELECT installation_id FROM app_installation WHERE singleton=1`).Scan(&value)
	parsed, parseErr := uuid.Parse(value)
	if err != nil || parseErr != nil || parsed.Version() != 7 || parsed.String() != value {
		return "", session.ErrCarrierUnavailable
	}
	return value, nil
}

// AccountSubject comes from trusted account setup, never an RPC-supplied principal.
func (r PresentationRepository) AccountSubject(ctx context.Context) (string, error) {
	var value string
	err := r.store.reader.QueryRowContext(ctx, `SELECT subject_id FROM app_account WHERE (SELECT COUNT(*) FROM app_account)=1`).Scan(&value)
	if err != nil || value == "" {
		return "", session.ErrCarrierUnavailable
	}
	return value, nil
}

func (r PresentationRepository) ControllerCredential(ctx context.Context, subject, binding string) (ControllerCredential, error) {
	return r.controllerCredential(ctx, "b.subject_id=? AND b.binding_id=?", subject, binding)
}
func (r PresentationRepository) CredentialByKey(ctx context.Context, subject, key string) (ControllerCredential, error) {
	return r.controllerCredential(ctx, "b.subject_id=? AND b.credential_key=?", subject, key)
}
func (r PresentationRepository) controllerCredential(ctx context.Context, where, subject, value string) (ControllerCredential, error) {
	var c ControllerCredential
	var generation, identity string
	rows, err := r.store.reader.QueryContext(ctx, `SELECT b.binding_id,b.subject_id,b.credential_key,b.expected_controller_id,b.health,m.generation,m.instance_id,m.file_identity,m.removing,m.allocation_started,m.allocation_key,m.allocation_workspace FROM controller_binding_references b JOIN controller_credential_metadata m USING(binding_id) WHERE `+where, subject, value)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	if !rows.Next() {
		return c, sql.ErrNoRows
	}
	if err = rows.Scan(&c.BindingID, &c.SubjectID, &c.CredentialKey, &c.ExpectedControllerID, &c.Health, &generation, &c.InstanceID, &identity, &c.Removing, &c.AllocationStarted, &c.AllocationKey, &c.AllocationWorkspace); err != nil {
		return ControllerCredential{}, err
	}
	if rows.Next() {
		return ControllerCredential{}, session.ErrCarrierUnavailable
	}
	if err = rows.Err(); err != nil {
		return ControllerCredential{}, err
	}
	c.Generation, err = strconv.ParseUint(generation, 10, 64)
	if err != nil || c.Generation == 0 || json.Unmarshal([]byte(identity), &c.FileIdentity) != nil {
		return ControllerCredential{}, session.ErrCarrierUnavailable
	}
	return c, nil
}

func validCredential(c ControllerCredential) bool {
	return c.BindingID != "" && c.SubjectID != "" && validLogicalKey(c.CredentialKey) && c.ExpectedControllerID != "" && c.InstanceID != "" && c.Generation > 0 && c.FileIdentity.Inode > 0 && c.FileIdentity.Size > 0 && (c.Health == "healthy" || c.Health == "unknown")
}
func insertCredential(ctx context.Context, tx *sql.Tx, c ControllerCredential) error {
	if !validCredential(c) {
		return session.ErrCarrierUnavailable
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM controller_binding_references WHERE credential_key=? OR expected_controller_id=?`, c.CredentialKey, c.ExpectedControllerID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return session.ErrBindingConflict
	}
	identity, _ := json.Marshal(c.FileIdentity)
	if _, err := tx.ExecContext(ctx, `INSERT INTO controller_binding_references(binding_id,subject_id,credential_key,expected_controller_id,health) VALUES(?,?,?,?,?)`, c.BindingID, c.SubjectID, c.CredentialKey, c.ExpectedControllerID, c.Health); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO controller_credential_metadata(binding_id,generation,instance_id,file_identity,allocation_started) VALUES(?,?,?,?,?)`, c.BindingID, strconv.FormatUint(c.Generation, 10), c.InstanceID, string(identity), c.AllocationStarted)
	return err
}
func (r PresentationRepository) RegisterCredential(ctx context.Context, c ControllerCredential) error {
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insertCredential(ctx, tx, c); err != nil {
		return err
	}
	return tx.Commit()
}

// AdoptCredential compares the exact prior binding and invalidates dependent
// projections in the same transaction. No provider or filesystem work occurs here.
func (r PresentationRepository) AdoptCredential(ctx context.Context, previous session.Binding, c ControllerCredential) (session.Binding, error) {
	if c.SubjectID != previous.SubjectID {
		return session.Binding{}, session.ErrCarrierUnavailable
	}
	tx, err := r.store.writer.BeginTx(ctx, nil)
	if err != nil {
		return session.Binding{}, err
	}
	defer tx.Rollback()
	var controller, run, workspace, provider string
	if err = tx.QueryRowContext(ctx, `SELECT controller_binding_id,run_id,workspace_id,provider_session_id FROM primary_session_bindings WHERE subject_id=? AND session_id=?`, previous.SubjectID, previous.ID).Scan(&controller, &run, &workspace, &provider); err != nil {
		return session.Binding{}, session.ErrBindingConflict
	}
	if controller != previous.ControllerBindingID || run != previous.RunID || workspace != previous.WorkspaceID || provider != previous.ProviderSessionID {
		return session.Binding{}, session.ErrBindingConflict
	}
	var shared int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM primary_session_bindings WHERE controller_binding_id=? AND NOT(subject_id=? AND session_id=?)`, previous.ControllerBindingID, previous.SubjectID, previous.ID).Scan(&shared); err != nil {
		return session.Binding{}, err
	}
	if shared != 0 {
		return session.Binding{}, session.ErrBindingConflict
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM controller_binding_references WHERE subject_id=? AND binding_id=?`, previous.SubjectID, previous.ControllerBindingID); err != nil {
		return session.Binding{}, err
	}
	if err = insertCredential(ctx, tx, c); err != nil {
		return session.Binding{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE primary_session_bindings SET controller_binding_id=? WHERE subject_id=? AND session_id=?`, c.BindingID, previous.SubjectID, previous.ID); err != nil {
		return session.Binding{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runtime_projection_cache SET freshness='stale' WHERE subject_id=? AND session_id=?`, previous.SubjectID, previous.ID); err != nil {
		return session.Binding{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO observation_refreshes(subject_id,session_id,refresh_mask) VALUES(?,?,?) ON CONFLICT(subject_id,session_id) DO UPDATE SET refresh_mask=refresh_mask|excluded.refresh_mask`, previous.SubjectID, previous.ID, uint16(observation.AllAggregates|observation.Session|observation.Artifacts)); err != nil {
		return session.Binding{}, err
	}
	if err = tx.Commit(); err != nil {
		return session.Binding{}, err
	}
	previous.ControllerBindingID = c.BindingID
	return previous, nil
}

// ReserveCredentialRemoval excludes concurrent local binding and StartRun
// dispatch. The file is removed only after this short transaction has ended.
func (r PresentationRepository) ReserveCredentialRemoval(ctx context.Context, subject, binding string) error {
	result, err := r.store.writer.ExecContext(ctx, `UPDATE controller_credential_metadata SET removing=1 WHERE binding_id=? AND allocation_started=0 AND EXISTS(SELECT 1 FROM controller_binding_references WHERE binding_id=? AND subject_id=?) AND NOT EXISTS(SELECT 1 FROM primary_session_bindings WHERE controller_binding_id=?) AND NOT EXISTS(SELECT 1 FROM provider_operation_attempts a WHERE a.subject_id=? AND a.state IN ('pending','outcome_unknown') AND (NOT json_valid(a.controller_references) OR EXISTS(SELECT 1 FROM json_each(CASE WHEN json_valid(a.controller_references) THEN a.controller_references ELSE '[]' END) j WHERE json_extract(j.value,'$.BindingID')=?)))`, binding, binding, subject, binding, subject, binding)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return session.ErrCarrierUnavailable
	}
	return nil
}

func (r PresentationRepository) MarkCredentialAllocated(ctx context.Context, subject, binding, key, workspace string) error {
	if !validLogicalKey(key) || len(key) > 256 || workspace == "" || len(workspace) > 256 {
		return session.ErrCarrierUnavailable
	}
	result, err := r.store.writer.ExecContext(ctx, `UPDATE controller_credential_metadata SET allocation_started=1,allocation_key=?,allocation_workspace=? WHERE binding_id=? AND removing=0 AND (allocation_started=0 OR (allocation_key=? AND allocation_workspace=?)) AND EXISTS(SELECT 1 FROM controller_binding_references WHERE binding_id=? AND subject_id=?)`, key, workspace, binding, key, workspace, binding, subject)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return session.ErrCarrierUnavailable
	}
	return nil
}
func (r PresentationRepository) RemoveCredentialReference(ctx context.Context, subject, binding string) error {
	_, err := r.store.writer.ExecContext(ctx, `DELETE FROM controller_binding_references WHERE subject_id=? AND binding_id=? AND NOT EXISTS(SELECT 1 FROM primary_session_bindings WHERE controller_binding_id=?)`, subject, binding, binding)
	return err
}
