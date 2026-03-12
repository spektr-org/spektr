// popup.js — Spektr browser extension logic
// MV3 compliant: no inline scripts, WASM via chrome.runtime.getURL

"use strict";

// ── State ─────────────────────────────────────────────────
let wasmReady = false;
let records   = [];
let schema    = null;
let queryMode = "local"; // 'local' | 'ai'
let chartReg  = {};

// ── Boot ──────────────────────────────────────────────────
document.addEventListener("DOMContentLoaded", () => {
  initWasm();

  document.getElementById("queryInput")
    .addEventListener("keydown", e => { if (e.key === "Enter") runQuery(); });

  const drop = document.getElementById("dropZone");
  drop.addEventListener("dragover",  e => { e.preventDefault(); drop.classList.add("drag-over"); });
  drop.addEventListener("dragleave", () => drop.classList.remove("drag-over"));
  drop.addEventListener("drop", e => {
    e.preventDefault(); drop.classList.remove("drag-over");
    const file = e.dataTransfer.files[0];
    if (file) loadCSV(file);
  });

  document.getElementById("csvFile").addEventListener("change", e => {
    const file = e.target.files[0];
    if (file) loadCSV(file);
  });

  document.getElementById("runBtn").addEventListener("click", runQuery);
});

// ── WASM init ─────────────────────────────────────────────
async function initWasm() {
  setStatus("loading", "Loading engine…");
  try {
    const go = new Go(); // from wasm_exec.js
    const wasmUrl = chrome.runtime.getURL("spektr.wasm");
    const result  = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);
    go.run(result.instance);
    await waitForSpektr(10000);
    wasmReady = true;
    setStatus("ready", "Engine ready");
  } catch (err) {
    setStatus("error", "WASM error");
    console.error("[Spektr] WASM init failed:", err);
  }
}

function waitForSpektr(timeout) {
  return new Promise((res, rej) => {
    if (globalThis.__spektr) return res();
    const deadline = Date.now() + timeout;
    const t = setInterval(() => {
      if (globalThis.__spektr) { clearInterval(t); res(); }
      else if (Date.now() > deadline) { clearInterval(t); rej(new Error("Spektr WASM timeout")); }
    }, 50);
  });
}

// ── Spektr API ────────────────────────────────────────────
const S = {
  discover(csv)             { return globalThis.__spektr.discover(csv); },
  parseCSV(csv, sch)        { return globalThis.__spektr.parseCSV(csv, JSON.stringify(sch)); },
  execute(spec, recs, opts) {
    return globalThis.__spektr.execute(
      JSON.stringify(spec),
      JSON.stringify(recs),
      opts ? JSON.stringify(opts) : undefined
    );
  },
  translate(q, sch, summary, key, model) {
    return globalThis.__spektr.translate(
      q, JSON.stringify(sch), JSON.stringify(summary), key, model || undefined
    );
  },
};

// ── Status ────────────────────────────────────────────────
function setStatus(state, text) {
  const pill = document.getElementById("statusPill");
  pill.className = "status-pill " + state;
  document.getElementById("statusText").textContent = text;
}

// ── CSV load ──────────────────────────────────────────────
async function loadCSV(file) {
  if (!wasmReady) { showError("Engine not ready yet."); return; }

  const text = await file.text();

  const discResult = S.discover(text);
  if (!discResult.ok) { showError("Schema discovery failed: " + discResult.error); return; }
  schema = discResult.data;

  const parseResult = S.parseCSV(text, schema);
  if (!parseResult.ok) { showError("CSV parse failed: " + parseResult.error); return; }
  records = parseResult.data;

  document.getElementById("fileBadge").innerHTML =
    `<div class="file-badge">📄 ${esc(file.name)} · ${records.length} rows</div>`;

  renderSchema(schema, records.length);
  buildSuggestions(schema);

  show("schemaPanel");
  show("configPanel");
  show("queryPanel");
  document.getElementById("emptyState").style.display = "none";
  document.getElementById("queryInput").disabled = false;
  document.getElementById("runBtn").disabled     = false;
  document.getElementById("queryInput").focus();
}

