// The range shared by the Tokens and GitHub tabs, kept in the URL hash as #tab/range
// so a reload keeps both. Every [data-range-control] element is one view of the same state.
export const RANGES = [["today", "Today"], ["7d", "7D"], ["30d", "30D"], ["90d", "90D"], ["ttm", "TTM"]];
const listeners = [];
let range = "7d";

export function parseHash() {
  const [tab, r] = location.hash.slice(1).split("/");
  return { tab, range: RANGES.some(([k]) => k === r) ? r : null };
}
if (parseHash().range) range = parseHash().range;

export const getRange = () => range;
export const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;
export const onRange = (fn) => listeners.push(fn);

// writeHash records the active tab and, on the range tabs, the range.
export function writeHash(tab) {
  const ranged = tab === "tokens" || tab === "github";
  history.replaceState(null, "", `#${tab}${ranged ? `/${range}` : ""}`);
}

function mark() {
  document.querySelectorAll("[data-range-control] button").forEach((b) => b.setAttribute("aria-selected", String(b.dataset.range === range)));
}

export function setRange(r) {
  if (r === range) return;
  range = r;
  mark();
  const tab = parseHash().tab;
  if (tab) writeHash(tab);
  listeners.forEach((fn) => fn(r));
}

document.querySelectorAll("[data-range-control]").forEach((el) => {
  el.classList.add("ranges");
  el.setAttribute("role", "group");
  el.setAttribute("aria-label", "Time range");
  RANGES.forEach(([k, label]) => {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = label;
    b.dataset.range = k;
    b.addEventListener("click", () => setRange(k));
    el.append(b);
  });
});
mark();
