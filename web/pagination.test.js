const test = require("node:test");
const assert = require("node:assert");
const { shouldAutoLoadMore } = require("./static/pagination.js");

test("shouldAutoLoadMore: near bottom with cursor and idle", () => {
  assert.strictEqual(shouldAutoLoadMore({
    scrollTop: 920,
    clientHeight: 500,
    scrollHeight: 1500,
  }, true, false), true);
});

test("shouldAutoLoadMore: blocks when far from bottom, no cursor, or loading", () => {
  const metrics = { scrollTop: 100, clientHeight: 500, scrollHeight: 1500 };

  assert.strictEqual(shouldAutoLoadMore(metrics, true, false), false);
  assert.strictEqual(shouldAutoLoadMore({ scrollTop: 920, clientHeight: 500, scrollHeight: 1500 }, false, false), false);
  assert.strictEqual(shouldAutoLoadMore({ scrollTop: 920, clientHeight: 500, scrollHeight: 1500 }, true, true), false);
});
