// Package workspace owns Gul's verified, presentation-only workspace attachment.
package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	ErrSelectionUnavailable     = errors.New("workspace selection unavailable")
	ErrSelectionCancelled       = errors.New("workspace selection cancelled")
	ErrHostSelectionUnavailable = errors.New("host workspace selection unavailable")
	ErrWorkspaceBlocked         = errors.New("workspace is not provisioned or compatible")
	ErrWorkspaceUninitialized   = errors.New("workspace is not initialized")
	ErrProfileMissing           = errors.New("workspace profile is missing or incompatible")
	ErrProfileServerUnavailable = errors.New("profile server unavailable")
	ErrIdentityMismatch         = errors.New("workspace identity mismatch; reattachment required")
	ErrReattachRequired         = errors.New("workspace reattachment required")
	ErrAttachmentNotFound       = errors.New("workspace attachment not found")
	ErrPersistenceUnavailable   = errors.New("workspace persistence unavailable")
)

type Attachment struct {
	SubjectID     string
	ID            string
	CanonicalRoot string
	ProviderID    string
	FileDevice    string
	FileInode     string
	DisplayName   string
	Favorite      bool
	Hidden        bool
}

type Inspection struct {
	CanonicalRoot string
	ProviderID    string
	Compatible    bool
	Blocker       Blocker
}

type Blocker uint8

const (
	BlockerUnknown Blocker = iota
	BlockerUninitialized
	BlockerProfileMissing
	BlockerProfileServerUnavailable
)

type Provider interface {
	InspectWorkspace(context.Context, string, *string) (Inspection, error)
}

type Repository interface {
	CreateAttachment(context.Context, Attachment) error
	ReplaceAttachment(context.Context, Attachment) error
	Attachment(context.Context, string, string) (Attachment, error)
	ListAttachments(context.Context, string) ([]Attachment, error)
}

// Picker is implemented only by a trusted host. A browser never supplies its path.
type Picker interface {
	PickDirectory(context.Context) (string, error)
}

type RootOption struct {
	ID    string
	Label string
}

type DirectoryOption struct {
	Name         string
	RelativePath string
}

type allowedRoot struct {
	id   string
	path string
	info os.FileInfo
}

type Service struct {
	provider          Provider
	repo              Repository
	picker            Picker
	roots             []allowedRoot
	rootConfigInvalid bool
	mu                sync.Mutex
}

// NewService resolves the host allowlist once. Any invalid root disables remote
// browsing as a whole, while leaving the host picker available.
func NewService(provider Provider, repo Repository, picker Picker, configuredRoots []string) *Service {
	s := &Service{provider: provider, repo: repo, picker: picker}
	for index, configured := range configuredRoots {
		root, err := openDirectory(configured)
		if err != nil || isPrivatePath(root.path) {
			if root.file != nil {
				root.file.Close()
			}
			s.roots = nil
			s.rootConfigInvalid = true
			return s
		}
		info, err := root.file.Stat()
		root.file.Close()
		if err != nil {
			s.roots = nil
			s.rootConfigInvalid = true
			return s
		}
		s.roots = append(s.roots, allowedRoot{id: fmt.Sprintf("root-%d", index+1), path: root.path, info: info})
	}
	return s
}

func (s *Service) RootConfigurationError() error {
	if s.rootConfigInvalid {
		return ErrHostSelectionUnavailable
	}
	return nil
}

func (s *Service) ListRegistrableRoots() []RootOption {
	counts := make(map[string]int, len(s.roots))
	for _, root := range s.roots {
		counts[filepath.Base(root.path)]++
	}
	options := make([]RootOption, 0, len(s.roots))
	for _, root := range s.roots {
		label := filepath.Base(root.path)
		if counts[label] > 1 {
			label = fmt.Sprintf("%s (%s)", label, root.id)
		}
		options = append(options, RootOption{ID: root.id, Label: label})
	}
	return options
}

func (s *Service) List(ctx context.Context, subjectID string) ([]Attachment, error) {
	if s.repo == nil || subjectID == "" {
		return nil, ErrSelectionUnavailable
	}
	entries, err := s.repo.ListAttachments(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
	}
	return entries, nil
}

