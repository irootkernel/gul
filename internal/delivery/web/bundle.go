package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed dist
var embedded embed.FS

var assets = mustSub(embedded, "dist")

const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Assets returns the one checked frontend bundle used by every delivery host.
func Assets() fs.FS {
	return assets
}

// ShellAssets is the desktop-shell delivery boundary. The later Wails task
// consumes this exact filesystem rather than creating another frontend build.
func ShellAssets() fs.FS {
	return Assets()
}

// BrowserHandler serves the same bundle for browser delivery. Authentication
// and the production listener remain owned by later tasks.
func BrowserHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		response.Header().Set("X-Content-Type-Options", "nosniff")
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		content, err := fs.ReadFile(Assets(), name)
		if err != nil && path.Ext(name) == "" {
			name = "index.html"
			content, err = fs.ReadFile(Assets(), name)
		}
		if err != nil {
			http.NotFound(response, request)
			return
		}

		response.Header().Set("Content-Type", contentType(name))
		http.ServeContent(response, request, name, time.Time{}, strings.NewReader(string(content)))
	})
}

func contentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

func mustSub(root fs.FS, directory string) fs.FS {
	sub, err := fs.Sub(root, directory)
	if err != nil {
		panic(err)
	}
	return sub
}
