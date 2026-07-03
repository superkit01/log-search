const test = require("node:test");
const assert = require("node:assert");
const { traceDrilldownState } = require("./static/trace.js");

test("traceDrilldownState: builds a 15 minute window around the log line", () => {
  const got = traceDrilldownState(
    { env: "test", app: "" },
    {
      ts: "2026-07-02T09:13:16.000Z",
      labels: { app: "boeet-alarm" },
    },
    "TID:abc"
  );

  assert.deepStrictEqual(got, {
    env: "test",
    app: "",
    levels: [],
    direction: "FORWARD",
    keyword: "",
    traceId: "TID:abc",
    start: 1782982696,
    end: 1782984496,
  });
});

test("traceDrilldownState: returns null when timestamp or trace id is missing", () => {
  assert.strictEqual(traceDrilldownState({}, { ts: "bad", labels: {} }, "TID:abc"), null);
  assert.strictEqual(traceDrilldownState({}, { ts: "2026-07-02T09:13:16.000Z", labels: {} }, ""), null);
});
