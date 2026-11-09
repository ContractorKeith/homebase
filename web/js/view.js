// Shared helpers for the Tokens and GitHub tabs: fetching with a stale-response guard, and
// loading, error and empty states.
import { esc } from "./fmt.js";

// loader returns load(url, render): it fetches url and calls render(data) unless a newer
// load started meanwhile. Failures show a message in host.
export function loader(host) {
  let seq = 0;
  return async (url, render) => {
    const mine = ++seq;
    host.setAttribute("aria-busy", "true");
    if (!host.firstChild) host.innerHTML = `<p class="sub">loading…</p>`;
    try {
      const r = await fetch(url);
      if (!r.ok) throw new Error(`${r.status}`);
      const data = await r.json();
      if (mine === seq) { host.removeAttribute("aria-busy"); render(data); }
    } catch (e) {
      if (mine === seq) { host.removeAttribute("aria-busy"); host.innerHTML = `<p class="empty">Unavailable (${esc(e.message)}). Is the hub reachable?</p>`; }
    }
  };
}

// segmented builds a toggle group; onPick receives the chosen key.
export function segmented(label, options, current, onPick) {
  const el = document.createElement("div");
  el.className = "ranges seg";
  el.setAttribute("role", "group");
  el.setAttribute("aria-label", label);
  options.forEach(([k, text]) => {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = text;
    b.dataset.key = k;
    b.setAttribute("aria-selected", String(k === current));
    b.addEventListener("click", () => {
      el.querySelectorAll("button").forEach((x) => x.setAttribute("aria-selected", String(x === b)));
      onPick(k);
    });
    el.append(b);
  });
  return el;
}
