package tokens

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ContractorKeith/homebase/internal/model"
)

// Options controls one Scan.
type Options struct {
	Home      string            // user home; empty means $HOME with CLAUDE_CONFIG_DIR / CODEX_HOME honored
	Agents    []string          // model.Agent* names; empty means all
	StatePath string            // optional state file; empty means no persistence
	Machine   string            // stamped on every row
	OnParse   func(path string) // optional hook, called for each file that is (re)parsed
}

// source describes where one agent keeps sessions and how to parse a file.
type source struct {
	agent string
	root  func(o Options) string
	match func(name string) bool
	parse func(path string) ([]model.Usage, error)
}

func isJSONL(name string) bool { return strings.HasSuffix(name, ".jsonl") }

var sources = []source{
	{model.AgentClaude, func(o Options) string { return envRoot(o, "CLAUDE_CONFIG_DIR", ".claude", "projects") }, isJSONL, ParseClaude},
	{model.AgentCodex, func(o Options) string { return envRoot(o, "CODEX_HOME", ".codex", "sessions") }, isJSONL, ParseCodex},
	{model.AgentGrok, func(o Options) string { return envRoot(o, "", ".grok", "sessions") },
		func(n string) bool { return n == "updates.jsonl" }, ParseGrok},
}

// envRoot returns the store root: <env>/<sub> when Home is unset and env is set, else <home>/<dot>/<sub>.
func envRoot(o Options, env, dot, sub string) string {
	if o.Home == "" && env != "" {
		if v := os.Getenv(env); v != "" {
			return filepath.Join(v, sub)
		}
	}
	home := o.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, dot, sub)
}

// Scan returns hourly totals for the chosen agents, summed across files and sorted by
// hour, agent, model. With a state file, unchanged files are not re-parsed and files that
// have since been deleted keep their contributions.
func Scan(o Options) ([]model.Usage, error) {
	st, err := loadState(o.StatePath)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, a := range o.Agents {
		want[a] = true
	}
	for _, s := range sources {
		if len(want) > 0 && !want[s.agent] {
			continue
		}
		root := s.root(o)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !s.match(d.Name()) {
				return nil // unreadable entries are skipped, not fatal
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if e, ok := st.Files[p]; ok && e.Size == info.Size() && e.MTime == info.ModTime().UnixNano() {
				return nil
			}
			if o.OnParse != nil {
				o.OnParse(p)
			}
			rows, err := s.parse(p)
			if err != nil {
				return nil
			}
			st.put(p, s.agent, info, rows)
			return nil
		})
	}
	if err := st.save(o.StatePath); err != nil {
		return nil, err
	}
	return st.totals(want, o.Machine), nil
}
