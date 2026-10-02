// Package controller owns Gul's local protected credential files. Provider
// verification remains authoritative; local validation never grants Run access.
package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	contract "github.com/rootkernel/gul/contract"
	pb "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/contract/port"
	"github.com/rootkernel/gul/internal/session"
	"github.com/rootkernel/gul/internal/storage"
	"golang.org/x/sys/unix"
)

const maximumCredentialBytes = 4096

var logicalKey = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var policyName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type Store struct {
	db                       *storage.Store
	home, root, installation string
	roots                    []string
	rootIdentity             os.FileInfo
}

type credential struct {
	SchemaVersion int           `json:"schema_version"`
	ControllerID  string        `json:"controller_id"`
	Kind          string        `json:"kind"`
	InstanceID    string        `json:"instance_id"`
	SubjectID     string        `json:"subject_id"`
	Capability    string        `json:"capability"`
	Launch        *launchIntent `json:"orchestration_launch,omitempty"`
}
type launchIntent struct {
	UseCase string `json:"use_case"`
	Policy  string `json:"specialist_policy_name"`
}

// New derives the fixed provider-home root from trusted host configuration and
// a persistent local installation ID. It refuses a workspace-visible store.
func New(ctx context.Context, db *storage.Store, home string, roots []string, caps *pb.GetCapabilitiesResponse) (*Store, error) {
	c := caps.GetControllerCarrier()
	if db == nil || c.GetSchemaVersion() != 1 || c.GetSchemaId() != contract.Admission().CredentialSchemaID || c.GetSchemaSha256() != port.ControllerCredentialSchemaSHA256 || c.GetCarrierRootPolicy() != pb.ControllerCarrierRootPolicy_CONTROLLER_CARRIER_ROOT_POLICY_DOLGORAE_OWNED_HOME ||
		c.GetCarrierRootLocator() != "home/.dolgorae/controller-carriers" || c.GetClientDescendantPattern() != "<client>/<installation-id>/" || !c.GetSameUidRequired() || !c.GetRegularFileRequired() || !c.GetSymlinksForbidden() || !c.GetCreateExclusiveRequired() ||
		c.GetCapabilityByteLength() != 32 || c.GetCapabilityEncoding() != pb.ControllerCapabilityEncoding_CONTROLLER_CAPABILITY_ENCODING_BASE64URL_NO_PADDING || c.GetNormalizedPrincipalRule() != pb.ControllerPrincipalRule_CONTROLLER_PRINCIPAL_RULE_KIND_SUBJECT_ID_ELSE_KIND_INSTANCE_ID || c.GetParentDirectoryMode() != 0700 || c.GetCredentialFileMode() != 0600 || c.GetMaximumFileBytes() != maximumCredentialBytes || c.GetInitialGeneration() != 1 ||
		!slices.Contains(c.GetAcceptedControllerKinds(), pb.ControllerKind_CONTROLLER_KIND_INTERACTIVE_CLIENT) {
		return nil, session.ErrCarrierUnavailable
	}
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, session.ErrCarrierUnavailable
		}
	}
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return nil, session.ErrCarrierUnavailable
	}
	id, err := db.Presentation().InstallationID(ctx)
	if err != nil {
		return nil, session.ErrCarrierUnavailable
	}
	s := &Store{db: db, home: home, root: filepath.Join(home, ".dolgorae", "controller-carriers", "gul", id), installation: id, roots: append([]string(nil), roots...)}
	if err = s.outsideWorkspaces(ctx); err != nil {
		return nil, err
	}
	f, err := s.openRoot(true)
	if err != nil {
		return nil, session.ErrCarrierUnavailable
	}
	defer f.Close()
	s.rootIdentity, err = f.Stat()
	if err != nil {
		return nil, session.ErrCarrierUnavailable
	}
	return s, nil
}

func (s *Store) outsideWorkspaces(ctx context.Context) error {
	roots, err := s.db.Presentation().WorkspaceRoots(ctx)
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	for _, root := range append(roots, s.roots...) {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return session.ErrCarrierUnavailable
		}
		for _, pair := range [][2]string{{root, s.root}, {s.root, root}} {
			rel, err := filepath.Rel(pair[0], pair[1])
			if err != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
				return session.ErrCarrierUnavailable
			}
			// File identity also detects case aliases on macOS volumes. Resolve
			// workspace aliases, then compare the selected directory to ancestors.
			info, statErr := os.Stat(pair[0])
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil || !info.IsDir() {
				return session.ErrCarrierUnavailable
			}
			path := pair[1]
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				path = resolved
			} else if !os.IsNotExist(err) {
				return session.ErrCarrierUnavailable
			}
			for {
				ancestor, err := os.Stat(path)
				if err == nil && os.SameFile(info, ancestor) || err != nil && !os.IsNotExist(err) {
					return session.ErrCarrierUnavailable
				}
				parent := filepath.Dir(path)
				if path == parent {
					break
				}
				path = parent
			}
		}
	}
	return nil
}

