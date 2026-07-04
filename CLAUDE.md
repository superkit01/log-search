# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`log-search` is a single-process Go web app (Gin) that queries an existing **Loki** instance and renders a server-side UI. Frontend and backend are not separated: Gin renders HTML templates and serves the JSON API the page calls. There is no frontend build step — `web/static/app.js` is plain JS and Chart.js is vendored under `web/static/`.

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

Request flow: **browser JSON filters → `internal/web` handler builds LogQL → `internal/loki` client → Loki HTTP API → flatten streams → `internal/logparse` fills level → JSON → browser**.

The layering is intentional and each boundary matters:

- **`internal/config`** — `Load(path)` reads YAML, then `validate()` both checks (`loki.url` must be `http(s)://host`) and **applies defaults** (`:8080`, `30s`, 1000). Validation mutates the receiver. Invalid config is fail-fast — `main` exits 1.
- **`internal/loki`** — talks to Loki over plain `net/http` (no Loki client library). `Client` is an **interface** so the web layer can be tested with a mock; `NewHTTPClient` is the only real implementation. Start/end are sent to Loki as **nanosecond** unix (`UnixNano()`), even though the API accepts seconds. `get()` is the single place that does HTTP and error mapping.
- **`internal/logparse`** — `DetectLevel(line string) Level` is a **pure** function; the loki package must not import it. It returns a `Level` (a `string` type, lowercase: `"error"`/`"warn"`/`"info"`/`"debug"`).
- **`internal/web`** — Gin handlers. It owns browser query JSON validation, LogQL construction, cursor adjustment, and level enrichment: the loki client returns `Line.Level == ""` and `handler.queryRange` fills it before responding.

### Two contracts that are easy to break

1. **Error status mapping** lives in `web.lokiStatusAndBody` (handler.go). A `*loki.HTTPError` with a 4xx status passes through as the **same 4xx** (e.g. a bad-LogQL response from Loki → 400 to the browser); everything else (network/timeout/5xx) → **502**. The `loki` client returns `&HTTPError` only for non-success HTTP responses; transport failures are plain wrapped errors so they stay 502. If you change error handling in either package, keep this split intact and covered by tests.

2. **`/api/query_range` is POST JSON.** The browser sends filters (`env`, `app`, `levels`, `keyword`, `traceId`, `start`, `end`, `direction`, optional `cursor`) and the web layer builds LogQL. The backend owns `limit` from config and returns `QueryResult` **directly**: `{"lines":[{ts,level,line,labels}], "total":N}`. Don't wrap it in `data`, and don't move LogQL construction or limit selection back to the browser.

3. **Visible line tags are allowlisted.** `handler.queryRange` returns only `env`, `app`, `level`, and `tid` in `Line.Labels`; add future tags through `logTagKeys`. `tid` is expected to be Loki structured metadata, queried with the LogQL pipeline filter `| tid="..."`, not a stream selector label.

### Wiring (`cmd/server/main.go`)

Templates and static assets are **embedded** into the binary via `go:embed` (`web/assets.go`, package `assets`, exposing `assets.FS`). `main` mounts them with `web.MountAssets(r, assets.FS)` — which parses `templates/*.html` (template name `index.html`) and serves `static/` at `/static`. Deployment needs only the binary + `config.yaml` — no `web/` directory on disk. (`make run`/`make build` still run from the repo root so the embed source `web/` is present at **build** time.)

## Conventions

- The `go` directive in go.mod is pinned to the `1.24` line (`go 1.24.5`); gin v1.10.0 is compatible with it. If `go mod tidy` bumps the directive or downgrades gin, that's a regression — gin v1.10 requires at least `go 1.24.5`, so the directive should not exceed the 1.24 line.
- `direction` defaults to `BACKWARD` and treats any non-`"FORWARD"` value as `BACKWARD` (lenient by design). Query `limit` is backend-owned from `loki.default_limit`; config validation bounds it to 1–5000 (`config.MaxQueryLimit`).
- Tests in `internal/loki` use `httptest` (real HTTP, not mocks of the client); tests in `internal/web` inject a mock `loki.Client`. The mock in `handler_test.go` must match the `loki.Client` interface signatures exactly — a signature drift compiles only there.
- Log lines are rendered with `textContent` in the frontend (not `innerHTML`) to avoid XSS from log content.
- **Query-form defaults & persistence** (`app.js` `loadFilters`): on first open (no `localStorage`) the form defaults to the first configured env (falling back to “演示”/`demo`), **all** level tabs selected, direction=`BACKWARD`, and a time range from the previous rounded hour to today 23:59. Returning users get saved env/levels/direction restored from the `localStorage` key `log-search:filters`; those defaults are only fallbacks when a value is unset. Quick-range buttons (`1h`, `3h`, `今天`) are the only shortcuts; the two datetime inputs refuse to stay empty (clearing refills the default start/end on `change`).
- **"All levels" means no level filter.** `search()` drops the level condition entirely when every level tab is active (`levelTabs.length === allLevelTabs.length` ⇒ `levels = []`). So the default all-selected state queries all levels, not a `{level=~"..."}` selector.
- **Tag colors are positional, not key-based** (`app.js` `TAG_PALETTE` + `style.css` `.tag-c0..c6`). `app` and `level` use fixed classes; every other label is colored by render position (cycling `tag-c0..c6`) so unknown/varying label keys across deployments always get a color. Adding a palette slot requires updating **both** the JS array and the CSS classes (dark + light).
- **Query form is one flowing row.** All conditions (app, time group, trace-id, keyword) and the search button live in a single `.row.query-row` (`flex-wrap: wrap`); `.query-row > * { flex: 0 0 auto }` keeps each at its fixed width (app/time 190px, trace-id/keyword 500px) and wraps instead of stretching. `.controls .row` no longer sets `align-items` (defaults to `stretch`).

## Design & plan documents

The spec and the task-by-task implementation plan live under `docs/superpowers/`. Consult `docs/superpowers/specs/2026-06-24-log-search-design.md` for the rationale behind the feature tier (LogQL query, not live tail) and the histogram-is-sample tradeoff before expanding scope.
