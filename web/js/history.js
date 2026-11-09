// History dialog: CPU, GPU and temperature charts for 1h / 24h / 7d, fetched from the hub.
const RANGES = ["1h", "24h", "7d"];
const dlg = document.getElementById("history");
const body = document.getElementById("history-body");
const title = document.getElementById("history-title");
const rangeBar = dlg.querySelector(".ranges");
let plots = [];
let current = { id: null, name: "", range: "1h" };

const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

function chart(heading, unit, series, points) {
  const wrap = document.createElement("div");
  wrap.className = "chart";
  wrap.innerHTML = `<h4>${heading}</h4>`;
  body.append(wrap);
  const usable = series.filter((s) => points.some((p) => p[s.key] != null));
  if (!usable.length) {
    wrap.insertAdjacentHTML("beforeend", `<div class="none">— no data in this range</div>`);
    return;
  }
  const data = [points.map((p) => p.t / 1000), ...usable.map((s) => points.map((p) => p[s.key] ?? null))];
  const grid = { stroke: css("--line"), width: 1 };
  const axis = { stroke: css("--muted"), grid, ticks: { show: false } };
  const holder = document.createElement("div");
  wrap.append(holder);
  plots.push(new uPlot({
    width: Math.max(300, holder.clientWidth || body.clientWidth || 800), height: 160,
    legend: { live: true },
    cursor: { points: { size: 8 } },
    scales: { y: unit === "%" ? { range: [0, 100] } : { range: (_u, lo, hi) => [Math.max(0, lo - 5), hi + 5] } },
    axes: [axis, { ...axis, size: 48, values: (_u, v) => v.map((x) => `${x}${unit}`) }],
    series: [{}, ...usable.map((s) => ({ label: s.label, stroke: css(s.color), width: 2, spanGaps: false, points: { show: false },
      value: (_u, v) => (v == null ? "—" : `${v.toFixed(1)}${unit}`) }))],
  }, data, holder));
}

async function load() {
  plots.forEach((p) => p.destroy());
  plots = [];
  body.innerHTML = `<p class="sub">loading…</p>`;
  rangeBar.querySelectorAll("button").forEach((b) => b.setAttribute("aria-selected", String(b.dataset.range === current.range)));
  let h;
  try {
    const r = await fetch(`/api/machines/${encodeURIComponent(current.id)}/history?range=${current.range}`);
    if (!r.ok) throw new Error(`${r.status}`);
    h = await r.json();
  } catch (e) {
    body.innerHTML = `<p class="sub">History unavailable (${e.message}).</p>`;
    return;
  }
  body.innerHTML = "";
  chart("CPU load", "%", [{ key: "cpu", label: "CPU", color: "--blue" }], h.points);
  chart("GPU load", "%", [{ key: "gpu", label: "GPU", color: "--orange" }], h.points);
  chart("Temperature", "°C", [{ key: "cpuTemp", label: "CPU", color: "--blue" }, { key: "gpuTemp", label: "GPU", color: "--orange" }], h.points);
}

RANGES.forEach((r) => {
  const b = document.createElement("button");
  b.textContent = r;
  b.dataset.range = r;
  b.addEventListener("click", () => { current.range = r; load(); });
  rangeBar.append(b);
});
dlg.querySelector(".close").addEventListener("click", () => dlg.close());
dlg.addEventListener("click", (e) => { if (e.target === dlg) dlg.close(); });
dlg.addEventListener("close", () => { plots.forEach((p) => p.destroy()); plots = []; });

export function openHistory(m) {
  current = { id: m.id, name: m.name, range: "1h" };
  title.textContent = `${m.name} history`;
  dlg.showModal();
  load();
}
