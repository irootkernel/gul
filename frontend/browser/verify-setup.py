"""E8-T1 component check in existing Chrome; installs nothing and uses no real account."""
import functools
import http.server
import json
import pathlib
import subprocess
import tempfile
import threading

ROOT = pathlib.Path(__file__).resolve().parents[2]
SESSION = "gul-e8-setup"


def cli(directory, *args):
    result = subprocess.run(["playwright-cli", f"-s={SESSION}", *args], cwd=directory, text=True, capture_output=True)
    assert result.returncode == 0 and "### Error" not in result.stdout, result.stdout + result.stderr


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


with tempfile.TemporaryDirectory(prefix="gul-e8-setup-") as directory:
    subprocess.run(["bun", "build", "frontend/browser/setup.tsx", "--target", "browser", "--outdir", directory],
                   cwd=ROOT, check=True, capture_output=True)
    pathlib.Path(directory, "index.html").write_text('<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><div id="root"></div><script type="module" src="setup.js"></script></body></html>')
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(QuietHandler, directory=directory))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = f"http://127.0.0.1:{server.server_port}/"
    try:
        cli(directory, "open", url + "?case=remote")
        cli(directory, "snapshot")
        cli(directory, "run-code", """async page => {
          await page.getByText('Open Gul on this Mac').waitFor();
          if (await page.locator('form,input,button').count() || await page.evaluate(() => window.fixture.calls) !== 0)
            throw Error('Remote setup exposed');
        }""")
        for scenario in ("success", "failure", "unavailable", "already-configured", "pending", "invalid", "oversized", "ime"):
            cli(directory, "goto", url + "?case=" + scenario)
            cli(directory, "snapshot")
            cli(directory, "run-code", f"""async page => {{
              const password = '한글 암호 😀 exact  spaces';
              const input = page.getByLabel('Password', {{exact:true}});
              await input.fill({'"short"' if scenario == 'invalid' else '"a".repeat(1025)' if scenario == 'oversized' else 'password'});
              if ({json.dumps(scenario == 'ime')}) {{
                await input.dispatchEvent('compositionstart', {{data:'ㅎ'}});
                await page.locator('form').evaluate(form => form.requestSubmit());
                await input.dispatchEvent('keydown', {{key:'Enter', code:'Enter', isComposing:true, keyCode:229}});
                await input.dispatchEvent('compositionend', {{data:'한글'}});
                await page.locator('form').evaluate(form => form.requestSubmit());
                if (await page.evaluate(() => window.fixture.calls) !== 0 || await input.inputValue() !== password)
                  throw Error('Composition submitted or changed the password');
                await input.dispatchEvent('keyup', {{key:'Enter', code:'Enter'}});
              }}
              await page.getByRole('button', {{name:'Set password', exact:true}}).click();
              if ({json.dumps(scenario in ('invalid', 'oversized'))}) {{
                await page.getByRole('alert').filter({{hasText:'Use at least 15'}}).waitFor();
                if (await page.evaluate(() => window.fixture.calls) !== 0 || await input.inputValue() !== '')
                  throw Error('Invalid password sent or retained');
                return;
              }}
              await page.waitForFunction(() => window.fixture.calls === 1);
              if (!await page.evaluate(() => window.fixture.matched)) throw Error('Password bytes changed');
              if ({json.dumps(scenario == 'pending')}) {{
                if (await input.inputValue() !== '' || !await input.isDisabled()) throw Error('Pending password retained');
                await page.locator('form').evaluate(form => form.dispatchEvent(new Event('submit', {{bubbles:true,cancelable:true}})));
                if (await page.evaluate(() => window.fixture.calls) !== 1) throw Error('Duplicate setup');
                await page.evaluate(() => window.fixture.resolve());
              }}
              if ({json.dumps(scenario in ('failure', 'unavailable', 'already-configured'))}) {{
                const message = {dict(failure='Setup could not finish', unavailable='cannot safely access its account data', **{'already-configured':'already has a password'}).get(scenario, '')!r};
                await page.getByRole('alert').filter({{hasText:message}}).waitFor();
                if (await input.inputValue() !== '') throw Error('Failed password retained');
                if (!await input.evaluate(element => document.activeElement === element)) throw Error('Failure did not restore password focus');
              }} else {{
                await page.getByRole('status').filter({{hasText:'password is configured'}}).waitFor();
                if (await page.locator('form,input').count()) throw Error('Setup form survived success');
              }}
              const text = await page.locator('body').innerText();
              if (text.includes(password) || text.includes('private diagnostic') ||
                  await page.evaluate(() => localStorage.length + sessionStorage.length) !== 0)
                throw Error('Password or diagnostic leak');
            }}""")
        print("PASS Chrome E8-T1: remote refusal, exact Unicode, minimum/maximum bounds, pending duplicate suppression, password clearing, sanitized/category failures with focus recovery, IME and no browser storage")
    finally:
        try:
            cli(directory, "close")
        finally:
            server.shutdown()
            server.server_close()
            thread.join()