// openRoot walks each component with directory descriptors and O_NOFOLLOW.
// Creation is restricted to the Dolgorae subtree; existing modes are never fixed.
func (s *Store) openRoot(create bool) (*os.File, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, session.ErrCarrierUnavailable
	}
	current := ""
	for _, part := range strings.Split(strings.TrimPrefix(s.root, "/"), "/") {
		current += "/" + part
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e == unix.ENOENT && create && strings.HasPrefix(current, s.home+"/") {
			e = unix.Mkdirat(fd, part, 0700)
			if e == nil || e == unix.EEXIST {
				if e = unix.Fsync(fd); e == nil {
					next, e = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				}
			}
		}
		unix.Close(fd)
		if e != nil {
			return nil, session.ErrCarrierUnavailable
		}
		fd = next
		var st unix.Stat_t
		if e = unix.Fstat(fd, &st); e != nil || (current == s.home && (st.Uid != uint32(os.Getuid()) || st.Mode&0022 != 0)) || (strings.HasPrefix(current, s.home+"/") && (st.Uid != uint32(os.Getuid()) || st.Mode&07777 != 0700)) {
			unix.Close(fd)
			return nil, session.ErrCarrierUnavailable
		}
	}
	f := os.NewFile(uintptr(fd), "controller-root")
	info, err := f.Stat()
	if err != nil || s.rootIdentity != nil && !os.SameFile(s.rootIdentity, info) {
		f.Close()
		return nil, session.ErrCarrierUnavailable
	}
	return f, nil
}

func identity(info os.FileInfo) (storage.FileIdentity, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Getuid()) || st.Nlink != 1 || !info.Mode().IsRegular() || st.Mode&07777 != 0600 {
		return storage.FileIdentity{}, session.ErrCarrierUnavailable
	}
	return storage.FileIdentity{Device: uint64(st.Dev), Inode: st.Ino, Size: info.Size(), Modified: info.ModTime().UnixNano()}, nil
}
func validText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func validUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

