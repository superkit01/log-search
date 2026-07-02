const test = require("node:test");
const assert = require("node:assert");
const { encodeShareState, decodeShareState } = require("./static/share.js");

test("encodeShareState: encodes current filters into query params", () => {
  const qs = encodeShareState({
    env: "prod",
    app: "api",
    levels: ["warn", "error"],
    direction: "FORWARD",
    keyword: "timeout error",
    traceId: "abc/123",
    start: 1782892800,
    end: 1782979140,
  });
  const params = new URLSearchParams(qs);

  assert.strictEqual(params.get("env"), "prod");
  assert.strictEqual(params.get("app"), "api");
  assert.strictEqual(params.get("levels"), "warn,error");
  assert.strictEqual(params.get("direction"), "FORWARD");
  assert.strictEqual(params.get("keyword"), "timeout error");
  assert.strictEqual(params.get("trace_id"), "abc/123");
  assert.strictEqual(params.get("start"), "1782892800");
  assert.strictEqual(params.get("end"), "1782979140");
});

test("encodeShareState: skips empty optional values", () => {
  assert.strictEqual(
    encodeShareState({ env: "test", levels: [], direction: "BACKWARD", app: "", keyword: "", traceId: "" }),
    "env=test&direction=BACKWARD"
  );
});

test("decodeShareState: returns null when no share params exist", () => {
  assert.strictEqual(decodeShareState("?foo=bar"), null);
  assert.strictEqual(decodeShareState(""), null);
});

test("decodeShareState: decodes query params into state", () => {
  assert.deepStrictEqual(
    decodeShareState("?env=prod&app=api&levels=warn%2Cerror&direction=FORWARD&keyword=timeout+error&trace_id=abc%2F123&start=1782892800&end=1782979140"),
    {
      env: "prod",
      app: "api",
      levels: ["warn", "error"],
      direction: "FORWARD",
      keyword: "timeout error",
      traceId: "abc/123",
      start: 1782892800,
      end: 1782979140,
    }
  );
});

test("decodeShareState: ignores invalid levels, direction, and timestamps", () => {
  assert.deepStrictEqual(
    decodeShareState("?levels=debug,bad,error&direction=SIDEWAYS&start=abc&end=-1"),
    {
      env: "",
      app: "",
      levels: ["debug", "error"],
      direction: "",
      keyword: "",
      traceId: "",
      start: 0,
      end: 0,
    }
  );
});
