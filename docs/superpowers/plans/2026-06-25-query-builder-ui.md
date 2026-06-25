# 查询 UI 三项改造 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把查询页面从"手填 LogQL 文本框"改造成"由选择项实时生成 LogQL",并增加 trace_id 结构化元数据筛选、方向文案改为倒序/正序。

**Architecture:** 纯前端改动。把 LogQL 拼装的纯逻辑抽到独立模块 `web/static/logql.js`(无 DOM 依赖,可在 Node 下单测);`app.js` 负责 DOM 接线(chips 状态、事件、调用 `buildLogQL`);模板与样式同步调整。后端(`internal/...`)一行不改。

**Tech Stack:** 原生 JS(无构建步骤)、Gin 模板、Chart.js(CDN)、Node 内置 `node:test`(仅用于纯逻辑单测,零依赖)。

## Global Constraints

- 后端 Go 代码、`/api/*` 响应结构、`lokiStatusAndBody` 的 4xx 透传 / 502 兜底契约、`QueryRange` 全局时间排序——**全部不得改动**。`make test` 必须持续全绿。
- `go` 指令锁定 `1.24` 线(go 1.24.5),gin v1.10.0;本次不碰 go.mod。
- 前端保持"无构建步骤、纯 JS";不引入打包工具或 npm 依赖。单测只用 Node 内置 `node:test`(本机 Node v22.11.0,已确认)。
- 日志行仍用 `textContent` 渲染,禁止 `innerHTML` 注入日志内容(防 XSS)。
- 方向 `option value` 保持 `FORWARD`/`BACKWARD` 不变,只改显示文案。
- trace_id 在 LogQL 中用结构化元数据的标签过滤表达式 `| trace_id="值"`。
- LogQL 流选择器不允许空(`{}` 报错)、不允许同一标签重复(`{a="x",a="y"}` 报错);`buildLogQL` 必须保证:无 chip 返回 null、重复标签后值覆盖前者。

参考 spec:[docs/superpowers/specs/2026-06-25-query-builder-ui-design.md](../specs/2026-06-25-query-builder-ui-design.md)

---

## 文件结构

| 文件 | 动作 | 职责 |
|---|---|---|
| `web/static/logql.js` | 新建 | 纯函数 `escapeLogQLValue`、`buildLogQL`;无 DOM;浏览器全局 + Node `module.exports` |
| `web/logql.test.js` | 新建 | Node `node:test` 单测,require `./static/logql.js`(放在 `web/` 不被 `/static` 暴露) |
| `web/templates/index.html` | 修改 | 方向文案、trace_id 输入、chips 容器、删除 LogQL 框、按钮调整、引入 logql.js |
| `web/static/style.css` | 修改 | `.chips` / `.chip` / `.chip-x` 样式 |
| `web/static/app.js` | 修改 | chips 状态与渲染、add-chip/search 接线、调用 `buildLogQL`;移除旧 `#query`/`add-label`/keyword 写回逻辑 |

---

## Task 1: 纯 LogQL 构造模块 + 单测

把"由选择项生成 LogQL"的最关键、最易错的逻辑(转义、组合、标签唯一、空处理)抽成纯函数并先行单测。这是后续接线的依赖。

**Files:**
- Create: `web/static/logql.js`
- Create: `web/logql.test.js`

**Interfaces:**
- Produces: `escapeLogQLValue(v) -> string`;`buildLogQL(chips, keyword, traceId) -> string|null`。chips 为 `{label, value}` 数组;无有效 chip 时返回 `null`。

- [ ] **Step 1: 写失败的单测**

创建 `web/logql.test.js`:

```js
const test = require("node:test");
const assert = require("node:assert");
const { escapeLogQLValue, buildLogQL } = require("./static/logql.js");

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
```

- [ ] **Step 2: 运行测试,确认失败**

Run: `node --test web/logql.test.js`
Expected: FAIL,报 `Cannot find module './static/logql.js'`(文件还没建)。

- [ ] **Step 3: 实现 `web/static/logql.js`**

```js
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
```

- [ ] **Step 4: 运行测试,确认通过**

Run: `node --test web/logql.test.js`
Expected: PASS,全部用例通过(13 个)。

- [ ] **Step 5: 确认 Go 侧未受影响**

Run: `make test`
Expected: PASS(本任务没碰 Go,只是确认基线绿)。

- [ ] **Step 6: 提交**

```bash
git add web/static/logql.js web/logql.test.js
git commit -m "feat(web): extract pure LogQL builder with node tests"
```

---

## Task 2: 模板改造(index.html)

重构筛选区:方向文案、新增 trace_id 输入、chips 容器、删除 LogQL 文本框、按钮调整、引入 logql.js。本任务后页面应能加载且无 JS 报错(但 app.js 还未接线,查询功能在 Task 4 完成)。

**Files:**
- Modify: `web/templates/index.html`

