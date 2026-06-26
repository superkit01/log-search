# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## What this is

`log-search` is a single-process Go web app (Gin) that queries an existing **Loki** instance and renders a server-side UI. Frontend and backend are not separated: Gin renders HTML templates and serves the JSON API the page calls. There is no frontend build step — `web/static/app.js` is plain JS and Chart.js loads from a CDN.

## Commands

```bash
make run        # go run ./cmd/server (needs ./config.yaml)
make build      # builds ./bin/log-search
make test       # go test ./...
make vet        # go vet ./...
make tidy       # go mod tidy

# single test / package
go test ./internal/web/ -run TestQueryRange_BadRange -v
go test ./internal/loki/ -v
```

Before first run: `cp config.example.yaml config.yaml` and set `loki.url` to the real Loki. `config.yaml` is gitignored (it holds environment-specific endpoints); only `config.example.yaml` is tracked. The server reads `--config` (default `config.yaml`).

## Architecture

Request flow: **browser JS → `internal/web` handler → `internal/loki` client → Loki HTTP API → flatten streams → `internal/logparse` fills level → JSON → browser**.

The layering is intentional and each boundary matters:

- **`internal/config`** — `Load(path)` reads YAML, then `validate()` both checks (`loki.url` must be `http(s)://host`) and **applies defaults** (`:8080`, `30s`, 1000). Validation mutates the receiver. Invalid config is fail-fast — `main` exits 1.
- **`internal/loki`** — talks to Loki over plain `net/http` (no Loki client library). `Client` is an **interface** so the web layer can be tested with a mock; `NewHTTPClient` is the only real implementation. Start/end are sent to Loki as **nanosecond** unix (`UnixNano()`), even though the API accepts seconds. `get()` is the single place that does HTTP and error mapping.
- **`internal/logparse`** — `DetectLevel(line string) Level` is a **pure** function; the loki package must not import it. It returns a `Level` (a `string` type, lowercase: `"error"`/`"warn"`/`"info"`/`"debug"`).
- **`internal/web`** — Gin handlers. It is the only layer that calls `logparse`, and it owns level enrichment: the loki client returns `Line.Level == ""` and `handler.queryRange` fills it before responding.

### Two contracts that are easy to break

1. **Error status mapping** lives in `web.lokiStatusAndBody` (handler.go). A `*loki.HTTPError` with a 4xx status passes through as the **same 4xx** (e.g. a bad-LogQL response from Loki → 400 to the browser); everything else (network/timeout/5xx) → **502**. The `loki` client returns `&HTTPError` only for non-success HTTP responses; transport failures are plain wrapped errors so they stay 502. If you change error handling in either package, keep this split intact and covered by tests.

2. **Response shapes differ between endpoints.** `/api/labels` and `/api/label/:name/values` return `{"data": [...]}` (via `respondData`). `/api/query_range` returns the `QueryResult` **directly**: `{"lines":[{ts,level,line,labels}], "total":N}`. The frontend (`app.js`) expects these exact shapes — don't wrap `query_range` in `data`.

### Wiring (`cmd/server/main.go`)

`main` is the only place templates and static assets are mounted: `r.LoadHTMLGlob("web/templates/*")` and `r.Static("/static", "./web/static")`. The `index` handler renders `"index.html"` with `{{.defaultLimit}}`. These paths are **relative to the working directory** — always run `make run` from the repo root, not from inside `cmd/server`.

## Conventions

- The `go` directive in go.mod is pinned to the `1.24` line (`go 1.24.5`); gin v1.10.0 is compatible with it. If `go mod tidy` bumps the directive or downgrades gin, that's a regression — gin v1.10 requires at least `go 1.24.5`, so the directive should not exceed the 1.24 line.
- `direction` defaults to `BACKWARD` and treats any non-`"FORWARD"` value as `BACKWARD` (lenient by design). `limit` is bounded 1–5000 (`maxLimit`).
- Tests in `internal/loki` use `httptest` (real HTTP, not mocks of the client); tests in `internal/web` inject a mock `loki.Client`. The mock in `handler_test.go` must match the `loki.Client` interface signatures exactly — a signature drift compiles only there.
- Log lines are rendered with `textContent` in the frontend (not `innerHTML`) to avoid XSS from log content.

## Design & plan documents

The spec and the task-by-task implementation plan live under `docs/superpowers/`. Consult `docs/superpowers/specs/2026-06-24-log-search-design.md` for the rationale behind the feature tier (LogQL query, not live tail) and the histogram-is-sample tradeoff before expanding scope.
