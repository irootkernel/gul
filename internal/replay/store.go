// Package replay retains the bounded, non-secret StartRun request needed for
// exact application replay after a lost allocation response.
package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

const MaximumAge = 72 * time.Hour
const MaximumBytes = 64 << 10

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

var ErrInvalid = errors.New("invalid replay material")
var ErrUnavailable = errors.New("replay material unavailable")

// StartRun contains semantic request fields only. Workspace and Controller
// paths are resolved afresh from these logical references before each RPC.
type StartRun struct {
	OperationID          string    `json:"operation_id"`
	SubjectID            string    `json:"subject_id"`
	WorkspaceID          string    `json:"workspace_id"`
	ProviderWorkspaceID  string    `json:"provider_workspace_id"`
	CredentialKey        string    `json:"credential_key"`
	ControllerID         string    `json:"controller_id"`
	ControllerGeneration uint64    `json:"controller_generation"`
	IdempotencyKey       string    `json:"idempotency_key"`
	ProfileName          string    `json:"profile_name"`
	ControlMode          int32     `json:"control_mode"`
	ExecutionLane        int32     `json:"execution_lane"`
	Purpose              int32     `json:"purpose"`
	PurposeLabel         *string   `json:"purpose_label,omitempty"`
	Model                *string   `json:"model,omitempty"`
	Effort               *string   `json:"effort,omitempty"`
	RequiredAssurance    int32     `json:"required_assurance"`
	RequiredCapabilities []string  `json:"required_capabilities"`
	Instructions         *string   `json:"instructions,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

func (s StartRun) valid() bool {
	return keyPattern.MatchString(s.OperationID) && s.SubjectID != "" && s.WorkspaceID != "" && s.ProviderWorkspaceID != "" &&
		fs.ValidPath(s.CredentialKey) && path.Clean(s.CredentialKey) == s.CredentialKey && s.CredentialKey != "." &&
		!strings.ContainsRune(s.CredentialKey, '\\') && s.ControllerID != "" && s.ControllerGeneration > 0 &&
		s.IdempotencyKey != "" && s.ProfileName != "" && s.ControlMode > 0 && s.ExecutionLane > 0 && s.Purpose > 0 &&
		s.RequiredAssurance > 0 && sort.StringsAreSorted(s.RequiredCapabilities) && unique(s.RequiredCapabilities) && !s.CreatedAt.IsZero()
}

func unique(values []string) bool {
	for i, value := range values {
		if value == "" || i > 0 && value == values[i-1] {
			return false
		}
	}
	return true
}

func Canonical(s StartRun) ([]byte, string, error) {
	if !s.valid() {
		return nil, "", ErrInvalid
	}
	b, err := json.Marshal(s)
	if err != nil || len(b) > MaximumBytes {
		return nil, "", ErrInvalid
	}
	digest := sha256.Sum256(b)
	return b, hex.EncodeToString(digest[:]), nil
}

// Store is rooted in a trusted application-support directory, outside every
// Workspace. Its parent is created by the host and must already be owner-only.
type Store struct{ Root string }

func (s Store) root() error {
	if !filepath.IsAbs(s.Root) {
		return ErrInvalid
	}
	info, err := os.Lstat(s.Root)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() || !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnavailable
	}
	return nil
}

func (s Store) path(key string) (string, error) {
	if !keyPattern.MatchString(key) || s.root() != nil {
		return "", ErrUnavailable
	}
	return filepath.Join(s.Root, key+".json"), nil
}

func (s Store) Put(request StartRun) (string, error) {
	b, digest, err := Canonical(request)
	if err != nil {
		return "", err
	}
	path, err := s.path(request.OperationID)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		f.Close()
		if !complete {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(b); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = syncDirectory(s.Root); err != nil {
		return "", err
	}
	complete = true
	return digest, nil
}

func (s Store) Get(key, digest string) (StartRun, error) {
	path, err := s.path(key)
	if err != nil {
		return StartRun{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return StartRun{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > MaximumBytes {
		return StartRun{}, ErrUnavailable
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return StartRun{}, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0600 || !os.SameFile(info, opened) {
		return StartRun{}, ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(f, MaximumBytes+1))
	if err != nil || len(b) > MaximumBytes {
		return StartRun{}, ErrUnavailable
	}
	actual := sha256.Sum256(b)
	if hex.EncodeToString(actual[:]) != digest {
		return StartRun{}, ErrUnavailable
	}
	var request StartRun
	if err := json.Unmarshal(b, &request); err != nil || request.OperationID != key || !request.valid() {
		return StartRun{}, ErrUnavailable
	}
	canonical, _, err := Canonical(request)
	if err != nil || string(canonical) != string(b) {
		return StartRun{}, ErrUnavailable
	}
	return request, nil
}

func (s Store) Delete(key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(s.Root)
}

// PurgeExpired deletes old files, including orphans left by a crash before
// attempt persistence. The caller then marks matching attempts unavailable.
func (s Store) PurgeExpired(now time.Time, maxAge time.Duration) ([]string, error) {
	if maxAge <= 0 || maxAge > MaximumAge || s.root() != nil {
		return nil, ErrInvalid
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, err
	}
	var expired []string
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".json" {
			return nil, fmt.Errorf("unknown replay store entry: %w", ErrUnavailable)
		}
		key := name[:len(name)-len(".json")]
		path, err := s.path(key)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Getuid() || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return nil, ErrUnavailable
		}
		if now.Sub(info.ModTime()) < maxAge {
			continue
		}
		if err := s.Delete(key); err != nil {
			return nil, err
		}
		expired = append(expired, key)
	}
	return expired, nil
}

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
