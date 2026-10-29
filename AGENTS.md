# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## What this is

`log-search` is a single-process Go web app (Gin) that queries an existing log storage backend (**Loki** or **Elasticsearch**) and renders a server-side UI. Frontend and backend are not separated: Gin renders HTML templates and serves the JSON API the page calls. There is no frontend build step — `web/static/app.js` is plain JS and Chart.js is vendored under `web/static/`.

## Commands

```bash
make run        # go run ./cmd/server (needs ./config.yaml)
make build      # builds ./bin/log-search
make test       # go test ./...
make vet        # go vet ./...
make tidy       # go mod tidy
make seed-loki  # generate Loki test data
make seed-es    # generate Elasticsearch test data

# single test / package
go test ./internal/web/ -run TestQueryRange_BadRange -v
go test ./internal/loki/ -v
go test ./internal/elasticsearch/ -v
node --test web/*.test.js
```

Before first run: `cp config.example.yaml config.yaml`, choose `storage.type` (`loki` or `es`), and set the matching storage URL/index. `config.yaml` is gitignored (it holds environment-specific endpoints); only `config.example.yaml` is tracked. The server reads `--config` (default `config.yaml`).

## Architecture

Request flow: **browser JSON filters → `internal/web` validates range/pagination → `internal/storage.Client` implementation builds backend query → Loki HTTP API or ES `_search` → normalize lines/tags → JSON → browser**.

The layering is intentional and each boundary matters:

- **`internal/config`** — `Load(path)` reads YAML, expands `${VAR}` / `${VAR:-default}`, then `validate()` both checks and **applies defaults** (`:8080`, `30s`, 1000). `storage.type` defaults to `loki`; valid tracked values are `loki` and `es`. Validation mutates the receiver. Invalid config is fail-fast — `main` exits 1.
- **`internal/storage`** — owns the backend-neutral `Client` interface, `Line`, `QueryResult`, `Direction`, and `HTTPError`, plus the query-grammar pieces both backends share: `QueryFilters` (with `NormalizedLevels`/`HasSelector`) and keyword parsing. `ParseKeyword` → `KeywordExpression` recognizes `&&` (AND) and `||` (OR); a field mixing both operators (e.g. `a && b || c`) is treated as one literal phrase, and empty terms are dropped. Web handlers depend on this package, not on a concrete Loki/ES client.
- **`internal/loki`** — talks to Loki over plain `net/http` (no Loki client library). Start/end are sent to Loki as **nanosecond** unix (`UnixNano()`), even though the API accepts seconds. It flattens streams and merges returned labels/structured metadata into `Line.Labels`. Keyword search maps through `filters.KeywordExpression()`: single/AND terms become chained `|= "term"` line filters; OR terms become one `|~ "regex"` filter (terms `regexp.QuoteMeta`-escaped and joined with `|`).
- **`internal/elasticsearch`** — converts `storage.QueryFilters` directly to ES DSL, then calls `/{index}/_search`. It expects documents with top-level `@timestamp`, `message`, `env`, `app`, `level`, and optional `tid`. Exact label filters use `.keyword` fields; keyword search maps through `filters.KeywordExpression()` — single/AND terms become multiple `match_phrase` must clauses, OR becomes a nested `bool`/`should` (`minimum_should_match: 1`). Index resolution (`buildTimeRangeIndex`) expands `{env}` plus date placeholders `{yyyy}` / `{yyyy.MM}` / `{yyyy.MM.dd}`: spans under 1 day resolve to a single index (at `start`), longer spans comma-join the `start` and `end` index names (ES multi-index syntax); a pattern with no `{` left after env substitution (e.g. `logs-{env}-*`) is passed through so ES applies its own wildcard, and patterns with no placeholders at all (e.g. `logs-*`) are passed through unchanged.
- **`internal/web`** — Gin handlers. It owns browser query JSON validation, cursor adjustment, and response shaping. It passes `storage.QueryFilters` to the active backend and does not build Loki LogQL. `handler.queryRange` copies `labels["level"]` into `Line.Level` before responding and allowlists visible tags. (Despite the legacy filename, `internal/web/logql.go` only holds the `queryRangeRequest` DTO and its `filters()` mapper — LogQL/DSL construction lives in the storage backends.)

### Contracts that are easy to break

1. **Error status mapping** lives in `web.storageStatusAndBody` (handler.go). A `*storage.HTTPError` with a 4xx status passes through as the **same 4xx** (e.g. a bad query response from storage → 400 to the browser); everything else (network/timeout/5xx) → **502**. Storage clients should return `&storage.HTTPError` only for non-success HTTP responses; transport failures should remain plain wrapped errors so they stay 502. If you change error handling in either package, keep this split intact and covered by tests.

