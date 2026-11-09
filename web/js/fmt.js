// Formatters. Unknown (null/undefined/NaN) always shows as "—".
const DASH = "—";
const known = (v) => v !== null && v !== undefined && Number.isFinite(v);

export const pct = (v) => (known(v) ? `${Math.round(v)}%` : DASH);
export const temp = (v) => (known(v) ? `${Math.round(v)}°C` : DASH);
export const watts = (v) => (known(v) ? `${v.toFixed(1)} W` : DASH);
export const mhz = (v) => (known(v) ? (v >= 1000 ? `${(v / 1000).toFixed(2)} GHz` : `${Math.round(v)} MHz`) : DASH);
export const rpm = (v) => (known(v) ? `${Math.round(v)} rpm` : DASH);

export function bytes(v) {
  if (!known(v)) return DASH;
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${u[i]}`;
}

export const rate = (v) => (known(v) ? `${bytes(v)}/s` : DASH);

export function uptime(s) {
  if (!known(s)) return DASH;
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  return d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${m}m` : `${m}m`;
}

export const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
export { known, DASH };

// Compact counts for token totals: 1.2K, 3.4M, 5.6B. Unknown shows "—".
export function count(v) {
  if (!known(v)) return DASH;
  const a = Math.abs(v);
  if (a < 1000) return String(Math.round(v));
  const u = [[1e9, "B"], [1e6, "M"], [1e3, "K"]].find(([n]) => a >= n);
  const x = v / u[0];
  return `${x.toFixed(Math.abs(x) < 10 ? 2 : Math.abs(x) < 100 ? 1 : 0)}${u[1]}`;
}

// Whole numbers with thousands separators.
export const int = (v) => (known(v) ? Math.round(v).toLocaleString("en-US") : DASH);
