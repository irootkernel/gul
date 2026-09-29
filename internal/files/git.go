package files

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type GitState string

var ErrGitLimit = errors.New("git read limit exceeded")

const (
	GitAvailable     GitState = "available"
	GitNotRepository GitState = "not-repository"
	GitUnbornHead    GitState = "unborn-head"
	GitUnavailable   GitState = "unavailable"
	GitLimitExceeded GitState = "limit-exceeded"
)

type ChangeKind string

const (
	ChangeClean      ChangeKind = "clean"
	ChangeModified   ChangeKind = "modified"
	ChangeAdded      ChangeKind = "added"
	ChangeUntracked  ChangeKind = "untracked"
	ChangeDeleted    ChangeKind = "deleted"
	ChangeRenamed    ChangeKind = "renamed"
	ChangeConflicted ChangeKind = "conflicted"
	ChangeMixed      ChangeKind = "mixed"
)

const MaxGitOutputBytes = 1 << 20
const MaxGitEntries = 2000
const GitCommandTimeout = 5 * time.Second

type GitStatus struct {
	State     GitState
	Direct    ChangeKind
	Aggregate ChangeKind
	Staged    bool
	Unstaged  bool
}

type Comparison struct {
	State          GitState
	Change         ChangeKind
	Head           Preview
	Working        Preview
	HeadMissing    bool
	WorkingMissing bool
	PreviousPath   string
}

// Compare always attempts the local Working preview first. Git degradation
// cannot turn an accessible file into an unavailable current preview.
func (s *Service) Compare(ctx context.Context, subject, workspaceID, relative string) (Comparison, error) {
	if relative == "." {
		return Comparison{}, ErrPathUnavailable
	}
	if err := s.gitAllowedPath(ctx, subject, workspaceID, relative); err != nil {
		return Comparison{}, err
	}
	result := Comparison{Change: ChangeClean}
	working, err := s.ReadPreview(ctx, subject, workspaceID, relative)
	if err == nil {
		result.Working = working
	} else if errors.Is(err, ErrPathUnavailable) {
		result.WorkingMissing = true
	} else {
		return Comparison{}, err
	}
	git, state, err := s.gitContext(ctx, subject, workspaceID, relative)
	if err != nil {
		return Comparison{}, err
	}
	result.State = state
	if state != GitAvailable {
		return result, nil
	}
	entries, state := s.gitEntries(ctx, subject, workspaceID, git)
	if state != GitAvailable {
		result.State = state
		return result, nil
	}
	headPath := relative
	for _, entry := range entries {
		if entry.path != relative && entry.previous != relative {
			continue
		}
		result.Change = entry.kind
		if entry.kind == ChangeRenamed && entry.previous != "" && entry.path == relative {
			headPath = entry.previous
			result.PreviousPath = entry.previous
		}
		break
	}
	if err := s.gitAllowedPath(ctx, subject, workspaceID, headPath); err != nil {
		return Comparison{}, err
	}
	ext := strings.ToLower(path.Ext(headPath))
	raster := rasterExtension(ext)
	limit := int64(MaxTextBytes + 1)
	if raster {
		limit = MaxImageBytes
	}
	body, size, missing, state := s.readHeadBlob(ctx, git, headPath, limit, !raster)
	if missing {
		result.HeadMissing = true
		return result, nil
	}
	if state != GitAvailable {
		if state == GitLimitExceeded && raster {
			result.Head = Preview{Kind: PreviewUnsupported}
			return result, nil
		}
		result.State = state
		return result, nil
	}
	load := func(ctx context.Context, asset string, max int64) ([]byte, string, error) {
		if err := s.gitAllowedPath(ctx, subject, workspaceID, asset); err != nil {
			return nil, "", err
		}
		data, size, missing, state := s.readHeadBlob(ctx, git, asset, max, false)
		if missing || state != GitAvailable {
			return nil, "", ErrPathUnavailable
		}
		return readRaster(bytes.NewReader(data), size)
	}
	result.Head, err = previewReader(ctx, headPath, bytes.NewReader(body), size, load)
	if err != nil {
		return Comparison{}, err
	}
	return result, nil
}

