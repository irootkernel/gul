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
  ["delivery", delivery, [/\bListenAndServe\b/, /"database\/sql"/, /"github\.com\/wailsapp/]],
  ["desktop", desktop, [/\bListenAndServe\b/, /"database\/sql"/, /"github\.com\/rootkernel\/gul\/contract\//, /"github\.com\/rootkernel\/gul\/internal\/storage"/, /\bNewService\s*\(/]],
  ["command", command, [/\bListenAndServe\b/, /"database\/sql"/, /"github\.com\/rootkernel\/gul\/contract\//, /"github\.com\/rootkernel\/gul\/internal\/storage"/, /\bNewService\s*\(/]],
  ["app", app, [/"github\.com\/rootkernel\/gul\/internal\/(?:delivery|desktop|storage)(?:\/|")/, /"github\.com\/wailsapp/, /\bListenAndServe\b/]],
  ["domain", domain, [/"github\.com\/rootkernel\/gul\/internal\//, /"github\.com\/wailsapp/, /"database\/sql"/, /\bListenAndServe\b/]],
  ["storage", storage, [/"github\.com\/rootkernel\/gul\/internal\/(?:app|delivery|desktop)(?:\/|")/, /"github\.com\/wailsapp/, /\bListenAndServe\b/]],
];

for (const [label, source, forbidden] of boundaries) {
  for (const pattern of forbidden) assert.doesNotMatch(source, pattern, `${label} foundation gained ${pattern}`);
}

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
    ["domain", '"github.com/rootkernel/gul/internal/app"', boundaries[6][2]],
    ["storage", '"github.com/rootkernel/gul/internal/delivery/web"', boundaries[7][2]],
  ]) {
    assert(forbidden.some(pattern => pattern.test(source)), `${label} negative fixture must match a forbidden pattern`);
  }
} finally {
  fs.rmSync(fixture, {recursive: true, force: true});
}

console.log("frontend, delivery, desktop, command, app, domain, and storage foundation boundaries passed");