**Interfaces:**
- Consumes: 无
- Produces: 给 app.js 使用的 DOM id —— `add-chip`(添加按钮)、`search`(查询按钮,沿用旧 id)、`chips`(chip 容器)、`keyword`、`trace-id`、`label-select`、`value-select`、`start`、`end`、`direction`、`limit`、`error-bar`。全局函数 `buildLogQL` 由 Task 1 的 logql.js 提供。

- [ ] **Step 1: 替换 `<section class="controls"> ... </section>` 整块**

把现有的整个 `<section class="controls">...</section>` 替换为:

```html
    <section class="controls">
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
      <div class="row">
        <label>关键词<input type="text" id="keyword" placeholder="匹配日志文本,如 error"></label>
        <label>trace_id<input type="text" id="trace-id" placeholder="结构化元数据 trace_id"></label>
      </div>
      <div class="row">
        <span class="range-group">快捷:
          <button type="button" data-mins="15">15m</button>
          <button type="button" data-mins="60">1h</button>
          <button type="button" data-mins="360">6h</button>
          <button type="button" data-mins="1440">24h</button>
        </span>
        <label>起 <input type="datetime-local" id="start"></label>
        <label>止 <input type="datetime-local" id="end"></label>
        <label>方向
          <select id="direction"><option value="BACKWARD">倒序</option><option value="FORWARD">正序</option></select>
        </label>
        <label>条数 <input type="number" id="limit" value="{{.defaultLimit}}" min="1" max="5000"></label>
      </div>
    </section>
```

要点:
- 删除了原 `#query` LogQL 文本框那一整行(及其旁的旧 `#search` 按钮)。
- `#search`(查询)按钮移到标签行;`id` 沿用 `search`,Task 4 保留其事件绑定。
- 原 `#add-label`(加入查询)按钮删除,新增 `#add-chip`(添加)。
- 方向文案 `BACKWARD→倒序`、`FORWARD→正序`,`value` 不变。
- 新增 `#chips` 容器、`#trace-id` 输入。

- [ ] **Step 2: 在底部脚本区引入 logql.js**

把:
```html
  <script src="/static/app.js"></script>
```
替换为:
```html
  <script src="/static/logql.js"></script>
  <script src="/static/app.js"></script>
```
(logql.js 必须在 app.js 之前,确保 `buildLogQL` 全局可用。)

- [ ] **Step 3: 验证模板渲染与构建**

Run: `make build`
Expected: 构建成功(模板在运行时由 Gin 解析,但 `go build` 确认没有破坏编译)。

Run: `make run`(或 `./bin/log-search`),浏览器打开 `http://localhost:8080/`。
Expected: 页面正常加载;能看到 标签/标签值/添加/查询、chips 空区、关键词、trace_id、倒序/正序。**此时点"查询"无反应或报错属正常**(app.js 接线在 Task 4)。
打开 DevTools Console:应无红色报错(若报 `buildLogQL is not defined`,检查 logql.js 是否先于 app.js 加载)。

- [ ] **Step 4: 提交**

```bash
git add web/templates/index.html
git commit -m "feat(web): query-builder template (drop LogQL box, add trace_id, chips)"
```

---

## Task 3: chips 样式(style.css)

为新增的 chips 区加样式,使其可读、可点删除。

**Files:**
- Modify: `web/static/style.css`

**Interfaces:**
- Consumes: Task 2 的 `.chips` / `.chip` / `.chip-x` class。

- [ ] **Step 1: 追加 chip 样式**

在 `web/static/style.css` 末尾追加:

```css
.chips { display: flex; flex-wrap: wrap; gap: 6px; min-height: 26px; align-items: center; }
.chips:empty::before { content: "未添加标签条件"; color: #777; font-size: 12px; }
.chip { background: #2c3a55; color: #cfe0ff; border: 1px solid #3a7bd5; border-radius: 12px; padding: 2px 4px 2px 10px; font-size: 12px; display: inline-flex; align-items: center; gap: 6px; font-family: ui-monospace, "SF Mono", Menlo, monospace; }
.chip-x { cursor: pointer; color: #9ab; padding: 0 5px; border-radius: 8px; line-height: 1; }
.chip-x:hover { color: #fff; background: #5a1d1d; }
```

- [ ] **Step 2: 验证**

刷新浏览器(Task 2 已 `make run`);chips 空区显示"未添加标签条件"灰字提示。样式正确即通过(CSS 无单测)。

- [ ] **Step 3: 提交**

```bash
git add web/static/style.css
git commit -m "style(web): chips filter styling"
```

---

## Task 4: app.js 接线(chips 状态 + 实时生成查询)

把模板新控件接起来:维护 chips 状态、添加/删除 chip、查询时用 `buildLogQL` 生成 LogQL;移除所有写 `#query` 文本框的旧逻辑。

**Files:**
- Modify: `web/static/app.js`

**Interfaces:**
- Consumes: Task 1 的全局 `buildLogQL(chips, keyword, traceId)`;Task 2 的 DOM id(`add-chip`、`chips`、`trace-id` 等)。
- Produces: 无(终端 UI 行为)。

