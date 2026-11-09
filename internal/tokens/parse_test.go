package tokens

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

const fixtures = "../../testdata/sessions"

// copyTree copies a fixture tree to dst.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func hour(h int) time.Time { return time.Date(2026, 11, 1, h, 0, 0, 0, time.UTC) }

func TestParseClaude(t *testing.T) {
	got, err := ParseClaude(filepath.Join(fixtures, "claude/-demo-app/sess-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Usage{
		// msg_A repeats on two lines and counts once; bad JSON and <synthetic> are skipped.
		{Agent: "claude", Model: "claude-demo-1", Hour: hour(10), Input: 11, Output: 25, CacheRead: 300, CacheWrite: 50, Sessions: 1},
		{Agent: "claude", Model: "claude-demo-2", Hour: hour(11), Input: 3, Output: 7, CacheWrite: 9, Sessions: 1},
	}
	checkRows(t, got, want)
}

func TestParseCodex(t *testing.T) {
	got, err := ParseCodex(filepath.Join(fixtures, "codex/2026/11/01/rollout-2026-11-01T10-00-00-demo.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Usage{
		// The 10:02:05 event repeats the total and adds nothing.
		{Agent: "codex", Model: "codex-demo-1", Hour: hour(10), Input: 800, Output: 250, CacheRead: 1000, CacheWrite: 20, Reasoning: 80, Sessions: 1},
		{Agent: "codex", Model: "codex-demo-2", Hour: hour(11), Input: 200, Output: 50, CacheRead: 500, Reasoning: 10, Sessions: 1},
	}
	checkRows(t, got, want)
}

func TestParseGrok(t *testing.T) {
	got, err := ParseGrok(filepath.Join(fixtures, "grok/%2Ftmp%2Fdemo/0000aaaa-0000-7000-8000-000000000001/updates.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Usage{
		{Agent: "grok", Model: "grok-demo-1", Hour: hour(10), Input: 400, Output: 100, CacheRead: 600, Reasoning: 40, Sessions: 1},
		{Agent: "grok", Model: "grok-demo-1", Hour: hour(11), Input: 200, Output: 30, CacheRead: 100, CacheWrite: 10, Reasoning: 10, Sessions: 1},
		{Agent: "grok", Model: "grok-demo-2", Hour: hour(11), Input: 200, Output: 20, Reasoning: 10, Sessions: 1},
	}
	checkRows(t, got, want)
}

// A turn without modelUsage takes its model from summary.json.
func TestParseGrokFallbackModel(t *testing.T) {
	got, err := ParseGrok(filepath.Join(fixtures, "grok/%2Ftmp%2Fdemo/0000aaaa-0000-7000-8000-000000000002/updates.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, got, []model.Usage{
		{Agent: "grok", Model: "grok-demo-3", Hour: hour(10), Input: 50, Output: 8, CacheRead: 20, Reasoning: 3, Sessions: 1},
	})
}

func checkRows(t *testing.T, got, want []model.Usage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}

func TestParseCodexCounterRestartNeverNegative(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rollout.jsonl")
	ev := func(ts string, in, out int) string {
		return `{"type":"event_msg","timestamp":"` + ts + `","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":` +
			strconv.Itoa(in) + `,"output_tokens":` + strconv.Itoa(out) + `}}}}` + "\n"
	}
	body := ev("2026-11-08T10:00:00Z", 1000, 100) + ev("2026-11-08T10:05:00Z", 50, 500)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	rows, err := ParseCodex(p)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %+v err %v", rows, err)
	}
	if r := rows[0]; r.Input != 1050 || r.Output != 600 {
		t.Fatalf("got input %d output %d, want 1050 and 600", r.Input, r.Output)
	}
}
