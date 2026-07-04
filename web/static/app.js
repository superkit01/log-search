const $ = (id) => document.getElementById(id);
let chart;
let currentLines = [];
let nextCursor = "";
let lastQueryPayload = null;
let isLoadingMore = false;
const FILTERS_KEY = "log-search:filters"; // localStorage 键:存 {env, levels, direction}
const THEME_KEY = "log-search:theme"; // localStorage 键:存主题
const DEFAULT_LEVELS = LEVEL_ORDER.slice();
const CHART_LEVELS = LEVEL_ORDER.slice().reverse();

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

function toLocalInput(ts) {
  const d = new Date(ts * 1000);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
}

function toUnix(inputVal) {
  if (!inputVal) return 0;
  return Math.floor(new Date(inputVal).getTime() / 1000);
}

async function apiJSON(path, payload) {
  const r = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const j = await r.json();
  if (!r.ok) throw new Error(j.error || ("HTTP " + r.status));
  return j;
}

function showError(msg) { const b = $("error-bar"); b.classList.remove("info"); b.textContent = msg; b.hidden = false; }
function showInfo(msg) { const b = $("error-bar"); b.classList.add("info"); b.textContent = msg; b.hidden = false; }
function hideError() { $("error-bar").hidden = true; }

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

// loadFilters 从 localStorage 恢复 env、level、direction;无记录或解析失败用默认值(env=首个配置项、level 全选、direction=BACKWARD)。
function loadFilters() {
  let env = "", levels = DEFAULT_LEVELS, direction = "";
  try {
    const raw = localStorage.getItem(FILTERS_KEY);
    if (raw) {
      const f = JSON.parse(raw);
      if (f && typeof f.env === "string") env = f.env;
      if (f && Array.isArray(f.levels) && f.levels.length > 0) levels = f.levels;
      if (f && typeof f.direction === "string") direction = f.direction;
    }
  } catch (_) { /* 解析失败用默认值 */ }

  const defaultEnvTab = document.querySelector("#env-tabs .env-tab");
  setActiveLevels(levels);
  setActiveTabValue("env-tabs", env || (defaultEnvTab ? defaultEnvTab.dataset.value : ""));
  setActiveTabValue("direction-tabs", direction || "BACKWARD");
}

function setActiveTabValue(id, value) {
  document.querySelectorAll("#" + id + " .env-tab").forEach(tab => {
    tab.classList.toggle("active", !!value && tab.dataset.value === value);
  });
}

function setActiveLevels(levels) {
  const selected = levels && levels.length > 0 ? new Set(levels) : null;
  document.querySelectorAll("#level-tabs .env-tab").forEach(tab => {
    tab.classList.toggle("active", !selected || selected.has(tab.dataset.value));
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

// setDefaultRange 把起止时间设为默认范围,作为页面初始值。
function setDefaultRange() {
  $("start").value = toLocalInput(defaultStartTs());
  $("end").value = toLocalInput(defaultEndTs());
}

function setQuickRange(range) {
  const ts = quickRangeTimestamps(range);
  $("start").value = toLocalInput(ts.start);
  $("end").value = toLocalInput(ts.end);
}

// 时间输入框不允许清空:用户清空后,在 change 时恢复为当天默认值,避免提交空范围。
["start", "end"].forEach((id) => {
  $(id).addEventListener("change", (e) => {
    if (!e.target.value) {
      e.target.value = toLocalInput(id === "start" ? defaultStartTs() : defaultEndTs());
    }
  });
});

$("search").addEventListener("click", search);
$("share").addEventListener("click", shareCurrentQuery);
$("scroll-area").addEventListener("scroll", onResultsScroll);
document.querySelectorAll(".time-shortcut").forEach((btn) => {
  btn.addEventListener("click", () => setQuickRange(btn.dataset.range));
});

// 带清除图标的字段:有值时显示图标,点击清空并聚焦。
// select 用 change 事件;文本输入用 input 事件,随输入实时显隐。
const clearSyncers = [];
function bindClear(inputId, clearId, evt) {
  const input = $(inputId), clear = $(clearId);
  const sync = () => { clear.hidden = !input.value; };
  input.addEventListener(evt, sync);
  clear.addEventListener("click", () => { input.value = ""; sync(); input.focus(); });
  sync();
  clearSyncers.push(sync);
}
bindClear("app-select", "app-clear", "change");
bindClear("trace-id", "trace-id-clear", "input");
bindClear("keyword", "keyword-clear", "input");

function syncClearButtons() {
  clearSyncers.forEach((sync) => sync());
}

// 文本输入框失焦时裁掉首尾空白(粘贴 traceId 时常见带尾随换行),避免把空格带入查询。
["keyword", "trace-id"].forEach((id) => {
  $(id).addEventListener("blur", (e) => { e.target.value = e.target.value.trim(); });
});

// 在输入框(关键词/链路id/时间)内按回车即查询,与点"查询"按钮等价;
// 仅响应 INPUT(按钮/选项卡走原生激活,避免重复触发)。
document.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && e.target.tagName === "INPUT") search();
});

