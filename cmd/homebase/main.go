// Command homebase is the dashboard hub (Mac Studio) and the per-machine agent.
package main

import (
	"fmt"
	"io"
	"os"
)

var version = "dev" // set by -ldflags "-X main.version=..."

// command runs one subcommand with its arguments and returns the exit code.
type command func(args []string, stdout, stderr io.Writer) int

// commands maps subcommand names to their implementations. Each milestone registers its
// real command from its own file's init (e.g. scan.go: commands["scan"] = runScan), so
// parallel branches never edit this map.
var commands = map[string]command{
	"hub":   stub("hub", "M4"),
	"agent": stub("agent", "M3"),
	"scan":  stub("scan", "M1"),
	"version": func(_ []string, stdout, _ io.Writer) int {
		fmt.Fprintln(stdout, "homebase", version)
		return 0
	},
}

func stub(name, milestone string) command {
	return func(_ []string, _, stderr io.Writer) int {
		fmt.Fprintf(stderr, "homebase %s: not built yet (%s)\n", name, milestone)
		return 1
	}
}

const usage = `usage: homebase <command> [flags]

commands:
  hub      run the dashboard hub and collector
  agent    report this machine's token usage and network quality to a hub
  scan     parse local agent sessions and print hourly usage
  service  install, remove or check the per-user service (launchd / systemd)
  version  print the version
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stderr, usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "homebase: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	return cmd(args[1:], stdout, stderr)
}
