function shouldSubmitInputEnter(e, composing) {
  if (!e || e.key !== "Enter") return false;
  if (!e.target || e.target.tagName !== "INPUT") return false;
  if (composing || e.isComposing || e.keyCode === 229) return false;
  return true;
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { shouldSubmitInputEnter };
}
