// Tokens tab: KPI tiles, a stacked bar chart by agent / model / machine, and a table.
import { barChart, seriesColor } from "./charts.js";
import { count, esc, int } from "./fmt.js";
import { tile } from "./kpi.js";
import { getRange, onRange, timeZone } from "./range.js";
import { loader, segmented } from "./view.js";

const TYPES = [["total", "Total"], ["input", "Input"], ["output", "Output"], ["cacheRead", "Cache read"], ["cacheWrite", "Cache write"], ["reasoning", "Reasoning"]];
const panel = document.getElementById("panel-tokens");
const bar = panel.querySelector(".toolbar");
const body = panel.querySelector(".view-body");
const load = loader(body);
const state = { by: "agent", type: "total" };
let data = null, plot = null;

bar.append(
  segmented("Group by", [["agent", "Agent"], ["model", "Model"], ["machine", "Machine"]], state.by, (k) => { state.by = k; refresh(); }),
  segmented("Token type", TYPES, state.type, (k) => { state.type = k; if (data) draw(); }),
);

export function refresh() {
  load(`/api/tokens?range=${getRange()}&by=${state.by}&tz=${encodeURIComponent(timeZone())}`, (d) => { data = d; draw(); });
}
onRange(() => { if (!panel.hidden) refresh(); });

function table(d) {
  const grand = d.totals.total || 1;
  const rows = d.series.map((s) => {
    const sum = (k) => s[k].reduce((a, b) => a + b, 0);
    return `<tr><th scope="row"><i class="sw" style="background:${seriesColor(s.slot)}"></i>${esc(s.name)}</th>
      <td>${int(sum("total"))}</td><td>${int(sum("input"))}</td><td>${int(sum("output"))}</td><td>${int(sum("cacheRead"))}</td>
      <td>${int(sum("cacheWrite"))}</td><td>${int(sum("reasoning"))}</td><td>${Math.round((sum("total") / grand) * 100)}%</td></tr>`;
  }).join("");
  return `<table class="data"><thead><tr><th scope="col">${esc(state.by)}</th><th>Total</th><th>Input</th><th>Output</th><th>Cache read</th><th>Cache write</th><th>Reasoning</th><th>Share</th></tr></thead><tbody>${rows}</tbody></table>
    <p class="sub">Total = input + output + cache read + cache write. Reasoning is already inside output.</p>`;
}

function draw() {
  plot?.destroy();
  plot = null;
  const d = data, t = d.totals, p = d.prevTotals;
  if (!d.anyData) {
    body.innerHTML = `<p class="empty">No token data yet: start <code>homebase agent</code> on each machine</p>`;
    return;
  }
  const tiles = [["Total tokens", "total"], ["Input", "input"], ["Output", "output"], ["Cache read", "cacheRead"], ["Cache write", "cacheWrite"]]
    .map(([l, k]) => tile(l, count(t[k]), t[k], p[k], k === "output" && t.reasoning ? `${count(t.reasoning)} reasoning` : "")).join("");
  const typeName = TYPES.find(([k]) => k === state.type)[1];
  body.innerHTML = `<div class="kpis">${tiles}</div>
    <div class="panel"><h3>${typeName} tokens by ${esc(state.by)}</h3>
      ${d.series.length ? `<div class="plot" id="tokens-plot"></div>` : `<p class="empty">No token use in this range.</p>`}</div>
    ${d.series.length ? `<div class="panel">${table(d)}</div>` : ""}`;
  if (!d.series.length) return;
  plot = barChart(document.getElementById("tokens-plot"), {
    labels: d.buckets.map((b) => b.label), stacked: true,
    series: d.series.map((s) => ({ name: s.name, color: seriesColor(s.slot), values: s[state.type] })),
  });
}
