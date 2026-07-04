const test = require("node:test");
const assert = require("node:assert");
const { quickRangeTimestamps, defaultStartTs, defaultEndTs } = require("./static/timeRange.js");

test("quickRangeTimestamps: 1h and 3h use rounded hour starts and today end", () => {
  const now = new Date("2026-07-03T08:35:12+08:00");

  assert.deepStrictEqual(quickRangeTimestamps("1h", now), {
    start: 1783033200,
    end: 1783094340,
  });
  assert.deepStrictEqual(quickRangeTimestamps("3h", now), {
    start: 1783026000,
    end: 1783094340,
  });
});

test("quickRangeTimestamps: today starts at 00:00 and ends at 23:59", () => {
  const now = new Date("2026-07-03T08:35:12+08:00");

  assert.deepStrictEqual(quickRangeTimestamps("today", now), {
    start: 1783008000,
    end: 1783094340,
  });
});

test("default time range is today", () => {
  const now = new Date("2026-07-03T08:35:12+08:00");
  const range = quickRangeTimestamps("today", now);

  assert.strictEqual(defaultStartTs(now), range.start);
  assert.strictEqual(defaultEndTs(now), range.end);
});