- [ ] **Step 1: 用下面的完整内容替换 `web/static/app.js`**

```js
const $ = (id) => document.getElementById(id);
let chart;
let chips = []; // 已选标签条件:[{label, value}],label 唯一

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

async function loadLabels() {
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  try {
    const j = await api("/api/labels?start=" + start + "&end=" + end);
    const sel = $("label-select");
    (j.data || []).forEach((l) => {
      const o = document.createElement("option");
      o.value = l; o.textContent = l;
      sel.appendChild(o);
    });
  } catch (e) { showError(e.message); }
}

$("label-select").addEventListener("change", async (e) => {
  const name = e.target.value;
  if (!name) return;
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  try {
    const j = await api("/api/label/" + encodeURIComponent(name) + "/values?start=" + start + "&end=" + end);
    const sel = $("value-select");
    sel.disabled = false;
    sel.innerHTML = "";
    (j.data || []).forEach((v) => {
      const o = document.createElement("option");
      o.value = v; o.textContent = v;
      sel.appendChild(o);
    });
  } catch (err) { showError(err.message); }
});

// 添加标签条件 chip:label 唯一,重复添加则替换值。
$("add-chip").addEventListener("click", () => {
  const label = $("label-select").value;
  const value = $("value-select").value;
  if (!label || !value) { showError("请先选择标签和标签值"); return; }
  hideError();
  const i = chips.findIndex((c) => c.label === label);
  if (i >= 0) chips[i].value = value;
  else chips.push({ label, value });
  renderChips();
});

function renderChips() {
  const box = $("chips");
  box.innerHTML = "";
  chips.forEach((c, i) => {
    const chip = document.createElement("span");
    chip.className = "chip";
    chip.textContent = c.label + '="' + c.value + '"';
    const x = document.createElement("span");
    x.className = "chip-x";
    x.textContent = "×";
    x.addEventListener("click", () => { chips.splice(i, 1); renderChips(); });
    chip.appendChild(x);
    box.appendChild(chip);
  });
}

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
  const keyword = $("keyword").value.trim();
  const traceId = $("trace-id").value.trim();
  const query = buildLogQL(chips, keyword, traceId);
  if (!query) { showError("请先添加至少一个标签条件"); return; }
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
  box.innerHTML = "";
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

setRange(60); // 默认最近 1 小时
loadLabels();
```

要点:
- 新增 `chips` 状态与 `renderChips()`;`#add-chip` 校验标签+标签值后入栈(label 唯一,重复则替换)。
- `search()` 用 `buildLogQL(chips, keyword, traceId)`;返回 null → 提示"请先添加至少一个标签条件"。
- 移除了旧的 `#add-label` 写回 `{...}` 逻辑、`#keyword` 回车写回 `#query` 逻辑、`#query` 相关代码。
- 关键词、trace_id 现在都在 `search()` 里即时读取(输入框),无需 change 事件。
- `renderResults` / `renderChart` 保持不变(textContent 渲染,防 XSS)。

- [ ] **Step 2: 语法校验**

Run: `node --check web/static/app.js`
Expected: 无输出(语法正确)。

- [ ] **Step 3: 纯逻辑单测仍绿(回归确认)**

Run: `node --test web/logql.test.js`
Expected: PASS。

- [ ] **Step 4: Go 侧回归**

Run: `make test`
Expected: PASS(未改 Go)。

- [ ] **Step 5: 手动端到端验证**

`make run`,浏览器打开 `http://localhost:8080/`(强制刷新 `Cmd+Shift+R` 清缓存),逐项验证:
1. 不加任何 chip 点"查询" → 红色提示"请先添加至少一个标签条件",Network 无 `/api/query_range` 请求。
2. 选标签 `app` → 选标签值 → 点"添加" → 出现 `app="值"` chip。
3. 再选标签 `env` → 选值 → 点"添加" → 出现第二个 chip。
4. 再次选 `app` 换个值 → 点"添加" → `app` chip 的值被替换(不出现两个 `app`)。
5. 点某 chip 的 `×` → 该 chip 删除。
6. 输入关键词、trace_id → 点"查询" → Network 里 `/api/query_range?query=...` 应为 `{app="...", env="..."} |= "关键词" | trace_id="..."` 形态(空项被跳过);返回日志按时间排序显示 + 直方图。
7. 方向切换"正序/倒序" → 结果先后顺序随之改变。
8. DevTools Console 全程无红色报错。

- [ ] **Step 6: 提交**

```bash
git add web/static/app.js
git commit -m "feat(web): chip-driven query builder wired to buildLogQL"
```

---

## 收尾验证(全部任务完成后)

- [ ] Run: `node --test web/logql.test.js` → PASS
- [ ] Run: `node --check web/static/app.js` → 无输出
- [ ] Run: `make test` → PASS
- [ ] Run: `make vet` → 无告警
- [ ] Run: `make run` → 页面完整可用:chips 增删、关键词、trace_id、方向、时间范围、结果排序、直方图均正常;Console 无报错。