function activeTabValue(id) {
  const tab = document.querySelector("#" + id + " .env-tab.active");
  return tab ? tab.dataset.value : "";
}

function activeLevelValues() {
  return Array.from(document.querySelectorAll("#level-tabs .env-tab.active")).map(t => t.dataset.value);
}

function queryLevels() {
  const levels = activeLevelValues();
  const allLevelTabs = document.querySelectorAll("#level-tabs .env-tab");
  return levels.length === allLevelTabs.length ? [] : levels;
}

function currentShareState() {
  const levels = queryLevels();
  return {
    env: activeTabValue("env-tabs"),
    app: $("app-select").value,
    levels,
    direction: activeTabValue("direction-tabs") || "BACKWARD",
    keyword: $("keyword").value.trim(),
    traceId: $("trace-id").value.trim(),
    start: toUnix($("start").value),
    end: toUnix($("end").value),
  };
}

function applyShareState(state) {
  if (state.start) $("start").value = toLocalInput(state.start);
  if (state.end) $("end").value = toLocalInput(state.end);
  $("keyword").value = state.keyword || "";
  $("trace-id").value = state.traceId || "";
  setActiveTabValue("env-tabs", state.env || "");
  setActiveLevels(state.levels || []);
  if (state.direction) setActiveTabValue("direction-tabs", state.direction);
}

function setSelectValue(sel, value) {
  if (!value) { sel.value = ""; return; }
  if (!Array.from(sel.options).some((o) => o.value === value)) {
    const o = document.createElement("option");
    o.value = value; o.textContent = value;
    sel.appendChild(o);
  }
  sel.value = value;
}

async function shareCurrentQuery() {
  const qs = encodeShareState(currentShareState());
  const url = window.location.origin + window.location.pathname + (qs ? "?" + qs : "");
  const copied = await copyText(url);
  setShareCopied(copied);
  if (!copied) showInfo("分享链接: " + url);
}

function setShareCopied(copied) {
  const copyIcon = document.querySelector(".share-icon-copy");
  const checkIcon = document.querySelector(".share-icon-check");
  const shareButton = $("share");
  if (!copyIcon || !checkIcon || !shareButton) return;
  copyIcon.style.display = copied ? "none" : "block";
  checkIcon.style.display = copied ? "block" : "none";
  shareButton.title = copied ? "已复制" : "复制分享链接";
  shareButton.setAttribute("aria-label", shareButton.title);
  if (!copied) return;
  setTimeout(() => {
    copyIcon.style.display = "block";
    checkIcon.style.display = "none";
    shareButton.title = "复制分享链接";
    shareButton.setAttribute("aria-label", shareButton.title);
  }, 1600);
}

async function search(options) {
  if (!options || !options.keepMessage) hideError();
  currentLines = [];
  nextCursor = "";
  isLoadingMore = false;
  lastQueryPayload = null;
  const envTab = document.querySelector("#env-tabs .env-tab.active");
  const env = envTab ? envTab.dataset.value : "";
  const app = $("app-select").value;
  const levels = queryLevels();
  const keyword = $("keyword").value.trim();
  const traceId = $("trace-id").value.trim();
  if (!env && !app && levels.length === 0) { showError("请至少选择环境、应用或级别"); return; }
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  if (!start || !end) { showError("请选择时间范围"); return; }
  const direction = activeTabValue("direction-tabs") || "BACKWARD";
  const payload = { env, app, levels, keyword, traceId, start, end, direction };
  lastQueryPayload = payload;
  try {
    const j = await apiJSON("/api/query_range", payload);
    applyQueryResult(j, false);
  } catch (e) {
    showError(e.message);
  }
}

async function loadMore() {
  if (!lastQueryPayload || !nextCursor || isLoadingMore) return;
  hideError();
  isLoadingMore = true;
  try {
    const j = await apiJSON("/api/query_range", { ...lastQueryPayload, cursor: nextCursor });
    applyQueryResult(j, true);
  } catch (e) {
    showError(e.message);
  } finally {
    isLoadingMore = false;
    scheduleAutoLoadCheck();
  }
}

function onResultsScroll(e) {
  if (shouldAutoLoadMore(e.currentTarget, !!nextCursor, isLoadingMore)) {
    loadMore();
  }
}

function scheduleAutoLoadCheck() {
  requestAnimationFrame(() => onResultsScroll({ currentTarget: $("scroll-area") }));
}

function applyQueryResult(res, append) {
  const lines = res.lines || [];
  currentLines = append ? currentLines.concat(lines) : lines.slice();
  nextCursor = res.nextCursor || "";
  renderResults(currentLines, lines, append);
  renderChart(currentLines);
  scheduleAutoLoadCheck();
}

