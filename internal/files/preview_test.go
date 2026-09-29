package files

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writePreviewFile(t *testing.T, root, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryPagesStayBoundedAndHidePrivateAliases(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 205; i++ {
		writePreviewFile(t, root, fmt.Sprintf("file-%03d", i), []byte("ok"))
	}
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret", []byte("private"))
	if err := os.Symlink(".dolgorae", filepath.Join(root, "private-alias")); err != nil {
		t.Fatal(err)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	token := ""
	seen := 0
	privateNodes := 0
	for {
		page, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 40, token)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Entries) > 40 {
			t.Fatalf("page size %d", len(page.Entries))
		}
		for _, entry := range page.Entries {
			if entry.Name == "private-alias" || entry.Name == "secret" {
				t.Fatalf("private entry leaked: %+v", entry)
			}
			if entry.ProviderManagedDenied {
				privateNodes++
				if entry.Name != ".dolgorae" || entry.Directory {
					t.Fatalf("private node %+v", entry)
				}
			}
			seen++
		}
		if page.NextToken == "" {
			break
		}
		if len(page.NextToken) != 32 {
			t.Fatalf("cursor token is not opaque: %q", page.NextToken)
		}
		if token == "" {
			if _, err := service.ListDirectory(t.Context(), "other", "entry", ".", 40, page.NextToken); !errors.Is(err, ErrInvalidPageToken) {
				t.Fatalf("foreign subject cursor = %v", err)
			}
		}
		token = page.NextToken
	}
	if seen != 206 || privateNodes != 1 {
		t.Fatalf("visible entries %d, private nodes %d", seen, privateNodes)
	}
	if _, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 101, ""); err == nil {
		t.Fatal("unbounded page accepted")
	}
	if _, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 10, "999999999"); !errors.Is(err, ErrInvalidPageToken) {
		t.Fatalf("invalid cursor = %v", err)
	}
}

func TestDirectoryRejectsOpaqueByteNamesWithoutBrowserAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, string([]byte{'x', 0xff})), []byte("opaque"), 0600); err != nil {
		if errors.Is(err, syscall.EILSEQ) {
			t.Skip("host filesystem rejects invalid byte names")
		}
		t.Fatal(err)
	}
	service := NewService(attachmentStore{attachedRoot(t, root)})
	page, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 10, "")
	if !errors.Is(err, ErrUnsupportedPathEncoding) || len(page.Entries) != 0 {
		t.Fatalf("opaque name = %+v, %v", page, err)
	}
}

func TestDirectoryCursorExpiryAndEviction(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "a.txt", []byte("a"))
	writePreviewFile(t, root, "b.txt", []byte("b"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	first, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 1, "")
	if err != nil || first.NextToken == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	service.cursorMu.Lock()
	service.cursors[first.NextToken].expires = time.Now().Add(-time.Second)
	service.cursorMu.Unlock()
	if _, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 1, first.NextToken); !errors.Is(err, ErrPageTokenExpired) {
		t.Fatalf("expired cursor = %v", err)
	}

	tokens := make([]string, maxDirectoryCursors+1)
	for i := range tokens {
		page, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 1, "")
		if err != nil || page.NextToken == "" {
			t.Fatalf("cursor %d = %+v, %v", i, page, err)
		}
		tokens[i] = page.NextToken
	}
	if _, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 1, tokens[0]); !errors.Is(err, ErrPageTokenExpired) {
		t.Fatalf("evicted cursor = %v", err)
	}
	if page, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 1, tokens[len(tokens)-1]); err != nil || len(page.Entries) != 1 {
		t.Fatalf("retained newest cursor = %+v, %v", page, err)
	}
}

