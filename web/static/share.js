const SHARE_KEYS = ["env", "app", "levels", "direction", "keyword", "trace_id", "start", "end"];
const VALID_LEVELS = new Set(["debug", "info", "warn", "error"]);
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

  if (env) params.set("env", env);
  if (app) params.set("app", app);
  if (levels.length > 0) params.set("levels", levels.join(","));
  if (VALID_DIRECTIONS.has(direction)) params.set("direction", direction);
  if (keyword) params.set("keyword", keyword);
  if (traceId) params.set("trace_id", traceId);
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
    env: normalizeText(params.get("env")),
    app: normalizeText(params.get("app")),
    levels,
    direction: VALID_DIRECTIONS.has(direction) ? direction : "",
    keyword: normalizeText(params.get("keyword")),
    traceId: normalizeText(params.get("trace_id")),
    start: positiveInt(params.get("start")),
    end: positiveInt(params.get("end")),
  };
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { encodeShareState, decodeShareState };
}
