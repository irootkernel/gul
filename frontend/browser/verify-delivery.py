"""Real Chrome check of the checked frontend through an isolated Gul HTTPS host.

The temporary Chrome context ignores only this fixture's self-signed local
certificate. It does not qualify Tailscale Serve, native pinning, or devices.
"""

import base64
import hashlib
import json
import os
import pathlib
import signal
import socket
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


ROOT = pathlib.Path(__file__).resolve().parents[2]


def cli(config, session, *args):
    result = subprocess.run(
        ["playwright-cli", "--config", str(config), f"-s={session}", *args],
        cwd=config.parent, text=True, capture_output=True, timeout=45,
    )
    if result.returncode or "### Error" in result.stdout:
        raise AssertionError(result.stdout + result.stderr)
    return result.stdout


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def wait_for_host(origin, process, data):
    context = ssl._create_unverified_context()  # Fixture certificate only.
    for _ in range(240):
        if process.poll() is not None:
            state = [(str(path), oct(path.stat().st_mode & 0o777)) for path in [data, *data.glob("*")] if path.exists()]
            raise RuntimeError("isolated Gul host exited before readiness: " + process.stderr.read().decode() + str(state))
        try:
            with urllib.request.urlopen(origin + "/manifest.webmanifest", context=context, timeout=1) as response:
                if response.status == 200:
                    return
        except (urllib.error.URLError, TimeoutError):
            time.sleep(0.1)
    raise RuntimeError("isolated Gul host did not become ready")


def certificate_spki_pin(port):
    certificate = ssl.get_server_certificate(("127.0.0.1", port)).encode()
    public = subprocess.run(["openssl", "x509", "-pubkey", "-noout"], input=certificate,
                            capture_output=True, check=True).stdout
    spki = subprocess.run(["openssl", "pkey", "-pubin", "-outform", "DER"], input=public,
                          capture_output=True, check=True).stdout
    return base64.b64encode(hashlib.sha256(spki).digest()).decode()


