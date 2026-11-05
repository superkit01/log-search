# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## What this is

`log-search` is a single-process Go web app (Gin) that queries an existing log storage backend (**Loki** or **Elasticsearch**) and renders a server-side UI. Frontend and backend are not separated: Gin renders HTML templates and serves the JSON API the page calls. There is no frontend build step — `web/static/js/app.js` is plain JS and Chart.js is vendored under `web/static/`.

## Commands

```bash
make run        # go run ./cmd/server (needs ./config.yaml)
make build      # builds ./bin/log-search
make test       # go test ./...
make vet        # go vet ./...
make tidy       # go mod tidy

# single test / package
go test ./internal/web/ -run TestQueryRange_BadRange -v
go test ./internal/storage/loki/ -v
go test ./internal/storage/elasticsearch/ -v
node --test web/tests/*.test.js
```

Before first run: `cp config.example.yaml config.yaml`, choose `storage.type` (`loki` or `es`), and set the matching storage URL/index. `config.yaml` is gitignored (it holds environment-specific endpoints); only `config.example.yaml` is tracked. The server reads `--config` (default `config.yaml`).

## Architecture

Request flow: **browser / MCP JSON filters → `internal/query` validates range/pagination → `internal/storage.Client` implementation builds backend query → Loki HTTP API or ES `_search` → normalize lines/tags → JSON → browser**.

The layering is intentional and each boundary matters:

- **`internal/config`** — `Load(path)` reads YAML, expands `${VAR}` / `${VAR:-default}`, then `validate()` both checks and **applies defaults** (`:8080`, `30s`, 1000). `storage.type` defaults to `loki`; valid tracked values are `loki` and `es`. Validation mutates the receiver. Invalid config is fail-fast — `main` exits 1.
- **`internal/storage`** — owns the backend-neutral `Client` interface, `Line`, `QueryResult`, `Direction`, and `HTTPError`, plus the query-grammar pieces both backends share: `QueryFilters` (with `NormalizedLevels`/`HasSelector`) and keyword parsing. `ParseKeyword` → `KeywordExpression` recognizes `&&` (AND) and `||` (OR); a field mixing both operators (e.g. `a && b || c`) is treated as one literal phrase, and empty terms are dropped. Web handlers depend on this package, not on a concrete Loki/ES client.
- **`internal/storage/loki`** — talks to Loki over plain `net/http` (no Loki client library). Start/end are sent to Loki as **nanosecond** unix (`UnixNano()`), even though the API accepts seconds. It flattens streams and merges returned labels/structured metadata into `Line.Labels`. Keyword search maps through `filters.KeywordExpression()`: single/AND terms become chained `|= "term"` line filters; OR terms become one `|~ "regex"` filter (terms `regexp.QuoteMeta`-escaped and joined with `|`).
- **`internal/storage/elasticsearch`** — converts `storage.QueryFilters` directly to ES DSL, then calls `/{index}/_search`. It expects documents with top-level `@timestamp`, `message`, `env`, `app`, `level`, and optional `tid`. Exact label filters use `.keyword` fields; keyword search maps through `filters.KeywordExpression()` — single/AND terms become multiple `match_phrase` must clauses, OR becomes a nested `bool`/`should` (`minimum_should_match: 1`). Index resolution (`buildTimeRangeIndex`) expands namespace/env and supported date placeholders across all dates in the configured index timezone. Fields are configurable; consult config.example.yaml and docs/api/查询接口.md.
- **`internal/query`** — shared Request DTO, validation, cursor adjustment, storage execution and response normalization. Both browser and MCP use Service.Execute. **`internal/web`** handles HTTP body limits and maps errors; request.go is a Request alias.

### Contracts that are easy to break

1. **Error status mapping** lives in `web.storageStatusAndBody` (handler.go). A `*storage.HTTPError` with a 4xx status passes through as the **same 4xx** (e.g. a bad query response from storage → 400 to the browser); everything else (network/timeout/5xx) → **502**. Storage clients should return `&storage.HTTPError` only for non-success HTTP responses; transport failures should remain plain wrapped errors so they stay 502. If you change error handling in either package, keep this split intact and covered by tests.

