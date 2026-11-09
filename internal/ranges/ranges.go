// Package ranges turns a range name (today, 7d, 30d, 90d, ttm) into time buckets in the
// viewer's zone, plus the previous period of the same length for "vs previous".
package ranges

import (
	"fmt"
	"time"
)

// Names lists the supported ranges in display order.
var Names = []string{"today", "7d", "30d", "90d", "ttm"}

// Bucket is one half-open interval [Start, End) with a short axis label.
type Bucket struct {
	Start time.Time
	End   time.Time
	Label string
}

// Valid reports whether rng is a known range name.
func Valid(rng string) bool {
	for _, n := range Names {
		if n == rng {
			return true
		}
	}
	return false
}

// Buckets returns the current and previous periods for rng, as seen from now in loc.
//   - today: hourly buckets from local midnight (24, or 23/25 on a DST change day);
//   - 7d, 30d, 90d: local days ending with today;
//   - ttm: 52 weeks ending this week, weeks starting Monday.
func Buckets(rng string, now time.Time, loc *time.Location) (cur, prev []Bucket, err error) {
	now = now.In(loc)
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	switch rng {
	case "today":
		return hours(today), hours(today.AddDate(0, 0, -1)), nil
	case "7d", "30d", "90d":
		n := map[string]int{"7d": 7, "30d": 30, "90d": 90}[rng]
		start := today.AddDate(0, 0, -(n - 1))
		return days(start, n), days(start.AddDate(0, 0, -n), n), nil
	case "ttm":
		monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
		start := monday.AddDate(0, 0, -7*51)
		return weeks(start, 52), weeks(start.AddDate(0, 0, -7*52), 52), nil
	}
	return nil, nil, fmt.Errorf("ranges: unknown range %q", rng)
}

// DayBuckets is Buckets for day-granular data (GitHub): "today" is a single day bucket.
func DayBuckets(rng string, now time.Time, loc *time.Location) (cur, prev []Bucket, err error) {
	if rng != "today" {
		return Buckets(rng, now, loc)
	}
	now = now.In(loc)
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	return days(today, 1), days(today.AddDate(0, 0, -1), 1), nil
}

// Find returns the index of the bucket containing t, or -1.
func Find(bs []Bucket, t time.Time) int {
	lo, hi := 0, len(bs)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case t.Before(bs[mid].Start):
			hi = mid
		case !t.Before(bs[mid].End):
			lo = mid + 1
		default:
			return mid
		}
	}
	return -1
}

// hours walks absolute hours from local midnight to the next local midnight.
func hours(midnight time.Time) []Bucket {
	next := midnight.AddDate(0, 0, 1)
	var out []Bucket
	for t := midnight; t.Before(next); t = t.Add(time.Hour) {
		out = append(out, Bucket{t, t.Add(time.Hour), t.Format("15:00")})
	}
	return out
}

func days(start time.Time, n int) []Bucket {
	out := make([]Bucket, n)
	for i := range out {
		s := start.AddDate(0, 0, i)
		out[i] = Bucket{s, s.AddDate(0, 0, 1), s.Format("Jan 2")}
	}
	return out
}

func weeks(start time.Time, n int) []Bucket {
	out := make([]Bucket, n)
	for i := range out {
		s := start.AddDate(0, 0, 7*i)
		out[i] = Bucket{s, s.AddDate(0, 0, 7), s.Format("Jan 2")}
	}
	return out
}
