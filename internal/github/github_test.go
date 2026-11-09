package github

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden file")

var opRE = regexp.MustCompile(`query (\w+)`)

// recorded serves testdata/github/<Op>[_<owner>_<name>][_<after>].json for each query.
func recorded(t *testing.T, calls *[]string) Runner {
	return func(_ context.Context, q string, vars map[string]string) ([]byte, error) {
		name := opRE.FindStringSubmatch(q)[1]
		if vars["owner"] != "" {
			name += "_" + vars["owner"] + "_" + vars["name"]
		}
		if vars["after"] != "" {
			name += "_" + vars["after"]
		}
		*calls = append(*calls, name)
		return os.ReadFile("../../testdata/github/" + name + ".json")
	}
}

func TestFetchRollsUpToGolden(t *testing.T) {
	var calls []string
	c := &Client{Run: recorded(t, &calls)}
	from := time.Date(2026, 11, 5, 13, 0, 0, 0, time.UTC) // floors to the start of the day
	days, err := c.Fetch(context.Background(), from, time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.MarshalIndent(days, "", "  ")
	got = append(got, '\n')
	const golden = "../../testdata/github/golden.json"
	if *update {
		os.WriteFile(golden, got, 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("rollup differs from golden:\n%s", got)
	}
	// Both paginated connections were followed to their last page.
	joined := strings.Join(calls, " ")
	for _, n := range []string{"PRs_c1", "History_ContractorKeith_homebase_h1"} {
		if !strings.Contains(joined, n) {
			t.Errorf("never fetched %s; calls: %s", n, joined)
		}
	}
}

func TestFetchSplitsLongRanges(t *testing.T) {
	var calls []string
	run := recorded(t, &calls)
	c := &Client{Run: func(ctx context.Context, q string, v map[string]string) ([]byte, error) {
		if v["from"] != "" && strings.Contains(q, "query Commits") {
			f, _ := time.Parse(time.RFC3339, v["from"])
			e, _ := time.Parse(time.RFC3339, v["to"])
			if e.Sub(f) > window {
				t.Errorf("window %s..%s is longer than %s", v["from"], v["to"], window)
			}
		}
		return run(ctx, q, v)
	}}
	end := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	if _, err := c.Fetch(context.Background(), end.AddDate(-1, 0, 0), end); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range calls {
		if c == "Commits" {
			n++
		}
	}
	if n != 5 {
		t.Fatalf("%d commit queries for 365 days, want 5", n)
	}
}

func TestFetchSurfacesErrors(t *testing.T) {
	c := &Client{Run: func(context.Context, string, map[string]string) ([]byte, error) {
		return nil, errors.New("gh: not logged in")
	}}
	if _, err := c.Fetch(context.Background(), time.Now().AddDate(0, 0, -3), time.Now()); err == nil {
		t.Fatal("want an error")
	}
	c = &Client{Run: func(context.Context, string, map[string]string) ([]byte, error) {
		return []byte(`{"errors":[{"message":"rate limited"}]}`), nil
	}}
	if _, err := c.Fetch(context.Background(), time.Now().AddDate(0, 0, -3), time.Now()); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err=%v", err)
	}
}
