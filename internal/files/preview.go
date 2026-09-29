package files

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rootkernel/gul/internal/domain"
)

const (
	MaxDirectoryPage      = domain.MaximumPageSize
	MaxTextBytes          = domain.MaximumFileTextBytes
	MaxTextLines          = domain.MaximumFileTextLines
	MaxImageBytes         = domain.MaximumFileImageBytes
	MaxImagePixels        = domain.MaximumFileImagePixels
	MaxMarkdownImages     = domain.MaximumFileMarkdownImages
	MaxMarkdownImageBytes = domain.MaximumFileMarkdownImageBytes
)

type Entry struct {
	Name                  string
	Directory             bool
	Size                  int64
	ProviderManagedDenied bool
}

type DirectoryPage struct {
	Entries   []Entry
	NextToken string
}

type directoryCursor struct {
	dir                            *os.File
	subject, workspaceID, relative string
	pageSize                       uint32
	pending                        string
	expires                        time.Time
}

const maxDirectoryCursors = 128
const directoryCursorTTL = 2 * time.Minute

var ErrInvalidPageToken = errors.New("invalid file page token")
var ErrPageTokenExpired = errors.New("file page token expired")

type PreviewKind string

const (
	PreviewText        PreviewKind = "text"
	PreviewMarkdown    PreviewKind = "markdown"
	PreviewRaster      PreviewKind = "raster"
	PreviewSVGSource   PreviewKind = "svg-source"
	PreviewUnsupported PreviewKind = "unsupported"
)

type MarkdownImage struct {
	Reference string
	MIME      string
	Data      []byte
}

type Preview struct {
	Kind           PreviewKind
	Text           string
	Image          []byte
	MIME           string
	Language       string
	Truncated      bool
	MarkdownImages []MarkdownImage
}

// ListDirectory retains a bounded number of short-lived directory descriptors
// so every page reads at most pageSize+1 raw names. The cursor is live, not a
// snapshot; explicit refresh starts again from page one.
func (s *Service) ListDirectory(ctx context.Context, subject, workspaceID, relative string, pageSize uint32, pageToken string) (DirectoryPage, error) {
	if s == nil {
		return DirectoryPage{}, ErrReattachRequired
	}
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize > MaxDirectoryPage {
		return DirectoryPage{}, ErrPathUnavailable
	}
	cursor, err := s.directoryCursor(ctx, subject, workspaceID, relative, pageSize, pageToken)
	if err != nil {
		return DirectoryPage{}, err
	}
	retained := false
	defer func() {
		if !retained {
			cursor.dir.Close()
		}
	}()
	names := make([]string, 0, pageSize+1)
	if cursor.pending != "" {
		names = append(names, cursor.pending)
	}
	more, readErr := cursor.dir.Readdirnames(int(pageSize) + 1 - len(names))
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return DirectoryPage{}, ErrPathUnavailable
	}
	names = append(names, more...)
	hasMore := len(names) > int(pageSize)
	if hasMore {
		cursor.pending = names[pageSize]
		names = names[:pageSize]
	}
	result := DirectoryPage{Entries: make([]Entry, 0, len(names))}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return DirectoryPage{}, err
		}
		if !utf8.ValidString(name) {
			return DirectoryPage{}, ErrUnsupportedPathEncoding
		}
		if privateComponent(name) {
			if name == privateDirectoryName {
				result.Entries = append(result.Entries, Entry{Name: name, ProviderManagedDenied: true})
			}
			continue
		}
		child := name
		if relative != "." {
			child = path.Join(relative, name)
		}
		file, _, openErr := s.Open(ctx, subject, workspaceID, child)
		if openErr != nil {
			continue
		} // escaped, private, vanished or special nodes never become links
		stat, statErr := file.Stat()
		file.Close()
		if statErr != nil {
			continue
		}
		result.Entries = append(result.Entries, Entry{Name: name, Directory: stat.IsDir(), Size: stat.Size()})
	}
	if err := ctx.Err(); err != nil {
		return DirectoryPage{}, err
	}
	if hasMore {
		result.NextToken, err = s.retainDirectoryCursor(cursor)
		if err != nil {
			return DirectoryPage{}, err
		}
		retained = true
	}
	return result, nil
}

