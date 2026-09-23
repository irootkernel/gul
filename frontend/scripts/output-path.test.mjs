import {expect, test} from "bun:test";
import {randomUUID} from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {validateOutputPath} from "./output-path.mjs";
import {buildFrontend, replaceOutput} from "./build.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(scriptDir, "../..");
const defaultOutput = path.join(repositoryRoot, "internal/delivery/web/dist");

test("allows only the checked bundle or a temporary output", () => {
  const temporaryOutput = path.join(os.tmpdir(), "gul-output-path-test");
  expect(validateOutputPath(defaultOutput, repositoryRoot, defaultOutput)).toBe(defaultOutput);
  expect(validateOutputPath(temporaryOutput, repositoryRoot, defaultOutput)).toBe(
    path.join(fs.realpathSync(os.tmpdir()), "gul-output-path-test"),
  );
  expect(() => validateOutputPath(repositoryRoot, repositoryRoot, defaultOutput)).toThrow("broad output");
  expect(() => validateOutputPath(path.parse(repositoryRoot).root, repositoryRoot, defaultOutput)).toThrow();
  expect(() => validateOutputPath(path.join(repositoryRoot, "frontend"), repositoryRoot, defaultOutput)).toThrow("repository");
});

test("rejects repository content when the checkout is below the temp root", () => {
  const temporaryRepository = fs.mkdtempSync(path.join(os.tmpdir(), "gul-output-repository-"));
  const temporaryDefault = path.join(temporaryRepository, "internal/delivery/web/dist");
  fs.mkdirSync(path.dirname(temporaryDefault), {recursive: true});
  try {
    expect(() => validateOutputPath(path.join(temporaryRepository, "frontend"), temporaryRepository, temporaryDefault)).toThrow("repository");
  } finally {
    fs.rmSync(temporaryRepository, {recursive: true, force: true});
  }
});

test("rejects a temporary path whose parent escapes through a symbolic link", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-output-symlink-"));
  const outside = fs.mkdtempSync(path.join(repositoryRoot, ".gul-output-outside-"));
  const link = path.join(fixture, "link");
  fs.symlinkSync(outside, link);
  try {
    expect(() => validateOutputPath(path.join(link, "bundle"), repositoryRoot, defaultOutput)).toThrow("repository");
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
    fs.rmSync(outside, {recursive: true, force: true});
  }
});

test("rejects a symbolic link used as the output itself", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-output-link-"));
  const target = path.join(fixture, "target");
  const output = path.join(fixture, "output");
  fs.mkdirSync(target);
  fs.writeFileSync(path.join(target, "sentinel.txt"), "keep\n");
  fs.symlinkSync(target, output);
  try {
    expect(() => validateOutputPath(output, repositoryRoot, defaultOutput)).toThrow("symbolic link");
    expect(fs.readFileSync(path.join(target, "sentinel.txt"), "utf8")).toBe("keep\n");
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});

test("rejects a dangling symbolic link used as the output itself", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-output-dangling-"));
  const target = path.join(fixture, "missing");
  const output = path.join(fixture, "output");
  fs.symlinkSync(target, output);
  try {
    expect(() => validateOutputPath(output, repositoryRoot, defaultOutput)).toThrow("symbolic link");
    expect(fs.readlinkSync(output)).toBe(target);
    expect(fs.existsSync(target)).toBe(false);
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});

test("rejects a path outside the repository and temporary root", () => {
  const outside = path.join(path.parse(repositoryRoot).root, `gul-output-outside-${randomUUID()}`);
  expect(() => validateOutputPath(outside, repositoryRoot, defaultOutput)).toThrow("temporary directory");
});

test("accepts an alternate spelling of the checked output", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-output-alias-"));
  const repository = path.join(fixture, "repository");
  const alias = path.join(fixture, "alias");
  const checked = path.join(repository, "internal/delivery/web/dist");
  fs.mkdirSync(path.dirname(checked), {recursive: true});
  fs.symlinkSync(repository, alias);
  try {
    expect(validateOutputPath(path.join(alias, "internal/delivery/web/dist"), repository, checked)).toBe(
      path.join(fs.realpathSync(repository), "internal/delivery/web/dist"),
    );
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});

