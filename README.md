# Log Search

单进程 Go / Gin 日志查询工具，支持 Loki 和 Elasticsearch。HTML、纯 JavaScript、Chart.js 与 LS 图标随二进制嵌入，没有前端构建步骤。

## 快速开始

1. 将 `config.example.yaml` 复制为 `config.yaml`。
2. 设置 `storage.type`（`loki` 或 `es`）、存储地址以及索引/字段映射。
3. 配置 `ui.namespaces` 中的平台、环境和应用，值须与实际日志一致。
4. 执行 `go run ./cmd/server --config config.yaml`，访问配置的地址（默认 `http://localhost:8080`）。

`config.yaml` 已被 Git 忽略。环境变量支持 `${VAR}` 和 `${VAR:-default}`。实际部署地址、可用环境和凭据以部署配置为准。

## 当前能力

- 多平台、环境、应用、级别、时间、关键词及 TraceId / TaskId / MessageId / SpanId / ParentSpanId 筛选。
- 关键词 `a && b` 为 AND，`a || b` 为 OR；混合两种运算符时按完整字面文本匹配。
- ES 使用 PIT + `search_after`，按时间、sequence、`_shard_doc` 同向排序；Loki 使用时间戳游标。
- 链路下钻保留当前平台、环境和时间，清空应用、级别等额外条件。
- 分享链接恢复查询条件；四行查询布局，Span 筛选默认折叠；齿轮设置内调整字号和主题。
- 图表只统计已加载日志，零值柱不展示；已加载行数和接口 `total` 均不是全量命中数。
- MCP `/mcp` 支持外部 Agent 只读查询日志，使用独立 Bearer API Key 认证。

## 工程目录

```text
cmd/server/                  应用入口与依赖装配
internal/
  config/                    配置加载与校验
  query/                     共享查询参数、校验和分页服务
  storage/                   存储接口、数据类型与关键词语法
    elasticsearch/           ES 查询与 PIT 分页实现
    loki/                    Loki 查询实现
  web/                       Gin 路由、HTTP handler 与资源挂载
  agenttools/                Agent 工具定义与查询结果适配
  mcp/                       MCP 协议与 Bearer 认证
web/
  templates/                 HTML 模板
  static/js/                 业务 JavaScript
  static/css/                页面样式
  static/img/                Logo 与 favicon
  static/vendor/             第三方前端资源
  tests/                     Node.js 前端测试
  assets.go                  嵌入模板和静态资源
docs/                        分类文档及导航
bin/                         本地构建产物，Git 忽略
config.example.yaml          可提交的配置示例
config.yaml                  本地配置，Git 忽略
```

Go 测试与被测代码放在同一包；前端测试集中在 `web/tests/`。具体存储实现放在 `internal/storage/` 子包，HTTP 和 MCP 通过共享查询服务调用。模板及静态资源继续嵌入二进制，无前端构建步骤。

## 文档导航

| 文档 | 内容 |
|---|---|
| [用户手册](docs/guides/用户手册.md) | 查询、分享、设置 |
| [MCP 接入](docs/integrations/MCP接入.md) | Agent 工具、Bearer Key 配置、分页与调用示例 |
| [查询接口](docs/api/查询接口.md) | 请求、响应、错误、分页与错误说明 |
| [开发交接文档](docs/architecture/开发交接文档.md) | 存储输入字段、字段来源、ES/Loki 映射和查询约定 |

## 存储配置

示例 ES 索引为 `app-log-{namespace}-{env}-{yyyyMMdd}`，日期时区为 `Asia/Shanghai`。支持 `{yyyyMMdd}`、`{yyyy.MM.dd}`、`{yyyy.MM}`、`{yyyy}`，跨日期展开覆盖的索引；通配模式直接交给 ES。

`storage.elasticsearch.fields` 控制精确查询字段，示例为 `env`、`app.keyword`、`level`、`traceId`。省略配置时兼容旧格式 `env.keyword`、`app.keyword`、`level.keyword`、`tid.keyword`。根据实际 mapping 选择字段：纯 keyword 使用原字段，text 存在 keyword 子字段时使用子字段。TaskId、MessageId 同时兼容原字段与 `.keyword` 子字段。

Loki 使用 `env`、`app`、`level` 等 stream labels，`tid` 等关联字段通过 structured metadata 的 pipeline filter 查询。具体 LogQL / DSL 由存储实现生成。

## 开发与部署

```bash
go test ./...
go vet ./...
node --test web/tests/*.test.js
go build -o bin/log-search ./cmd/server
```

模块保持 `go 1.24.5`、Gin `v1.10.0`。部署只需二进制与配置；修改静态资源或模板后须重新构建。

Linux amd64 交叉编译（PowerShell）：

```powershell
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o bin/log-search ./cmd/server
```

Linux/macOS shell：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/log-search ./cmd/server
```

目标架构以服务器 `uname -m` 为准，`aarch64` 对应 arm64。未指定时按本机 OS/架构构建。也可使用仓库 Dockerfile。
