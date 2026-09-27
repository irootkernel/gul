import {expect, test} from "bun:test";
import {renderToStaticMarkup} from "react-dom/server";
import {WriterAccessMode} from "../../api/generated/ts/gul/v1/gul_pb";
import {WriterStatus} from "./writer-status";

test("only an explicit backend WRITE mode is displayed as WRITE", () => {
  for (const mode of [WriterAccessMode.UNSPECIFIED, WriterAccessMode.READ_ONLY,
    WriterAccessMode.WRITE, WriterAccessMode.BLOCKED, 99 as WriterAccessMode]) {
    const html = renderToStaticMarkup(<WriterStatus mode={mode} />);
    expect(html.includes(">WRITE<")).toBe(mode === WriterAccessMode.WRITE);
  }
});
