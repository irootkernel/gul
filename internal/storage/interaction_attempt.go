package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rootkernel/gul/internal/interaction"
)

type InteractionAttemptRepository struct{ store *Store }

func (s *Store) InteractionAttempts() InteractionAttemptRepository {
	return InteractionAttemptRepository{s}
}

func (r InteractionAttemptRepository) BeginInteraction(ctx context.Context, b interaction.Bound, interactionID, key string) (string, error) {
	if b.Binding.SubjectID == "" || b.Binding.RunID == "" || b.Binding.ControllerBindingID == "" || b.Carrier.ControllerID == "" || interactionID == "" || key == "" {
		return "", interaction.ErrInvalid
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	id := "interaction_" + hex.EncodeToString(random[:])
	// Protected response bytes and their digest never enter SQLite. The hash
	// binds only non-secret operation identity and the provider key.
	digest := sha256.Sum256([]byte(fmt.Sprintf("ResolveInteraction\x00%s\x00%s\x00%s\x00%s\x00%s", b.Binding.SubjectID, b.Binding.RunID, interactionID, b.Carrier.ControllerID, key)))
	now := time.Now().UTC()
	_, dispatch, err := r.store.Attempts().BeginMutation(ctx, MutationAttempt{OperationAttempt: OperationAttempt{
		OperationID: id, SubjectID: b.Binding.SubjectID, Kind: "ResolveInteraction", RequestSHA256: hex.EncodeToString(digest[:]),
		ControllerReferences: []ControllerReference{{Role: "source", BindingID: b.Binding.ControllerBindingID, ExpectedControllerID: b.Carrier.ControllerID}},
		State:                "pending", CreatedAt: now}, TargetRef: interactionID, DeadlineAt: now.Add(20 * time.Second), ReconciliationRoute: "interaction_refresh"})
	if err != nil {
		if errors.Is(err, ErrMutationConflict) {
			return "", interaction.ErrAttemptConflict
		}
		return "", err
	}
	if !dispatch {
		return "", ErrMutationConflict
	}
	return id, nil
}

func (r InteractionAttemptRepository) FinishInteraction(ctx context.Context, id, outcome string) error {
	if outcome == "" {
		return r.store.Attempts().MarkOutcomeUnknown(ctx, id)
	}
	return r.store.Attempts().ResolveMutation(ctx, id, outcome)
}

func (r InteractionAttemptRepository) ReconcileInteraction(ctx context.Context, b interaction.Bound, interactionID string, status interaction.Status) error {
	if status != interaction.Resolved && status != interaction.Stale {
		return nil
	}
	rows, err := r.store.reader.QueryContext(ctx, `SELECT p.operation_id,p.controller_references FROM provider_operation_attempts p
JOIN mutation_attempt_details d ON d.operation_id=p.operation_id
WHERE p.subject_id=? AND p.operation_kind='ResolveInteraction' AND p.state='outcome_unknown' AND d.target_ref=?`, b.Binding.SubjectID, interactionID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id, encoded string
		if err := rows.Scan(&id, &encoded); err != nil {
			rows.Close()
			return err
		}
		var refs []ControllerReference
		if json.Unmarshal([]byte(encoded), &refs) != nil {
			rows.Close()
			return interaction.ErrBlocked
		}
		if len(refs) == 1 && refs[0].BindingID == b.Binding.ControllerBindingID && refs[0].ExpectedControllerID == b.Carrier.ControllerID {
			ids = append(ids, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	outcome := "resolved"
	if status == interaction.Stale {
		outcome = "stale"
	}
	for _, id := range ids {
		if err := r.store.Attempts().ResolveMutation(ctx, id, outcome); err != nil {
			return err
		}
	}
	return nil
}

var _ interaction.MutationAttempts = InteractionAttemptRepository{}
