// 纯 LogQL 构造逻辑。不访问 DOM,因此可在 Node 下单测。
// 浏览器里通过 <script src="/static/logql.js"> 加载(函数成为全局);
// Node 下通过 require("./static/logql.js") 引入。

// escapeLogQLValue 转义反斜杠和双引号,使值能安全放进 LogQL 双引号字符串字面量。
function escapeLogQLValue(v) {
  return String(v).replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

// 级别严重度升序。level 视作 Loki 流标签(值小写 debug/info/warn/error,与 logparse 一致)。
const LEVEL_ORDER = ["debug", "info", "warn", "error"];

// levelMatcher: 入参阈值级别,返回 "该级别及以上" 的 level 标签匹配器字符串,
// 或 null(debug/空/未知 → 不加匹配器,显示全部,含无 level 标签的流)。
// 例: "warn" → 'level=~"warn|error"'; "info" → 'level=~"info|warn|error"'; "error" → 'level=~"error"'。
// 匹配值是固定可信串(级别名来自常量,无用户输入),不转义。
function levelMatcher(level) {
  const idx = LEVEL_ORDER.indexOf(level);
  if (idx <= 0) return null; // debug(0)、空(-1)、未知(-1)都不加
  return 'level=~"' + LEVEL_ORDER.slice(idx).join("|") + '"';
}

// buildLogQL 由筛选条件拼装 LogQL。流选择器(chips + level 匹配器)为空时返回 null,
// 由调用方提示错误。chips: [{label, value}];重复标签折叠(后值覆盖)。
// level: "debug"/"info"/"warn"/"error";非 debug 时并入流选择器作 level=~"..." 匹配器。
function buildLogQL(chips, keyword, traceId, level) {
  const byLabel = {};
  const order = [];
  for (const c of chips) {
    if (!c || !c.label || c.value == null || c.value === "") continue;
    if (!(c.label in byLabel)) order.push(c.label);
    byLabel[c.label] = c.value;
  }
  const lm = levelMatcher(level);
  if (order.length === 0 && !lm) return null;
  const matchers = order.map((l) => l + '="' + escapeLogQLValue(byLabel[l]) + '"');
  if (lm) matchers.push(lm);
  const parts = ["{" + matchers.join(", ") + "}"];
  const kw = (keyword || "").trim();
  if (kw) parts.push('|= "' + escapeLogQLValue(kw) + '"');
  const tid = (traceId || "").trim();
  if (tid) parts.push('| trace_id="' + escapeLogQLValue(tid) + '"');
  return parts.join(" ");
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { escapeLogQLValue, buildLogQL, levelMatcher };
}
