# homebase

A local web dashboard for Keith's three machines (Mac Studio, MacBook Air, Omarchy). It has
three tabs. **Machines** shows per-core CPU, GPU, temperatures and network quality.
**Tokens** shows Claude, Codex and Grok token usage. **GitHub** shows commits, PRs and lines
changed. Tokens and GitHub each have five time ranges: today by the hour, 7 days, 30 days,
90 days and the trailing twelve months. It is one Go binary. In hub mode on the always-on Mac
Studio it collects and stores history. In agent mode on each machine it reports local token
usage and network quality. Keith opens it in a browser on the MacBook. "Done" for v1 is
every v1 row in `docs/PLAN.md` §3 working across all three machines.

## Status
v1 (M0–M6) merged and installed 2026-11-08: hub + agent on the Mac Studio, agents on the
MacBook Air and Omarchy. No release tag yet; skeptic-review comes before the tag.

## Pointers
- `docs/PLAN.md`: goal, feature matrix, architecture, data model, API, milestones.

## Commands
```bash
go build -o homebase ./cmd/homebase
go test -race ./...
test -z "$(gofmt -l .)" && go vet ./... && staticcheck ./...
scripts/accept.sh            # the acceptance; worker, reviewer and CI run it
./homebase hub  --config ~/.config/homebase/hub.json     # Mac Studio
./homebase agent --hub http://studio:7780 --key-file ~/.config/homebase/key
```

## Conventions & Gotchas
- Go, `CGO_ENABLED=0` on every target (SQLite through `modernc.org/sqlite`).
- Machine metrics come from the machine-monitor hub API (`/api/devices`, `/api/stream`,
  `/api/history`). Never re-collect CPU, GPU or temperature data here.
- Token parsers read agent session files and never write them. Claude repeats a message's
  usage in several lines: dedupe by `message.id`. Codex `token_count.total_token_usage` is
  cumulative per session: store deltas, not sums.
- Claude Code deletes transcripts after `cleanupPeriodDays` (30 by default). The hub's
  hourly rollups are the long-term record, and a re-scan must never lower a stored hour.
- Any metric can be unknown. Store and send null, never 0.
- All times are stored in UTC and bucketed into hours in the viewer's local zone.

## Out of Scope
- An activity monitor or process list (machine-monitor's TUI has one).
- Remote control of machines, alerts, or anything reachable from the public internet.
- Cost in dollars (Later, see PLAN).
