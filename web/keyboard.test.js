const test = require("node:test");
const assert = require("node:assert");
const { shouldSubmitInputEnter } = require("./static/keyboard.js");

test("shouldSubmitInputEnter: normal input enter submits", () => {
  assert.strictEqual(
    shouldSubmitInputEnter({ key: "Enter", target: { tagName: "INPUT" } }, false),
    true,
  );
});

test("shouldSubmitInputEnter: ignores IME composition enter", () => {
  assert.strictEqual(
    shouldSubmitInputEnter({ key: "Enter", target: { tagName: "INPUT" }, isComposing: true }, false),
    false,
  );
  assert.strictEqual(
    shouldSubmitInputEnter({ key: "Enter", target: { tagName: "INPUT" }, keyCode: 229 }, false),
    false,
  );
  assert.strictEqual(
    shouldSubmitInputEnter({ key: "Enter", target: { tagName: "INPUT" } }, true),
    false,
  );
});

test("shouldSubmitInputEnter: ignores non-input or non-enter keys", () => {
  assert.strictEqual(
    shouldSubmitInputEnter({ key: "Enter", target: { tagName: "TEXTAREA" } }, false),
    false,
  );
  assert.strictEqual(
    shouldSubmitInputEnter({ key: "Escape", target: { tagName: "INPUT" } }, false),
    false,
  );
});
