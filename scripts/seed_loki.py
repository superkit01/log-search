#!/usr/bin/env python3
"""Seed Loki with synthetic logs for local log-search testing."""

from __future__ import annotations

import argparse
import json
import random
import sys
import time
import urllib.error
import urllib.request


LEVELS = ("debug", "info", "warn", "error")
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
    "validation failed\nfield=user_id reason=missing\nhandler=createOrder",
    "downstream timeout\nservice=payment-api attempt=2\nnext_action=retry",
    "stack trace sample\nmain.process_order:42\nworker.run:88\nruntime.goexit:1700",
)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate synthetic log lines and push them to Loki.",
    )
    parser.add_argument(
        "--url",
        default="http://localhost:3100",
        help="Loki base URL (default: %(default)s)",
    )
    parser.add_argument(
        "--env",
        default="test",
        help="env label value (default: %(default)s)",
    )
    parser.add_argument(
        "--apps",
        default="service-x,service-y",
        help="comma-separated app label values (default: %(default)s)",
    )
    parser.add_argument(
        "--count",
        type=positive_int,
        default=120,
        help="number of log lines to generate (default: %(default)s)",
    )
    parser.add_argument(
        "--minutes",
        type=positive_int,
        default=30,
        help="spread generated logs over the last N minutes (default: %(default)s)",
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
        help="print the Loki push payload instead of sending it",
    )
    return parser.parse_args(argv)


def positive_int(value: str) -> int:
    parsed = int(value)
    if parsed <= 0:
        raise argparse.ArgumentTypeError("must be greater than 0")
    return parsed


def push_url(base_url: str) -> str:
    return base_url.rstrip("/") + "/loki/api/v1/push"


def parse_apps(raw: str) -> list[str]:
    apps = [app.strip() for app in raw.split(",") if app.strip()]
    if not apps:
        raise ValueError("--apps must contain at least one app")
    return apps


def build_payload(
    *,
    apps: list[str],
    env: str,
    count: int,
    start_ns: int,
    step_ns: int,
    seed: int | None,
) -> dict[str, list[dict[str, object]]]:
    rng = random.Random(seed)
    streams = []
    for i in range(count):
        app = apps[i % len(apps)]
        level = LEVELS[i % len(LEVELS)]
        tid = f"tid-{(i // 4) + 1:04d}"
        timestamp_ns = start_ns + (i * step_ns)
        latency_ms = rng.randint(8, 1800)
        status = status_for_level(level, rng)
        line_obj = {
            "ts": timestamp_ns,
            "level": level,
            "env": env,
            "app": app,
            "tid": tid,
            "message": message_for_index(i, rng),
            "method": rng.choice(("GET", "POST", "PUT")),
            "path": rng.choice(("/api/orders", "/api/users", "/api/payments", "/api/search")),
            "status": status,
            "latency_ms": latency_ms,
        }
        streams.append(
            {
                "stream": {
                    "env": env,
                    "app": app,
                    "level": level,
                },
                "values": [
                    [
                        str(timestamp_ns),
                        json.dumps(line_obj, ensure_ascii=False, separators=(",", ":")),
                        {"tid": tid},
                    ]
                ],
            }
        )
    return {"streams": streams}


def message_for_index(index: int, rng: random.Random) -> str:
    if index % 6 == 5:
        return MULTILINE_MESSAGES[(index // 6) % len(MULTILINE_MESSAGES)]
    return rng.choice(MESSAGES)


def status_for_level(level: str, rng: random.Random) -> int:
    if level == "error":
        return rng.choice((500, 502, 503))
    if level == "warn":
        return rng.choice((400, 404, 429))
    return rng.choice((200, 200, 201, 204))


def send_payload(base_url: str, payload: dict[str, object]) -> tuple[int, str]:
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        push_url(base_url),
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Loki returned HTTP {err.code}: {body}") from err
    except urllib.error.URLError as err:
        raise RuntimeError(f"failed to connect to Loki: {err.reason}") from err


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        apps = parse_apps(args.apps)
    except ValueError as err:
        print(err, file=sys.stderr)
        return 2

    now_ns = time.time_ns()
    window_ns = args.minutes * 60 * 1_000_000_000
    step_ns = max(window_ns // args.count, 1)
    start_ns = now_ns - window_ns
    payload = build_payload(
        apps=apps,
        env=args.env,
        count=args.count,
        start_ns=start_ns,
        step_ns=step_ns,
        seed=args.seed,
    )

    if args.dry_run:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        return 0

    try:
        status, body = send_payload(args.url, payload)
    except RuntimeError as err:
        print(err, file=sys.stderr)
        return 1

    suffix = f": {body}" if body else ""
    print(f"pushed {args.count} log lines to {push_url(args.url)} (HTTP {status}){suffix}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