def main():
    checked = ROOT / "internal/delivery/web/dist"
    manifest = json.loads((checked / "bundle-manifest.json").read_text())
    expected = {"index.html", "manifest.webmanifest", "service-worker.js", "icons/gul-192.png", "icons/gul-512.png"}
    if not expected <= {entry["path"] for entry in manifest["files"]}:
        raise RuntimeError("checked frontend bundle lacks E8 delivery assets; regenerate it before this check")
    with tempfile.TemporaryDirectory(prefix="gul-e8-delivery-") as temporary:
        work = pathlib.Path(temporary).resolve()
        data = work / "Gul"
        host_binary = work / "gul-host"
        setup_binary = work / "delivery-setup"
        subprocess.run(["go", "build", "-o", str(host_binary), "./cmd/gul"], cwd=ROOT, check=True, timeout=180, capture_output=True)
        subprocess.run(["go", "build", "-o", str(setup_binary), "./frontend/browser/delivery-setup.go"], cwd=ROOT, check=True, timeout=180, capture_output=True)
        port = free_port()
        origin = f"https://127.0.0.1:{port}"
        config = work / "playwright-cli.json"
        process = subprocess.Popen([str(host_binary), "serve", "--data-directory", str(data), "--port", str(port)],
                                   cwd=ROOT, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, start_new_session=True)
        forge = f"g8f{port}"
        browser = f"g8d{port}"
        opened_sessions = []
        primary_error = None
        cleanup_errors = []
        try:
            wait_for_host(origin, process, data)
            pin = certificate_spki_pin(port)
            config.write_text(json.dumps({"browser": {"browserName": "chromium", "isolated": False, "userDataDir": str(work / "chrome-profile"),
                "launchOptions": {"channel": "chrome", "args": [f"--ignore-certificate-errors-spki-list={pin}"]}}}))
            cli(config, forge, "open", origin)
            opened_sessions.append(forge)
            cli(config, forge, "run-code", """async page => {
              await page.getByText('Open Gul on this Mac to set your password.').waitFor();
              if (await page.getByRole('form', {name:'Set your Gul password'}).count()) throw Error('Ordinary browser got setup controls');
              const status = await page.evaluate(async () => (await fetch('/_gul/native-bootstrap', {
                method:'POST', headers:{'X-Gul-Native-Host':'f'.repeat(43), 'Content-Type':'application/json'},
                body:JSON.stringify({challenge:'c'.repeat(43)})})).status);
              if (status !== 403) throw Error('Forged browser native bootstrap status '+status);
              await page.addInitScript(() => {window.__GUL_NATIVE_BOOTSTRAP__ = {setupCredential:'f'.repeat(43)}});
              await page.reload();
              await page.getByRole('heading', {name:'Set your Gul password'}).waitFor();
              if (await page.evaluate(() => '__GUL_NATIVE_BOOTSTRAP__' in window)) throw Error('Native field survived startup');
              await page.getByLabel('Password', {exact:true}).fill('chrome fixture password 2026');
              await page.getByRole('button', {name:'Set password'}).click();
              await page.getByRole('alert').filter({hasText:'Setup could not finish'}).waitFor();
              if (await page.getByRole('heading', {name:'Sign in to Gul'}).count()) throw Error('Forged grant configured account');
            }""")
            cli(config, forge, "close")
            opened_sessions.remove(forge)

            denied = subprocess.run([str(setup_binary), str(data)], cwd=ROOT, env={k: v for k, v in os.environ.items() if k != "GUL_RUN_DELIVERY_FIXTURE"}, capture_output=True, timeout=20)
            if denied.returncode == 0 or b"opt-in isolated test fixture" not in denied.stderr:
                raise RuntimeError("setup fixture ran without explicit opt-in")
            setup_env = {**os.environ, "GUL_RUN_DELIVERY_FIXTURE": "1"}
            subprocess.run([str(setup_binary), str(data)], cwd=ROOT, env=setup_env, check=True, timeout=20)
            cli(config, browser, "open", origin)
            opened_sessions.append(browser)
            cli(config, browser, "resize", "390", "844")
            cli(config, browser, "run-code", """async page => {
              await page.getByRole('heading', {name:'Sign in to Gul'}).waitFor();
              const cdp = await page.context().newCDPSession(page);
              const manifest = await cdp.send('Page.getAppManifest');
              if (manifest.errors?.length || !manifest.data) throw Error('Chrome rejected PWA manifest: '+JSON.stringify(manifest.errors));
              const data = JSON.parse(manifest.data);
              if (data.name !== 'Gul' || data.display !== 'standalone' || data.icons?.length !== 2) throw Error('Incomplete installed manifest');
              const registration = await navigatorCheck(page);
              if (!registration.ok) throw Error('Service worker inactive: '+JSON.stringify(registration));
              if ((await page.evaluate(() => caches.keys())).length) throw Error('User data entered CacheStorage');
              async function navigatorCheck(page) {
                return page.evaluate(async () => {
                  const ready = await Promise.race([navigator.serviceWorker.ready, new Promise(resolve => setTimeout(() => resolve(null), 10000))]);
                  return {ok:!!ready?.active && ready.scope === location.origin + '/', state:ready?.active?.state,
                    scope:ready?.scope, secure:isSecureContext, registrations:(await navigator.serviceWorker.getRegistrations()).length};
                });
              }
            }""")
            cli(config, browser, "run-code", """async page => {
              await page.getByLabel('Password', {exact:true}).fill('chrome fixture password 2026');
              await page.getByRole('button', {name:'Sign in'}).click();
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              await page.getByRole('button', {name:'Files', exact:true}).click();
              if (await page.evaluate(() => sessionStorage.getItem('gul.presentation.tab')) !== 'files') throw Error('Safe tab was not retained');
              const auth = page.waitForResponse(response => response.url().endsWith('/gul.v1.AuthService/GetSession'));
              const nav = page.waitForResponse(response => response.url().endsWith('/gul.v1.WorkspacePresentationService/GetNavigation'));
              await page.reload();
              await Promise.all([auth, nav]);
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              if (await page.getByRole('button', {name:'Files', exact:true}).getAttribute('aria-current') !== 'page') throw Error('Browser refresh lost presentation tab');
              if (!await page.evaluate(() => navigator.serviceWorker.controller)) throw Error('Browser refresh is not SW controlled');
              const storage = await page.evaluate(async () => ({keys:await caches.keys(), local:localStorage.length,
                session:Object.keys(sessionStorage)}));
              if (storage.keys.length || storage.local || storage.session.join(',') !== 'gul.presentation.tab') throw Error('Unexpected user/runtime browser storage');
              const onlineOrigin = await page.evaluate(() => location.origin);
              await page.context().setOffline(true);
              try {
                try { await page.reload({waitUntil:'domcontentloaded'}); } catch (error) {
                  if (!String(error).includes('net::ERR_INTERNET_DISCONNECTED')) throw error;
                }
                if (await page.getByRole('main', {name:'Gul operator'}).count()) throw Error('Offline navigation exposed stale operator');
                if ((await page.evaluate(() => typeof caches === 'undefined' ? [] : caches.keys())).length) throw Error('Offline navigation populated CacheStorage');
              } finally {
                await page.context().setOffline(false);
              }
              const reconnectAuth = page.waitForResponse(response => response.url().endsWith('/gul.v1.AuthService/GetSession'));
              const reconnectNav = page.waitForResponse(response => response.url().endsWith('/gul.v1.WorkspacePresentationService/GetNavigation'));
              await page.goto(onlineOrigin + '/');
              await Promise.all([reconnectAuth, reconnectNav]);
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              await page.getByRole('button', {name:'Sign out'}).click();
              await page.getByRole('heading', {name:'Sign in to Gul'}).waitFor();
              if (await page.getByRole('main', {name:'Gul operator'}).count()) throw Error('Protected operator survived sign-out');
              await page.reload();
              await page.getByRole('heading', {name:'Sign in to Gul'}).waitFor();
              if (await page.getByRole('main', {name:'Gul operator'}).count()) throw Error('Offline/stale operator appeared after reload');
            }""")
            cli(config, browser, "run-code", """async page => {
              await page.getByLabel('Password', {exact:true}).fill('chrome fixture password 2026');
              await page.getByRole('button', {name:'Sign in'}).click();
              await page.getByRole('main', {name:'Gul operator'}).waitFor();
              const pageCdp = await page.context().newCDPSession(page);
              const installability = await pageCdp.send('Page.getInstallabilityErrors');
              if (installability.installabilityErrors?.length) throw Error('Chrome installability: '+JSON.stringify(installability.installabilityErrors));
            }""")
            if os.environ.get("GUL_RUN_INSTALLED_PWA_TEST") == "1":
                node_root = subprocess.run(["npm", "root", "-g"], capture_output=True, text=True, check=True, timeout=15).stdout.strip()
                installed_env = os.environ.copy()
                installed_env["NODE_PATH"] = node_root
                subprocess.run(["node", str(ROOT / "frontend/browser/verify-installed-pwa.mjs"), origin, pin,
                                str(work / "installed-profile")], cwd=work, env=installed_env, check=True)
        except BaseException as error:
            primary_error = error
        finally:
            for session in reversed(opened_sessions):
                try:
                    cli(config, session, "close")
                except Exception as error:
                    cleanup_errors.append(RuntimeError(f"Chrome session {session} did not close: {error}"))
            if process.poll() is None:
                try:
                    process.send_signal(signal.SIGINT)
                except ProcessLookupError:
                    cleanup_errors.append(RuntimeError("isolated Gul host exited before SIGINT"))
            else:
                cleanup_errors.append(RuntimeError("isolated Gul host exited before SIGINT"))
            try:
                process.communicate(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                try:
                    process.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    cleanup_errors.append(RuntimeError("isolated Gul host did not stop after kill"))
                cleanup_errors.append(RuntimeError("isolated Gul host did not exit within 10 seconds after SIGINT"))
            else:
                if process.returncode != 0:
                    cleanup_errors.append(RuntimeError(f"isolated Gul host exited with status {process.returncode}"))
        if primary_error and cleanup_errors:
            raise BaseExceptionGroup("Chrome delivery and cleanup failed", [primary_error, *cleanup_errors])
        if primary_error:
            raise primary_error
        if cleanup_errors:
            raise ExceptionGroup("Chrome delivery cleanup failed", cleanup_errors)
    installation = "installed PWA passed and removed" if os.environ.get("GUL_RUN_INSTALLED_PWA_TEST") == "1" else "installed PWA NOT RUN"
    print("PASS Chrome checked entry: HTTPS login, SW-controlled refresh, offline/reconnect fresh auth/navigation, network-only storage, logout, forged setup denial; " + installation)


if __name__ == "__main__":
    main()
