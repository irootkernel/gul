package files

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
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
			if _, err := service.ListDirectory(t.Context(), "other", "entry", ".", 40, page.NextToken); err == nil {
				t.Fatal("foreign subject used cursor")
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
	if _, err := service.ListDirectory(t.Context(), "owner", "entry", ".", 10, "999999999"); err == nil {
		t.Fatal("invalid cursor accepted")
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
