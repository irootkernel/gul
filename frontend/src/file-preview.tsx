import type {ReactNode} from "react";
import {FilePreviewKind, type ReadPreviewResponse} from "../../api/generated/ts/gul/v1/gul_pb";
import {SafeMarkdown, imageDataURL, type SafeImage} from "./safe-markdown";

const sourceKeywords = /\b(?:break|case|const|continue|def|else|export|false|for|func|function|if|import|let|nil|null|package|return|struct|switch|true|type|var|while)\b/g;

function sourceCode(text: string, language: string): ReactNode {
 if (!language) return text;
 const parts: ReactNode[] = [];
 let offset = 0;
 for (const match of text.matchAll(sourceKeywords)) {
  if (match.index > offset) parts.push(text.slice(offset, match.index));
  parts.push(<span className="file-token-keyword" key={match.index}>{match[0]}</span>);
  offset = match.index + match[0].length;
 }
 parts.push(text.slice(offset));
 return parts;
}

// This component receives only bounded FileService data. It has no edit action.
export function FilePreview({preview}: {preview: ReadPreviewResponse}) {
 const note = preview.truncated ? <p role="status">Preview truncated at the file read limit.</p> : null;
 if (preview.kind === FilePreviewKind.RASTER) {
  const src = imageDataURL({mimeType: preview.mimeType, data: preview.image});
  return <section aria-label="File preview">{src ? <img src={src} alt="Workspace image preview" /> : <p>Image preview unavailable.</p>}</section>;
 }
 if (preview.kind === FilePreviewKind.MARKDOWN) {
  const images = new Map<string, SafeImage>();
  for (const item of preview.markdownImages) images.set(item.reference, {mimeType: item.mimeType, data: item.data});
  return <section aria-label="File preview"><SafeMarkdown text={preview.text} images={images} />{note}</section>;
 }
 if (preview.kind === FilePreviewKind.TEXT || preview.kind === FilePreviewKind.SVG_SOURCE) {
  return <section aria-label="File preview"><pre><code className={preview.language ? `language-${preview.language}` : undefined}>{sourceCode(preview.text, preview.language)}</code></pre>{note}</section>;
 }
 return <section aria-label="File preview"><p>Preview unavailable for this file type or encoding.</p></section>;
}
