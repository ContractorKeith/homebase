// KPI tiles with the change against the previous period of the same length.
import { DASH, esc } from "./fmt.js";

// delta describes cur vs prev as arrow + percent; no prior data reads "new" or "—".
export function delta(cur, prev) {
  if (!prev) return cur ? { arrow: "▲", text: "new", label: "no data in the previous period" } : { arrow: "", text: DASH, label: "no change data" };
  const p = ((cur - prev) / prev) * 100;
  if (Math.abs(p) < 0.5) return { arrow: "▬", text: "0%", label: "unchanged vs previous period" };
  const up = p > 0;
  const t = `${Math.abs(p) < 10 ? Math.abs(p).toFixed(1) : Math.round(Math.abs(p))}%`;
  return { arrow: up ? "▲" : "▼", text: t, label: `${up ? "up" : "down"} ${t} vs previous period` };
}

// tile renders one KPI. value/sub are display strings; cur/prev feed the change line.
export function tile(label, value, cur, prev, sub = "") {
  const d = delta(cur, prev);
  return `<div class="kpi"><div class="kpi-label">${esc(label)}</div><div class="kpi-value${value.length > 10 ? " long" : ""}">${esc(value)}</div>
    <div class="kpi-delta" aria-label="${esc(d.label)}"><span aria-hidden="true">${d.arrow}</span> ${esc(d.text)} <span class="sub">vs previous</span></div>
    ${sub ? `<div class="sub">${esc(sub)}</div>` : ""}</div>`;
}
