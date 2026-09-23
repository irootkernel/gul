export function validateProductGoManifest(goManifest, versionManifest) {
  const lines = goManifest.split("\n").map(line => line.trim()).filter(Boolean);
  const goParts = versionManifest.GUL_GO_VERSION?.split(".") ?? [];
  if (goParts.length !== 3) throw new Error("GUL_GO_VERSION must contain major.minor.patch");

  requireExactly(lines, "module github.com/rootkernel/gul", "product module identity");
  requireExactly(lines, `go ${goParts[0]}.${goParts[1]}.0`, "product Go language version");
  requireExactly(lines, `toolchain go${versionManifest.GUL_GO_VERSION}`, "product Go toolchain");
  requireExactly(lines, `connectrpc.com/connect v${versionManifest.GUL_CONNECT_GO_VERSION}`, "product Connect Go pin");
  requireExactly(lines, "github.com/rootkernel/gul/contract v0.0.0", "checked local contract module");
  requireExactly(lines, `google.golang.org/protobuf v${versionManifest.GUL_PROTOBUF_GO_VERSION}`, "product Protobuf Go pin");
  requireExactly(lines, `modernc.org/sqlite v${versionManifest.GUL_MODERNC_SQLITE_VERSION}`, "product SQLite pin");
  requireExactly(lines, "replace github.com/rootkernel/gul/contract => ./contract", "checked local contract replacement");

  if (lines.some(line => /^exclude\b/.test(line) || (/^replace\b/.test(line) && line !== "replace github.com/rootkernel/gul/contract => ./contract"))) {
    throw new Error("product Go manifest contains an unapproved replace or exclude directive");
  }
}

function requireExactly(lines, expected, label) {
  if (lines.filter(line => line === expected).length !== 1) {
    throw new Error(`${label} must be exactly ${expected}`);
  }
}
