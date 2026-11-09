# homebase

A local web dashboard for three machines: per-core CPU, GPU, temperatures and network
quality; Claude, Codex and Grok token usage; and GitHub activity. Tokens and GitHub each have
five time ranges: today by the hour, 7 days, 30 days, 90 days and the trailing twelve months.

**Status:** planned. The build plan is in [docs/PLAN.md](docs/PLAN.md).

## Install

Build it:

    go install github.com/ContractorKeith/homebase/cmd/homebase@latest

Or from a clone: `go build -o homebase ./cmd/homebase`. Services start the binary at the
path you run `service install` from, so put it where it will stay (`~/.local/bin`).

**Hub, on the Mac Studio.** You need machine-monitor's hub on :7777 with its key file, and
`gh auth login` done.

    homebase service install --role hub -- --mm-key-file ~/.config/machine-monitor/key

The hub makes its own key at `~/.config/homebase/key` on first start.

**Agent, on every machine** (the Studio too). First copy the hub's key file to
`~/.config/homebase/key` on that machine, then:

    homebase service install --role agent -- --hub http://mac-studio:7780 --key-file ~/.config/homebase/key

**Open the dashboard** on the MacBook once at `http://<studio-tailscale-name>:7780/?key=<key>`.
The cookie remembers it.

Add `--dry-run` to see the file and commands without running anything. Check a service with
`homebase service status --role hub|agent`. Mac logs are in `~/Library/Logs/homebase/`;
on Linux use `journalctl --user -u homebase-agent`.

**Uninstall:** `homebase service uninstall --role hub|agent`.
