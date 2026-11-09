package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ptr(f float64) *float64 { return &f }

var h0 = time.Date(2026, 11, 8, 14, 0, 0, 0, time.UTC)

func TestUpsertUsageKeepsMaxAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	row := model.Usage{Machine: "m1", Agent: model.AgentClaude, Model: "claude-opus-5-5",
		Hour: h0.Add(17 * time.Minute), Input: 100, Output: 50, CacheRead: 1000, Sessions: 2}
	for i := 0; i < 2; i++ {
		if err := s.UpsertUsage(ctx, []model.Usage{row}); err != nil {
			t.Fatal(err)
		}
	}
	lower := row
	lower.Input, lower.Output = 10, 80 // re-scan after old files were deleted, plus new output
	if err := s.UpsertUsage(ctx, []model.Usage{lower}); err != nil {
		t.Fatal(err)
	}
	got, err := s.UsageBetween(ctx, h0, h0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := row
	want.Hour, want.Output = h0, 80
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestUpsertUsageRejectsMissingMachine(t *testing.T) {
	if err := open(t).UpsertUsage(context.Background(), []model.Usage{{Agent: "claude", Hour: h0}}); err == nil {
		t.Fatal("want error for a row without machine")
	}
}

func TestNetKeepsNilAndIgnoresDuplicates(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	n := model.NetSample{Machine: "m1", Target: "1.1.1.1", TS: h0, RTTMs: ptr(12.5), LossPct: ptr(0)}
	sp := model.SpeedTest{Machine: "m1", TS: h0, DownMbps: ptr(900), Server: "cloudflare"}
	for i := 0; i < 2; i++ {
		if err := s.InsertNet(ctx, []model.NetSample{n}, []model.SpeedTest{sp}); err != nil {
			t.Fatal(err)
		}
	}
	gn, err := s.NetBetween(ctx, h0, h0.Add(time.Second))
	if err != nil || len(gn) != 1 || gn[0].JitterMs != nil || *gn[0].RTTMs != 12.5 {
		t.Fatalf("net: %+v %v", gn, err)
	}
	gs, err := s.SpeedBetween(ctx, h0, h0.Add(time.Second))
	if err != nil || len(gs) != 1 || gs[0].UpMbps != nil || *gs[0].DownMbps != 900 {
		t.Fatalf("speed: %+v %v", gs, err)
	}
}

func TestGitHubDaysReplace(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	d := model.GitHubDay{Repo: "ContractorKeith/homebase", Day: "2026-11-08", Commits: 3, Additions: 400}
	if err := s.ReplaceGitHubDays(ctx, []model.GitHubDay{d}); err != nil {
		t.Fatal(err)
	}
	d.Commits = 1
	if err := s.ReplaceGitHubDays(ctx, []model.GitHubDay{d}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GitHubBetween(ctx, "2026-11-08", "2026-11-09")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], d) {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestReopenKeepsData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hub.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUsage(ctx, []model.Usage{{Machine: "m", Agent: "codex", Hour: h0, Input: 1}}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, _ := s.UsageBetween(ctx, h0, h0.Add(time.Hour))
	if len(got) != 1 {
		t.Fatalf("got %d rows after reopen", len(got))
	}
}