function renderResults(allLines, pageLines, append) {
  const box = $("results");
  if (!append) {
    box.replaceChildren();
  }
  let meta = box.querySelector(".meta");
  if (!meta) {
    meta = document.createElement("div");
    meta.className = "meta";
    box.appendChild(meta);
  }
  meta.textContent = "已加载 " + allLines.length + " 行";
  pageLines.forEach((l) => {
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

    // 添加其他标签：按出现顺序从配色清单(TAG_PALETTE)依次取色,兼容不同部署的标签 key
    if (l.labels) {
      let idx = 0;
      Object.entries(l.labels).forEach(([key, value]) => {
        if (key !== "app" && key !== "level") {
          tags.appendChild(makeCopyableTag(key, value, TAG_PALETTE[idx % TAG_PALETTE.length], l));
          idx++;
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

// toBeijingDateObj 将任意时区的 Date 对象转换为东八区（UTC+8）的 Date 对象。
function toBeijingDateObj(date) {
  const utc = date.getTime() + (date.getTimezoneOffset() * 60000);
  return new Date(utc + (3600000 * 8));
}

// 转换日期为东八区（UTC+8）- 格式：2026-06-28
function toBeijingDate(isoString) {
  return toBeijingDateObj(new Date(isoString)).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).replace(/\//g, '-');
}

// 转换时间为东八区（UTC+8）- 格式：20:05:30.123
function toBeijingTime(isoString) {
  const d = toBeijingDateObj(new Date(isoString));
  const hours = String(d.getHours()).padStart(2, '0');
  const minutes = String(d.getMinutes()).padStart(2, '0');
  const seconds = String(d.getSeconds()).padStart(2, '0');
  const milliseconds = String(d.getMilliseconds()).padStart(3, '0');
  return `${hours}:${minutes}:${seconds}.${milliseconds}`;
}

// makeCopyableTag 构造一个可点击复制的标签:value 写入按钮文本,key 用作 title 提示;
// 点击时把 value 写入剪贴板,短暂显示"已复制"反馈。值转义由 textContent 保证(无 XSS)。
function makeCopyableTag(key, value, extraClass, line) {
  const tag = document.createElement("button");
  tag.type = "button";
  tag.className = "tag" + (extraClass ? " " + extraClass : "");
  tag.title = "点击复制 " + key;
  tag.textContent = key + ": " + value;
  tag.dataset.key = key;
  tag.dataset.value = String(value);
  if (key === "tid") {
    tag.title = "打开链路日志";
    tag.addEventListener("click", () => openTraceDrilldown(line, String(value)));
  } else {
    tag.addEventListener("click", () => copyText(String(value), tag));
  }
  return tag;
}

function openTraceDrilldown(line, traceId) {
  const state = traceDrilldownState(currentShareState(), line, traceId);
  if (!state) return;
  const qs = encodeShareState(state);
  window.open(window.location.origin + window.location.pathname + "?" + qs, "_blank", "noopener");
}

// copyText 把文本写入剪贴板,成功后在目标元素上短暂显示"已复制"反馈。
async function copyText(text, target) {
  let copied = false;
  try {
    await navigator.clipboard.writeText(text);
    copied = true;
  } catch (e) {
    // clipboard API 不可用(非 HTTPS/旧浏览器)时回退到 execCommand
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try { copied = document.execCommand("copy"); } catch (_) {}
    ta.remove();
  }
  if (!target) return copied;
  if (!copied) return false;
  target.classList.add("copied");
  target.textContent = "已复制";
  setTimeout(() => {
    target.classList.remove("copied");
    // 用 data-key/data-value 重建原文,避免在"已复制"回显期间被快速二次点击覆盖错
    target.textContent = target.dataset.key + ": " + target.dataset.value;
  }, 800);
  return true;
}

// 标签配色清单:渲染"其他标签"时按出现位置依次取色、循环使用。
// 不按 key 匹配——不同系统部署的标签 key 各不相同,按位置上色可保证每个标签都有颜色且彼此区分。
const TAG_PALETTE = ["tag-c0", "tag-c1", "tag-c2", "tag-c3", "tag-c4", "tag-c5", "tag-c6"];

function renderChart(lines) {
  const colors = { error: "#ff6b6b", warn: "#ffd166", info: "#69c0ff", debug: "#888" };
  const buckets = {};
  lines.forEach((l) => {
    const d = new Date(l.ts);
    d.setSeconds(0, 0);
    const key = d.getTime();
    const lvl = CHART_LEVELS.includes(l.level) ? l.level : "info";
    buckets[key] = buckets[key] || emptyLevelBucket();
    buckets[key][lvl]++;
  });
  const keys = Object.keys(buckets).map(Number).sort((a, b) => a - b);
  const labels = keys.map((k) => new Date(k).toLocaleTimeString());
  const datasets = CHART_LEVELS.map((lvl) => ({
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

function emptyLevelBucket() {
  return Object.fromEntries(LEVEL_ORDER.map((lvl) => [lvl, 0]));
}

async function init() {
  setDefaultRange();  // 默认当前时间往前1小时整点 ~ 今天 23:59
  loadFilters();      // 恢复 env、level
  loadTheme();        // 恢复主题

  const shared = decodeShareState(window.location.search);
  if (shared) applyShareState(shared);
  if (shared) {
    setSelectValue($("app-select"), shared.app || "");
    syncClearButtons();
    await search({ keepMessage: true });
  }
}

init();
