// GitHub tab: KPI tiles, commits per bucket, lines added/deleted, and the top repos.
import { barChart, seriesColor } from "./charts.js";
import { count, esc, int } from "./fmt.js";
import { tile } from "./kpi.js";
import { getRange, onRange, timeZone } from "./range.js";
import { loader } from "./view.js";

const panel = document.getElementById("panel-github");
const body = panel.querySelector(".view-body");
const load = loader(body);
let plots = [];

export function refresh() {
  load(`/api/github?range=${getRange()}&tz=${encodeURIComponent(timeZone())}`, draw);
}
onRange(() => { if (!panel.hidden) refresh(); });

function repoTable(repos) {
  const rows = repos.map((r) => `<tr><th scope="row">${esc(r.repo)}</th><td>${int(r.commits)}</td><td>+${int(r.additions)}</td><td>−${int(r.deletions)}</td>
    <td>${int(r.prsOpened)}</td><td>${int(r.prsMerged)}</td><td>${int(r.issues)}</td><td>${int(r.reviews)}</td></tr>`).join("");
  return `<table class="data"><thead><tr><th scope="col">Repo</th><th>Commits</th><th>Added</th><th>Deleted</th><th>PRs opened</th><th>PRs merged</th><th>Issues</th><th>Reviews</th></tr></thead><tbody>${rows}</tbody></table>`;
}

function draw(d) {
  plots.forEach((p) => p.destroy());
  plots = [];
  const t = d.totals, p = d.prevTotals;
  if (!Object.values(t).some(Boolean)) {
    body.innerHTML = `<p class="empty">No GitHub activity in this range yet.</p>`;
    return;
  }
  const lines = t.additions + t.deletions, prevLines = p.additions + p.deletions;
  body.innerHTML = `<div class="kpis">
      ${tile("Commits", int(t.commits), t.commits, p.commits)}
      ${tile("PRs opened", int(t.prsOpened), t.prsOpened, p.prsOpened)}
      ${tile("PRs merged", int(t.prsMerged), t.prsMerged, p.prsMerged)}
      ${tile("Lines + / −", `+${count(t.additions)} / −${count(t.deletions)}`, lines, prevLines)}
      ${tile("Issues", int(t.issues), t.issues, p.issues)}
      ${tile("Reviews", int(t.reviews), t.reviews, p.reviews)}</div>
    <div class="panel"><h3>Commits</h3><div class="plot" id="gh-commits"></div></div>
    <div class="panel"><h3>Lines added and deleted</h3><div class="plot" id="gh-lines"></div></div>
    <div class="panel"><h3>Top repos</h3>${d.repos.length ? repoTable(d.repos) : `<p class="sub">—</p>`}</div>`;
  const labels = d.buckets.map((b) => b.label), s = d.series;
  plots.push(
    barChart(document.getElementById("gh-commits"), { labels, series: [{ name: "Commits", color: seriesColor(1), values: s.commits }], height: 200 }),
    barChart(document.getElementById("gh-lines"), { labels, signed: true, height: 200, series: [
      { name: "Added", color: seriesColor(3), values: s.additions }, { name: "Deleted", color: seriesColor(2), values: s.deletions.map((v) => -v) }] }),
  );
}
