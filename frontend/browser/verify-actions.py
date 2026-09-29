"""Selected real-Chrome check; requires existing Bun and playwright-cli, installs nothing."""
import functools
import http.server
import json
import pathlib
import re
import subprocess
import tempfile
import threading

ROOT = pathlib.Path(__file__).resolve().parents[2]
SESSION = "gul-e4-actions"


def cli(directory, *args):
    result = subprocess.run(["playwright-cli", f"-s={SESSION}", *args], cwd=directory, text=True, capture_output=True)
    assert result.returncode == 0 and "### Error" not in result.stdout, result.stdout + result.stderr
    return result.stdout


def snapshot(directory):
    output = cli(directory, "snapshot")
    match = re.search(r"\[Snapshot\]\(([^)]+)\)", output)
    assert match, output
    return (pathlib.Path(directory) / match.group(1)).read_text()


def ref(snap, text):
    lines = [line for line in snap.splitlines() if text in line]
    assert len(lines) == 1, (text, lines)
    match = re.search(r"\[ref=(e\d+)\]", lines[0])
    assert match, lines[0]
    return match.group(1)


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


with tempfile.TemporaryDirectory(prefix="gul-actions-browser-") as directory:
    subprocess.run(["bun", "build", "frontend/browser/actions.tsx", "--target", "browser", "--outdir", directory], cwd=ROOT, check=True)
    pathlib.Path(directory, "index.html").write_text('<!doctype html><html><body><div id="root"></div><script type="module" src="actions.js"></script></body></html>')
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(QuietHandler, directory=directory))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = f"http://127.0.0.1:{server.server_port}/"
    try:
        cli(directory, "open", url + "?case=approval")
        for case, label in [("approval", "Approve once"), ("denial", "Decline"), ("answer", "Send answer"), ("expiration", None), ("cancellation", "Cancel"), ("provider_failure", "Approve once")]:
            cli(directory, "goto", url + "?case=" + case)
            snap = snapshot(directory)
            if case == "answer":
                inputs = re.findall(r'textbox "Answer for question [12]: (?:Prototype|Reset) question" \[ref=(e\d+)\]', snap)
                assert len(inputs) == 2, snap
                cli(directory, "fill", inputs[0], "first-fixture")
                cli(directory, "fill", inputs[1], "second-fixture")
            if label:
                cli(directory, "click", ref(snap, 'button "' + label + '"'))
            snapshot(directory)
            cli(directory, "run-code", f'''async page => {{
              await page.locator('[data-outcome="{case}"]').waitFor();
              const got = await page.evaluate(() => ({{...window.fixture, empty: [...document.querySelectorAll('input')].every(input => input.value === ''), disabled: [...document.querySelectorAll('button')].every(button => button.disabled), local: localStorage.length, session: sessionStorage.length}}));
              if (got.calls !== {0 if case == "expiration" else 1} || !got.disabled || got.local || got.session) throw Error('Outcome admission or storage regression');
              if ({json.dumps(case)} === 'answer' && (!got.matched || !got.empty || JSON.stringify(got.keys) !== '["__proto__","reset"]')) throw Error('Answer regression');
              if ({json.dumps(case)} !== 'expiration') await page.waitForFunction(() => window.fixture.cleared);
              if ((await page.locator('body').innerText()).includes('private diagnostic')) throw Error('Diagnostic leak');
            }}''')
        for case in ["writer", "busy", "prompt", "prompt_write_busy", "prompt_write_unsupported", "prompt_write_unknown"]:
            cli(directory, "goto", url + "?case=" + case)
            snap = snapshot(directory)
            if case.startswith("prompt_write_"):
                cli(directory, "fill", ref(snap, 'textbox "Prompt"'), "retained write draft")
                cli(directory, "click", ref(snap, 'button "Send prompt"'))
                snapshot(directory)
                expected = {"prompt_write_busy": "Writer busy.", "prompt_write_unsupported": "cannot change access", "prompt_write_unknown": "previous outcome is unresolved"}[case]
                cli(directory, "run-code", f'''async page => {{
                  const text = await page.locator('body').innerText();
                  if (!text.includes({json.dumps(expected)}) || text.includes('private diagnostic') || await page.evaluate(() => window.fixture.calls) !== 1 || await page.getByRole('textbox').inputValue() !== 'retained write draft' || !await page.getByRole('button', {{name:'Send prompt', exact:true}}).isDisabled()) throw Error('Typed prompt failure or retry barrier lost');
                }}''')
                continue
            if case == "prompt":
                cli(directory, "fill", ref(snap, 'textbox "Prompt"'), "explicit draft")
                cli(directory, "click", ref(snap, 'button "Observe terminal Turn"'))
                snap = snapshot(directory)
                cli(directory, "run-code", "async page => { if (await page.evaluate(() => window.fixture.calls) !== 0 || await page.getByRole('textbox').inputValue() !== 'explicit draft' || await page.getByRole('checkbox').isChecked()) throw Error('Queued prompt or implicit consent'); }")
                cli(directory, "click", ref(snap, 'checkbox "I confirm'))
                cli(directory, "click", ref(snap, 'button "Send prompt"'))
            else:
                label = "Release writer" if case == "writer" else "Acquire writer"
                cli(directory, "click", ref(snap, 'button "' + label + '"'))
            snapshot(directory)
            cli(directory, "run-code", f'''async page => {{
              if (await page.evaluate(() => window.fixture.calls) !== 1) throw Error('Mutation count');
              const text = await page.locator('body').innerText();
              if ({json.dumps(case)} === 'writer' && !text.includes('Unowned')) throw Error('Unowned window hidden');
              if ({json.dumps(case)} === 'busy' && (!text.includes('Writer busy.') || text.includes('private diagnostic'))) throw Error('Typed conflict lost');
              if ({json.dumps(case)} !== 'prompt' && await page.getByRole('button').filter({{hasText: /writer$/}}).evaluateAll(buttons => buttons.some(button => !button.disabled))) throw Error('Blind retry allowed');
              if ({json.dumps(case)} === 'prompt' && (!await page.evaluate(() => window.fixture.consent) || await page.getByRole('textbox').inputValue() !== '')) throw Error('Explicit submit/consent failed');
            }}''')
        cli(directory, "goto", url + "?case=artifact")
        rendered = snapshot(directory)
        assert "Verified result" in rendered and "한글 原文" in rendered, rendered
        cli(directory, "run-code", """async page => {
          if (await page.locator('article script, article img, article iframe, article a, article object, article embed').count()) throw Error('Active content rendered');
          if (await page.evaluate(() => window.pwned !== undefined)) throw Error('Script executed');
          if (await page.evaluate(() => performance.getEntriesByType('resource').some(entry => /example.invalid|file:/.test(entry.name)))) throw Error('External resource loaded');
          if (!(await page.locator('article').innerText()).includes('../../etc/passwd')) throw Error('Opaque path text lost');
        }""")
        print("PASS real Chrome: six outcomes; one-shot clearing; writer release window and typed conflict; explicit draft submission and interruption consent; typed WRITE submission failures; inert artifact Markdown")
    finally:
        cli(directory, "close")
        server.shutdown()
        server.server_close()
