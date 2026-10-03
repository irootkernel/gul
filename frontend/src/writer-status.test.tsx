import {expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {WriterAccessMode} from "../../api/generated/ts/gul/v1/gul_pb";
import {WriterStatus} from "./writer-status";

test("only an explicit backend WRITE mode is displayed as WRITE", () => {
  for (const mode of [WriterAccessMode.UNSPECIFIED, WriterAccessMode.READ_ONLY,
    WriterAccessMode.WRITE, WriterAccessMode.BLOCKED, WriterAccessMode.UNVERIFIED, 99 as WriterAccessMode]) {
    const html = renderToStaticMarkup(<WriterStatus mode={mode} />);
    expect(html.includes(">WRITE<")).toBe(mode === WriterAccessMode.WRITE);
  }
});

test("declared unverified mode stays distinct from blocked and WRITE", () => {
  expect(renderToStaticMarkup(<WriterStatus mode={WriterAccessMode.UNVERIFIED} />)).toContain("Policy unverified");
  expect(renderToStaticMarkup(<WriterStatus mode={99 as WriterAccessMode} />)).toContain("Blocked");
});
