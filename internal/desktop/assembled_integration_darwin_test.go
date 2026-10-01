package desktop

import (
	"bufio"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	publicv1 "github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1"
	"github.com/rootkernel/gul/test/acceptance/fixture"
)

// The probe uses the checked application's DOM, without an HTTP test endpoint
// or a Wails API bridge. Its result leaves the child through test-only stdout.
const nativeAcceptanceScript = `(() => {
 const text = document.body?.innerText || '';
 const button = name => [...document.querySelectorAll('button')].find(b => b.textContent === name);
 const password = document.querySelector('input[type=password]');
 if (password) {
  if (!window.__acceptanceLogin) {
   window.__acceptanceLogin = true;
   Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(password, 'assembled fixture password 2026');
   password.dispatchEvent(new Event('input', {bubbles:true}));
   setTimeout(() => button('Sign in')?.click(), 100);
  }
  return 'waiting';
 }
 if (!document.querySelector('main[aria-label="Gul operator"]')) return 'waiting';
 if (!window.__acceptanceSession) { const session = button('Session'); if (session) {window.__acceptanceSession=true;session.click();} return 'waiting'; }
 if (!window.__acceptanceHistory) { const history = button('Prompt History'); if (history) {window.__acceptanceHistory=true;history.click();} return 'waiting'; }
 if (!window.__acceptanceOriginal) { const original = document.querySelector('button[aria-label="View full original prompt 1"]'); if (original) {window.__acceptanceOriginal=true;original.click();} return 'waiting'; }
 const original = document.querySelector('[aria-label="Full original prompt"] pre');
 if (original?.textContent !== 'Native original\r\n한글') return 'waiting';
 if (!window.__acceptanceFile) { const file = button('README.md'); if (file) {window.__acceptanceFile=true;file.click();} return 'waiting'; }
 if (!text.includes('Safe native file original.')) return 'waiting';
 if (text.includes('secret.txt') || localStorage.length) return 'unsafe';
 return 'passed';
})()`

func TestNativeAssembledCheckedApplication(t *testing.T) {
	if os.Getenv("GUL_RUN_NATIVE_WAILS_TEST") != "1" {
		t.Skip("set GUL_RUN_NATIVE_WAILS_TEST=1 for the isolated assembled AppKit/WebKit check")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "Workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("Safe native file original.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, ".dolgorae"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".dolgorae", "secret.txt"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := fixture.New(filepath.Join(root, "Gul"), workspace, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := f.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	if err = f.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.Provider.GetRun(t.Context(), &publicv1.GetRunRequest{Run: f.Run})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Provider.SubmitTurn(t.Context(), &publicv1.SubmitTurnRequest{Run: f.Run, Controller: &publicv1.ControllerCarrierRef{AbsoluteFilePath: filepath.Join(f.Workspace, ".dolgorae", "controller.json"), ExpectedControllerId: "acceptance-controller", ExpectedControllerGeneration: 1}, ExpectedStateRevision: snapshot.Run.StateRevision, IdempotencyKey: "native-seed", Message: "Native original\r\n한글", WriteIntent: publicv1.WriteIntent_WRITE_INTENT_READ})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Complete(); err != nil {
		t.Fatal(err)
	}
	isolated := filepath.Join(root, "Native")
	if err = os.Mkdir(isolated, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), nativeHelperMarker+"=1", "GUL_NATIVE_ASSEMBLED_PROBE=1", "GUL_NATIVE_TEST_URL="+f.Host.Origin(), "GUL_NATIVE_TEST_DER="+base64.StdEncoding.EncodeToString(f.Host.CertificateDER()), "GUL_NATIVE_TEST_CREDENTIAL=", "HOME="+isolated, "CFFIXED_USER_HOME="+isolated, "TMPDIR="+isolated)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(root, "native-stderr-")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	results := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "GUL_NATIVE_ACCEPTANCE:") {
				results <- strings.TrimPrefix(scanner.Text(), "GUL_NATIVE_ACCEPTANCE:")
				return
			}
		}
		results <- "child exited"
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	finished := false
	defer func() {
		stdin.Close()
		if !finished {
			cmd.Process.Kill()
			<-done
		}
	}()
	select {
	case result := <-results:
		if result != "passed" {
			log, _ := os.ReadFile(stderr.Name())
			t.Fatalf("native checked bundle: %s: %s", result, log)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("native checked application timed out")
	}
	stdin.Close()
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("assembled native window did not close")
	}
}
