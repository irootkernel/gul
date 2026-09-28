package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rootkernel/gul/internal/action"
)

type WriterAttemptRepository struct{ store *Store }

func (s *Store) WriterAttempts() WriterAttemptRepository { return WriterAttemptRepository{s} }

func (r WriterAttemptRepository) BeginWriter(ctx context.Context, b action.Bound, acquire bool, revision uint64) (string, error) {
	if b.Binding.SubjectID == "" || b.Binding.RunID == "" || b.Binding.ControllerBindingID == "" || b.Carrier.ControllerID == "" || revision == 0 {
		return "", action.ErrInvalid
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	id := "writer_" + hex.EncodeToString(random[:])
	kind := "ReleaseWriter"
	if acquire {
		kind = "AcquireWriter"
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", kind, b.Binding.SubjectID, b.Binding.RunID, b.Carrier.ControllerID, revision)))
	now := time.Now().UTC()
	_, dispatch, err := r.store.Attempts().BeginMutation(ctx, MutationAttempt{OperationAttempt: OperationAttempt{
		OperationID: id, SubjectID: b.Binding.SubjectID, Kind: kind, RequestSHA256: hex.EncodeToString(digest[:]),
		ControllerReferences: []ControllerReference{{Role: "source", BindingID: b.Binding.ControllerBindingID, ExpectedControllerID: b.Carrier.ControllerID}},
		State:                "pending", CreatedAt: now}, TargetRef: b.Binding.RunID, DeadlineAt: now.Add(15 * time.Second), ReconciliationRoute: "run_writer"})
	if err != nil {
		if errors.Is(err, ErrMutationConflict) {
			return "", action.ErrOutcomeUnknown
		}
		return "", err
	}
	if !dispatch {
		return "", ErrMutationConflict
	}
	return id, nil
}

func (r WriterAttemptRepository) FinishWriter(ctx context.Context, id, outcome string) error {
	if outcome == "" {
		return r.store.Attempts().MarkOutcomeUnknown(ctx, id)
	}
	return r.store.Attempts().ResolveMutation(ctx, id, outcome)
}

var _ action.MutationAttempts = WriterAttemptRepository{}
