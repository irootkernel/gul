package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rootkernel/gul/internal/observation"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/workspace"
)

func observationBinding(t *testing.T, s *Store) observation.Binding {
	t.Helper()
	ctx := t.Context()
	if err := s.Auth().CreateAccount(ctx, "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Presentation().CreateAttachment(ctx, workspace.Attachment{SubjectID: "owner", ID: "ws", CanonicalRoot: "/workspace", ProviderID: "provider-workspace", FileDevice: "1", FileInode: "2", DisplayName: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Presentation().InsertBinding(ctx, session.Binding{SubjectID: "owner", ID: "session", WorkspaceID: "ws", RunID: "run", ControllerBindingID: "controller", ProviderSessionID: "provider-session"}); err != nil {
		t.Fatal(err)
	}
	return observation.Binding{SubjectID: "owner", SessionID: "session", ProviderID: "dolgorae", RunID: "run", WorkspaceID: "provider-workspace", AbsoluteRoot: "/workspace"}
}
func TestObservationAtomicCommitAndRestart(t *testing.T) {
	s, filename := openTestStore(t)
	b := observationBinding(t, s)
	ctx := t.Context()
	repo := s.Observation()
	in := observation.Invalidation{Binding: b, Cursor: "12", Floor: observation.Stamp{Head: "12", Run: 12, Writer: 4, Interaction: 10}, Refresh: observation.Run | observation.Interaction | observation.Timeline, CorrelationID: "gul-correlation", At: time.Now()}
	if _, err := repo.Commit(ctx, in); err == nil {
		t.Fatal("committed unvalidated event")
	}
	if err := repo.Validate(ctx, b, "12"); err != nil {
		t.Fatal(err)
	}
	// Fail after projection writes but before allocation/transaction commit.
	if _, err := s.writer.ExecContext(ctx, `CREATE TRIGGER test_fault BEFORE INSERT ON client_event_journal BEGIN SELECT RAISE(ABORT,'fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(ctx, in); err == nil {
		t.Fatal("missing injected failure")
	}
	cp, err := repo.Checkpoint(ctx, b)
	if err != nil || cp.Validated != "12" || cp.Committed != "0" {
		t.Fatalf("checkpoint %+v %v", cp, err)
	}
	var count int
	if err := s.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM runtime_projection_cache").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial projection %d %v", count, err)
	}
	if _, err := s.writer.ExecContext(ctx, "DROP TRIGGER test_fault"); err != nil {
		t.Fatal(err)
	}
	n, err := repo.Commit(ctx, in)
	if err != nil || n.Sequence != 1 {
		t.Fatalf("delivery %+v %v", n, err)
	}
	entry, err := s.Cache().ProjectionStamp(ctx, b.SubjectID, b.SessionID, "run")
	if err != nil || entry.Freshness != "stale" || entry.InvalidationFloor.RunStateRevision != 12 {
		t.Fatalf("projection %+v %v", entry, err)
	}
	if _, err := repo.Commit(ctx, in); !errors.Is(err, observation.ErrInvalid) {
		t.Fatalf("duplicate commit %v", err)
	}
	foreign := b
	foreign.SubjectID = "other"
	if _, err := repo.Checkpoint(ctx, foreign); !errors.Is(err, observation.ErrUnbound) {
		t.Fatal("foreign binding", err)
	}
	s.Close()
	reopened, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cp, err = reopened.Observation().Checkpoint(ctx, b)
	if err != nil || cp.Validated != "12" || cp.Committed != "12" {
		t.Fatalf("restart %+v %v", cp, err)
	}
	// Delivery allocation belongs to its own domain and cannot advance upstream.
	seq, err := reopened.Delivery().Append(ctx, "owner", "local_presentation", time.Now())
	if err != nil || seq != 2 {
		t.Fatal(seq, err)
	}
	after, _ := reopened.Observation().Checkpoint(ctx, b)
	if after != cp {
		t.Fatal("delivery advanced provider cursor")
	}
}

func TestMigrationFromEveryRetainedVersionToObservationSchema(t *testing.T) {
	for version := 1; version < schemaVersion; version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(dir, "gul.sqlite")
			db, err := sql.Open("sqlite", filename)
			if err != nil {
				t.Fatal(err)
			}
			for migration := 1; migration <= version; migration++ {
				for _, statement := range migrationStatements(migration) {
					if _, err := db.Exec(statement); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.Exec("INSERT INTO schema_migrations(version,digest,applied_at) VALUES (?,?,?)", migration, digestStatements(migrationStatements(migration)), timestamp(time.Now())); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if err := os.Chmod(filename, 0600); err != nil {
				t.Fatal(err)
			}
			store, err := Open(t.Context(), filename)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			var got int
			if err := store.reader.QueryRow("PRAGMA user_version").Scan(&got); err != nil || got != schemaVersion {
				t.Fatal(got, err)
			}
		})
	}
}

func TestRestoredCheckpointRejectsRegressedAggregateStamp(t *testing.T) {
	s, filename := openTestStore(t)
	b := observationBinding(t, s)
	ctx := t.Context()
	in := observation.Invalidation{Binding: b, Cursor: "12", Floor: observation.Stamp{Head: "12", Run: 12, Writer: 4, Interaction: 10}, Refresh: observation.Timeline, CorrelationID: "correlation", At: time.Now()}
	if err := s.Observation().Validate(ctx, b, in.Cursor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Observation().Commit(ctx, in); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cp, err := reopened.Observation().Checkpoint(ctx, b)
	if err != nil || cp.Stamp != in.Floor {
		t.Fatal(cp, err)
	}
	for _, stamp := range []observation.Stamp{{Head: "13", Run: 13, Writer: 3, Interaction: 10}, {Head: "13", Run: 13, Writer: 4, Interaction: 9}} {
		in.Cursor = "13"
		in.Floor = stamp
		if err := reopened.Observation().Validate(ctx, b, in.Cursor); err != nil {
			t.Fatal(err)
		}
		if _, err := reopened.Observation().Commit(ctx, in); !errors.Is(err, observation.ErrInvalid) {
			t.Fatalf("accepted regression: %v", err)
		}
		after, _ := reopened.Observation().Checkpoint(ctx, b)
		if after.Committed != cp.Committed || after.Stamp != cp.Stamp {
			t.Fatal("regression changed checkpoint")
		}
	}
}
func TestRemovedBindingCannotCommitAfterWriterContention(t *testing.T) {
	s, _ := openTestStore(t)
	b := observationBinding(t, s)
	ctx := t.Context()
	repo := s.Observation()
	if err := repo.Validate(ctx, b, "2"); err != nil {
		t.Fatal(err)
	}
	conn, err := s.writer.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	waits := s.writer.Stats().WaitCount
	done := make(chan error, 1)
	go func() {
		_, err := repo.Commit(ctx, observation.Invalidation{Binding: b, Cursor: "2", Floor: observation.Stamp{Head: "2", Run: 2}, Refresh: observation.Run, CorrelationID: "correlation", At: time.Now()})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for s.writer.Stats().WaitCount == waits {
		if time.Now().After(deadline) {
			t.Fatal("commit never waited for writer")
		}
		time.Sleep(time.Millisecond)
	}
	// The writer performs the same local deletion while Commit awaits admission.
	if _, err := conn.ExecContext(ctx, "DELETE FROM direct_session_presentations WHERE subject_id='owner' AND session_id='session'"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM workspace_entries WHERE subject_id='owner' AND workspace_id='ws'"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	select {
	case err := <-done:
		if !errors.Is(err, observation.ErrUnbound) {
			t.Fatalf("unbound commit %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("commit did not finish")
	}
	var count int
	if err := s.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM client_event_journal").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestDeliveryReplayIsBoundedAndSubjectSessionScoped(t *testing.T) {
	s, _ := openTestStore(t)
	b := observationBinding(t, s)
	ctx := t.Context()
	repo := s.Observation()
	for i := 1; i <= observation.MaximumReplay+1; i++ {
		cursor := observation.Cursor(fmt.Sprint(i))
		if err := repo.Validate(ctx, b, cursor); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Commit(ctx, observation.Invalidation{Binding: b, Cursor: cursor, Floor: observation.Stamp{Head: cursor, Run: uint64(i)}, Refresh: observation.Run, CorrelationID: "correlation", At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	fallback, err := repo.ReadDelivery(ctx, "owner", "session", 0)
	if err != nil || !fallback.SnapshotRequired || len(fallback.Events) != 0 || fallback.Head != observation.MaximumReplay+1 {
		t.Fatalf("fallback %+v %v", fallback, err)
	}
	replay, err := repo.ReadDelivery(ctx, "owner", "session", observation.MaximumReplay-1)
	if err != nil || replay.SnapshotRequired || len(replay.Events) != 2 || replay.Events[0].Sequence != observation.MaximumReplay || replay.Events[1].Sequence != observation.MaximumReplay+1 {
		t.Fatalf("replay %+v %v", replay, err)
	}
	if _, err := repo.ReadDelivery(ctx, "other", "session", 0); !errors.Is(err, observation.ErrUnbound) {
		t.Fatal(err)
	}
	if _, err := repo.ReadDelivery(ctx, "owner", "other", 0); !errors.Is(err, observation.ErrUnbound) {
		t.Fatal(err)
	}
	if _, err := repo.ReadDelivery(ctx, "owner", "session", fallback.Head+1); !errors.Is(err, observation.ErrInvalid) {
		t.Fatal(err)
	}
}
