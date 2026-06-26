const $ = (id) => document.getElementById(id);
let chart;
let chips = []; // 已选标签条件:[{label, value}],label 唯一

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

async function loadLabels() {
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  try {
    const j = await api("/api/labels?start=" + start + "&end=" + end);
    const sel = $("label-select");
    (j.data || []).forEach((l) => {
      const o = document.createElement("option");
      o.value = l; o.textContent = l;
      sel.appendChild(o);
    });
  } catch (e) { showError(e.message); }
}

$("label-select").addEventListener("change", async (e) => {
  const name = e.target.value;
  if (!name) return;
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  try {
    const j = await api("/api/label/" + encodeURIComponent(name) + "/values?start=" + start + "&end=" + end);
    const sel = $("value-select");
    sel.disabled = false;
    sel.innerHTML = "";
    (j.data || []).forEach((v) => {
      const o = document.createElement("option");
      o.value = v; o.textContent = v;
      sel.appendChild(o);
    });
  } catch (err) { showError(err.message); }
});

// 添加标签条件 chip:label 唯一,重复添加则替换值。
$("add-chip").addEventListener("click", () => {
  const label = $("label-select").value;
  const value = $("value-select").value;
  if (!label || !value) { showError("请先选择标签和标签值"); return; }
  hideError();
  const i = chips.findIndex((c) => c.label === label);
  if (i >= 0) chips[i].value = value;
  else chips.push({ label, value });
  renderChips();
});

function renderChips() {
  const box = $("chips");
  box.replaceChildren();
  chips.forEach((c, i) => {
    const chip = document.createElement("span");
    chip.className = "chip";
    chip.textContent = c.label + '="' + c.value + '"';
    const x = document.createElement("span");
    x.className = "chip-x";
    x.textContent = "×";
    x.addEventListener("click", () => { chips.splice(i, 1); renderChips(); });
    chip.appendChild(x);
    box.appendChild(chip);
  });
}

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
  const keyword = $("keyword").value.trim();
  const traceId = $("trace-id").value.trim();
  const query = buildLogQL(chips, keyword, traceId);
  if (!query) { showError("请先添加至少一个标签条件"); return; }
  const start = toUnix($("start").value);
  const end = toUnix($("end").value);
  if (!start || !end) { showError("请选择时间范围(可点快捷按钮)"); return; }
  const limit = $("limit").value || 1000;
  const direction = $("direction").value;
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
  box.innerHTML = "";
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

setRange(60); // 默认最近 1 小时
loadLabels();
