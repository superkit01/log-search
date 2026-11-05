const test = require("node:test");
const assert = require("node:assert");
const { traceDrilldownState } = require("../static/js/trace.js");

test("traceDrilldownState: preserves the current query time range", () => {
  const got = traceDrilldownState(
    { env: "test", app: "", direction: "BACKWARD", spanId: "span", parentSpanId: "parent", taskId: "task", start: 1782950400, end: 1783036799 },
    {
      ts: "2026-07-02T09:13:16.000Z",
      labels: { app: "alert-service" },
    },
    "TID:abc"
  );

  assert.deepStrictEqual(got, {
    namespace: "",
    env: "test",
    app: "",
    levels: [],
    direction: "BACKWARD",
    keyword: "",
    traceId: "TID:abc",
    start: 1782950400,
    end: 1783036799,
  });
});

test("traceDrilldownState: returns null when current state or trace id is missing", () => {
  assert.strictEqual(traceDrilldownState(null, { labels: {} }, "TID:abc"), null);
  assert.strictEqual(traceDrilldownState({}, { ts: "2026-07-02T09:13:16.000Z", labels: {} }, ""), null);
});

test("traceDrilldownState: does not require a log timestamp to retain the range", () => {
  const got = traceDrilldownState({ start: 100, end: 200 }, { labels: {} }, " trace ");
  assert.strictEqual(got.start, 100);
  assert.strictEqual(got.end, 200);
  assert.strictEqual(got.traceId, "trace");
});

test("trace drilldown retains namespace", () => {
 assert.strictEqual(traceDrilldownState({namespace:"ems"}, {ts:"2026-10-28T06:00:00Z"}, "trace").namespace, "ems");
});
