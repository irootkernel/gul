import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {validateProductPackage} from "./validate-product-package.mjs";

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const versions = Object.fromEntries(
  fs.readFileSync(path.join(sourceRoot, "toolchain/versions.env"), "utf8")
    .split("\n")
    .filter(line => line.startsWith("GUL_"))
    .map(line => line.split("=", 2)),
);
const current = JSON.parse(fs.readFileSync(path.join(sourceRoot, "package.json"), "utf8"));

assert.doesNotThrow(() => validateProductPackage(current, versions));

for (const [name, mutate, pattern] of [
  ["package name", manifest => { manifest.name = "gul"; }, /package identity/],
  ["public package", manifest => { manifest.private = false; }, /package identity/],
  ["CommonJS package", manifest => { manifest.type = "commonjs"; }, /package identity/],
  ["React range", manifest => { manifest.dependencies.react = "^19.2.7"; }, /react must be exactly/],
  ["React DOM drift", manifest => { manifest.dependencies["react-dom"] = "19.2.6"; }, /react-dom must be exactly/],
  ["protobuf drift", manifest => { manifest.dependencies["@bufbuild/protobuf"] = "2.13.0"; }, /@bufbuild\/protobuf must be exactly/],
  ["Connect drift", manifest => { manifest.dependencies["@connectrpc/connect"] = "2.0.0"; }, /@connectrpc\/connect must be exactly/],
  ["Connect Web drift", manifest => { manifest.dependencies["@connectrpc/connect-web"] = "2.0.0"; }, /@connectrpc\/connect-web must be exactly/],
  ["TypeScript drift", manifest => { manifest.devDependencies.typescript = "7.0.1"; }, /typescript must be exactly/],
  ["lifecycle script", manifest => { manifest.scripts.postinstall = "echo unsafe"; }, /scripts drifted/],
  ["React types drift", manifest => { manifest.devDependencies["@types/react"] = "19.2.0"; }, /@types\/react must be exactly/],
  ["React DOM types drift", manifest => { manifest.devDependencies["@types/react-dom"] = "19.2.0"; }, /@types\/react-dom must be exactly/],
  ["Bun types drift", manifest => { manifest.devDependencies["@types/bun"] = "1.4.1"; }, /@types\/bun must be exactly/],
  ["extra dependency", manifest => { manifest.dependencies.zod = "4.1.5"; }, /dependencies drifted/],
  ["trusted dependency", manifest => { manifest.trustedDependencies = ["react"]; }, /trustedDependencies is not allowed/],
]) {
  const fixture = structuredClone(current);
  mutate(fixture);
  assert.throws(() => validateProductPackage(fixture, versions), pattern, name);
}

console.log("product package manifest fixtures passed");
