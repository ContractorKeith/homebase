# homebase — Build Plan

Status: M0 landed 2026-11-08. Sources studied: machine-monitor's hub API and
report schema (`internal/report/report.go`), and the session stores Claude Code, Codex and
Grok write on Omarchy.

## 1. Goal

One page to check three machines (Mac Studio, MacBook Air, Omarchy), agent token usage and
GitHub output. The hub runs on the Mac Studio because it is always on, so history has no gaps
when the MacBook sleeps. Keith opens the hub in a browser on the MacBook over the LAN or
Tailscale. v1 is done when every **v1** row in §3 works across all three machines and
`scripts/accept.sh` passes from a clean clone.

## 2. Inspirations and what to skip

| Source | Copy | Skip |
|---|---|---|
| machine-monitor (own) | Its hub is the machine-metrics source: per-core CPU, GPU, temps, net throughput, history | Re-collecting any of it; its process panel, power actions and alerts |
| session-board (own) | Its list of session-store paths per agent and the habit of reading them without writing | Its status model; it is Rust, so share nothing at the code level |
| ccusage-style tools | Dedupe per message and break usage into input / output / cache read / cache write | Dollar pricing tables in v1 |

## 3. Feature matrix

**v1** = in scope now. **Later** = after v1. **Drop** = deliberately never.

| # | Feature | Decision | Milestone | Notes |
|---|---|---|---|---|
| 1 | Machine cards: online, uptime, CPU total, per-core heat strip, GPU usage and VRAM, CPU/GPU temp, fans | v1 | M2 | From the machine-monitor hub, live over SSE |
| 2 | Per-machine CPU, GPU and temperature history (1 h / 24 h / 7 d) | v1 | M2 | machine-monitor `/api/history` |
| 3 | Network: throughput RX/TX | v1 | M2 | machine-monitor `net` |
| 4 | Network quality: latency, jitter and packet loss to the gateway and to 1.1.1.1, every 30 s | v1 | M3 | homebase agent; plain `ping` is enough, no ICMP privileges |
| 5 | Network speed: down/up test every 6 h plus "run now" | v1 | M3 | Cloudflare speed endpoints; caps at a 25 MB transfer per test |
| 6 | Token usage for Claude, Codex and Grok, per machine and model: input, output, cache read, cache write, reasoning | v1 | M1, M4 | Hourly rollups; see §5 |
| 7 | Five ranges for tokens and GitHub: today by hour, 7 d / 30 d / 90 d by day, TTM by week | v1 | M5 | One range control on every chart |
| 8 | Backfill tokens from all session files on disk at first run | v1 | M1 | Claude only goes back about 30 days, so TTM fills in over time |
| 9 | GitHub: commits, PRs opened/merged, lines added/deleted, issues, reviews, for everything Keith authors (own repos, SlowBooks, forks) | v1 | M4 | `gh api` GraphQL contributions + per-commit stats; cached |
| 10 | Rate-limit meters for each agent (Codex `rate_limits`, Claude 5 h window) | Later | | Data partly there; nice once the core works |
| 11 | Cost in dollars | Later | | Needs a dated price table per model |
| 12 | Process list, power actions, alerts | Drop | | machine-monitor has them |
| 13 | Internet-facing access, login screens | Drop | | Bound to LAN/Tailscale with a shared key |

## 4. Architecture

```
cmd/homebase/           hub | agent | scan | version
internal/tokens/        session parsers (claude, codex, grok) → hourly Usage rows; pure, fixture-tested
internal/netq/          ping and speed probes → NetSample rows
internal/github/        gh client, contribution + commit-stat fetch, cache
internal/model/         shared row and batch types (Usage, NetSample, SpeedTest, GitHubDay, batches)
internal/store/         SQLite schema, migrations, upserts, range queries (the only package touching the DB)
internal/mm/            machine-monitor hub client (devices snapshot, SSE, history)
internal/hub/           HTTP API, agent ingest, schedulers, embedded web UI
web/                    static HTML/JS/CSS embedded with go:embed; uPlot for charts
testdata/               session-file fixtures per agent, gh JSON fixtures, golden API responses
```

Rules:
- Only `internal/store` touches SQLite. Only `internal/hub` serves HTTP.
- The hub's DB is the source of truth for history. The agents' local scan state (file
  offsets) is a cache, so deleting it re-sends rows that upsert to the same totals.
- The web UI is built with no build step: plain ES modules and one vendored chart library.

### Dependencies and why

