# Log Search

基于 Gin + Loki 的日志查询工程(前后端不分离)。Gin 渲染 HTML 模板并提供页面调用的 JSON API,前端是纯 JS(`web/static/app.js`)+ Chart.js(CDN),无前端构建步骤。

## 快速开始

1. 复制配置并填入你的 Loki 地址:
   `cp config.example.yaml config.yaml`
2. 运行:`make run`(或 `go run ./cmd/server`)
3. 浏览器打开 http://localhost:8080

## 功能

- **查询条件**:环境(开发/测试/生产)、级别(DEBUG/INFO/WARN/ERROR,多选)、方向(倒序/正序)选项卡;应用下拉、链路 id、关键词文本框——后三者各带一键清除图标。
- **时间范围**:开始/结束时间,默认从当前时间往前 1 小时整点到当天 `23:59`;支持 `1h`、`3h`、`今天` 快捷设置;时间框不允许清空(清空会自动回填默认时间)。
- **首次打开默认**:环境=测试、级别全选、方向=倒序。已选条件记在浏览器(localStorage),老用户下次打开沿用上次选择。
- **结果展示**:按级别着色的日志行;可点击复制的标签(`app`/`level` 用固定颜色,其余标签按出现位置依次取色,兼容不同系统不同的标签 key);每分钟 × 级别堆叠柱状图;长日志(>10 行)可折叠。

## 开发

- 测试:`make test`
- 构建:`make build`

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
