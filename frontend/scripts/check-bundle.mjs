import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {execFileSync} from "node:child_process";
import {fileURLToPath} from "node:url";
import {compareTrees, readTree} from "./bundle-tree.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(scriptDir, "../..");
const checkedOutput = path.join(repositoryRoot, "internal/delivery/web/dist");
export function checkBundle(temporaryRoot, checkedOutput) {
  compareTrees(readTree(temporaryRoot), readTree(checkedOutput));
}

if (process.argv[1] && fs.realpathSync(fileURLToPath(import.meta.url)) === fs.realpathSync(path.resolve(process.argv[1]))) {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "gul-frontend-"));
  try {
    execFileSync("bun", [path.join(scriptDir, "build.mjs"), temporaryRoot], {
      cwd: repositoryRoot,
      stdio: "inherit",
    });
    checkBundle(temporaryRoot, checkedOutput);
    console.log("frontend bundle is reproducible");
  } finally {
    fs.rmSync(temporaryRoot, {recursive: true, force: true});
  }
}
