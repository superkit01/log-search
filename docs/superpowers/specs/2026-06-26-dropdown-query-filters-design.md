# 下拉框驱动的查询条件改造(Dropdown-driven query filters)

- 日期:2026-06-26
- 状态:设计
- 范围:**纯前端改动**(`web/templates/index.html`、`web/static/app.js`、`web/static/logql.js`、`web/static/style.css`、`web/logql.test.js`)。后端 Go 代码、响应结构、错误映射契约均不变。

## 1. 背景与动机

当前查询区用通用「标签 + 标签值 + 添加」构建器生成 chip:用户需"选标签 → 选值 → 点添加"三步才能加一个条件,且每加一个标签都要重复一遍。对高频的"按环境 / 级别 / 应用过滤"场景不够便捷。

本次把查询条件改为三个**专用下拉框**,选择即条件,并把**环境**和**级别**两个选择持久化到 `localStorage`,打开页面默认恢复,减少重复操作。

## 2. 目标 / 非目标

**目标**

- 查询区改为 **环境 / 级别 / 应用** 三个下拉框(单选)。
- 环境 → `env` 标签匹配器;应用 → `app` 标签匹配器;级别 → LogQL 正则行过滤(阈值语义)。
- 环境、级别选择持久化到 `localStorage`,刷新后恢复。
- 后端零改动。

**非目标(YAGNI)**

- 不做下拉框多选(每框单选)。
- 不做"应用列表随环境联动":`/api/label/:name/values` 只接受标签名 + 时间范围,无法按其他标签过滤,要做需 series 查询,超出本次范围。
- 不持久化应用、时间范围、关键词、方向、条数(仅 env + level,按用户明确要求)。
- 不改后端、不改响应结构、不改错误映射契约。

## 3. 决策记录

| 决策 | 选择 | 理由 |
|---|---|---|
| 替换 vs 并存通用 chip 构建器 | **替换** | 用户选择;界面最简洁,符合"更方便"目标 |
| env 取值 | **dev / test / prod** | 用户确认(开发=dev,测试=test,生产=prod) |
| 应用列表来源 | **`app` 标签** | 与本仓库 spec 示例 `app="nginx"` 一致 |
| 级别过滤机制 | **LogQL 正则 `|~`(方案 A)** | 在 Loki 侧过滤,高效、尊重 `limit`;非前端过滤 |
| debug 阈值行为 | **不加过滤(显示全部)** | 用户意图"debug 及以上 = 全部";不过滤才能显示无法识别级别的行 |
| 首次访问默认级别 | **info** | 最常见场景,过滤掉 debug 噪声 |

**级别过滤方案对比(为何选 A)**

- **A. LogQL 正则行过滤 `|~`(选用)**:按阈值取"该级别及以上"的关键词并集做正则。Loki 侧过滤,高效,不破坏 `limit` 语义;文本日志 `[ERROR]` 与结构化 `"level":"error"` 都能命中(原始行含该词)。代价:偶发误命中(如某 info 行里出现 "error" 一词,在 warn+ 过滤时会被保留)——属"多显示"方向,可接受。
- **B. 前端拉取后按 `DetectLevel` 过滤(不选)**:复用现有准确的级别解析,但破坏 `limit` 语义(取 1000 可能只显示 200)、浪费带宽、图表只反映过滤后样本。

## 4. 用户界面

查询区**替换**原 `label-select / value-select / add-chip` 行(该整行移除),改为:

| 控件 | id | 选项 |
|---|---|---|
| 环境 | `env-select` | 不限(`""`) / 开发(`dev`) / 测试(`test`) / 生产(`prod`) |
| 级别 | `level-select` | `debug` / `info` / `warn` / `error`(无"不限";`debug` 即"显示全部") |
| 应用 | `app-select` | 不限(`""`) + 从 `/api/label/app/values` 动态加载的值 |
| 查询 | `search`(保留) | 触发查询 |

- **移除**:`#label-select`、`#value-select`、`#add-chip`、`#chips` 容器及其整行。
- **保留**:关键词、trace_id、时间快捷按钮、起止、方向、条数。
- 选中状态由下拉框自身展示,不再需要 chip 面板。

## 5. LogQL 拼装规则(`logql.js`)

`buildLogQL(chips, keyword, traceId, level)` —— 新增第 4 个参数 `level`(向后兼容:缺省 `undefined` → 不追加级别过滤,现有 3 参测试仍通过)。

- **chips**:由 env/app 下拉框生成。env 选中 → `{label:"env", value:<env>}`;app 选中 → `{label:"app", value:<app>}`。复用现有"按 label 去重、后值覆盖、空值跳过"逻辑。
- **流选择器 `{}`**:由 chips 组成。**至少 1 个 chip**(即 env/app 至少选一个),否则返回 `null`,由调用方报错"请至少选择环境或应用"。`level` 单独不能构成查询。
- **关键词**:非空 → 追加 `|= "kw"`(值经 `escapeLogQLValue` 转义)。
- **级别(新增)**:`levelRegex(level)` 返回正则字符串,非空则追加 `|~ "<regex>"`。该正则是**固定可信串**(关键词来自常量,无用户输入),**不经 `escapeLogQLValue`**;仅用 `"` 包裹。
- **trace_id**:非空 → 追加 `| trace_id="..."`(值转义)。
- **管道顺序**:`{selector} |= "kw" |~ "level" | trace_id="..."`。