func (s *Service) directoryCursor(ctx context.Context, subject, workspaceID, relative string, pageSize uint32, token string) (*directoryCursor, error) {
	if token == "" {
		dir, _, err := s.Open(ctx, subject, workspaceID, relative)
		if err != nil {
			return nil, err
		}
		info, err := dir.Stat()
		if err != nil || !info.IsDir() {
			dir.Close()
			return nil, ErrPathUnavailable
		}
		return &directoryCursor{dir: dir, subject: subject, workspaceID: workspaceID, relative: relative, pageSize: pageSize}, nil
	}
	if len(token) != 32 {
		return nil, ErrInvalidPageToken
	}
	s.cursorMu.Lock()
	cursor := s.cursors[token]
	if cursor == nil {
		s.cursorMu.Unlock()
		return nil, ErrPageTokenExpired
	}
	if cursor.subject != subject || cursor.workspaceID != workspaceID || cursor.relative != relative || cursor.pageSize != pageSize {
		s.cursorMu.Unlock()
		return nil, ErrInvalidPageToken
	}
	delete(s.cursors, token)
	s.cursorMu.Unlock()
	if time.Now().After(cursor.expires) {
		cursor.dir.Close()
		return nil, ErrPageTokenExpired
	}
	// A cursor is reusable only while the same directory is still reachable
	// through the verified Workspace path and subject-scoped attachment.
	current, _, err := s.Open(ctx, subject, workspaceID, relative)
	if err != nil {
		cursor.dir.Close()
		return nil, err
	}
	currentInfo, currentErr := current.Stat()
	cursorInfo, cursorErr := cursor.dir.Stat()
	current.Close()
	if currentErr != nil || cursorErr != nil || !os.SameFile(currentInfo, cursorInfo) {
		cursor.dir.Close()
		return nil, ErrPageTokenExpired
	}
	return cursor, nil
}

func (s *Service) retainDirectoryCursor(cursor *directoryCursor) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", ErrPathUnavailable
	}
	token := hex.EncodeToString(random[:])
	now := time.Now()
	s.cursorMu.Lock()
	defer s.cursorMu.Unlock()
	if s.cursors == nil {
		s.cursors = make(map[string]*directoryCursor)
	}
	for key, old := range s.cursors {
		if now.After(old.expires) {
			old.dir.Close()
			delete(s.cursors, key)
		}
	}
	if len(s.cursors) >= maxDirectoryCursors {
		var oldestKey string
		var oldest *directoryCursor
		for key, item := range s.cursors {
			if oldest == nil || item.expires.Before(oldest.expires) {
				oldestKey, oldest = key, item
			}
		}
		oldest.dir.Close()
		delete(s.cursors, oldestKey)
	}
	cursor.expires = now.Add(directoryCursorTTL)
	s.cursors[token] = cursor
	return token, nil
}

func (s *Service) ReadPreview(ctx context.Context, subject, workspaceID, relative string) (Preview, error) {
	file, _, err := s.Open(ctx, subject, workspaceID, relative)
	if err != nil {
		return Preview{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Preview{}, ErrPathUnavailable
	}
	load := func(ctx context.Context, asset string, limit int64) ([]byte, string, error) {
		file, _, err := s.Open(ctx, subject, workspaceID, asset)
		if err != nil {
			return nil, "", err
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil || !stat.Mode().IsRegular() || stat.Size() > limit {
			return nil, "", ErrPathUnavailable
		}
		return readRaster(file, stat.Size())
	}
	return previewReader(ctx, relative, file, info.Size(), load)
}

type rasterLoader func(context.Context, string, int64) ([]byte, string, error)

func previewReader(ctx context.Context, relative string, reader io.Reader, size int64, load rasterLoader) (Preview, error) {
	ext := strings.ToLower(path.Ext(relative))
	if rasterExtension(ext) {
		data, kind, err := readRaster(reader, size)
		if err != nil {
			return Preview{Kind: PreviewUnsupported}, nil
		}
		if err := ctx.Err(); err != nil {
			return Preview{}, err
		}
		return Preview{Kind: PreviewRaster, Image: data, MIME: kind}, nil
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxTextBytes+1))
	if err != nil {
		return Preview{}, ErrPathUnavailable
	}
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	truncated := len(data) > MaxTextBytes
	if truncated {
		data = data[:MaxTextBytes]
		for removed := 0; removed < 3 && !utf8.Valid(data); removed++ {
			data = data[:len(data)-1]
		}
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return Preview{Kind: PreviewUnsupported}, nil
	}
	lines := 0
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' && data[i] != '\r' {
			continue
		}
		lines++
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			i++
		}
		if lines == MaxTextLines {
			if i+1 < len(data) {
				data = data[:i+1]
				truncated = true
			}
			break
		}
	}
	result := Preview{Kind: PreviewText, Text: string(data), Language: sourceLanguage(ext), Truncated: truncated}
	if ext == ".svg" {
		result.Kind = PreviewSVGSource
	}
	if ext == ".md" || ext == ".markdown" {
		result.Kind = PreviewMarkdown
		result.MarkdownImages, err = markdownImages(ctx, relative, result.Text, load)
		if err != nil {
			return Preview{}, err
		}
	}
	return result, nil
}

