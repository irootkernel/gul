import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";

const expectedBrowser = {
  types: ["react", "react-dom"],
  include: ["src/app.tsx", "src/gul.tsx", "src/launch-selection.tsx", "src/**/*.d.ts"],
};
const expectedTest = {
  types: ["bun", "react", "react-dom"],
  include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
};

export function validateFrontendConfig(browser, test) {
  requireArray(browser.compilerOptions?.types, expectedBrowser.types, "browser types");
  requireArray(browser.include, expectedBrowser.include, "browser includes");
  requireArray(test.compilerOptions?.types, expectedTest.types, "test types");
  requireArray(test.include, expectedTest.include, "test includes");
}

function requireArray(actual, expected, label) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) throw new Error(`frontend ${label} drifted`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const read = name => JSON.parse(fs.readFileSync(path.join(sourceRoot, "frontend", name), "utf8"));
  validateFrontendConfig(read("tsconfig.json"), read("tsconfig.test.json"));
}
