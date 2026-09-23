import {expect, test} from "bun:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {compareApiTrees, normalizeGeneratedTS, renderBounds, replaceGeneratedTree} from "./api-command.mjs";

test("generated API drift detects changed bytes, inventory, and symlinks", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "gul-api-check-"));
  const actual = path.join(root, "actual");
  const checked = path.join(root, "checked");
  fs.mkdirSync(actual);
  fs.mkdirSync(checked);
  try {
    fs.writeFileSync(path.join(actual, "gul.pb.go"), "one\n");
    fs.writeFileSync(path.join(checked, "gul.pb.go"), "one\n");
    expect(() => compareApiTrees(actual, checked)).not.toThrow();
    fs.writeFileSync(path.join(checked, "gul.pb.go"), "two\n");
    expect(() => compareApiTrees(actual, checked)).toThrow("gul.pb.go");
    fs.writeFileSync(path.join(checked, "extra.ts"), "extra\n");
    expect(() => compareApiTrees(actual, checked)).toThrow("inventory drifted");
    fs.symlinkSync(path.join(checked, "gul.pb.go"), path.join(actual, "link"));
    expect(() => compareApiTrees(actual, checked)).toThrow("non-regular file: link");
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});

test("generated API replacement swaps checked output and removes its backup", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "gul-api-generate-"));
  const staging = path.join(root, "staging");
  const checked = path.join(root, "generated");
  try {
    fs.mkdirSync(staging);
    fs.mkdirSync(checked);
    fs.writeFileSync(path.join(staging, "client"), "new");
    fs.writeFileSync(path.join(checked, "client"), "old");
    replaceGeneratedTree(staging, checked);
    expect(fs.readFileSync(path.join(checked, "client"), "utf8")).toBe("new");
    expect(fs.existsSync(staging)).toBe(false);
    expect(fs.readdirSync(root)).toEqual(["generated"]);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});

test("generated API replacement restores checked output after a failed swap", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "gul-api-restore-"));
  const staging = path.join(root, "staging");
  const checked = path.join(root, "generated");
  try {
    fs.mkdirSync(staging);
    fs.mkdirSync(checked);
    fs.writeFileSync(path.join(staging, "client"), "new");
    fs.writeFileSync(path.join(checked, "client"), "old");
    let renames = 0;
    const rename = (from, to) => {
      renames += 1;
      if (renames === 2) throw new Error("injected swap failure");
      fs.renameSync(from, to);
    };
    expect(() => replaceGeneratedTree(staging, checked, rename)).toThrow("injected swap failure");
    expect(renames).toBe(3);
    expect(fs.readFileSync(path.join(checked, "client"), "utf8")).toBe("old");
    expect(fs.readFileSync(path.join(staging, "client"), "utf8")).toBe("new");
    expect(fs.readdirSync(root).sort()).toEqual(["generated", "staging"]);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});

test("generated API replacement handles the first generated tree", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "gul-api-first-"));
  const staging = path.join(root, "staging");
  const checked = path.join(root, "generated");
  try {
    fs.mkdirSync(staging);
    fs.writeFileSync(path.join(staging, "client"), "first");
    replaceGeneratedTree(staging, checked);
    expect(fs.readFileSync(path.join(checked, "client"), "utf8")).toBe("first");
    expect(fs.readdirSync(root)).toEqual(["generated"]);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});

test("generated API replacement preserves both failures and the backup when restore fails", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "gul-api-double-failure-"));
  const staging = path.join(root, "staging");
  const checked = path.join(root, "generated");
  try {
    fs.mkdirSync(staging);
    fs.mkdirSync(checked);
    fs.writeFileSync(path.join(checked, "client"), "old");
    let renames = 0;
    let failure;
    try {
      replaceGeneratedTree(staging, checked, (from, to) => {
        renames += 1;
        if (renames > 1) throw new Error(`rename failure ${renames}`);
        fs.renameSync(from, to);
      });
    } catch (error) {
      failure = error;
    }
    expect(failure).toBeInstanceOf(AggregateError);
    expect(failure.errors.map(error => error.message)).toEqual(["rename failure 2", "rename failure 3"]);
    const backup = fs.readdirSync(root).find(name => name.startsWith(".generated-backup-"));
    expect(failure.message).toContain(path.join(root, backup));
    expect(fs.readFileSync(path.join(root, backup, "client"), "utf8")).toBe("old");
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});

test("Gul bounds authority renders both clients and rejects malformed bounds", () => {
  const bounds = JSON.parse(fs.readFileSync(path.join(import.meta.dir, "../proto/bounds.json"), "utf8"));
  const rendered = renderBounds(bounds);
  expect(rendered.go).toBe(fs.readFileSync(path.join(import.meta.dir, "../generated/go/gul/v1/bounds.go"), "utf8"));
  expect(rendered.ts).toBe(fs.readFileSync(path.join(import.meta.dir, "../generated/ts/gul/v1/bounds.ts"), "utf8"));
  for (const malformed of [
    {...bounds, extra: 1},
    Object.fromEntries(Object.entries(bounds).filter(([key]) => key !== "maximum_page_size")),
    {...bounds, maximum_page_size: 0},
    {...bounds, maximum_page_size: 1.5},
  ]) {
    expect(() => renderBounds(malformed)).toThrow("invalid Gul API bounds authority");
  }
  expect(normalizeGeneratedTS("export {};\n\n")).toBe("export {};\n");
});
