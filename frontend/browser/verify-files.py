"""Exercise E7 file navigation in real Chrome against explicit typed clients."""
import functools
import http.server
import pathlib
import re
import subprocess
import tempfile
import threading

ROOT = pathlib.Path(__file__).resolve().parents[2]
SESSION = "gul-e7-files"


def cli(directory, *args):
    result = subprocess.run(["playwright-cli", f"-s={SESSION}", *args], cwd=directory, text=True, capture_output=True)
    assert result.returncode == 0 and "### Error" not in result.stdout, result.stdout + result.stderr
    return result.stdout


def snapshot(directory):
    output = cli(directory, "snapshot")
    match = re.search(r"\[Snapshot\]\(([^)]+)\)", output)
    assert match, output
    return (pathlib.Path(directory) / match.group(1)).read_text()


def ref(snap, label):
    lines = [line for line in snap.splitlines() if label in line]
    assert len(lines) == 1, (label, lines)
    match = re.search(r"\[ref=(e\d+)\]", lines[0])
    assert match, lines[0]
    return match.group(1)


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


with tempfile.TemporaryDirectory(prefix="gul-e7-files-") as directory:
    subprocess.run(["bun", "build", "frontend/browser/files.tsx", "--target", "browser", "--outdir", directory], cwd=ROOT, check=True)
    pathlib.Path(directory, "index.html").write_text('<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="files.css"></head><body><div id="root"></div><script type="module" src="files.js"></script></body></html>')
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(QuietHandler, directory=directory))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        cli(directory, "open", f"http://127.0.0.1:{server.server_port}/")
        cli(directory, "resize", "1280", "800")
        snap = snapshot(directory)
        assert "Alpha" in snap and "First session" in snap and "A writer is active" in snap, snap
        assert 'option "Hidden"' not in snap and "Archived session" not in snap, snap
        assert "Provider managed" in snap, snap
        cli(directory, "run-code", "async page => { await page.getByRole('region', {name:'Workspace files'}).waitFor(); const shown = await page.locator('.operator__pane').evaluateAll(nodes => nodes.map(node => getComputedStyle(node).display)); if (shown.some(value => value === 'none')) throw Error('Desktop pane missing'); if (await page.getByRole('button', {name:'.dolgorae'}).count()) throw Error('Private node is navigable'); }")
        cli(directory, "click", ref(snap, 'button "▸ docs"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "README.md"'))
        snap = snapshot(directory)
        assert "Preview of docs/README.md" in snap, snap
        cli(directory, "click", ref(snap, 'button "Compare HEAD and Working"'))
        snap = snapshot(directory)
        assert "Working docs/README.md" in snap and "not a Git repository" in snap, snap
        cli(directory, "click", ref(snap, 'button "Back to explorer"'))
        snap = snapshot(directory)
        assert "README.md" in snap and "docs" in snap, snap
        cli(directory, "resize", "390", "844")
        snap = snapshot(directory)
        cli(directory, "run-code", "async page => { const shown = await page.locator('.operator__pane').evaluateAll(nodes => nodes.map(node => getComputedStyle(node).display)); if (shown.filter(value => value !== 'none').length !== 1 || shown[0] === 'none') throw Error('Mobile navigation did not isolate Sessions'); }")
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.navigation = 1}); }")
        cli(directory, "click", ref(snap, 'button "Another session"'))
        snap = snapshot(directory)
        assert "Session selection is unavailable" in snap, snap
        cli(directory, "click", ref(snap, 'button "Another session"'))
        snap = snapshot(directory)
        assert "Another session" in snap, snap
        cli(directory, "run-code", "async page => { if (await page.getByRole('button', {name:'Chat'}).getAttribute('aria-current') !== 'page') throw Error('Session selection did not open Chat'); if ((await page.evaluate(() => window.fixture.navigation)) !== 2) throw Error('Session selection did not persist navigation'); }")
        cli(directory, "click", ref(snap, 'button "Files"'))
        snap = snapshot(directory)
        assert "README.md" in snap and "docs" in snap, snap
        cli(directory, "click", ref(snap, 'button "Sessions"'))
        snap = snapshot(directory)
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.navigation = 1}); }")
        cli(directory, "select", ref(snap, 'combobox "Workspace"'), "beta")
        snap = snapshot(directory)
        assert "Workspace selection is unavailable" in snap and "Alpha" in snap, snap
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.sessions = 1; window.fixture.faults.navigationDelayMs = 500}); await page.getByRole('combobox', {name:'Workspace'}).selectOption('beta'); if (!await page.getByRole('button', {name:'Another session'}).isDisabled()) throw Error('Session selection remained enabled during navigation'); await page.waitForFunction(() => document.querySelector('select[aria-label=Workspace]').value === 'beta' && !document.querySelector('select[aria-label=Workspace]').disabled); await page.evaluate(() => {window.fixture.faults.navigationDelayMs = 0}); }")
        snap = snapshot(directory)
        assert "Sessions are unavailable" in snap and "No sessions in this workspace" not in snap, snap
        cli(directory, "click", ref(snap, 'button "Retry sessions"'))
        snap = snapshot(directory)
        assert "Second session" in snap and "Sessions are unavailable" not in snap, snap
        cli(directory, "click", ref(snap, 'button "Files"'))
        snap = snapshot(directory)
        assert "beta.txt" in snap, snap
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.moreDelayMs = 500}); await page.getByRole('button', {name:'Load more files'}).click(); await page.getByRole('button', {name:'beta.txt'}).click(); await page.waitForTimeout(650); await page.getByRole('button', {name:'Back to explorer'}).click(); if (await page.getByRole('button', {name:'second.txt'}).count()) throw Error('Stale page appended after selection changed'); await page.evaluate(() => {window.fixture.faults.moreDelayMs = 0}); }")
        snap = snapshot(directory)
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.more = 1}); }")
        cli(directory, "click", ref(snap, 'button "Load more files"'))
        snap = snapshot(directory)
        assert "More files are unavailable" in snap and "second.txt" not in snap, snap
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.overlappingPage = 1}); }")
        cli(directory, "click", ref(snap, 'button "Load more files"'))
        snap = snapshot(directory)
        assert "More files are unavailable" not in snap, snap
        assert "second.txt" in snap, snap
        cli(directory, "run-code", "async page => { if (await page.getByRole('button', {name:'beta.txt', exact:true}).count() !== 1 || await page.getByRole('button', {name:'second.txt', exact:true}).count() !== 1) throw Error('Overlapping directory page duplicated an entry'); }")
        cli(directory, "click", ref(snap, 'button "beta.txt"'))
        snap = snapshot(directory)
        assert "Preview of beta.txt" in snap and "Git status is unavailable" in snap, snap
        cli(directory, "click", ref(snap, 'button "Back to explorer"'))
        snap = snapshot(directory)
        assert "second.txt" in snap, snap
        cli(directory, "click", ref(snap, 'button "beta.txt"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Sessions"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Files"'))
        snap = snapshot(directory)
        assert "Preview of beta.txt" in snap, snap
        cli(directory, "click", ref(snap, 'button "Sessions"'))
        snap = snapshot(directory)
        cli(directory, "select", ref(snap, 'combobox "Workspace"'), "alpha")
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Files"'))
        snap = snapshot(directory)
        assert "README.md" in snap and "docs" in snap, snap
        cli(directory, "click", ref(snap, 'button "Sessions"'))
        snap = snapshot(directory)
        cli(directory, "select", ref(snap, 'combobox "Workspace"'), "beta")
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Files"'))
        snap = snapshot(directory)
        assert "Preview of beta.txt" in snap, snap
        cli(directory, "click", ref(snap, 'button "Sessions"'))
        snap = snapshot(directory)
        cli(directory, "select", ref(snap, 'combobox "Workspace"'), "alpha")
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Files"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Refresh files"'))
        cli(directory, "run-code", "async page => { await page.waitForFunction(() => window.fixture.refresh === 1); const f = await page.evaluate(() => window.fixture); if (f.compare !== 1 || f.preview < 1 || f.directory < 3 || f.navigation !== 7) throw Error('Typed client integration lost'); }")
        snap = snapshot(directory)
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.preview = 1}); }")
        cli(directory, "click", ref(snap, 'button "Refresh files"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "README.md"'))
        snap = snapshot(directory)
        assert "File preview is unavailable" in snap and "Loading preview" not in snap, snap
        cli(directory, "click", ref(snap, 'button "Refresh files"'))
        snap = snapshot(directory)
        assert "Preview of docs/README.md" in snap and "File preview is unavailable" not in snap, snap
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.compare = 1}); }")
        cli(directory, "click", ref(snap, 'button "Compare HEAD and Working"'))
        snap = snapshot(directory)
        assert "Git review is unavailable" in snap and "Preview of docs/README.md" in snap, snap
        cli(directory, "click", ref(snap, 'button "Back to explorer"'))
        snap = snapshot(directory)
        assert "Git review is unavailable" not in snap, snap
        cli(directory, "click", ref(snap, 'button "README.md"'))
        snap = snapshot(directory)
        assert "Git review is unavailable" not in snap, snap
        cli(directory, "click", ref(snap, 'button "Back to explorer"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Parent directory"'))
        snap = snapshot(directory)
        assert "Git review is unavailable" not in snap, snap
        cli(directory, "click", ref(snap, 'button "▸ docs"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "README.md"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "Compare HEAD and Working"'))
        snap = snapshot(directory)
        assert "Working docs/README.md" in snap and "Git review is unavailable" not in snap, snap
        cli(directory, "run-code", """async page => {
          await page.evaluate(() => {window.fixture.faults.compare = 1; window.fixture.faults.compareDelayMs = 400});
          await page.getByRole('button', {name:'Compare HEAD and Working'}).click();
          await page.getByRole('button', {name:'Back to explorer'}).click();
          await page.getByRole('button', {name:'README.md'}).click();
          await page.waitForTimeout(550);
          if (await page.getByText('Git review is unavailable.').count()) throw Error('Late compare failure reached reopened preview');
          await page.evaluate(() => {window.fixture.faults.compareDelayMs = 0});
        }""")
        cli(directory, "run-code", """async page => {
          await page.evaluate(() => {window.fixture.faults.refresh = 1; window.fixture.faults.refreshDelayMs = 400});
          await page.getByRole('button', {name:'Refresh files'}).click();
          await page.getByRole('button', {name:'Back to explorer'}).click();
          await page.getByRole('button', {name:'Parent directory'}).click();
          await page.waitForTimeout(550);
          if (await page.getByText('File refresh is unavailable.').count()) throw Error('Late refresh failure reached new directory');
          await page.evaluate(() => {window.fixture.faults.refreshDelayMs = 0});
        }""")
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "▸ docs"'))
        snap = snapshot(directory)
        cli(directory, "click", ref(snap, 'button "README.md"'))
        snap = snapshot(directory)
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.refresh = 1}); }")
        cli(directory, "click", ref(snap, 'button "Refresh files"'))
        snap = snapshot(directory)
        assert "File refresh is unavailable" in snap and "Preview of docs/README.md" in snap, snap
        cli(directory, "click", ref(snap, 'button "Refresh files"'))
        snap = snapshot(directory)
        assert "File refresh is unavailable" not in snap and "Preview of docs/README.md" in snap, snap
        cli(directory, "click", ref(snap, 'button "Back to explorer"'))
        snap = snapshot(directory)
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.directory = 1}); }")
        cli(directory, "click", ref(snap, 'button "Parent directory"'))
        snap = snapshot(directory)
        assert "Directory is unavailable" in snap, snap
        cli(directory, "click", ref(snap, 'button "Refresh files"'))
        snap = snapshot(directory)
        assert "▸ docs" in snap and "Directory is unavailable" not in snap, snap
        cli(directory, "click", ref(snap, 'button "Sessions"'))
        snap = snapshot(directory)
        cli(directory, "run-code", "async page => { await page.evaluate(() => {window.fixture.faults.echoCurrentWorkspace = 1}); }")
        cli(directory, "select", ref(snap, 'combobox "Workspace"'), "beta")
        snap = snapshot(directory)
        assert "First session" in snap and "No sessions in this workspace" not in snap, snap
        cli(directory, "select", ref(snap, 'combobox "Workspace"'), "gamma")
        snap = snapshot(directory)
        assert "No sessions in this workspace" in snap and "Sessions are unavailable" not in snap, snap
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?stale-nav=1")
        snap = snapshot(directory)
        assert "First session" in snap and 'option "Hidden"' not in snap, snap
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?workspace-fault=1")
        snap = snapshot(directory)
        assert "Workspace navigation is unavailable" in snap, snap
        cli(directory, "click", ref(snap, 'button "Retry workspace load"'))
        snap = snapshot(directory)
        assert "First session" in snap and "Workspace navigation is unavailable" not in snap, snap
        print("PASS real Chrome: desktop panes, mobile navigation, file preview/review, denied node, context preservation, explicit refresh")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Sessions', exact:true}).click();
          await page.getByRole('combobox', {name:'Workspace'}).selectOption('beta');
          await page.getByRole('button', {name:'Files', exact:true}).click();
          await page.getByRole('button', {name:'beta.txt', exact:true}).waitFor();
          await page.evaluate(() => {window.fixture.faults.overlappingPage = 1; window.fixture.faults.overlappingDirectory = 1;});
          await page.getByRole('button', {name:'Load more files'}).click();
          await page.getByRole('button', {name:'▸ beta.txt', exact:true}).waitFor();
          const names = await page.locator('.file-pane__list button').allTextContents();
          if (names.join('|') !== '▸ beta.txt|second.txt') throw Error('Overlapping page lost latest metadata or first-seen row order');
        }""")
    finally:
        try:
            cli(directory, "close")
        finally:
            server.shutdown()
            thread.join()
