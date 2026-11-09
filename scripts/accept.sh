#!/usr/bin/env bash
# scripts/accept.sh — the acceptance for this repo. Worker, reviewer and CI run this
# exact script; the orchestrator never retypes it. Keep it under a minute.
# Bash 3.2 compatible (macOS): no mapfile, no ${var,,}, no associative arrays.
set -eu

cd "$(dirname "$0")/.."
bin="${ACCEPT_BIN:-./homebase}"
scratch="$(mktemp -d)"
trap 'rm -r "$scratch"' EXIT

if [ ! -f go.mod ]; then
  echo "accept: no code yet (pre-M0)"; exit 0
fi

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
# M1: copy testdata/sessions/{claude,codex,grok} into $HOME's agent store paths.

echo "== goldens"
# M1: for a in claude codex grok; do "$bin" scan --agent $a --json | diff - testdata/golden/scan-$a.json; done
# M5: start hub on "$scratch/hub.db" with fake machine-monitor; diff each /api range with its golden.

echo "== live checks"
# M4: post the same usage batch twice; totals unchanged.
# M4: post without the key; expect HTTP 401.

echo "accept: ok"
