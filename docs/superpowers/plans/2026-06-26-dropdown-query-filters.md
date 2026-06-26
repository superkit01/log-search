# 下拉框驱动的查询条件改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把查询区的通用「标签+标签值+添加」chip 构建器替换为环境/级别/应用三个下拉框,级别按阈值生成 LogQL 正则行过滤,环境与级别选择持久化到 localStorage。

**Architecture:** 纯前端改动。`logql.js`(纯函数,Node 单测)新增 `levelRegex` 并给 `buildLogQL` 加第 4 参 `level`;`index.html` 换控件;`app.js` 负责下拉框事件、应用列表加载、localStorage 读写与 `search` 重写。env/app 仍以 chip 形式进 `{}` 流选择器,level 进管道 `|~`。后端零改动。

**Tech Stack:** Go 1.24 + Gin(后端不动)、原生 JS + CDN Chart.js(前端)、`node:test`(logql.js 单测)。

## Global Constraints

- **纯前端**:后端 Go 代码、响应结构(`/api/labels`、`/api/label/:name/values` 返回 `{"data":[...]}`;`/api/query_range` 直接返回 `{lines,total}`)、错误映射契约(`web.lokiStatusAndBody`)均不变。
- `go` 指令钉在 1.24 行(`go 1.24.5`);gin v1.10.0;`go mod tidy` 不得抬升指令或降级 gin(否则视为回归)。
- 日志行用 `textContent` 渲染(防 XSS)——保持。
- `buildLogQL` 第 4 参 `level` **向后兼容**:3 参调用结果不变(现有 `web/logql.test.js` 用例全绿)。
- 无前端构建步骤;plain JS,函数为全局(经典 `<script>` 加载)。
- 每个任务结束 commit;提交信息末尾加 `Co-Authored-By: Claude <noreply@anthropic.com>`。

---

### Task 1: `logql.js` 增加级别阈值正则过滤(TDD)

**Files:**
- Modify: `web/static/logql.js`(全文重写,见 Step 3)
- Test: `web/logql.test.js`(追加用例,见 Step 1)

**Interfaces:**
- Produces: `levelRegex(level: string|null|undefined): string|null` —— 入参 `"debug"/"info"/"warn"/"error"`;`debug`、空串、`undefined`、未知值 → 返回 `null`(不加过滤);`info/warn/error` → 返回形如 `(?i)\b(?:INFO|...|PANIC)\b` 的正则字符串(注意:返回值里是**字面反斜杠** `\b`,JS 源码里写作 `\\b`)。
- Produces: `buildLogQL(chips, keyword, traceId, level)` —— 第 4 参 `level` 缺省时等价 `"debug"`(不过滤);非空且非 debug 时在关键词 `|=` 之后、trace_id 之前追加 `|~ "<levelRegex>"`。正则为固定可信串,**不经 `escapeLogQLValue`**。
- Exports: `{ escapeLogQLValue, buildLogQL, levelRegex }`。

- [ ] **Step 1: 追加失败测试**

在 `web/logql.test.js` 顶部把 require 改为解构出 `levelRegex`:

```js
const { escapeLogQLValue, buildLogQL, levelRegex } = require("./static/logql.js");
```

在文件末尾追加:

```js
test("levelRegex: debug/空/undefined/未知 → null", () => {
  assert.strictEqual(levelRegex("debug"), null);
  assert.strictEqual(levelRegex(""), null);
  assert.strictEqual(levelRegex(undefined), null);
  assert.strictEqual(levelRegex("bogus"), null);
});

test("levelRegex: info → info 及以上关键词并集", () => {
  assert.strictEqual(
    levelRegex("info"),
    "(?i)\\b(?:INFO|INFORMATION|WARN|WARNING|ERROR|ERR|FATAL|PANIC)\\b"
  );
});

test("levelRegex: warn → warn 及以上", () => {
  assert.strictEqual(
    levelRegex("warn"),
    "(?i)\\b(?:WARN|WARNING|ERROR|ERR|FATAL|PANIC)\\b"
  );
});

test("levelRegex: error → 仅 error 及以上", () => {
  assert.strictEqual(
    levelRegex("error"),
    "(?i)\\b(?:ERROR|ERR|FATAL|PANIC)\\b"
  );
});

test("buildLogQL: level=warn 追加为 |~ 正则行过滤", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "", "", "warn"),
    '{app="nginx"} |~ "(?i)\\b(?:WARN|WARNING|ERROR|ERR|FATAL|PANIC)\\b"'
  );
});

test("buildLogQL: level=debug 不追加级别过滤", () => {
  assert.strictEqual(
    buildLogQL([{ label: "app", value: "nginx" }], "", "", "debug"),
    '{app="nginx"}'
  );
});

test("buildLogQL: env+app+level+kw+trace 完整组合,顺序为 selector |= |~ | trace_id", () => {
  assert.strictEqual(
    buildLogQL(
      [{ label: "env", value: "prod" }, { label: "app", value: "nginx" }],
      "timeout",
      "t1",
      "warn"
    ),
    '{env="prod", app="nginx"} |= "timeout" |~ "(?i)\\b(?:WARN|WARNING|ERROR|ERR|FATAL|PANIC)\\b" | trace_id="t1"'
  );
});
```

