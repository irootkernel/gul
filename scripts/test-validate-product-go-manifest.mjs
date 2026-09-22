import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {validateProductGoManifest} from "./validate-product-go-manifest.mjs";

const versions = {GUL_GO_VERSION: "1.26.6"};
const valid = `module github.com/rootkernel/gul

go 1.26.0

toolchain go1.26.6
`;

assert.doesNotThrow(() => validateProductGoManifest(valid, versions));

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const currentVersions = Object.fromEntries(
  fs.readFileSync(path.join(sourceRoot, "toolchain", "versions.env"), "utf8")
    .split("\n")
    .filter(line => line.startsWith("GUL_"))
    .map(line => line.split("=", 2)),
);
assert.doesNotThrow(() => validateProductGoManifest(
  fs.readFileSync(path.join(sourceRoot, "go.mod"), "utf8"),
  currentVersions,
));

for (const [name, manifest, pattern] of [
  ["wrong module", valid.replace("github.com/rootkernel/gul", "example.test/gul"), /module identity/],
  ["wrong language version", valid.replace("go 1.26.0", "go 1.27.0"), /language version/],
  ["wrong toolchain patch", valid.replace("go1.26.6", "go1.26.7"), /toolchain/],
  ["replace directive", `${valid}replace example.test/a => ../a\n`, /replace or exclude/],
  ["exclude directive", `${valid}exclude example.test/a v1.0.0\n`, /replace or exclude/],
]) {
  assert.throws(() => validateProductGoManifest(manifest, versions), pattern, name);
}

console.log("product Go manifest fixtures passed");
