import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {validateProductMakefile} from "./validate-product-makefile.mjs";

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const current = fs.readFileSync(path.join(sourceRoot, "Makefile"), "utf8");

assert.doesNotThrow(() => validateProductMakefile(current));
for (const [name, mutate, pattern] of [
  ["generation skips package validation", source => source.replace("\t@node scripts/validate-product-package.mjs\n", ""), /generate-frontend recipe drifted/],
  ["drift check skips the builder", source => source.replace("\t@bun frontend\/scripts\/check-bundle.mjs", "\t@true"), /frontend-check recipe drifted/],
  ["test-unit bypasses the package command", source => source.replace("\t@bun run test", "\t@bun test frontend"), /package-owned frontend test/],
  ["test-unit drops typechecking", source => source.replace("\t@bun run typecheck\n", ""), /retain frontend typechecking/],
  ["test-int drops bundle drift", source => source.replace("\t@\$\(MAKE\) --no-print-directory frontend-check\n", ""), /retain the frontend drift gate/],
  ["test-int drops API drift", source => source.replace("\t@\$\(MAKE\) --no-print-directory api-check\n", ""), /retain the Gul API drift gate/],
  ["generation skips accepted errors", source => source.replace("\t@node contract/port/generate-errors.mjs generate\n", ""), /generate-api recipe drifted/],
  ["check skips accepted errors", source => source.replace("\t@node contract/port/generate-errors.mjs check\n", ""), /api-check recipe drifted/],
]) {
  assert.throws(() => validateProductMakefile(mutate(current)), pattern, name);
}

console.log("product Makefile fixtures passed");