// ── Schema display ────────────────────────────────────────
function renderSchema(sch, rowCount) {
  document.getElementById("schemaStats").innerHTML =
    `<div>${rowCount} <span>rows</span></div>
     <div>${sch.dimensions.length} <span>dimensions</span></div>
     <div>${sch.measures.length} <span>measures</span></div>`;

  document.getElementById("dimPills").innerHTML =
    sch.dimensions.map(d =>
      `<span class="schema-pill dim" title="${esc(d.description||d.key)}">${esc(d.key)}</span>`
    ).join("");

  document.getElementById("measPills").innerHTML =
    sch.measures.map(m =>
      `<span class="schema-pill meas" title="${esc(m.description||m.key)}">${esc(m.key)}</span>`
    ).join("");
}

// ── Suggestions ───────────────────────────────────────────
function buildSuggestions(sch) {
  const dims = sch.dimensions.map(d => d.key);
  const sugs = [];
  if (dims[0]) sugs.push(`count records by ${dims[0]}`);
  if (dims[1]) sugs.push(`count records by ${dims[1]}`);
  if (dims[0] && dims[1]) sugs.push(`breakdown ${dims[0]} by ${dims[1]}`);
  if (dims[0]) sugs.push(`show distribution of ${dims[0]}`);
  if (dims[0]) sugs.push(`pie chart by ${dims[0]}`);

  const container = document.getElementById("suggestions");
  container.innerHTML = sugs.map(s =>
    `<button class="sug-chip" data-query="${esc(s)}">${esc(s)}</button>`
  ).join("");

  container.addEventListener("click", e => {
    const chip = e.target.closest(".sug-chip");
    if (chip) {
      document.getElementById("queryInput").value = chip.dataset.query;
      document.getElementById("queryInput").focus();
    }
  });
}

// ── Mode toggle ───────────────────────────────────────────
function setMode(mode) {
  queryMode = mode;
  document.getElementById("btnLocal").classList.toggle("active", mode === "local");
  document.getElementById("btnAI").classList.toggle("active",    mode === "ai");

  const aiFields = document.getElementById("aiFields");
  aiFields.classList.toggle("visible", mode === "ai");

  document.getElementById("modeHint").innerHTML = mode === "local"
    ? `<span style="color:var(--accent4);font-weight:600">Local mode:</span>
       parses "count X by Y" / "breakdown by Z" entirely on-device. Zero API calls.`
    : `<span style="color:var(--accent4);font-weight:600">AI / NL mode:</span>
       translates natural language to queries via your AI provider.
       Your key is used only in this extension and never sent to Spektr.`;
}
window.setMode = setMode;

// ── Query runner ──────────────────────────────────────────
async function runQuery() {
  const q = document.getElementById("queryInput").value.trim();
  if (!q || !wasmReady || !records.length) return;

  const btn = document.getElementById("runBtn");
  const txt = document.getElementById("runBtnText");
  btn.disabled = true;
  txt.innerHTML = '<span class="spinner"></span>';
  clearErrors();

  try {
    let spec;

    if (queryMode === "ai") {
      const key   = document.getElementById("aiApiKey").value.trim();
      const model = document.getElementById("aiModel").value.trim() || undefined;
      if (!key) { showError("Enter an API key to use AI / NL mode."); return; }
      const summary = buildSummary();
      const tr = S.translate(q, schema, summary, key, model);
      if (!tr.ok) { showError("AI translation failed: " + tr.error); return; }
      spec = tr.data.querySpec;
    } else {
      spec = localTranslate(q, schema);
      if (!spec) {
        showError(`Couldn't parse "${q}". Try: "count records by [field]" or "breakdown by [dim]"`);
        return;
      }
    }

    const result = S.execute(spec, records);
    if (!result.ok) { showError("Execution failed: " + result.error); return; }

    renderResult(q, spec, result.data);
    document.getElementById("queryInput").value = "";

  } finally {
    btn.disabled = false;
    txt.textContent = "Run";
  }
}

