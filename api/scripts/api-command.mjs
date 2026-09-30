import {execFileSync} from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";

const apiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(apiRoot, "..");
const contractRoot = path.join(repositoryRoot, "contract");
const checkedRoot = path.join(apiRoot, "generated");
const boundNames = {
  default_page_size: "DefaultPageSize",
  maximum_page_size: "MaximumPageSize",
  maximum_token_bytes: "MaximumTokenBytes",
  maximum_page_metadata_bytes: "MaximumPageMetadataBytes",
  maximum_preview_bytes: "MaximumPreviewBytes",
  maximum_inline_original_bytes: "MaximumInlineOriginalBytes",
  maximum_artifact_chunk_bytes: "MaximumArtifactChunkBytes",
  maximum_close_outcome_bytes: "MaximumCloseOutcomeBytes",
  maximum_file_text_bytes: "MaximumFileTextBytes",
  maximum_file_text_lines: "MaximumFileTextLines",
  maximum_file_image_bytes: "MaximumFileImageBytes",
  maximum_file_image_pixels: "MaximumFileImagePixels",
  maximum_file_markdown_images: "MaximumFileMarkdownImages",
  maximum_file_markdown_image_bytes: "MaximumFileMarkdownImageBytes",
  minimum_password_characters: "MinimumPasswordCharacters",
  maximum_password_bytes: "MaximumPasswordBytes",
};

export function normalizeGeneratedTS(source) {
  return source.replace(/\n+$/, "\n");
}

export function renderBounds(bounds) {
  if (JSON.stringify(Object.keys(bounds).sort()) !== JSON.stringify(Object.keys(boundNames).sort()) ||
      Object.values(bounds).some(value => !Number.isSafeInteger(value) || value < 1)) {
    throw new Error("invalid Gul API bounds authority");
  }
  return {
    go: `// Code generated from api/proto/bounds.json; DO NOT EDIT.\npackage gulv1\n\nconst (\n${Object.entries(boundNames).map(([key, name]) => `\t${name} = ${bounds[key]}`).join("\n")}\n)\n`,
    ts: `// Code generated from api/proto/bounds.json; DO NOT EDIT.\n${Object.entries(boundNames).map(([key, name]) => `export const ${name[0].toLowerCase()+name.slice(1)} = ${bounds[key]};`).join("\n")}\n`,
  };
}

export function compareApiTrees(actualRoot, checkedRoot) {
  const actual = readTree(actualRoot);
  const checked = readTree(checkedRoot);
  const names = tree => tree.map(([name]) => name);
  if (JSON.stringify(names(actual)) !== JSON.stringify(names(checked))) {
    throw new Error(`Gul API generated inventory drifted: ${names(actual).join(", ")} vs ${names(checked).join(", ")}`);
  }
  for (let index = 0; index < actual.length; index += 1) {
    if (!actual[index][1].equals(checked[index][1])) {
      throw new Error(`Gul API generated file drifted: ${actual[index][0]}`);
    }
  }
}

export function replaceGeneratedTree(stagingRoot, checkedRoot, rename = fs.renameSync) {
  const backupRoot = fs.mkdtempSync(path.join(path.dirname(checkedRoot), ".generated-backup-"));
  fs.rmdirSync(backupRoot);
  const hadChecked = fs.existsSync(checkedRoot);
  try {
    if (hadChecked) rename(checkedRoot, backupRoot);
    rename(stagingRoot, checkedRoot);
  } catch (error) {
    if (hadChecked && fs.existsSync(backupRoot)) {
      try {
        rename(backupRoot, checkedRoot);
      } catch (restoreError) {
        throw new AggregateError([error, restoreError],
          `Gul API replacement and restore failed; previous generated tree is at ${backupRoot}`);
      }
    }
    throw error;
  }
  if (hadChecked) fs.rmSync(backupRoot, {recursive: true});
}

function readTree(root) {
  const files = [];
  function walk(directory, relative) {
    for (const entry of fs.readdirSync(directory, {withFileTypes: true})) {
      const name = path.join(relative, entry.name);
      const fullPath = path.join(directory, entry.name);
      if (entry.isDirectory()) walk(fullPath, name);
      else if (entry.isFile()) files.push([name, fs.readFileSync(fullPath)]);
      else throw new Error(`Gul API generated tree contains a non-regular file: ${name}`);
    }
  }
  walk(root, "");
  return files.sort(([left], [right]) => left.localeCompare(right));
}

export function runApiCommand(mode) {
  if (mode !== "generate" && mode !== "check") throw new Error("expected generate or check");
  const stagingRoot = fs.mkdtempSync(path.join(apiRoot, ".generated-stage-"));
  try {
    const goPlugin = execFileSync("go", ["tool", "-n", "protoc-gen-go"], {cwd: contractRoot, encoding: "utf8", env: {...process.env, GOTOOLCHAIN: "local"}}).trim();
    const connectPlugin = execFileSync("go", ["tool", "-n", "protoc-gen-connect-go"], {cwd: contractRoot, encoding: "utf8", env: {...process.env, GOTOOLCHAIN: "local"}}).trim();
    const esPlugin = path.join(contractRoot, "node_modules/.bin/protoc-gen-es");
    fs.mkdirSync(path.join(stagingRoot, "go"));
    fs.mkdirSync(path.join(stagingRoot, "ts"));
    execFileSync("buf", ["lint", "api/proto"], {cwd: repositoryRoot, stdio: "inherit"});
    execFileSync("protoc", [
      "-I", path.join(apiRoot, "proto"),
      `--plugin=protoc-gen-go=${goPlugin}`,
      `--plugin=protoc-gen-connect-go=${connectPlugin}`,
      `--plugin=protoc-gen-es=${esPlugin}`,
      `--go_out=${path.join(stagingRoot, "go")}`, "--go_opt=paths=source_relative",
      `--connect-go_out=${path.join(stagingRoot, "go")}`, "--connect-go_opt=paths=source_relative",
      `--es_out=${path.join(stagingRoot, "ts")}`, "--es_opt=target=ts",
      "gul/v1/gul.proto",
    ], {cwd: repositoryRoot, stdio: "inherit"});
    const generatedTS = path.join(stagingRoot, "ts/gul/v1/gul_pb.ts");
    fs.writeFileSync(generatedTS, normalizeGeneratedTS(fs.readFileSync(generatedTS, "utf8")));
    const bounds = JSON.parse(fs.readFileSync(path.join(apiRoot, "proto/bounds.json"), "utf8"));
    const rendered = renderBounds(bounds);
    fs.writeFileSync(path.join(stagingRoot, "go/gul/v1/bounds.go"), rendered.go);
    fs.writeFileSync(path.join(stagingRoot, "ts/gul/v1/bounds.ts"), rendered.ts);
    if (mode === "check") {
      compareApiTrees(stagingRoot, checkedRoot);
    } else {
      replaceGeneratedTree(stagingRoot, checkedRoot);
    }
  } finally {
    fs.rmSync(stagingRoot, {recursive: true, force: true});
  }
}

if (process.argv[1] && fs.realpathSync(fileURLToPath(import.meta.url)) === fs.realpathSync(path.resolve(process.argv[1]))) {
  runApiCommand(process.argv[2]);
}
