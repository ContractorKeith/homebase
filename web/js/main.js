// Tabs, the live /api/stream connection and the machine cards.
import { renderCard } from "./card.js";
import { openHistory } from "./history.js";

const machines = new Map();
const cards = new Map();
const grid = document.getElementById("cards");
const empty = document.getElementById("machines-empty");
const conn = document.getElementById("conn");

// Tabs
const tabs = [...document.querySelectorAll("[role=tab]")];
function select(tab) {
  tabs.forEach((t) => {
    const on = t === tab;
    t.setAttribute("aria-selected", String(on));
    t.tabIndex = on ? 0 : -1;
    document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
  });
  history.replaceState(null, "", `#${tab.id.replace("tab-", "")}`);
}
tabs.forEach((t, i) => {
  t.addEventListener("click", () => select(t));
  t.addEventListener("keydown", (e) => {
    const n = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (n) { const next = tabs[(i + n + tabs.length) % tabs.length]; next.focus(); select(next); }
  });
});
select(document.getElementById(`tab-${location.hash.slice(1)}`) ?? tabs[0]);

function draw(m) {
  let el = cards.get(m.id);
  if (!el) {
    el = document.createElement("button");
    el.type = "button";
    el.className = "card";
    el.addEventListener("click", () => openHistory(machines.get(m.id)));
    cards.set(m.id, el);
  }
  renderCard(el, m);
  return el;
}

// layout puts the cards in machine order and drops removed ones.
function layout() {
  const list = [...machines.values()].sort((a, b) => a.order - b.order || a.name.localeCompare(b.name));
  grid.replaceChildren(...list.map(draw));
  empty.hidden = list.length > 0;
}

function setConn(text, bad) {
  conn.textContent = text;
  conn.classList.toggle("bad", !!bad);
}

function handle(msg) {
  switch (msg.type) {
    case "snapshot":
      machines.clear();
      msg.machines.forEach((m) => machines.set(m.id, m));
      layout();
      break;
    case "machine": {
      const isNew = !machines.has(msg.machine.id);
      machines.set(msg.machine.id, msg.machine);
      if (isNew) layout(); else draw(msg.machine);
      break;
    }
    case "status":
      for (const [id, online] of Object.entries(msg.online)) {
        const m = machines.get(id);
        if (m && m.online !== online) { m.online = online; draw(m); }
      }
      break;
    case "removed":
      machines.delete(msg.id);
      cards.delete(msg.id);
      layout();
      break;
    case "mm":
      setConn(msg.connected ? "live" : `machine-monitor unreachable`, !msg.connected);
      break;
  }
}

const es = new EventSource("/api/stream");
es.onopen = () => setConn("live");
es.onmessage = (e) => handle(JSON.parse(e.data));
es.onerror = () => setConn("reconnecting…", true);
