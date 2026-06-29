const test = require("node:test");
const assert = require("node:assert");
const { escapeLogQLValue, buildLogQL, levelMatcher } = require("./static/logql.js");

test("escapeLogQLValue: 普通值不变", () => {
  assert.strictEqual(escapeLogQLValue("nginx"), "nginx");
});

test("escapeLogQLValue: 转义双引号", () => {
  assert.strictEqual(escapeLogQLValue('a"b'), 'a\\"b');
});

test("escapeLogQLValue: 转义反斜杠", () => {
  assert.strictEqual(escapeLogQLValue("a\\b"), "a\\\\b");
});

test("escapeLogQLValue: 非字符串被强制转换", () => {
  assert.strictEqual(escapeLogQLValue(123), "123");
});

test("buildLogQL: 无 chip 返回 null", () => {
  assert.strictEqual(buildLogQL([], "", ""), null);
});

test("buildLogQL: 单个 chip", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "", ""),
    '{app="nginx"}'
  );
});

test("buildLogQL: 多个 chip 用逗号 AND 组合", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }, { label: "env", value: "local" }], "", ""),
    '{app="nginx", env="local"}'
  );
});

test("buildLogQL: 重复标签折叠,后值覆盖", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "a" }, { label: "app", value: "b" }], "", ""),
    '{app="b"}'
  );
});

test("buildLogQL: 空值 chip 被跳过", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "" }, { label: "env", value: "local" }], "", ""),
    '{env="local"}'
  );
});

test("buildLogQL: 关键词追加为 |= 过滤", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "error", ""),
    '{app="nginx"} |= "error"'
  );
});

test("buildLogQL: trace_id 追加为结构化元数据过滤", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "", "abc123"),
    '{app="nginx"} | trace_id="abc123"'
  );
});

test("buildLogQL: 关键词 + trace_id 同时存在", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "error", "abc123"),
    '{app="nginx"} |= "error" | trace_id="abc123"'
  );
});

test("buildLogQL: 各值中的特殊字符被转义", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: 'a"b' }], 'c"d', 'e"f'),
    '{app="a\\"b"} |= "c\\"d" | trace_id="e\\"f"'
  );
});

test("levelMatcher: debug/空/undefined/未知 → null", () => {
  assert.strictEqual(levelMatcher("debug"), null);
  assert.strictEqual(levelMatcher(""), null);
  assert.strictEqual(levelMatcher(undefined), null);
  assert.strictEqual(levelMatcher("bogus"), null);
});

test("levelMatcher: info → info 及以上(大小写不敏感)", () => {
  assert.strictEqual(levelMatcher("info"), 'level=~"(?i)info|warn|error"');
});

test("levelMatcher: warn → warn 及以上", () => {
  assert.strictEqual(levelMatcher("warn"), 'level=~"(?i)warn|error"');
});

test("levelMatcher: error → 仅 error", () => {
  assert.strictEqual(levelMatcher("error"), 'level=~"(?i)error"');
});

test("buildLogQL: levels=[warn] 并入流选择器作大小写不敏感 level=~ 匹配器", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "", "", ["warn"]),
    '{app="nginx", level=~"(?i)warn"}'
  );
});

test("buildLogQL: levels 为空数组时不加 level 匹配器", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "", "", []),
    '{app="nginx"}'
  );
});

test("buildLogQL: 无 chip 且 levels=[warn] → 仅 level 选择器(合法)", () => {
  assert.strictEqual(buildLogQL([], "", "", ["warn"]), '{level=~"(?i)warn"}');
});

test("buildLogQL: env+app+levels+kw+trace 完整组合", () => {
  assert.strictEqual(
    buildLogQL(
      [{ label: "env", value: "prod" }, { label: "app", value: "nginx" }],
      "timeout",
      "t1",
      ["info", "warn", "error"]
    ),
    '{env="prod", app="nginx", level=~"(?i)info|warn|error"} |= "timeout" | trace_id="t1"'
  );
});