// uniqueJSON rejects duplicate properties, including inside launch intent.
func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 4 {
		return session.ErrCarrierUnavailable
	}
	token, err := d.Token()
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' {
		return session.ErrCarrierUnavailable
	}
	keys := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || keys[key] {
			return session.ErrCarrierUnavailable
		}
		keys[key] = true
		if err = uniqueJSON(d, depth+1); err != nil {
			return err
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') {
		return session.ErrCarrierUnavailable
	}
	return nil
}
func decode(body []byte, subject, instance, expected string) (credential, error) {
	var c credential
	if !utf8.Valid(body) {
		return c, session.ErrCarrierUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if uniqueJSON(d, 0) != nil {
		return c, session.ErrCarrierUnavailable
	}
	if _, err := d.Token(); err != io.EOF {
		return c, session.ErrCarrierUnavailable
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return c, session.ErrCarrierUnavailable
	}
	for _, key := range []string{"schema_version", "controller_id", "kind", "instance_id", "subject_id", "capability"} {
		if _, ok := fields[key]; !ok {
			return c, session.ErrCarrierUnavailable
		}
	}
	for key := range fields {
		switch key {
		case "schema_version", "controller_id", "kind", "instance_id", "subject_id", "capability", "orchestration_launch":
		default:
			return c, session.ErrCarrierUnavailable
		}
	}
	if value, ok := fields["orchestration_launch"]; ok {
		var launch map[string]json.RawMessage
		if json.Unmarshal(value, &launch) != nil || len(launch) != 2 || launch["use_case"] == nil || launch["specialist_policy_name"] == nil {
			return c, session.ErrCarrierUnavailable
		}
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return credential{}, session.ErrCarrierUnavailable
	}
	if c.SchemaVersion != 1 || !validUUID(c.ControllerID) || c.ControllerID != expected || c.Kind != "interactive_client" || c.InstanceID != instance || c.SubjectID != subject || !validText(c.InstanceID, 128) || !validText(c.SubjectID, 256) || len(c.Capability) != 43 {
		return credential{}, session.ErrCarrierUnavailable
	}
	capability, err := base64.RawURLEncoding.Strict().DecodeString(c.Capability)
	defer clear(capability)
	if err != nil || len(capability) != 32 {
		return credential{}, session.ErrCarrierUnavailable
	}
	if c.Launch != nil && (c.Launch.UseCase != "dolgorae_orchestrated_session" || !policyName.MatchString(c.Launch.Policy)) {
		return credential{}, session.ErrCarrierUnavailable
	}
	return c, nil
}

func (s *Store) trustedSubject(ctx context.Context, subject string) error {
	actual, err := s.db.Presentation().AccountSubject(ctx)
	if err != nil || actual != subject || !validText(subject, 256) {
		return session.ErrCarrierUnavailable
	}
	return nil
}

// Validate returns only public identity and a derived backend reference. The
// secret decode and byte buffers remain bounded to this call.
func (s *Store) Validate(ctx context.Context, subject, key, controller string, generation uint64) (storage.ControllerCredential, session.Carrier, error) {
	if s == nil || s.trustedSubject(ctx, subject) != nil || !logicalKey.MatchString(key) || !validUUID(controller) || generation == 0 || s.outsideWorkspaces(ctx) != nil {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	root, err := s.openRoot(false)
	if err != nil {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	defer root.Close()
	fd, err := unix.Openat(int(root.Fd()), key, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	f := os.NewFile(uintptr(fd), "controller-carrier")
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	before, err := identity(info)
	if err != nil || before.Size <= 0 || before.Size > maximumCredentialBytes {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(f, maximumCredentialBytes+1))
	defer clear(body)
	if err != nil || len(body) > maximumCredentialBytes {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	c, err := decode(body, subject, s.installation, controller)
	if err != nil {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	c.Capability = "" // Never return or retain credential capability material.
	info, err = f.Stat()
	if err != nil {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	after, err := identity(info)
	if err != nil || after != before {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	var leaf unix.Stat_t
	if unix.Fstatat(int(root.Fd()), key, &leaf, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(leaf.Dev) != before.Device || leaf.Ino != before.Inode || leaf.Mode&unix.S_IFMT != unix.S_IFREG || leaf.Uid != uint32(os.Getuid()) || leaf.Mode&0777 != 0600 {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	check, err := s.openRoot(false)
	if err != nil {
		return storage.ControllerCredential{}, session.Carrier{}, session.ErrCarrierUnavailable
	}
	check.Close()
	meta := storage.ControllerCredential{BindingReference: storage.BindingReference{SubjectID: subject, CredentialKey: key, ExpectedControllerID: controller, Health: "healthy"}, Generation: generation, InstanceID: s.installation, FileIdentity: before}
	return meta, session.Carrier{AbsolutePath: filepath.Join(s.root, key), ControllerID: controller, Generation: generation}, nil
}

func (s *Store) Create(ctx context.Context, subject, policy string) (storage.ControllerCredential, error) {
	if s == nil || s.trustedSubject(ctx, subject) != nil || (policy != "" && !policyName.MatchString(policy)) || s.outsideWorkspaces(ctx) != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	id, err := uuid.NewV7()
	if err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	capability := make([]byte, 32)
	defer clear(capability)
	if _, err = rand.Read(capability); err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	c := credential{SchemaVersion: 1, ControllerID: id.String(), Kind: "interactive_client", InstanceID: s.installation, SubjectID: subject, Capability: base64.RawURLEncoding.EncodeToString(capability)}
	if policy != "" {
		c.Launch = &launchIntent{UseCase: "dolgorae_orchestrated_session", Policy: policy}
	}
	body, err := json.Marshal(c)
	c.Capability = ""
	defer clear(body)
	if err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	root, err := s.openRoot(false)
	if err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	defer root.Close()
	key := id.String() + ".json"
	fd, err := unix.Openat(int(root.Fd()), key, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	f := os.NewFile(uintptr(fd), "new-controller-carrier")
	written, err := f.Write(body)
	if err == nil && written != len(body) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = root.Sync()
	}
	if err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	meta, _, err := s.Validate(ctx, subject, key, id.String(), 1)
	if err != nil {
		return storage.ControllerCredential{}, err
	}
	binding, err := uuid.NewV7()
	if err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	meta.BindingID = binding.String()
	meta.Health = "unknown"
	if err = s.db.Presentation().RegisterCredential(ctx, meta); err != nil {
		return storage.ControllerCredential{}, session.ErrCarrierUnavailable
	}
	return meta, nil
}

func (s *Store) ResolveCarrierReference(ctx context.Context, subject, binding string) (session.Carrier, error) {
	if s == nil {
		return session.Carrier{}, session.ErrCarrierUnavailable
	}
	stored, err := s.db.Presentation().ControllerCredential(ctx, subject, binding)
	if err != nil {
		return session.Carrier{}, session.ErrCarrierUnavailable
	}
	meta, carrier, err := s.Validate(ctx, subject, stored.CredentialKey, stored.ExpectedControllerID, stored.Generation)
	if err != nil || stored.Removing || stored.InstanceID != meta.InstanceID || stored.FileIdentity != meta.FileIdentity {
		return session.Carrier{}, session.ErrCarrierUnavailable
	}
	return carrier, nil
}
func (s *Store) Resolve(ctx context.Context, subject, binding string) (session.Carrier, error) {
	return s.ResolveCarrierReference(ctx, subject, binding)
}

// ValidateRPC runs after transport capacity is acquired, immediately before
// each protected request. Only side-effect-free VerifyController may validate a
// host-selected candidate that has not yet been installed as a local binding.
func (s *Store) ValidateRPC(ctx context.Context, method string, ref *pb.ControllerCarrierRef, allocation *pb.StartRunRequest) error {
	reject := func() error { return connect.NewError(connect.CodeUnauthenticated, session.ErrCarrierUnavailable) }
	if s == nil || ref == nil || ref.AbsoluteFilePath != filepath.Clean(ref.AbsoluteFilePath) || filepath.Dir(ref.AbsoluteFilePath) != s.root {
		return reject()
	}
	subject, err := s.db.Presentation().AccountSubject(ctx)
	if err != nil {
		return reject()
	}
	key := filepath.Base(ref.AbsoluteFilePath)
	stored, err := s.db.Presentation().CredentialByKey(ctx, subject, key)
	if method == "ControllerService.VerifyController" {
		_, _, err = s.Validate(ctx, subject, key, ref.ExpectedControllerId, ref.ExpectedControllerGeneration)
		if err != nil {
			return reject()
		}
		return nil
	}
	if err != nil {
		return reject()
	}
	if stored.ExpectedControllerID != ref.ExpectedControllerId || stored.Generation != ref.ExpectedControllerGeneration {
		return reject()
	}
	_, err = s.ResolveCarrierReference(ctx, subject, stored.BindingID)
	if err != nil {
		return reject()
	}
	if method == "RunService.StartRun" && s.db.Presentation().MarkCredentialAllocated(ctx, subject, stored.BindingID, allocation.GetIdempotencyKey(), allocation.GetWorkspace().GetExpectedWorkspaceId()) != nil {
		return reject()
	}
	return nil
}

func (s *Store) RemoveUnused(ctx context.Context, subject, binding string) error {
	stored, err := s.db.Presentation().ControllerCredential(ctx, subject, binding)
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	if _, err = s.ResolveCarrierReference(ctx, subject, binding); err != nil {
		return err
	}
	if err = s.db.Presentation().ReserveCredentialRemoval(ctx, subject, binding); err != nil {
		return session.ErrCarrierUnavailable
	}
	root, err := s.openRoot(false)
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	defer root.Close()
	// Move the selected entry to an unpredictable private name before inspecting
	// it. A replaced leaf is preserved; only the pinned regular file is unlinked.
	id, err := uuid.NewV7()
	if err != nil {
		return session.ErrCarrierUnavailable
	}
	quarantine := ".remove-" + id.String()
	fd := int(root.Fd())
	if err = unix.Renameat(fd, stored.CredentialKey, fd, quarantine); err != nil {
		return session.ErrCarrierUnavailable
	}
	selected, err := unix.Openat(fd, quarantine, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	var pinned storage.FileIdentity
	if err == nil {
		file := os.NewFile(uintptr(selected), "removing-controller-carrier")
		info, statErr := file.Stat()
		if statErr == nil {
			pinned, statErr = identity(info)
		}
		err = statErr
		file.Close()
	}
	if err != nil || pinned != stored.FileIdentity {
		// Linkat without AT_SYMLINK_FOLLOW restores the entry only if its old
		// name is still absent. Never overwrite a concurrent replacement.
		if unix.Linkat(fd, quarantine, fd, stored.CredentialKey, 0) == nil {
			_ = unix.Unlinkat(fd, quarantine, 0)
		}
		_ = root.Sync()
		return session.ErrCarrierUnavailable
	}
	if err = unix.Unlinkat(fd, quarantine, 0); err != nil {
		return session.ErrCarrierUnavailable
	}
	if err = root.Sync(); err != nil {
		return session.ErrCarrierUnavailable
	}
	if err = s.db.Presentation().RemoveCredentialReference(ctx, subject, binding); err != nil {
		return session.ErrCarrierUnavailable
	}
	return nil
}

var _ session.CarrierResolver = (*Store)(nil)
