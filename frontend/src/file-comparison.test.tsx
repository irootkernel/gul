import {expect, test} from "bun:test";
import {create} from "@bufbuild/protobuf";
import {renderToStaticMarkup} from "react-dom/server";
import {
 CompareFixedRevisionsResponseSchema, FileChangeKind, FileGitState,
 FilePreviewKind, GetGitStatusResponseSchema, ReadPreviewResponseSchema,
} from "../../api/generated/ts/gul/v1/gul_pb";
import {FileComparison, FileStatus} from "./file-comparison";

test("Git status exposes a non-color label and staged state", () => {
 const status = create(GetGitStatusResponseSchema, {
  state: FileGitState.AVAILABLE, direct: FileChangeKind.RENAMED,
  aggregate: FileChangeKind.MIXED, staged: true, unstaged: true,
 });
 const html = renderToStaticMarkup(<FileStatus status={status} />);
 expect(html).toContain("File status: Renamed");
 expect(html).toContain("Contained changes: Mixed changes");
 expect(html).toContain("Staged");
 expect(html).toContain("Unstaged");
});

test("comparison displays both fixed revisions and missing sides", () => {
 const result = create(CompareFixedRevisionsResponseSchema, {
  state: FileGitState.AVAILABLE, change: FileChangeKind.DELETED,
  head: create(ReadPreviewResponseSchema, {kind: FilePreviewKind.TEXT, text: "old"}),
  workingMissing: true,
 });
 const html = renderToStaticMarkup(<FileComparison comparison={result} />);
 expect(html).toContain("HEAD revision");
 expect(html).toContain("Working revision");
 expect(html).toContain("old");
 expect(html).toContain("File missing in this revision.");
 expect(html).toContain('aria-pressed="true"');
});

test("Git degradation retains the current preview", () => {
 const result = create(CompareFixedRevisionsResponseSchema, {
  state: FileGitState.NOT_REPOSITORY,
  working: create(ReadPreviewResponseSchema, {kind: FilePreviewKind.TEXT, text: "current"}),
 });
 const html = renderToStaticMarkup(<FileComparison comparison={result} />);
 expect(html).toContain("Git comparison is unavailable");
 expect(html).toContain("current");
});