2. **`/api/query_range` is POST JSON.** The browser sends filters (`env`, `app`, `levels`, `keyword`, `traceId`, `start`, `end`, `direction`, optional `cursor`) and the web layer forwards structured `storage.QueryFilters` to the active backend. The backend owns concrete query construction and `limit` comes from config. `start`/`end` are unix **seconds**; `cursor` is a unix-**nanosecond** timestamp string. The web layer derives `NextCursor` itself (set only when a full `limit` of lines returns, equal to the last line's `UnixNano()`) — backends must **not** set it. Cursor advance is FORWARD → `start = cursor+1ns`, BACKWARD → `end = cursor-1ns`; a cursor that leaves no remaining range is rejected with 400. The request body is capped at 16 KiB (`maxQueryRangeBodyBytes`) → 413. The response is `QueryResult` **directly**: `{"lines":[{ts,level,line,labels}], "total":N, "nextCursor"?: "<ns>"}`. Don't wrap it in `data`, and don't move Loki LogQL or ES DSL construction back to the browser.

3. **Visible line tags are allowlisted.** `handler.queryRange` returns only `env`, `app`, `level`, and `tid` in `Line.Labels`; add future tags through `logTagKeys`. `tid` is expected to be Loki structured metadata when using Loki, queried with the LogQL pipeline filter `| tid="..."`, not a stream selector label. With ES, `tid` is a normal document field queried as `tid.keyword`.

### Wiring (`cmd/server/main.go`)

**ES pagination extension (sequence sorting):** Elasticsearch additionally implements `storage.CursorClient`. Web requests use `QueryRangePage` with opaque `es1:` cursors that bind the original query/range/direction to a PIT and exact `search_after` values. Sorting is `@timestamp`, `sequence`, `_shard_doc` (same direction); missing sequence is tolerated. Backend pagination metadata is returned only in `QueryResult.Page` (`json:"-"`); the web layer still owns public `NextCursor`. ES cursor requests must not advance start/end. Legacy numeric time cursors and Loki retain the timestamp rules above. PITs renew for 2 minutes per page and are closed at end of results; abandoned searches expire automatically.

Templates and static assets are **embedded** into the binary via `go:embed` (`web/assets.go`, package `assets`, exposing `assets.FS`). `main` mounts them with `web.MountAssets(r, assets.FS)` — which parses `templates/*.html` (template name `index.html`) and serves `static/` at `/static`. Deployment needs only the binary + `config.yaml` — no `web/` directory on disk. (`make run`/`make build` still run from the repo root so the embed source `web/` is present at **build** time.)

## Conventions

- The `go` directive in go.mod is pinned to the `1.24` line (`go 1.24.5`); gin v1.10.0 is compatible with it. If `go mod tidy` bumps the directive or downgrades gin, that's a regression — gin v1.10 requires at least `go 1.24.5`, so the directive should not exceed the 1.24 line.
- `direction` defaults to `BACKWARD` and treats any non-`"FORWARD"` value as `BACKWARD` (lenient by design). Query `limit` is backend-owned from the active storage config (`storage.loki.default_limit` or `storage.elasticsearch.default_limit`); config validation bounds it to 1–5000 (`config.MaxQueryLimit`).
- Tests in `internal/loki` use `httptest` (real HTTP, not mocks of the client); tests in `internal/web` inject a mock `storage.Client`. The mock in `handler_test.go` must match the `storage.Client` interface signatures exactly — a signature drift compiles only there.
- Log lines are rendered with `textContent` in the frontend (not `innerHTML`) to avoid XSS from log content.
- **Query-form defaults & persistence** (`app.js` `loadFilters` + `timeRange.js`): on first open (no `localStorage`) the form defaults to the first configured env (falling back to “演示”/`demo`), **all** level tabs selected, direction=`BACKWARD`, and a time range of today 00:00–23:59. Returning users get saved env/levels/direction restored from the `localStorage` key `log-search:filters`; those defaults are only fallbacks when a value is unset. Quick-range buttons (`1h`, `3h`, `今天`) are the only shortcuts; the two datetime inputs refuse to stay empty (clearing refills the default start/end on `change`).
- **"All levels" means no level filter.** `search()` drops the level condition entirely when every level tab is active (`levelTabs.length === allLevelTabs.length` ⇒ `levels = []`). So the default all-selected state queries all levels, not a `{level=~"..."}` selector.
- **Keyword grammar is shared by both backends.** `a && b` means AND, `a || b` means OR (`storage.ParseKeyword`); mixing both operators in one field falls back to a single literal phrase. The raw keyword string is forwarded as-is and each backend re-derives the expression via `filters.KeywordExpression()` — do not pre-split it in the web layer, or the two backends will drift.
- **Enter submits the query unless an IME is composing.** `keyboard.js` `shouldSubmitInputEnter` ignores Enter while a CJK input method is mid-composition (`compositionstart`/`compositionend` toggle a shared `inputComposing` flag) and also guards the legacy `keyCode === 229`, so the composition-confirm Enter never fires a premature search.
- **ES test data and UI apps must match.** `scripts/seed_es.py` writes apps `api-server`, `worker`, `gateway`, and `auth-service`; `ui.apps` must list values that actually exist in ES documents, or selecting an app will correctly return zero rows. The ES script writes `tid` on 10% of documents.
- **Tag colors are positional, not key-based** (`app.js` `TAG_PALETTE` + `style.css` `.tag-c0..c6`). `app` and `level` use fixed classes; every other label is colored by render position (cycling `tag-c0..c6`) so unknown/varying label keys across deployments always get a color. Adding a palette slot requires updating **both** the JS array and the CSS classes (dark + light).
- **Query form is one flowing row.** All conditions (app, time group, trace-id, keyword) and the search button live in a single `.row.query-row` (`flex-wrap: wrap`); `.query-row > * { flex: 0 0 auto }` keeps each at its fixed width (app/time 190px, trace-id/keyword 500px) and wraps instead of stretching. `.controls .row` no longer sets `align-items` (defaults to `stretch`).

## Design & plan documents

The spec and the task-by-task implementation plan live under `docs/superpowers/`. Consult `docs/superpowers/specs/2026-06-24-log-search-design.md` for the rationale behind the feature tier (LogQL query, not live tail) and the histogram-is-sample tradeoff before expanding scope.
