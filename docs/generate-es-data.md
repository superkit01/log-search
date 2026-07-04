# 生成 Elasticsearch 测试数据

这个 Python 工具用于生成测试用的日志数据并批量插入到 Elasticsearch。

## 使用方法

### 方式 1: 使用 Makefile

```bash
# 使用默认 ES 地址 (http://localhost:9200)
make seed-es

# 指定 ES 地址
ES_URL=http://your-es:9200 make seed-es
```

### 方式 2: 直接运行 Python 脚本

```bash
# 使用默认参数
./scripts/seed_es.py

# 指定 ES 地址
ES_URL=http://your-es:9200 ./scripts/seed_es.py

# 查看所有选项
./scripts/seed_es.py --help
```

## 命令行选项

```
--url        ES 地址 (默认: http://localhost:9200)
--index      索引名，支持 {date} 占位符 (默认: logs-{date})
--env        环境标签值 (默认: test)
--count      生成日志条数 (默认: 1000)
--hours      时间跨度(小时) (默认: 24)
--seed       随机种子 (用于可重现数据)
--dry-run    打印数据而不发送
```

## 使用示例

```bash
# 生成 1000 条测试数据，分布在过去 24 小时
./scripts/seed_es.py

# 生成 5000 条数据，分布在过去 7 天
./scripts/seed_es.py --count 5000 --hours 168

# 使用自定义 ES 地址和索引
./scripts/seed_es.py --url http://prod-es:9200 --index app-logs-{date}

# 查看生成的数据而不发送
./scripts/seed_es.py --dry-run | head -50
```

## 生成的数据特征

- **数量**: 默认 1000 条（可通过 --count 调整）
- **时间范围**: 默认过去 24 小时（可通过 --hours 调整）
- **应用**: api-server, worker, gateway, auth-service
- **级别**: debug, info, warn, error
- **字段**:
  - `@timestamp`: ISO 8601 格式时间戳
  - `message`: 日志消息
  - `env`: 环境名称
  - `app`: 应用名称
  - `level`: 日志级别
  - `host`: 主机名
  - `method`: HTTP 方法
  - `path`: 请求路径
  - `status`: HTTP 状态码
  - `latency_ms`: 延迟毫秒
  - `tid`: 链路追踪 ID (10% 的日志)
  - `pod`: Pod 名称 (20% 的日志)

## 验证数据

```bash
# 查看所有索引的文档数
curl -s 'localhost:9200/_cat/count/logs-*?pretty'

# 查看最新的日志
curl -s 'localhost:9200/logs-*/_search?pretty&size=3&sort=@timestamp:desc'

# 查询特定级别的日志
curl -s 'localhost:9200/logs-*/_search?pretty' -H 'Content-Type: application/json' -d '{
  "query": {"term": {"level": "error"}},
  "size": 5,
  "sort": [{"@timestamp": "desc"}]
}'

# 查询特定应用的日志
curl -s 'localhost:9200/logs-*/_search?pretty' -H 'Content-Type: application/json' -d '{
  "query": {"term": {"app": "api-server"}},
  "size": 5
}'
```

## 配合 log-search 使用

1. 修改 `config.yaml` 使用 ES:
   ```yaml
   storage:
     type: es
     elasticsearch:
       url: "http://localhost:9200"
       index: "logs-{yyyy.MM.dd}"
   ```

2. 生成测试数据:
   ```bash
   make seed-es
   ```

3. 启动服务:
   ```bash
   make run
   ```

4. 在界面中查询测试数据

## 对比 Loki 脚本

| 特性 | Loki (seed_loki.py) | ES (seed_es.py) |
|-----|---------------------|-----------------|
| 数据格式 | LogQL push API | ES bulk API |
| 时间戳 | 纳秒 | ISO 8601 |
| 索引 | stream 标签 | document 字段 |
| 查询 | LogQL | ES DSL |
