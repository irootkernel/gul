package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ImageReference struct{ WorkspaceID, RelativePath, Detail string }
type StagedImage struct{ Path, Detail string }
type stagedInput struct {
	run, digest, directory string
	images                 []StagedImage
	accepted               bool
	acceptedAt             time.Time
}

// SubmitImages snapshots already checked descriptors into private host scratch
// files. Dolgorae never reopens a mutable Workspace path after Gul's guard.
// Unknown input remains protected and bounded; it is never replayed on restart.
type SubmitImages struct {
	Root  string
	Files *Service
	mu    sync.Mutex
	live  map[string]stagedInput
}

func (s *SubmitImages) Stage(ctx context.Context, subject, workspace, run, key string, references []ImageReference) ([]StagedImage, error) {
	if s == nil || s.Files == nil || len(references) > 16 {
		return nil, ErrPathUnavailable
	}
	if len(references) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Lstat(s.Root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrPathUnavailable
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(owner.Uid) != os.Getuid() {
		return nil, ErrPathUnavailable
	}
	resolved, err := filepath.EvalSymlinks(s.Root)
	if err != nil || resolved != s.Root {
		return nil, ErrPathUnavailable
	}
	var data [][]byte
	var extensions []string
	defer func() {
		for _, b := range data {
			clear(b)
		}
	}()
	hash := sha256.New()
	for _, ref := range references {
		if ref.WorkspaceID != workspace || (ref.Detail != "auto" && ref.Detail != "low" && ref.Detail != "high") {
			return nil, ErrPathUnavailable
		}
		file, relative, err := s.Files.Open(ctx, subject, workspace, ref.RelativePath)
		if err != nil {
			return nil, err
		}
		stat, err := file.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			file.Close()
			return nil, ErrPathUnavailable
		}
		body, mime, err := readRaster(file, stat.Size())
		attachment, attachmentErr := s.Files.attachments.Attachment(ctx, subject, workspace)
		current, pathErr := verifiedOpenPath(file, attachment.CanonicalRoot)
		file.Close()
		if err != nil || attachmentErr != nil || pathErr != nil || current != relative {
			clear(body)
			return nil, ErrPathUnavailable
		}
		ext := ""
		switch mime {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		default:
			clear(body)
			return nil, ErrPathUnavailable
		}
		data = append(data, body)
		extensions = append(extensions, ext)
		hash.Write([]byte(ref.WorkspaceID + "\x00" + ref.RelativePath + "\x00" + ref.Detail + "\x00"))
		d := sha256.Sum256(body)
		hash.Write(d[:])
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if old, ok := s.live[key]; ok {
		if old.run != run || old.digest != digest {
			return nil, ErrPathUnavailable
		}
		return old.images, nil
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil || len(entries) >= 16 {
		return nil, ErrPathUnavailable
	}
	directory, err := os.MkdirTemp(s.Root, inputPrefix(run))
	if err != nil {
		return nil, ErrPathUnavailable
	}
	complete := false
	defer func() {
		if !complete {
			os.RemoveAll(directory)
		}
	}()
	images := make([]StagedImage, 0, len(data))
	for i, body := range data {
		file, err := os.CreateTemp(directory, "image-*"+extensions[i])
		if err != nil {
			return nil, ErrPathUnavailable
		}
		_, err = file.Write(body)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return nil, ErrPathUnavailable
		}
		images = append(images, StagedImage{Path: file.Name(), Detail: references[i].Detail})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.live == nil {
		s.live = make(map[string]stagedInput)
	}
	s.live[key] = stagedInput{run: run, digest: digest, directory: directory, images: images}
	complete = true
	return images, nil
}

func (s *SubmitImages) Accepted(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.live[key]
	if ok {
		v.accepted = true
		v.acceptedAt = time.Now()
		s.live[key] = v
	}
}
func inputPrefix(run string) string {
	digest := sha256.Sum256([]byte(run))
	return "submit-" + hex.EncodeToString(digest[:]) + "-"
}

// Rejected is safe only for a known pre-dispatch refusal or a resolved provider
// rejection. Unknown acceptance keeps its files until aggregate closure.
func (s *SubmitImages) Rejected(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.live[key]; ok && !v.accepted {
		if os.RemoveAll(v.directory) == nil {
			delete(s.live, key)
		}
	}
}

func (s *SubmitImages) ReleaseRun(run string, closed bool, observedAfter time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, v := range s.live {
		if v.run == run && (closed || (v.accepted && observedAfter.After(v.acceptedAt))) {
			if os.RemoveAll(v.directory) == nil {
				delete(s.live, key)
			}
		}
	}
	// Only authoritative whole-session closure can settle files left by an
	// earlier host process. Idle alone cannot settle an unknown Submit outcome.
	if closed {
		entries, err := os.ReadDir(s.Root)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), inputPrefix(run)) {
				os.RemoveAll(filepath.Join(s.Root, entry.Name()))
			}
		}
	}
}
