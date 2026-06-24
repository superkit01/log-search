# 日志查询工程 — 设计文档 (Spec)

- 日期:2026-06-24
- 状态:已确认,待写实现计划
- 模块路径:`log-search`

## 1. 目标

构建一个**前后端不分离**的日志查询 Web 工程:用 Go + Gin 渲染页面并处理后端逻辑,日志数据从已有的 **Loki** 实例查询。用户在浏览器里用 LogQL 查询日志,查看按级别着色的结果列表和结果直方图统计。

## 2. 范围

**包含(In Scope)**
- 单页查询界面(Gin 渲染 HTML)
- 标签 / 标签值选择、时间范围(含快捷按钮)、关键词
- 原生 LogQL 查询输入框
- 查询方向(FORWARD / BACKWARD)、返回条数限制(limit)
- 结果列表 + 按日志级别着色
- 结果直方图统计(基于返回样本,按时间桶 + 级别堆叠柱状图)
- YAML 配置文件指定 Loki 地址

**不包含(Out of Scope)**
- 登录鉴权(内网工具,直接访问)
- 实时 tail 流式跟踪(属「高级」档,后续可加)
- 聚合查询 / `count_over_time` 全量精确统计
- 日志推送 / 采集(只做查询消费方)
- Loki 本地启动方案(用户已有实例)

## 3. 技术栈

- Go 1.24 + Gin `v1.10`
- 配置:`gopkg.in/yaml.v3`
- 日志:标准库 `log/slog`
- HTTP 客户端:标准库 `net/http`(直连 Loki HTTP API)
- 前端:Gin HTML 模板 + 原生 JS + CSS;直方图用 CDN 引入 Chart.js(无构建步骤、无本地 vendor)

## 4. 关键架构决策

**方案 A:Go 直连 Loki HTTP API(已选定)**

后端用 `net/http` 直接请求 Loki 的 HTTP 接口(`/loki/api/v1/query_range`、`/loki/api/v1/labels`、`/loki/api/v1/label/{name}/values`)。

- Loki HTTP API 稳定、文档清晰,本就是为查询消费方设计
- 依赖极少,编译快、体积小
- 完全掌控超时 / 重试 / 错误透传
- 后续接 Mock 源只需替换底层 HTTP 调用

不采用官方 `loki-client-go`(面向日志推送 agent、依赖重、查询能力弱),也不采用社区薄封装库(维护活跃度不可控)。

## 5. 分层与目录结构

每个单元单一职责、可独立测试。

```
log-search/
├── cmd/server/main.go            # 入口:加载配置 → 组装依赖 → 启动 Gin
├── internal/
│   ├── config/config.go          # 配置结构 + 加载 + 校验
│   ├── loki/
│   │   ├── client.go             # Client 接口 + httpClient 实现
│   │   ├── types.go              # Loki 响应结构体
│   │   └── client_test.go        # httptest mock 测试
│   ├── logparse/
│   │   ├── level.go              # 级别解析(纯函数)
│   │   └── level_test.go         # 表驱动测试
│   └── web/
│       ├── handler.go            # 路由 + handler
│       └── handler_test.go       # 注入 mock loki.Client 测试
├── web/
│   ├── templates/{layout,index}.html
│   └── static/{app.js,style.css}
├── config.example.yaml
├── go.mod / go.sum / Makefile / README.md
└── docs/superpowers/specs/       # 本文档
```

**依赖清单**
- `github.com/gin-gonic/gin`
- `gopkg.in/yaml.v3`
- 其余标准库

## 6. 路由设计

**页面(HTML)**
- `GET /` → 渲染查询主页面 `index.html`

**JSON API(前端 JS 调用)**

| 方法 路径 | 说明 |
|---|---|
| `GET /api/labels` | 标签列表(透传 Loki `/labels`,带 start/end) |
| `GET /api/label/:name/values` | 指定标签的值列表 |
| `GET /api/query_range` | 主查询 |

## 7. `/api/query_range` 契约

**请求参数(query string)**
- `query`:LogQL 字符串(必填)
- `start` / `end`:unix 秒
- `limit`:返回条数,默认 1000,上限 5000
- `direction`:`FORWARD` / `BACKWARD`,默认 `BACKWARD`