func (s *Service) readHeadBlob(ctx context.Context, git gitContext, relative string, max int64, prefix bool) ([]byte, int64, bool, GitState) {
	abs := filepath.Join(git.workspaceRoot, filepath.FromSlash(relative))
	repoRelative, err := filepath.Rel(git.repoRoot, abs)
	if err != nil || !fs.ValidPath(filepath.ToSlash(repoRelative)) {
		return nil, 0, true, GitAvailable
	}
	repoRelative = filepath.ToSlash(repoRelative)
	if privateComponent(filepath.FromSlash(repoRelative)) {
		return nil, 0, true, GitAvailable
	}
	entry, err := s.runGit(ctx, git.repoRoot, 8192, "ls-tree", "-z", git.head, "--", ":(literal)"+repoRelative)
	if err != nil {
		return nil, 0, false, GitUnavailable
	}
	if len(entry) == 0 {
		return nil, 0, true, GitAvailable
	}
	separator := bytes.IndexByte(entry, '\t')
	if separator < 0 || !bytes.HasPrefix(entry, []byte("100")) || string(entry[separator+1:len(entry)-1]) != repoRelative {
		return nil, 0, false, GitUnavailable
	}
	sizeBytes, err := s.runGit(ctx, git.repoRoot, 32, "cat-file", "-s", git.head+":"+repoRelative)
	if err != nil {
		return nil, 0, false, GitUnavailable
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeBytes)), 10, 64)
	if err != nil || size < 0 {
		return nil, 0, false, GitUnavailable
	}
	if size > max && !prefix {
		return nil, size, false, GitLimitExceeded
	}
	data, overflow, err := s.runGitPrefix(ctx, git.repoRoot, max, "show", git.head+":"+repoRelative)
	if err != nil || (!prefix && overflow) || (!overflow && int64(len(data)) != size) {
		return nil, size, false, GitUnavailable
	}
	return data, size, false, GitAvailable
}

type gitEntry struct {
	path             string // Workspace-relative, never an absolute or private path
	previous         string
	kind             ChangeKind
	staged, unstaged bool
}

type gitContext struct {
	workspaceRoot string
	repoRoot      string
	head          string
}

func (s *Service) GitStatus(ctx context.Context, subject, workspaceID, relative string) (GitStatus, error) {
	if err := s.gitAllowedPath(ctx, subject, workspaceID, relative); err != nil {
		return GitStatus{}, err
	}
	git, state, err := s.gitContext(ctx, subject, workspaceID, relative)
	if err != nil {
		return GitStatus{}, err
	}
	if state != GitAvailable {
		return GitStatus{State: state, Direct: ChangeClean, Aggregate: ChangeClean}, nil
	}
	entries, state := s.gitEntries(ctx, subject, workspaceID, git)
	if state != GitAvailable {
		return GitStatus{State: state, Direct: ChangeClean, Aggregate: ChangeClean}, nil
	}
	result := GitStatus{State: GitAvailable, Direct: ChangeClean, Aggregate: ChangeClean}
	var aggregate ChangeKind
	for _, entry := range entries {
		if entry.path == relative || entry.previous == relative {
			result.Direct = entry.kind
			result.Staged = entry.staged
			result.Unstaged = entry.unstaged
		}
		if relative == "." || entry.path == relative || entry.previous == relative || strings.HasPrefix(entry.path, relative+"/") || strings.HasPrefix(entry.previous, relative+"/") {
			if aggregate == "" {
				aggregate = entry.kind
			} else if aggregate != entry.kind {
				aggregate = ChangeMixed
			}
		}
	}
	if aggregate != "" {
		result.Aggregate = aggregate
	}
	return result, nil
}

func (s *Service) gitAllowedPath(ctx context.Context, subject, workspaceID, relative string) error {
	if err := validateRelativePath(relative); err != nil {
		return err
	}
	if privateComponent(filepath.FromSlash(relative)) {
		return ErrPathUnavailable
	}
	if relative == "." {
		file, _, err := s.Open(ctx, subject, workspaceID, ".")
		if err == nil {
			file.Close()
		}
		return err
	}
	parts := strings.Split(relative, "/")
	for i := range parts {
		prefix := path.Join(parts[:i+1]...)
		file, _, err := s.Open(ctx, subject, workspaceID, prefix)
		if err == nil {
			file.Close()
			continue
		}
		if errors.Is(err, ErrReattachRequired) || errors.Is(err, ErrUnsupportedPathEncoding) {
			return err
		}
		parent := "."
		if i > 0 {
			parent = path.Join(parts[:i]...)
		}
		dir, _, openErr := s.Open(ctx, subject, workspaceID, parent)
		if openErr != nil {
			return openErr
		}
		var stat unix.Stat_t
		statErr := unix.Fstatat(int(dir.Fd()), parts[i], &stat, unix.AT_SYMLINK_NOFOLLOW)
		dir.Close()
		if errors.Is(statErr, unix.ENOENT) {
			return nil
		} // deleted path below a verified parent
		return ErrPathUnavailable // existing alias, private target or unsupported node
	}
	return nil
}

