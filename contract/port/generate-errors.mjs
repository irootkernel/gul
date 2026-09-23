import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {execFileSync} from "node:child_process";

const directory = path.dirname(fileURLToPath(import.meta.url));
const policy = JSON.parse(fs.readFileSync(path.join(directory, "../upstream/dolgorae-grpc-error-mapping-v1.json"), "utf8"));
const codes = new Set();
for (const section of ["required_action_overrides", "status_overrides", "retry_classification_overrides", "recovery_classification_overrides"]) {
  for (const values of Object.values(policy[section])) for (const code of values) codes.add(code);
}
for (const overrides of Object.values(policy.method_overrides)) for (const code of Object.keys(overrides)) codes.add(code);
for (const overrides of Object.values(policy.method_class_overrides)) for (const code of Object.keys(overrides)) codes.add(code);
const output = execFileSync("gofmt", [], {input: `// Code generated from dolgorae-grpc-error-mapping-v1.json; DO NOT EDIT.\npackage port\n\nvar acceptedErrorCodes = map[string]struct{}{\n${[...codes].sort().map(code => `\t${JSON.stringify(code)}: {},`).join("\n")}\n}\n`, encoding: "utf8"});
const target = path.join(directory, "error_catalog_gen.go");
if (process.argv[2] === "check") {
  if (fs.readFileSync(target, "utf8") !== output) throw new Error("accepted error catalog drifted");
} else if (process.argv[2] === "generate") {
  fs.writeFileSync(target, output);
} else {
  throw new Error("expected generate or check");
}