func rasterExtension(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func sourceLanguage(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".json":
		return "json"
	case ".py":
		return "python"
	case ".sh":
		return "shell"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".md", ".markdown":
		return "markdown"
	case ".svg", ".xml":
		return "xml"
	default:
		return ""
	}
}

func readRaster(file io.Reader, size int64) ([]byte, string, error) {
	if size <= 0 || size > MaxImageBytes {
		return nil, "", ErrPathUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxImageBytes+1))
	if err != nil || len(data) != int(size) {
		return nil, "", ErrPathUnavailable
	}
	var kind string
	var width, height int
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) && uint64(binary.LittleEndian.Uint32(data[4:8]))+8 == uint64(len(data)) {
		kind = "image/webp"
		width, height = webpSize(data)
	} else {
		config, format, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil {
			return nil, "", ErrPathUnavailable
		}
		width, height = config.Width, config.Height
		switch format {
		case "png":
			kind = "image/png"
		case "jpeg":
			kind = "image/jpeg"
		case "gif":
			kind = "image/gif"
		}
	}
	if kind == "" || width <= 0 || height <= 0 || int64(width)*int64(height) > MaxImagePixels {
		return nil, "", ErrPathUnavailable
	}
	return data, kind, nil
}

func webpSize(data []byte) (int, int) {
	if len(data) < 30 {
		return 0, 0
	}
	switch string(data[12:16]) {
	case "VP8X":
		return 1 + int(data[24]) + (int(data[25]) << 8) + (int(data[26]) << 16), 1 + int(data[27]) + (int(data[28]) << 8) + (int(data[29]) << 16)
	case "VP8L":
		if len(data) < 25 || data[20] != 0x2f {
			return 0, 0
		}
		return 1 + int(data[21]) + (int(data[22]&0x3f) << 8), 1 + int(data[22]>>6) + (int(data[23]) << 2) + (int(data[24]&0x0f) << 10)
	case "VP8 ":
		if len(data) < 30 || !bytes.Equal(data[23:26], []byte{0x9d, 0x01, 0x2a}) {
			return 0, 0
		}
		return int(binary.LittleEndian.Uint16(data[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(data[28:30]) & 0x3fff)
	}
	return 0, 0
}

var markdownImagePattern = regexp.MustCompile(`!\[[^\]\n]{0,256}\]\(([^)\n]{1,512})\)`)

func markdownImages(ctx context.Context, relative, content string, load rasterLoader) ([]MarkdownImage, error) {
	result := make([]MarkdownImage, 0)
	seen := make(map[string]bool)
	total := 0
	code := false
	for _, line := range strings.FieldsFunc(content, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if strings.HasPrefix(line, "```") {
			code = !code
			continue
		}
		if code {
			continue
		}
		for offset := 0; offset < len(line) && len(result) < MaxMarkdownImages; {
			match := markdownImagePattern.FindStringSubmatchIndex(line[offset:])
			if match == nil {
				break
			}
			ref := line[offset+match[2] : offset+match[3]]
			offset += match[1]
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if seen[ref] || strings.ContainsAny(ref, `\:#?%`) || strings.HasPrefix(ref, "/") {
				continue
			}
			seen[ref] = true
			asset := path.Join(path.Dir(relative), ref)
			if !fs.ValidPath(asset) || asset == "." {
				continue
			}
			data, kind, err := load(ctx, asset, MaxMarkdownImageBytes-int64(total))
			if err != nil {
				continue
			}
			total += len(data)
			result = append(result, MarkdownImage{Reference: ref, MIME: kind, Data: data})
		}
		if len(result) >= MaxMarkdownImages {
			break
		}
	}
	return result, ctx.Err()
}
