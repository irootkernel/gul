"""Exercise E7 session presentation with typed fake clients in real Chrome."""
import functools
import http.server
import pathlib
import subprocess
import tempfile
import threading

ROOT = pathlib.Path(__file__).resolve().parents[2]
SESSION = "gul-e7-activity"


def cli(directory, *args):
    result = subprocess.run(["playwright-cli", f"-s={SESSION}", *args], cwd=directory, text=True, capture_output=True)
    assert result.returncode == 0 and "### Error" not in result.stdout, result.stdout + result.stderr
    return result.stdout


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


with tempfile.TemporaryDirectory(prefix="gul-e7-activity-") as directory:
    subprocess.run(["bun", "build", "frontend/browser/activity.tsx", "--target", "browser", "--outdir", directory], cwd=ROOT, check=True)
    pathlib.Path(directory, "index.html").write_text('<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="activity.css"></head><body><div id="root"></div><script type="module" src="activity.js"></script></body></html>')
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(QuietHandler, directory=directory))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        cli(directory, "open", f"http://127.0.0.1:{server.server_port}/")
        cli(directory, "resize", "390", "844")
        cli(directory, "run-code", """async page => {
          await page.getByRole('heading', {name:'Approve command'}).waitFor();
          const action = await page.getByRole('region', {name:'Action required'}).boundingBox();
          const timeline = await page.getByRole('region', {name:'Conversation timeline'}).boundingBox();
          if (!action || !timeline || action.y >= timeline.y || action.y >= 844) throw Error('Approval priority or mobile reachability lost');
          const body = await page.locator('body').innerText();
          if (!body.includes('ACTIVE') || !body.includes('Connected') || !body.includes('READ_ONLY') || !body.includes('USER_APPROVAL_REQUIRED')) throw Error('Projected activity missing');
          if (!body.includes('final preview') || body.includes('Complete final answer')) throw Error('Conversation preview incorrect');
          if (await page.getByRole('region', {name:'Conversation timeline'}).locator('ol li').count() !== 2) throw Error('Conversation page order incorrect');
        }""")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'View full response'}).click();
          await page.getByText('Complete final answer').waitFor();
          await page.getByRole('button', {name:'More conversation'}).click();
          await page.waitForFunction(() => document.querySelectorAll('[aria-label="Conversation timeline"] ol li').length === 3);
          if (await page.getByRole('region', {name:'Conversation timeline'}).locator('ol li').count() !== 3) throw Error('Chronological pagination lost');
          await page.getByRole('button', {name:'Refresh conversation'}).click();
          await page.waitForFunction(() => document.querySelectorAll('[aria-label="Conversation timeline"] ol li').length === 2);
          if (await page.getByText('Complete final answer').count()) throw Error('Local original survived provider refresh');
          await page.getByRole('button', {name:'View full response'}).click();
          await page.getByText('Complete final answer').waitFor();
          await page.getByRole('button', {name:'More conversation'}).click();
          await page.waitForFunction(() => document.querySelectorAll('[aria-label="Conversation timeline"] ol li').length === 3);
          const entries = await page.getByRole('region', {name:'Conversation timeline'}).locator('ol li').evaluateAll(nodes => nodes.map(node => node.id));
          if (new Set(entries).size !== 3 || entries.join(',') !== 'conversation-entry-1,conversation-entry-2,conversation-entry-3') throw Error('Refresh reordered or duplicated entries');
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'More accepted prompts'}).click();
          const history = page.getByRole('region', {name:'Prompt History'});
          if (await history.locator('li').count() !== 2) throw Error('Same-text prompts collapsed');
          const historyText = await history.innerText();
          if (!historyText.includes('#1 · 2026-09-29T12:00:00.000Z') || !historyText.includes('#2 · 2026-09-29T12:01:00.000Z')) throw Error('Accepted ordinal or provider time lost');
          await history.getByRole('button', {name:'View full original'}).last().click();
          await page.getByText('Exact entry-3').waitFor();
          await history.getByRole('button', {name:'Open matching Turn'}).last().click();
          await page.getByText('Linked Turn: repeat').waitFor();
          if (await page.getByRole('button', {name:'Conversation', exact:true}).getAttribute('aria-current') !== 'page') throw Error('Turn navigation lost');
        }""")
        cli(directory, "run-code", """async page => {
          const close = page.getByRole('button', {name:'Request whole-session close'});
          if (!await close.isDisabled()) throw Error('Interrupt consent omitted');
          const beforeConsent = await page.evaluate(() => ({...window.fixture}));
          await page.getByRole('checkbox', {name:'I confirm interruption of active owned work.'}).check();
          await page.waitForFunction(previous => window.fixture.action > previous, beforeConsent.action);
          const afterConsent = await page.evaluate(() => ({...window.fixture}));
          if (afterConsent.execution !== beforeConsent.execution || afterConsent.pending !== beforeConsent.pending ||
              afterConsent.card !== beforeConsent.card || !await page.getByRole('heading', {name:'Approve command'}).count())
            throw Error('Consent reloaded the full session or hid pending requests');
          await close.click();
          await page.getByText('Close outcome unresolved. Inspect provider state; do not repeat the request.').last().waitFor();
          if (!await close.isDisabled() || await page.evaluate(() => window.fixture.close) !== 1) throw Error('Ambiguous close allowed retry');
          await page.getByRole('button', {name:'Refresh current state'}).click();
          if (!await close.isDisabled() || await page.evaluate(() => window.fixture.close) !== 1) throw Error('Refresh repeated ambiguous close');
          if (await page.evaluate(() => localStorage.length || sessionStorage.length)) throw Error('Sensitive state persisted in browser');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Chat', exact:true}).click();
          await page.getByRole('heading', {name:'Approve command'}).waitFor();
          const status = await page.locator('.operator__header-status').innerText();
          if (!status.includes('Provider READY') || !status.includes('ACTIVE') || !status.includes('Writer READ_ONLY') ||
              !status.includes('Policy USER_APPROVAL_REQUIRED') || !status.includes('Requests 1')) throw Error('Mobile navigation status incomplete');
          const action = await page.getByRole('region', {name:'Action required'}).boundingBox();
          if (!action || action.y >= 844) throw Error('Mobile action card needs hidden scrolling');
          await page.getByRole('button', {name:'Sessions', exact:true}).click();
          await page.getByRole('button', {name:'Fixture Session'}).click();
          if (!(await page.locator('.operator__header-status').innerText()).includes('Provider READY'))
            throw Error('Same-session selection erased the activity summary');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated&selection-untyped")
        cli(directory, "run-code", """async page => {
          await page.locator('.operator__header-status').waitFor();
          await page.getByRole('button', {name:'Fixture Session'}).click();
          await page.getByText('Session selection is unavailable.', {exact:false}).waitFor();
          if (!(await page.locator('.operator__header-status').innerText()).includes('Provider READY'))
            throw Error('Failed session selection erased the activity summary');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?pending-overflow")
        cli(directory, "run-code", """async page => {
          await page.getByText('Some interaction requests are unavailable.', {exact:false}).waitFor();
          if (await page.evaluate(() => window.fixture.card) !== 100)
            throw Error('Oversized pending list exceeded the bounded card fan-out');
        }""")
        for code, phrase in [
            ("WORKSPACE_NOT_PROVISIONED", "Provision it in Dolgorae"),
            ("PROFILE_MISSING", "Configure a supported provider profile"),
            ("CONTROLLER_MISMATCH", "Verify or reset the Controller"),
            ("PROVIDER_BLOCKED", "Review provider permissions"),
            ("PROTOCOL_INCOMPATIBLE", "Upgrade or migrate the provider"),
            ("PROFILE_SERVER_UNAVAILABLE", "Restore and verify the profile server"),
        ]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?blocker={code}")
            cli(directory, "run-code", f"""async page => {{
              await page.getByText('{phrase}', {{exact:false}}).waitFor();
              if (await page.getByRole('button', {{name:'Refresh current state'}}).count() ||
                  await page.getByRole('button', {{name:'Request whole-session close'}}).isEnabled() ||
                  (await page.locator('body').innerText()).includes('private diagnostic')) throw Error('Unsafe operator blocker display');
            }}""")
        for mode in ["projection_unknown", "projection_recovery", "settling", "confirmed_projection", "stale"]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?close={mode}")
            cli(directory, "run-code", """async page => {
              await page.getByRole('heading', {name:'Close session'}).waitFor();
              await page.getByRole('checkbox', {name:'I confirm interruption of active owned work.'}).check();
              if (!await page.getByRole('button', {name:'Request whole-session close'}).isDisabled() ||
                  await page.evaluate(() => window.fixture.close) !== 0) throw Error('Projected close or stale snapshot allowed a close');
            }""")
            if mode == "confirmed_projection":
                cli(directory, "run-code", "async page => { await page.getByText('Whole-session close confirmed by current provider projection.').waitFor(); }")
        for mode, message in [("in_progress", "Close request accepted; provider settlement pending."),
                              ("confirmed_receipt", "Close response received. Checking the current provider projection."),
                              ("recovery", "Close recovery required. Follow provider recovery outside Gul.")]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?close={mode}")
            cli(directory, "run-code", f"""async page => {{
              await page.getByRole('checkbox', {{name:'I confirm interruption of active owned work.'}}).check();
              await page.getByRole('button', {{name:'Request whole-session close'}}).click();
              await page.getByText('{message}').waitFor();
              if (!await page.getByRole('button', {{name:'Request whole-session close'}}).isDisabled() ||
                  await page.evaluate(() => window.fixture.close) !== 1 ||
                  (await page.locator('body').innerText()).includes('Whole-session close confirmed by current provider projection.')) throw Error('Close receipt promoted or repeated');
            }}""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?close=rejected")
        cli(directory, "run-code", """async page => {
          await page.getByRole('checkbox', {name:'I confirm interruption of active owned work.'}).check();
          await page.getByRole('button', {name:'Request whole-session close'}).click();
          await page.getByText('Close rejected. Check current state before another request.').waitFor();
          if (await page.evaluate(() => window.fixture.close) !== 1) throw Error('Rejected close count changed');
          await page.getByRole('button', {name:'Request whole-session close'}).click();
          if (await page.evaluate(() => window.fixture.close) !== 2) throw Error('Definitive rejection did not allow a new eligible request');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated&pending-failure")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Chat', exact:true}).click();
          await page.getByText('Interaction requests are unavailable. Check provider state before acting.').waitFor();
          if (!(await page.locator('.operator__header-status').innerText()).includes('Requests Unavailable') ||
              (await page.getByRole('region', {name:'Action required'}).innerText()).includes('Loading interaction requests')) throw Error('Unknown pending state displayed as zero or loading');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?blocker=PROFILE_MISSING&card-failure")
        cli(directory, "run-code", """async page => {
          await page.getByText('Configure a supported provider profile', {exact:false}).waitFor();
          await page.getByText('Some interaction requests are unavailable.', {exact:false}).waitFor();
          if (await page.getByRole('button', {name:'Refresh current state'}).count() ||
              await page.getByRole('button', {name:'Approve once'}).count()) throw Error('Mixed failures masked external blocker or allowed mutation');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated&workspace-block=PROFILE_MISSING")
        cli(directory, "run-code", """async page => {
          await page.getByText('Configure a supported provider profile', {exact:false}).waitFor();
          if (await page.getByRole('button', {name:'Retry workspace load'}).count()) throw Error('Workspace retry shown for external blocker');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?invalid-time")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByText('Time unavailable').waitFor();
          if (!await page.getByRole('button', {name:'More accepted prompts'}).count()) throw Error('Invalid timestamp unmounted the view');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'View full original'}).click();
          await page.getByText('Exact entry-1').waitFor();
          const before = await page.evaluate(() => window.fixture.original);
          await page.getByRole('button', {name:'More accepted prompts'}).click();
          await page.waitForFunction(() => document.querySelectorAll('[aria-label="Prompt History"] li').length === 2);
          if (await page.evaluate(() => window.fixture.original) !== before || !await page.getByText('Exact entry-1').count()) throw Error('Pagination refetched or cleared full original');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?conversation-snapshot-change")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'More conversation'}).click();
          await page.getByText('More conversation is unavailable.', {exact:false}).waitFor();
          if (await page.getByRole('region', {name:'Conversation timeline'}).locator('ol li').count() !== 2) throw Error('Changed conversation snapshot was merged');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?history-snapshot-change")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'More accepted prompts'}).click();
          await page.getByText('More Prompt History is unavailable.', {exact:false}).waitFor();
          if (await page.getByRole('region', {name:'Prompt History'}).locator('li').count() !== 1) throw Error('Changed Prompt History snapshot was merged');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?history-failure")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByText('Prompt History is unavailable.', {exact:false}).waitFor();
          await page.getByRole('button', {name:'Refresh Prompt History'}).click();
          await page.getByRole('button', {name:'More accepted prompts'}).waitFor();
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?entry-interaction")
        cli(directory, "run-code", """async page => {
          await page.getByRole('region', {name:'Conversation timeline'}).getByText('Approval opened').waitFor();
          if ((await page.getByRole('region', {name:'Conversation timeline'}).innerText()).includes('make test')) throw Error('Command stream leaked into normal conversation');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?card-mismatch")
        cli(directory, "run-code", """async page => {
          await page.getByText('Some interaction requests are unavailable.', {exact:false}).waitFor();
          if (await page.getByRole('button', {name:'Approve once'}).count()) throw Error('Mismatched card became actionable');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?eligibility-unknown")
        cli(directory, "run-code", """async page => {
          await page.getByRole('checkbox', {name:'I confirm interruption of active owned work.'}).check();
          if (!await page.getByRole('button', {name:'Request whole-session close'}).isDisabled()) throw Error('Typed unknown outcome did not block close');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?artifact")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'View full response'}).click();
          await page.getByText('# Complete final answer').waitFor();
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'View full original'}).click();
          await page.getByText('# 한글 prompt original').waitFor();
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?artifact-mismatch")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'View full response'}).click();
          await page.getByText('Full response is unavailable.', {exact:false}).waitFor();
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'View full original'}).click();
          await page.getByText('Full original is unavailable.', {exact:false}).waitFor();
        }""")
        for suffix, expected in [("", "Linked Turn: repeat"), ("&linked-exact", "Linked Turn: " + "x" * 1024),
                                 ("&linked-missing", "The linked Turn is unavailable."),
                                 ("&linked-failure", "The linked Turn is unavailable."),
                                 ("&linked-oversize", "The linked Turn is unavailable.")]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?linked=1{suffix}")
            cli(directory, "run-code", f"""async page => {{
              await page.getByRole('button', {{name:'Prompt History', exact:true}}).click();
              await page.getByRole('button', {{name:'More accepted prompts'}}).click();
              await page.getByRole('region', {{name:'Prompt History'}}).getByRole('button', {{name:'Open matching Turn'}}).last().click();
              await page.getByText('{expected}', {{exact:false}}).waitFor();
              const calls = await page.evaluate(() => window.fixture.entry);
              if (calls !== 1) throw Error('Unloaded Turn lookup was skipped or duplicated');
              await page.getByRole('button', {{name:'More conversation'}}).click();
              await page.waitForFunction(() => document.querySelectorAll('[aria-label="Conversation timeline"] ol li').length === 3);
              if (await page.evaluate(() => window.fixture.entry) !== calls) throw Error('Linked Turn was refetched on pagination');
            }}""")
        for suffix in ["execution-failure", "close=stale", "blocker=PROFILE_MISSING"]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?{suffix}")
            cli(directory, "run-code", """async page => {
              await page.getByRole('heading', {name:'Approve command'}).waitFor();
              const action = page.getByRole('region', {name:'Action required'});
              if (!await action.getByText('Needs your decision').count() ||
                  !await action.getByRole('button', {name:'Approve once'}).isDisabled() ||
                  (await action.innerText()).includes('Interaction details are unavailable')) throw Error('Loaded card hidden or actionable on stale state');
            }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?close=transport")
        cli(directory, "run-code", """async page => {
          await page.getByRole('checkbox', {name:'I confirm interruption of active owned work.'}).check();
          const close = page.getByRole('button', {name:'Request whole-session close'});
          await close.click();
          await page.getByText('Close outcome unresolved. Inspect provider state; do not repeat the request.').last().waitFor();
          await page.getByRole('button', {name:'Refresh current state'}).click();
          if (!await close.isDisabled() || await page.evaluate(() => window.fixture.close) !== 1) throw Error('Transport failure permitted re-close');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?close=rejected_external")
        cli(directory, "run-code", """async page => {
          await page.getByRole('checkbox', {name:'I confirm interruption of active owned work.'}).check();
          await page.getByRole('button', {name:'Request whole-session close'}).click();
          await page.getByText('Configure a supported provider profile', {exact:false}).waitFor();
          if (await page.getByRole('button', {name:'Refresh current state'}).count() ||
              !await page.getByRole('button', {name:'Request whole-session close'}).isDisabled() ||
              await page.evaluate(() => window.fixture.close) !== 1) throw Error('External close rejection permitted retry');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated&pending-typed")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Chat', exact:true}).click();
          await page.getByText('Review provider permissions outside Gul.', {exact:false}).waitFor();
          if (await page.getByRole('button', {name:'Refresh current state'}).count() ||
              !(await page.locator('.operator__header-status').innerText()).includes('Requests Unavailable')) throw Error('Typed pending blocker exposed retry or count');
        }""")
        for suffix in ["card-failure", "card-resolved"]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?{suffix}")
            cli(directory, "run-code", """async page => {
              await page.getByText('Some interaction requests are unavailable.', {exact:false}).waitFor();
              if (await page.getByRole('button', {name:'Approve once'}).count()) throw Error('Unavailable card became actionable');
            }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?card-typed")
        cli(directory, "run-code", """async page => {
          await page.getByText('Configure a supported provider profile', {exact:false}).waitFor();
          if (await page.getByRole('button', {name:'Refresh current state'}).count() ||
              await page.getByRole('button', {name:'Approve once'}).count()) throw Error('Typed card blocker exposed retry or action');
        }""")
        for suffix in ["invalid-nanos", "invalid-time"]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?{suffix}")
            cli(directory, "run-code", """async page => {
              await page.getByRole('button', {name:'Prompt History', exact:true}).click();
              await page.getByText('Time unavailable').waitFor();
            }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?invalid-page")
        cli(directory, "run-code", """async page => {
          await page.getByText('Conversation is unavailable.', {exact:false}).waitFor();
          if (await page.getByRole('region', {name:'Conversation timeline'}).locator('ol li').count()) throw Error('Malformed page rendered');
        }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated&eligibility-failure")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Chat', exact:true}).click();
          await page.getByText('Current session state is unavailable.', {exact:false}).waitFor();
          const status = await page.locator('.operator__header-status').innerText();
          if (!status.includes('Provider READY') || !status.includes('ACTIVE') || !status.includes('Requests 1') ||
              !status.includes('Writer Unavailable') || !status.includes('Assurance Unavailable')) throw Error('Partial activity was hidden');
        }""")
        for suffix, expected in [("workspace-untyped", "Workspace navigation is unavailable. Retry workspace load."),
                                 ("sessions-untyped", "Sessions are unavailable. Retry sessions.")]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated&{suffix}")
            cli(directory, "run-code", f"""async page => {{
              await page.getByText('{expected}').waitFor();
              if ((await page.locator('body').innerText()).includes('private diagnostic')) throw Error('Navigation diagnostic leaked');
            }}""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?integrated&selection-untyped")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Fixture Session'}).click();
          await page.getByText('Session selection is unavailable. Try again.').waitFor();
          if ((await page.locator('body').innerText()).includes('private diagnostic')) throw Error('Selection diagnostic leaked');
        }""")
        for suffix, panel, button, expected in [
            ("conversation-overlap", "Conversation timeline", "More conversation", 3),
            ("history-overlap", "Prompt History", "More accepted prompts", 2),
        ]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?{suffix}")
            cli(directory, "run-code", f"""async page => {{
              if ('{panel}' === 'Prompt History') await page.getByRole('button', {{name:'Prompt History', exact:true}}).click();
              await page.getByRole('button', {{name:'{button}'}}).click();
              await page.waitForFunction(() => document.querySelector('[aria-label="{panel}"] ol')?.children.length === {expected});
              if (await page.getByRole('region', {{name:'{panel}'}}).locator('ol li').count() !== {expected})
                throw Error('Overlapping continuation duplicated identity');
            }}""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?inline-oversize")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'View full response'}).click();
          await page.getByText('Full response is unavailable.', {exact:false}).waitFor();
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'View full original'}).click();
          await page.getByText('Full original is unavailable.', {exact:false}).waitFor();
        }""")
        for suffix, button, message in [
            ("entry-drift", "View full response", "Full response is unavailable."),
            ("prompt-drift", "View full original", "Full original is unavailable."),
        ]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?{suffix}")
            cli(directory, "run-code", f"""async page => {{
              if ('{suffix}' === 'prompt-drift') await page.getByRole('button', {{name:'Prompt History', exact:true}}).click();
              await page.getByRole('button', {{name:'{button}'}}).click();
              await page.getByText('{message}', {{exact:false}}).waitFor();
            }}""")
        for mode in ["unspecified", "missing"]:
            cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?close={mode}")
            cli(directory, "run-code", """async page => {
              await page.getByRole('checkbox', {name:'I confirm interruption of active owned work.'}).check();
              await page.getByRole('button', {name:'Request whole-session close'}).click();
              await page.getByText('Close outcome unresolved. Inspect provider state; do not repeat the request.').last().waitFor();
              if (!await page.getByRole('button', {name:'Request whole-session close'}).isDisabled() ||
                  await page.evaluate(() => window.fixture.close) !== 1) throw Error('Missing close outcome permitted retry');
            }""")
        cli(directory, "goto", f"http://127.0.0.1:{server.server_port}/?positive-card")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Approve once'}).click();
          await page.getByText('Approval confirmed.').waitFor();
          if (await page.evaluate(() => window.fixture.resolve) !== 1) throw Error('SessionDetail did not route actionable card');
        }""")
        print("PASS real Chrome: mobile status/action priority, chronological conversation, original prompts and Turn, close ambiguity")
    finally:
        try:
            cli(directory, "close")
        finally:
            server.shutdown()
            thread.join()
