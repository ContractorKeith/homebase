// Network quality on the machine cards: the latest probe results and speed test from
// /api/net, a "Run speed test" button and a 24h / 7d latency chart.
import { DASH, esc, known } from "./fmt.js";
import { css } from "./charts.js";

const latest = new Map(); // machine id -> { gateway, internet, speed }
const TARGETS = [["gateway", "Gateway"], ["internet", "Internet"]];
const num = (v, unit, d = 1) => (known(v) ? `${v.toFixed(d)}${unit}` : DASH);

function age(ts) {
  const s = (Date.now() - new Date(ts).getTime()) / 1000;
  return s < 90 ? "just now" : s < 5400 ? `${Math.round(s / 60)}m ago` : s < 129600 ? `${Math.round(s / 3600)}h ago` : `${Math.round(s / 86400)}d ago`;
}

function probe(label, n) {
  if (!n) return `<div class="row"><span>${label}</span><b>${DASH}</b></div>`;
  return `<div class="row"><span>${label} <span class="sub">${age(n.ts)}</span></span>
    <b>${num(n.rttMs, " ms")} · jitter ${num(n.jitterMs, " ms")} · loss ${num(n.lossPct, "%", 0)}</b></div>`;
}

// netqHTML is the inside of a card's network slot for machine id.
export function netqHTML(id) {
  const q = latest.get(id) ?? {};
  const sp = q.speed;
  const speed = sp ? `↓ ${num(sp.downMbps, "", 0)} / ↑ ${num(sp.upMbps, "", 0)} Mbps <span class="sub">${age(sp.ts)}</span>` : DASH;
  return `<button type="button" class="linkrow" data-act="netchart" data-id="${esc(id)}" aria-label="Open latency history">
      ${TARGETS.map(([k, l]) => probe(l, q[k])).join("")}</button>
    <div class="row"><span>Speed test</span><b>${speed}</b></div>
    <div class="row"><button type="button" class="mini" data-act="speedtest" data-id="${esc(id)}">Run speed test</button><span class="sub" data-note></span></div>`;
}

function paint() {
  document.querySelectorAll(".slot-netq[data-id]").forEach((el) => {
    const note = el.querySelector("[data-note]")?.textContent ?? "";
    el.innerHTML = netqHTML(el.dataset.id);
    el.querySelector("[data-note]").textContent = note;
  });
}

async function poll() {
  try {
    const r = await fetch("/api/net?range=24h");
    if (!r.ok) return;
    const d = await r.json();
    latest.clear();
    for (const n of d.latest) {
      const q = latest.get(n.machine) ?? {};
      q[n.target === "gateway" ? "gateway" : "internet"] = n;
      latest.set(n.machine, q);
    }
    for (const s of d.speed) latest.set(s.machine, { ...(latest.get(s.machine) ?? {}), speed: s });
    paint();
  } catch { /* the hub is unreachable; the connection badge already says so */ }
}
poll();
setInterval(poll, 30000);

async function speedtest(btn) {
  const note = btn.parentElement.querySelector("[data-note]");
  btn.disabled = true;
  try {
    const r = await fetch(`/api/speedtest/${encodeURIComponent(btn.dataset.id)}`, { method: "POST" });
    note.textContent = r.ok ? "queued: result in about a minute" : r.status === 404 ? "not available yet" : `failed (${r.status})`;
    if (r.ok) setTimeout(poll, 60000);
  } catch { note.textContent = "failed"; }
  setTimeout(() => { btn.disabled = false; }, 5000);
}

// --- latency dialog ---
const dlg = document.getElementById("netdlg");
const body = document.getElementById("netdlg-body");
const rangeBar = dlg.querySelector(".ranges");
let plot = null, cur = { id: "", name: "", range: "24h" };

async function loadChart() {
  plot?.destroy();
  plot = null;
  body.innerHTML = `<p class="sub">loading…</p>`;
  rangeBar.querySelectorAll("button").forEach((b) => b.setAttribute("aria-selected", String(b.dataset.range === cur.range)));
  let d;
  try {
    const r = await fetch(`/api/net?range=${cur.range}&machine=${encodeURIComponent(cur.id)}`);
    if (!r.ok) throw new Error(`${r.status}`);
    d = await r.json();
  } catch (e) { body.innerHTML = `<p class="sub">Latency unavailable (${esc(e.message)}).</p>`; return; }
  const by = Object.fromEntries(d.series.map((s) => [s.target === "gateway" ? "gateway" : "internet", s.points]));
  const ts = [...new Set(Object.values(by).flat().map((p) => p.t))].sort((a, b) => a - b);
  if (!ts.length) { body.innerHTML = `<p class="none">${DASH} no latency samples in this range</p>`; return; }
  const col = (pts) => { const m = new Map((pts ?? []).map((p) => [p.t, p.rttMs])); return ts.map((t) => m.get(t) ?? null); };
  body.innerHTML = `<h4>Round-trip time (ms)</h4><div></div>`;
  const grid = { stroke: css("--line"), width: 1 }, axis = { stroke: css("--muted"), grid, ticks: { show: false } };
  const host = body.lastChild;
  plot = new uPlot({
    width: Math.max(300, host.clientWidth || 800), height: 200, cursor: { points: { size: 8 } },
    scales: { y: { range: (_u, lo, hi) => [0, Math.max(hi * 1.15, 1)] } },
    axes: [axis, { ...axis, size: 48 }],
    series: [{}, ...TARGETS.map(([k, label], i) => ({ label, stroke: css(i ? "--series-2" : "--series-1"), width: 2, spanGaps: false, points: { show: false },
      value: (_u, v) => (v == null ? DASH : `${v.toFixed(1)} ms`) }))],
  }, [ts.map((t) => t / 1000), col(by.gateway), col(by.internet)], host);
}

["24h", "7d"].forEach((r) => {
  const b = document.createElement("button");
  b.textContent = r;
  b.dataset.range = r;
  b.addEventListener("click", () => { cur.range = r; loadChart(); });
  rangeBar.append(b);
});
dlg.querySelector(".close").addEventListener("click", () => dlg.close());
dlg.addEventListener("click", (e) => { if (e.target === dlg) dlg.close(); });
dlg.addEventListener("close", () => { plot?.destroy(); plot = null; });

// Card clicks: the network row opens the chart, the button queues a test.
document.getElementById("cards").addEventListener("click", (e) => {
  const el = e.target.closest("[data-act]");
  if (!el) return;
  if (el.dataset.act === "speedtest") return void speedtest(el);
  const card = el.closest(".card");
  cur = { id: el.dataset.id, name: card.querySelector("h3").textContent, range: "24h" };
  document.getElementById("netdlg-title").textContent = `${cur.name} network`;
  dlg.showModal();
  loadChart();
});
