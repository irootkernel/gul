import type {ReactNode} from "react";

// A small presentation allowlist: text, paragraphs, headings and fenced code.
// HTML, links, images and embedded resources remain literal text.
export function SafeMarkdown({text}: {text: string}) {
 const blocks: ReactNode[] = [];
 const lines = text.split(/\r\n|\n|\r/);
 let paragraph: string[] = [];
 let code: string[] | undefined;
 const flush = () => {
  if (paragraph.length) blocks.push(<p key={blocks.length} style={{whiteSpace: "pre-wrap"}}>{paragraph.join("\n")}</p>);
  paragraph = [];
 };
 for (const line of lines) {
  if (line.startsWith("```")) {
   flush();
   if (code) {blocks.push(<pre key={blocks.length}><code>{code.join("\n")}</code></pre>); code = undefined;}
   else code = [];
  } else if (code) code.push(line);
  else if (line.startsWith("# ")) {flush(); blocks.push(<h2 key={blocks.length}>{line.slice(2)}</h2>);}
  else if (line.startsWith("## ")) {flush(); blocks.push(<h3 key={blocks.length}>{line.slice(3)}</h3>);}
  else if (line.trim() === "") flush();
  else paragraph.push(line);
 }
 flush();
 if (code) blocks.push(<pre key={blocks.length}><code>{code.join("\n")}</code></pre>);
 return <article aria-label="Verified content">{blocks}</article>;
}
