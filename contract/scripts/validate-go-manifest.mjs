const directModules = [
  ["connectrpc.com/connect", "GUL_CONNECT_GO_VERSION"],
  ["google.golang.org/protobuf", "GUL_PROTOBUF_GO_VERSION"],
];

const toolModules = [
  "connectrpc.com/connect/cmd/protoc-gen-connect-go",
  "google.golang.org/protobuf/cmd/protoc-gen-go",
];

export function validateGoManifest(goManifest, versionManifest) {
  const lines = goManifest.split("\n").map((line) => line.trim());
  const goParts = versionManifest.GUL_GO_VERSION?.split(".") ?? [];
  if (goParts.length !== 3) throw new Error("GUL_GO_VERSION must contain major.minor.patch");
  if (lines.filter((line) => line === `go ${goParts[0]}.${goParts[1]}.0`).length !== 1) {
    throw new Error("contract Go language version must match the pinned toolchain series");
  }
  if (lines.filter((line) => line === `toolchain go${versionManifest.GUL_GO_VERSION}`).length !== 1) {
    throw new Error("contract Go toolchain must match GUL_GO_VERSION exactly");
  }
  if (lines.some((line) => /^(replace|exclude)\b/.test(line))) {
    throw new Error("contract Go manifest must not contain replace or exclude directives");
  }
  for (const [module, versionKey] of directModules) {
    const expected = `${module} v${versionManifest[versionKey]}`;
    if (lines.filter((line) => line === expected).length !== 1) {
      throw new Error(`${module} must be one exact direct requirement at ${versionManifest[versionKey]}`);
    }
  }
  for (const module of toolModules) {
    if (lines.filter((line) => line === module).length !== 1) {
      throw new Error(`${module} must be one exact Go tool requirement`);
    }
  }
}
