# Log Search

基于 Gin + Loki 的日志查询工程(前后端不分离)。Gin 渲染 HTML 模板并提供页面调用的 JSON API,前端是纯 JS(`web/static/app.js`)+ 本地 vendored Chart.js,无前端构建步骤。

## 快速开始

1. 复制配置并填入你的 Loki 地址:
   `cp config.example.yaml config.yaml`
2. 运行:`make run`(或 `go run ./cmd/server`)
3. 浏览器打开 http://localhost:8080

## 功能

- **查询条件**:环境(来自配置)、级别(DEBUG/INFO/WARN/ERROR,多选)、方向(倒序/正序)选项卡;应用下拉、链路 id、关键词文本框——后三者各带一键清除图标。
- **配置化选项**:环境选项和应用下拉从 `config.yaml` 的 `ui.envs` / `ui.apps` 读取。
- **后端生成 LogQL**:浏览器向 `/api/query_range` 发送 JSON 筛选条件;后端负责拼接 LogQL、设置 limit、处理游标分页。
- **链路筛选**:`tid` 按 Loki structured metadata 查询,LogQL 形如 `| tid="..."`,不是 stream label selector。
- **标签展示**:结果行只展示 `env`、`app`、`level`、`tid` 四类 tags;后续扩展从后端 allowlist 增加。
- **时间范围**:开始/结束时间,默认从当前时间往前 1 小时整点到当天 `23:59`;支持 `1h`、`3h`、`今天` 快捷设置;时间框不允许清空(清空会自动回填默认时间)。
- **首次打开默认**:环境=第一个配置项(未配置时为“演示”)、级别全选、方向=倒序。已选条件记在浏览器(localStorage),老用户下次打开沿用上次选择。
- **结果展示**:按级别着色的日志行;可点击复制的标签(`app`/`level` 用固定颜色,其余标签按出现位置依次取色,兼容不同系统不同的标签 key);每分钟 × 级别堆叠柱状图;长日志(>10 行)可折叠。
- **部署收紧**:静态资源全部嵌入二进制;HTTP server 配置了基础超时,`/api/query_range` 请求体限制为 16KB。

## 测试数据

可用仓库内脚本向 Loki 写入本地测试日志:

```bash
python3 scripts/seed_loki.py --url http://localhost:3100
python3 scripts/seed_loki.py --count 2 --dry-run
```

脚本默认写入 `env=test`、`app=service-x/service-y`、`level=debug/info/warn/error`,并把 `tid` 放在 Loki push value 的第三段 structured metadata 中。

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