**处理**
1. 校验参数(query 非空、start<end、limit 在上限内)
2. 调 Loki `query_range`(start/end 转**纳秒**,direction 透传)
3. 把 Loki 返回的 `streams` 拍平为日志行,对每行做级别检测
4. 返回 JSON

**响应**
```json
{
  "lines": [
    {"ts":"2026-06-24T12:00:00.123Z","level":"error","line":"...","labels":{"app":"nginx"}}
  ],
  "total": 142
}
```

## 8. 查询区 UI 布局(消除歧义)

「LogQL 查询」档的查询区由以下控件组成,主输入是 **LogQL 输入框**:

- **标签选择器(label)**:下拉选 label;选中后联动出该 label 的值,点击值即把 `key="value"` 拼进 LogQL 输入框的 `{...}` 匹配器,辅助构建 selector
- **关键词输入框**:辅助项,填入后自动在 LogQL 末尾追加 `|= "keyword"`(即关键词是 LogQL 的一部分,不是独立过滤通道)
- **原生 LogQL 输入框**:可手写完整查询,是最终提交给后端的内容(标签 / 关键词的辅助操作都是修改这个框的内容)
- **时间范围**:快捷按钮(最近 15m / 1h / 6h / 24h)+ 自定义起止;`end` 默认 now
- **方向 / 条数**:direction 下拉(FORWARD/BACKWARD)、limit 输入

即:前端最终提交的 `query` 就是 LogQL 框的内容;标签选择器和关键词框只是帮你拼 LogQL 的辅助控件。

## 9. 直方图统计(关键折中)

前端拿到带 `level` + `ts` 的 `lines` 后,按时间桶(每分钟一格)和级别分组,用 Chart.js 画堆叠柱状图。

- ✅ 级别解析只存在 Go 一处,前端只做纯数据分组(无正则)
- ⚠️ 直方图基于返回样本,受 `limit` 上限影响 —— 这是「LogQL 查询」档可接受的折中;全量精确统计需升「高级」档用 `count_over_time`,后续可加

## 10. 配置

**`config.yaml`**
```yaml
server:
  addr: ":8080"
loki:
  url: "http://localhost:3100"   # 用户的 Loki 地址
  timeout: 30s                   # HTTP 客户端超时
  default_limit: 1000            # 查询默认条数
```

- 配置路径:flag `--config`(默认 `config.yaml`)
- 启动时校验 `loki.url`,无效则立即退出(fail-fast)并打印明确原因
- 提供 `config.example.yaml` 作为模板

## 11. 错误处理

- **Loki 不可达 / 超时**:handler 返回 `502`,`{"error":"无法连接 Loki: ..."}`
- **LogQL 语法错**(Loki 返回 400):原样透传 Loki 错误信息,`{"error":"Loki: <msg>"}`
- **参数非法**(start>end、limit 越界、缺 query):返回 `400`,`{"error":"..."}`
- **配置缺失 / 无效**:启动即失败,不进入半启动状态
- 前端:所有 fetch 失败统一在结果区顶部显示错误条,不破坏已加载数据

## 12. 级别解析规则(`logparse.DetectLevel`)

按优先级匹配,命中即返回,纯函数 + 正则实现:

1. JSON:`"level":"error"` / `"levelname":"ERROR"`
2. key=value:`level=error`、`lvl=error`
3. 文本:`[ERROR]`、`ERROR`、`WARN`、`INFO`、`DEBUG`

映射:
- ERROR → 红
- WARN → 黄
- INFO → 蓝
- DEBUG → 灰
- 无匹配 → 默认(按 INFO)

## 13. 测试策略

- `loki/client_test.go`:`httptest` 起假 Loki,返回固定 `streams` JSON,断言 QueryRange / Labels / LabelValues 解析正确;并测超时、非 200 错误透传
- `logparse/level_test.go`:表驱动,覆盖 JSON / kv / 文本三格式各档级别 + 无匹配
- `web/handler_test.go`:注入 mock `loki.Client` 接口,测 `/api/query_range` 参数校验与 502/400 透传;不依赖真实 Loki
- Mock 通过 `loki.Client` 接口注入;真实网络只发生在 `httpClient` 一个结构体内

## 14. 依赖与运行

- `go mod init log-search`
- `make run`(或 `go run ./cmd/server`)→ `http://localhost:8080`
- 启动前复制 `config.example.yaml` → `config.yaml` 并填入真实 Loki 地址
