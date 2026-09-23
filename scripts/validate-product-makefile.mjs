import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";

const expectedRecipes = {
  "generate-frontend": [
    "@./scripts/toolchain-check.sh --bun-only",
    "@bun install --frozen-lockfile --ignore-scripts",
    "@node scripts/validate-product-package.mjs",
    "@bun frontend/scripts/build.mjs",
  ],
  "frontend-check": [
    "@./scripts/toolchain-check.sh --bun-only",
    "@bun install --frozen-lockfile --ignore-scripts",
    "@node scripts/validate-product-package.mjs",
    "@bun frontend/scripts/check-bundle.mjs",
  ],
  "generate-api": [
    "@./scripts/toolchain-check.sh --api-only",
    "@cd contract && bun install --frozen-lockfile --ignore-scripts",
    "@node api/scripts/api-command.mjs generate",
    "@node contract/port/generate-errors.mjs generate",
  ],
  "api-check": [
    "@./scripts/toolchain-check.sh --api-only",
    "@cd contract && bun install --frozen-lockfile --ignore-scripts",
    "@node api/scripts/api-command.mjs check",
    "@node contract/port/generate-errors.mjs check",
  ],
};

export function validateProductMakefile(source) {
  for (const [target, expected] of Object.entries(expectedRecipes)) {
    const recipe = readRecipe(source, target);
    if (JSON.stringify(recipe) !== JSON.stringify(expected)) {
      throw new Error(`${target} recipe drifted`);
    }
  }
  const testUnit = readRecipe(source, "test-unit");
  if (!testUnit.includes("@bun run typecheck")) throw new Error("test-unit must retain frontend typechecking");
  if (!testUnit.includes("@bun run test")) throw new Error("test-unit must use the package-owned frontend test command");
  const testIntegration = readRecipe(source, "test-int");
  if (!testIntegration.includes("@$(MAKE) --no-print-directory frontend-check")) {
    throw new Error("test-int must retain the frontend drift gate");
  }
  if (!testIntegration.includes("@$(MAKE) --no-print-directory api-check")) {
    throw new Error("test-int must retain the Gul API drift gate");
  }
}

function readRecipe(source, target) {
  const match = source.match(new RegExp(`^${target}:\\n((?:\\t[^\\n]*\\n)+)`, "m"));
  if (!match) throw new Error(`${target} recipe is missing`);
  return match[1].trimEnd().split("\n").map(line => line.slice(1));
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  validateProductMakefile(fs.readFileSync(path.join(sourceRoot, "Makefile"), "utf8"));
}
