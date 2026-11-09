package ranges

import (
	"testing"
	"time"
)

func ny(t *testing.T) *time.Location {
	t.Helper()
	l, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	return l
}

func TestRangeTodayDST(t *testing.T) {
	loc := ny(t)
	for _, c := range []struct {
		name string
		now  time.Time
		want int
	}{
		{"plain", time.Date(2026, 6, 10, 15, 0, 0, 0, loc), 24},
		{"spring forward", time.Date(2026, 3, 8, 15, 0, 0, 0, loc), 23},
		{"fall back", time.Date(2026, 11, 1, 15, 0, 0, 0, loc), 25},
	} {
		cur, prev, err := Buckets("today", c.now, loc)
		if err != nil || len(cur) != c.want {
			t.Errorf("%s: %d buckets, %v; want %d", c.name, len(cur), err, c.want)
			continue
		}
		if !prev[len(prev)-1].End.Equal(cur[0].Start) {
			t.Errorf("%s: previous does not end where today starts", c.name)
		}
		for i := 1; i < len(cur); i++ {
			if !cur[i].Start.Equal(cur[i-1].End) {
				t.Errorf("%s: gap before bucket %d", c.name, i)
			}
		}
	}
}

func TestRangeDaysAcrossDST(t *testing.T) {
	loc := ny(t)
	now := time.Date(2026, 3, 10, 9, 0, 0, 0, loc)
	cur, prev, _ := Buckets("7d", now, loc)
	if len(cur) != 7 || cur[0].Start != time.Date(2026, 3, 4, 0, 0, 0, 0, loc) {
		t.Fatalf("7d starts %v", cur[0].Start)
	}
	if d := cur[4].End.Sub(cur[4].Start); d != 23*time.Hour { // Mar 8
		t.Errorf("spring-forward day is %v", d)
	}
	if !prev[6].End.Equal(cur[0].Start) || len(prev) != 7 {
		t.Error("previous 7d is not contiguous")
	}
}

func TestRangeMonthEnds(t *testing.T) {
	loc := time.UTC
	cur, prev, _ := Buckets("30d", time.Date(2026, 3, 1, 12, 0, 0, 0, loc), loc)
	if cur[0].Label != "Jan 31" || cur[29].Label != "Mar 1" || len(prev) != 30 || prev[29].Label != "Jan 30" {
		t.Errorf("30d across Feb: first %s last %s prev-last %s", cur[0].Label, cur[29].Label, prev[29].Label)
	}
	cur, _, _ = Buckets("90d", time.Date(2024, 3, 1, 0, 0, 0, 0, loc), loc) // leap year
	if len(cur) != 90 || cur[0].Label != "Dec 3" {
		t.Errorf("90d across leap Feb starts %s", cur[0].Label)
	}
}

func TestRangeTTMNewYear(t *testing.T) {
	loc := time.UTC
	// Friday 2027-01-01: this week began Monday 2026-12-28.
	cur, prev, _ := Buckets("ttm", time.Date(2027, 1, 1, 10, 0, 0, 0, loc), loc)
	last := cur[51]
	if len(cur) != 52 || len(prev) != 52 {
		t.Fatalf("got %d and %d weeks", len(cur), len(prev))
	}
	if !last.Start.Equal(time.Date(2026, 12, 28, 0, 0, 0, 0, loc)) || !last.End.Equal(time.Date(2027, 1, 4, 0, 0, 0, 0, loc)) {
		t.Errorf("last week %v - %v", last.Start, last.End)
	}
	for _, b := range append(append([]Bucket{}, cur...), prev...) {
		if b.Start.Weekday() != time.Monday {
			t.Fatalf("week starts %v", b.Start.Weekday())
		}
	}
	if !prev[51].End.Equal(cur[0].Start) {
		t.Error("previous TTM is not contiguous")
	}
	if i := Find(cur, time.Date(2027, 1, 3, 23, 0, 0, 0, loc)); i != 51 {
		t.Errorf("Find = %d", i)
	}
	if Find(cur, time.Date(2027, 1, 4, 0, 0, 0, 0, loc)) != -1 {
		t.Error("Find past the end")
	}
}

func TestRangeUnknownAndDayBuckets(t *testing.T) {
	if _, _, err := Buckets("1y", time.Now(), time.UTC); err == nil {
		t.Error("1y should fail")
	}
	cur, prev, _ := DayBuckets("today", time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC), time.UTC)
	if len(cur) != 1 || len(prev) != 1 || cur[0].Label != "May 5" {
		t.Errorf("DayBuckets today = %v", cur)
	}
}
