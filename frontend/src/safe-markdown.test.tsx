import {expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {SafeMarkdown} from "./safe-markdown";

test("verified content renders only inert markdown elements", () => {
 const text = '# Result\r\n\r\n<script>window.pwned=1</script>\n<img src="https://example.invalid/leak">\n![image](https://example.invalid/image)\n[run](javascript:alert(1))\n../../etc/passwd\n\n```html\n<iframe src="file:///etc/passwd"></iframe>\n```';
 const html = renderToStaticMarkup(<SafeMarkdown text={text} />);
 expect(html).toContain("<h2>Result</h2>"); expect(html).toContain("<pre><code>");
 expect(html).toContain("&lt;script&gt;"); expect(html).toContain("../../etc/passwd");
 expect(html).not.toMatch(/<(script|img|iframe|a|object|embed)\b/);
 expect(html).not.toMatch(/<[^>]+\bsrc=/);
});
test("Unicode and text remain visible without HTML interpretation", () => {
 const html = renderToStaticMarkup(<SafeMarkdown text={'한글 原文\n\n## Detail\nA & B < C'} />);
 expect(html).toContain("한글 原文");expect(html).toContain("<h3>Detail</h3>");expect(html).toContain("A &amp; B &lt; C");
});
test("image reference grammar preserves the server's 256/512 bounds", () => {
 const image = {mimeType: "image/png", data: new Uint8Array([1])};
 const valid = ["a.png", "a".repeat(512)];
 for (const reference of valid) {
  const html = renderToStaticMarkup(<SafeMarkdown text={`![${"a".repeat(256)}](${reference})`} images={new Map([[reference, image]])} />);
  expect(html).toContain("<img");
 }
 const longAlt = renderToStaticMarkup(<SafeMarkdown text={`![${"a".repeat(257)}](a.png)`} images={new Map([["a.png", image]])} />);
 const longReference = renderToStaticMarkup(<SafeMarkdown text={`![x](${"a".repeat(513)})`} images={new Map([["a".repeat(513), image]])} />);
 expect(longAlt).not.toContain("<img");
 expect(longReference).not.toContain("<img");
});
