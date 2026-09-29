import type {GetMetadataResponse, PromptOriginal, ReadChunkResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {maximumArtifactChunkBytes, maximumInlineOriginalBytes} from "../../api/generated/ts/gul/v1/bounds";

export type PromptArtifactClient = {
  getMetadata(request: {sessionId: string; artifactRef: string}): Promise<GetMetadataResponse>;
  readChunk(request: {sessionId: string; artifactRef: string; offset: bigint; length: number}): Promise<ReadChunkResponse>;
};

const maximumSize = 64 * 1024 * 1024;
const textMediaTypes = new Set(["text/plain", "text/plain; charset=utf-8", "text/markdown", "text/markdown; charset=utf-8", "text/x-diff"]);

// Artifact references are opaque Gul IDs. The browser reads only through the
// authenticated presentation client and checks the complete body before use.
export async function readPromptOriginal(client: PromptArtifactClient, sessionId: string, artifactRef: string) {
  const metadata = await client.getMetadata({sessionId, artifactRef});
  if (metadata.artifactRef !== artifactRef || !textMediaTypes.has(metadata.mediaType) ||
      metadata.byteLength < 0n || metadata.byteLength > BigInt(maximumSize) || !/^[0-9a-f]{64}$/.test(metadata.sha256)) {
    throw new Error("Original prompt metadata is unavailable.");
  }
  const bytes = new Uint8Array(Number(metadata.byteLength));
  for (let offset = 0; offset < bytes.length;) {
    const length = Math.min(maximumArtifactChunkBytes, bytes.length - offset);
    const part = await client.readChunk({sessionId, artifactRef, offset: BigInt(offset), length});
    if (part.totalLength !== metadata.byteLength || part.sha256 !== metadata.sha256 || part.data.length !== length) {
      throw new Error("Original prompt failed integrity checks.");
    }
    bytes.set(part.data, offset);
    offset += length;
  }
  const digest = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))).map(byte => byte.toString(16).padStart(2, "0")).join("");
  if (digest !== metadata.sha256) throw new Error("Original prompt failed integrity checks.");
  return new TextDecoder("utf-8", {fatal: true}).decode(bytes);
}

export async function resolvePromptOriginal(client: PromptArtifactClient, sessionId: string, content: PromptOriginal["content"] | undefined) {
  if (content?.case === "inlineUtf8" && new TextEncoder().encode(content.value).length <= maximumInlineOriginalBytes) return content.value;
  if (content?.case === "artifactRef") return readPromptOriginal(client, sessionId, content.value);
  throw new Error("Original prompt unavailable");
}