// ── Local keyword translator ──────────────────────────────
function localTranslate(q, sch) {
  const lower = q.toLowerCase().trim();
  const dims  = sch.dimensions.map(d => d.key);

  function findDim(text) {
    return dims.find(d => text.includes(d.replace(/_/g, " ")) || text.includes(d)) || null;
  }

  let visualize   = "bar";
  if (/pie|share|proportion|split|doughnut/i.test(lower))  visualize = "pie";
  if (/trend|over time|timeline|growth|line/i.test(lower)) visualize = "line";
  if (/table|list|show all|detail/i.test(lower))           visualize = "table";

  let aggregation = "count";
  let measure     = "record_count";
  if (/sum|total/i.test(lower))        aggregation = "sum";
  if (/avg|average|mean/i.test(lower)) aggregation = "avg";

  const meas = sch.measures.map(m => m.key);
  const explicitMeas = meas.find(m => lower.includes(m.replace(/_/g, " ")) || lower.includes(m));
  if (explicitMeas && explicitMeas !== "record_count") measure = explicitMeas;

  const byMatch = lower.match(/\bby\s+(.+?)(?:\s+and\s+|\s*$)/i);
  if (byMatch) {
    const g = findDim(byMatch[1]);
    if (g) return makeSpec({ groupBy: [g], aggregation, measure, visualize, title: q });
  }

  const ofMatch = lower.match(/(?:distribution of|show|count)\s+(.+)/i);
  if (ofMatch) {
    const g = findDim(ofMatch[1]) || dims[0];
    if (g) return makeSpec({ groupBy: [g], aggregation, measure, visualize, title: q });
  }

  if (dims.length) return makeSpec({ groupBy: [dims[0]], aggregation, measure, visualize, title: q });
  return null;
}

function makeSpec({ groupBy, aggregation, measure, visualize, title }) {
  return {
    intent: "chart",
    filters: { dimensions: {} },
    aggregation, measure, groupBy,
    sortBy: "value_desc",
    limit: 20,
    visualize,
    title: title || groupBy.join(" + "),
    reply: `${aggregation} of ${measure} grouped by ${groupBy.join(", ")}.`,
    confidence: 0.85,
  };
}

function buildSummary() {
  const dims = {};
  schema.dimensions.forEach(d => {
    dims[d.key] = [...new Set(records.map(r => r.dimensions[d.key]).filter(Boolean))].slice(0, 8);
  });
  return { recordCount: records.length, dimensions: dims };
}

// ── Render result ─────────────────────────────────────────
function renderResult(query, spec, result) {
  const id       = "r_" + Date.now();
  const canvasId = "c_" + id;
  const container = document.getElementById("results");

  const type  = (result.chartConfig && result.chartConfig.chartType) || "table";
  const badge = { bar:"badge-bar", pie:"badge-pie", line:"badge-line", table:"badge-table" }[type] || "badge-bar";

  const hasChart = result.chartConfig && (result.chartConfig.series||[]).length > 0;
  const hasTable = result.tableData  && (result.tableData.rows||[]).length > 0;
  const gridStyle = (hasChart && hasTable) ? "" : "grid-template-columns:1fr;";

  const card = document.createElement("div");
  card.className = "result-card";
  card.id = id;
  card.innerHTML = `
    <div class="result-header">
      <span class="result-query">▷ ${esc(query)}</span>
      <div class="result-meta">
        <span class="badge ${badge}">${type}</span>
        <button class="dismiss-btn" data-id="${id}" title="Remove">✕</button>
      </div>
    </div>
    <div class="result-body" style="${gridStyle}">
      ${result.reply ? `<div class="result-reply" style="grid-column:1/-1">${esc(result.reply)}</div>` : ""}
      ${hasChart ? `<div class="chart-wrap"><canvas id="${canvasId}"></canvas></div>` : ""}
      ${hasTable ? buildTableHtml(result.tableData) : ""}
    </div>`;

  card.querySelector(".dismiss-btn").addEventListener("click", () => {
    card.remove();
    if (!document.getElementById("results").children.length) {
      document.getElementById("emptyState").style.display = "block";
    }
  });

  container.insertBefore(card, container.firstChild);

  if (hasChart) requestAnimationFrame(() => drawChart(canvasId, result.chartConfig));
}