func (s *Service) BrowseRegistrableRoot(rootID, relative string) ([]DirectoryOption, error) {
	if s.rootConfigInvalid {
		return nil, ErrHostSelectionUnavailable
	}
	selected, err := s.openAllowlisted(rootID, relative)
	if err != nil {
		return nil, ErrSelectionUnavailable
	}
	defer selected.file.Close()
	entries, err := selected.file.ReadDir(0)
	if err != nil {
		return nil, ErrSelectionUnavailable
	}
	options := make([]DirectoryOption, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.EqualFold(name, ".dolgorae") {
			continue
		}
		fd, err := unix.Openat(int(selected.file.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		unix.Close(fd)
		next := name
		if relative != "." {
			next = path.Join(relative, name)
		}
		options = append(options, DirectoryOption{Name: name, RelativePath: next})
	}
	slices.SortFunc(options, func(a, b DirectoryOption) int { return strings.Compare(a.Name, b.Name) })
	return options, nil
}

func (s *Service) RegisterFromHostSelection(ctx context.Context, subjectID string) (Attachment, error) {
	if s.picker == nil {
		return Attachment{}, ErrHostSelectionUnavailable
	}
	path, err := s.picker.PickDirectory(ctx)
	if err != nil {
		if errors.Is(err, ErrSelectionCancelled) {
			return Attachment{}, ErrSelectionCancelled
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Attachment{}, err
		}
		return Attachment{}, ErrHostSelectionUnavailable
	}
	selected, err := openDirectory(path)
	if err != nil || isPrivatePath(selected.path) {
		if selected.file != nil {
			selected.file.Close()
		}
		return Attachment{}, ErrSelectionUnavailable
	}
	defer selected.file.Close()
	return s.register(ctx, subjectID, selected, nil)
}

func (s *Service) RegisterFromAllowlistPath(ctx context.Context, subjectID, rootID, relative string) (Attachment, error) {
	if s.rootConfigInvalid {
		return Attachment{}, ErrHostSelectionUnavailable
	}
	selected, err := s.openAllowlisted(rootID, relative)
	if err != nil {
		return Attachment{}, ErrSelectionUnavailable
	}
	defer selected.file.Close()
	for i := range s.roots {
		if s.roots[i].id == rootID {
			return s.register(ctx, subjectID, selected, &s.roots[i])
		}
	}
	return Attachment{}, ErrSelectionUnavailable
}

func (s *Service) register(ctx context.Context, subjectID string, selected directory, root *allowedRoot) (Attachment, error) {
	if subjectID == "" || s.provider == nil || s.repo == nil {
		return Attachment{}, ErrSelectionUnavailable
	}
	inspection, err := s.provider.InspectWorkspace(ctx, selected.path, nil)
	if err != nil {
		return Attachment{}, err
	}
	if !inspection.Compatible || inspection.ProviderID == "" {
		return Attachment{}, inspectionBlocker(inspection)
	}
	canonical, err := filepath.EvalSymlinks(inspection.CanonicalRoot)
	if err != nil || !filepath.IsAbs(canonical) || isPrivatePath(canonical) {
		return Attachment{}, ErrIdentityMismatch
	}
	if root != nil && !contained(root.path, canonical) {
		return Attachment{}, ErrIdentityMismatch
	}
	selectedInfo, err := selected.file.Stat()
	if err != nil {
		return Attachment{}, ErrIdentityMismatch
	}
	providerInfo, err := os.Stat(canonical)
	if err != nil || !os.SameFile(selectedInfo, providerInfo) {
		return Attachment{}, ErrIdentityMismatch
	}
	device, inode, ok := fileIdentity(selectedInfo)
	if !ok {
		return Attachment{}, ErrIdentityMismatch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.repo.ListAttachments(ctx, subjectID)
	if err != nil {
		return Attachment{}, fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
	}
	var matched *Attachment
	for _, entry := range previous {
		if !attachmentRelated(entry, canonical, inspection.ProviderID, device, inode) {
			continue
		}
		if matched != nil && matched.ID != entry.ID {
			return Attachment{}, ErrIdentityMismatch
		}
		copy := entry
		matched = &copy
	}
	if matched != nil {
		if attachmentExact(*matched, canonical, inspection.ProviderID, device, inode) {
			return *matched, nil
		}
		matched.ProviderID = inspection.ProviderID
		matched.CanonicalRoot = canonical
		matched.FileDevice, matched.FileInode = device, inode
		if err := s.repo.ReplaceAttachment(ctx, *matched); err != nil {
			return Attachment{}, fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
		}
		return *matched, nil
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return Attachment{}, err
	}
	entry := Attachment{SubjectID: subjectID, ID: hex.EncodeToString(identity[:]), CanonicalRoot: canonical,
		ProviderID: inspection.ProviderID, FileDevice: device, FileInode: inode, DisplayName: filepath.Base(canonical)}
	if err := s.repo.CreateAttachment(ctx, entry); err != nil {
		// Another service or process may have committed the same attachment
		// after our duplicate read. Return it only after verifying all identity
		// fields, regardless of the storage driver's constraint error text.
		if current, readErr := s.repo.ListAttachments(ctx, subjectID); readErr == nil {
			for _, candidate := range current {
				if attachmentExact(candidate, canonical, inspection.ProviderID, device, inode) {
					return candidate, nil
				}
			}
		}
		return Attachment{}, fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
	}
	return entry, nil
}

func attachmentRelated(entry Attachment, canonical, providerID, device, inode string) bool {
	return entry.ProviderID == providerID || entry.CanonicalRoot == canonical ||
		entry.FileDevice == device && entry.FileInode == inode
}

func attachmentExact(entry Attachment, canonical, providerID, device, inode string) bool {
	return entry.ProviderID == providerID && entry.CanonicalRoot == canonical &&
		entry.FileDevice == device && entry.FileInode == inode
}

// Revalidate supplies the only WorkspaceRef material that later scoped calls
// may use. A browser-provided ID is a lookup key, never the provider identity.
func (s *Service) Revalidate(ctx context.Context, subjectID, entryID string) (Attachment, error) {
	if s.provider == nil || s.repo == nil || subjectID == "" || entryID == "" {
		return Attachment{}, ErrReattachRequired
	}
	entry, err := s.repo.Attachment(ctx, subjectID, entryID)
	if err != nil {
		if errors.Is(err, ErrAttachmentNotFound) {
			return Attachment{}, ErrReattachRequired
		}
		return Attachment{}, fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
	}
	selected, err := openDirectory(entry.CanonicalRoot)
	if err != nil {
		return Attachment{}, ErrReattachRequired
	}
	defer selected.file.Close()
	selectedInfo, err := selected.file.Stat()
	if err != nil {
		return Attachment{}, ErrReattachRequired
	}
	device, inode, ok := fileIdentity(selectedInfo)
	if !ok || device != entry.FileDevice || inode != entry.FileInode {
		return Attachment{}, ErrReattachRequired
	}
	inspection, err := s.provider.InspectWorkspace(ctx, entry.CanonicalRoot, &entry.ProviderID)
	if err != nil {
		return Attachment{}, err
	}
	if !inspection.Compatible {
		return Attachment{}, inspectionBlocker(inspection)
	}
	if inspection.ProviderID != entry.ProviderID {
		return Attachment{}, ErrReattachRequired
	}
	canonical, err := filepath.EvalSymlinks(inspection.CanonicalRoot)
	if err != nil || !filepath.IsAbs(canonical) || isPrivatePath(canonical) {
		return Attachment{}, ErrReattachRequired
	}
	providerInfo, providerErr := os.Stat(canonical)
	if providerErr != nil || !os.SameFile(selectedInfo, providerInfo) {
		return Attachment{}, ErrReattachRequired
	}
	return entry, nil
}

func inspectionBlocker(inspection Inspection) error {
	switch inspection.Blocker {
	case BlockerUninitialized:
		return ErrWorkspaceUninitialized
	case BlockerProfileMissing:
		return ErrProfileMissing
	case BlockerProfileServerUnavailable:
		return ErrProfileServerUnavailable
	default:
		return ErrWorkspaceBlocked
	}
}

func fileIdentity(info os.FileInfo) (string, string, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", false
	}
	return fmt.Sprint(stat.Dev), fmt.Sprint(stat.Ino), true
}

type directory struct {
	path string
	file *os.File
}

func openDirectory(value string) (directory, error) {
	if !filepath.IsAbs(value) || strings.ContainsRune(value, 0) {
		return directory{}, ErrSelectionUnavailable
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil || !filepath.IsAbs(resolved) {
		return directory{}, ErrSelectionUnavailable
	}
	fd, err := unix.Open(resolved, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return directory{}, ErrSelectionUnavailable
	}
	return directory{path: filepath.Clean(resolved), file: os.NewFile(uintptr(fd), resolved)}, nil
}

func (s *Service) openAllowlisted(rootID, relative string) (directory, error) {
	if !fs.ValidPath(relative) || strings.ContainsRune(relative, '\\') || isPrivatePath(relative) {
		return directory{}, ErrSelectionUnavailable
	}
	for _, root := range s.roots {
		if root.id != rootID {
			continue
		}
		selected, err := openDirectory(root.path)
		if err != nil {
			return directory{}, ErrSelectionUnavailable
		}
		info, err := selected.file.Stat()
		if err != nil || !os.SameFile(info, root.info) {
			selected.file.Close()
			return directory{}, ErrSelectionUnavailable
		}
		if relative == "." {
			return selected, nil
		}
		for _, component := range strings.Split(relative, "/") {
			fd, err := unix.Openat(int(selected.file.Fd()), component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			selected.file.Close()
			if err != nil {
				return directory{}, ErrSelectionUnavailable
			}
			selected.path = filepath.Join(selected.path, component)
			selected.file = os.NewFile(uintptr(fd), selected.path)
		}
		return selected, nil
	}
	return directory{}, ErrSelectionUnavailable
}

func contained(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	// The final SameFile check still proves the selected directory's identity.
	// This spelling check permits case aliases on case-insensitive host volumes.
	if strings.EqualFold(root, candidate) {
		return true
	}
	prefix := strings.TrimSuffix(root, string(filepath.Separator)) + string(filepath.Separator)
	return len(candidate) >= len(prefix) && strings.EqualFold(candidate[:len(prefix)], prefix)
}

func isPrivatePath(value string) bool {
	for _, component := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.EqualFold(component, ".dolgorae") {
			return true
		}
	}
	return false
}
