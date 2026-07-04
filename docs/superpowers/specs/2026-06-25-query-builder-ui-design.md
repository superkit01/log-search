# 查询 UI 三项改造设计

日期:2026-06-25
状态:已与用户确认,待实现

> 当前实现备注(2026-07):本文记录的是早期 UI 改造设计。当前代码已经演进为浏览器提交 `/api/query_range` POST JSON,由后端构造 LogQL;链路字段使用 Loki structured metadata `tid`,过滤语法为 `| tid="..."`,不是本文中的 `trace_id`。当前契约以 `AGENTS.md` / `README.md` / `2026-06-24-log-search-design.md` 的更新版本为准。

## 背景

当前 `log-search` 的查询 UI 存在三处需要改造:

1. 查询方向的中文文案是"新→旧 / 旧→新",用户希望改成更通用的"倒序 / 正序"。
2. 没有针对 Loki 结构化元数据(structured metadata)的筛选入口。用户日志经 promtail 写入,`trace_id` 被提升为结构化元数据,需要能按 `trace_id` 过滤。
3. 页面有一个手填 LogQL 的文本框,用户要求去掉它,改为由选择项实时生成查询;同时原"加入查询"按钮要改为"查询"功能。

## 关键技术事实(已核实)

- **结构化元数据的过滤语法**:Loki 会把结构化元数据自动提取到返回结果的标签中,用**标签过滤表达式**过滤。`trace_id` 的过滤写法是 `| trace_id="值"`(与关键词过滤 `|= "kw"` 同属 LogQL 管道阶段)。来源:Loki 官方文档 "What is structured metadata"。
- **后端无需改动**:`/api/query_range` 原样转发 `query` 参数到 Loki。本次三项改造**全部是前端改动**(`web/templates/index.html`、`web/static/app.js`,以及 `web/static/style.css` 的少量样式)。Go 代码、响应结构、错误映射契约均不变。

## 改动 1:方向文案

`web/templates/index.html` 的 `#direction` `<select>` 选项文案:

- `FORWARD` → **正序**(旧→新)
- `BACKWARD` → **倒序**(新→旧)

`option value`(`FORWARD`/`BACKWARD`)保持不变,后端 `direction` 取值与全局排序逻辑不受影响。倒序(`BACKWARD`)仍是默认选中项。

## 改动 2:trace_id 输入框

在筛选区新增 `<input type="text" id="trace-id" placeholder="trace_id">`,key 固定为 `trace_id`,value 由用户输入。

- 查询拼装时若该值非空 → 在 LogQL 末尾追加 `| trace_id="值"`。
- 后端不动。

## 改动 3:去掉 LogQL 文本框,实时生成查询

这是主要改动,重构筛选区的交互模型。

### 交互模型

- **标签区**:`#label-select`(标签下拉,已有)+ `#value-select`(标签值下拉,已有)+ **新增「添加」按钮**。点「添加」时,把当前 `标签="值"` 作为一个**条件 chip** 加入已选列表。「添加」前必须同时选中标签和标签值,否则前端报错"请先选择标签和标签值",不添加。
- **chips 区**:展示所有已添加的标签条件,形如 `app="nginx" ×`、`env="local" ×`。每个 chip 的 `×` 可删除该条件。多个 chip 在 LogQL 流选择器内以 AND(逗号)组合。**标签名唯一**:LogQL 流选择器不允许同一标签出现两次(`{app="x", app="y"}` 是语法错误),因此若「添加」的标签名已存在于 chips 中,则**更新该标签的值**(替换),而不是新增重复条目。
- **关键词** `#keyword`(已有):沿用,非空时生成 `|= "kw"`。
- **trace_id** `#trace-id`(改动 2):非空时生成 `| trace_id="值"`。
- **「查询」按钮**:由原"加入查询"按钮改造而来。点击时按拼装规则生成 LogQL 并执行查询。

### 查询拼装规则

按顺序拼接,跳过空项:

```
{chip1="val1", chip2="val2", ...} |= "关键词" | trace_id="trace值"
```

- **流选择器 `{}`**:由所有 chips 组成。**至少需要 1 个 chip** —— LogQL 流选择器不允许为空;若 chips 为空,点「查询」时前端报错"请先添加至少一个标签条件",不发送请求。各 chip 标签名天然唯一(见"标签名唯一"约束),不会出现 `{a, a}`。
- **关键词过滤 `|= "关键词"`**:仅当 `#keyword` 非空时附加。
- **trace_id 过滤 `| trace_id="..."`**:仅当 `#trace-id` 非空时附加。
- **值转义**:chip 值、关键词、trace_id 中若包含 `"` 或 `\`,在拼入 LogQL 前用反斜杠转义,避免破坏 LogQL 语法。

### 移除项

- `#query` LogQL 文本输入框(模板中的整行 label + input)。
- 原有的"读 `#query.value`、正则改写 `{...}`"逻辑(`add-label`、`keyword` 写回文本框的部分)。

### 保留不变

- 时间范围输入 + 快捷按钮(`15m/1h/6h/24h`)及 `setRange` 默认值逻辑。
- `limit`、`direction`。
- 标签 / 标签值的加载与级联(`loadLabels`、`label-select` change → `value-select`),二者仍读取输入框的 start/end。
- 结果渲染 `renderResults`、直方图 `renderChart`、错误展示。

### 数据流

选择项 → 点「查询」→ JS 按"查询拼装规则"生成 LogQL → `GET /api/query_range?query=...&start&end&limit&direction` → 后端(不变)→ Loki → 后端全局时间排序(已实现)→ 返回 → 前端渲染。

## 后端契约影响

无。`internal/loki`、`internal/web`、`internal/config`、`internal/logparse` 均不改:
- `query_range` 仍直接返回 `QueryResult`(`{lines, total}`)。
- `lokiStatusAndBody` 的 4xx 透传 / 502 兜底契约不变。
- `QueryRange` 的全局时间排序(已实现)继续生效。

## 测试策略

- **后端**:无逻辑变更,现有 Go 测试不动;`make test` 应全绿。
- **前端**:项目无 JS 测试框架。验证手段:
  - `node --check web/static/app.js` 做语法校验。
  - 手动覆盖拼装用例:chips 为空 → 报错;单标签;多标签(AND);带关键词;带 trace_id;三者并存;值含 `"` 或 `\` 的转义。

## 不在本次范围(YAGNI)

- 字段名下拉(trace_id 之外的其它结构化元数据字段)。
- trace_id 值的下拉联想(用户已选择简化为纯输入)。
- chips 多条件之外的 OR / 否定语义。
- 查询历史的持久化。
