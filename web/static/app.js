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

// 文本输入框失焦时裁掉首尾空白(粘贴 traceId 时常见带尾随换行),避免把空格带入查询。
["keyword", "trace-id"].forEach((id) => {
  $(id).addEventListener("blur", (e) => { e.target.value = e.target.value.trim(); });
});

// 在输入框(关键词/链路id/时间)内按回车即查询,与点"查询"按钮等价;
// 仅响应 INPUT(按钮/选项卡走原生激活,避免重复触发)。
document.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && e.target.tagName === "INPUT") search();
});

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

    // 时间戳（东八区）分成两行显示
    const ts = document.createElement("span");
    ts.className = "ts";
    const tsDate = document.createElement("div");
    tsDate.className = "ts-date";
    tsDate.textContent = toBeijingDate(l.ts);
    const tsTime = document.createElement("div");
    tsTime.className = "ts-time";
    tsTime.textContent = toBeijingTime(l.ts);
    ts.appendChild(tsDate);
    ts.appendChild(tsTime);

    // 内容区域（标签 + 日志文本）
    const content = document.createElement("div");
    content.className = "content";

    // 标签区域
    const tags = document.createElement("div");
    tags.className = "tags";

    // 添加 app 标签
    if (l.labels && l.labels.app) {
      tags.appendChild(makeCopyableTag("app", l.labels.app, "tag-app"));
    }

    // 添加 level 标签
    tags.appendChild(makeCopyableTag("level", l.level || "info", "tag-level"));

    // 添加其他标签（根据类型添加不同的CSS类）
    if (l.labels) {
      Object.entries(l.labels).forEach(([key, value]) => {
        if (key !== "app" && key !== "level") {
          tags.appendChild(makeCopyableTag(key, value, getTagClass(key)));
        }
      });
    }

    // 日志文本（支持折叠/展开）
    const txt = document.createElement("div");
    txt.className = "txt";

    const lines = l.line.split('\n');
    if (lines.length > 10) {
      // 创建折叠容器
      const txtWrapper = document.createElement("div");
      txtWrapper.className = "txt-wrapper collapsed";

      // 显示前10行
      const previewLines = lines.slice(0, 10);
      const previewText = document.createElement("div");
      previewText.className = "txt-preview";
      previewText.textContent = previewLines.join('\n');
      txtWrapper.appendChild(previewText);

      // 隐藏的完整内容
      const fullText = document.createElement("div");
      fullText.className = "txt-full";
      fullText.textContent = l.line;
      txtWrapper.appendChild(fullText);

      // 展开/折叠按钮
      const toggleBtn = document.createElement("button");
      toggleBtn.className = "txt-toggle";
      toggleBtn.textContent = "展开 >>";
      toggleBtn.onclick = () => {
        txtWrapper.classList.toggle("collapsed");
        toggleBtn.textContent = txtWrapper.classList.contains("collapsed") ? "展开 >>" : "<< 折叠";
      };

      txt.appendChild(txtWrapper);
      txt.appendChild(toggleBtn);
    } else {
      txt.textContent = l.line;
    }

    content.appendChild(tags);
    content.appendChild(txt);
    row.appendChild(ts);
    row.appendChild(content);
    box.appendChild(row);
  });
}

// 转换日期为东八区（UTC+8）- 格式：2026-06-28
function toBeijingDate(isoString) {
  const d = new Date(isoString);
  const utc = d.getTime() + (d.getTimezoneOffset() * 60000);
  const beijingTime = new Date(utc + (3600000 * 8));
  return beijingTime.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).replace(/\//g, '-');
}

// 转换时间为东八区（UTC+8）- 格式：20:05:30.123
function toBeijingTime(isoString) {
  const d = new Date(isoString);
  const utc = d.getTime() + (d.getTimezoneOffset() * 60000);
  const beijingTime = new Date(utc + (3600000 * 8));

  const hours = String(beijingTime.getHours()).padStart(2, '0');
  const minutes = String(beijingTime.getMinutes()).padStart(2, '0');
  const seconds = String(beijingTime.getSeconds()).padStart(2, '0');
  const milliseconds = String(beijingTime.getMilliseconds()).padStart(3, '0');

  return `${hours}:${minutes}:${seconds}.${milliseconds}`;
}

// makeCopyableTag 构造一个可点击复制的标签:value 写入按钮文本,key 用作 title 提示;
// 点击时把 value 写入剪贴板,短暂显示"已复制"反馈。值转义由 textContent 保证(无 XSS)。
function makeCopyableTag(key, value, extraClass) {
  const tag = document.createElement("button");
  tag.type = "button";
  tag.className = "tag" + (extraClass ? " " + extraClass : "");
  tag.title = "点击复制 " + key;
  tag.textContent = key + ": " + value;
  tag.dataset.key = key;
  tag.dataset.value = String(value);
  tag.addEventListener("click", () => copyText(String(value), tag));
  return tag;
}

// copyText 把文本写入剪贴板,成功后在目标元素上短暂显示"已复制"反馈。
async function copyText(text, target) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (e) {
    // clipboard API 不可用(非 HTTPS/旧浏览器)时回退到 execCommand
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); } catch (_) {}
    ta.remove();
  }
  if (!target) return;
  target.classList.add("copied");
  target.textContent = "已复制";
  setTimeout(() => {
    target.classList.remove("copied");
    // 用 data-key/data-value 重建原文,避免在"已复制"回显期间被快速二次点击覆盖错
    target.textContent = target.dataset.key + ": " + target.dataset.value;
  }, 800);
}

// 根据标签key返回对应的CSS类
function getTagClass(key) {
  const tagMap = {
    'env': 'tag-env',
    'region': 'tag-region',
    'instance': 'tag-instance',
    'host': 'tag-host',
    'job': 'tag-instance',
    'service': 'tag-app'
  };
  return tagMap[key] || '';
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
