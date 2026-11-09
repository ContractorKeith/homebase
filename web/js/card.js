// One machine card, rendered from a /api/machines item. Re-rendered in place on each update.
import { bytes, DASH, esc, known, mhz, pct, rate, rpm, temp, uptime, watts } from "./fmt.js";
import { netqHTML } from "./netq.js";

const SPARK_POINTS = 90;
const sparks = new Map(); // machine id -> recent CPU totals

function pushSpark(m) {
  const a = sparks.get(m.id) ?? [];
  a.push(known(m.cpu.usage) ? m.cpu.usage : null);
  if (a.length > SPARK_POINTS) a.shift();
  sparks.set(m.id, a);
  return a;
}

// sparkline draws CPU total as one SVG path; null breaks the line.
function sparkline(vals) {
  const w = 300, h = 40;
  let d = "", pen = false;
  vals.forEach((v, i) => {
    if (v === null) { pen = false; return; }
    const x = ((i + SPARK_POINTS - vals.length) / (SPARK_POINTS - 1)) * w, y = h - 2 - (Math.min(100, v) / 100) * (h - 4);
    d += `${pen ? "L" : "M"}${x.toFixed(1)} ${y.toFixed(1)}`;
    pen = true;
  });
  return `<svg class="spark" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" role="img" aria-label="CPU total, recent"><path d="${d}" vector-effect="non-scaling-stroke"/></svg>`;
}

function cores(list) {
  if (!list.length) return `<div class="sub">per-core: ${DASH}</div>`;
  const cells = list.map((v, i) => {
    if (!known(v)) return `<i title="core ${i}: ${DASH}"></i>`;
    const p = Math.max(0, Math.min(100, v));
    return `<i title="core ${i}: ${Math.round(p)}%" style="background:color-mix(in oklab, var(--blue) ${Math.round(8 + p * 0.92)}%, var(--cell-off))"></i>`;
  });
  return `<div class="cores" role="img" aria-label="Per-core CPU load">${cells.join("")}</div>`;
}

function gpu(g, i, many) {
  const label = many ? `GPU ${i + 1}` : "GPU";
  const memPct = known(g.memUsed) && known(g.memTotal) && g.memTotal > 0 ? (g.memUsed / g.memTotal) * 100 : null;
  return `<div class="row"><span>${esc(label)}${g.name ? ` <span class="sub">${esc(g.name)}</span>` : ""}</span><b>${pct(g.usage)} · ${temp(g.temp)} · ${watts(g.power)}</b></div>
    <div class="bar2" role="img" aria-label="VRAM used"><i style="width:${known(memPct) ? memPct.toFixed(0) : 0}%"></i></div>
    <div class="row"><span>VRAM</span><b>${bytes(g.memUsed)} / ${bytes(g.memTotal)}</b></div>`;
}

export function renderCard(el, m) {
  const history = pushSpark(m);
  el.classList.toggle("off", !m.online);
  el.setAttribute("aria-label", `${m.name}: open history`);
  const sensors = [...m.temps.filter((t) => known(t.value)).map((t) => `${esc(t.name ?? "sensor")} ${temp(t.value)}`),
    ...m.fans.map((f) => `${esc(f.name ?? "fan")} ${rpm(f.value)}`)];
  // The network slot is built once and repainted by netq.js, so its buttons keep focus.
  if (!el.querySelector(":scope > .top")) {
    el.innerHTML = `<div class="top"></div><div class="slot-netq" data-slot="netq" data-id="${esc(m.id)}">${netqHTML(m.id)}</div>`;
  }
  el.querySelector(":scope > .top").innerHTML = `
    <div class="head"><h3>${esc(m.name)}</h3><span class="state ${m.online ? "on" : ""}">${m.online ? "online" : "offline"}</span>
      <span class="up" title="uptime">up ${uptime(m.uptime)}</span></div>
    <div class="body" style="display:flex;flex-direction:column;gap:12px">
      <div class="row"><span>CPU</span><b class="big">${pct(m.cpu.usage)}</b></div>
      ${sparkline(history)}
      ${cores(m.cpu.cores)}
      <div class="row"><span>${m.cpu.threads ? `${m.cpu.threads} threads · ` : ""}${mhz(m.cpu.clockMHz)}</span><b>${temp(m.cpu.temp)}</b></div>
      ${m.gpus.length ? m.gpus.map((g, i) => gpu(g, i, m.gpus.length > 1)).join("") : `<div class="row"><span>GPU</span><b>${DASH}</b></div>`}
      <div class="row"><span>Fans &amp; sensors</span><span class="sub">${sensors.length ? sensors.join(" · ") : DASH}</span></div>
      <div class="row"><span>Net ↓ / ↑</span><b>${rate(m.net.rx)} / ${rate(m.net.tx)}</b></div>
    </div>`;
}