test("a refused build leaves its target untouched", async () => {
  const refusedOutput = path.join(repositoryRoot, "frontend", ".output-path-fixture");
  const sentinel = path.join(refusedOutput, "sentinel.txt");
  fs.mkdirSync(refusedOutput, {recursive: true});
  fs.writeFileSync(sentinel, "keep\n");
  try {
    const process = Bun.spawn(["bun", path.join(scriptDir, "build.mjs"), refusedOutput], {
      cwd: repositoryRoot,
      stderr: "pipe",
      stdout: "pipe",
    });
    expect(await process.exited).not.toBe(0);
    expect(fs.readFileSync(sentinel, "utf8")).toBe("keep\n");
  } finally {
    fs.rmSync(refusedOutput, {recursive: true, force: true});
  }
});

test("the build CLI runs from a symlink-spelled checkout", async () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-build-alias-"));
  const alias = path.join(fixture, "checkout");
  const output = path.join(fixture, "output");
  fs.symlinkSync(repositoryRoot, alias);
  try {
    const process = Bun.spawn(["bun", path.join(alias, "frontend/scripts/build.mjs"), output], {
      cwd: alias,
      stderr: "pipe",
      stdout: "pipe",
    });
    expect(await process.exited).toBe(0);
    expect(fs.existsSync(path.join(output, "bundle-manifest.json"))).toBe(true);
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});

test("a failed compilation preserves an allowed previous bundle", async () => {
  const output = fs.mkdtempSync(path.join(os.tmpdir(), "gul-build-failure-"));
  const sentinel = path.join(output, "sentinel.txt");
  fs.writeFileSync(sentinel, "keep\n");
  try {
    await expect(buildFrontend({output, entrypoint: path.join(output, "missing.tsx")})).rejects.toThrow("frontend build failed");
    expect(fs.readFileSync(sentinel, "utf8")).toBe("keep\n");
  } finally {
    fs.rmSync(output, {recursive: true, force: true});
  }
});

test("a successful build replaces an allowed tree without a backup", async () => {
  const output = fs.mkdtempSync(path.join(os.tmpdir(), "gul-build-success-"));
  fs.writeFileSync(path.join(output, "obsolete.txt"), "remove\n");
  try {
    await buildFrontend({output});
    expect(fs.existsSync(path.join(output, "obsolete.txt"))).toBe(false);
    expect(fs.existsSync(path.join(output, "bundle-manifest.json"))).toBe(true);
    const siblings = fs.readdirSync(path.dirname(output));
    expect(siblings.some(name => name.startsWith(`${path.basename(output)}.previous-`))).toBe(false);
  } finally {
    fs.rmSync(output, {recursive: true, force: true});
  }
});

test("a failed swap restores the previous output", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-build-rollback-"));
  const output = path.join(fixture, "output");
  const staging = path.join(fixture, "staging");
  fs.mkdirSync(output);
  fs.mkdirSync(staging);
  fs.writeFileSync(path.join(output, "previous.txt"), "keep\n");
  fs.writeFileSync(path.join(staging, "replacement.txt"), "new\n");
  let calls = 0;
  const rename = (source, destination) => {
    calls += 1;
    if (calls === 2) throw new Error("swap failed");
    fs.renameSync(source, destination);
  };
  try {
    expect(() => replaceOutput(staging, output, rename)).toThrow("swap failed");
    expect(calls).toBe(3);
    expect(fs.readFileSync(path.join(output, "previous.txt"), "utf8")).toBe("keep\n");
    expect(fs.existsSync(staging)).toBe(true);
    expect(fs.readdirSync(fixture).sort()).toEqual(["output", "staging"]);
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});

test("a failed swap and restore reports the preserved backup", () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "gul-build-double-failure-"));
  const output = path.join(fixture, "output");
  const staging = path.join(fixture, "staging");
  fs.mkdirSync(output);
  fs.mkdirSync(staging);
  fs.writeFileSync(path.join(output, "previous.txt"), "keep\n");
  let calls = 0;
  const rename = (source, destination) => {
    calls += 1;
    if (calls > 1) throw new Error(`rename ${calls} failed`);
    fs.renameSync(source, destination);
  };
  try {
    let failure;
    try {
      replaceOutput(staging, output, rename);
    } catch (error) {
      failure = error;
    }
    expect(failure).toBeInstanceOf(AggregateError);
    expect(failure.errors.map(error => error.message)).toEqual(["rename 2 failed", "rename 3 failed"]);
    expect(calls).toBe(3);
    const backup = fs.readdirSync(fixture).find(name => name.startsWith("output.previous-"));
    expect(backup).toBeDefined();
    expect(failure.message).toContain(path.join(fixture, backup));
    expect(fs.readFileSync(path.join(fixture, backup, "previous.txt"), "utf8")).toBe("keep\n");
    expect(fs.existsSync(staging)).toBe(true);
  } finally {
    fs.rmSync(fixture, {recursive: true, force: true});
  }
});
