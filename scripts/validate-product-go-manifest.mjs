export function validateProductGoManifest(goManifest, versionManifest) {
  const lines = goManifest.split("\n").map(line => line.trim()).filter(Boolean);
  const goParts = versionManifest.GUL_GO_VERSION?.split(".") ?? [];
  if (goParts.length !== 3) throw new Error("GUL_GO_VERSION must contain major.minor.patch");

  requireExactly(lines, "module github.com/rootkernel/gul", "product module identity");
  requireExactly(lines, `go ${goParts[0]}.${goParts[1]}.0`, "product Go language version");
  requireExactly(lines, `toolchain go${versionManifest.GUL_GO_VERSION}`, "product Go toolchain");

  if (lines.some(line => /^(replace|exclude)\b/.test(line))) {
    throw new Error("product Go manifest must not contain replace or exclude directives");
  }
}

function requireExactly(lines, expected, label) {
  if (lines.filter(line => line === expected).length !== 1) {
    throw new Error(`${label} must be exactly ${expected}`);
  }
}
