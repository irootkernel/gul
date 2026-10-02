package deployment

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderLaunchdPlist(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home & <owner>")
	for _, dir := range []string{home, filepath.Join(home, "Library"), filepath.Join(home, "Library", "Logs"), filepath.Join(home, "Library", "Logs", "Gul")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	logDir := filepath.Join(home, "Library", "Logs", "Gul")
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := os.WriteFile(filepath.Join(logDir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	config := LaunchdConfig{
		Home: home, BinaryPath: filepath.Join(home, "Gul & <app>", "gul"),
		DataRoot: filepath.Join(home, "Library", "Application Support", "Gul"),
		Port:     18423, TailnetHost: "node.ts.net:443",
		DolgoraeExecutable: filepath.Join(home, "provider & executable"), WorkspaceRoots: []string{filepath.Join(home, "workspace & root")}, Policies: []string{"policy & name"},
	}
	plist, err := RenderLaunchdPlist(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"--dolgorae-executable", "--workspace-root", "--policy", "provider &amp; executable", "workspace &amp; root", "policy &amp; name"} {
		if !strings.Contains(string(plist), value) {
			t.Fatal("provider configuration lost or unescaped", value)
		}
	}
	decoder := xml.NewDecoder(bytes.NewReader(plist))
	var stringsFound []string
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "string" {
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				t.Fatal(err)
			}
			stringsFound = append(stringsFound, value)
		}
	}
	for _, value := range []string{config.BinaryPath, config.DataRoot, "--port", "18423", "--tailnet-host", config.TailnetHost, filepath.Join(logDir, "stdout.log"), filepath.Join(logDir, "stderr.log")} {
		found := false
		for _, got := range stringsFound {
			found = found || got == value
		}
		if !found {
			t.Fatalf("plist lost or changed path %q", value)
		}
	}
	if !bytes.Contains(plist, []byte("<key>Umask</key><integer>63</integer>")) || !bytes.Contains(plist, []byte("<key>ThrottleInterval</key><integer>30</integer>")) {
		t.Fatal("agent lacks protected umask or restart throttle")
	}
	defaults := config
	defaults.Port = 0
	defaultPlist, err := RenderLaunchdPlist(defaults)
	if err != nil || bytes.Contains(defaultPlist, []byte("<string>--port</string>")) {
		t.Fatalf("default port should be selected by the host: %v", err)
	}
	for _, test := range []struct {
		name        string
		breakConfig func(*LaunchdConfig) error
	}{
		{"invalid port", func(c *LaunchdConfig) error { c.Port = 65536; return nil }},
		{"invalid tailnet", func(c *LaunchdConfig) error { c.TailnetHost = "example.com:443"; return nil }},
		{"relative binary", func(c *LaunchdConfig) error { c.BinaryPath = "gul"; return nil }},
		{"wrong data root", func(c *LaunchdConfig) error { c.DataRoot = "/tmp/Gul"; return nil }},
		{"missing log", func(c *LaunchdConfig) error { return os.Remove(filepath.Join(logDir, "stderr.log")) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := config
			if err := test.breakConfig(&copy); err != nil {
				t.Fatal(err)
			}
			if _, err := RenderLaunchdPlist(copy); err == nil {
				t.Fatal("unsafe agent rendered")
			}
		})
	}
}

func TestRenderLaunchdRejectsUnsafeLogs(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	logDir := filepath.Join(home, "Library", "Logs", "Gul")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := os.WriteFile(filepath.Join(logDir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	config := LaunchdConfig{Home: home, BinaryPath: "/Applications/Gul.app/Contents/MacOS/gul", DataRoot: filepath.Join(home, "Library", "Application Support", "Gul")}
	parent := filepath.Dir(logDir)
	for _, mode := range []os.FileMode{0770, 0702} {
		if err := os.Chmod(parent, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := RenderLaunchdPlist(config); err == nil || !strings.Contains(err.Error(), "log parent") {
			t.Fatalf("writable log parent accepted: %v", err)
		}
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderLaunchdPlist(config); err == nil {
		t.Fatal("public log directory accepted")
	}
	if err := os.Chmod(logDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(logDir, "stdout.log"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderLaunchdPlist(config); err == nil {
		t.Fatal("public log file accepted")
	}
	if err := os.Remove(filepath.Join(logDir, "stdout.log")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(logDir, "stderr.log"), filepath.Join(logDir, "stdout.log")); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderLaunchdPlist(config); err == nil {
		t.Fatal("symlink log file accepted")
	}
	config.BinaryPath = "/Applications/\x01gul"
	if _, err := RenderLaunchdPlist(config); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatal("invalid XML path accepted")
	}
}
