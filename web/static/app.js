const $ = (id) => document.getElementById(id);
let chart;
const FILTERS_KEY = "log-search:filters"; // localStorage 键:存 {env, level}
const THEME_KEY = "log-search:theme"; // localStorage 键:存主题

// 主题切换
function toggleTheme() {
  const isLight = document.body.classList.toggle("light-theme");
  localStorage.setItem(THEME_KEY, isLight ? "light" : "dark");
  updateThemeIcon();
}

function updateThemeIcon() {
  const isLight = document.body.classList.contains("light-theme");
  document.querySelector(".theme-icon-dark").style.display = isLight ? "none" : "block";
  document.querySelector(".theme-icon-light").style.display = isLight ? "block" : "none";
}

function loadTheme() {
  const saved = localStorage.getItem(THEME_KEY);
  if (saved === "light") {
    document.body.classList.add("light-theme");
  }
  updateThemeIcon();
}

$("theme-toggle").addEventListener("click", toggleTheme);

function unixNow() { return Math.floor(Date.now() / 1000); }

function toLocalInput(ts) {
  const d = new Date(ts * 1000);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
}

function toUnix(inputVal) {
  if (!inputVal) return 0;
  return Math.floor(new Date(inputVal).getTime() / 1000);
}

async function api(path) {
  const r = await fetch(path);
  const j = await r.json();
  if (!r.ok) throw new Error(j.error || ("HTTP " + r.status));
  return j;
}

function showError(msg) { const b = $("error-bar"); b.textContent = msg; b.hidden = false; }
function hideError() { $("error-bar").hidden = true; }

// loadAppValues 用当前时间范围拉取 app 标签的值列表,填充应用下拉框(保留首个"不限")。
async function loadAppValues() {
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  try {
    const j = await api("/api/label/app/values?start=" + start + "&end=" + end);
    const sel = $("app-select");
    const blank = sel.querySelector('option[value=""]');
    sel.replaceChildren(blank);
    (j.data || []).forEach((v) => {
      const o = document.createElement("option");
      o.value = v; o.textContent = v;
      sel.appendChild(o);
    });
  } catch (e) { showError(e.message); }
}

// saveFilters 把当前 env、levels、direction 写入 localStorage;失败静默降级(隐私模式等)。
function saveFilters() {
  try {
    const envTab = document.querySelector("#env-tabs .env-tab.active");
    const env = envTab ? envTab.dataset.value : "";
    const levelTabs = document.querySelectorAll("#level-tabs .env-tab.active");
    const levels = Array.from(levelTabs).map(t => t.dataset.value);
    const directionTab = document.querySelector("#direction-tabs .env-tab.active");
    const direction = directionTab ? directionTab.dataset.value : "BACKWARD";
    const filters = { env, levels, direction };
    localStorage.setItem(FILTERS_KEY, JSON.stringify(filters));
  } catch (_) { /* localStorage 不可用时忽略,不阻塞查询 */ }
}

// loadFilters 从 localStorage 恢复 env、level;无记录或解析失败用默认值(env 不限、level=info)。
function loadFilters() {
  let env = "", levels = ["info"], direction = "";
  try {
    const raw = localStorage.getItem(FILTERS_KEY);
    if (raw) {
      const f = JSON.parse(raw);
      if (f && typeof f.env === "string") env = f.env;
      if (f && Array.isArray(f.levels) && f.levels.length > 0) levels = f.levels;
      if (f && typeof f.direction === "string") direction = f.direction;
    }
  } catch (_) { /* 解析失败用默认值 */ }

  // 清除所有选项卡的active类
  document.querySelectorAll(".env-tab").forEach(tab => tab.classList.remove("active"));

  // 设置级别选项卡active状态（支持多选）
  const levelsSet = new Set(levels);
  document.querySelectorAll("#level-tabs .env-tab").forEach(tab => {
    if (levelsSet.has(tab.dataset.value)) {
      tab.classList.add("active");
    }
  });

  // 设置环境选项卡active状态（默认prod）
  const envValue = env || "prod";
  document.querySelectorAll("#env-tabs .env-tab").forEach(tab => {
    if (tab.dataset.value === envValue) {
      tab.classList.add("active");
    }
  });

  // 设置方向选项卡active状态（默认BACKWARD）
  const directionValue = direction || "BACKWARD";
  document.querySelectorAll("#direction-tabs .env-tab").forEach(tab => {
    if (tab.dataset.value === directionValue) {
      tab.classList.add("active");
    }
  });
}

// 级别选项卡点击处理（支持多选）
document.querySelectorAll("#level-tabs .env-tab").forEach((tab) => {
  tab.addEventListener("click", () => {
    tab.classList.toggle("active");
    // 确保至少有一个选中
    const hasActive = document.querySelector("#level-tabs .env-tab.active");
    if (!hasActive) {
      tab.classList.add("active");
    }
    saveFilters();
  });
});

