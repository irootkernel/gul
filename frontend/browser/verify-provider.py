#!/usr/bin/env python3
"""Checked Chrome UI + ordinary Gul + published Dolgorae + upstream native fake.

This opt-in check does not qualify live Codex, devices, or production deployment.
"""
import base64
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import signal
import ssl
import sqlite3
import struct
import subprocess
import sys
import tempfile
import time
import zlib

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("acceptance", pathlib.Path(__file__).with_name("verify-acceptance.py"))
acceptance = importlib.util.module_from_spec(spec)
spec.loader.exec_module(acceptance)
delivery = acceptance.delivery

def png():
    def chunk(kind, body):
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(b"\x00\xff\x00\x00")) + chunk(b"IEND", b"")

def main():
    binary = pathlib.Path(os.environ["GUL_E2_DOLGORAE_EXECUTABLE"]).resolve()
    archive = pathlib.Path(os.environ["GUL_E2_DOLGORAE_ARCHIVE"]).resolve()
    source = pathlib.Path(os.environ["GUL_E2_DOLGORAE_SOURCE"]).resolve()
    # Published artifacts are the gate, never the supplemental source build.
    for path, digest in ((binary, "8154564ffaa3014bef235aed6a8cc0da017cdc1501267fa9c85aa927227bc824"),
                         (archive, "91fff11625ec546730d7bc18c68c51f5b367a6623700b09661a2b9589ce35bd2")):
        if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            raise AssertionError("published release digest mismatch")
    mounted = " on /Volumes/RootKernel (" in subprocess.check_output(["mount"], text=True)
    scratch = "/Volumes/RootKernel/tmp" if mounted and os.access("/Volumes/RootKernel", os.W_OK) else tempfile.gettempdir()
    with tempfile.TemporaryDirectory(prefix="gp-", dir=scratch) as temporary:
        work = pathlib.Path(temporary).resolve()
        driver = work / "browser-driver"
        subprocess.run(["go", "build", "-o", str(driver), "./test/provider/browser"], cwd=ROOT, check=True, timeout=180)
        subprocess.run([sys.executable, str(ROOT / "test/provider/fixtures/prepare_session.py"), str(binary), str(work), str(source)], cwd=ROOT, check=True, timeout=120)
        workspace = work / "workspace"
        (workspace / "picture.png").write_bytes(png())
        (workspace / ".dolgorae/private.png").write_bytes(png())
        (workspace / "private-alias").symlink_to(".dolgorae", target_is_directory=True)
        port = delivery.free_port()
        environment = {**os.environ, "HOME": str(work / "home"), "TMPDIR": str(work / "tmp"), "GUL_RUN_PUBLISHED_ACCEPTANCE": "1"}
        os.environ.update({"HOME": environment["HOME"], "TMPDIR": environment["TMPDIR"],
                           "PLAYWRIGHT_DAEMON_SESSION_DIR": str(work / "pd"),
                           "PLAYWRIGHT_DAEMON_SOCKETS_DIR": str(work / "ps")})
        process = subprocess.Popen([str(driver), str(work), str(binary), str(port)], cwd=ROOT, env=environment,
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        config = work / "playwright-cli.json"
        browser = f"g2{port}"
        opened = False
        browser_pid = None
        try:
            origin = acceptance.receive(process)["origin"]
            pin = delivery.certificate_spki_pin(port)
            certificate = work / "fixture-certificate.pem"
            certificate.write_text(ssl.get_server_certificate(("127.0.0.1", port), timeout=10))
            os.environ["NODE_EXTRA_CA_CERTS"] = str(certificate)
            config.write_text(json.dumps({"browser": {"browserName": "chromium", "isolated": False,
                "userDataDir": str(work / "chrome-profile"), "launchOptions": {"channel": "chrome",
                "args": [f"--ignore-certificate-errors-spki-list={pin}"]}}}))
            def run(code):
                def bounded_error(content):
                    return re.sub(r"(?im)^.*(?:cookie:|x-gul-csrf:).*$", "[fixture credential redacted]", content)[-4000:]
                daemon_errors = {path: path.read_text()[-4000:] for path in (work / "pd").glob("*/*.err")}
                try:
                    return delivery.cli(config, browser, "run-code", "async page => {page.setDefaultTimeout(20000);" + code + "}")
                except (AssertionError, subprocess.TimeoutExpired) as error:
                    for path, content in daemon_errors.items():
                        if content:
                            print("Prior browser daemon error: " + bounded_error(content), flush=True)
                    for path in (work / "pd").glob("*/*.err"):
                        content = path.read_text()[-4000:]
                        if content and content != daemon_errors.get(path):
                            print("Browser daemon error: " + bounded_error(content), flush=True)
                    try:
                        diagnostic = delivery.cli(config, browser, "run-code", "async page => {return {url:page.url(),closed:page.isClosed(),dom:await page.evaluate(()=>{const prompt=document.querySelector('textarea[aria-label=Prompt]');return {prompt:prompt?{disabled:prompt.disabled,readOnly:prompt.readOnly,length:prompt.value.length}:null,messages:[...document.querySelectorAll('[role=alert],[role=status]')].slice(0,20).map(e=>e.textContent.slice(0,500))};}),errors:page.gulErrors,counts:page.gulCounts,actions:page.gulActions};}")
                        print("Bounded fixture browser state: " + diagnostic, flush=True)
                        for action in acceptance.browser_reply(diagnostic).get('actions', [])[-2:]:
                            print("Action snapshot: " + acceptance.decode_message(action, 'GetActionStateResponse'), flush=True)
                    except (AssertionError, subprocess.TimeoutExpired) as diagnostic_error:
                        print("Browser diagnostic unavailable: " + str(diagnostic_error)[:1000], flush=True)
                    print("Native fake turns: " + str(sum(1 for _ in (work / 'native-inputs.jsonl').open())) if (work / 'native-inputs.jsonl').exists() else "No native turns", flush=True)
                    raise AssertionError(str(error)[:2000]) from None
            def ready():
                for attempt in range(30):
                    reply = acceptance.browser_reply(run("""
                      await page.getByRole('button', {name:'Refresh current state', exact:true}).click();
                      try {await page.waitForFunction(() => {const b=[...document.querySelectorAll('button')].find(b=>b.textContent==='Send prompt');return b&&!b.disabled;}, null, {timeout:15000}); return true;}
                      catch {await page.waitForTimeout(3000);return false;}
                    """))
                    if reply:
                        return
                print(run("return {text:(await page.locator('main').innerText()).slice(0,7000),errors:page.gulErrors,counts:page.gulCounts,actions:page.gulActions};"),flush=True)
                raise AssertionError("fresh idle submission eligibility did not converge")
            def submit(expected="SUBMIT_OUTCOME_ACCEPTED"):
                for attempt in range(30):
                    result = acceptance.browser_reply(run("""
                      await page.getByRole('button', {name:'Refresh current state', exact:true}).click();
                      try {await page.waitForFunction(() => {const b=[...document.querySelectorAll('button')].find(b=>b.textContent==='Send prompt');return b&&!b.disabled;}, null, {timeout:15000});}
                      catch {await page.waitForTimeout(3000);return {pending:true};}
                      const draft=await page.getByLabel('Prompt',{exact:true}).inputValue();
                      const sent=page.gulSent.Submit||0;
                      const reply=page.gulWaitForResponse(r=>r.url().endsWith('.DirectSessionService/Submit'));
                      try {await page.getByRole('button',{name:'Send prompt',exact:true}).click({timeout:5000});}
                      catch(error) {if((page.gulSent.Submit||0)===sent) {await page.waitForTimeout(5000);return {pending:true};}throw error;}
                      const response=await reply;
                      return {status:response.status(),body:(await response.body()).toString('base64'),draftRetained:await page.getByLabel('Prompt',{exact:true}).inputValue()===draft};
                    """))
                    if result.get("pending"):
                        continue
                    if result["status"] == 429:
                        time.sleep(5)
                        continue
                    if result["status"] != 200:
                        raise AssertionError("Submit failed: " + str(result))
                    body=acceptance.decode_message(result["body"], "SubmitResponse")
                    if "SUBMIT_OUTCOME_REJECTED" in body and "ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED" in body:
                        if not result["draftRetained"]:
                            raise AssertionError("freshness refusal lost the draft")
                        time.sleep(3)
                        continue
                    if "outcome: " + expected not in body:
                        raise AssertionError("unexpected Submit outcome: " + body)
                    # Each native turn invalidates several ordinary host reads.
                    # Pace this campaign within the production request window.
                    time.sleep(12)
                    return
                print(run("return {text:(await page.locator('main').innerText()).slice(0,7000),errors:page.gulErrors,counts:page.gulCounts,actions:page.gulActions};"),flush=True)
                raise AssertionError("submission did not converge")
            opened_reply = delivery.cli(config, browser, "open", origin)
            pid = re.search(r"opened with pid (\d+)", opened_reply)
            if pid:
                browser_pid = int(pid.group(1))
            opened = True
            delivery.cli(config, browser, "snapshot")
            run("""
              await page.route('**/manifest.webmanifest',async route=>{
                try {const response=await route.fetch();page.gulTLSProbe=response.status();await route.fulfill({response});}
                catch(error) {page.gulTLSProbe=String(error).split('Call log:')[0];await route.abort('failed');}
              },{times:1});
              await page.evaluate(()=>fetch('/manifest.webmanifest').catch(()=>undefined));
              if(page.gulTLSProbe!==200) throw Error('Fixture TLS route fetch failed: '+page.gulTLSProbe);
              await page.unroute('**/manifest.webmanifest');
            """)
            run("""
              page.gulErrors=[];
              page.gulSent={};
              page.on('request',request=>{if(request.url().includes('/gul.v1.')) {const name=request.url().split('/').pop();page.gulSent[name]=(page.gulSent[name]||0)+1;}});
              page.gulWaitForResponse=predicate=>{const reply=page.waitForResponse(predicate);reply.catch(()=>{});return reply;};
              page.gulCounts={};page.gulActions=[];page.gulResultPages=[];
              page.on('response', async response => {if (!response.url().includes('/gul.v1.')) return; const name=response.url().split('/').pop();page.gulCounts[name]=(page.gulCounts[name]||0)+1;
                if(name==='ListSpecialistResults'&&response.status()===200) {const body=await response.body().catch(()=>undefined);if(body) page.gulResultPages.push(body.toString('base64'));page.gulResultPages=page.gulResultPages.slice(-3);}
                if(name==='GetActionState'&&response.status()===200) {const body=await response.body().catch(()=>undefined);if(body) page.gulActions.push(body.toString('base64'));page.gulActions=page.gulActions.slice(-5);}
                if (response.status()>=400) {page.gulErrors.push({url:name,status:response.status(),body:(await response.text().catch(()=>'Transport body unavailable')).slice(0,1200)}); page.gulErrors=page.gulErrors.slice(-8);}});
              await page.getByLabel('Password', {exact:true}).fill('published browser fixture password');
              await page.getByRole('button', {name:'Sign in', exact:true}).click();
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              await page.getByText('Launch compatibility', {exact:true}).click();
              await page.getByLabel(/^Runtime Profile/).selectOption('default');
              await page.getByLabel(/^Model/).selectOption('gpt-5.6');
              await page.getByRole('form',{name:'Session configuration',exact:true}).getByLabel(/^Effort/).selectOption('medium');
              await page.getByLabel(/^Execution lane/).selectOption({label:'Dedicated'});
              await page.getByLabel(/^Required assurance/).selectOption({label:'BEST_EFFORT_PERSONAL_ALPHA'});
              await page.getByLabel(/^Specialist Policy/).selectOption('carrier-test');
            """)
            refused = acceptance.browser_reply(run("""
              // Keep protobuf framing and retry labels; change only the known effort field.
              await page.route('**/gul.v1.DirectSessionService/CreateSession', async route => {
                const body = route.request().postDataBuffer();
                const effort = String.fromCharCode(0x1a, 6) + 'medium';
                const offset = body.indexOf(effort);
                if (offset < 0 || offset !== body.lastIndexOf(effort)) throw Error('Expected one selected effort field');
                body.write('foobar', offset + 2, 'utf8');
                await route.continue({postData: body});
              }, {times: 1});
              const reply = page.gulWaitForResponse(r => r.url().endsWith('.DirectSessionService/CreateSession'));
              await page.getByRole('button', {name:'Create session', exact:true}).click();
              const response = await reply;
              const error = await response.json();
              await page.getByText('Launch configuration is unsupported. Review the selected profile and launch options.', {exact:true}).waitFor();
              await page.waitForFunction(() => {const b=[...document.querySelectorAll('button')].find(b=>b.textContent==='Create session');return b&&!b.disabled;});
              if(await page.getByRole('button', {name:'Recover session creation', exact:true}).count()) throw Error('Known no-dispatch refusal retained recovery');
              await page.unroute('**/gul.v1.DirectSessionService/CreateSession');
              return {status: response.status(), error};
            """))
            details = [item for item in refused['error'].get('details', []) if item.get('type') == 'gul.v1.DomainError']
            if refused['status'] != 400 or refused['error'].get('code') != 'failed_precondition' or len(details) != 1:
                raise AssertionError('Launch refusal lacked its exact typed pre-dispatch proof: ' + str(refused))
            detail = acceptance.decode_message(details[0]['value'], 'DomainError')
            if 'ERROR_CODE_PROVIDER_BLOCKED' not in detail or 'ACTION_CLASS_USE_SUPPORTED_PROFILE' not in detail:
                raise AssertionError('Launch refusal did not prove unsupported configuration')
            with sqlite3.connect(f"file:{work / 'Gul/gul.sqlite'}?mode=ro", uri=True) as database:
                for table in ('session_creations', 'controller_credential_metadata'):
                    if database.execute(f'SELECT count(*) FROM {table}').fetchone()[0] != 0:
                        raise AssertionError('Unsupported launch allocated persistent provider state')
            if (work / 'native-inputs.jsonl').exists() and (work / 'native-inputs.jsonl').stat().st_size:
                raise AssertionError('Unsupported launch dispatched native input')
            run("""
              const reply = page.gulWaitForResponse(r => r.url().endsWith('.DirectSessionService/CreateSession'));
              await page.getByRole('button', {name:'Create session', exact:true}).click();
              const response = await reply;
              if (response.status() !== 200) throw Error(await response.text());
              await page.getByLabel('Prompt', {exact:true}).waitFor();
              if (!await page.getByRole('button', {name:'Acquire writer', exact:true}).isDisabled()) throw Error('Threadless Acquire was enabled');
              await page.getByLabel(/^Prompt access/).selectOption('write');
              await page.getByLabel('Prompt', {exact:true}).fill('first WRITE through assembled Gul');
            """)
            submit()
            if not (workspace / "provider-write.txt").exists():
                raise AssertionError("provider-authoritative fake workspace WRITE effect missing")
            run("""
              await page.getByRole('button', {name:'Refresh current state', exact:true}).click();
              await page.getByRole('region', {name:'Writer state'}).getByText('ACTIVE', {exact:true}).waitFor();
              await page.getByLabel(/^Prompt access/).selectOption('read');
              await page.getByLabel('Prompt', {exact:true}).fill('wait-for-approval');
            """)
            ready()
            run("""
              await page.getByRole('region', {name:'Writer state'}).getByText('Policy unverified', {exact:true}).waitFor();
              if (!await page.getByRole('button', {name:'Acquire writer', exact:true}).isDisabled()) throw Error('Unverified transition enabled Acquire');
            """)
            submit()
            for attempt in range(30):
                reply = acceptance.browser_reply(run("""
                  await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
                  try {await page.waitForFunction(() => {const b=[...document.querySelectorAll('button')].find(b=>b.textContent==='Approve once');return b&&!b.disabled;},null,{timeout:15000});}
                  catch {await page.waitForTimeout(3000);return {pending:true};}
                  if(!await page.getByRole('button',{name:'Send prompt',exact:true}).isDisabled()) throw Error('Waiting Turn admitted a prompt');
                  await page.getByLabel('Prompt',{exact:true}).fill('busy draft stays in browser');
                  const sent=page.gulSent.Resolve||0;
                  const reply=page.gulWaitForResponse(r=>r.url().endsWith('.InteractionPresentationService/Resolve'));
                  try {await page.getByRole('button',{name:'Approve once',exact:true}).click({timeout:5000});}
                  catch(error) {if((page.gulSent.Resolve||0)===sent) return {pending:true};throw error;}
                  const response=await reply;
                  return {status:response.status(),body:(await response.body()).toString('base64')};
                """))
                if reply.get("pending"):
                    continue
                if reply["status"] != 200:
                    error=json.loads(base64.b64decode(reply["body"]))
                    details=[detail for detail in error.get("details",[]) if detail.get("type")=="gul.v1.DomainError"]
                    if error.get("code")!="failed_precondition" or len(details)!=1:
                        raise AssertionError("Interaction failed without safe retry evidence: "+str(error))
                    detail=acceptance.decode_message(details[0]["value"],"DomainError")
                    if "code: ERROR_CODE_PROVIDER_BLOCKED" not in detail or "action: ACTION_CLASS_REFETCH_INTERACTION" not in detail:
                        raise AssertionError("Interaction refusal was not safe to retry: "+detail)
                    time.sleep(8)
                    continue
                if 'INTERACTION_RESOLUTION_OUTCOME_RESOLVED' not in acceptance.decode_message(reply['body'],'ResolveResponse'):
                    raise AssertionError('Interaction did not resolve')
                break
            else:
                raise AssertionError('waiting Interaction did not converge')
            time.sleep(8)
            for image_path,effort in ((".dolgorae/private.png",""),("private-alias/private.png",""),("","unsupported-effort")):
                run("await page.getByLabel('Image paths in this workspace (one per line)',{exact:true}).fill("+json.dumps(image_path)+");await page.getByLabel('Effort override (optional)',{exact:true}).fill("+json.dumps(effort)+");await page.getByLabel('Prompt',{exact:true}).fill('invalid input stays a draft');")
                for attempt in range(30):
                    ready()
                    reply=acceptance.browser_reply(run("""
                      const reply=page.gulWaitForResponse(r=>r.url().endsWith('.DirectSessionService/Submit'));
                      await page.getByRole('button',{name:'Send prompt',exact:true}).click();
                      const response=await reply;
                      return {status:response.status(),body:(await response.body()).toString('base64'),draft:await page.getByLabel('Prompt',{exact:true}).inputValue()};
                    """))
                    if reply['draft']!='invalid input stays a draft':
                        raise AssertionError('invalid input lost its draft')
                    if reply['status']==400 and json.loads(base64.b64decode(reply['body'])).get('code')=='invalid_argument':
                        break
                    if reply['status']==200:
                        detail=acceptance.decode_message(reply['body'],'SubmitResponse')
                        if 'SUBMIT_OUTCOME_REJECTED' in detail and 'ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED' in detail:
                            time.sleep(3)
                            continue
                    raise AssertionError('invalid input was not refused before dispatch')
                else:
                    raise AssertionError('invalid input guard did not converge')
            run("""
              await page.getByRole('button', {name:'Refresh current state', exact:true}).click();
              await page.getByLabel('Image paths in this workspace (one per line)', {exact:true}).fill('picture.png');
              await page.getByLabel('Effort override (optional)', {exact:true}).fill('high');
              await page.getByLabel('Prompt', {exact:true}).fill('accepted image and effort override');
            """)
            submit()
            inputs = [json.loads(line) for line in (work / "native-inputs.jsonl").read_text().splitlines()]
            image_turn = inputs[-1]
            image_items = [item for item in image_turn["input"] if item["type"] == "localImage"]
            if len(image_items) != 1 or str(workspace) in image_items[0]["path"] or image_turn["effort"] != "high":
                raise AssertionError("guarded private image/effort did not reach actual provider runtime")
            run("""
              await page.getByLabel('Image paths in this workspace (one per line)',{exact:true}).fill('');
              await page.getByLabel('Effort override (optional)',{exact:true}).fill('');
              await page.getByLabel(/^Prompt access/).selectOption('write');
              await page.getByLabel('Prompt',{exact:true}).fill('subsequent explicit WRITE');
            """)
            submit()
            run("await page.getByLabel(/^Prompt access/).selectOption('read');")
            expected_prompts=["first WRITE through assembled Gul", "wait-for-approval", "accepted image and effort override", "subsequent explicit WRITE"]
            for index in range(47):
                text=f"history prompt {index}\n원문 <script>inert</script>"
                run("await page.getByLabel('Prompt',{exact:true}).fill("+json.dumps(text)+");")
                submit()
                expected_prompts.append(text)
                if index%10==0:
                    print(f"published browser accepted {len(expected_prompts)} primary prompts",flush=True)
            # Lose only the browser response after the real host has accepted.
            # The browser retains uncertainty and never sends the draft itself.
            run("await page.getByLabel('Prompt',{exact:true}).evaluate(element=>{if(element.disabled||element.readOnly) throw Error('Prompt is not editable');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(element,'긴 입력\\n'.repeat(40000));element.dispatchEvent(new Event('input',{bubbles:true}));});")
            for attempt in range(30):
                ready()
                captured=acceptance.browser_reply(run("""
                  page.gulCapturedSubmit=undefined;page.gulFaultError=undefined;
                  await page.route('**/gul.v1.DirectSessionService/Submit',async route => {
                    try {
                      const response=await route.fetch();
                      const body=(await response.body()).toString('base64');
                      page.gulHeldSubmitRoute=route;page.gulHeldSubmitResponse=response;
                      page.gulCapturedSubmit={status:response.status(),body};
                    } catch(error) {page.gulFaultError=String(error).split('Call log:')[0];await route.abort('failed');}
                  },{times:1});
                  await page.getByRole('button',{name:'Send prompt',exact:true}).click();
                  for(let tries=0;!page.gulCapturedSubmit&&!page.gulFaultError&&tries<80;tries++) await page.waitForTimeout(250);
                  if(page.gulFaultError) throw Error(page.gulFaultError);
                  if(!page.gulCapturedSubmit) throw Error('Submit response was not captured');
                  return page.gulCapturedSubmit;
                """))
                detail=acceptance.decode_message(captured['body'],'SubmitResponse') if captured['status']==200 else ''
                if 'SUBMIT_OUTCOME_ACCEPTED' in detail:
                    run("""
                      await page.gulHeldSubmitRoute.abort('failed');
                      await page.unroute('**/gul.v1.DirectSessionService/Submit');
                      await page.waitForTimeout(1000);
                      if(await page.getByLabel('Prompt',{exact:true}).inputValue()!=='긴 입력\\n'.repeat(40000)) throw Error('uncertain draft was lost');
                      await page.waitForFunction(()=>{const prompt=document.querySelector('textarea[aria-label=Prompt]');return prompt&&!prompt.disabled&&!prompt.readOnly;});
                    """)
                    break
                # Deliver known refusal normally; only accepted input loses its
                # browser receipt. Unknown effects never authorize another send.
                run("await page.gulHeldSubmitRoute.fulfill({response:page.gulHeldSubmitResponse});await page.unroute('**/gul.v1.DirectSessionService/Submit');")
                if captured['status']==429 or 'SUBMIT_OUTCOME_REJECTED' in detail and 'ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED' in detail:
                    time.sleep(5)
                    continue
                raise AssertionError('long input was not accepted: '+detail)
            else:
                raise AssertionError('long input acceptance did not converge')
            expected_prompts.append('긴 입력\n'*40000)
            time.sleep(8)
            run("""
              await page.getByRole('button', {name:'Refresh current state', exact:true}).click();
              await page.getByLabel('Effort override (optional)', {exact:true}).fill('');
              // Fill times out on the retained 40k-line controlled textarea.
              // Dispatch input only after ordinary editing
              // is available; keep React state and provider guards in force.
              await page.getByLabel('Prompt', {exact:true}).evaluate(element=>{
                if(element.disabled||element.readOnly) throw Error('Prompt is not editable');
                Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(element,'hold-busy Primary interruption qualification');
                element.dispatchEvent(new Event('input',{bubbles:true}));
              });
            """)
            submit()
            expected_prompts.append("hold-busy Primary interruption qualification")
            run("""
              await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
              await page.getByLabel('Prompt',{exact:true}).fill('busy Primary draft stays local');
              if(!await page.getByRole('button',{name:'Interrupt Primary turn',exact:true}).isDisabled()) throw Error('Primary interruption admitted without consent');
              await page.getByLabel('I confirm interruption of active owned work.',{exact:true}).check();
            """)
            for attempt in range(12):
                captured=acceptance.browser_reply(run("""
                  await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
                  await page.waitForFunction(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent==='Interrupt Primary turn');return b&&!b.disabled;},null,{timeout:10000});
                  page.gulCapturedInterrupt=undefined;page.gulInterruptError=undefined;
                  await page.route('**/gul.v1.DirectSessionService/InterruptPrimary',async route=>{
                    try {
                      const response=await route.fetch();
                      page.gulHeldInterruptRoute=route;page.gulHeldInterruptResponse=response;
                      page.gulCapturedInterrupt={status:response.status(),body:(await response.body()).toString('base64')};
                    } catch(error) {page.gulInterruptError=String(error).split('Call log:')[0];await route.abort('failed');}
                  },{times:1});
                  await page.getByRole('button',{name:'Interrupt Primary turn',exact:true}).click();
                  for(let tries=0;!page.gulCapturedInterrupt&&!page.gulInterruptError&&tries<80;tries++) await page.waitForTimeout(250);
                  if(page.gulInterruptError) throw Error(page.gulInterruptError);
                  if(!page.gulCapturedInterrupt) throw Error('Primary interrupt response was not captured');
                  return page.gulCapturedInterrupt;
                """))
                interrupt_receipt=acceptance.decode_message(captured['body'],'InterruptPrimaryResponse') if captured['status']==200 else ''
                if 'INTERRUPT_OUTCOME_ACCEPTED' in interrupt_receipt:
                    run("""
                      page.gulInterruptSent=page.gulSent.InterruptPrimary;
                      await page.route('**/gul.v1.DirectSessionService/GetExecutionState',route=>route.abort('failed'));
                      await page.route('**/gul.v1.WriterActionService/GetActionState',route=>route.abort('failed'));
                      await page.gulHeldInterruptRoute.abort('failed');
                      await page.unroute('**/gul.v1.DirectSessionService/InterruptPrimary');
                      await page.getByText('Primary interruption outcome unresolved. Inspect provider state; do not repeat the request.',{exact:true}).waitFor();
                      if(!await page.getByRole('button',{name:'Interrupt Primary turn',exact:true}).isDisabled()) throw Error('uncertain Primary interruption was repeatable');
                      if(await page.getByLabel('Prompt',{exact:true}).inputValue()!=='busy Primary draft stays local') throw Error('Primary interrupt lost the draft');
                      await page.waitForTimeout(6000);
                      if(page.gulSent.InterruptPrimary!==page.gulInterruptSent) throw Error('Primary interruption was automatically retransmitted');
                      await page.unroute('**/gul.v1.DirectSessionService/GetExecutionState');
                      await page.unroute('**/gul.v1.WriterActionService/GetActionState');
                    """)
                    break
                # Retry only the delivered, explicit no-dispatch freshness refusal.
                run("await page.gulHeldInterruptRoute.fulfill({response:page.gulHeldInterruptResponse});await page.unroute('**/gul.v1.DirectSessionService/InterruptPrimary');")
                if 'INTERRUPT_OUTCOME_REJECTED' in interrupt_receipt and 'ACTION_BLOCKER_FRESH_SNAPSHOT_REQUIRED' in interrupt_receipt:
                    run("await page.getByText('Primary interruption was not admitted. Refresh current state.',{exact:true}).waitFor();")
                    time.sleep(3)
                    continue
                raise AssertionError('published provider did not accept Primary interruption: '+interrupt_receipt)
            else:
                raise AssertionError('fresh Primary interruption acceptance did not converge')
            ready()
            # Reopening v0.1.3 can report UNVERIFIED background. Verify the
            # retained active session here; restart after confirmed Close below.
            run("""
              if(await page.getByText('Whole-session close confirmed by current provider projection.',{exact:true}).count()) throw Error('Primary interruption closed the whole session');
              if(page.gulSent.InterruptPrimary!==page.gulInterruptSent) throw Error('Gul restart retransmitted Primary interruption');
              await page.getByLabel('I confirm interruption of active owned work.',{exact:true}).uncheck();
            """)
            busy_native=[json.loads(line) for line in (work/'native-inputs.jsonl').read_text().splitlines()]
            busy_turn=busy_native[-1]
            native_interrupts=[message for line in (work/'native-protocol.jsonl').read_text().splitlines() if (message:=json.loads(line)).get('method')=='turn/interrupt']
            if len(native_interrupts)!=1 or native_interrupts[0]['params'].get('turnId')!=busy_turn['turn'] or native_interrupts[0]['params'].get('threadId')!=busy_turn['thread']:
                raise AssertionError('Primary interruption targeted another Turn or repeated')
            terminal=[entry for line in (work/'native-completions.jsonl').read_text().splitlines() if (entry:=json.loads(line)).get('turn')==busy_turn['turn']]
            if len(busy_native)!=len(expected_prompts) or len(terminal)!=1 or terminal[0]['status']!='interrupted':
                raise AssertionError('Primary interruption/restart replayed a draft or lacked native terminal observation')
            run("await page.getByLabel('Prompt',{exact:true}).fill('publish-specialist');")
            submit()
            for attempt in range(30):
                found=acceptance.browser_reply(run("""
                  const result=page.getByRole('region',{name:'Specialist results'}).getByRole('button').first();
                  if(await result.count()) {await result.click();return true;}
                  await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
                  await page.waitForTimeout(3000);return false;
                """))
                if found:
                    break
            else:
                print(run("return {text:(await page.locator('main').innerText()).slice(0,7000),errors:page.gulErrors,counts:page.gulCounts,actions:page.gulActions};"),flush=True)
                native=[json.loads(line) for line in (work/'native-inputs.jsonl').read_text().splitlines()]
                print('Native prompt summaries: '+json.dumps([{'thread':item['thread'],'text':'\n'.join(part.get('text','') for part in item['input'] if part['type']=='text')[:160]} for item in native]),flush=True)
                replies=[]
                for line in (work/'native-protocol.jsonl').read_text().splitlines():
                    message=json.loads(line)
                    if message.get('id') in (8001,8002,8003,8004,8005,8006):
                        replies.append(message)
                print('Bounded fixture Broker replies: '+json.dumps(replies)[-5000:],flush=True)
                raise AssertionError("public Specialist result did not converge")
            run("""
              await page.getByLabel('Specialist result original',{exact:true}).waitFor();
              const text=await page.getByLabel('Specialist result original',{exact:true}).textContent();
              if(text!=='Gul Specialist result\\n'+'verified large result line\\n'.repeat(50000)) throw Error('Verified large public Specialist artifact differs');
            """)
            artifact_counts=acceptance.browser_reply(run("return page.gulCounts;"))
            if artifact_counts.get("ReadChunk",0)<2 or not artifact_counts.get("GetMetadata",0):
                raise AssertionError("large result did not exercise verified chunk retrieval")
            result_page=acceptance.decode_message(acceptance.browser_reply(run("return page.gulResultPages.at(-1);")),"ListSpecialistResultsResponse")
            result_identity={key:re.findall(r"(?m)^\s*"+key+r": (.+)$",result_page) for key in ("result_id","specialist_view_id","role_label","publication_order","byte_length","sha256")}
            if any(len(value)!=1 for value in result_identity.values()):
                raise AssertionError("public result identity/integrity metadata missing")
            expected_prompts.append("publish-specialist")
            inputs=[json.loads(line) for line in (work/'native-inputs.jsonl').read_text().splitlines()]
            primary_thread=inputs[0]['thread']
            primary=[item for item in inputs if item['thread']==primary_thread]
            actual=['\n'.join(part.get('text','') for part in item['input'] if part['type']=='text') for item in primary]
            if actual!=expected_prompts or len(inputs)!=len(expected_prompts)+1 or any(item['sandbox']!='workspaceWrite' for item in primary):
                raise AssertionError('native dispatch sequence, writer retention or counts differ')
            run("await page.reload();await page.getByRole('main',{name:'Gul operator'}).waitFor();await page.getByRole('button',{name:'Prompt History',exact:true}).click();")
            for attempt in range(20):
                result=acceptance.browser_reply(run("""
                  const history=page.getByLabel('Prompt History',{exact:true});
                  const count=await history.locator('li').count();
                  if(!count) {await history.getByRole('button',{name:'Refresh Prompt History',exact:true}).click();await page.waitForTimeout(3000);return {pending:true};}
                  const more=history.getByRole('button',{name:'More accepted prompts',exact:true});
                  if(await more.count()) {await more.click();await page.waitForTimeout(1000);return {pending:true};}
                  return {count,labels:await history.getByRole('button',{name:/View full original prompt/}).evaluateAll(elements=>elements.map(e=>e.getAttribute('aria-label')))};
                """))
                if not result.get('pending'):
                    break
            if result.get('count')!=len(expected_prompts) or result['labels']!=[f'View full original prompt {i+1}' for i in range(len(expected_prompts))]:
                raise AssertionError('paged history omitted, reordered or duplicated accepted prompts')
            for ordinal in (1,4,51,52,53,54):
                expected=expected_prompts[ordinal-1]
                original_length=len(expected.encode('utf-16-le'))//2
                body=acceptance.browser_reply(run("const ordinal="+str(ordinal)+";const originalLength="+str(original_length)+";"+"""
                  const history=page.getByLabel('Prompt History',{exact:true});
                  const reply=page.gulWaitForResponse(r=>r.url().endsWith('.DirectSessionService/GetPromptHistoryItem'));
                  await history.getByRole('button',{name:'View full original prompt '+ordinal,exact:true}).click();
                  if((await reply).status()!==200) throw Error('Prompt original query failed');
                  await page.waitForFunction(length=>document.querySelector('[aria-label="Full original prompt"] pre')?.textContent.length===length,originalLength);
                  return await page.getByLabel('Full original prompt',{exact:true}).locator('pre').textContent();
                """))
                if body!=expected:
                    raise AssertionError(f'original prompt {ordinal} changed across paging/restart')
            acceptance.command(process,'lose-start-receipt')
            run("""
              await page.getByText('Launch compatibility',{exact:true}).click();
              await page.getByLabel(/^Runtime Profile/).selectOption('default');
              await page.getByLabel(/^Model/).selectOption('gpt-5.6');
              await page.getByRole('form',{name:'Session configuration',exact:true}).getByLabel(/^Effort/).selectOption('medium');
              await page.getByLabel(/^Execution lane/).selectOption({label:'Dedicated'});
              await page.getByLabel(/^Required assurance/).selectOption({label:'BEST_EFFORT_PERSONAL_ALPHA'});
              await page.getByLabel(/^Specialist Policy/).selectOption('carrier-test');
              const reply=page.gulWaitForResponse(r=>r.url().endsWith('.DirectSessionService/CreateSession'));
              await page.getByRole('button',{name:'Create session',exact:true}).click();
              if((await reply).status()===200) throw Error('lost allocation receipt was hidden');
              await page.getByRole('button',{name:'Recover session creation',exact:true}).waitFor();
              if(!await page.getByRole('button',{name:'Create session',exact:true}).isDisabled()) throw Error('unresolved allocation admitted another Create');
            """)
            acceptance.command(process,'restore-start-receipt')
            run("""
              const reply=page.gulWaitForResponse(r=>r.url().endsWith('.DirectSessionService/RecoverCreation'));
              await page.getByRole('button',{name:'Recover session creation',exact:true}).click();
              if((await reply).status()!==200) throw Error('exact allocation recovery failed');
              await page.getByLabel('Prompt',{exact:true}).waitFor();
              await page.getByLabel(/^Prompt access/).selectOption('write');
              await page.getByLabel('Prompt',{exact:true}).fill('competing writer stays a draft');
              await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
              await page.waitForTimeout(5000);
              if(!await page.getByRole('button',{name:'Send prompt',exact:true}).isDisabled()) throw Error('competing writer enabled Send');
              if(await page.getByLabel('Prompt',{exact:true}).inputValue()!=='competing writer stays a draft') throw Error('competing draft lost');
            """)
            run("""
              const other=page.getByRole('complementary',{name:'Workspaces and sessions'}).locator('li > button:not([aria-current])');
              if(await other.count()!==1) throw Error('Expected only the original Primary beside the selected unrelated session');
              await other.click();
            """)
            run("""
              await page.getByRole('button',{name:'Conversation',exact:true}).click();
              await page.getByLabel('I confirm interruption of active owned work.',{exact:true}).check();
              await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
              await page.waitForFunction(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent==='Request whole-session close');return b&&!b.disabled;});
              await page.route('**/gul.v1.DirectSessionService/GetExecutionState',route=>route.abort('failed'));
              await page.route('**/gul.v1.DirectSessionService/CloseRuntime',async route=>{
                try {
                  const response=await route.fetch();
                  if(response.status()!==200) throw Error('root close failed');
                  page.gulLostClose=true;await route.abort('failed');
                } catch(error) {page.gulFaultError=String(error).split('Call log:')[0];await route.abort('failed');}
              },{times:1});
              await page.getByRole('button',{name:'Request whole-session close',exact:true}).click();
              await page.getByText('Close outcome unresolved. Inspect provider state; do not repeat the request.',{exact:true}).first().waitFor();
              if(page.gulFaultError) throw Error(page.gulFaultError);
              if(!page.gulLostClose||!await page.getByRole('button',{name:'Request whole-session close',exact:true}).isDisabled()) throw Error('uncertain close repeated');
              await page.unroute('**/gul.v1.DirectSessionService/GetExecutionState');
              await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
              await page.getByText('Whole-session close confirmed by current provider projection.',{exact:true}).waitFor();
              await page.getByRole('button',{name:'Prompt History',exact:true}).click();
            """)
            interrupt_count_before_restart=sum(json.loads(line).get('method')=='turn/interrupt' for line in (work/'native-protocol.jsonl').read_text().splitlines())
            acceptance.command(process,'restart')
            time.sleep(5)
            if not acceptance.browser_reply(run("return page.gulSent.InterruptPrimary===page.gulInterruptSent;")):
                raise AssertionError('Gul restart retransmitted Primary interruption')
            interrupt_count_after_restart=sum(json.loads(line).get('method')=='turn/interrupt' for line in (work/'native-protocol.jsonl').read_text().splitlines())
            if interrupt_count_after_restart!=interrupt_count_before_restart:
                raise AssertionError('Gul restart replayed a tokenless native interruption')
            if len((work/'native-inputs.jsonl').read_text().splitlines())!=len(inputs):
                raise AssertionError('Gul restart repeated a native Turn')
            run("await page.reload();await page.getByRole('main',{name:'Gul operator'}).waitFor();await page.getByRole('button',{name:'Prompt History',exact:true}).click();")
            run("""
              const history=page.getByLabel('Prompt History',{exact:true});
              await history.getByRole('button',{name:'More accepted prompts',exact:true}).click();
              await history.getByRole('button',{name:'View full original prompt 54',exact:true}).waitFor();
              if(await history.locator('li').count()!==54) throw Error('closed Prompt History was not retained');
              await history.getByRole('button',{name:'View full original prompt 52',exact:true}).click();
              await page.waitForFunction(()=>document.querySelector('[aria-label=\"Full original prompt\"] pre')?.textContent==='긴 입력\\n'.repeat(40000));
            """)
            retained_counts=acceptance.browser_reply(run("return {...page.gulCounts};"))
            run("""
              const result=page.getByRole('region',{name:'Specialist results'}).getByRole('button').first();
              await result.waitFor();await result.click();
              await page.getByLabel('Specialist result original',{exact:true}).waitFor();
              const text=await page.getByLabel('Specialist result original',{exact:true}).textContent();
              if(text!=='Gul Specialist result\\n'+'verified large result line\\n'.repeat(50000)) throw Error('Post-close/restart Specialist artifact changed');
            """)
            retained_page=acceptance.decode_message(acceptance.browser_reply(run("return page.gulResultPages.at(-1);")),"ListSpecialistResultsResponse")
            retained_identity={key:re.findall(r"(?m)^\s*"+key+r": (.+)$",retained_page) for key in result_identity}
            if retained_identity!=result_identity:
                raise AssertionError("public result identity or integrity changed after Close/restart")
            after_counts=acceptance.browser_reply(run("return page.gulCounts;"))
            if after_counts.get("ReadChunk",0)-retained_counts.get("ReadChunk",0)<2 or after_counts.get("GetMetadata",0)<=retained_counts.get("GetMetadata",0):
                raise AssertionError("post-close/restart did not reread the verified multi-chunk artifact")
            run("""
              const other=page.getByRole('complementary',{name:'Workspaces and sessions'}).locator('li > button:not([aria-current])');
              if(await other.count()!==1) throw Error('Expected only the unrelated session beside the selected closed Primary');
              await other.click();
            """)
            run("await page.getByLabel(/^Prompt access/).selectOption('write');await page.getByLabel('Prompt',{exact:true}).fill('unrelated session remains available');")
            ready()
            run("if(await page.getByText('Whole-session close confirmed by current provider projection.',{exact:true}).count()) throw Error('root close affected unrelated session');")
            run("""
              await page.getByText('Launch compatibility',{exact:true}).click();
              await page.getByLabel(/^Runtime Profile/).selectOption('default');
              await page.getByLabel(/^Model/).selectOption('gpt-5.6');
              await page.getByRole('form',{name:'Session configuration',exact:true}).getByLabel(/^Effort/).selectOption('medium');
              await page.getByLabel(/^Execution lane/).selectOption({label:'Shared read-only'});
              await page.getByLabel(/^Required assurance/).selectOption({label:'BEST_EFFORT_PERSONAL_ALPHA'});
              await page.getByLabel(/^Specialist Policy/).selectOption('carrier-test');
              await page.getByLabel('I understand this session will remain read-only.',{exact:true}).check();
              const reply=page.gulWaitForResponse(r=>r.url().endsWith('.DirectSessionService/CreateSession'));
              await page.getByRole('button',{name:'Create session',exact:true}).click();
              if((await reply).status()!==200) throw Error('shared read-only creation failed');
              await page.getByLabel('Prompt',{exact:true}).waitFor();
              await page.getByLabel(/^Prompt access/).selectOption('write');
              await page.getByLabel('Prompt',{exact:true}).fill('shared WRITE stays a draft');
              await page.getByRole('button',{name:'Refresh current state',exact:true}).click();
              await page.waitForTimeout(5000);
              if(!await page.getByRole('button',{name:'Send prompt',exact:true}).isDisabled()) throw Error('shared WRITE enabled Send');
            """)
            if len((work/'native-inputs.jsonl').read_text().splitlines())!=len(inputs):
                raise AssertionError('close replayed a native Turn')
            print("PASS published assembled creation, explicit READ/WRITE, waiting Interaction, guarded image/effort, verified large result, paged originals, uncertain draft without replay, Gul restart and root close",flush=True)

        finally:
            cleanup_errors = []
            if opened:
                try:
                    delivery.cli(config, browser, "close")
                except (AssertionError, subprocess.TimeoutExpired) as error:
                    cleanup_errors.append("Browser close failed: " + str(error)[:2000])
                    if browser_pid is not None:
                        command = subprocess.run(["ps", "-p", str(browser_pid), "-o", "command="], text=True, capture_output=True).stdout
                        if "--daemon-session=" in command and str(work / "pd") in command:
                            try:
                                os.kill(browser_pid, signal.SIGTERM)
                            except ProcessLookupError:
                                pass
            if process.poll() is None:
                try:
                    process.stdin.write("stop\n"); process.stdin.flush()
                except BrokenPipeError:
                    pass
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill(); process.wait(timeout=5)
            if process.returncode:
                cleanup_errors.append("published host failed: " + process.stderr.read()[-4000:])
            (work / "alive").unlink(missing_ok=True)
            time.sleep(0.5)
            if cleanup_errors:
                if sys.exc_info()[0] is None:
                    raise RuntimeError("\n".join(cleanup_errors))
                print("\n".join(cleanup_errors), flush=True)

if __name__ == "__main__":
    main()
