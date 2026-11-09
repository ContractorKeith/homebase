# homebase

A local web dashboard for three machines: per-core CPU, GPU, temperatures and network
quality; Claude, Codex and Grok token usage; and GitHub activity. Tokens and GitHub each have
five time ranges: today by the hour, 7 days, 30 days, 90 days and the trailing twelve months.

**Status:** v1 running on the Mac Studio (hub), MacBook Air and Omarchy. The build plan is in
[docs/PLAN.md](docs/PLAN.md).

## Install

Build it:

    go install github.com/ContractorKeith/homebase/cmd/homebase@latest

Or from a clone: `go build -o homebase ./cmd/homebase`. Services start the binary at the
path you run `service install` from, so put it where it will stay (`~/.local/bin`).

**Hub, on the Mac Studio.** You need machine-monitor's hub running on :7777
(`machine-monitor service install --role hub`) and `gh auth login` done. Save
machine-monitor's key where homebase can read it, then install:

    (umask 077; machine-monitor hub --show-key > ~/.config/homebase/mm.key)
    homebase service install --role hub -- --mm http://127.0.0.1:7777 --mm-key-file ~/.config/homebase/mm.key

The hub makes its own key at `~/.config/homebase/key` on first start.

**Agent, on every machine** (the Studio too). First copy the hub's key file to
`~/.config/homebase/key` on that machine, then:

    homebase service install --role agent -- --hub http://<studio-tailscale-ip>:7780 --key-file ~/.config/homebase/key

Use the Studio's Tailscale IP (`tailscale ip -4` on the Studio), or `http://127.0.0.1:7780` on
the Studio itself. machine-monitor's hub answers only requests addressed by IP or its own
name, so use the IP for its agents too.

**Open the dashboard** on the MacBook once at `http://<studio-tailscale-ip>:7780/?key=<key>`
(the key is in `~/.config/homebase/key` on the Studio).
The cookie remembers it.

Add `--dry-run` to see the file and commands without running anything. Check a service with
`homebase service status --role hub|agent`. Mac logs are in `~/Library/Logs/homebase/`;
on Linux use `journalctl --user -u homebase-agent`.

**Uninstall:** `homebase service uninstall --role hub|agent`.
