// Explicit opt-in: PWA.install can create macOS app shims outside the temporary profile.
import { createRequire } from "node:module";
import path from "node:path";

if (process.env.GUL_RUN_INSTALLED_PWA_TEST !== "1") {
  throw new Error("Installed PWA test requires GUL_RUN_INSTALLED_PWA_TEST=1");
}

const [origin, pin, profile] = process.argv.slice(2);
if (!origin || !/^https:\/\/127\.0\.0\.1:\d+$/.test(origin) ||
    !/^[A-Za-z0-9+/]{43}=$/.test(pin) || !path.isAbsolute(profile) ||
    !profile.includes("gul-e8-delivery-")) {
  throw new Error("Installed PWA test requires the isolated HTTPS fixture and temporary profile");
}

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");
async function bounded(label, operation, milliseconds = 30000) {
  let timer;
  try {
    return await Promise.race([
      operation,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timed out; outcome unknown`)), milliseconds);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

const context = await chromium.launchPersistentContext(profile, {
  channel: "chrome",
  headless: false,
  timeout: 30000,
  viewport: { width: 390, height: 844 },
  args: [`--ignore-certificate-errors-spki-list=${pin}`],
});
context.setDefaultTimeout(15000);
context.setDefaultNavigationTimeout(15000);

let cdp;
let manifestId;
let installAttempted = false;
let failure;
try {
  const page = context.pages()[0] ?? await context.newPage();
  await page.goto(origin);
  await page.getByRole("heading", { name: "Sign in to Gul" }).waitFor();
  await page.getByLabel("Password", { exact: true }).fill("chrome fixture password 2026");
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByRole("main", { name: "Gul operator" }).waitFor();

  cdp = await context.newCDPSession(page); // Playwright's local pipe is the privileged CDP transport.
  const installability = await cdp.send("Page.getInstallabilityErrors");
  if (installability.installabilityErrors?.length) {
    throw new Error(`Chrome installability: ${JSON.stringify(installability.installabilityErrors)}`);
  }
  manifestId = await page.evaluate(() => location.origin + "/");
  installAttempted = true;
  await bounded("PWA.install", cdp.send("PWA.install", { manifestId }));
  await bounded("PWA standalone setting", cdp.send("PWA.changeAppUserSettings", {
    manifestId, displayMode: "standalone",
  }));

  const [launched, app] = await Promise.all([
    bounded("PWA.launch", cdp.send("PWA.launch", { manifestId })),
    context.waitForEvent("page", { timeout: 20000 }),
  ]);
  if (!launched.targetId) throw new Error("Installed PWA launch returned no target");
  await app.getByRole("main", { name: "Gul operator" }).waitFor();
  if (!await app.evaluate(() => matchMedia("(display-mode: standalone)").matches)) {
    throw new Error("Installed PWA did not launch in standalone display mode");
  }

  await app.getByRole("button", { name: "Files", exact: true }).click();
  if (await app.evaluate(() => sessionStorage.getItem("gul.presentation.tab")) !== "files") {
    throw new Error("Installed PWA did not retain the safe presentation tab");
  }
  const auth = app.waitForResponse(response => response.url().endsWith("/gul.v1.AuthService/GetSession"));
  const navigation = app.waitForResponse(response => response.url().endsWith("/gul.v1.WorkspacePresentationService/GetNavigation"));
  await app.reload();
  await Promise.all([auth, navigation]);
  await app.getByRole("main", { name: "Gul operator" }).waitFor();
  if (await app.getByRole("button", { name: "Files", exact: true }).getAttribute("aria-current") !== "page") {
    throw new Error("Installed PWA refresh lost presentation tab");
  }
  const storage = await app.evaluate(async () => ({
    controlled: !!navigator.serviceWorker.controller,
    cache: await caches.keys(),
    local: localStorage.length,
    session: Object.keys(sessionStorage),
  }));
  if (!storage.controlled || storage.cache.length || storage.local ||
      storage.session.join(",") !== "gul.presentation.tab") {
    throw new Error(`Installed PWA used unexpected browser storage: ${JSON.stringify(storage)}`);
  }
  await app.getByRole("button", { name: "Sign out" }).click();
  await app.getByRole("heading", { name: "Sign in to Gul" }).waitFor();
  await app.reload();
  await app.getByRole("heading", { name: "Sign in to Gul" }).waitFor();
  if (await app.getByRole("main", { name: "Gul operator" }).count()) {
    throw new Error("Installed PWA showed protected content after sign-out");
  }
} catch (error) {
  failure = error;
} finally {
  if (installAttempted) {
    try {
      await bounded("PWA.uninstall", cdp.send("PWA.uninstall", { manifestId }));
    } catch (error) {
      failure = failure ? new AggregateError([failure, error], "PWA check and uninstall failed; OS state unknown") : error;
    }
  }
  try {
    await bounded("Chrome close", context.close(), 20000);
  } catch (error) {
    failure = failure ? new AggregateError([failure, error], "PWA check and Chrome close failed; process state unknown") : error;
  }
}
if (failure) throw failure;
console.log("PASS installed standalone PWA: launch, refresh with fresh auth/navigation, safe storage, sign-out, uninstall");
