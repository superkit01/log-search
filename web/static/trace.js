function traceDrilldownState(current, line, traceId) {
  const tid = (traceId || "").trim();
  if (!current || !tid) return null;
  return {
    namespace: (current && current.namespace) || ((line && line.labels) || {}).namespace || "",
    env: (current && current.env) || ((line && line.labels) || {}).env || "",
    app: "",
    levels: [],
    direction: (current && current.direction) || "FORWARD",
    keyword: "",
    traceId: tid,
    start: current.start,
    end: current.end,
  };
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { traceDrilldownState };
}
