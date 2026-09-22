import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import { validateGoManifest } from "./validate-go-manifest.mjs";

const versions = {
  GUL_GO_VERSION: "1.26.6",
  GUL_CONNECT_GO_VERSION: "1.20.0",
  GUL_PROTOBUF_GO_VERSION: "1.36.12",
};
const valid = `module example.test/contract

go 1.26.0

toolchain go1.26.6

require (
  connectrpc.com/connect v1.20.0
  google.golang.org/protobuf v1.36.12
)

tool (
  connectrpc.com/connect/cmd/protoc-gen-connect-go
  google.golang.org/protobuf/cmd/protoc-gen-go
)
`;

assert.doesNotThrow(() => validateGoManifest(valid, versions));

const contractRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const currentVersions = Object.fromEntries(
  fs.readFileSync(path.join(contractRoot, "..", "toolchain", "versions.env"), "utf8")
    .split("\n")
    .filter(line => line.startsWith("GUL_"))
    .map(line => line.split("=", 2)),
);
assert.doesNotThrow(() => validateGoManifest(
  fs.readFileSync(path.join(contractRoot, "go.mod"), "utf8"),
  currentVersions,
));

for (const [name, manifest, pattern] of [
  ["indirect dependency", valid.replace("connectrpc.com/connect v1.20.0", "connectrpc.com/connect v1.20.0 // indirect"), /exact direct requirement/],
  ["version prefix", valid.replace("connectrpc.com/connect v1.20.0", "connectrpc.com/connect v1.20.0-rc.1"), /exact direct requirement/],
  ["wrong toolchain patch", valid.replace("go1.26.6", "go1.26.7"), /toolchain/],
  ["comment decoy", valid.replace("connectrpc.com/connect v1.20.0", "// connectrpc.com/connect v1.20.0\n  connectrpc.com/connect v1.19.0"), /exact direct requirement/],
  ["replace directive", `${valid}\nreplace connectrpc.com/connect => ../connect\n`, /replace or exclude/],
  ["exclude directive", `${valid}\nexclude connectrpc.com/connect v1.19.0\n`, /replace or exclude/],
  ["missing tool", valid.replace("  connectrpc.com/connect/cmd/protoc-gen-connect-go\n", ""), /exact Go tool requirement/],
]) {
  assert.throws(() => validateGoManifest(manifest, versions), pattern, name);
}

console.log("Go manifest validator fixtures passed");
