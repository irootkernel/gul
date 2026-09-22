import {expect, test} from "bun:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {compareTrees, readTree} from "./bundle-tree.mjs";
import {checkBundle} from "./check-bundle.mjs";

test("bundle comparison rejects changed bytes and file inventories", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-bundle-tree-"));
  const actual = path.join(fixture, "actual");
  const expected = path.join(fixture, "expected");
  fs.mkdirSync(actual);
  fs.mkdirSync(expected);
  try {
    fs.writeFileSync(path.join(actual, "index.html"), "first\n");
    fs.writeFileSync(path.join(expected, "index.html"), "first\n");
    expect(() => compareTrees(readTree(actual), readTree(expected))).not.toThrow();
    fs.writeFileSync(path.join(expected, "index.html"), "second\n");
    expect(() => compareTrees(readTree(actual), readTree(expected))).toThrow("index.html");
    fs.writeFileSync(path.join(expected, "extra.css"), "extra\n");
    expect(() => compareTrees(readTree(actual), readTree(expected))).toThrow("file inventory differs");
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});

test("bundle inventory refuses symbolic links before reading them", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-bundle-link-"));
  fs.writeFileSync(path.join(fixture, "target.txt"), "private\n");
  fs.symlinkSync(path.join(fixture, "target.txt"), path.join(fixture, "link.txt"));
  try {
    expect(() => readTree(fixture)).toThrow("non-regular file: link.txt");
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});

test("bundle check reads the checked tree and rejects drift", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-bundle-check-"));
  const actual = path.join(fixture, "actual");
  const checked = path.join(fixture, "checked");
  fs.mkdirSync(actual);
  fs.mkdirSync(checked);
  try {
    fs.writeFileSync(path.join(actual, "index.html"), "first\n");
    fs.writeFileSync(path.join(checked, "index.html"), "first\n");
    expect(() => checkBundle(actual, checked)).not.toThrow();
    fs.writeFileSync(path.join(checked, "index.html"), "second\n");
    expect(() => checkBundle(actual, checked)).toThrow("index.html");
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});
