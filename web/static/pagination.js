const AUTO_LOAD_THRESHOLD_PX = 80;

function shouldAutoLoadMore(metrics, hasCursor, isLoading) {
  if (!hasCursor || isLoading) return false;
  return metrics.scrollTop + metrics.clientHeight >= metrics.scrollHeight - AUTO_LOAD_THRESHOLD_PX;
}

if (typeof module !== "undefined") {
  module.exports = { shouldAutoLoadMore };
}
