// uPlot bar charts on an ordinal x axis (one slot per range bucket). Colors come from CSS
// variables so light and dark follow the page.
import { count } from "./fmt.js";

export const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
export const seriesColor = (slot) => css(slot >= 1 && slot <= 8 ? `--series-${slot}` : "--muted");

// barChart draws series into host. opts: labels (one per bucket), series [{name, color,
// values}], stacked (cumulative bars, bottom series first), fmt (value formatter),
// height. Returns the plot; call destroy() before drawing again.
export function barChart(host, { labels, series, stacked = false, fmt = count, height = 220, signed = false }) {
  const n = labels.length;
  const xs = labels.map((_, i) => i);
  const real = series.map((s) => s.values);
  let data = real;
  if (stacked) {
    const run = new Array(n).fill(0);
    data = real.map((vals) => vals.map((v, i) => (run[i] += v)));
  }
  // Stacked bars draw the tallest cumulative first so the shorter ones sit on top of it.
  const order = stacked ? series.map((_, i) => series.length - 1 - i) : series.map((_, i) => i);
  const surface = css("--surface");
  const muted = css("--muted"), grid = { stroke: css("--line"), width: 1 };
  const step = Math.max(1, Math.ceil(n / Math.max(2, Math.floor(host.clientWidth / 70 || 8))));
  const opts = {
    width: Math.max(300, host.clientWidth || 800), height,
    cursor: { x: false, y: false, points: { show: false } },
    legend: { live: true, markers: { width: 0, fill: (u, i) => u.series[i].fill() } },
    scales: { x: { time: false, range: () => [-0.5, n - 0.5] }, y: { range: (_u, lo, hi) => [signed ? Math.min(0, lo * 1.1) : 0, Math.max(hi * 1.1, 1)] } },
    axes: [
      { stroke: muted, grid: { show: false }, ticks: { show: false }, splits: () => xs.filter((i) => i % step === 0), values: (_u, v) => v.map((i) => labels[i] ?? "") },
      { stroke: muted, grid, ticks: { show: false }, size: 56, values: (_u, v) => v.map((x) => fmt(Math.abs(x))) },
    ],
    series: [{ label: "Period", value: (_u, _v, _s, idx) => (idx == null ? "—" : labels[idx]) },
      ...order.map((i) => ({
        label: series[i].name, fill: series[i].color, stroke: stacked ? surface : series[i].color, width: stacked ? 2 : 0, points: { show: false },
        paths: uPlot.paths.bars({ size: [signed ? 0.8 : 0.7, 64], radius: stacked ? 0 : 0.2, gap: stacked ? 0 : 2 }),
        value: (_u, _v, _s, idx) => (idx == null ? "—" : fmt(Math.abs(real[i][idx]))),
      }))],
  };
  const u = new uPlot(opts, [xs, ...order.map((i) => data[i])], host);
  const ro = new ResizeObserver(() => u.setSize({ width: Math.max(300, host.clientWidth), height }));
  ro.observe(host);
  const destroy = u.destroy.bind(u);
  u.destroy = () => { ro.disconnect(); destroy(); };
  return u;
}
