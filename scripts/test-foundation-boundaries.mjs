import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {fileURLToPath} from "node:url";

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const frontendRoot = path.join(sourceRoot, "frontend/src");
const frontend = sourceFiles(frontendRoot).map(name => fs.readFileSync(name, "utf8")).join("\n");
const html = fs.readFileSync(path.join(sourceRoot, "frontend/index.html"), "utf8");
const delivery = fs.readFileSync(path.join(sourceRoot, "internal/delivery/web/bundle.go"), "utf8");

function sourceFiles(directory) {
  return fs.readdirSync(directory, {withFileTypes: true}).flatMap(entry => {
    const name = path.join(directory, entry.name);
    if (entry.isDirectory()) return sourceFiles(name);
    if (/\.(?:ts|tsx|js|jsx|mjs)$/.test(entry.name) && !/\.(?:test|d)\.(?:ts|tsx|js|jsx|mjs)$/.test(entry.name)) return [name];
    return [];
  });
}

const boundaries = [
  ["frontend", frontend, [/\bfetch\s*\(/, /\bWebSocket\b/, /\bXMLHttpRequest\b/, /\bEventSource\b/, /\bsendBeacon\s*\(/, /\bpostMessage\s*\(/, /\bimport\s*\(/, /\blocalStorage\b/, /\bindexedDB\b/, /contract\/generated/, /from ["'][^"']*wails/i]],
  ["frontend HTML", html, [/\bon[a-z]+\s*=/i, /\b(?:src|href)\s*=\s*["'](?:https?:)?\/\//i, /<script\b[^>]*>[^<]*\S[^<]*<\/script>/i]],
  ["delivery", delivery, [/\bListenAndServe\b/, /"database\/sql"/, /"github\.com\/wailsapp/]],
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
  for (const [label, source, forbidden] of [
    ["frontend", "new XMLHttpRequest()", boundaries[0][2]],
    ["frontend HTML", '<script src="https://example.invalid/a.js"></script>', boundaries[1][2]],
    ["delivery", '"database/sql"', boundaries[2][2]],
  ]) {
    assert(forbidden.some(pattern => pattern.test(source)), `${label} negative fixture must match a forbidden pattern`);
  }
} finally {
  fs.rmSync(fixture, {recursive: true, force: true});
}

console.log("frontend and delivery foundation boundaries passed");
