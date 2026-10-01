import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {fileURLToPath} from "node:url";

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const frontendRoot = path.join(sourceRoot, "frontend/src");
const frontend = sourceFiles(frontendRoot).map(name => fs.readFileSync(name, "utf8")).join("\n");
const html = fs.readFileSync(path.join(sourceRoot, "frontend/index.html"), "utf8");
const delivery = goSourceFiles(path.join(sourceRoot, "internal/delivery")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const desktop = goSourceFiles(path.join(sourceRoot, "internal/desktop")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const command = goSourceFiles(path.join(sourceRoot, "cmd")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const app = goSourceFiles(path.join(sourceRoot, "internal/app")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const domain = goSourceFiles(path.join(sourceRoot, "internal/domain")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const storage = goSourceFiles(path.join(sourceRoot, "internal/storage")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const workspace = goSourceFiles(path.join(sourceRoot, "internal/workspace"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const workspaceAdapter = goSourceFiles(path.join(sourceRoot, "internal/workspace/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const presentation = goSourceFiles(path.join(sourceRoot, "internal/presentation"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const session = goSourceFiles(path.join(sourceRoot, "internal/session"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const sessionAdapter = goSourceFiles(path.join(sourceRoot, "internal/session/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const launch = goSourceFiles(path.join(sourceRoot, "internal/launch"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const launchAdapter = goSourceFiles(path.join(sourceRoot, "internal/launch/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const observation = goSourceFiles(path.join(sourceRoot, "internal/observation"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const observationAdapter = goSourceFiles(path.join(sourceRoot, "internal/observation/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const interaction = goSourceFiles(path.join(sourceRoot, "internal/interaction"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const interactionAdapter = goSourceFiles(path.join(sourceRoot, "internal/interaction/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const action = goSourceFiles(path.join(sourceRoot, "internal/action"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const actionAdapter = goSourceFiles(path.join(sourceRoot, "internal/action/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const history = goSourceFiles(path.join(sourceRoot, "internal/history"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const historyAdapter = goSourceFiles(path.join(sourceRoot, "internal/history/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const reconnect = goSourceFiles(path.join(sourceRoot, "internal/reconnect"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const reconnectAdapter = goSourceFiles(path.join(sourceRoot, "internal/reconnect/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const sessionclose = goSourceFiles(path.join(sourceRoot, "internal/sessionclose"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const recovery = goSourceFiles(path.join(sourceRoot, "internal/recovery"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const operation = goSourceFiles(path.join(sourceRoot, "internal/operation"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const replay = goSourceFiles(path.join(sourceRoot, "internal/replay"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const mutation = goSourceFiles(path.join(sourceRoot, "internal/mutation"))
  .filter(name => !name.includes(`${path.sep}contractprovider${path.sep}`))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const mutationAdapter = goSourceFiles(path.join(sourceRoot, "internal/mutation/contractprovider"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const files = goSourceFiles(path.join(sourceRoot, "internal/files"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const host = goSourceFiles(path.join(sourceRoot, "internal/host"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const deployment = goSourceFiles(path.join(sourceRoot, "internal/deployment"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const auth = goSourceFiles(path.join(sourceRoot, "internal/auth"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const browserHelper = goSourceFiles(path.join(sourceRoot, "frontend/browser"))
  .map(name => fs.readFileSync(name, "utf8")).join("\n");
const composition = goSourceFiles(path.join(sourceRoot, "internal/composition")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const submit = goSourceFiles(path.join(sourceRoot, "internal/submit")).map(name => fs.readFileSync(name, "utf8")).join("\n");
const scenarioImport = /"github\.com\/rootkernel\/gul\/contract\/scenario(?:\/[^\"]+)?"/;

function sourceFiles(directory) {
  return fs.readdirSync(directory, {withFileTypes: true}).flatMap(entry => {
    const name = path.join(directory, entry.name);
    if (entry.isDirectory()) return sourceFiles(name);
    if (/\.(?:ts|tsx|js|jsx|mjs)$/.test(entry.name) && !/\.(?:test|d)\.(?:ts|tsx|js|jsx|mjs)$/.test(entry.name)) return [name];
    return [];
  });
}

function goSourceFiles(directory) {
  return fs.readdirSync(directory, {withFileTypes: true}).flatMap(entry => {
    const name = path.join(directory, entry.name);
    if (entry.isDirectory()) return goSourceFiles(name);
    if (entry.name.endsWith(".go") && !entry.name.endsWith("_test.go")) return [name];
    return [];
  });
}

const boundaries = [
  ["frontend", frontend, [/\bfetch\s*\(/, /\bWebSocket\b/, /\bXMLHttpRequest\b/, /\bEventSource\b/, /\bsendBeacon\s*\(/, /\bpostMessage\s*\(/, /\bimport\s*\(/, /\blocalStorage\b/, /\bindexedDB\b/, /contract\/generated/, /from ["'][^"']*wails/i]],
  ["frontend HTML", html, [/\bon[a-z]+\s*=/i, /\b(?:src|href)\s*=\s*["'](?:https?:)?\/\//i, /<script\b[^>]*>[^<]*\S[^<]*<\/script>/i]],
  ["delivery", delivery, [/\bListenAndServe\b/, /"database\/sql"/, /"github\.com\/wailsapp/, scenarioImport]],
  ["desktop", desktop, [/\bListenAndServe\b/, /"database\/sql"/, /"github\.com\/rootkernel\/gul\/contract\//, /"github\.com\/rootkernel\/gul\/internal\/storage"/, /\bNewService\s*\(/, scenarioImport]],
  ["command", command, [/\bListenAndServe\b/, /"database\/sql"/, /"github\.com\/rootkernel\/gul\/contract\//, /"github\.com\/rootkernel\/gul\/internal\/storage"/, /\bNewService\s*\(/, scenarioImport]],
  ["app", app, [/"github\.com\/rootkernel\/gul\/internal\/(?:delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /\bListenAndServe\b/, scenarioImport]],
  ["domain", domain, [/"github\.com\/rootkernel\/gul\/internal\//, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["storage", storage, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /\bListenAndServe\b/, scenarioImport]],
  ["workspace", workspace, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["workspace adapter", workspaceAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["presentation", presentation, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["session", session, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["session adapter", sessionAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["launch", launch, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["launch adapter", launchAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["observation", observation, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["observation adapter", observationAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["interaction", interaction, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["interaction adapter", interactionAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["action", action, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["reconnect", reconnect, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["reconnect adapter", reconnectAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["action adapter", actionAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["history", history, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["history adapter", historyAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["sessionclose", sessionclose, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["recovery", recovery, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["operation", operation, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["replay", replay, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["mutation", mutation, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["mutation adapter", mutationAdapter, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/api\/generated(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["files", files, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage)(?:\/|")/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/, scenarioImport]],
  ["host", host, [/"github\.com\/wailsapp/, /"database\/sql"/, /"github\.com\/rootkernel\/gul\/(?:contract|api\/generated)(?:\/|")/, /"github\.com\/rootkernel\/gul\/internal\/desktop(?:\/|")/, scenarioImport]],
  ["deployment", deployment, [/"github\.com\/wailsapp/, /"database\/sql"/, /"github\.com\/rootkernel\/gul\/(?:internal|contract|api)(?:\/|")/, scenarioImport]],
  // Auth consumes generated password bounds; it owns no runtime adapter.
  ["auth", auth, [/"github\.com\/wailsapp/, /"database\/sql"/, /"github\.com\/rootkernel\/gul\/contract(?:\/|")/, /"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop|storage|host)(?:\/|")/, scenarioImport]],
  ["browser helper", browserHelper, [/"github\.com\/wailsapp/, /"database\/sql"/, scenarioImport]],
];

for (const [label, source, forbidden] of boundaries) {
  for (const pattern of forbidden) assert.doesNotMatch(source, pattern, `${label} foundation gained ${pattern}`);
}

// The assembly layer can connect ports, but cannot introduce a production fixture.
for (const [label, source] of [["composition", composition], ["submit", submit], ...boundaries.slice(2).map(([label, source]) => [label, source])]) {
  assert.doesNotMatch(source, /"github\.com\/rootkernel\/gul\/test\/acceptance(?:\/[^"]*)?"/, `${label} imports an acceptance fixture`);
}
for (const pattern of [scenarioImport, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/]) assert.doesNotMatch(composition, pattern);
// Submit consumes generated text bounds like auth consumes password bounds.
for (const pattern of boundaries.find(([label]) => label === "auth")[2]) assert.doesNotMatch(submit, pattern);

const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-boundary-"));
try {
  const added = path.join(fixture, "added.jsx");
  fs.writeFileSync(added, "fetch('/api');\n");
  assert(sourceFiles(fixture).includes(added));
  assert.throws(() => assert.doesNotMatch(fs.readFileSync(added, "utf8"), boundaries[0][2][0]), /expected to not match/);
  const addedGo = path.join(fixture, "added.go");
  fs.writeFileSync(addedGo, 'package fixture\nimport "database/sql"\n');
  assert(goSourceFiles(fixture).includes(addedGo));
  for (const [label, source, forbidden] of [
    ["frontend", "new XMLHttpRequest()", boundaries[0][2]],
    ["frontend HTML", '<script src="https://example.invalid/a.js"></script>', boundaries[1][2]],
    ["delivery", '"database/sql"', boundaries[2][2]],
    ["desktop", '"github.com/rootkernel/gul/internal/storage"', boundaries[3][2]],
    ["command", '"github.com/rootkernel/gul/contract/generated"', boundaries[4][2]],
    ["app", '"github.com/rootkernel/gul/internal/desktop"', boundaries[5][2]],
    ["app", '"github.com/rootkernel/gul/contract/port"', boundaries[5][2]],
    ["app", '"github.com/rootkernel/gul/api/generated/go/gul/v1"', boundaries[5][2]],
    ["domain", '"github.com/rootkernel/gul/internal/app"', boundaries[6][2]],
    ["storage", '"github.com/rootkernel/gul/internal/delivery/web"', boundaries[7][2]],
    ["storage", '"github.com/rootkernel/gul/contract/port"', boundaries[7][2]],
    ["storage", '"github.com/rootkernel/gul/api/generated/go/gul/v1"', boundaries[7][2]],
    ["workspace", '"github.com/rootkernel/gul/internal/storage"', boundaries[8][2]],
    ["workspace", '"github.com/rootkernel/gul/contract/port"', boundaries[8][2]],
    ["workspace adapter", '"github.com/rootkernel/gul/internal/storage"', boundaries[9][2]],
    ["presentation", '"github.com/rootkernel/gul/internal/storage"', boundaries[10][2]],
    ...boundaries.slice(15, -4).flatMap(([label, , forbidden]) => [
      [label, '"github.com/rootkernel/gul/internal/storage"', forbidden],
      [label, '"github.com/rootkernel/gul/api/generated/go/gul/v1"', forbidden],
      [label, '"database/sql"', forbidden],
      ...(label.endsWith(" adapter") ? [] : [[label, '"github.com/rootkernel/gul/contract/port"', forbidden]]),
    ]),
    ...boundaries.slice(-4).flatMap(([label, , forbidden]) => [
      [label, '"github.com/wailsapp/wails/v3/pkg/application"', forbidden],
      [label, '"database/sql"', forbidden],
      ...(label === "browser helper" ? [] : [[label, '"github.com/rootkernel/gul/contract/generated"', forbidden]]),
    ]),
    ...boundaries.slice(2).map(([label, , forbidden]) => [label, '"github.com/rootkernel/gul/contract/scenario"', forbidden]),
    ...boundaries.slice(2).map(([label, , forbidden]) => [label, '"github.com/rootkernel/gul/contract/scenario/helper"', forbidden]),
  ]) {
    assert(forbidden.some(pattern => pattern.test(source)), `${label} negative fixture must match a forbidden pattern`);
  }
} finally {
  fs.rmSync(fixture, {recursive: true, force: true});
}

console.log("frontend, delivery, desktop, command, app, domain, storage, workspace, presentation, session, launch, observation, interaction, action, history, reconnect, sessionclose, recovery, operation, replay, mutation, files, host, deployment, auth, and browser helper foundation boundaries passed");
