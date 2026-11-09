package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ContractorKeith/homebase/internal/service"
)

func init() { commands["service"] = runService }

const serviceUsage = "usage: homebase service install|uninstall|status --role hub|agent [--dry-run] [-- <flags for hub/agent>]\n"

// runService installs, removes or reports the per-user service for a role.
func runService(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprint(stderr, serviceUsage)
		return 2
	}
	action := args[0]
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	fs.SetOutput(stderr)
	role := fs.String("role", "", "hub or agent (required)")
	dry := fs.Bool("dry-run", false, "print the file and commands without running them")
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if !service.Valid(*role) || (action != "install" && action != "uninstall" && action != "status") {
		fmt.Fprint(stderr, serviceUsage)
		return 2
	}
	rest := fs.Args() // everything after "--"
	if action == "install" && *role == "agent" && !(hasFlag(rest, "hub") && hasFlag(rest, "key-file")) {
		fmt.Fprintln(stderr, "homebase service: agent needs -- --hub URL --key-file FILE")
		return 2
	}
	m, err := service.Default(stdout)
	if err != nil {
		fmt.Fprintf(stderr, "homebase service: %v\n", err)
		return 1
	}
	m.DryRun = *dry
	switch action {
	case "install":
		err = m.Install(service.Spec{Role: *role, Args: service.ExpandHome(m.Home, rest)})
	case "uninstall":
		err = m.Uninstall(*role)
	default:
		err = m.Status(*role)
	}
	if err != nil {
		fmt.Fprintf(stderr, "homebase service: %v\n", err)
		return 1
	}
	return 0
}

// hasFlag reports whether args holds --name or --name=value (one or two dashes).
func hasFlag(args []string, name string) bool {
	for _, a := range args {
		a = strings.TrimLeft(a, "-")
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}
