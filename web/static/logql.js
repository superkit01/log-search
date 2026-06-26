// 纯 LogQL 构造逻辑。不访问 DOM,因此可在 Node 下单测。
// 浏览器里通过 <script src="/static/logql.js"> 加载(函数成为全局);
// Node 下通过 require("./static/logql.js") 引入。

// escapeLogQLValue 转义反斜杠和双引号,使值能安全放进 LogQL 双引号字符串字面量。
function escapeLogQLValue(v) {
  return String(v).replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

// buildLogQL 由筛选条件拼装 LogQL。无流选择器(chips 为空)时返回 null,
// 由调用方提示错误。chips: [{label, value}];重复标签折叠(后值覆盖)。
function buildLogQL(chips, keyword, traceId) {
  const byLabel = {};
  const order = [];
  for (const c of chips) {
    if (!c || !c.label || c.value == null || c.value === "") continue;
    if (!(c.label in byLabel)) order.push(c.label);
    byLabel[c.label] = c.value;
  }
  if (order.length === 0) return null;
  const selector = "{" +
    order.map((l) => l + '="' + escapeLogQLValue(byLabel[l]) + '"').join(", ") +
    "}";
  const parts = [selector];
  const kw = (keyword || "").trim();
  if (kw) parts.push('|= "' + escapeLogQLValue(kw) + '"');
  const tid = (traceId || "").trim();
  if (tid) parts.push('| trace_id="' + escapeLogQLValue(tid) + '"');
  return parts.join(" ");
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { escapeLogQLValue, buildLogQL };
}
