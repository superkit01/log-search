// Display preferences for the log page.
function initDisplaySettings(onFontSize) {
  const el = id => document.getElementById(id);
  const read = key => { try { return localStorage.getItem(key); } catch (_) { return null; } };
  const save = (key, value) => { try { localStorage.setItem(key, value); } catch (_) {} };
  const applySize = value => {
    const size = [90,100,110,120,130,150].includes(Number(value)) ? Number(value) : 100;
    document.documentElement.style.fontSize = `${16 * size / 100}px`;
    el("font-size").value = String(size);
    if (onFontSize) onFontSize(size);
  };
  const applyTheme = light => {
    document.body.classList.toggle("light-theme", light);
    el("theme-label").textContent = light ? "浅色" : "深色";
    el("theme-toggle").setAttribute("aria-label", light ? "切换为深色主题" : "切换为浅色主题");
    document.querySelector(".theme-icon-dark").style.display = light ? "none" : "block";
    document.querySelector(".theme-icon-light").style.display = light ? "block" : "none";
  };
  applySize(read("log-search:font-size"));
  applyTheme(read("log-search:theme") === "light");
  el("font-size").addEventListener("change", e => { applySize(e.target.value); save("log-search:font-size", e.target.value); });
  el("theme-toggle").addEventListener("click", () => {
    const light = !document.body.classList.contains("light-theme");
    applyTheme(light); save("log-search:theme", light ? "light" : "dark");
  });
  const mcpDialog = el("mcp-dialog");
  el("mcp-open").addEventListener("click", () => {
    el("settings").open = false;
    mcpDialog.showModal();
  });
  el("mcp-close").addEventListener("click", () => mcpDialog.close());
  mcpDialog.addEventListener("click", e => {
    const bounds = mcpDialog.getBoundingClientRect();
    if (e.target === mcpDialog && (e.clientX < bounds.left || e.clientX > bounds.right || e.clientY < bounds.top || e.clientY > bounds.bottom)) mcpDialog.close();
  });
  mcpDialog.addEventListener("close", () => el("settings").querySelector("summary").focus());
  document.addEventListener("click", e => { if (!el("settings").contains(e.target)) el("settings").open = false; });
  document.addEventListener("keydown", e => {
    if (e.key === "Escape" && el("settings").open) { el("settings").open = false; el("settings").querySelector("summary").focus(); }
  });
}