| Module | Why |
|---|---|
| `modernc.org/sqlite` | SQLite with no cgo, so the static cross-builds keep working |
| uPlot (vendored JS, ~50 KB) | Fast time-series charts with no npm toolchain |
| `gh` CLI (runtime, on the Studio) | Already authenticated; avoids storing a GitHub token in homebase |

## 5. Data model

- **Machine**: `id` = machine-monitor device id; `name` from the hub's rename. Agents send
  the same id (`collect.DeviceID` paths) so token and network rows join to the card.
- **Usage**: `(machine, agent, model, hour_utc)` → input, output, cache_read, cache_write,
  reasoning, sessions. `agent` ∈ claude | codex | grok. Upsert takes the max of stored and
  incoming per column, so a re-scan after Claude deletes old files never lowers history.
- **NetSample**: `(machine, target, ts)` → rtt_ms, jitter_ms, loss_pct; `SpeedTest`:
  `(machine, ts)` → down_mbps, up_mbps, server.
- **GitHubDay**: `(repo, day)` → commits, additions, deletions, prs_opened, prs_merged,
  issues, reviews. Lines come from authored commits on default branches only, so a squash
  merge is not counted twice.
- Ranges: today = 24 hourly buckets in local time; 7/30/90 d = daily; TTM = 52 weekly.

## 6. CLI and API contract

```
homebase hub   [--listen :7780] [--mm http://localhost:7777] [--mm-key-file F] [--db F]
homebase agent --hub URL --key-file F [--interval 5m] [--no-speedtest]
homebase scan  [--agent claude|codex|grok] --json      # parse local sessions, print rows, no network
homebase version

GET  /api/machines                         machine cards (proxied + merged netq)
GET  /api/stream                           SSE: machine updates, netq samples
GET  /api/machines/:id/history?range=1h|24h|7d
GET  /api/tokens?range=today|7d|30d|90d|ttm&by=agent|model|machine
GET  /api/github?range=today|7d|30d|90d|ttm&by=metric|repo
POST /api/ingest/usage                     agent → hub, key required
POST /api/ingest/net                       agent → hub, key required
POST /api/speedtest/:machine               queue a "run now"
```

Every endpoint needs the shared key (a header or a cookie set once by `/?key=`). Exit codes:
0 ok, 1 error, 2 usage. Config precedence: flag > env (`HOMEBASE_*`) > file > default.

## 7. Milestones

Each milestone is one GitHub issue, one branch, one PR, one worker thread in its own
worktree. M1, M2 and M3 can run in parallel once M0 lands.

| M | Deliverable | Depends on | Done when |
|---|---|---|---|
| M0 | Scaffold, CI (Linux + macOS), `internal/store` schema, fixtures dir, `accept.sh` skeleton | – | `accept.sh` passes on the scaffold |
| M1 | `internal/tokens` parsers for all three agents + `homebase scan` | M0 | Rows 6, 8: goldens for each fixture, dedupe and delta tests |
| M2 | `internal/mm` client + Machines tab | M0 | Rows 1–3 against a fake machine-monitor hub |
| M3 | `internal/netq` + agent mode posting net samples | M0 | Rows 4–5 with a fake ping and a local speed server |
| M4 | Hub ingest, schedulers, GitHub fetcher | M1, M3 | Rows 6, 9: ingest is idempotent; gh fixtures roll up to goldens |
| M5 | Tokens and GitHub tabs with the five ranges | M2, M4 | Row 7: golden range responses; UI renders every range |
| M6 | Install: launchd (macOS) and systemd user (Omarchy) units, README | M5 | Hub on the Studio, agents on all three, dashboard open on the MacBook |

## 8. Acceptance

`scripts/accept.sh` builds, runs the gate (race tests, gofmt, vet, staticcheck), runs
`homebase scan` against the fixtures and diffs the output with `testdata/golden/`, then starts a
hub on a scratch DB with a fake machine-monitor and checks every API range against its golden.
Live checks: ingest the same batch twice and confirm the totals do not change; ingest without
the key and expect a 401.

## 9. Process

- One milestone per worker thread (`worker-brief`), draft PR, orchestrator reads the full
  diff and reruns `scripts/accept.sh`, `pr-stress` per PR, then squash merge.
- Before the tag: `skeptic-review` on main, macOS on the Mac Studio and Linux on Omarchy.
- Tracking: GitHub issues per milestone; the repo's card on the Basecamp `github` board.
- The orchestrator writes the worklog entry after each merge; workers report only.
