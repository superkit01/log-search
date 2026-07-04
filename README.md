# Log Search

基于 Gin 的日志查询工程(前后端不分离),可通过配置选择 Loki 或 Elasticsearch 作为后端存储。Gin 渲染 HTML 模板并提供页面调用的 JSON API,前端是纯 JS(`web/static/app.js`)+ 本地 vendored Chart.js,无前端构建步骤。

## 快速开始

1. 复制配置并填入你的存储地址:
   `cp config.example.yaml config.yaml`
   - Loki: `storage.type: loki`,配置 `storage.loki.url`
   - Elasticsearch: `storage.type: es`,配置 `storage.elasticsearch.url` 和 `storage.elasticsearch.index`
2. 运行:`make run`(或 `go run ./cmd/server`)
3. 浏览器打开 http://localhost:8080

## 功能

- **可切换存储后端**:`storage.type` 支持 `loki` 或 `es`;Web 层统一接收页面 JSON 筛选条件,具体后端客户端自行组织 Loki LogQL 或 ES DSL 查询。
- **查询条件**:环境(来自配置)、级别(DEBUG/INFO/WARN/ERROR,多选)、方向(倒序/正序)选项卡;应用下拉、链路 id、关键词文本框——后三者各带一键清除图标。
- **配置化选项**:环境选项和应用下拉从 `config.yaml` 的 `ui.envs` / `ui.apps` 读取。
- **后端生成查询**:浏览器向 `/api/query_range` 发送 JSON 筛选条件;Web 层负责校验、设置 limit、处理游标分页。Loki 后端根据筛选条件构造 LogQL;ES 后端根据同一组 JSON 字段直接构造 ES DSL。
- **链路筛选**:`tid` 在 Loki 中按 structured metadata 查询,LogQL 形如 `| tid="..."`,不是 stream label selector;ES 后端按文档字段 `tid.keyword` 查询。
- **关键词查询**:关键词默认匹配日志正文;支持 `a && b` 表示同时包含两个条件,支持 `a || b` 表示包含任意一个条件。Loki 后端会生成多个 line filter 或正则 line filter;ES 后端会对 `message` 字段生成 `match_phrase` 查询。
- **标签展示**:结果行只展示 `env`、`app`、`level`、`tid` 四类 tags;后续扩展从后端 allowlist 增加。
- **时间范围**:开始/结束时间,默认今天 `00:00` 到今天 `23:59`;支持 `1h`、`3h`、`今天` 快捷设置;时间框不允许清空(清空会自动回填默认时间)。
- **首次打开默认**:环境=第一个配置项(未配置时为“演示”)、级别全选、方向=倒序。已选条件记在浏览器(localStorage),老用户下次打开沿用上次选择。
- **结果展示**:按级别着色的日志行;可点击复制的标签(`app`/`level` 用固定颜色,其余标签按出现位置依次取色,兼容不同系统不同的标签 key);每分钟 × 级别堆叠柱状图;长日志(>10 行)可折叠。
- **部署收紧**:静态资源全部嵌入二进制;HTTP server 配置了基础超时,`/api/query_range` 请求体限制为 16KB。

## 数据格式要求

### Loki

Loki 数据需要把稳定查询维度放在 stream labels,把链路 id 放在 structured metadata:

- stream labels: `env`、`app`、`level`
- structured metadata: `tid`
- log line: Loki value 的日志正文

查询映射:

- `env` / `app` / `level` 会进入 LogQL selector,例如 `{env="test", app="api-server"}`
- `tid` 会进入 LogQL pipeline filter,例如 `| tid="tid-0001"`
- 返回给页面的 tags 只保留 `env`、`app`、`level`、`tid`

### Elasticsearch

ES 每条日志文档需要使用顶层字段承载同一组信息:

```json
{
  "@timestamp": "2026-07-04T21:27:00+08:00",
  "message": "log text here",
  "env": "test",
  "app": "api-server",
  "level": "info",
  "tid": "tid-0001"
}
```

字段要求:

- `@timestamp`: 必填,字符串或 date 字段,需要能按 RFC3339/RFC3339Nano 表达时间,例如 `2026-07-04T21:27:00+08:00` 或 `2026-07-04T13:27:00Z`
- `message`: 必填,日志正文,页面展示为 log line
- `env`: 必填,等价 Loki label `env`
- `app`: 必填,等价 Loki label `app`
- `level`: 必填,建议小写 `debug`、`info`、`warn`、`error`
- `tid`: 可选,等价 Loki structured metadata `tid`;只有存在该字段的文档才能按链路 id 查询

当前 ES 查询实现会使用 `env.keyword`、`app.keyword`、`tid.keyword` 做精确匹配,使用 `level.keyword` 做级别集合匹配,并使用 `message` 的 `match_phrase` 做日志内容关键字匹配。因此标签字段应是 `keyword` 类型,或保留 ES 默认动态映射生成的 `.keyword` 子字段。推荐 mapping:

```json
{
  "mappings": {
    "properties": {
      "@timestamp": { "type": "date" },
      "message": { "type": "text" },
      "env": { "type": "keyword" },
      "app": { "type": "keyword" },
      "level": { "type": "keyword" },
      "tid": { "type": "keyword" }
    }
  }
}
```

页面配置里的 `ui.envs` / `ui.apps` 必须和 Loki labels 或 ES 文档中的实际 `env` / `app` 值一致;否则选择对应筛选项时会正常返回空结果。

## 测试数据

可用仓库内脚本写入测试日志。

### Loki

```bash
make seed-loki
python3 scripts/seed_loki.py --url http://localhost:3100
python3 scripts/seed_loki.py --count 2 --dry-run
```

脚本默认写入 `env=test`、`app=service-x/service-y`、`level=debug/info/warn/error`,并把 `tid` 放在 Loki push value 的第三段 structured metadata 中。

### Elasticsearch

```bash
make seed-es
ES_URL=http://localhost:9200 make seed-es
./scripts/seed_es.py --url http://localhost:9200 --index logs-{date}
./scripts/seed_es.py --count 2 --dry-run
```

ES 脚本默认写入 `env=test`、`app=api-server/worker/gateway/auth-service`、`level=debug/info/warn/error`,并在约 10% 的日志中写入 `tid` 字段。页面的 `ui.apps` 需要和 ES 文档里的 `app` 值一致,否则选择应用后会查不到数据。更多说明见 `docs/generate-es-data.md`。

## 开发

- 测试:`make test`
- 构建:`make build`
- 前端纯逻辑测试:`node --test web/*.test.js`
- 测试数据脚本测试:`python3 -B -m unittest discover -s scripts -p '*_test.py'`

## 交叉编译(部署到 Linux)

在 macOS 上直接 `go build` 会按本机架构产出(Apple Silicon 是 `linux/arm64`),拷到 `x86_64` 服务器会报 `cannot execute binary file: Exec format error`。部署到 Linux 需显式指定 `GOOS`/`GOARCH`:

```bash
# x86_64 Linux(大多数服务器)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ./bin/log-search ./cmd/server
# arm64 Linux
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o ./bin/log-search ./cmd/server
```

服务器架构用 `uname -m` 确认:`x86_64` → `amd64`;`aarch64` → `arm64`。

仓库自带的 `Dockerfile`(多阶段构建)也可直接用于部署,无需手动指定目标架构。
