import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {GetMetadataResponseSchema, ReadChunkResponseSchema} from "../../api/generated/ts/gul/v1/gul_pb";
import {readPromptOriginal} from "./prompt-original";

test("full prompt artifact is decoded only after complete digest verification", async () => {
  const bytes = new TextEncoder().encode("첫째 줄\r\n둘째 줄");
  const digest = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))).map(byte => byte.toString(16).padStart(2, "0")).join("");
  const calls: [bigint, number][] = [];
  const client = {
    getMetadata: async () => create(GetMetadataResponseSchema, {artifactRef: "artifact-1", mediaType: "text/plain; charset=utf-8", byteLength: BigInt(bytes.length), sha256: digest}),
    readChunk: async ({offset, length}: {offset: bigint; length: number}) => {
      calls.push([offset, length]);
      return create(ReadChunkResponseSchema, {data: bytes, totalLength: BigInt(bytes.length), sha256: digest});
    },
  };
  expect(await readPromptOriginal(client, "session-1", "artifact-1")).toBe("첫째 줄\r\n둘째 줄");
  expect(calls).toEqual([[0n, bytes.length]]);
  await expect(readPromptOriginal({...client, readChunk: async () => create(ReadChunkResponseSchema,
    {data: bytes, totalLength: BigInt(bytes.length), sha256: "0".repeat(64)})}, "session-1", "artifact-1")).rejects.toThrow("integrity");
  await expect(readPromptOriginal({...client, getMetadata: async () => create(GetMetadataResponseSchema,
    {artifactRef: "artifact-1", mediaType: "text/plain", byteLength: 64n * 1024n * 1024n + 1n, sha256: digest})},
  "session-1", "artifact-1")).rejects.toThrow("metadata");
});

test("multi-chunk original uses exact offsets and refuses a damaged continuation", async () => {
  const bytes = new TextEncoder().encode("한".repeat(90_000));
  const digest = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))).map(byte => byte.toString(16).padStart(2, "0")).join("");
  const calls: [bigint, number][] = [];
  const metadata = create(GetMetadataResponseSchema, {artifactRef: "artifact-2", mediaType: "text/plain", byteLength: BigInt(bytes.length), sha256: digest});
  const client = {
    getMetadata: async () => metadata,
    readChunk: async ({offset, length}: {offset: bigint; length: number}) => {
      calls.push([offset, length]);
      return create(ReadChunkResponseSchema, {data: bytes.slice(Number(offset), Number(offset) + length), totalLength: BigInt(bytes.length), sha256: digest});
    },
  };
  expect(await readPromptOriginal(client, "session-1", "artifact-2")).toBe("한".repeat(90_000));
  expect(calls).toEqual([[0n, 256 * 1024], [BigInt(256 * 1024), bytes.length - 256 * 1024]]);
  await expect(readPromptOriginal({...client, readChunk: async request => {
    const part = await client.readChunk(request);
    return request.offset ? create(ReadChunkResponseSchema, {...part, data: part.data.slice(0, -1)}) : part;
  }}, "session-1", "artifact-2")).rejects.toThrow("integrity");
});

test("artifact metadata rejects unsupported identity, media, size and digest", async () => {
  const bytes = new TextEncoder().encode("원문");
  const digest = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))).map(byte => byte.toString(16).padStart(2, "0")).join("");
  const valid = {artifactRef: "artifact-3", mediaType: "text/markdown; charset=utf-8", byteLength: BigInt(bytes.length), sha256: digest};
  const client = {
    getMetadata: async () => create(GetMetadataResponseSchema, valid),
    readChunk: async () => create(ReadChunkResponseSchema, {data: bytes, totalLength: BigInt(bytes.length), sha256: digest}),
  };
  expect(await readPromptOriginal(client, "session-1", "artifact-3")).toBe("원문");
  for (const invalid of [
    {artifactRef: "wrong"}, {mediaType: "image/svg+xml"}, {byteLength: -1n}, {sha256: "bad"},
  ]) {
    await expect(readPromptOriginal({...client, getMetadata: async () => create(GetMetadataResponseSchema, {...valid, ...invalid})},
      "session-1", "artifact-3")).rejects.toThrow("metadata");
  }
  await expect(readPromptOriginal({...client, readChunk: async () => create(ReadChunkResponseSchema,
    {data: bytes, totalLength: BigInt(bytes.length) + 1n, sha256: digest})}, "session-1", "artifact-3")).rejects.toThrow("integrity");
  const changed = bytes.slice();
  changed[0] = changed[0]! ^ 1;
  await expect(readPromptOriginal({...client, readChunk: async () => create(ReadChunkResponseSchema,
    {data: changed, totalLength: BigInt(bytes.length), sha256: digest})}, "session-1", "artifact-3")).rejects.toThrow("integrity");
});
