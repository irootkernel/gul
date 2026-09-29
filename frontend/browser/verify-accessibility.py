"""Selected real-browser E7-T3 check; uses existing Bun and playwright-cli."""
import functools
import http.server
import pathlib
import subprocess
import tempfile
import threading

ROOT = pathlib.Path(__file__).resolve().parents[2]
SESSION = "gul-a11y"


def cli(directory, *args):
    result = subprocess.run(["playwright-cli", f"-s={SESSION}", *args], cwd=directory, text=True, capture_output=True)
    assert result.returncode == 0 and "### Error" not in result.stdout, result.stdout + result.stderr


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


with tempfile.TemporaryDirectory(prefix="gul-e7-accessibility-") as directory:
    for name in ("actions", "activity", "files"):
        target = pathlib.Path(directory, name)
        target.mkdir()
        subprocess.run(["bun", "build", f"frontend/browser/{name}.tsx", "--target", "browser", "--outdir", str(target)],
                       cwd=ROOT, check=True, capture_output=True)
        style = f'<link rel="stylesheet" href="{name}.css">' if name != "actions" else ""
        (target / "index.html").write_text(f'<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1">{style}</head><body><div id="root"></div><script type="module" src="{name}.js"></script></body></html>')
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(QuietHandler, directory=directory))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = f"http://127.0.0.1:{server.server_port}"
    try:
        cli(directory, "open", url + "/actions/?case=prompt_ime")
        cli(directory, "run-code", """async page => {
          const input = page.getByRole('textbox', {name:'Prompt'});
          await input.fill('한글 입력');
          await input.dispatchEvent('compositionstart', {data:'ㅎ'});
          await page.locator('form').evaluate(form => form.requestSubmit());
          if (await page.evaluate(() => window.fixture.calls) !== 0) throw Error('Composition-only prompt submitted');
          await input.dispatchEvent('keydown', {key:'Enter', code:'Enter', isComposing:true, keyCode:229});
          await page.locator('form').evaluate(form => form.requestSubmit());
          if (await page.evaluate(() => window.fixture.calls) !== 0 || await input.inputValue() !== '한글 입력')
            throw Error('Prompt submitted or draft changed during composition');
          await input.dispatchEvent('compositionend', {data:'한글 입력'});
          await page.locator('form').evaluate(form => form.requestSubmit());
          if (await page.evaluate(() => window.fixture.calls) !== 0) throw Error('Composition Enter submitted after compositionend');
          await input.dispatchEvent('keyup', {key:'Enter', code:'Enter'});
          await page.getByRole('button', {name:'Send prompt'}).focus();
          await page.keyboard.press('Enter');
          await page.waitForFunction(() => window.fixture.calls === 1);
          if (await page.evaluate(() => window.fixture.prompt) !== '한글 입력' || await input.inputValue() !== '')
            throw Error('Prompt was lost or duplicated after explicit keyboard submit');
        }""")
        cli(directory, "goto", url + "/actions/?case=answer_ime")
        cli(directory, "run-code", """async page => {
          const input = page.getByRole('textbox', {name:'Answer for question 1: Korean answer'});
          await input.fill('한글 입력');
          await input.dispatchEvent('compositionstart', {data:'ㅎ'});
          await page.locator('form').evaluate(form => form.requestSubmit());
          if (await page.evaluate(() => window.fixture.calls) !== 0) throw Error('Composition-only answer submitted');
          await input.dispatchEvent('keydown', {key:'Enter', code:'Enter', isComposing:true, keyCode:229});
          await page.locator('form').evaluate(form => form.requestSubmit());
          await input.dispatchEvent('compositionend', {data:'한글 입력'});
          await page.locator('form').evaluate(form => form.requestSubmit());
          if (await page.evaluate(() => window.fixture.calls) !== 0 || await input.inputValue() !== '한글 입력')
            throw Error('Answer submitted or changed during composition');
          await input.dispatchEvent('keyup', {key:'Enter', code:'Enter'});
          await input.focus();
          await page.keyboard.press('Enter');
          await page.locator('[data-outcome="answer"]').waitFor();
          if (await page.evaluate(() => window.fixture.calls) !== 1 || !await page.evaluate(() => window.fixture.matched))
            throw Error('Korean answer not sent exactly once after composition');
        }""")
        for scenario in ("prompt_ime", "answer_ime"):
            cli(directory, "goto", url + "/actions/?case=" + scenario)
            cli(directory, "run-code", f"""async page => {{
              const input = page.getByRole('textbox', {{name:'{'Prompt' if scenario == 'prompt_ime' else 'Answer for question 1: Korean answer'}'}});
              await input.fill('한글 입력');
              await input.dispatchEvent('compositionstart', {{data:'ㅎ'}});
              await input.dispatchEvent('compositionend', {{data:'한글 입력'}});
              await input.dispatchEvent('keydown', {{key:'Enter', code:'Enter', keyCode:13, isComposing:false}});
              await page.locator('form').evaluate(form => form.requestSubmit());
              await page.getByRole('button', {{name:'{'Send prompt' if scenario == 'prompt_ime' else 'Send answer'}'}}).evaluate(button => button.click());
              if (await page.evaluate(() => window.fixture.calls) !== 0 || await input.inputValue() !== '한글 입력')
                throw Error('WebKit-order commit submitted');
              await input.dispatchEvent('keyup', {{key:'Enter', code:'Enter', keyCode:13}});
              await page.getByRole('button', {{name:'{'Send prompt' if scenario == 'prompt_ime' else 'Send answer'}'}}).click();
              await page.waitForFunction(() => window.fixture.calls === 1);
            }}""")
        cli(directory, "goto", url + "/actions/?case=prompt_ime")
        cli(directory, "run-code", """async page => {
          const input = page.getByRole('textbox', {name:'Prompt'});
          await input.fill('한글 입력');
          await input.dispatchEvent('compositionstart', {data:'ㅎ'});
          await input.dispatchEvent('keydown', {key:'Process', code:'KeyA', keyCode:229, isComposing:true});
          await input.dispatchEvent('compositionend', {data:'한글 입력'});
          await page.getByRole('button', {name:'Send prompt'}).click();
          await page.waitForFunction(() => window.fixture.calls === 1);
          if (await page.evaluate(() => window.fixture.prompt) !== '한글 입력') throw Error('Non-Enter IME key stranded draft');
        }""")
        cli(directory, "goto", url + "/actions/?case=prompt_ime")
        cli(directory, "run-code", """async page => {
          const input = page.getByRole('textbox', {name:'Prompt'});
          await input.fill('한글 입력');
          await input.dispatchEvent('compositionstart', {data:'ㅎ'});
          await input.dispatchEvent('keydown', {key:'Enter', code:'Enter', keyCode:229, isComposing:true});
          await input.dispatchEvent('compositionend', {data:'한글 입력'});
          await page.getByRole('button', {name:'Send prompt'}).click();
          await page.waitForFunction(() => window.fixture.calls === 1);
          if (await page.evaluate(() => window.fixture.prompt) !== '한글 입력') throw Error('Lost keyup stranded explicit pointer submit');
        }""")
        for scenario, key in (("prompt_ime", "Enter"), ("answer_ime", "Space")):
            cli(directory, "goto", url + "/actions/?case=" + scenario)
            cli(directory, "run-code", f"""async page => {{
              const input = page.getByRole('textbox', {{name:'{'Prompt' if scenario == 'prompt_ime' else 'Answer for question 1: Korean answer'}'}});
              await input.fill('한글 입력');
              await input.dispatchEvent('compositionstart', {{data:'ㅎ'}});
              await input.dispatchEvent('keydown', {{key:'Enter', code:'Enter', keyCode:229, isComposing:true}});
              await input.dispatchEvent('compositionend', {{data:'한글 입력'}});
              await page.getByRole('button', {{name:'{'Send prompt' if scenario == 'prompt_ime' else 'Send answer'}'}}).focus();
              await page.keyboard.press('{key}');
              await page.waitForFunction(() => window.fixture.calls === 1);
              if ({'await page.evaluate(() => window.fixture.prompt) !== "한글 입력"' if scenario == 'prompt_ime' else '!await page.evaluate(() => window.fixture.matched)'})
                throw Error('Lost keyup stranded explicit keyboard submit');
            }}""")
        for scenario in ("prompt_ime", "answer_ime"):
            cli(directory, "goto", url + "/actions/?case=" + scenario)
            cli(directory, "run-code", f"""async page => {{
              const input = page.getByRole('textbox', {{name:'{'Prompt' if scenario == 'prompt_ime' else 'Answer for question 1: Korean answer'}'}});
              await input.fill('한글 입력');
              await input.dispatchEvent('compositionstart', {{data:'ㅎ'}});
              await page.locator('form').evaluate(form => form.requestSubmit());
              if (await page.evaluate(() => window.fixture.calls) !== 0) throw Error('Active composition submitted');
              await page.getByRole('button', {{name:'{'Send prompt' if scenario == 'prompt_ime' else 'Send answer'}'}}).click();
              await page.waitForFunction(() => window.fixture.calls === 1);
              if ({'await page.evaluate(() => window.fixture.prompt) !== "한글 입력"' if scenario == 'prompt_ime' else '!await page.evaluate(() => window.fixture.matched)'})
                throw Error('Lost compositionend stranded explicit submit');
            }}""")
        cli(directory, "goto", url + "/actions/?case=answer")
        cli(directory, "run-code", """async page => {
          const labels = await page.locator('input[name^="answer-"]').evaluateAll(inputs => inputs.map(input => input.getAttribute('aria-label')));
          if (JSON.stringify(labels) !== JSON.stringify(['Answer for question 1: Prototype question', 'Answer for question 2: Reset question']))
            throw Error('User-input questions lack distinct accessible names');
        }""")
        for width, height in ((820, 1080), (390, 844)):
            for scenario in ("prompt_ime", "answer_ime"):
                cli(directory, "goto", url + "/actions/?case=" + scenario)
                cli(directory, "run-code", f"""async page => {{
                  await page.setViewportSize({{width:{width}, height:{height}}});
                  const input = page.getByRole('textbox', {{name:{'"Prompt"' if scenario == 'prompt_ime' else '"Answer for question 1: Korean answer"'}}});
                  await input.fill('한글 입력');
                  await input.dispatchEvent('compositionstart', {{data:'ㅎ'}});
                  await page.locator('form').evaluate(form => form.requestSubmit());
                  if (await page.evaluate(() => window.fixture.calls) !== 0) throw Error('Composition-only submit at {width}px');
                  await input.dispatchEvent('keydown', {{key:'Enter', code:'Enter', isComposing:true, keyCode:229}});
                  await page.locator('form').evaluate(form => form.requestSubmit());
                  await input.dispatchEvent('compositionend', {{data:'한글 입력'}});
                  await page.locator('form').evaluate(form => form.requestSubmit());
                  if (await page.evaluate(() => window.fixture.calls) !== 0 || await input.inputValue() !== '한글 입력')
                    throw Error('Composition submitted at {width}px');
                  await input.dispatchEvent('keyup', {{key:'Enter', code:'Enter'}});
                  await page.getByRole('button', {{name:'{'Send prompt' if scenario == 'prompt_ime' else 'Send answer'}'}}).click();
                  await page.waitForFunction(() => window.fixture.calls === 1);
                  if ({'await page.evaluate(() => window.fixture.prompt) !== "한글 입력"' if scenario == 'prompt_ime' else '!await page.evaluate(() => window.fixture.matched)'})
                    throw Error('Korean submission failed at {width}px');
                }}""")
        for width, height in ((1280, 800), (820, 1080), (390, 844)):
            cli(directory, "goto", url + "/activity/?integrated")
            cli(directory, "run-code", f"""async page => {{
              await page.setViewportSize({{width:{width}, height:{height}}});
              if ({width} < 800) {{await page.getByRole('button', {{name:'Sessions', exact:true}}).focus(); await page.keyboard.press('Enter');}}
              await page.getByRole('button', {{name:'Fixture Session'}}).focus();
              await page.keyboard.press('Enter');
              await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'Chat');
              if (await page.evaluate(() => getComputedStyle(document.activeElement).outlineWidth) === '0px')
                throw Error('Chat destination has no visible focus at {width}px');
              await page.getByRole('button', {{name:'Prompt History', exact:true}}).focus();
              await page.keyboard.press('Enter');
              await page.getByRole('button', {{name:'Open matching Turn for prompt 1'}}).focus();
              await page.keyboard.press('Enter');
              await page.waitForFunction(() => document.activeElement?.textContent?.trim()?.startsWith('Linked Turn: repeat'));
              if (await page.evaluate(() => getComputedStyle(document.activeElement).outlineWidth) === '0px')
                throw Error('Linked Turn has no visible focus at {width}px');
              await page.getByRole('button', {{name:'Prompt History', exact:true}}).focus();
              await page.keyboard.press('Enter');
              await page.getByRole('button', {{name:'Open matching Turn for prompt 1'}}).focus();
              await page.keyboard.press('Enter');
              await page.waitForFunction(() => document.activeElement?.textContent?.trim()?.startsWith('Linked Turn: repeat'));
              if (!await page.getByRole('button', {{name:'View full response for conversation entry 2'}}).count())
                throw Error('Response action has no distinct accessible name');
              await page.getByRole('button', {{name:'Refresh conversation'}}).focus();
              await page.keyboard.press('Enter');
              await page.waitForFunction(() => window.fixture.conversation >= 2);
              if (await page.evaluate(() => document.activeElement?.textContent?.trim()) !== 'Refresh conversation')
                throw Error('Refresh stole focus back to linked Turn');
              const bad = await page.locator('button, input, textarea, select').evaluateAll(els => els.filter(el =>
                !el.getAttribute('aria-label') && !el.labels?.length && !el.textContent?.trim()).length);
              if (bad) throw Error('Unlabelled operator controls');
              if ({width} < 800) {{
                await page.getByRole('button', {{name:'Files', exact:true}}).focus();
                await page.keyboard.press('Enter');
                await page.keyboard.press('Tab');
                if (!(await page.evaluate(() => document.activeElement?.textContent?.trim()))?.includes('Refresh files'))
                  throw Error('Mobile navigation entered a hidden pane');
                const focus = await page.evaluate(() => getComputedStyle(document.activeElement).outlineWidth);
                if (focus === '0px') throw Error('Keyboard focus indicator is missing');
              }}
            }}""")
        for width, height in ((1280, 800), (820, 1080), (390, 844)):
            cli(directory, "goto", url + "/files/")
            cli(directory, "run-code", f"""async page => {{
              await page.setViewportSize({{width:{width}, height:{height}}});
              if ({width} < 800) await page.getByRole('button', {{name:'Files', exact:true}}).click();
              await page.getByRole('button', {{name:'docs', exact:false}}).click();
              await page.getByRole('navigation', {{name:'File path'}}).waitFor();
              if (await page.evaluate(() => document.activeElement?.getAttribute('aria-label')) !== 'File path')
                throw Error('Directory navigation lost path focus');
              if (await page.evaluate(() => getComputedStyle(document.activeElement).outlineWidth) === '0px')
                throw Error('File path has no visible focus at {width}px');
              await page.getByRole('button', {{name:'README.md'}}).click();
              if (await page.evaluate(() => document.activeElement?.textContent?.trim()) !== 'Back to explorer')
                throw Error('Preview did not focus Back');
              await page.getByRole('button', {{name:'Back to explorer'}}).click();
              if (await page.evaluate(() => document.activeElement?.textContent?.trim()) !== 'README.md')
                throw Error('Explorer did not restore file focus');
              await page.getByRole('button', {{name:'Parent directory'}}).click();
              if (await page.evaluate(() => document.activeElement?.getAttribute('aria-label')) !== 'File path')
                throw Error('Parent navigation lost path focus');
            }}""")
        for suffix, expected in (("", "Linked Turn: repeat"), ("&linked-failure", "The linked Turn is unavailable.")):
            cli(directory, "goto", url + "/activity/?integrated" + suffix)
            cli(directory, "run-code", f"""async page => {{
              await page.setViewportSize({{width:1280, height:800}});
              await page.getByRole('button', {{name:'Prompt History', exact:true}}).click();
              await page.getByRole('button', {{name:'More accepted prompts'}}).click();
              if (!await page.getByRole('button', {{name:'View full original prompt 2'}}).count())
                throw Error('Original-prompt action has no distinct accessible name');
              await page.getByRole('button', {{name:'Open matching Turn for prompt 2'}}).focus();
              await page.keyboard.press('Enter');
              await page.waitForFunction(() => document.activeElement?.textContent?.trim()?.startsWith('{expected}'));
              if (!await page.getByText('{expected}', {{exact:false}}).count()) throw Error('Unloaded Turn destination missing');
            }}""")
        cli(directory, "goto", url + "/activity/?integrated&linked-delay")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'More accepted prompts'}).click();
          await page.getByRole('button', {name:'Open matching Turn for prompt 2'}).click();
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.waitForFunction(() => window.fixture.entry === 1);
          await page.waitForTimeout(550);
          await page.getByRole('button', {name:'Conversation', exact:true}).click();
          await page.waitForFunction(() => document.activeElement?.textContent?.trim()?.startsWith('Linked Turn: repeat'));
        }""")
        cli(directory, "goto", url + "/activity/?integrated&linked-delay&input-card")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Prompt History', exact:true}).click();
          await page.getByRole('button', {name:'More accepted prompts'}).click();
          await page.getByRole('button', {name:'Open matching Turn for prompt 2'}).click();
          const input = page.getByRole('textbox', {name:'Answer for question 1: Korean answer'});
          await input.fill('한글 입력');
          await page.waitForFunction(() => window.fixture.entry === 1);
          await page.waitForTimeout(550);
          if (!await input.evaluate(element => element === document.activeElement) || await input.inputValue() !== '한글 입력')
            throw Error('Delayed linked Turn stole answer focus');
        }""")
        cli(directory, "goto", url + "/activity/?integrated&workspace-untyped")
        cli(directory, "run-code", """async page => {
          await page.getByRole('button', {name:'Retry workspace load'}).waitFor();
          await page.keyboard.press('Tab');
          if (await page.evaluate(() => document.activeElement?.textContent?.trim()) !== 'Retry workspace load' ||
              await page.evaluate(() => getComputedStyle(document.activeElement).outlineWidth) === '0px')
            throw Error('Workspace recovery button has no visible keyboard focus');
        }""")
        print('PASS real Chrome: Korean composition, explicit keyboard submit, desktop/tablet/phone focus and semantic controls')
    finally:
        cli(directory, "close")
        server.shutdown()
        server.server_close()
