const test = require("node:test");
const assert = require("node:assert");
const { shouldSubmitQueryEnter } = require("../static/js/keyboard.js");

const input = { tagName: "INPUT", closest: () => null };

test("shouldSubmitQueryEnter: page, filters and focused buttons submit", () => {
  for (const tagName of ["BODY", "INPUT", "BUTTON", "SUMMARY"]) {
    const target = { tagName, closest: () => null };
    assert.strictEqual(shouldSubmitQueryEnter({ key: "Enter", target }, false), true, tagName);
  }
});

test("shouldSubmitQueryEnter: ignores IME composition enter", () => {
  assert.strictEqual(
    shouldSubmitQueryEnter({ key: "Enter", target: input, isComposing: true }, false),
    false,
  );
  assert.strictEqual(
    shouldSubmitQueryEnter({ key: "Enter", target: input, keyCode: 229 }, false),
    false,
  );
  assert.strictEqual(
    shouldSubmitQueryEnter({ key: "Enter", target: input }, true),
    false,
  );
});

test("shouldSubmitQueryEnter: preserves the active dialog", () => {
  const target = { closest: selector => selector === "dialog[open]" ? target : null };
  assert.strictEqual(shouldSubmitQueryEnter({ key: "Enter", target }, false), false);
});

test("shouldSubmitQueryEnter: ignores repeats, modified keys and handled events", () => {
  const enter = { key: "Enter", target: input };
  assert.strictEqual(shouldSubmitQueryEnter({ ...enter, key: "Escape" }, false), false);
  for (const flag of ["repeat", "altKey", "ctrlKey", "metaKey", "shiftKey", "defaultPrevented"]) {
    assert.strictEqual(shouldSubmitQueryEnter({ ...enter, [flag]: true }, false), false, flag);
  }
});