- [ ] **Step 2: 运行测试,确认失败**

Run: `node --test web/logql.test.js`
Expected: FAIL —— 新增用例报 `levelRegex is not a function`(尚未实现/导出)。原有的 escapeLogQLValue / 3 参 buildLogQL 用例应仍 PASS。

- [ ] **Step 3: 用以下内容整体覆盖 `web/static/logql.js`**

```js
// 纯 LogQL 构造逻辑。不访问 DOM,因此可在 Node 下单测。
// 浏览器里通过 <script src="/static/logql.js"> 加载(函数成为全局);
// Node 下通过 require("./static/logql.js") 引入。

// escapeLogQLValue 转义反斜杠和双引号,使值能安全放进 LogQL 双引号字符串字面量。
function escapeLogQLValue(v) {
  return String(v).replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

// 级别阈值 → 正则。严重度升序:debug < info < warn < error。
// 阈值取"该档及以上"各档关键词的并集,词边界 \b 包裹、大小写不敏感((?i))。
// debug/空/未知 → 返回 null(调用方不加级别过滤,显示全部,含无法识别级别的行)。
// 词边界保证 ERR 不会误命中 ERROR 内部;各 token 顺序不影响正确性。
const LEVEL_ORDER = ["debug", "info", "warn", "error"];
const LEVEL_TOKENS = {
  debug: ["DEBUG", "TRACE"],
  info: ["INFO", "INFORMATION"],
  warn: ["WARN", "WARNING"],
  error: ["ERROR", "ERR", "FATAL", "PANIC"],
};
function levelRegex(level) {
  const idx = LEVEL_ORDER.indexOf(level);
  if (idx <= 0) return null; // debug(0)、空(-1)、未知(-1)都不过滤
  const tokens = LEVEL_ORDER.slice(idx).flatMap((l) => LEVEL_TOKENS[l]);
  return "(?i)\\b(?:" + tokens.join("|") + ")\\b";
}

// buildLogQL 由筛选条件拼装 LogQL。无流选择器(chips 为空)时返回 null,
// 由调用方提示错误。chips: [{label, value}];重复标签折叠(后值覆盖)。
// level: "debug"/"info"/"warn"/"error";debug 或缺省 → 不加级别过滤。
function buildLogQL(chips, keyword, traceId, level) {
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
  const lr = levelRegex(level);
  if (lr) parts.push('|~ "' + lr + '"');
  const tid = (traceId || "").trim();
  if (tid) parts.push('| trace_id="' + escapeLogQLValue(tid) + '"');
  return parts.join(" ");
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { escapeLogQLValue, buildLogQL, levelRegex };
}
```

- [ ] **Step 4: 运行测试,确认全绿**

Run: `node --test web/logql.test.js`
Expected: PASS —— 新增 7 条用例 + 原有用例全部通过。

- [ ] **Step 5: Commit**

```bash
git add web/static/logql.js web/logql.test.js
git commit -m "$(cat <<'EOF'
feat(web): level threshold regex filter in buildLogQL

Add levelRegex and a 4th `level` param to buildLogQL. debug/empty/unknown
yields no filter; info/warn/error append a `|~` regex of the level keywords
at or above the threshold. Backward compatible: existing 3-arg calls are
unchanged.

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 下拉框查询 UI(模板 + app.js + 样式)

**Files:**
- Modify: `web/templates/index.html`(查询区首行替换 + 删 chips 行)
- Modify: `web/static/app.js`(整体重写,见 Step 2)
- Modify: `web/static/style.css`(删 `.chips`/`.chip`/`.chip-x` 死样式)

**Interfaces:**
- Consumes: Task 1 的 `buildLogQL(chips, keyword, traceId, level)`(已实现)。
- Produces: 页面 DOM 新增 `#env-select`、`#level-select`、`#app-select`;移除 `#label-select`、`#value-select`、`#add-chip`、`#chips`。
- localStorage 键:`log-search:filters`,值 `{"env":"prod","level":"info"}`(仅 env、level)。

- [ ] **Step 1: 替换 `web/templates/index.html` 查询区**

把 `<section class="controls">` 之后到关键词行之前的**两个** `.row`(标签/标签值/添加/查询 行 + chips 行)替换为单个新行。精确替换:把下面这段旧代码——

