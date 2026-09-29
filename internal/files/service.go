// Package files owns read-only access beneath a verified Workspace root.
package files

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/rootkernel/gul/internal/workspace"
	"golang.org/x/sys/unix"
)

var (
	ErrPathUnavailable         = errors.New("file path unavailable")
	ErrUnsupportedPathEncoding = errors.New("file path is not UTF-8")
	ErrReattachRequired        = errors.New("workspace reattachment required")
)

const privateDirectoryName = ".dolgorae"

// Attachments supplies the saved, subject-scoped root without contacting the
// runtime provider. Every open checks that root's filesystem identity again.
type Attachments interface {
	Attachment(context.Context, string, string) (workspace.Attachment, error)
}

type Service struct {
	attachments Attachments
	cursorMu    sync.Mutex
	cursors     map[string]*directoryCursor
	revisions   map[string]uint64
	watchers    int
	gitBinary   string
}

func NewService(attachments Attachments) *Service {
	return &Service{attachments: attachments, gitBinary: "git"}
}

type Node struct {
	Directory bool
	Size      int64
}

func (s *Service) Inspect(ctx context.Context, subject, workspaceID, relative string) (Node, error) {
	file, _, err := s.Open(ctx, subject, workspaceID, relative)
	if err != nil {
		return Node{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Node{}, ErrPathUnavailable
	}
	return Node{Directory: info.IsDir(), Size: info.Size()}, nil
}

func validateRelativePath(relative string) error {
	if !utf8.ValidString(relative) {
		return ErrUnsupportedPathEncoding
	}
	if len(relative) > 4096 || !fs.ValidPath(relative) || strings.ContainsAny(relative, "\\\x00") {
		return ErrPathUnavailable
	}
	return nil
}

// Open returns an already checked descriptor and its resolved root-relative
// path. Callers must use the descriptor instead of reopening the path.
func (s *Service) Open(ctx context.Context, subject, workspaceID, relative string) (*os.File, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if err := validateRelativePath(relative); err != nil {
		return nil, "", err
	}
	if subject == "" || workspaceID == "" {
		return nil, "", ErrPathUnavailable
	}
	if s == nil || s.attachments == nil {
		return nil, "", ErrReattachRequired
	}
	entry, err := s.attachments.Attachment(ctx, subject, workspaceID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, "", ctxErr
	}
	if err != nil || entry.ProviderID == "" || entry.CanonicalRoot == "" || !filepath.IsAbs(entry.CanonicalRoot) ||
		filepath.Clean(entry.CanonicalRoot) != entry.CanonicalRoot {
		return nil, "", ErrReattachRequired
	}
	canonical, err := filepath.EvalSymlinks(entry.CanonicalRoot)
	if err != nil || canonical != entry.CanonicalRoot || privateComponent(canonical) {
		return nil, "", ErrReattachRequired
	}
	root, err := unix.Open(entry.CanonicalRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", ErrReattachRequired
	}
	defer unix.Close(root)
	var rootInfo unix.Stat_t
	if err := unix.Fstat(root, &rootInfo); err != nil ||
		fmt.Sprint(rootInfo.Dev) != entry.FileDevice || fmt.Sprint(rootInfo.Ino) != entry.FileInode {
		return nil, "", ErrReattachRequired
	}
	if relative == "." {
		copy, err := unix.Dup(root)
		if err != nil {
			return nil, "", ErrPathUnavailable
		}
		file := os.NewFile(uintptr(copy), entry.CanonicalRoot)
		if !rootPathCurrent(entry.CanonicalRoot, rootInfo) {
			file.Close()
			return nil, "", ErrReattachRequired
		}
		if _, err := verifiedOpenPath(file, entry.CanonicalRoot); err != nil {
			file.Close()
			return nil, "", err
		}
		return file, ".", nil
	}
	file, resolved, err := walk(root, entry.CanonicalRoot, relative)
	if err != nil {
		return nil, "", err
	}
	if !rootPathCurrent(entry.CanonicalRoot, rootInfo) {
		file.Close()
		return nil, "", ErrReattachRequired
	}
	actual, err := verifiedOpenPath(file, entry.CanonicalRoot)
	if err != nil || actual != resolved {
		file.Close()
		return nil, "", ErrPathUnavailable
	}
	return file, actual, nil
}

// A directory opened during the walk can be moved before a child is opened.
// Check the opened node's current location before handing its descriptor to a
// reader; its original component names alone cannot prove containment.
func verifiedOpenPath(file *os.File, rootPath string) (string, error) {
	actual, err := descriptorPath(file)
	if err != nil || !filepath.IsAbs(actual) || filepath.Clean(actual) != actual {
		return "", ErrPathUnavailable
	}
	relative, err := filepath.Rel(rootPath, actual)
	if err != nil || !fs.ValidPath(filepath.ToSlash(relative)) || privateComponent(relative) {
		return "", ErrPathUnavailable
	}
	pathInfo, err := os.Lstat(actual)
	if err != nil {
		return "", ErrPathUnavailable
	}
	openInfo, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, openInfo) {
		return "", ErrPathUnavailable
	}
	return filepath.ToSlash(relative), nil
}

