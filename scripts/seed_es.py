#!/usr/bin/env python3
"""Seed Elasticsearch with synthetic logs for local log-search testing."""

from __future__ import annotations

import argparse
import json
import random
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta


LEVELS = ("debug", "info", "warn", "error")
APPS = ("api-server", "worker", "gateway", "auth-service")
ENVS = ("prod", "staging", "dev")
MESSAGES = (
    "received request",
    "loaded user profile",
    "cache lookup completed",
    "called downstream payment api",
    "database query finished",
    "retrying transient failure",
    "slow request detected",
    "request completed",
)
MULTILINE_MESSAGES = (
    "validation failed\\nfield=user_id reason=missing\\nhandler=createOrder",
    "downstream timeout\\nservice=payment-api attempt=2\\nnext_action=retry",
    "stack trace sample\\nmain.process_order:42\\nworker.run:88",
)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate synthetic log entries and push them to Elasticsearch.",
    )
    parser.add_argument(
        "--url",
        default="http://localhost:9200",
        help="Elasticsearch base URL (default: %(default)s)",
    )
    parser.add_argument(
        "--index",
        default="logs-{date}",
        help="Index pattern, use {date} for today's date in YYYY.MM.DD format (default: %(default)s)",
    )
    parser.add_argument(
        "--env",
        default="test",
        help="env label value (default: %(default)s)",
    )
    parser.add_argument(
        "--count",
        type=positive_int,
        default=1000,
        help="number of log entries to generate (default: %(default)s)",
    )
    parser.add_argument(
        "--hours",
        type=positive_int,
        default=24,
        help="spread generated logs over the last N hours (default: %(default)s)",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=None,
        help="random seed for reproducible data",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="print the ES bulk payload instead of sending it",
    )
    return parser.parse_args(argv)


def positive_int(value: str) -> int:
    parsed = int(value)
    if parsed <= 0:
        raise argparse.ArgumentTypeError("must be greater than 0")
    return parsed


def build_bulk_payload(
    *,
    index: str,
    env: str,
    count: int,
    start_time: datetime,
    step_seconds: int,
    seed: int | None,
) -> str:
    rng = random.Random(seed)
    bulk_lines = []

    for i in range(count):
        timestamp = start_time + timedelta(seconds=i * step_seconds)
        app = APPS[i % len(APPS)]
        level = LEVELS[i % len(LEVELS)]
        tid = f"tid-{(i // 4) + 1:04d}"

        # 构建日志条目
        # 使用 UTC 时间并添加 Z 后缀，确保与 Go 的 RFC3339Nano 格式兼容
        doc = {
            "@timestamp": timestamp.isoformat() + "+08:00",  # 中国时区
            "message": message_for_index(i, rng),
            "env": env,
            "app": app,
            "level": level,
            "host": f"host-{i % 10}",
            "method": rng.choice(("GET", "POST", "PUT")),
            "path": rng.choice(("/api/orders", "/api/users", "/api/payments", "/api/search")),
            "status": status_for_level(level, rng),
            "latency_ms": rng.randint(8, 1800),
        }

        # 10% 的日志有 traceId
        if i % 10 == 0:
            doc["tid"] = tid

        # 20% 的日志有 pod 信息
        if i % 5 == 0:
            doc["pod"] = f"{app}-{i % 5}"

        # 添加一些关键词用于搜索测试
        if i % 20 == 0:
            doc["message"] = f"[TEST] {doc['message']} with keyword timeout"

        # 构建 bulk API 格式
        # 第一行：index 操作
        index_op = {"index": {"_index": index}}
        bulk_lines.append(json.dumps(index_op, ensure_ascii=False))

        # 第二行：文档数据
        bulk_lines.append(json.dumps(doc, ensure_ascii=False))

    return "\n".join(bulk_lines) + "\n"


def message_for_index(index: int, rng: random.Random) -> str:
    if index % 6 == 5:
        # 处理多行消息中的转义
        msg = MULTILINE_MESSAGES[(index // 6) % len(MULTILINE_MESSAGES)]
        # 在 JSON 中不需要 \\n，直接使用 \n
        return msg.replace("\\n", "\n")
    return rng.choice(MESSAGES)


def status_for_level(level: str, rng: random.Random) -> int:
    if level == "error":
        return rng.choice((500, 502, 503))
    if level == "warn":
        return rng.choice((400, 404, 429))
    return rng.choice((200, 200, 201, 204))


def send_payload(es_url: str, bulk_data: str) -> tuple[int, str]:
    url = es_url.rstrip("/") + "/_bulk"
    req = urllib.request.Request(
        url,
        data=bulk_data.encode("utf-8"),
        headers={"Content-Type": "application/x-ndjson"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read().decode("utf-8", errors="replace")
            return resp.status, body
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"ES returned HTTP {err.code}: {body}") from err
    except urllib.error.URLError as err:
        raise RuntimeError(f"failed to connect to ES: {err.reason}") from err


def check_es_health(es_url: str) -> None:
    """检查 ES 是否可达"""
    try:
        url = es_url.rstrip("/") + "/_cluster/health"
        with urllib.request.urlopen(url, timeout=5) as resp:
            if resp.status >= 400:
                raise RuntimeError(f"ES health check failed: HTTP {resp.status}")
    except urllib.error.URLError as err:
        raise RuntimeError(f"failed to connect to ES: {err.reason}") from err


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)

    # 替换索引名中的日期占位符
    today = datetime.now().strftime("%Y.%m.%d")
    index = args.index.replace("{date}", today)

    # 计算时间范围：从今天 00:00:00 开始，到现在
    now = datetime.now()
    start_time = now.replace(hour=0, minute=0, second=0, microsecond=0)
    time_range_hours = (now - start_time).total_seconds() / 3600
    step_seconds = max(int(time_range_hours * 3600 / args.count), 1)

    # 构建 bulk payload
    payload = build_bulk_payload(
        index=index,
        env=args.env,
        count=args.count,
        start_time=start_time,
        step_seconds=step_seconds,
        seed=args.seed,
    )

    if args.dry_run:
        print(payload)
        return 0

    # 检查 ES 连接
    try:
        print(f"Checking ES connection at {args.url}...")
        check_es_health(args.url)
        print("✓ ES connection OK")
    except RuntimeError as err:
        print(f"✗ {err}", file=sys.stderr)
        return 1

    # 发送数据
    try:
        print(f"Pushing {args.count} entries to index {index}...")
        status, body = send_payload(args.url, payload)

        # 解析响应检查是否有错误
        result = json.loads(body) if body else {}
        if result.get("errors"):
            errors = []
            for item in result.get("items", []):
                if err := item.get("index", {}).get("error"):
                    errors.append(err)
            if errors:
                print(f"✗ Some entries failed to index: {errors[0]}", file=sys.stderr)
                return 1

        suffix = f": {body}" if body and len(body) < 200 else ""
        print(f"✓ Pushed {args.count} log entries (HTTP {status}){suffix}")
        return 0
    except RuntimeError as err:
        print(f"✗ {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
