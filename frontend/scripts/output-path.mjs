import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export function validateOutputPath(requestedOutput, repositoryRoot, defaultOutput) {
  const resolvedOutput = path.resolve(requestedOutput);
  const resolvedRoot = fs.realpathSync(repositoryRoot);
  const temporaryRoot = fs.realpathSync(os.tmpdir());

  if (resolvedOutput === path.resolve(repositoryRoot) || resolvedOutput === path.parse(resolvedOutput).root) {
    throw new Error("refusing to replace a broad output directory");
  }
  if (fs.lstatSync(resolvedOutput, {throwIfNoEntry: false})?.isSymbolicLink()) {
    throw new Error("frontend output must not be a symbolic link");
  }

  const canonicalParent = fs.realpathSync(path.dirname(resolvedOutput));
  const canonicalOutput = path.join(canonicalParent, path.basename(resolvedOutput));
  const canonicalDefault = path.join(fs.realpathSync(path.dirname(defaultOutput)), path.basename(defaultOutput));
  if (canonicalOutput === resolvedRoot || isWithin(resolvedRoot, canonicalOutput)) {
    if (canonicalOutput !== canonicalDefault || canonicalDefault !== path.join(resolvedRoot, "internal/delivery/web/dist")) {
      throw new Error("frontend output must not replace the repository or its contents");
    }
  } else if (!isWithin(temporaryRoot, canonicalOutput)) {
    throw new Error("frontend output must be the checked bundle directory or a temporary directory");
  }

  return canonicalOutput;
}

function isWithin(parent, child) {
  const relative = path.relative(parent, child);
  return relative !== "" && !relative.startsWith(`..${path.sep}`) && relative !== ".." && !path.isAbsolute(relative);
}