func TestTextPreviewLimitsAndEncoding(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "source.go", []byte("package main\nfunc main() {}\n"))
	writePreviewFile(t, root, "large.txt", []byte(strings.Repeat("a", MaxTextBytes+10)))
	writePreviewFile(t, root, "lines.txt", []byte(strings.Repeat("line\n", MaxTextLines+10)))
	writePreviewFile(t, root, "exact-lines.txt", []byte(strings.Repeat("line\n", MaxTextLines)))
	writePreviewFile(t, root, "cr-lines.md", []byte(strings.Repeat("# x\r", MaxTextLines+10)))
	writePreviewFile(t, root, "crlf-lines.md", []byte(strings.Repeat("# x\r\n", MaxTextLines+10)))
	writePreviewFile(t, root, "binary.txt", []byte{0xff, 0xfe})
	writePreviewFile(t, root, "boundary.txt", []byte(strings.Repeat("a", MaxTextBytes-1)+"€tail"))
	writePreviewFile(t, root, "invalid-middle.txt", append([]byte("invalid"), append([]byte{0xff}, bytes.Repeat([]byte{'a'}, MaxTextBytes)...)...))
	writePreviewFile(t, root, "unsafe.svg", []byte(`<svg onload="alert(1)"><script>alert(1)</script></svg>`))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for _, tc := range []struct {
		name      string
		kind      PreviewKind
		truncated bool
		max       int
	}{
		{"source.go", PreviewText, false, MaxTextBytes}, {"large.txt", PreviewText, true, MaxTextBytes},
		{"lines.txt", PreviewText, true, MaxTextBytes}, {"binary.txt", PreviewUnsupported, false, 0},
		{"exact-lines.txt", PreviewText, false, MaxTextBytes}, {"cr-lines.md", PreviewMarkdown, true, MaxTextBytes},
		{"crlf-lines.md", PreviewMarkdown, true, MaxTextBytes},
		{"boundary.txt", PreviewText, true, MaxTextBytes}, {"invalid-middle.txt", PreviewUnsupported, false, 0},
		{"unsafe.svg", PreviewSVGSource, false, MaxTextBytes},
	} {
		preview, err := service.ReadPreview(t.Context(), "owner", "entry", tc.name)
		if err != nil || preview.Kind != tc.kind || preview.Truncated != tc.truncated || len(preview.Text) > tc.max || len(preview.Image) != 0 {
			t.Fatalf("%s = %+v, %v", tc.name, preview, err)
		}
		if tc.name == "boundary.txt" && preview.Text != strings.Repeat("a", MaxTextBytes-1) {
			t.Fatalf("boundary prefix changed: %d bytes", len(preview.Text))
		}
		if tc.name == "cr-lines.md" && strings.Count(preview.Text, "\r") != MaxTextLines {
			t.Fatalf("CR line cap = %d", strings.Count(preview.Text, "\r"))
		}
		if tc.name == "crlf-lines.md" && strings.Count(preview.Text, "\r\n") != MaxTextLines {
			t.Fatalf("CRLF line cap = %d", strings.Count(preview.Text, "\r\n"))
		}
	}
}

func TestRasterAndMarkdownAssetsUseGuardedReads(t *testing.T) {
	root := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var pngData, jpegData, gifData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegData, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&gifData, img, nil); err != nil {
		t.Fatal(err)
	}
	webpData, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"a.png": pngData.Bytes(), "b.jpg": jpegData.Bytes(), "c.gif": gifData.Bytes(), "d.webp": webpData} {
		writePreviewFile(t, root, name, body)
	}
	if err := os.Mkdir(filepath.Join(root, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, filepath.Join(root, ".dolgorae"), "secret.png", pngData.Bytes())
	if err := os.Symlink(".dolgorae/secret.png", filepath.Join(root, "private.png")); err != nil {
		t.Fatal(err)
	}
	writePreviewFile(t, root, "page.md", []byte("# Preview\n![safe](a.png)\n![private](private.png)\n![external](https://example.invalid/x.png)\n![escape](../../x.png)\n"))
	writePreviewFile(t, root, "late-asset.md", []byte(strings.Repeat("![external](https://example.invalid/x.png)\n", 40)+"![safe](a.png)\n"))
	writePreviewFile(t, root, "fenced-assets.md", []byte("```md\n"+strings.Repeat("![code](a.png)\n", 8)+"```\n![visible](a.png)\n"))
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for name, want := range map[string]string{"a.png": "image/png", "b.jpg": "image/jpeg", "c.gif": "image/gif", "d.webp": "image/webp"} {
		preview, err := service.ReadPreview(t.Context(), "owner", "entry", name)
		if err != nil || preview.Kind != PreviewRaster || preview.MIME != want || len(preview.Image) == 0 {
			t.Fatalf("%s = %+v, %v", name, preview, err)
		}
	}
	preview, err := service.ReadPreview(t.Context(), "owner", "entry", "page.md")
	if err != nil || preview.Kind != PreviewMarkdown || len(preview.MarkdownImages) != 1 || preview.MarkdownImages[0].Reference != "a.png" {
		t.Fatalf("markdown = %+v, %v", preview, err)
	}
	late, err := service.ReadPreview(t.Context(), "owner", "entry", "late-asset.md")
	if err != nil || len(late.MarkdownImages) != 1 || late.MarkdownImages[0].Reference != "a.png" {
		t.Fatalf("late asset = %+v, %v", late, err)
	}
	fenced, err := service.ReadPreview(t.Context(), "owner", "entry", "fenced-assets.md")
	if err != nil || len(fenced.MarkdownImages) != 1 || fenced.MarkdownImages[0].Reference != "a.png" {
		t.Fatalf("fenced asset = %+v, %v", fenced, err)
	}
	writePreviewFile(t, root, "too-large.png", bytes.Repeat([]byte{0}, MaxImageBytes+1))
	tooLarge, err := service.ReadPreview(t.Context(), "owner", "entry", "too-large.png")
	if err != nil || tooLarge.Kind != PreviewUnsupported {
		t.Fatalf("oversize = %+v, %v", tooLarge, err)
	}
	if _, err := service.ReadPreview(t.Context(), "owner", "entry", "private.png"); !errors.Is(err, ErrPathUnavailable) {
		t.Fatalf("private alias = %v", err)
	}
}

