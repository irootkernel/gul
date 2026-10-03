package provider_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1/dolgoraev1connect"
)

// This opt-in release gate never discovers an installed runtime. Its executable
// must match the published artifact; the source build is a separate supplement.
func TestPublishedReleaseHandshake(t *testing.T) {
	binary := os.Getenv("GUL_E2_DOLGORAE_EXECUTABLE")
	if binary == "" {
		t.Skip("set GUL_E2_DOLGORAE_EXECUTABLE to the pinned published artifact")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("the pinned published executable requires darwin/arm64")
	}
	var lock struct {
		SchemaVersion int `json:"schema_version"`
		Files         []struct{ Path, SHA256 string }
		Release       struct {
			Version      string
			Archive      struct{ SHA256 string }
			Executable   struct{ SHA256 string }
			Capabilities struct {
				Features   map[string]bool
				Methods    []string `json:"grpc_methods"`
				Credential struct {
					SchemaID     string `json:"schema_id"`
					SchemaSHA256 string `json:"schema_sha256"`
				} `json:"controller_credential"`
			} `json:"advertised_capabilities"`
		}
	}
	bytes, err := os.ReadFile("../../contract/dependency-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.SchemaVersion != 1 {
		t.Fatal("unsupported dependency-lock schema version")
	}
	archive := os.Getenv("GUL_E2_DOLGORAE_ARCHIVE")
	if archive == "" {
		t.Fatal("set GUL_E2_DOLGORAE_ARCHIVE to the pinned published archive")
	}
	root, err := os.MkdirTemp(providerTemporaryBase(), "gul-v013-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	binary, err = qualifyReleaseArtifacts(archive, binary, lock.Release.Archive.SHA256, lock.Release.Executable.SHA256, root)
	if err != nil {
		t.Fatal(err)
	}
	home, work, socketParent := filepath.Join(root, "home"), filepath.Join(root, "work"), filepath.Join(root, "rpc")
	for _, dir := range []string{home, work, socketParent} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	childEnv := []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TMPDIR=" + root}
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer initCancel()
	initCmd := exec.CommandContext(initCtx, binary, "init", work, "--non-git")
	initCmd.Env, initCmd.Dir, initCmd.Stderr = childEnv, work, io.Discard
	if output, err := initCmd.Output(); err != nil {
		t.Fatalf("isolated provider initialization failed: %v (%s)", err, machineFailureCode(output))
	}
	versionCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	versionCmd := exec.CommandContext(versionCtx, binary, "version", "--json")
	versionCmd.Env, versionCmd.Dir = childEnv, work
	versionBytes, err := versionCmd.Output()
	if err != nil {
		t.Fatalf("published executable version failed: %v (%s)", err, machineFailureCode(versionBytes))
	}
	var version struct{ Version string }
	if json.Unmarshal(versionBytes, &version) != nil || strings.TrimPrefix(version.Version, "v") != lock.Release.Version {
		t.Fatal("published executable version mismatch")
	}
	socket := filepath.Join(socketParent, "g.sock")
	cmd := exec.Command(binary, "serve", "--socket", socket)
	cmd.Env, cmd.Dir, cmd.Stderr = childEnv, work, io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal("cannot start isolated release gateway")
	}
	done := make(chan error, 1)
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			if err != nil {
				t.Error("isolated gateway did not shut down cleanly")
			}
			if _, err := os.Lstat(socket); !os.IsNotExist(err) {
				t.Error("gateway did not clean up its own socket")
			}
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("isolated gateway exceeded shutdown deadline")
		}
	})
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		status := "INVALID_READINESS"
		if scanner.Scan() {
			var envelope struct {
				OK      bool   `json:"ok"`
				Command string `json:"command"`
			}
			if json.Unmarshal(scanner.Bytes(), &envelope) == nil && envelope.OK && envelope.Command == "serve" {
				status = ""
			} else {
				status = machineFailureCode(scanner.Bytes())
			}
		}
		ready <- status
		for scanner.Scan() {
		}
		done <- cmd.Wait()
	}()
	select {
	case status := <-ready:
		if status != "" {
			t.Fatalf("isolated gateway rejected readiness: %s", status)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("isolated gateway exceeded startup deadline")
	}
	info, err := os.Lstat(socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 {
		t.Fatal("gateway socket is not private")
	}
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := dolgoraev1connect.NewRuntimeServiceClient(&http.Client{Transport: transport}, "http://localhost", connect.WithGRPC())
	instance := uuid.NewString()
	requestContext := func(protocol uint32) *publicv1.RequestContext {
		return &publicv1.RequestContext{ProtocolVersion: protocol, ClientRequestId: uuid.NewString(), ClientInstanceId: instance}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := client.GetCapabilities(ctx, connect.NewRequest(&publicv1.GetCapabilitiesRequest{
		Context: requestContext(0), MinimumProtocolVersion: 1, MaximumProtocolVersion: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	caps := response.Msg
	if strings.TrimPrefix(caps.DolgoraeVersion, "v") != lock.Release.Version || caps.GetContext().GetProtocolVersion() != 1 ||
		caps.GetProtocol().GetMinimumClientProtocolVersion() != 1 || caps.GetProtocol().GetMaximumClientProtocolVersion() != 1 {
		t.Fatal("published gRPC version negotiation mismatch")
	}
	var descriptorDigest string
	for _, file := range lock.Files {
		if file.Path == "upstream/dolgorae-public-v1.descriptor.pb" {
			descriptorDigest = file.SHA256
		}
	}
	if descriptorDigest == "" || descriptorDigest != caps.DescriptorSha256 {
		t.Fatal("published descriptor missing or mismatched")
	}
	carrier := caps.GetControllerCarrier()
	if carrier.GetSchemaId() != lock.Release.Capabilities.Credential.SchemaID || carrier.GetSchemaVersion() != 1 ||
		carrier.GetSchemaSha256() != lock.Release.Capabilities.Credential.SchemaSHA256 {
		t.Fatal("published credential schema mismatch")
	}
	for _, method := range lock.Release.Capabilities.Methods {
		if !slices.Contains(caps.SupportedMethods, method) {
			t.Fatalf("missing required method %s", method)
		}
	}
	features := caps.GetFeatures().ProtoReflect()
	fields := features.Descriptor().Fields()
	if len(lock.Release.Capabilities.Features) != fields.Len() {
		t.Fatal("published feature inventory mismatch")
	}
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		expected, exists := lock.Release.Capabilities.Features[string(field.Name())]
		if !exists || features.Get(field).Bool() != expected {
			t.Fatalf("published feature mismatch: %s", field.Name())
		}
	}
	lanes := map[publicv1.ExecutionLane]bool{}
	for _, lane := range caps.GetLanes().GetItems() {
		if _, duplicate := lanes[lane.Lane]; duplicate {
			t.Fatal("duplicate published lane")
		}
		lanes[lane.Lane] = lane.WriterSupport
	}
	if !lanes[publicv1.ExecutionLane_EXECUTION_LANE_DEDICATED] || lanes[publicv1.ExecutionLane_EXECUTION_LANE_SHARED_READONLY] || len(lanes) != 2 {
		t.Fatal("published lane writer support mismatch")
	}
	if caps.AccessPolicyTransition != publicv1.SupportState_SUPPORT_STATE_UNVERIFIED {
		t.Fatal("unexpected reader-to-writer transition admission")
	}
	if _, err := client.ListProfiles(ctx, connect.NewRequest(&publicv1.ListProfilesRequest{Context: requestContext(1)})); err != nil {
		t.Fatal(err)
	}
	_, err = client.GetCapabilities(ctx, connect.NewRequest(&publicv1.GetCapabilitiesRequest{
		Context: requestContext(0), MinimumProtocolVersion: 2, MaximumProtocolVersion: 2,
	}))
	if err == nil {
		t.Fatal("unsupported protocol was admitted")
	}
	var typedCode string
	if connectErr, ok := err.(*connect.Error); ok {
		for _, detail := range connectErr.Details() {
			value, detailErr := detail.Value()
			if failure, ok := value.(*publicv1.DolgoraeErrorDetail); detailErr == nil && ok {
				typedCode = failure.DolgoraeErrorCode
			}
		}
	}
	if typedCode != "PROTOCOL_VERSION_UNSUPPORTED" {
		t.Fatal("unsupported protocol did not return the pinned typed refusal")
	}
}

func qualifyReleaseArtifacts(archive, binary, archiveDigest, executableDigest, root string) (string, error) {
	archiveFile, err := os.Open(archive)
	if err != nil {
		return "", errors.New("cannot open explicitly selected archive")
	}
	archiveHash := sha256.New()
	_, readErr := io.Copy(archiveHash, archiveFile)
	closeErr := archiveFile.Close()
	if readErr != nil || closeErr != nil {
		return "", errors.New("cannot read or close explicitly selected archive")
	}
	if got := hex.EncodeToString(archiveHash.Sum(nil)); got != archiveDigest {
		return "", fmt.Errorf("published archive digest mismatch: got %s, want %s", got, archiveDigest)
	}
	f, err := os.Open(binary)
	if err != nil {
		return "", errors.New("cannot open explicitly selected executable")
	}
	copyPath := filepath.Join(root, "dolgorae")
	copyFile, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		f.Close()
		return "", errors.New("cannot create private executable copy")
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(copyFile, hash), f)
	closeErr = copyFile.Close()
	f.Close()
	if copyErr != nil || closeErr != nil {
		return "", errors.New("cannot copy or close explicitly selected executable")
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != executableDigest {
		return "", fmt.Errorf("published executable digest mismatch: got %s, want %s", got, executableDigest)
	}
	return copyPath, nil
}

func TestReleaseArtifactRefusal(t *testing.T) {
	archive, binary := []byte("published archive fixture"), []byte("published executable fixture")
	archiveDigest, executableDigest := sha256.Sum256(archive), sha256.Sum256(binary)
	for name, fixture := range map[string]struct {
		archive, binary []byte
		failure         string
	}{
		"verified":               {archive, binary, ""},
		"wrong archive":          {[]byte("wrong archive"), binary, "archive digest mismatch"},
		"truncated archive":      {archive[:3], binary, "archive digest mismatch"},
		"wrong executable":       {archive, []byte("source-built executable"), "executable digest mismatch"},
		"truncated executable":   {archive, binary[:3], "executable digest mismatch"},
		"archive I/O failure":    {nil, binary, "cannot read or close explicitly selected archive"},
		"executable I/O failure": {archive, nil, "cannot copy or close explicitly selected executable"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			archivePath, binaryPath, copyRoot := filepath.Join(root, "archive"), filepath.Join(root, "selected"), filepath.Join(root, "private")
			for path, content := range map[string][]byte{archivePath: fixture.archive, binaryPath: fixture.binary} {
				var err error
				if content == nil {
					err = os.Mkdir(path, 0700)
				} else {
					err = os.WriteFile(path, content, 0700)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(copyRoot, 0700); err != nil {
				t.Fatal(err)
			}
			qualified, err := qualifyReleaseArtifacts(archivePath, binaryPath, hex.EncodeToString(archiveDigest[:]), hex.EncodeToString(executableDigest[:]), copyRoot)
			if fixture.failure != "" {
				if err == nil || !strings.Contains(err.Error(), fixture.failure) || qualified != "" {
					t.Fatalf("refusal returned executable %q and error %v", qualified, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			copied, err := os.ReadFile(qualified)
			if err != nil || string(copied) != string(binary) {
				t.Fatal("verified executable copy differs from selected bytes")
			}
			info, err := os.Stat(qualified)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatal("verified executable copy is not private")
			}
		})
	}
}

// Only the machine error code is safe to print; messages and details can carry
// private filesystem or credential information.
func machineFailureCode(output []byte) string {
	var envelope struct{ Error struct{ Code string } }
	if json.Unmarshal(output, &envelope) == nil && len(envelope.Error.Code) > 0 && len(envelope.Error.Code) <= 128 &&
		strings.Trim(envelope.Error.Code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789") == "" {
		return envelope.Error.Code
	}
	return "INVALID_MACHINE_RESPONSE"
}
