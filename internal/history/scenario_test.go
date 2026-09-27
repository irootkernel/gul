package history_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/domain"
	"github.com/rootkernel/gul/internal/history"
	"github.com/rootkernel/gul/internal/history/contractprovider"
	"github.com/rootkernel/gul/internal/session"
	sessionprovider "github.com/rootkernel/gul/internal/session/contractprovider"
	"github.com/rootkernel/gul/internal/storage"
	"github.com/rootkernel/gul/internal/workspace"
)

type spaces struct{ entry workspace.Attachment }

func (w spaces) Revalidate(_ context.Context, subject, id string) (workspace.Attachment, error) {
	if subject != w.entry.SubjectID || id != w.entry.ID {
		return workspace.Attachment{}, history.ErrAuthority
	}
	return w.entry, nil
}

type carriers struct{ carrier session.Carrier }

func (c carriers) Resolve(_ context.Context, subject, id string) (session.Carrier, error) {
	if subject != "owner" || id != "binding" {
		return session.Carrier{}, history.ErrAuthority
	}
	return c.carrier, nil
}
func TestScenarioHistoryReplaySQLiteReopenClosureAndPublishedResults(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, "gul.sqlite")
	store, err := storage.Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	if err = store.Auth().CreateAccount(ctx, "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	h := scenario.New(time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC))
	inspected, err := h.InspectWorkspace(ctx, &publicv1.InspectWorkspaceRequest{AbsolutePath: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	ws := spaces{workspace.Attachment{SubjectID: "owner", ID: "workspace", ProviderID: inspected.WorkspaceId, CanonicalRoot: "/workspace", FileDevice: "1", FileInode: "1", DisplayName: "Workspace"}}
	if err = store.Presentation().CreateAttachment(ctx, ws.entry); err != nil {
		t.Fatal(err)
	}
	cs := carriers{session.Carrier{ControllerID: "controller", AbsolutePath: "/carrier/controller", Generation: 1}}
	if err = h.RegisterController(scenario.ControllerSpec{ID: cs.carrier.ControllerID, Generation: 1, CarrierPath: cs.carrier.AbsolutePath, OrchestrationLaunch: true, PolicyName: "preprovisioned"}); err != nil {
		t.Fatal(err)
	}
	carrier := &publicv1.ControllerCarrierRef{AbsoluteFilePath: cs.carrier.AbsolutePath, ExpectedControllerId: cs.carrier.ControllerID, ExpectedControllerGeneration: 1}
	root, err := h.StartRun(ctx, &publicv1.StartRunRequest{Workspace: &publicv1.WorkspaceRef{AbsolutePath: "/workspace", ExpectedWorkspaceId: ws.entry.ProviderID}, Controller: carrier, IdempotencyKey: "start", ProfileName: "default", ControlMode: publicv1.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: publicv1.PurposeKind_PURPOSE_KIND_INTERACTIVE})
	if err != nil {
		t.Fatal(err)
	}
	run := &publicv1.RunRef{RunId: root.Run.RunId, Workspace: &publicv1.WorkspaceRef{AbsolutePath: "/workspace", ExpectedWorkspaceId: ws.entry.ProviderID}}
	bind, err := session.NewService(ws, store.Presentation(), sessionprovider.Provider{Port: h}, cs).BindPrimary(ctx, "owner", "workspace", run.RunId, "binding")
	if err != nil {
		t.Fatal(err)
	}
	caps, err := h.GetCapabilities(ctx, &publicv1.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	// The scenario's generic capability fixture omits this field. This checked
	// negotiation fixture supplies the immutable conformance value explicitly.
	caps.Artifacts.MaximumInlineResponseBytes = 1048576
	adapter, err := contractprovider.New(h, caps)
	if err != nil {
		t.Fatal(err)
	}
	svc := history.New(store.Presentation(), ws, cs, adapter)
	texts := []string{"same\r\n한글", "same\r\n한글", strings.Repeat("긴 입력\r\n", 40000)}
	for i, text := range texts {
		r, err := h.GetRun(ctx, &publicv1.GetRunRequest{Run: run})
		if err != nil {
			t.Fatal(err)
		}
		request := &publicv1.SubmitTurnRequest{Run: run, Controller: carrier, IdempotencyKey: fmt.Sprintf("submit-%d", i), Message: text, ExpectedStateRevision: r.Run.StateRevision}
		accepted, err := h.SubmitTurn(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := h.SubmitTurn(ctx, request)
		if err != nil || replay.AcceptedTurn.TurnId != accepted.AcceptedTurn.TurnId {
			t.Fatal("exact replay", err)
		}
		status := publicv1.TurnStatus_TURN_STATUS_COMPLETED
		if i == 1 {
			status = publicv1.TurnStatus_TURN_STATUS_FAILED
		}
		if i == 2 {
			status = publicv1.TurnStatus_TURN_STATUS_INTERRUPTED
		}
		if err = h.CompleteTurn(run.RunId, status); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.ListHistory(ctx, "owner", bind.ID, "", 1)
	if err != nil || len(first.Items) != 1 {
		t.Fatal(first, err)
	}
	var entries []history.Entry
	entries = append(entries, first.Items...)
	token := first.Next
	for token != "" {
		page, err := svc.ListHistory(ctx, "owner", bind.ID, token, 1)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, page.Items...)
		token = page.Next
	}
	if len(entries) != 3 || entries[0].ID == entries[1].ID || entries[2].Ordinal != 3 {
		t.Fatal(entries)
	}
	child, err := h.SpawnSpecialist(run.RunId, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = h.PublishResult(run.RunId, &publicv1.OrchestratedSessionResult{ResultId: fmt.Sprintf("result-%d", i), TaskId: fmt.Sprintf("task-%d", i), SpecialistRun: child, SpecialistRole: "reviewer"}, []byte("# Result\n<script>inert</script>")); err != nil {
			t.Fatal(err)
		}
	}
	results, err := svc.ListResults(ctx, "owner", bind.ID, "", 1)
	if err != nil || len(results.Items) != 1 || results.Next == "" {
		t.Fatal(results, err)
	}
	if err = h.PublishResult(run.RunId, &publicv1.OrchestratedSessionResult{ResultId: "new-result", TaskId: "new-task", SpecialistRun: child, SpecialistRole: "reviewer"}, []byte("later")); err != nil {
		t.Fatal(err)
	}
	remaining, err := svc.ListResults(ctx, "owner", bind.ID, results.Next, 100)
	if err != nil || len(remaining.Items) != 1 || !remaining.Complete {
		t.Fatal("result head changed", remaining, err)
	}
	resultBody, err := svc.ReadChunk(ctx, "owner", bind.ID, results.Items[0].ArtifactRef, 0, uint32(results.Items[0].Length))
	if err != nil || string(resultBody.Data) != "# Result\n<script>inert</script>" {
		t.Fatal(resultBody, err)
	}
	if err = h.CompleteSpecialist(child.RunId); err != nil {
		t.Fatal(err)
	}
	current, err := h.GetRun(ctx, &publicv1.GetRunRequest{Run: run})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.CloseRun(ctx, &publicv1.CloseRunRequest{Run: run, Controller: carrier, ExpectedStateRevision: current.Run.StateRevision}); err == nil {
		t.Fatal("scenario close must first report pending")
	}
	pending, err := h.GetOrchestratedSession(ctx, &publicv1.GetOrchestratedSessionRequest{RootRun: run, Controller: carrier})
	if err != nil || pending.Session.CloseProgress != publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_SETTLING {
		t.Fatal(pending, err)
	}
	if err = h.SetCloseProgress(run.RunId, publicv1.SessionCloseProgress_SESSION_CLOSE_PROGRESS_COMPLETED); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err = contractprovider.New(h, caps)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt := history.New(store.Presentation(), ws, cs, adapter)
	if _, err = rebuilt.ListHistory(ctx, "owner", bind.ID, first.Next, 1); !errors.Is(err, domain.ErrPageTokenExpired) {
		t.Fatal("old token", err)
	}
	page, err := rebuilt.ListHistory(ctx, "owner", bind.ID, "", 100)
	if err != nil || len(page.Items) != 3 || page.RunState != "closed" {
		t.Fatal(page, err)
	}
	for i, e := range page.Items {
		if e.ID != entries[i].ID || e.Ordinal != uint64(i+1) {
			t.Fatal("identity/ordinal changed", e)
		}
		original, err := rebuilt.GetOriginal(ctx, "owner", bind.ID, e.PromptID, true)
		if err != nil {
			t.Fatal(err)
		}
		if original.Inline != nil {
			if *original.Inline != texts[i] {
				t.Fatal("original normalized")
			}
		} else {
			meta, err := rebuilt.GetMetadata(ctx, "owner", bind.ID, original.ArtifactRef)
			if err != nil {
				t.Fatal(err)
			}
			var bytes []byte
			for offset := uint64(0); offset < meta.Length; {
				length := uint32(min(uint64(history.MaximumChunkBytes), meta.Length-offset))
				chunk, err := rebuilt.ReadChunk(ctx, "owner", bind.ID, original.ArtifactRef, offset, length)
				if err != nil {
					t.Fatal(err)
				}
				bytes = append(bytes, chunk.Data...)
				offset += uint64(length)
			}
			if string(bytes) != texts[i] {
				t.Fatal("large original changed")
			}
		}
	}
	if _, err = rebuilt.ListResults(ctx, "owner", bind.ID, results.Next, 1); !errors.Is(err, domain.ErrPageTokenExpired) {
		t.Fatal("old result token", err)
	}
	reopenedResults, err := rebuilt.ListResults(ctx, "owner", bind.ID, "", 100)
	if err != nil || len(reopenedResults.Items) != 3 || reopenedResults.Items[0].ID != results.Items[0].ID || reopenedResults.Items[0].ArtifactRef != results.Items[0].ArtifactRef {
		t.Fatal("closed result reconstruction", reopenedResults, err)
	}
	if metadata, err := rebuilt.GetMetadata(ctx, "owner", bind.ID, results.Items[0].ArtifactRef); err != nil || metadata.Length != results.Items[0].Length {
		t.Fatal(metadata, err)
	}
	conversation, err := rebuilt.ListConversation(ctx, "owner", bind.ID, "", 100)
	if err != nil || len(conversation.Items) != 6 {
		t.Fatal(conversation, err)
	}
}
