import type {ReactNode} from "react";
import {maximumFileImageBytes, maximumFileMarkdownImages} from "../../api/generated/ts/gul/v1/bounds";

export type SafeImage = {mimeType: string; data: Uint8Array};

export function imageDataURL(image: SafeImage): string | undefined {
 const allowed = ["image/png", "image/jpeg", "image/webp", "image/gif"];
 if (!allowed.includes(image.mimeType) || image.data.byteLength > maximumFileImageBytes) return undefined;
 let binary = "";
 for (let offset = 0; offset < image.data.byteLength; offset += 8192) {
  binary += String.fromCharCode(...image.data.subarray(offset, offset + 8192));
 }
 return `data:${image.mimeType};base64,${btoa(binary)}`;
}

function inlineImages(text: string, images: ReadonlyMap<string, SafeImage> | undefined, remaining: {count: number}, urls: Map<string, string>): ReactNode[] {
 if (!images?.size) return [text];
 const result: ReactNode[] = [];
 const pattern = /!\[([^\]\n]{0,256})\]\(([^)\n]{1,512})\)/g;
 let start = 0;
 for (const match of text.matchAll(pattern)) {
  const index = match.index;
  const reference = match[2]!;
  const image = images.get(reference);
  let src = urls.get(reference);
  if (!src && image && remaining.count > 0) {
   src = imageDataURL(image);
   if (src) urls.set(reference, src);
  }
  if (!src || remaining.count === 0) continue;
  if (index > start) result.push(text.slice(start, index));
  result.push(<img key={index} src={src} alt={match[1]} loading="lazy" />);
  remaining.count--;
  start = index + match[0].length;
 }
 result.push(text.slice(start));
 return result;
}

// A small presentation allowlist: text, paragraphs, headings, fenced code, and verified raster images.
// HTML, links, unverified images, and embedded resources remain literal text.
export function SafeMarkdown({text, images}: {text: string; images?: ReadonlyMap<string, SafeImage>}) {
 const blocks: ReactNode[] = [];
 const remaining = {count: maximumFileMarkdownImages};
 const urls = new Map<string, string>();
 const lines = text.split(/\r\n|\n|\r/);
 let paragraph: string[] = [];
 let code: string[] | undefined;
 const flush = () => {
  if (paragraph.length) blocks.push(<p key={blocks.length} style={{whiteSpace: "pre-wrap"}}>{inlineImages(paragraph.join("\n"), images, remaining, urls)}</p>);
  paragraph = [];
 };
 for (const line of lines) {
  if (line.startsWith("```")) {
   flush();
   if (code) {blocks.push(<pre key={blocks.length}><code>{code.join("\n")}</code></pre>); code = undefined;}
   else code = [];
  } else if (code) code.push(line);
  else if (line.startsWith("# ")) {flush(); blocks.push(<h2 key={blocks.length}>{inlineImages(line.slice(2), images, remaining, urls)}</h2>);}
  else if (line.startsWith("## ")) {flush(); blocks.push(<h3 key={blocks.length}>{inlineImages(line.slice(3), images, remaining, urls)}</h3>);}
  else if (line.trim() === "") flush();
  else paragraph.push(line);
 }
 flush();
 if (code) blocks.push(<pre key={blocks.length}><code>{code.join("\n")}</code></pre>);
 return <article aria-label="Verified content">{blocks}</article>;
}