function buildTableHtml(td) {
  const cols = td.columns || [];
  const rows = td.rows    || [];
  const thead = cols.map(c => `<th>${esc(c.label || c.key)}</th>`).join("");
  const tbody = rows.map(row =>
    `<tr>${row.map((cell, i) => {
      const cls = cols[i]?.align === "right" ? ' class="num"' : "";
      return `<td${cls}>${esc(String(cell ?? ""))}</td>`;
    }).join("")}</tr>`
  ).join("");
  let foot = "";
  if (td.summary) {
    const vals = Object.values(td.summary.values || {});
    foot = `<tfoot><tr class="tfoot-row">
      <td>${esc(td.summary.label || "Total")}</td>
      ${vals.slice(1).map(v => `<td class="num">${esc(String(v))}</td>`).join("")}
    </tr></tfoot>`;
  }
  return `<div class="table-wrap"><table><thead><tr>${thead}</tr></thead><tbody>${tbody}</tbody>${foot}</table></div>`;
}

// ── Chart.js renderer ─────────────────────────────────────
const PALETTE = ["#6c5ce7","#e84393","#00b894","#e67e22","#0984e3","#a29bfe","#fd79a8","#55efc4"];

function drawChart(canvasId, cfg) {
  const canvas = document.getElementById(canvasId);
  if (!canvas) return;
  if (chartReg[canvasId]) chartReg[canvasId].destroy();

  const series = cfg.series || [];
  const labels = (series[0]?.data || []).map(p => p.label);
  let config;

  if (cfg.chartType === "pie") {
    config = {
      type: "doughnut",
      data: {
        labels,
        datasets: [{ data: (series[0]?.data||[]).map(p => p.value),
          backgroundColor: PALETTE, borderWidth: 2, borderColor: "#ffffff" }]
      },
      options: {
        responsive: true, maintainAspectRatio: false,
        plugins: {
          legend: { position: "right", labels: { color: "#44445a", font: { size: 10 }, boxWidth: 10, padding: 10 } },
          title:  { display: !!cfg.title, text: cfg.title, color: "#1a1a2e", font: { size: 12, weight: "700" } }
        }
      }
    };
  } else if (cfg.chartType === "line") {
    config = {
      type: "line",
      data: {
        labels,
        datasets: series.map((s, i) => ({
          label: s.name,
          data: s.data.map(p => p.value),
          borderColor: PALETTE[i % PALETTE.length],
          backgroundColor: PALETTE[i % PALETTE.length] + "18",
          borderWidth: 2, pointRadius: 3, tension: 0.35, fill: i === 0,
        }))
      },
      options: scaleOptions(cfg)
    };
  } else {
    config = {
      type: "bar",
      data: {
        labels,
        datasets: series.map((s, i) => ({
          label: s.name,
          data: s.data.map(p => p.value),
          backgroundColor: PALETTE[i % PALETTE.length] + "cc",
          borderColor: PALETTE[i % PALETTE.length],
          borderWidth: 1, borderRadius: 5,
        }))
      },
      options: scaleOptions(cfg)
    };
  }

  chartReg[canvasId] = new Chart(canvas, config);
}

function scaleOptions(cfg) {
  return {
    responsive: true, maintainAspectRatio: false,
    plugins: {
      legend: { display: (cfg.series||[]).length > 1, labels: { color: "#44445a", font: { size: 10 } } },
      title:  { display: !!cfg.title, text: cfg.title, color: "#1a1a2e", font: { size: 12, weight: "700" } }
    },
    scales: {
      x: { ticks: { color: "#8888a0", font: { size: 9 }, maxRotation: 30 }, grid: { color: "#e2e2ea" } },
      y: { ticks: { color: "#8888a0", font: { size: 9 } },                  grid: { color: "#e2e2ea" } }
    }
  };
}

// ── Helpers ───────────────────────────────────────────────
function show(id)       { document.getElementById(id).style.display = "block"; }
function esc(s)         { return String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;"); }
function showError(msg) {
  const d = document.createElement("div");
  d.className = "error-card";
  d.textContent = "⚠ " + msg;
  document.getElementById("results").prepend(d);
  setTimeout(() => d.remove(), 6000);
}
function clearErrors() { document.querySelectorAll(".error-card").forEach(e => e.remove()); }