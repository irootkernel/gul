"""E8-T2 Chrome component check; uses injected auth, no account or deployment."""
import functools
import http.server
import json
import pathlib
import subprocess
import tempfile
import threading

ROOT = pathlib.Path(__file__).resolve().parents[2]
SESSION = "gul-e8-auth"


def cli(directory, *args):
    result = subprocess.run(["playwright-cli", f"-s={SESSION}", *args], cwd=directory, text=True, capture_output=True)
    assert result.returncode == 0 and "### Error" not in result.stdout, result.stdout + result.stderr


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


with tempfile.TemporaryDirectory(prefix="gul-e8-auth-") as directory:
    subprocess.run(["bun", "build", "frontend/browser/auth.tsx", "--target", "browser", "--outdir", directory], cwd=ROOT, check=True, capture_output=True)
    pathlib.Path(directory, "index.html").write_text('<!doctype html><html><body><div id="root"></div><script type="module" src="auth.js"></script></body></html>')
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(QuietHandler, directory=directory))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = f"http://127.0.0.1:{server.server_port}/"
    try:
        cli(directory, "open", url + "?case=remote")
        cli(directory, "run-code", """async page => {
          await page.getByText('Open Gul on this Mac').waitFor();
          if (await page.locator('form,input,button').count() || await page.getByText('Protected operator').count()) throw Error('Remote setup/product exposed');
        }""")
        cli(directory, "goto", url + "?case=unavailable")
        cli(directory, "run-code", """async page => {
          await page.getByRole('alert').filter({hasText:'authentication is unavailable'}).waitFor();
          if ((await page.locator('body').innerText()).includes('private') || await page.getByText('Protected operator').count()) throw Error('Failed auth exposed details/product');
        }""")
        for scenario in ("setup", "setup-denied", "setup-rate", "success", "credentials", "rate", "failure", "pending", "ime", "logout-failure", "logout-pending"):
            cli(directory, "goto", url + "?case=" + scenario)
            cli(directory, "run-code", f"""async page => {{
              const input = page.getByLabel('Password', {{exact:true}});
              const password = '한글 암호 😀 exact  spaces';
              await input.fill(password);
              if ({json.dumps(scenario == 'ime')}) {{
                await input.dispatchEvent('compositionstart', {{data:'ㅎ'}});
                await page.locator('form').evaluate(form => form.requestSubmit());
                await input.dispatchEvent('keydown', {{key:'Enter', code:'Enter', isComposing:true, keyCode:229}});
                await input.dispatchEvent('compositionend', {{data:'한글'}});
                await page.locator('form').evaluate(form => form.requestSubmit());
                if (await page.evaluate(() => window.fixture.calls) !== 0) throw Error('IME logged in');
                await input.dispatchEvent('keyup', {{key:'Enter', code:'Enter'}});
              }}
              await page.getByRole('button', {{name:{json.dumps('Set password' if scenario.startswith('setup') else 'Sign in')}, exact:true}}).click();
              await page.waitForFunction(() => window.fixture.calls === 1);
              if (!await page.evaluate(() => window.fixture.matched)) throw Error('Password changed');
              if ({json.dumps(scenario in ('setup-denied', 'setup-rate'))}) {{
                await page.getByRole('heading', {{name:'Set your Gul password'}}).waitFor({{timeout:2000}});
                const message = {json.dumps('Too many setup attempts' if scenario == 'setup-rate' else 'Reopen Gul locally')};
                await page.getByRole('alert').filter({{hasText:message}}).waitFor({{timeout:2000}});
                if (await input.inputValue() !== '' || !await input.evaluate(element => document.activeElement === element) || await page.getByRole('heading', {{name:'Sign in to Gul'}}).count()) throw Error('Setup failure lost form or focus');
                return;
              }}
              if ({json.dumps(scenario == 'setup')}) {{
                await page.getByRole('heading', {{name:'Sign in to Gul'}}).waitFor();
                if (await input.inputValue() !== '') throw Error('Setup password retained');
                return;
              }}
              if ({json.dumps(scenario == 'pending')}) {{
                if (await input.inputValue() !== '' || !await input.isDisabled()) throw Error('Pending password retained');
                await page.locator('form').evaluate(form => form.dispatchEvent(new Event('submit', {{bubbles:true,cancelable:true}})));
                if (await page.evaluate(() => window.fixture.calls) !== 1) throw Error('Duplicate login');
                await page.evaluate(() => window.fixture.resolve());
              }}
              if ({json.dumps(scenario in ('credentials', 'rate', 'failure'))}) {{
                const message = {json.dumps(dict(credentials='password is incorrect', rate='Too many sign-in', failure='Sign-in is unavailable').get(scenario, ''))};
                await page.getByRole('alert').filter({{hasText:message}}).waitFor();
                if (await input.inputValue() !== '' || !await input.evaluate(element => document.activeElement === element)) throw Error('Failure input/focus incorrect');
              }} else {{
                await page.getByText('Protected operator', {{exact:true}}).waitFor();
                if (await page.locator('input').count()) throw Error('Password survived login');
                if ({json.dumps(scenario == 'logout-pending')}) {{
                  await page.getByRole('button', {{name:'Sign out', exact:true}}).click();
                  await page.getByRole('status').filter({{hasText:'Signing out'}}).waitFor();
                  if (await page.locator('input,button').count() || await page.getByText('Protected operator').count()) throw Error('Pending logout allowed content or a racing login');
                  await page.evaluate(() => window.fixture.resolve());
                }} else if ({json.dumps(scenario == 'logout-failure')}) {{
                  await page.getByRole('button', {{name:'Sign out', exact:true}}).click();
                  await page.getByRole('alert').filter({{hasText:'Sign-out could not be confirmed'}}).waitFor();
                  if (await page.getByText('Protected operator').count()) throw Error('Product survived uncertain logout');
                  await page.getByRole('button', {{name:'Retry sign-out'}}).click();
                  await page.waitForFunction(() => window.fixture.logoutCalls === 2);
                }} else {{
                  await page.evaluate(() => window.fixture.expire());
                }}
                await page.getByRole('heading', {{name:'Sign in to Gul'}}).waitFor();
                if (await page.getByText('Protected operator').count()) throw Error('Product survived expiry/logout');
              }}
              const text = await page.locator('body').innerText();
              if (text.includes(password) || text.includes('private') || await page.evaluate(() => localStorage.length + sessionStorage.length) !== 0) throw Error('Secret/diagnostic stored or displayed');
            }}""")
        print("PASS Chrome E8-T2: remote setup refusal, setup-to-login and failed-setup form/focus/rate recovery, exact Unicode, pending clear/duplicates, sanitized login/rate failures, focus/IME, expiry/logout product removal, uncertain logout retry and no storage")
    finally:
        try:
            cli(directory, "close")
        finally:
            server.shutdown()
            server.server_close()
            thread.join()
