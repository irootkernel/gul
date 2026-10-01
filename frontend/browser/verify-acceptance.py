#!/usr/bin/env python3
"""Exercise the checked entry against the explicitly injected assembled fake host."""
import importlib.util
import json
import os
import pathlib
import select
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True

ROOT = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("delivery", pathlib.Path(__file__).with_name("verify-delivery.py"))
delivery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(delivery)


def receive(process):
    if not select.select([process.stdout], [], [], 25)[0]:
        raise RuntimeError("Acceptance driver timed out")
    line = process.stdout.readline()
    if not line:
        raise RuntimeError("Acceptance driver exited: " + process.stderr.read())
    return json.loads(line)


def command(process, value):
    process.stdin.write(value + "\n")
    process.stdin.flush()
    result = receive(process)
    if not result.get("ok"):
        raise RuntimeError(f"Fixture command {value}: {result}")
    return result



def verify_unknown_draft(binary, work):
    port = delivery.free_port()
    workspace = work / "UnknownWorkspace"
    workspace.mkdir(mode=0o700)
    process = subprocess.Popen([str(binary), str(work / "UnknownGul"), str(workspace), str(port)], cwd=ROOT,
        env={**os.environ, "GUL_RUN_ASSEMBLED_ACCEPTANCE": "1"}, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    config = work / "unknown-playwright-cli.json"
    browser = f"g14unknown{port}"
    opened = False
    try:
        origin = receive(process)["origin"]
        pin = delivery.certificate_spki_pin(port)
        config.write_text(json.dumps({"browser": {"browserName": "chromium", "isolated": False, "userDataDir": str(work / "unknown-chrome-profile"),
            "launchOptions": {"channel": "chrome", "args": [f"--ignore-certificate-errors-spki-list={pin}"]}}}))
        command(process, "initialize")
        delivery.cli(config, browser, "open", origin)
        opened = True
        run = lambda code: delivery.cli(config, browser, "run-code", "async page => {page.setDefaultTimeout(20000);" + code + "}")
        run("""
          await page.getByLabel('Password', {exact:true}).fill('assembled fixture password 2026');
          await page.getByRole('button', {name:'Sign in', exact:true}).click();
          await page.getByRole('main', {name:'Gul operator'}).waitFor();
          await page.getByRole('button', {name:'Session', exact:true}).click();
          await page.getByLabel('Prompt', {exact:true}).evaluate(element => {Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(element, 'a'.repeat(1048577)); element.dispatchEvent(new Event('input', {bubbles:true}));});
          await page.getByText('Prompt exceeds the 1,048,576 byte limit. Shorten it before sending.', {exact:true}).waitFor();
          if (!await page.getByRole('button', {name:'Send prompt'}).isDisabled()) throw Error('Oversized prompt could send');
          await page.getByLabel('Prompt', {exact:true}).fill('unknown draft remains');
          await page.waitForFunction(() => {const b = [...document.querySelectorAll('button')].find(b => b.textContent === 'Send prompt'); return b && !b.disabled;});
        """)
        if command(process, "unknown-submit")["submitCalls"] != 0:
            raise AssertionError("Oversized draft was dispatched")
        run("""
          const reply = page.waitForResponse(r => r.url().endsWith('.DirectSessionService/Submit'));
          await page.getByRole('button', {name:'Send prompt'}).click();
          const response = await reply; if (response.status() !== 200) throw Error('Unknown request failed before effect');
          await page.getByText('The previous outcome is unresolved.', {exact:true}).first().waitFor();
          if (await page.getByLabel('Prompt', {exact:true}).inputValue() !== 'unknown draft remains') throw Error('Unknown outcome discarded draft');
          if (!await page.getByRole('button', {name:'Send prompt'}).isDisabled()) throw Error('Unknown outcome admitted another send');
        """)
        if command(process, "restart")["submitCalls"] != 1:
            raise AssertionError("Unknown Submit replayed at restart")
        run("""
          await page.reload();
          await page.getByRole('main', {name:'Gul operator'}).waitFor();
          await page.getByRole('button', {name:'Session', exact:true}).click();
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.waitForFunction(() => document.querySelectorAll('[aria-label="Prompt History"] li').length === 1);
        """)
        if command(process, "complete")["submitCalls"] != 1:
            raise AssertionError("Unknown Submit replayed on convergence")
    finally:
        if opened:
            delivery.cli(config, browser, "close")
        if process.poll() is None:
            process.stdin.write("stop\n")
            process.stdin.flush()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
        if process.returncode:
            raise RuntimeError("Unknown acceptance host failed: " + process.stderr.read())


def main():
    with tempfile.TemporaryDirectory(prefix="gul-e14-acceptance-") as temporary:
        work = pathlib.Path(temporary).resolve()
        workspace = work / "Workspace"
        workspace.mkdir(mode=0o700)
        (workspace / "README.md").write_text("# Acceptance workspace\n\nSafe file original.\n")
        private = workspace / ".dolgorae"
        private.mkdir()
        (private / "secret.txt").write_text("never expose this fixture secret")
        binary = work / "acceptance"
        subprocess.run(["go", "build", "-o", str(binary), "./test/acceptance"], cwd=ROOT, check=True, timeout=180)
        denied = subprocess.run([str(binary)], cwd=ROOT, capture_output=True, env={k: v for k, v in os.environ.items() if k != "GUL_RUN_ASSEMBLED_ACCEPTANCE"})
        if denied.returncode == 0 or b"opt-in isolated test fixture" not in denied.stderr:
            raise AssertionError("Uninjected acceptance executable ran")
        port = delivery.free_port()
        process = subprocess.Popen([str(binary), str(work / "Gul"), str(workspace), str(port)], cwd=ROOT,
            env={**os.environ, "GUL_RUN_ASSEMBLED_ACCEPTANCE": "1"}, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        config = work / "playwright-cli.json"
        browser = f"g14{port}"
        opened = False
        try:
            origin = receive(process)["origin"]
            pin = delivery.certificate_spki_pin(port)
            config.write_text(json.dumps({"browser": {"browserName": "chromium", "isolated": False, "userDataDir": str(work / "chrome-profile"),
                "launchOptions": {"channel": "chrome", "args": [f"--ignore-certificate-errors-spki-list={pin}"]}}}))
            run = lambda code: delivery.cli(config, browser, "run-code", "async page => {page.setDefaultTimeout(20000);" + code + "}")
            delivery.cli(config, browser, "open", origin)
            opened = True
            delivery.cli(config, browser, "snapshot")
            run("""
              await page.getByText('Open Gul on this Mac to set your password.').waitFor();
              if (await page.getByRole('form', {name:'Set your Gul password'}).count()) throw Error('Browser gained setup authority');
            """)
            command(process, "initialize")
            run("""
              await page.reload();
              await page.getByLabel('Password', {exact:true}).fill('assembled fixture password 2026');
              await page.getByRole('button', {name:'Sign in', exact:true}).click();
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              await page.getByText('Launch compatibility', {exact:true}).click();
              await page.getByRole('form', {name:'Session configuration'}).waitFor();
              await page.getByLabel(/^Runtime Profile/).selectOption({index:1});
              await page.getByLabel(/^Model/).selectOption({index:1});
              await page.getByLabel(/^Effort/).selectOption({index:1});
              await page.getByLabel(/^Execution lane/).selectOption({label:'Dedicated'});
              await page.getByLabel(/^Required assurance/).selectOption({label:'BEST_EFFORT_PERSONAL_ALPHA'});
              await page.getByLabel(/^Specialist Policy/).selectOption('preprovisioned');
              await page.getByRole('button', {name:'Check configuration', exact:true}).click();
              await page.getByRole('status').filter({hasText:'Configuration is compatible. Session creation requires the runtime integration.'}).waitFor();
              await page.getByText('Launch compatibility', {exact:true}).click();
              await page.getByRole('button', {name:'Session', exact:true}).click();
              await page.getByRole('button', {name:'Send prompt'}).waitFor();
              await page.getByLabel('Prompt', {exact:true}).fill('same\\r\\n한글');
              await page.waitForFunction(() => {const button = [...document.querySelectorAll('button')].find(b => b.textContent === 'Send prompt'); return button && !button.disabled;});
              await page.getByRole('button', {name:'Send prompt'}).click();
              await page.waitForFunction(() => document.querySelector('textarea')?.value === '');
              await page.getByLabel('Prompt', {exact:true}).fill('busy draft stays');
              if (!await page.getByRole('button', {name:'Send prompt'}).isDisabled()) throw Error('Busy prompt could send');
            """)
            command(process, "approval")
            run("""
              await page.getByRole('heading', {name:'Fixture approval'}).waitFor();
              await page.getByRole('button', {name:'Approve once'}).click();
              await page.getByText('No pending interaction requests.').waitFor();
              if (await page.getByLabel('Prompt', {exact:true}).inputValue() !== 'busy draft stays') throw Error('Refresh discarded busy draft');
            """)
            command(process, "complete")
            run("""
              await page.waitForFunction(() => {const button = [...document.querySelectorAll('button')].find(b => b.textContent === 'Send prompt'); return button && !button.disabled;});
              await page.getByLabel('Prompt', {exact:true}).fill('same\\r\\n한글');
              await page.getByRole('button', {name:'Send prompt'}).click();
              await page.waitForFunction(() => document.querySelector('textarea')?.value === '');
            """)
            command(process, "complete")
            run("""
              await page.getByLabel('Prompt', {exact:true}).evaluate(element => {Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(element, '긴 입력\\n'.repeat(40000)); element.dispatchEvent(new Event('input', {bubbles:true}));});
              await page.waitForFunction(() => {const button = [...document.querySelectorAll('button')].find(b => b.textContent === 'Send prompt'); return button && !button.disabled;});
              const reply = page.waitForResponse(r => r.url().endsWith('.DirectSessionService/Submit'));
              await page.getByRole('button', {name:'Send prompt'}).click();
              const response = await reply; const body = await response.text(); if (response.status() !== 200) throw Error('Long submit '+response.status()+' '+body); try {await page.waitForFunction(() => document.querySelector('textarea')?.value === '');} catch {throw Error('Long submit response: '+body+'; draft length '+await page.getByLabel('Prompt', {exact:true}).inputValue().then(v=>v.length));}
              await page.waitForFunction(() => document.querySelector('textarea')?.value === '');
            """)
            command(process, "complete")
            command(process, "restart")
            run("""
              await page.reload();
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              await page.getByRole('button', {name:'Prompt History', exact:true}).click();
              await page.getByLabel('Prompt History').waitFor();
            """)
            # Inspect the real panel before asserting its exact original controls.
            delivery.cli(config, browser, "snapshot")
            run("""
              const history = page.getByLabel('Prompt History', {exact:true});
              await page.waitForFunction(() => document.querySelectorAll('[aria-label="Prompt History"] li').length === 3);
              const previews = await history.locator('li').allTextContents();
              if (previews[0].includes('never queued') || previews.some(p => p.includes('busy draft stays'))) throw Error('Busy draft entered accepted history');
              await history.getByRole('button', {name:/View full original prompt/}).last().click();
              const original = history.locator('pre');
              await original.waitFor();
              if ((await original.textContent()) !== '긴 입력\\n'.repeat(40000)) throw Error('Large original changed after restart');
            """)
            command(process, "disconnect")
            run("""
              await page.reload();
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              await page.getByRole('button', {name:'README.md', exact:true}).click();
              await page.getByText('Safe file original.', {exact:false}).waitFor();
              if (await page.getByText('secret.txt', {exact:true}).count()) throw Error('Provider private tree exposed');
            """)
            command(process, "reconnect")
            command(process, "stream-loss")
            run("""
              // Hold rejoin until a real provider publication has occurred in the gap.
              let firstConnection = true;
              await page.route('**/gul.v1.ClientEventService/WatchClientEvents', async route => {
                if (firstConnection) {firstConnection = false; await route.fulfill({status:503, contentType:'application/json', body:'{"code":"unavailable","message":"fixture stream loss"}'}); return;}
                await page.waitForFunction(() => document.documentElement.dataset.acceptanceResume === 'yes');
                await route.continue();
              });
              await page.reload();
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              await page.getByRole('button', {name:'Session', exact:true}).click();
              await page.getByText('Observation disconnected. Checking fresh state before reconnect.', {exact:true}).waitFor();
              if (!await page.getByRole('button', {name:'Send prompt'}).isDisabled()) throw Error('Disconnected browser admitted submit');
              if (await page.getByLabel('Specialist results').locator('li').count()) throw Error('Unexpected result before the gap');
              if (await page.getByText('Checking current state…', {exact:true}).getAttribute('role') !== 'status') throw Error('Pending eligibility announced an alert');
              await page.evaluate(() => {globalThis.acceptanceContentReads = {};});
              await page.route('**/gul.v1.DirectSessionService/List*', async route => {
                const method = route.request().url().split('/').pop();
                if (['ListConversation', 'ListPromptHistory', 'ListSpecialistResults'].includes(method)) {
                  await page.evaluate(method => {const reads = globalThis.acceptanceContentReads; if (reads) reads[method] = (reads[method] || 0) + 1;}, method);
                }
                await route.continue();
              });
            """)
            command(process, "results")
            run("""
              await page.waitForTimeout(4000);
              if (await page.getByRole('button', {name:'Reviewer · 1', exact:true}).count()) throw Error('Fixture did not isolate the event gap');
              await page.evaluate(() => {globalThis.acceptanceContentReads = {}; document.documentElement.dataset.acceptanceResume = 'yes';});
              await page.getByText('Observation disconnected. Checking fresh state before reconnect.', {exact:true}).waitFor({state:'hidden'});
              await page.getByRole('button', {name:'Reviewer · 1', exact:true}).waitFor();
              try {await page.waitForFunction(() => ['ListConversation', 'ListPromptHistory', 'ListSpecialistResults'].every(method => globalThis.acceptanceContentReads[method] > 0));}
              catch (error) {throw Error('Content did not catch up: ' + JSON.stringify(await page.evaluate(() => globalThis.acceptanceContentReads)), {cause:error});}
              await page.getByRole('button', {name:'Reviewer · 1', exact:true}).click();
              await page.getByLabel('Specialist result original').waitFor();
              const body = await page.getByLabel('Specialist result original').textContent();
              if (body !== 'Specialist original\\r\\n한글 <script>inert</script>') throw Error('Result original changed');
              if (await page.getByLabel('Specialist results').locator('script').count()) throw Error('Result script executed');
              await page.waitForFunction(() => {const b = [...document.querySelectorAll('button')].find(b => b.textContent === 'Request whole-session close'); return b && !b.disabled;});
              await page.getByRole('button', {name:'Request whole-session close'}).click();
              await page.getByText('Close request accepted; provider settlement pending.').waitFor();
              if (await page.getByText('Whole-session close confirmed by current provider projection.').count()) throw Error('Close prematurely confirmed');
            """)
            run("await page.waitForTimeout(6500); if (!await page.getByRole('button', {name:'Request whole-session close'}).isDisabled()) throw Error('Pending close became available');")
            result = command(process, "settle-close")
            if result["closeCalls"] != 1:
                raise AssertionError("Close invoked more than the root once")
            run("""
              await page.getByText('Whole-session close confirmed by current provider projection.').waitFor();
              await page.getByRole('button', {name:'Prompt History', exact:true}).click();
              await page.waitForFunction(() => document.querySelectorAll('[aria-label="Prompt History"] li').length === 3);
              const storage = await page.evaluate(async () => ({cache:await caches.keys(), local:localStorage.length, session:Object.keys(sessionStorage)}));
              if (storage.cache.length || storage.local || storage.session.some(k => k !== 'gul.presentation.tab')) throw Error('Operator material persisted in browser storage');
            """)
        except Exception:
            if opened:
                print(delivery.cli(config, browser, "run-code", "async page => {return (await page.locator('body').innerText()).slice(0,4000);}"))
                print(delivery.cli(config, browser, "console"))
            raise
        finally:
            if opened:
                delivery.cli(config, browser, "close")
            if process.poll() is None:
                process.stdin.write("stop\n")
                process.stdin.flush()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
            if process.returncode:
                raise RuntimeError("Acceptance host failed: " + process.stderr.read())
        verify_unknown_draft(binary, work)
    print("PASS assembled Chrome: real TLS/account/session/CSRF clients, sequential and busy prompts, waiting approval, verified originals/results, Gul restart, offline files, automatic browser/provider reconnect, unknown draft retention without replay, bounded prompt, aggregate root close; fake provider only")


if __name__ == "__main__":
    main()
