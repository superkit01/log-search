# Log Search

基于 Gin + Loki 的日志查询工程(前后端不分离)。

## 快速开始

1. 复制配置并填入你的 Loki 地址:
   `cp config.example.yaml config.yaml`
2. 运行:`make run`(或 `go run ./cmd/server`)
3. 浏览器打开 http://localhost:8080

## 开发

- 测试:`make test`
- 构建:`make build`
