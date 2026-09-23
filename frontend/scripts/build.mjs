import {createHash, randomUUID} from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {validateOutputPath} from "./output-path.mjs";
import {readTree} from "./bundle-tree.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(scriptDir, "../..");
const defaultOutput = path.join(repositoryRoot, "internal/delivery/web/dist");

export async function buildFrontend({
  output = defaultOutput,
  entrypoint = path.join(repositoryRoot, "frontend/src/gul.tsx"),
} = {}) {
  const requestedOutput = validateOutputPath(output, repositoryRoot, defaultOutput);
  const outputParent = path.dirname(requestedOutput);
  const stagingOutput = fs.mkdtempSync(path.join(outputParent, `.${path.basename(requestedOutput)}.build-`));

  try {
    let result;
    try {
      result = await Bun.build({
        define: {"process.env.NODE_ENV": '"production"'},
        entrypoints: [entrypoint],
        minify: true,
        naming: "assets/[name].[ext]",
        outdir: stagingOutput,
        sourcemap: "none",
        splitting: false,
        target: "browser",
      });
    } catch (error) {
      throw new Error("frontend build failed", {cause: error});
    }
    if (!result.success) {
      for (const log of result.logs) console.error(log);
      throw new Error("frontend build failed");
    }

    fs.copyFileSync(path.join(repositoryRoot, "frontend/index.html"), path.join(stagingOutput, "index.html"));

    const files = [];
    for (const [relativePath, bytes] of readTree(stagingOutput)) {
      files.push({
        path: relativePath,
        sha256: createHash("sha256").update(bytes).digest("hex"),
        size: bytes.length,
      });
    }
    const manifest = {
      schema: "gul.frontend-bundle/v1",
      entrypoint: "index.html",
      files,
    };
    fs.writeFileSync(
      path.join(stagingOutput, "bundle-manifest.json"),
      `${JSON.stringify(manifest, null, 2)}\n`,
    );

    replaceOutput(stagingOutput, requestedOutput);
  } finally {
    fs.rmSync(stagingOutput, {recursive: true, force: true});
  }
}

export function replaceOutput(staging, destination, rename = fs.renameSync) {
  const backup = `${destination}.previous-${process.pid}-${randomUUID()}`;
  const hadDestination = fs.existsSync(destination);
  if (hadDestination) rename(destination, backup);
  try {
    rename(staging, destination);
  } catch (error) {
    if (hadDestination) {
      try {
        rename(backup, destination);
      } catch (restoreError) {
        throw new AggregateError([error, restoreError],
          `Frontend replacement and restore failed; previous bundle is at ${backup}`);
      }
    }
    throw error;
  }
  if (hadDestination) fs.rmSync(backup, {recursive: true, force: true});
}

if (process.argv[1] && fs.realpathSync(fileURLToPath(import.meta.url)) === fs.realpathSync(path.resolve(process.argv[1]))) {
  await buildFrontend({output: process.argv[2] ?? defaultOutput});
}