```html
      <div class="row">
        <label>标签
          <select id="label-select"><option value="">-- 选择标签 --</option></select>
        </label>
        <label>标签值
          <select id="value-select" disabled><option value="">-- 先选标签 --</option></select>
        </label>
        <button type="button" id="add-chip">添加</button>
        <button type="button" id="search">查询</button>
      </div>
      <div class="row">
        <div id="chips" class="chips"></div>
      </div>
```

替换为:

```html
      <div class="row">
        <label>环境
          <select id="env-select">
            <option value="">不限</option>
            <option value="dev">开发</option>
            <option value="test">测试</option>
            <option value="prod">生产</option>
          </select>
        </label>
        <label>级别
          <select id="level-select">
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </select>
        </label>
        <label>应用
          <select id="app-select"><option value="">不限</option></select>
        </label>
        <button type="button" id="search">查询</button>
      </div>
```

(其后的关键词 / trace_id 行、时间范围行原样保留。)

- [ ] **Step 2: 用以下内容整体覆盖 `web/static/app.js`**

```js
const $ = (id) => document.getElementById(id);
let chart;
const FILTERS_KEY = "log-search:filters"; // localStorage 键:存 {env, level}

function unixNow() { return Math.floor(Date.now() / 1000); }

function toLocalInput(ts) {
  const d = new Date(ts * 1000);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
}

function toUnix(inputVal) {
  if (!inputVal) return 0;
  return Math.floor(new Date(inputVal).getTime() / 1000);
}

async function api(path) {
  const r = await fetch(path);
  const j = await r.json();
  if (!r.ok) throw new Error(j.error || ("HTTP " + r.status));
  return j;
}

function showError(msg) { const b = $("error-bar"); b.textContent = msg; b.hidden = false; }
function hideError() { $("error-bar").hidden = true; }

// loadAppValues 用当前时间范围拉取 app 标签的值列表,填充应用下拉框(保留首个"不限")。
async function loadAppValues() {
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  try {
    const j = await api("/api/label/app/values?start=" + start + "&end=" + end);
    const sel = $("app-select");
    const blank = sel.querySelector('option[value=""]');
    sel.replaceChildren(blank);
    (j.data || []).forEach((v) => {
      const o = document.createElement("option");
      o.value = v; o.textContent = v;
      sel.appendChild(o);
    });
  } catch (e) { showError(e.message); }
}

// saveFilters 把当前 env、level 写入 localStorage;失败静默降级(隐私模式等)。
function saveFilters() {
  try {
    const filters = { env: $("env-select").value, level: $("level-select").value };
    localStorage.setItem(FILTERS_KEY, JSON.stringify(filters));
  } catch (_) { /* localStorage 不可用时忽略,不阻塞查询 */ }
}

// loadFilters 从 localStorage 恢复 env、level;无记录或解析失败用默认值(env 不限、level=info)。
function loadFilters() {
  let env = "", level = "info";
  try {
    const raw = localStorage.getItem(FILTERS_KEY);
    if (raw) {
      const f = JSON.parse(raw);
      if (f && typeof f.env === "string") env = f.env;
      if (f && typeof f.level === "string") level = f.level;
    }
  } catch (_) { /* 解析失败用默认值 */ }
  if (env) $("env-select").value = env;
  $("level-select").value = level;
}

$("env-select").addEventListener("change", saveFilters);
$("level-select").addEventListener("change", saveFilters);

// setRange 把起止时间设为 [end - mins, end],供快捷按钮和页面默认值共用。
function setRange(mins) {
  const end = unixNow();
  $("start").value = toLocalInput(end - mins * 60);
  $("end").value = toLocalInput(end);
}

document.querySelectorAll(".range-group button").forEach((b) => {
  b.addEventListener("click", () => setRange(parseInt(b.dataset.mins, 10)));
});

$("search").addEventListener("click", search);

async function search() {
  hideError();
  const env = $("env-select").value;
  const app = $("app-select").value;
  const level = $("level-select").value;
  const chips = [];
  if (env) chips.push({ label: "env", value: env });
  if (app) chips.push({ label: "app", value: app });
  const keyword = $("keyword").value.trim();
  const traceId = $("trace-id").value.trim();
  const query = buildLogQL(chips, keyword, traceId, level);
  if (!query) { showError("请至少选择环境或应用"); return; }
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  if (!start || !end) { showError("请选择时间范围(可点快捷按钮)"); return; }
  const limit = $("limit").value || 1000;
  const direction = $("direction").value;
  const url = "/api/query_range?query=" + encodeURIComponent(query) +
    "&start=" + start + "&end=" + end + "&limit=" + limit + "&direction=" + direction;
  try {
    const j = await api(url);
    renderResults(j);
    renderChart(j.lines || []);
  } catch (e) {
    showError(e.message);
  }
}

function renderResults(res) {
  const box = $("results");
  const lines = res.lines || [];
  box.replaceChildren();
  const meta = document.createElement("div");
  meta.className = "meta";
  meta.textContent = "返回 " + lines.length + " 行(总计 " + res.total + ")";
  box.appendChild(meta);
  lines.forEach((l) => {
    const row = document.createElement("div");
    row.className = "line lvl-" + (l.level || "info");
    const ts = document.createElement("span");
    ts.className = "ts";
    ts.textContent = l.ts;
    const txt = document.createElement("span");
    txt.className = "txt";
    txt.textContent = l.line;
    row.appendChild(ts);
    row.appendChild(txt);
    box.appendChild(row);
  });
}

function renderChart(lines) {
  const levels = ["error", "warn", "info", "debug"];
  const colors = { error: "#ff6b6b", warn: "#ffd166", info: "#69c0ff", debug: "#888" };
  const buckets = {};
  lines.forEach((l) => {
    const d = new Date(l.ts);
    d.setSeconds(0, 0);
    const key = d.getTime();
    const lvl = levels.includes(l.level) ? l.level : "info";
    buckets[key] = buckets[key] || { error: 0, warn: 0, info: 0, debug: 0 };
    buckets[key][lvl]++;
  });
  const keys = Object.keys(buckets).map(Number).sort((a, b) => a - b);
  const labels = keys.map((k) => new Date(k).toLocaleTimeString());
  const datasets = levels.map((lvl) => ({
    label: lvl,
    stack: "s",
    backgroundColor: colors[lvl],
    data: keys.map((k) => buckets[k][lvl]),
  }));
  if (chart) chart.destroy();
  chart = new Chart($("chart"), {
    type: "bar",
    data: { labels, datasets },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: { legend: { labels: { color: "#ddd" } } },
      scales: {
        x: { stacked: true, ticks: { color: "#999" } },
        y: { stacked: true, beginAtZero: true, ticks: { color: "#999" } },
      },
    },
  });
}

setRange(60);       // 默认最近 1 小时
loadAppValues();    // 填充应用下拉(需先有时间范围)
loadFilters();      // 恢复 env、level
```

