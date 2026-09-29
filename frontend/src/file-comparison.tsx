import {useState} from "react";
import {
 FileChangeKind, FileGitState, type CompareFixedRevisionsResponse,
 type GetGitStatusResponse, type ReadPreviewResponse,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {FilePreview} from "./file-preview";

const changeLabels: Partial<Record<FileChangeKind, [string, string]>> = {
 [FileChangeKind.CLEAN]: ["✓", "Clean"],
 [FileChangeKind.MODIFIED]: ["●", "Modified"],
 [FileChangeKind.ADDED]: ["+", "Added"],
 [FileChangeKind.UNTRACKED]: ["?", "Untracked"],
 [FileChangeKind.DELETED]: ["−", "Deleted"],
 [FileChangeKind.RENAMED]: ["↪", "Renamed"],
 [FileChangeKind.CONFLICTED]: ["!", "Conflicted"],
 [FileChangeKind.MIXED]: ["◆", "Mixed changes"],
};

export function FileChangeBadge({kind, aggregate = false}: {kind: FileChangeKind; aggregate?: boolean}) {
 const [symbol, label] = changeLabels[kind] ?? ["?", "Status unavailable"];
 return <span className={`file-change file-change--${FileChangeKind[kind]?.toLowerCase() ?? "unknown"}`} aria-label={`${aggregate ? "Contained changes" : "File status"}: ${label}`}>
  <span aria-hidden="true">{symbol}</span> {label}
 </span>;
}

export function FileStatus({status}: {status: GetGitStatusResponse}) {
 if (status.state !== FileGitState.AVAILABLE) return <p role="status">Git review is unavailable. Current file browsing remains available.</p>;
 return <div className="file-status">
  <FileChangeBadge kind={status.direct} />
  <FileChangeBadge kind={status.aggregate} aggregate />
  {status.staged && <span>Staged</span>}{status.unstaged && <span>Unstaged</span>}
 </div>;
}

function Revision({name, preview, missing}: {name: string; preview: ReadPreviewResponse | undefined; missing: boolean}) {
 return <section aria-label={`${name} revision`}><h3>{name}</h3>{missing || !preview ? <p>File missing in this revision.</p> : <FilePreview preview={preview} />}</section>;
}

// Wide layouts show both fixed revisions; narrow layouts offer an explicit
// revision switch so neither source is silently substituted for the other.
export function FileComparison({comparison}: {comparison: CompareFixedRevisionsResponse}) {
 const [selected, setSelected] = useState<"HEAD" | "Working">("Working");
 const available = comparison.state === FileGitState.AVAILABLE;
 return <section aria-label="File comparison" className="file-comparison">
  <p>Working files may change while a writer is active.</p>
  {comparison.previousRelativePath && <p>Renamed from {comparison.previousRelativePath}</p>}
  {available ? <>
   <div className="file-comparison__wide">
    <Revision name="HEAD" preview={comparison.head} missing={comparison.headMissing} />
    <Revision name="Working" preview={comparison.working} missing={comparison.workingMissing} />
   </div>
   <div className="file-comparison__narrow">
    <div role="group" aria-label="Revision">
     <button type="button" aria-pressed={selected === "HEAD"} onClick={() => setSelected("HEAD")}>HEAD</button>
     <button type="button" aria-pressed={selected === "Working"} onClick={() => setSelected("Working")}>Working</button>
    </div>
    <Revision name={selected} preview={selected === "HEAD" ? comparison.head : comparison.working} missing={selected === "HEAD" ? comparison.headMissing : comparison.workingMissing} />
   </div>
  </> : <>
   <p role="status">Git comparison is unavailable. Current file preview remains available.</p>
   <Revision name="Working" preview={comparison.working} missing={comparison.workingMissing} />
  </>}
 </section>;
}