func rootPathCurrent(rootPath string, expected unix.Stat_t) bool {
	canonical, err := filepath.EvalSymlinks(rootPath)
	if err != nil || canonical != rootPath {
		return false
	}
	info, err := os.Stat(rootPath)
	if err != nil {
		return false
	}
	current, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint64(current.Dev) == uint64(expected.Dev) && current.Ino == expected.Ino
}

func walk(root int, rootPath, relative string) (*os.File, string, error) {
	const maxSymlinks = 40
	current, err := unix.Dup(root)
	if err != nil {
		return nil, "", ErrPathUnavailable
	}
	defer func() {
		if current >= 0 {
			unix.Close(current)
		}
	}()
	parts := strings.Split(relative, "/")
	resolved := make([]string, 0, len(parts))
	symlinks := 0
	for len(parts) > 0 {
		name := parts[0]
		parts = parts[1:]
		if privateComponent(name) {
			return nil, "", ErrPathUnavailable
		}
		var before unix.Stat_t
		if err := unix.Fstatat(current, name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return nil, "", ErrPathUnavailable
		}
		if before.Mode&unix.S_IFMT == unix.S_IFLNK {
			symlinks++
			if symlinks > maxSymlinks {
				return nil, "", ErrPathUnavailable
			}
			buffer := make([]byte, 4096)
			n, err := unix.Readlinkat(current, name, buffer)
			if err != nil || n == len(buffer) || !utf8.Valid(buffer[:n]) {
				return nil, "", ErrPathUnavailable
			}
			target := string(buffer[:n])
			if filepath.IsAbs(target) {
				target, err = filepath.Rel(rootPath, filepath.Clean(target))
				if err != nil {
					return nil, "", ErrPathUnavailable
				}
			} else {
				target = path.Join(append(resolved, target)...)
			}
			if len(parts) > 0 {
				target = path.Join(target, path.Join(parts...))
			}
			if !fs.ValidPath(target) || target == "." {
				return nil, "", ErrPathUnavailable
			}
			parts = strings.Split(target, "/")
			resolved = resolved[:0]
			unix.Close(current)
			current, err = unix.Dup(root)
			if err != nil {
				return nil, "", ErrPathUnavailable
			}
			continue
		}
		if before.Mode&unix.S_IFMT != unix.S_IFREG && before.Mode&unix.S_IFMT != unix.S_IFDIR {
			return nil, "", ErrPathUnavailable
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
		if len(parts) > 0 {
			flags |= unix.O_DIRECTORY
		}
		next, err := unix.Openat(current, name, flags, 0)
		if err != nil {
			return nil, "", ErrPathUnavailable
		}
		var after unix.Stat_t
		if err := unix.Fstat(next, &after); err != nil || before.Dev != after.Dev || before.Ino != after.Ino ||
			(before.Mode&unix.S_IFMT) != (after.Mode&unix.S_IFMT) {
			unix.Close(next)
			return nil, "", ErrPathUnavailable
		}
		unix.Close(current)
		current = next
		resolved = append(resolved, name)
	}
	file := os.NewFile(uintptr(current), filepath.Join(rootPath, filepath.Join(resolved...)))
	current = -1
	return file, path.Join(resolved...), nil
}

func privateComponent(value string) bool {
	for _, name := range strings.Split(value, string(filepath.Separator)) {
		if strings.EqualFold(name, privateDirectoryName) {
			return true
		}
	}
	return false
}
