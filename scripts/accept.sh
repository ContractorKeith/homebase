#!/usr/bin/env bash
# scripts/accept.sh — the acceptance for this repo. Worker, reviewer and CI run this
# exact script; the orchestrator never retypes it. Keep it under a minute.
# Bash 3.2 compatible (macOS): no mapfile, no ${var,,}, no associative arrays.
set -eu

cd "$(dirname "$0")/.."
bin="${ACCEPT_BIN:-./homebase}"
scratch="$(mktemp -d)"
trap 'rm -r "$scratch"' EXIT

echo "== gate"
go test -race ./...
test -z "$(gofmt -l .)"
go vet ./...
staticcheck ./...

echo "== build"
CGO_ENABLED=0 go build -o "$bin" ./cmd/homebase

echo "== fixture"
# Isolate everything that could touch real user data.
export HOME="$scratch/home" XDG_CACHE_HOME="$scratch/cache" XDG_CONFIG_HOME="$scratch/config"
mkdir -p "$HOME" "$XDG_CACHE_HOME" "$XDG_CONFIG_HOME"
mkdir -p "$HOME/.claude" "$HOME/.codex" "$HOME/.grok"
cp -R testdata/sessions/claude "$HOME/.claude/projects"
cp -R testdata/sessions/codex "$HOME/.codex/sessions"
cp -R testdata/sessions/grok "$HOME/.grok/sessions"

echo "== goldens"
for a in claude codex grok; do "$bin" scan --agent $a --json | diff - testdata/golden/scan-$a.json; done
# M5: start hub on "$scratch/hub.db" with fake machine-monitor; diff each /api range with its golden.

echo "== live checks"
"$bin" version | grep -q '^homebase '                    # version prints
set +e; "$bin" nope 2>/dev/null; rc=$?; set -e; test $rc -eq 2   # unknown command is a usage error
# M4: post the same usage batch twice; totals unchanged. Post without the key; expect 401.
port=$((20000 + RANDOM % 20000)); hubkey=accept-key; printf '%s\n' "$hubkey" > "$scratch/key"
"$bin" hub --listen 127.0.0.1:$port --key-file "$scratch/key" --db "$scratch/hub.db" \
	--no-github --mm http://127.0.0.1:1 >/dev/null 2>&1 &
hubpid=$!
trap 'kill $hubpid 2>/dev/null || true; rm -r "$scratch"' EXIT
hour="$(date -u +%Y-%m-%dT%H:00:00Z)"
batch='{"machine":"accept","rows":[{"agent":"claude","model":"m","hour":"'$hour'","input":10,"output":20,"cacheRead":0,"cacheWrite":0,"reasoning":0,"sessions":1}]}'
ingest() { curl -s -o /dev/null -w '%{http_code}' -X POST "$@" -d "$batch" "http://127.0.0.1:$port/api/ingest/usage"; }
for _ in 1 2 3 4 5 6 7 8 9 10; do curl -s -o /dev/null "http://127.0.0.1:$port/" && break; sleep 0.5; done
test "$(ingest -H "Authorization: Bearer $hubkey")" = 200
test "$(ingest -H "Authorization: Bearer $hubkey")" = 200
test "$(ingest)" = 401
if command -v sqlite3 >/dev/null; then
	test "$(sqlite3 "$scratch/hub.db" 'select count(*), sum(input), sum(output) from usage')" = '1|10|20'
fi
kill $hubpid; wait $hubpid 2>/dev/null || true

echo "accept: ok"
