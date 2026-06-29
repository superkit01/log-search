# syntax=docker/dockerfile:1

# ---- 构建阶段 ----
FROM golang:1.24-alpine AS builder
WORKDIR /src

# 先拷依赖,利用层缓存(go mod download 只在 go.mod/go.sum 变化时重跑)。
COPY go.mod go.sum ./
RUN go mod download

# 拷源码并构建。CGO_ENABLED=0 产出全静态二进制,可跑在 alpine/distroless/scratch;
# -trimpath -s -w 去除调试信息与路径,缩小体积。
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/log-search ./cmd/server

# ---- 运行阶段 ----
FROM alpine:3.20
# 访问 Loki(若为 https)需要 CA 证书;tzdata 供时区处理。
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
# 二进制(模板与静态资源已用 go:embed 编译进二进制,无需单独拷贝 web/)
COPY --from=builder /out/log-search /app/log-search
# 默认配置:config.yaml 被 .gitignore(含环境相关的 Loki 地址,不入库),
# 这里用 config.example.yaml 兜底;运行时用 -v ./config.yaml:/app/config.yaml 挂真实配置覆盖。
COPY config.example.yaml /app/config.yaml

# 非 root 运行
RUN adduser -D -u 10001 app && chown -R app /app
USER app

EXPOSE 8080
# ENTRYPOINT 为二进制,CMD 给默认 --config,便于覆盖:
#   docker run log-search --config /path/to/other.yaml
ENTRYPOINT ["/app/log-search"]
CMD ["--config", "/app/config.yaml"]
