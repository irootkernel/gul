import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {validateProductGoManifest} from "./validate-product-go-manifest.mjs";

const versions = {
  GUL_GO_VERSION: "1.26.6",
  GUL_CONNECT_GO_VERSION: "1.20.0",
  GUL_PROTOBUF_GO_VERSION: "1.36.12",
  GUL_MODERNC_SQLITE_VERSION: "1.57.0",
};
const valid = `module github.com/rootkernel/gul

go 1.26.0

toolchain go1.26.6

require (
  connectrpc.com/connect v1.20.0
  github.com/rootkernel/gul/contract v0.0.0
  google.golang.org/protobuf v1.36.12
  modernc.org/sqlite v1.57.0
)
replace github.com/rootkernel/gul/contract => ./contract
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
  ["wrong Connect pin", valid.replace("connect v1.20.0", "connect v1.21.0"), /Connect Go pin/],
  ["missing local contract", valid.replace("github.com/rootkernel/gul/contract v0.0.0\n", ""), /checked local contract module/],
  ["wrong local contract path", valid.replace("=> ./contract", "=> ../contract"), /checked local contract replacement/],
  ["wrong Protobuf pin", valid.replace("protobuf v1.36.12", "protobuf v1.36.11"), /Protobuf Go pin/],
  ["wrong SQLite pin", valid.replace("sqlite v1.57.0", "sqlite v1.57.1"), /SQLite pin/],
  ["replace directive", `${valid}replace example.test/a => ../a\n`, /unapproved replace or exclude/],
  ["exclude directive", `${valid}exclude example.test/a v1.0.0\n`, /unapproved replace or exclude/],
]) {
  assert.throws(() => validateProductGoManifest(manifest, versions), pattern, name);
}

console.log("product Go manifest fixtures passed");
