function shouldSubmitQueryEnter(e, composing) {
  if (!e || e.key !== "Enter" || !e.target) return false;
  if (e.defaultPrevented || e.repeat || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return false;
  if (composing || e.isComposing || e.keyCode === 229) return false;
  return !e.target.closest?.("dialog[open]");
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { shouldSubmitQueryEnter };
}
