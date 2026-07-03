const TRACE_DRILLDOWN_WINDOW_SECONDS = 15 * 60;

function traceDrilldownState(current, line, traceId) {
  const ts = Math.floor(new Date(line && line.ts).getTime() / 1000);
  const tid = (traceId || "").trim();
  if (!Number.isFinite(ts) || !tid) return null;
  return {
    env: (current && current.env) || ((line && line.labels) || {}).env || "",
    app: "",
    levels: [],
    direction: "FORWARD",
    keyword: "",
    traceId: tid,
    start: ts - TRACE_DRILLDOWN_WINDOW_SECONDS,
    end: ts + TRACE_DRILLDOWN_WINDOW_SECONDS,
  };
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { traceDrilldownState };
}
