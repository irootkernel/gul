package provider_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	contract "github.com/rootkernel/gul/contract"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/internal/composition"
	"github.com/rootkernel/gul/internal/controller"
	"github.com/rootkernel/gul/internal/gateway"
	"github.com/rootkernel/gul/internal/storage"
	"google.golang.org/protobuf/proto"
)

// This is published Dolgorae and actual files/gRPC with an upstream native fake
// app-server. It qualifies carrier semantics, not live model or E2-T3 UI behavior.
func TestPublishedControllerCarriersAndAdoption(t *testing.T) {
	binary, archive, source := os.Getenv("GUL_E2_DOLGORAE_EXECUTABLE"), os.Getenv("GUL_E2_DOLGORAE_ARCHIVE"), os.Getenv("GUL_E2_DOLGORAE_SOURCE")
	if binary == "" {
		t.Skip("published provider qualification is opt-in")
	}
	if source == "" {
		t.Fatal("matching v0.1.3 source fixtures required")
	}
	root, err := os.MkdirTemp(providerTemporaryBase(), "gul-c-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	release := contract.QualifiedRelease()
	binary, err = qualifyReleaseArtifacts(archive, binary, release.Archive.SHA256, release.Executable.SHA256, root)
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("fixture Python unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	prepare := exec.CommandContext(ctx, python, "fixtures/prepare_controller.py", binary, root, source)
	prepare.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "TMPDIR="+filepath.Join(root, "tmp"))
	if output, err := prepare.CombinedOutput(); err != nil {
		t.Fatalf("isolated native fake setup: %v (%s)", err, output)
	}
	home, work := filepath.Join(root, "home"), filepath.Join(root, "workspace")
	g := gateway.New(gateway.Config{Executable: binary, Home: home, WorkspaceRoots: []string{work}})
	if err = g.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := g.Stop(stop); err != nil {
			t.Error(err)
		}
	})
	caps := g.NegotiatedCapabilities()
	ready, cancelReady := context.WithTimeout(ctx, 15*time.Second)
	defer cancelReady()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for caps == nil {
		select {
		case <-ready.Done():
			t.Fatal("published handshake unavailable", g.Status())
		case <-ticker.C:
			caps = g.NegotiatedCapabilities()
		}
	}
	data := filepath.Join(root, "data")
	if err = os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(ctx, filepath.Join(data, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Auth().CreateAccount(ctx, "carrier-subject", time.Now()); err != nil {
		t.Fatal(err)
	}
	carriers, err := controller.New(ctx, db, home, []string{work}, caps)
	if err != nil {
		t.Fatal(err)
	}
	g.SetCarrierValidator(carriers.ValidateRPC)
	r, err := composition.NewQualified(db, composition.Config{Port: g, Carriers: carriers, Roots: []string{work}, Policies: []string{"carrier-test"}}, caps)
	if err != nil {
		t.Fatal(err)
	}
	roots := r.Workspaces.ListRegistrableRoots()
	if len(roots) != 1 {
		t.Fatal("isolated registrable root unavailable")
	}
	attachment, err := r.Workspaces.RegisterFromAllowlistPath(ctx, "carrier-subject", roots[0].ID, ".")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := carriers.Create(ctx, "carrier-subject", "carrier-test")
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := carriers.Resolve(ctx, "carrier-subject", credential.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	ref := &pb.WorkspaceRef{AbsolutePath: work, ExpectedWorkspaceId: attachment.ProviderID}
	startRequest := &pb.StartRunRequest{Workspace: ref, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: carrier.Generation}, IdempotencyKey: uuid.NewString(), ProfileName: "default", ControlMode: pb.ControlMode_CONTROL_MODE_DIRECT_INTERACTIVE, ExecutionLane: pb.ExecutionLane_EXECUTION_LANE_DEDICATED, Purpose: pb.PurposeKind_PURPOSE_KIND_IMPLEMENTATION, Model: proto.String("gpt-5.6"), Effort: proto.String("medium"), Instructions: proto.String("Exercise the isolated Controller carrier contract."), RequiredAssurance: pb.AssuranceLevel_ASSURANCE_LEVEL_BEST_EFFORT_PERSONAL_ALPHA}
	started, err := g.StartRun(ctx, startRequest)
	if err != nil {
		t.Fatal("published carrier StartRun rejected", err)
	}
	runID := started.GetRun().GetRunId()
	if runID == "" {
		t.Fatal("missing Run identity")
	}
	runRef := &pb.RunRef{Workspace: ref, RunId: runID}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		current, err := g.GetRun(cleanup, &pb.GetRunRequest{Run: runRef})
		if err != nil {
			t.Error("owned Run cleanup read", err)
			return
		}
		_, err = g.CloseRun(cleanup, &pb.CloseRunRequest{Run: runRef, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: carrier.Generation}, ExpectedStateRevision: current.GetRun().GetStateRevision()})
		if err != nil {
			t.Error("owned Run cleanup", err)
		}
	})
	replayed, err := g.StartRun(ctx, startRequest)
	if err != nil || !replayed.GetExactReplay() || replayed.GetRun().GetRunId() != runID {
		t.Fatal("published same-allocation replay failed", err)
	}
	reused := proto.Clone(startRequest).(*pb.StartRunRequest)
	reused.IdempotencyKey = uuid.NewString()
	if _, err = g.StartRun(ctx, reused); err == nil {
		t.Fatal("same credential dispatched another allocation")
	}
	verified, err := g.VerifyController(ctx, &pb.VerifyControllerRequest{Run: runRef, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: 1}})
	if err != nil || verified.GetController().GetInstanceId() != credential.InstanceID || verified.GetController().GetSubjectId() != "carrier-subject" {
		t.Fatal("published verification did not preserve principal", err)
	}
	bound, err := r.Sessions.BindPrimary(ctx, "carrier-subject", attachment.ID, runID, credential.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(carrier.AbsolutePath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(body)
	selectedKey := "adopted.json"
	selected := filepath.Join(filepath.Dir(carrier.AbsolutePath), selectedKey)
	if err = os.WriteFile(selected, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err = r.AdoptController(ctx, "carrier-subject", bound.ID, selectedKey); err != nil {
		t.Fatal("matching published Controller adoption", err)
	}
	adopted, err := db.Presentation().Binding(ctx, "carrier-subject", bound.ID)
	if err != nil || adopted.ControllerBindingID == bound.ControllerBindingID {
		t.Fatal("binding not atomically replaced", err)
	}
	carrier, err = carriers.Resolve(ctx, "carrier-subject", adopted.ControllerBindingID)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../adopted.json", selected, "missing.json"} {
		if err = r.AdoptController(ctx, "carrier-subject", bound.ID, bad); err == nil {
			t.Fatal("unsafe host selection admitted")
		}
	}
	foreign, err := carriers.Create(ctx, "carrier-subject", "carrier-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.AdoptController(ctx, "carrier-subject", bound.ID, foreign.CredentialKey); err == nil {
		t.Fatal("foreign Controller accepted")
	}
	foreignCarrier, err := carriers.Resolve(ctx, "carrier-subject", foreign.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.VerifyController(ctx, &pb.VerifyControllerRequest{Run: runRef, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: foreignCarrier.AbsolutePath, ExpectedControllerId: foreignCarrier.ControllerID, ExpectedControllerGeneration: 1}}); err == nil {
		t.Fatal("foreign Controller accepted by published provider")
	}
	wrongGeneration := &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: 2}
	if _, err = g.VerifyController(ctx, &pb.VerifyControllerRequest{Run: runRef, Controller: wrongGeneration}); err == nil {
		t.Fatal("wrong generation accepted by published provider")
	}
	var document map[string]any
	if json.Unmarshal(body, &document) != nil {
		t.Fatal("carrier decode")
	}
	wrongCapability := make([]byte, 32)
	defer clear(wrongCapability)
	if _, err = rand.Read(wrongCapability); err != nil {
		t.Fatal(err)
	}
	goodCapability := document["capability"]
	document["capability"] = base64.RawURLEncoding.EncodeToString(wrongCapability)
	invalidSecret, _ := json.Marshal(document)
	defer clear(invalidSecret)
	invalidPath := filepath.Join(filepath.Dir(carrier.AbsolutePath), "invalid-capability.json")
	if err = os.WriteFile(invalidPath, invalidSecret, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = g.VerifyController(ctx, &pb.VerifyControllerRequest{Run: runRef, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: invalidPath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: carrier.Generation}}); err == nil {
		t.Fatal("wrong capability accepted by published provider")
	}
	document["capability"] = goodCapability
	document["subject_id"] = "foreign-subject"
	changed, _ := json.Marshal(document)
	defer clear(changed)
	badPath := filepath.Join(filepath.Dir(carrier.AbsolutePath), "foreign-principal.json")
	if err = os.WriteFile(badPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err = r.AdoptController(ctx, "carrier-subject", bound.ID, "foreign-principal.json"); err == nil {
		t.Fatal("foreign principal accepted")
	}
	if err = os.Chmod(carrier.AbsolutePath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = g.VerifyController(ctx, &pb.VerifyControllerRequest{Run: runRef, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: 1}}); err == nil {
		t.Fatal("unsafe mode reached published provider")
	}
	if err = os.Chmod(carrier.AbsolutePath, 0600); err != nil {
		t.Fatal(err)
	}
	original := carrier.AbsolutePath + "-original"
	if err = os.Rename(carrier.AbsolutePath, original); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(carrier.AbsolutePath, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = g.SubmitTurn(ctx, &pb.SubmitTurnRequest{Run: runRef, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: 1}}); err == nil {
		t.Fatal("replacement was silently authorized")
	}
	if err = os.Remove(carrier.AbsolutePath); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(original, carrier.AbsolutePath); err != nil {
		t.Fatal(err)
	}
	if _, err = g.VerifyController(ctx, &pb.VerifyControllerRequest{Run: runRef, Controller: &pb.ControllerCarrierRef{AbsoluteFilePath: carrier.AbsolutePath, ExpectedControllerId: carrier.ControllerID, ExpectedControllerGeneration: 1}}); err == nil {
		t.Fatal("symlink reached published provider")
	}
	if err = os.Remove(carrier.AbsolutePath); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(original, carrier.AbsolutePath); err != nil {
		t.Fatal(err)
	}
}