**级别阈值 → 正则**

镜像 `logparse` 识别的 token,大小写不敏感(`(?i)`)、词边界(`\b`)。严重度升序:`debug < info < warn < error`。阈值 T 的关键词集合 = T 及以上各档 token 的并集:

| 阈值 | 追加的正则 | 说明 |
|---|---|---|
| `debug` 或空 | *(不追加)* | 显示全部,含无法识别级别的行 |
| `info` | `\b(INFO\|INFORMATION\|WARN\|WARNING\|ERROR\|ERR\|FATAL\|PANIC)\b` | info 及以上 |
| `warn` | `\b(WARN\|WARNING\|ERROR\|ERR\|FATAL\|PANIC)\b` | warn 及以上 |
| `error` | `\b(ERROR\|ERR\|FATAL\|PANIC)\b` | 仅 error 及以上 |

实际输出形如:`|~ "(?i)\b(WARN|WARNING|ERROR|ERR|FATAL|PANIC)\b"`。

实现用一张"档位 → token 列表"常量表 + 严重度顺序数组,按阈值切片取并集拼接,避免手写出错。`levelRegex` 导出以便单测。

**完整示例**

- `env=prod, app=nginx, level=warn, kw=timeout, trace_id=t1` →
  `{env="prod", app="nginx"} |= "timeout" |~ "(?i)\b(WARN|WARNING|ERROR|ERR|FATAL|PANIC)\b" | trace_id="t1"`
- `env=prod`(其余空,level 默认)→ `{env="prod"}`
- `env` 与 `app` 都"不限" → 返回 `null` → 前端报错"请至少选择环境或应用"。

## 6. localStorage(`app.js`)

- 键:`log-search:filters`;值:`{"env":"prod","level":"info"}`(JSON)。
- **写**:`env-select` / `level-select` 的 `change` 事件触发 `saveFilters()`,只存 `env`、`level`。空值也存(`""` 表示环境不限;`debug` 表示级别全显)。
- **读**:页面初始化时 `loadFilters()` 恢复 `env-select`、`level-select`。解析失败或无记录 → `env=""`(不限),`level="info"`(默认)。
- `app-select` **不持久化**:每次从接口加载。
- **容错**:读写均 `try/catch`;`localStorage` 不可用(隐私模式等)时降级为不持久化,不抛错、不阻塞查询。

## 7. 应用列表加载(`app.js`)

- `loadAppValues()`:用当前 `start/end` 调 `/api/label/app/values?start=&end=`,把返回的 `data` 填充进 `app-select`(保留首个"不限" option,清空其余)。
- **初始化顺序**:`setRange(60)` → `loadAppValues()` → `loadFilters()`(恢复 env/level)。必须先设时间范围,因为标签值接口要求 `start/end`。
- 失败:`showError`;`app-select` 保持只有"不限"。

## 8. `search()` 改造(`app.js`)

- 读 `env-select` / `app-select` / `level-select`;构造 `chips`(env/app 非空才 push)。
- `keyword` / `trace_id` / 范围 / `limit` / `direction` 同现有。
- `query = buildLogQL(chips, keyword, traceId, level)`。
- `query` 为 `null` → `showError("请至少选择环境或应用")`,不发请求。
- 其余(请求、渲染结果、渲染图表)不变。

## 9. 改动文件

- `web/templates/index.html`:查询区首行替换为 env/level/app 下拉框 + 查询按钮;移除 chips 行。
- `web/static/app.js`:下拉框事件、`saveFilters`/`loadFilters`、`loadAppValues`、`search` 改造;移除 `label-select`/`value-select`/`add-chip`/`renderChips`/`chips` 相关代码。
- `web/static/logql.js`:`buildLogQL` 增加 `level` 参数;新增 `levelRegex`;导出 `levelRegex`。
- `web/static/style.css`:移除未用的 `.chip` / `.chip-x` 样式;下拉框排版微调(沿用现有 `label select` 风格)。
- `web/logql.test.js`:新增 `level` 阈值 → 正则用例;现有 3 参用例保持通过。

后端 `internal/*` 零改动。

## 10. 测试

- `web/logql.test.js`(`node --test web/logql.test.js`):
  - `levelRegex` / `buildLogQL` 第 4 参:`debug` 与空 → 不追加;`info` / `warn` / `error` → 正确的并集,且三者互不相同。
  - 完整组合:`env + app + level + kw + trace_id` 拼出预期串。
  - 向后兼容:现有 3 参调用结果不变。
- `node --check web/static/app.js`:语法校验。
- `go test ./...`:后端未动,应全绿(回归确认)。
- 手测:浏览器选 env + level → 查询 → 检查实际发出的 `query` 串;刷新页面 → env / level 恢复。

## 11. 风险 / 取舍

- 级别正则偶发误命中(见 §3)——可接受的"多显示"方向。
- 应用列表不随环境联动(接口限制)——明确非目标。
- 应用列表仅在页面初始化时按默认 1h 窗口加载一次;若用户随后把时间范围扩到 24h,只在该窗口外有日志的应用不会出现在下拉框。已知限制,本次不处理(可在后续按"时间范围变化时重载 app 列表"迭代)。
- `localStorage` 被禁用 → 降级,不持久化,不报错(见 §6)。