// 环境选项卡点击处理
document.querySelectorAll("#env-tabs .env-tab").forEach((tab) => {
  tab.addEventListener("click", () => {
    document.querySelectorAll("#env-tabs .env-tab").forEach(t => t.classList.remove("active"));
    tab.classList.add("active");
    saveFilters();
  });
});

// 方向选项卡点击处理
document.querySelectorAll("#direction-tabs .env-tab").forEach((tab) => {
  tab.addEventListener("click", () => {
    document.querySelectorAll("#direction-tabs .env-tab").forEach(t => t.classList.remove("active"));
    tab.classList.add("active");
    saveFilters();
  });
});

// setRange 把起止时间设为 [end - mins, end],供快捷按钮和页面默认值共用。
function setRange(mins) {
  const end = unixNow();
  $("start").value = toLocalInput(end - mins * 60);
  $("end").value = toLocalInput(end);
}

document.querySelectorAll(".range-group button").forEach((b) => {
  b.addEventListener("click", () => setRange(parseInt(b.dataset.mins, 10)));
});

$("search").addEventListener("click", search);

async function search() {
  hideError();
  const envTab = document.querySelector("#env-tabs .env-tab.active");
  const env = envTab ? envTab.dataset.value : "";
  const app = $("app-select").value;
  const levelTabs = document.querySelectorAll("#level-tabs .env-tab.active");
  const allLevelTabs = document.querySelectorAll("#level-tabs .env-tab");
  let levels = Array.from(levelTabs).map(t => t.dataset.value);
  // 如果全部选中，去掉level过滤条件
  if (levelTabs.length === allLevelTabs.length) {
    levels = [];
  }
  const chips = [];
  if (env) chips.push({ label: "env", value: env });
  if (app) chips.push({ label: "app", value: app });
  const keyword = $("keyword").value.trim();
  const traceId = $("trace-id").value.trim();
  const query = buildLogQL(chips, keyword, traceId, levels);
  if (!query) { showError("请至少选择环境、应用或级别"); return; }
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  if (!start || !end) { showError("请选择时间范围(可点快捷按钮)"); return; }
  const limit = 1000;
  const directionTab = document.querySelector("#direction-tabs .env-tab.active");
  const direction = directionTab ? directionTab.dataset.value : "BACKWARD";
  const url = "/api/query_range?query=" + encodeURIComponent(query) +
    "&start=" + start + "&end=" + end + "&limit=" + limit + "&direction=" + direction;
  try {
    const j = await api(url);
    renderResults(j);
    renderChart(j.lines || []);
  } catch (e) {
    showError(e.message);
  }
}

function renderResults(res) {
  const box = $("results");
  const lines = res.lines || [];
  box.replaceChildren();
  const meta = document.createElement("div");
  meta.className = "meta";
  meta.textContent = "返回 " + lines.length + " 行(总计 " + res.total + ")";
  box.appendChild(meta);
  lines.forEach((l) => {
    const row = document.createElement("div");
    row.className = "line lvl-" + (l.level || "info");
    const ts = document.createElement("span");
    ts.className = "ts";
    ts.textContent = l.ts;
    const txt = document.createElement("span");
    txt.className = "txt";
    txt.textContent = l.line;
    row.appendChild(ts);
    row.appendChild(txt);
    box.appendChild(row);
  });
}

function renderChart(lines) {
  const levels = ["error", "warn", "info", "debug"];
  const colors = { error: "#ff6b6b", warn: "#ffd166", info: "#69c0ff", debug: "#888" };
  const buckets = {};
  lines.forEach((l) => {
    const d = new Date(l.ts);
    d.setSeconds(0, 0);
    const key = d.getTime();
    const lvl = levels.includes(l.level) ? l.level : "info";
    buckets[key] = buckets[key] || { error: 0, warn: 0, info: 0, debug: 0 };
    buckets[key][lvl]++;
  });
  const keys = Object.keys(buckets).map(Number).sort((a, b) => a - b);
  const labels = keys.map((k) => new Date(k).toLocaleTimeString());
  const datasets = levels.map((lvl) => ({
    label: lvl,
    stack: "s",
    backgroundColor: colors[lvl],
    data: keys.map((k) => buckets[k][lvl]),
  }));
  if (chart) chart.destroy();
  chart = new Chart($("chart"), {
    type: "bar",
    data: { labels, datasets },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: { legend: { labels: { color: "#ddd" } } },
      scales: {
        x: { stacked: true, ticks: { color: "#999" } },
        y: { stacked: true, beginAtZero: true, ticks: { color: "#999" } },
      },
    },
  });
}

setRange(60);       // 默认最近 1 小时
loadAppValues();    // 填充应用下拉(需先有时间范围)
loadFilters();      // 恢复 env、level
loadTheme();        // 恢复主题
