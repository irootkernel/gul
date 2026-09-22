import fs from "node:fs";
import path from "node:path";

export function readTree(root) {
  const files = [];
  walk(root, "", files);
  return files.sort(([left], [right]) => left.localeCompare(right));
}

function walk(root, relative, files) {
  const entries = fs.readdirSync(path.join(root, relative), {withFileTypes: true});
  entries.sort((left, right) => left.name.localeCompare(right.name));
  for (const entry of entries) {
    const child = path.join(relative, entry.name);
    if (entry.isDirectory()) {
      walk(root, child, files);
    } else if (entry.isFile()) {
      files.push([child, fs.readFileSync(path.join(root, child))]);
    } else {
      throw new Error(`frontend bundle contains a non-regular file: ${child}`);
    }
  }
}

export function compareTrees(actual, expected) {
  const actualPaths = actual.map(([name]) => name);
  const expectedPaths = expected.map(([name]) => name);
  if (JSON.stringify(actualPaths) !== JSON.stringify(expectedPaths)) {
    throw new Error(`checked frontend bundle drifted: file inventory differs (${actualPaths.join(", ")} vs ${expectedPaths.join(", ")})`);
  }
  for (let index = 0; index < actual.length; index += 1) {
    const [name, actualBytes] = actual[index];
    const expectedBytes = expected[index][1];
    if (!actualBytes.equals(expectedBytes)) {
      throw new Error(`checked frontend bundle drifted: ${name}`);
    }
  }
}