2. **`/api/query_range` is POST JSON.** The browser sends filters (`env`, `app`, `levels`, `keyword`, `traceId`, `start`, `end`, `direction`, optional `cursor`) and the web layer forwards structured `storage.QueryFilters` to the active backend. The backend owns concrete query construction and `limit` is an optional request integer (1–1000), falling back to the active storage default_limit when omitted/null. `start`/`end` are unix **seconds**; `cursor` is a unix-**nanosecond** timestamp string. The web layer derives `NextCursor` itself (set only when a full effective `limit` of lines returns, equal to the last line's `UnixNano()`) — backends must **not** set it. Cursor advance is FORWARD → `start = cursor+1ns`, BACKWARD → `end = cursor-1ns`; a cursor that leaves no remaining range is rejected with 400. The request body is capped at 16 KiB (`maxQueryRangeBodyBytes`) → 413. The response is `QueryResult` **directly**: `{"lines":[{ts,level,line,labels}], "total":N, "nextCursor"?: "<ns>"}`. Don't wrap it in `data`, and don't move Loki LogQL or ES DSL construction back to the browser.

3. **Visible line tags are allowlisted.** `query.Service` allowlists env/app/level/tid/traceId/namespace/spanId/parentSpanId/taskId/messageId/sequence/logger_name/thread_name/host in `Line.Labels`; extend `internal/query/service.go` logTagKeys for new visible tags. `tid` is expected to be Loki structured metadata when using Loki, queried with the LogQL pipeline filter `| tid="..."`, not a stream selector label. With ES, `tid` is a normal document field queried as `tid.keyword`.

### Wiring (`cmd/server/main.go`)

**ES pagination extension (sequence sorting):** Elasticsearch additionally implements `storage.CursorClient`. Web requests use `QueryRangePage` with opaque `es1:` cursors that bind the original query/range/direction to a PIT and exact `search_after` values. Sorting is `@timestamp`, `sequence`, `_shard_doc` (same direction); missing sequence is tolerated. Backend pagination metadata is returned only in `QueryResult.Page` (`json:"-"`); the web layer still owns public `NextCursor`. ES cursor requests must not advance start/end. Legacy numeric time cursors and Loki retain the timestamp rules above. PITs renew for 2 minutes per page and are closed at end of results; abandoned searches expire automatically.

Templates and static assets are **embedded** into the binary via `go:embed` (`web/assets.go`, package `assets`, exposing `assets.FS`). `main` mounts them with `web.MountAssets(r, assets.FS)` — which parses `templates/*.html` (template name `index.html`) and serves `static/` at `/static`. Deployment needs only the binary + `config.yaml` — no `web/` directory on disk. (`make run`/`make build` still run from the repo root so the embed source `web/` is present at **build** time.)

## Conventions

- The `go` directive in go.mod is pinned to the `1.24` line (`go 1.24.5`); gin v1.10.0 is compatible with it. If `go mod tidy` bumps the directive or downgrades gin, that's a regression — preserve the existing dependency baseline without incidental upgrades.
- `direction` defaults to `BACKWARD` and treats any non-`"FORWARD"` value as `BACKWARD` (lenient by design). Query `limit` may be overridden per request (1–1000), otherwise it uses the active storage config (`storage.loki.default_limit` or `storage.elasticsearch.default_limit`); config validation bounds it to 1–1000 (`config.MaxQueryLimit`).
- Tests in `internal/storage/loki` use `httptest` (real HTTP, not mocks of the client); tests in `internal/web` inject a mock `storage.Client`. The mock in `handler_test.go` must match the `storage.Client` interface signatures exactly — a signature drift compiles only there.
- Log lines are rendered with `textContent` in the frontend (not `innerHTML`) to avoid XSS from log content.
- **Query-form defaults & persistence** (`app.js` `loadFilters` + `timeRange.js`): on first open (no `localStorage`) the form defaults to the first configured env (falling back to “演示”/`demo`), **all** level tabs selected, direction=`BACKWARD`, and a time range of today 00:00–23:59. Returning users get saved env/levels/direction restored from the `localStorage` key `log-search:filters`; those defaults are only fallbacks when a value is unset. Quick-range buttons (`1h`, `3h`, `今天`) are the only shortcuts; the two datetime inputs refuse to stay empty (clearing refills the default start/end on `change`).
- **"All levels" means no level filter.** `search()` drops the level condition entirely when every level tab is active (`levelTabs.length === allLevelTabs.length` ⇒ `levels = []`). So the default all-selected state queries all levels, not a `{level=~"..."}` selector.
- **Keyword grammar is shared by both backends.** `a && b` means AND, `a || b` means OR (`storage.ParseKeyword`); mixing both operators in one field falls back to a single literal phrase. The raw keyword string is forwarded as-is and each backend re-derives the expression via `filters.KeywordExpression()` — do not pre-split it in the web layer, or the two backends will drift.
- **Enter submits the query unless an IME is composing.** `keyboard.js` `shouldSubmitInputEnter` ignores Enter while a CJK input method is mid-composition (`compositionstart`/`compositionend` toggle a shared `inputComposing` flag) and also guards the legacy `keyCode === 229`, so the composition-confirm Enter never fires a premature search.
- **UI apps must match stored data.** `ui.apps` must list values that exist in the configured storage; selecting an absent app correctly returns zero rows.
- **Tag colors are stable by key** (`app.js` `tagColorClass` + `TAG_PALETTE`, `style.css` `.tag-c0..c6`). `app` and `level` use fixed classes; every other label uses a deterministic key hash so the same key keeps its color across rows, pagination, and reloads regardless of label order or missing fields. Adding a palette slot requires updating **both** the JS array and the CSS classes (dark + light).
- **Query form has four rows.** App/time; trace/task/message IDs; keyword/search/expand arrows; collapsed span/parent-span IDs. Shared settings manage theme/font size.

## Documentation

Current docs: [user guide](docs/guides/用户手册.md), [API contract](docs/api/查询接口.md), [architecture](docs/architecture/开发交接文档.md), [MCP integration](docs/integrations/MCP接入.md). Histogram counts only loaded lines. Detail/facets/live-tail are not implemented.

## Agent tools and MCP

`internal/agenttools` owns model-independent tool schemas and query result/link formatting. `internal/mcp` exposes stateless Streamable HTTP at `/mcp` using `Authorization: Bearer <key>` (`mcp.credentials` list of `{name, api-key}` entries only). `Handler.RegisterMCP` connects tools directly to the shared query service. Config validation fails when enabled without a valid key, with blank user names/keys, duplicate user names, or duplicate keys. Removing a user key and restarting revokes it. Never expose or log the MCP key in UI options/tool results. Normal UI/API access rules remain unchanged. See `docs/integrations/MCP接入.md`.
