import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {validateFrontendConfig} from "./validate-frontend-config.mjs";

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const read = name => JSON.parse(fs.readFileSync(path.join(sourceRoot, "frontend", name), "utf8"));
const browser = read("tsconfig.json");
const test = read("tsconfig.test.json");

assert.doesNotThrow(() => validateFrontendConfig(browser, test));
for (const [name, mutate, pattern] of [
  ["Bun globals enter browser types", value => { value.browser.compilerOptions.types.push("bun"); }, /browser types drifted/],
  ["browser source leaves production program", value => { value.browser.include = ["src/app.tsx"]; }, /browser includes drifted/],
  ["tests lose Bun globals", value => { value.test.compilerOptions.types.shift(); }, /test types drifted/],
]) {
  const fixture = {browser: structuredClone(browser), test: structuredClone(test)};
  mutate(fixture);
  assert.throws(() => validateFrontendConfig(fixture.browser, fixture.test), pattern, name);
}

console.log("frontend TypeScript config fixtures passed");
