const SHARE_KEYS = ["namespace", "env", "app", "levels", "direction", "keyword", "trace_id", "span_id", "parent_span_id", "task_id", "message_id", "start", "end"];
const SHARE_LEVELS = typeof LEVEL_ORDER !== "undefined" ? LEVEL_ORDER : require("./levels.js").LEVEL_ORDER;
const VALID_LEVELS = new Set(SHARE_LEVELS);
const VALID_DIRECTIONS = new Set(["BACKWARD", "FORWARD"]);

function normalizeText(v) {
  return (v || "").trim();
}

function positiveInt(v) {
  const n = Number(v);
  return Number.isInteger(n) && n > 0 ? n : 0;
}

function encodeShareState(state) {
  const params = new URLSearchParams();
  const env = normalizeText(state && state.env);
  const app = normalizeText(state && state.app);
  const keyword = normalizeText(state && state.keyword);
  const traceId = normalizeText(state && state.traceId);
  const direction = normalizeText(state && state.direction);
  const start = positiveInt(state && state.start);
  const end = positiveInt(state && state.end);
  const levels = Array.isArray(state && state.levels) ? state.levels.filter((l) => VALID_LEVELS.has(l)) : [];

  if (normalizeText(state && state.namespace)) params.set("namespace", normalizeText(state.namespace));
  if (env) params.set("env", env);
  if (app) params.set("app", app);
  if (levels.length > 0) params.set("levels", levels.join(","));
  if (VALID_DIRECTIONS.has(direction)) params.set("direction", direction);
  if (keyword) params.set("keyword", keyword);
  if (traceId) params.set("trace_id", traceId);
 for (const [key, param] of [["spanId", "span_id"], ["parentSpanId", "parent_span_id"], ["taskId", "task_id"], ["messageId", "message_id"]]) {
 const value = normalizeText(state && state[key]);
 if (value) params.set(param, value);
 }
  if (start) params.set("start", String(start));
  if (end) params.set("end", String(end));
  return params.toString();
}

function decodeShareState(search) {
  const params = new URLSearchParams(search || "");
  if (!SHARE_KEYS.some((k) => params.has(k))) return null;

  const direction = normalizeText(params.get("direction"));
  const levels = normalizeText(params.get("levels"))
    .split(",")
    .map((l) => l.trim())
    .filter((l) => VALID_LEVELS.has(l));

  return {
    namespace: normalizeText(params.get("namespace")),
    env: normalizeText(params.get("env")),
    app: normalizeText(params.get("app")),
    levels,
    direction: VALID_DIRECTIONS.has(direction) ? direction : "",
    keyword: normalizeText(params.get("keyword")),
    traceId: normalizeText(params.get("trace_id")),
 spanId: normalizeText(params.get("span_id")),
 parentSpanId: normalizeText(params.get("parent_span_id")),
 taskId: normalizeText(params.get("task_id")),
    messageId: normalizeText(params.get("message_id")),
    start: positiveInt(params.get("start")),
    end: positiveInt(params.get("end")),
  };
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { encodeShareState, decodeShareState };
}
