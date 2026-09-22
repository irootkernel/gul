import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";

const expectedScripts = {
  build: "make generate-frontend",
  "check:bundle": "make frontend-check",
  test: "bun test frontend",
  typecheck: "tsc -p frontend/tsconfig.json --noEmit && tsc -p frontend/tsconfig.test.json --noEmit",
};
const expectedDependencies = ["react", "react-dom"];
const expectedDevDependencies = ["@types/bun", "@types/react", "@types/react-dom", "typescript"];

export function validateProductPackage(manifest, versions) {
  if (manifest.name !== "@rootkernel/gul" || manifest.private !== true || manifest.type !== "module") {
    throw new Error("product package identity must remain private @rootkernel/gul ESM");
  }
  if (JSON.stringify(manifest.scripts) !== JSON.stringify(expectedScripts)) {
    throw new Error("product package scripts drifted");
  }
  requireVersion(manifest.dependencies, "react", versions.GUL_REACT_VERSION);
  requireVersion(manifest.dependencies, "react-dom", versions.GUL_REACT_DOM_VERSION);
  requireVersion(manifest.devDependencies, "typescript", versions.GUL_TYPESCRIPT_VERSION);
  requireVersion(manifest.devDependencies, "@types/react", versions.GUL_REACT_TYPES_VERSION);
  requireVersion(manifest.devDependencies, "@types/react-dom", versions.GUL_REACT_DOM_TYPES_VERSION);
  requireVersion(manifest.devDependencies, "@types/bun", versions.GUL_BUN_TYPES_VERSION);
  requireNames(manifest.dependencies, expectedDependencies, "dependencies");
  requireNames(manifest.devDependencies, expectedDevDependencies, "devDependencies");
  for (const field of ["trustedDependencies", "overrides", "resolutions"]) {
    if (field in manifest) throw new Error(`${field} is not allowed in the product package`);
  }

  for (const group of [manifest.dependencies, manifest.devDependencies]) {
    for (const [name, version] of Object.entries(group ?? {})) {
      if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
        throw new Error(`${name} must use an exact version`);
      }
    }
  }
}

function requireVersion(group, name, expected) {
  if (group?.[name] !== expected) throw new Error(`${name} must be exactly ${expected}`);
}

function requireNames(group, expected, label) {
  const actual = Object.keys(group ?? {}).sort();
  if (JSON.stringify(actual) !== JSON.stringify([...expected].sort())) {
    throw new Error(`product package ${label} drifted`);
  }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const versions = Object.fromEntries(
    fs.readFileSync(path.join(sourceRoot, "toolchain/versions.env"), "utf8")
      .split("\n")
      .filter(line => line.startsWith("GUL_"))
      .map(line => line.split("=", 2)),
  );
  const manifest = JSON.parse(fs.readFileSync(path.join(sourceRoot, "package.json"), "utf8"));
  validateProductPackage(manifest, versions);
}
