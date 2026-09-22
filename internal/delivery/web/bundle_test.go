package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type bundleManifest struct {
	Schema     string `json:"schema"`
	Entrypoint string `json:"entrypoint"`
	Files      []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Size   int    `json:"size"`
	} `json:"files"`
}

func TestEmbeddedBundleMatchesManifest(t *testing.T) {
	manifestBytes, err := fs.ReadFile(Assets(), "bundle-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest bundleManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "gul.frontend-bundle/v1" || manifest.Entrypoint != "index.html" {
		t.Fatalf("unexpected bundle manifest: %#v", manifest)
	}
	manifestPaths := make([]string, 0, len(manifest.Files)+1)
	for _, file := range manifest.Files {
		manifestPaths = append(manifestPaths, file.Path)
		content, err := fs.ReadFile(Assets(), file.Path)
		if err != nil {
			t.Fatalf("read %s: %v", file.Path, err)
		}
		digest := sha256.Sum256(content)
		if got := hex.EncodeToString(digest[:]); got != file.SHA256 || len(content) != file.Size {
			t.Fatalf("%s does not match its manifest", file.Path)
		}
	}
	manifestPaths = append(manifestPaths, "bundle-manifest.json")
	slices.Sort(manifestPaths)
	for name, bundle := range map[string]fs.FS{"browser": Assets(), "shell": ShellAssets()} {
		if paths := inventory(t, bundle); !slices.Equal(paths, manifestPaths) {
			t.Fatalf("%s paths = %v, want manifest paths %v", name, paths, manifestPaths)
		}
	}
	index, err := fs.ReadFile(Assets(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`id="root"`,
		`href="/assets/gul.css"`,
		`src="/assets/gul.js"`,
	} {
		if !bytes.Contains(index, []byte(marker)) {
			t.Fatalf("checked index is missing %s", marker)
		}
	}
}

func TestBrowserAndShellDeliverTheSameBundle(t *testing.T) {
	for _, name := range inventory(t, ShellAssets()) {
		shellContent, err := fs.ReadFile(ShellAssets(), name)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		response := httptest.NewRecorder()
		BrowserHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /%s status = %d", name, response.Code)
		}
		if got := response.Body.Bytes(); !bytes.Equal(got, shellContent) {
			t.Fatalf("browser and shell bytes differ for %s", name)
		}
	}
}

func TestBrowserDeliveryHasNoProductAPI(t *testing.T) {
	index, err := fs.ReadFile(Assets(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, requestPath := range []string{"/", "/index.html", "/workspace", "/assets/../index.html"} {
		for _, credential := range []struct{ name, value string }{{}, {"Authorization", "Bearer ignored"}, {"Cookie", "session=ignored"}} {
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			if credential.name != "" {
				request.Header.Set(credential.name, credential.value)
			}
			response := httptest.NewRecorder()
			BrowserHandler().ServeHTTP(response, request)
			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), index) {
				t.Fatalf("GET %s with %s did not return the checked index", requestPath, credential.name)
			}
		}
	}
	javascript, err := fs.ReadFile(Assets(), "assets/gul.js")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(javascript, []byte("Product access remains unavailable until authentication is configured.")) {
		t.Fatal("delivered JavaScript lost the fail-closed foundation message")
	}
}

func TestBrowserDeliveryRejectsMissingPathsAndWriteMethods(t *testing.T) {
	for _, requestPath := range []string{
		"/assets/missing.js",
		"/../go.mod",
		"/%2e%2e/go.mod",
		"/assets/../../go.mod",
	} {
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		response := httptest.NewRecorder()
		BrowserHandler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", requestPath, response.Code, http.StatusNotFound)
		}
		assertSafetyHeaders(t, response)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		for _, requestPath := range []string{"/", "/assets/gul.js"} {
			request := httptest.NewRequest(method, requestPath, strings.NewReader(""))
			response := httptest.NewRecorder()
			BrowserHandler().ServeHTTP(response, request)
			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s status = %d, want %d", method, requestPath, response.Code, http.StatusMethodNotAllowed)
			}
			if allow := response.Header().Get("Allow"); allow != "GET, HEAD" {
				t.Fatalf("%s %s Allow = %q", method, requestPath, allow)
			}
			assertSafetyHeaders(t, response)
		}
	}
}

func TestBrowserDeliveryHeadersAndHead(t *testing.T) {
	cases := []struct {
		path        string
		contentType string
	}{
		{path: "/", contentType: "text/html; charset=utf-8"},
		{path: "/workspace", contentType: "text/html; charset=utf-8"},
		{path: "/index.html", contentType: "text/html; charset=utf-8"},
		{path: "/assets/gul.js", contentType: "text/javascript; charset=utf-8"},
		{path: "/assets/gul.css", contentType: "text/css; charset=utf-8"},
	}
	for _, testCase := range cases {
		requestPath := testCase.path
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		response := httptest.NewRecorder()
		BrowserHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", requestPath, response.Code)
		}
		if response.Header().Get("Content-Type") != testCase.contentType {
			t.Fatalf("GET %s missing delivery headers", requestPath)
		}
		assertSafetyHeaders(t, response)

		headRequest := httptest.NewRequest(http.MethodHead, requestPath, nil)
		headResponse := httptest.NewRecorder()
		BrowserHandler().ServeHTTP(headResponse, headRequest)
		if headResponse.Code != response.Code || headResponse.Body.Len() != 0 {
			t.Fatalf("HEAD %s status/body = %d/%d", requestPath, headResponse.Code, headResponse.Body.Len())
		}
		for _, header := range []string{"Cache-Control", "Content-Type", "Content-Length", "Content-Security-Policy", "X-Content-Type-Options"} {
			if headResponse.Header().Get(header) != response.Header().Get(header) {
				t.Fatalf("HEAD %s %s differs from GET", requestPath, header)
			}
		}
	}
}

func assertSafetyHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Security-Policy") != "default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'" {
		t.Fatalf("response missing browser safety headers: %v", response.Header())
	}
}

func inventory(t *testing.T, bundle fs.FS) []string {
	t.Helper()
	var paths []string
	if err := fs.WalkDir(bundle, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(paths)
	return paths
}