- [ ] **Step 3: 删除 `web/static/style.css` 中已失效的 chip 样式**

精确删除以下 5 行(原文件第 25–29 行):

```css
.chips { display: flex; flex-wrap: wrap; gap: 6px; min-height: 26px; align-items: center; }
.chips:empty::before { content: "未添加标签条件"; color: #777; font-size: 12px; }
.chip { background: #2c3a55; color: #cfe0ff; border: 1px solid #3a7bd5; border-radius: 12px; padding: 2px 4px 2px 10px; font-size: 12px; display: inline-flex; align-items: center; gap: 6px; font-family: ui-monospace, "SF Mono", Menlo, monospace; }
.chip-x { cursor: pointer; color: #9ab; padding: 0 5px; border-radius: 8px; line-height: 1; }
.chip-x:hover { color: #fff; background: #5a1d1d; }
```

(下拉框沿用现有全局 `select` 样式,无需新增规则。`.logql` 死样式属先前改动遗留,不在本次范围,保留不动。)

- [ ] **Step 4: 自动化校验**

Run: `node --check web/static/app.js && node --test web/logql.test.js && go test ./... && make vet`
Expected: 全部通过,无报错。`go test` 应全绿(后端未动,回归确认)。

- [ ] **Step 5: 手动校验(若 config.yaml 指向可达 Loki)**

Run: `make build && ./bin/log-search`(或 `make run`),浏览器打开 `http://localhost:8080`。
Expected:
- 查询区显示 环境/级别/应用 三个下拉框 + 查询按钮;无原"标签/标签值/添加"控件,无 chip 面板。
- 级别默认选中 `info`;应用下拉含"不限" + 实际应用列表(Loki 不可达时显示错误条,应用仅"不限",属预期)。
- 选 环境=生产、级别=warn、应用=某应用 → 点查询 →(用浏览器 DevTools Network 看 `/api/query_range` 的 `query` 参数)应形如 `{env="prod", app="..."} |~ "(?i)\b(WARN|WARNING|ERROR|ERR|FATAL|PANIC)\b"`。
- 环境/级别都"不限" → 点查询 → 出现"请至少选择环境或应用"。
- 改 环境/级别 后刷新页面 → 两个下拉框恢复上次选择(localStorage 生效);应用下拉重新加载。

- [ ] **Step 6: Commit**

```bash
git add web/templates/index.html web/static/app.js web/static/style.css
git commit -m "$(cat <<'EOF'
feat(web): dropdown-driven query filters with localStorage

Replace the generic label/value chip builder with three dropdowns
(env / level / app). env and app become stream-selector matchers; level
applies the threshold regex from buildLogQL. env+level persist to
localStorage and restore on load; app list loads from /api/label/app/values.

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```
