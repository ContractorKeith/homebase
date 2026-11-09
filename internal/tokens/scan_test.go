package tokens

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

// scanHome builds a scratch home holding all three fixture stores.
func scanHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	copyTree(t, filepath.Join(fixtures, "claude"), filepath.Join(home, ".claude/projects"))
	copyTree(t, filepath.Join(fixtures, "codex"), filepath.Join(home, ".codex/sessions"))
	copyTree(t, filepath.Join(fixtures, "grok"), filepath.Join(home, ".grok/sessions"))
	return home
}

func mustScan(t *testing.T, o Options) []model.Usage {
	t.Helper()
	rows, err := Scan(o)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestScanSumsAcrossFiles(t *testing.T) {
	home := scanHome(t)
	rows := mustScan(t, Options{Home: home, Agents: []string{"claude"}, Machine: "m1"})
	// claude-demo-2 at 11:00: sess-1 (input 3) plus the subagent file (input 2).
	for _, r := range rows {
		if r.Machine != "m1" {
			t.Errorf("machine = %q", r.Machine)
		}
		if r.Model == "claude-demo-2" && (r.Input != 5 || r.Sessions != 2) {
			t.Errorf("claude-demo-2 = %+v, want input 5 sessions 2", r)
		}
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2", len(rows))
	}
}

func TestScanStateSkipsUnchangedAndSurvivesDelete(t *testing.T) {
	home := scanHome(t)
	state := filepath.Join(t.TempDir(), "state.json")
	var parsed []string
	o := Options{Home: home, StatePath: state, OnParse: func(p string) { parsed = append(parsed, p) }}

	first := mustScan(t, o)
	if len(parsed) != 5 {
		t.Fatalf("first scan parsed %d files, want 5", len(parsed))
	}
	if fi, err := os.Stat(state); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode: %v %v", fi, err)
	}

	parsed = nil
	if second := mustScan(t, o); !reflect.DeepEqual(first, second) || len(parsed) != 0 {
		t.Fatalf("unchanged rescan differs or re-parsed %v", parsed)
	}

	// Deleting a transcript keeps its totals.
	if err := os.Remove(filepath.Join(home, ".claude/projects/-demo-app/sess-1.jsonl")); err != nil {
		t.Fatal(err)
	}
	if third := mustScan(t, o); !reflect.DeepEqual(first, third) {
		t.Fatalf("totals changed after delete:\n%+v\n%+v", first, third)
	}
}

func TestScanReparsesOnlyGrownFile(t *testing.T) {
	home := scanHome(t)
	state := filepath.Join(t.TempDir(), "state.json")
	var parsed []string
	o := Options{Home: home, StatePath: state, OnParse: func(p string) { parsed = append(parsed, p) }}
	before := mustScan(t, o)

	path := filepath.Join(home, ".claude/projects/-demo-app/sess-1/subagents/agent-1.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","timestamp":"2026-11-01T12:00:00.000Z","message":{"id":"msg_E","model":"claude-demo-2","usage":{"input_tokens":40,"output_tokens":1}}}` + "\n"
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
	f.Close()

	parsed = nil
	after := mustScan(t, o)
	if len(parsed) != 1 || parsed[0] != path {
		t.Fatalf("re-parsed %v, want only %s", parsed, path)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("rows %d -> %d, want one new hour", len(before), len(after))
	}
	last := after[len(after)-1]
	if !last.Hour.Equal(time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)) || last.Input != 40 {
		t.Errorf("new row = %+v", last)
	}
}

func TestScanMissingStoreIsEmpty(t *testing.T) {
	if rows := mustScan(t, Options{Home: t.TempDir()}); len(rows) != 0 {
		t.Errorf("got %v", rows)
	}
}