func (s *Service) gitContext(ctx context.Context, subject, workspaceID, relative string) (gitContext, GitState, error) {
	root, _, err := s.Open(ctx, subject, workspaceID, ".")
	if err != nil {
		return gitContext{}, "", err
	}
	rootPath, err := descriptorPath(root)
	root.Close()
	if err != nil {
		return gitContext{}, "", ErrReattachRequired
	}
	probe := relative
	for probe != "." {
		file, _, openErr := s.Open(ctx, subject, workspaceID, probe)
		if openErr == nil {
			info, statErr := file.Stat()
			file.Close()
			if statErr == nil && info.IsDir() {
				break
			}
		}
		probe = path.Dir(probe)
	}
	probeFile, _, err := s.Open(ctx, subject, workspaceID, probe)
	if err != nil {
		return gitContext{}, "", err
	}
	probePath, err := descriptorPath(probeFile)
	probeFile.Close()
	if err != nil {
		return gitContext{}, "", ErrPathUnavailable
	}
	output, runErr := s.runGit(ctx, probePath, 4096, "rev-parse", "--show-toplevel")
	if errors.Is(runErr, exec.ErrNotFound) || errors.Is(runErr, os.ErrNotExist) || errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, ErrGitLimit) {
		return gitContext{}, GitUnavailable, nil
	}
	if runErr != nil {
		return gitContext{}, GitNotRepository, nil
	}
	repoRoot := strings.TrimSpace(string(output))
	canonical, err := filepath.EvalSymlinks(repoRoot)
	if err != nil || canonical != repoRoot || !containedPath(rootPath, repoRoot) {
		return gitContext{}, GitNotRepository, nil
	}
	head, runErr := s.runGit(ctx, repoRoot, 128, "rev-parse", "--verify", "HEAD^{commit}")
	if errors.Is(runErr, exec.ErrNotFound) || errors.Is(runErr, os.ErrNotExist) || errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, ErrGitLimit) {
		return gitContext{workspaceRoot: rootPath, repoRoot: repoRoot}, GitUnavailable, nil
	}
	if runErr != nil {
		return gitContext{workspaceRoot: rootPath, repoRoot: repoRoot}, GitUnbornHead, nil
	}
	return gitContext{workspaceRoot: rootPath, repoRoot: repoRoot, head: strings.TrimSpace(string(head))}, GitAvailable, nil
}

func containedPath(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && fs.ValidPath(filepath.ToSlash(relative))
}

func (s *Service) runGit(parent context.Context, dir string, limit int64, args ...string) ([]byte, error) {
	data, overflow, err := s.runGitPrefix(parent, dir, limit, args...)
	if overflow {
		return nil, ErrGitLimit
	}
	return data, err
}

func (s *Service) runGitPrefix(parent context.Context, dir string, limit int64, args ...string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(parent, GitCommandTimeout)
	defer cancel()
	arguments := append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-C", dir}, args...)
	cmd := exec.CommandContext(ctx, s.gitBinary, arguments...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	if readErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, false, readErr
	}
	if int64(len(data)) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return data, true, nil
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, err
	}
	return data, false, nil
}

func (s *Service) gitEntries(ctx context.Context, subject, workspaceID string, git gitContext) ([]gitEntry, GitState) {
	raw, err := s.runGit(ctx, git.repoRoot, MaxGitOutputBytes, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".", ":(exclude,icase)**/"+privateDirectoryName, ":(exclude,icase)**/"+privateDirectoryName+"/**")
	if err != nil {
		if errors.Is(err, ErrGitLimit) {
			return nil, GitLimitExceeded
		}
		return nil, GitUnavailable
	}
	entries := make([]gitEntry, 0)
	for position := 0; position < len(raw); {
		if len(entries) >= MaxGitEntries || len(raw)-position < 4 || raw[position+2] != ' ' {
			return nil, GitLimitExceeded
		}
		x, y := raw[position], raw[position+1]
		position += 3
		end := bytes.IndexByte(raw[position:], 0)
		if end < 0 {
			return nil, GitUnavailable
		}
		file := string(raw[position : position+end])
		position += end + 1
		previous := ""
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			end = bytes.IndexByte(raw[position:], 0)
			if end < 0 {
				return nil, GitUnavailable
			}
			previous = string(raw[position : position+end])
			position += end + 1
		}
		relative, err := filepath.Rel(git.workspaceRoot, filepath.Join(git.repoRoot, filepath.FromSlash(file)))
		if err != nil {
			continue
		}
		relative = filepath.ToSlash(relative)
		if err := s.gitAllowedPath(ctx, subject, workspaceID, relative); err != nil {
			continue
		}
		if previous != "" {
			old, oldErr := filepath.Rel(git.workspaceRoot, filepath.Join(git.repoRoot, filepath.FromSlash(previous)))
			if oldErr == nil {
				old = filepath.ToSlash(old)
				if s.gitAllowedPath(ctx, subject, workspaceID, old) == nil {
					previous = old
				} else {
					previous = ""
				}
			}
		}
		entries = append(entries, gitEntry{path: relative, previous: previous, kind: classifyGitChange(x, y), staged: x != ' ' && x != '?', unstaged: y != ' ' || x == '?'})
	}
	return entries, GitAvailable
}

func classifyGitChange(x, y byte) ChangeKind {
	if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') || (x == 'A' && y == 'D') || (x == 'D' && y == 'A') {
		return ChangeConflicted
	}
	if x == 'R' || y == 'R' {
		return ChangeRenamed
	}
	if x == 'D' || y == 'D' {
		return ChangeDeleted
	}
	if x == 'A' || y == 'A' {
		return ChangeAdded
	}
	if x == '?' {
		return ChangeUntracked
	}
	return ChangeModified
}