func TestRasterRejectsDeclaredOversizeDimensions(t *testing.T) {
	root := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	largePNG := bytes.Clone(encoded.Bytes())
	binary.BigEndian.PutUint32(largePNG[16:20], 6000)
	binary.BigEndian.PutUint32(largePNG[20:24], 5000)
	binary.BigEndian.PutUint32(largePNG[29:33], crc32.ChecksumIEEE(largePNG[12:29]))
	writePreviewFile(t, root, "huge.png", largePNG)
	largeWebP := make([]byte, 30)
	copy(largeWebP, "RIFF")
	binary.LittleEndian.PutUint32(largeWebP[4:8], 22)
	copy(largeWebP[8:], "WEBPVP8X")
	largeWebP[24], largeWebP[25] = 0x6f, 0x17 // width 6000
	largeWebP[27], largeWebP[28] = 0x87, 0x13 // height 5000
	writePreviewFile(t, root, "huge.webp", largeWebP)
	service := NewService(attachmentStore{attachedRoot(t, root)})
	for _, name := range []string{"huge.png", "huge.webp"} {
		preview, err := service.ReadPreview(t.Context(), "owner", "entry", name)
		if err != nil || preview.Kind != PreviewUnsupported || len(preview.Image) != 0 {
			t.Fatalf("%s = %+v, %v", name, preview, err)
		}
	}
}

func TestMarkdownImageCountAndAggregateBudget(t *testing.T) {
	var source strings.Builder
	for i := 0; i < MaxMarkdownImages+1; i++ {
		fmt.Fprintf(&source, "![image](%d.png)\n", i)
	}
	calls := 0
	images, err := markdownImages(context.Background(), "page.md", source.String(), func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
		calls++
		return []byte("image"), "image/png", nil
	})
	if err != nil || len(images) != MaxMarkdownImages || calls != MaxMarkdownImages {
		t.Fatalf("count cap: %d assets, %d loads, %v", len(images), calls, err)
	}
	data := bytes.Repeat([]byte{'x'}, 3<<20)
	var limits []int64
	images, err = markdownImages(context.Background(), "page.md", "![a](a.png)\n![b](b.png)\n![c](c.png)\n", func(_ context.Context, _ string, limit int64) ([]byte, string, error) {
		limits = append(limits, limit)
		if int64(len(data)) > limit {
			return nil, "", ErrPathUnavailable
		}
		return data, "image/png", nil
	})
	if err != nil || len(images) != 2 || len(limits) != 3 || limits[2] != MaxMarkdownImageBytes-2*int64(len(data)) {
		t.Fatalf("aggregate cap: %d assets, limits %v, %v", len(images), limits, err)
	}
}

func TestMarkdownImageReferenceGrammar(t *testing.T) {
	for _, tc := range []struct {
		text, reference string
	}{
		{"![x](a.png)", "a.png"},
		{"![" + strings.Repeat("a", 256) + "](a.png)", "a.png"},
		{"![x](" + strings.Repeat("a", 512) + ")", strings.Repeat("a", 512)},
		{"![" + strings.Repeat("a", 257) + "](a.png)", ""},
		{"![x](" + strings.Repeat("a", 513) + ")", ""},
	} {
		match := markdownImagePattern.FindStringSubmatch(tc.text)
		if tc.reference == "" {
			if match != nil {
				t.Fatalf("unexpected match for %d-byte reference", len(tc.text))
			}
		} else if len(match) != 2 || match[1] != tc.reference {
			t.Fatalf("reference %q = %v", tc.reference, match)
		}
	}
}
