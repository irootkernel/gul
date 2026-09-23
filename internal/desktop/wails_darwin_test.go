package desktop

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rootkernel/gul/internal/delivery/web"
)

func TestWailsHostUsesCheckedBundleWithoutServices(t *testing.T) {
	options := wailsOptions(func() {})
	if options.Name != "Gul" || options.OnShutdown == nil || !options.Mac.ApplicationShouldTerminateAfterLastWindowClosed || len(options.Services) != 0 || options.Assets.Handler == nil {
		t.Fatalf("unsafe shell options: %+v", options)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	options.Assets.Handler.ServeHTTP(response, request)
	want, err := fs.ReadFile(web.ShellAssets(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(response.Result().Body)
	if err != nil || response.Code != http.StatusOK || string(got) != string(want) {
		t.Fatalf("shell bundle = status %d, bytes %d, %v", response.Code, len(got), err)
	}
}
