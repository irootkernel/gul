import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {renderToStaticMarkup} from "react-dom/server";
import {FilePreviewKind, ReadPreviewResponseSchema} from "../../api/generated/ts/gul/v1/gul_pb";
import {FilePreview} from "./file-preview";

test("Markdown preview renders only server-validated raster assets", () => {
 const preview = create(ReadPreviewResponseSchema, {
  kind: FilePreviewKind.MARKDOWN,
  text: '# Review\n![safe](safe.png)\n![external](https://example.invalid/a.png)\n<script>alert(1)</script>',
  markdownImages: [{reference: "safe.png", mimeType: "image/png", data: new Uint8Array([1, 2, 3])},
   {reference: "https://example.invalid/a.png", mimeType: "image/svg+xml", data: new Uint8Array([1])}],
 });
 const html = renderToStaticMarkup(<FilePreview preview={preview} />);
 expect(html).toContain('<img src="data:image/png;');
 expect(html).toContain('data:image/png;base64,AQID');
 expect(html).toContain('![external](https://example.invalid/a.png)');
 expect(html).toContain('&lt;script&gt;');
 expect(html).not.toContain('<script>');
 expect(html).not.toContain('src="https:');
});

test("SVG stays source and identifiable text receives inert highlighting", () => {
 const svg = renderToStaticMarkup(<FilePreview preview={create(ReadPreviewResponseSchema, {
  kind: FilePreviewKind.SVG_SOURCE, text: '<svg onload="alert(1)"/>', language: "xml",
 })} />);
 expect(svg).toContain('&lt;svg onload=');
 expect(svg).not.toContain('<svg');
 expect(svg).not.toContain('<img');
 const go = renderToStaticMarkup(<FilePreview preview={create(ReadPreviewResponseSchema, {
  kind: FilePreviewKind.TEXT, text: "package main\nfunc main() {}", language: "go", truncated: true,
 })} />);
 expect(go).toContain('class="file-token-keyword"');
 expect(go).toContain('Preview truncated');
});

test("repeated Markdown references cannot multiply a validated raster asset without bound", () => {
 const preview = create(ReadPreviewResponseSchema, {
  kind: FilePreviewKind.MARKDOWN,
  text: "![x](safe.png) ".repeat(2000),
  markdownImages: [{reference: "safe.png", mimeType: "image/png", data: new Uint8Array([1, 2, 3])}],
 });
 const html = renderToStaticMarkup(<FilePreview preview={preview} />);
 expect((html.match(/<img\b/g) ?? []).length).toBe(8);
 expect((html.match(/data:image\/png;base64/g) ?? []).length).toBe(8);
 expect(html).toContain("![x](safe.png)");
});
