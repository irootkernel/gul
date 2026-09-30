import {expect, test} from "bun:test";
import {fileURLToPath} from "node:url";

test("public mounts reuse one root, reject another root, and pass writer activity", () => {
  // Keep the React DOM mock inside one bounded child so other tests use real imports.
  const script = `
    import {mock} from "bun:test";
    import {strict as assert} from "node:assert";
    let created = 0;
    const rendered = [];
    mock.module("react-dom/client", () => ({createRoot: () => {
      created++;
      return {render: element => rendered.push(element)};
    }}));
    const {mountFoundation, mountOperator, mountAuthenticated} = await import("./frontend/src/gul.tsx");
    const root = {}, clients = {}, auth = {};
    mountFoundation(root);
    mountOperator(root, clients, true);
    assert.equal(rendered.at(-1).props.children.props.writerActive, true);
    assert.equal(rendered.at(-1).props.children.props.clients, clients);
    mountOperator(root, clients, false);
    assert.equal(rendered.at(-1).props.children.props.writerActive, false);
    mountAuthenticated(root, auth, clients);
    assert.equal(created, 1);
    assert.equal(rendered.at(-1).props.children.props.client, auth);
    assert.throws(() => mountOperator({}, clients), /root changed/);
    assert.throws(() => mountAuthenticated(null, auth, clients), /root is missing/);
    assert.equal(created, 1);
    assert.equal(rendered.length, 4);
  `;
  const child = Bun.spawnSync({cmd: [process.execPath, "--eval", script],
    cwd: fileURLToPath(new URL("../../", import.meta.url)), timeout: 10_000,
    stdout: "pipe", stderr: "pipe"});
  expect({exit: child.exitCode, error: child.stderr.toString()}).toEqual({exit: 0, error: ""});
});
