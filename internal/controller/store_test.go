package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/scenario"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/storage"
	"golang.org/x/sys/unix"
)

func fixture(t *testing.T) (*Store, *storage.Store, string, *pb.GetCapabilitiesResponse) {
	t.Helper()
	base, err := os.MkdirTemp("/private/tmp", "gul-carrier-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	home, data := filepath.Join(base, "home"), filepath.Join(base, "data")
	for _, dir := range []string{home, data} {
		if err = os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := storage.Open(t.Context(), filepath.Join(data, "gul.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Auth().CreateAccount(t.Context(), "account-1", time.Now()); err != nil {
		t.Fatal(err)
	}
	caps, err := scenario.New(time.Now()).GetCapabilities(t.Context(), &pb.GetCapabilitiesRequest{MinimumProtocolVersion: 1, MaximumProtocolVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(t.Context(), db, home, nil, caps)
	if err != nil {
		t.Fatal(err)
	}
	return s, db, base, caps
}

func TestCreateSchemaIdentityDurabilityAndExclusiveCarrier(t *testing.T) {
	s, db, base, caps := fixture(t)
	one, err := s.Create(t.Context(), "account-1", "review-policy")
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Create(t.Context(), "account-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if one.ExpectedControllerID == two.ExpectedControllerID || one.CredentialKey == two.CredentialKey || one.BindingID == two.BindingID || one.Generation != 1 {
		t.Fatal("credential identity reused")
	}
	path := filepath.Join(s.root, one.CredentialKey)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(body)
	var document map[string]any
	if json.Unmarshal(body, &document) != nil {
		t.Fatal("invalid emitted credential")
	}
	if len(document) != 7 || document["schema_version"] != float64(1) || document["kind"] != "interactive_client" || document["instance_id"] != s.installation || document["subject_id"] != "account-1" || !validUUID(document["controller_id"].(string)) {
		t.Fatal("wrong public schema/provenance")
	}
	secret, err := base64.RawURLEncoding.Strict().DecodeString(document["capability"].(string))
	if err != nil || len(secret) != 32 {
		t.Fatal("non-canonical capability")
	}
	defer clear(secret)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatal("exclusive collision did not refuse")
	}
	if _, err = s.Resolve(t.Context(), "account-1", one.BindingID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(t.Context(), "browser-subject", ""); !errors.Is(err, session.ErrCarrierUnavailable) {
		t.Fatal("caller chose principal")
	}
	if _, err = s.Create(t.Context(), "account-1", "../policy"); err == nil {
		t.Fatal("invalid launch policy")
	}
	id := s.installation
	fileName := filepath.Join(base, "data", "gul.sqlite")
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{fileName, fileName + "-wal"} {
		data, readErr := os.ReadFile(name)
		if readErr != nil {
			continue
		}
		if bytes.Contains(data, []byte(document["capability"].(string))) || bytes.Contains(data, []byte(s.root)) {
			t.Fatal("SQLite retained secret or absolute carrier path")
		}
	}
	reopened, err := storage.Open(t.Context(), fileName)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := New(t.Context(), reopened, s.home, nil, caps)
	if err != nil || restored.installation != id {
		t.Fatal("installation changed across reopen", err)
	}
	if _, err = restored.Resolve(t.Context(), "account-1", one.BindingID); err != nil {
		t.Fatal("persisted logical reference unavailable", err)
	}
}

func TestCarrierReplacementAndRPCIdentityFailClosed(t *testing.T) {
	for _, test := range []string{"mode", "symlink", "replacement", "principal", "kind", "unknown-field", "duplicate", "generation", "traversal", "wrong-subject", "hardlink", "directory", "fifo", "oversized", "null-launch", "missing-field"} {
		t.Run(test, func(t *testing.T) {
			s, _, _, _ := fixture(t)
			c, err := s.Create(t.Context(), "account-1", "")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.root, c.CredentialKey)
			ref := &pb.ControllerCarrierRef{AbsoluteFilePath: path, ExpectedControllerId: c.ExpectedControllerID, ExpectedControllerGeneration: 1}
			if err = s.ValidateRPC(t.Context(), "RunService.SubmitTurn", ref, nil); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(body)
			switch test {
			case "mode":
				err = os.Chmod(path, 0644)
			case "symlink":
				err = os.Rename(path, path+"-real")
				if err == nil {
					err = os.Symlink(path+"-real", path)
				}
			case "replacement":
				err = os.Rename(path, path+"-old")
				if err == nil {
					err = os.WriteFile(path, body, 0600)
				}
			case "hardlink":
				err = os.Link(path, path+"-alias")
			case "directory", "fifo":
				err = os.Remove(path)
				if err == nil && test == "directory" {
					err = os.Mkdir(path, 0600)
				} else if err == nil {
					err = unix.Mkfifo(path, 0600)
				}
			case "oversized":
				err = os.WriteFile(path, append(body, bytes.Repeat([]byte(" "), maximumCredentialBytes)...), 0600)
			case "generation":
				ref.ExpectedControllerGeneration = 2
			case "traversal":
				ref.AbsoluteFilePath = filepath.Join(s.root, "nested") + "/../" + c.CredentialKey
			case "wrong-subject":
				if _, err = s.Resolve(t.Context(), "other", c.BindingID); err == nil {
					t.Fatal("foreign subject resolved")
				}
				return
			default:
				var doc map[string]any
				if json.Unmarshal(body, &doc) != nil {
					t.Fatal("decode")
				}
				if test == "principal" {
					doc["instance_id"] = "foreign"
				}
				if test == "kind" {
					doc["kind"] = "automation"
				}
				if test == "unknown-field" {
					doc["generation"] = 1
				}
				if test == "null-launch" {
					doc["orchestration_launch"] = nil
				}
				if test == "missing-field" {
					delete(doc, "instance_id")
				}
				changed, _ := json.Marshal(doc)
				defer clear(changed)
				if test == "duplicate" {
					changed = append([]byte(`{"schema_version":1,`), changed[1:]...)
				}
				err = os.WriteFile(path, changed, 0600)
			}
			if err != nil {
				t.Fatal("fixture mutation", err)
			}
			if connect.CodeOf(s.ValidateRPC(t.Context(), "RunService.SubmitTurn", ref, nil)) != connect.CodeUnauthenticated {
				t.Fatal("changed carrier admitted")
			}
		})
	}
}

func TestCandidateSchemaRequiresExactPropertyNames(t *testing.T) {
	s, _, _, _ := fixture(t)
	c, err := s.Create(t.Context(), "account-1", "review-policy")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(s.root, c.CredentialKey))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(body)
	key := "candidate.json"
	path := filepath.Join(s.root, key)
	// An unregistered candidate has no stored file pin to mask parser failures.
	validate := func() error {
		_, _, err := s.Validate(t.Context(), "account-1", key, c.ExpectedControllerID, 1)
		return err
	}
	if err = os.WriteFile(path, body, 0600); err != nil || validate() != nil {
		t.Fatal("canonical unregistered candidate refused", err)
	}
	for _, nested := range []bool{false, true} {
		for _, aliasOnly := range []bool{false, true} {
			name := "root"
			if nested {
				name = "launch"
			}
			if aliasOnly {
				name += "/alias-only"
			} else {
				name += "/canonical-plus-alias"
			}
			t.Run(name, func(t *testing.T) {
				var doc map[string]any
				if err := json.Unmarshal(body, &doc); err != nil {
					t.Fatal(err)
				}
				fields, canonical, alias := doc, "capability", "Capability"
				if nested {
					fields = doc["orchestration_launch"].(map[string]any)
					canonical, alias = "use_case", "USE_CASE"
				}
				fields[alias] = fields[canonical]
				if aliasOnly {
					delete(fields, canonical)
				}
				changed, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				defer clear(changed)
				if err = os.WriteFile(path, changed, 0600); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(validate(), session.ErrCarrierUnavailable) {
					t.Fatal("non-schema property accepted")
				}
				ref := &pb.ControllerCarrierRef{AbsoluteFilePath: path, ExpectedControllerId: c.ExpectedControllerID, ExpectedControllerGeneration: 1}
				if connect.CodeOf(s.ValidateRPC(t.Context(), "ControllerService.VerifyController", ref, nil)) != connect.CodeUnauthenticated {
					t.Fatal("non-schema candidate passed RPC validation")
				}
			})
		}
	}
}

func TestWrongOwnerTypeAndUnsafeRoot(t *testing.T) {
	s, db, _, caps := fixture(t)
	c, err := s.Create(t.Context(), "account-1", "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.root, c.CredentialKey))
	if err != nil {
		t.Fatal(err)
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid++
	if _, err = identity(wrongOwner{info, &stat}); err == nil {
		t.Fatal("wrong uid admitted")
	}
	if _, err = New(t.Context(), db, s.home, []string{s.home}, caps); err == nil {
		t.Fatal("workspace-visible store admitted")
	}
	parent := filepath.Dir(s.root)
	if err = os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Resolve(t.Context(), "account-1", c.BindingID); err == nil {
		t.Fatal("unsafe parent admitted")
	}
	if err = os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(s.root, s.root+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(s.root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(t.Context(), "account-1", ""); err == nil {
		t.Fatal("replaced root admitted")
	}
}

type wrongOwner struct {
	os.FileInfo
	stat *syscall.Stat_t
}

func (w wrongOwner) Sys() any { return w.stat }

func TestUnusedRemovalAndAllocationReservation(t *testing.T) {
	s, _, _, _ := fixture(t)
	c, err := s.Create(t.Context(), "account-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RemoveUnused(t.Context(), "account-1", c.BindingID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.root, c.CredentialKey)); !os.IsNotExist(err) {
		t.Fatal("unused carrier retained")
	}
	c, err = s.Create(t.Context(), "account-1", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := &pb.ControllerCarrierRef{AbsoluteFilePath: filepath.Join(s.root, c.CredentialKey), ExpectedControllerId: c.ExpectedControllerID, ExpectedControllerGeneration: 1}
	if err = s.ValidateRPC(t.Context(), "RunService.StartRun", ref, &pb.StartRunRequest{IdempotencyKey: "allocation", Workspace: &pb.WorkspaceRef{ExpectedWorkspaceId: "workspace"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.ValidateRPC(t.Context(), "RunService.StartRun", ref, &pb.StartRunRequest{IdempotencyKey: "allocation", Workspace: &pb.WorkspaceRef{ExpectedWorkspaceId: "workspace"}}); err != nil {
		t.Fatal("same allocation replay refused", err)
	}
	for _, request := range []*pb.StartRunRequest{
		{IdempotencyKey: "another-allocation", Workspace: &pb.WorkspaceRef{ExpectedWorkspaceId: "workspace"}},
		{IdempotencyKey: "allocation", Workspace: &pb.WorkspaceRef{ExpectedWorkspaceId: "another-workspace"}},
	} {
		if err = s.ValidateRPC(t.Context(), "RunService.StartRun", ref, request); err == nil {
			t.Fatal("credential reused for another allocation")
		}
	}
	if err = s.RemoveUnused(t.Context(), "account-1", c.BindingID); err == nil {
		t.Fatal("potential allocation carrier removed")
	}
	if _, err = os.Stat(ref.AbsoluteFilePath); err != nil {
		t.Fatal("allocated credential disappeared")
	}
	if _, _, err = s.Validate(context.Background(), "account-1", "../secret", uuid.NewString(), 1); err == nil {
		t.Fatal("invalid local reference")
	}
}

func TestRPCRefusesConcurrentReplacementAndSymlinkRace(t *testing.T) {
	s, _, _, _ := fixture(t)
	c, err := s.Create(t.Context(), "account-1", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.root, c.CredentialKey)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(body)
	original := path + "-original"
	if err = os.Rename(path, original); err != nil {
		t.Fatal(err)
	}
	stop, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		candidate := path + "-candidate"
		for i := 0; ; i++ {
			select {
			case <-stop:
				finished <- nil
				return
			default:
			}
			var err error
			if i%2 == 0 {
				err = os.WriteFile(candidate, body, 0600)
			} else {
				err = os.Symlink(original, candidate)
			}
			if err == nil {
				err = os.Rename(candidate, path)
			}
			if err != nil {
				finished <- err
				return
			}
		}
	}()
	defer func() {
		close(stop)
		if err := <-finished; err != nil {
			t.Error("replacement fixture", err)
		}
	}()
	ref := &pb.ControllerCarrierRef{AbsoluteFilePath: path, ExpectedControllerId: c.ExpectedControllerID, ExpectedControllerGeneration: 1}
	for i := 0; i < 100; i++ {
		if err := s.ValidateRPC(t.Context(), "RunService.SubmitTurn", ref, nil); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatal("racing replacement reached authorization", err)
		}
	}
}

func TestWorkspaceAliasCannotExposeProtectedStore(t *testing.T) {
	s, db, base, caps := fixture(t)
	alias := filepath.Join(base, "workspace-alias")
	if err := os.Symlink(s.root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := New(t.Context(), db, s.home, []string{alias}, caps); err == nil {
		t.Fatal("workspace symlink alias exposed store")
	}
	caseAlias := strings.Replace(s.home, "home", "HOME", 1)
	if _, err := os.Stat(caseAlias); os.IsNotExist(err) {
		t.Skip("volume has case-sensitive paths; symlink alias checked")
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := New(t.Context(), db, s.home, []string{caseAlias}, caps); err == nil {
		t.Fatal("case alias of HOME exposed store")
	}
}
