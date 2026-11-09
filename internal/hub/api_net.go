package hub

import (
	"net/http"
	"sort"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

const maxNetPoints = 300 // per machine and target

// netPoint is one downsampled latency point; unknown values are null.
type netPoint struct {
	T        int64    `json:"t"` // unix milliseconds
	RTTMs    *float64 `json:"rttMs"`
	JitterMs *float64 `json:"jitterMs"`
	LossPct  *float64 `json:"lossPct"`
}

type netSeries struct {
	Machine string     `json:"machine"`
	Target  string     `json:"target"`
	Points  []netPoint `json:"points"`
}

// avgPtr averages the known values; nil when none are known.
func avgPtr(vs []*float64) *float64 {
	var sum float64
	n := 0
	for _, v := range vs {
		if v != nil {
			sum += *v
			n++
		}
	}
	if n == 0 {
		return nil
	}
	a := sum / float64(n)
	return &a
}

// downsample averages samples into at most max points of equal span from..to.
func downsample(ss []model.NetSample, from, to time.Time, max int) []netPoint {
	span := to.Sub(from) / time.Duration(max)
	if span < time.Second {
		span = time.Second
	}
	var out []netPoint
	var rtt, jit, loss []*float64
	var idx int64 = -1
	flush := func() {
		if idx >= 0 {
			out = append(out, netPoint{T: from.Add(time.Duration(idx)*span + span/2).UnixMilli(),
				RTTMs: avgPtr(rtt), JitterMs: avgPtr(jit), LossPct: avgPtr(loss)})
		}
		rtt, jit, loss = nil, nil, nil
	}
	for _, n := range ss {
		i := int64(n.TS.Sub(from) / span)
		if i != idx {
			flush()
			idx = i
		}
		rtt, jit, loss = append(rtt, n.RTTMs), append(jit, n.JitterMs), append(loss, n.LossPct)
	}
	flush()
	return out
}

// handleNet serves GET /api/net?range=24h|7d&machine=: the latest sample per machine and
// target, the latest speed test per machine and a downsampled latency series.
func (s *Server) handleNet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	span := map[string]time.Duration{"": 24 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}[q.Get("range")]
	if span == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "range must be 24h or 7d"})
		return
	}
	if s.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no database"})
		return
	}
	machine, now := q.Get("machine"), s.Now()
	from, to := now.Add(-span), now.Add(time.Second)
	samples, err := s.Store.NetBetween(r.Context(), from, to)
	if err == nil {
		var speed []model.SpeedTest
		// Speed tests run every 6 h; look back a month so the card always has the last one.
		if speed, err = s.Store.SpeedBetween(r.Context(), now.AddDate(0, 0, -30), to); err == nil {
			writeJSON(w, http.StatusOK, buildNet(samples, speed, machine, from, to))
			return
		}
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

type netKey struct{ machine, target string }

func buildNet(samples []model.NetSample, speed []model.SpeedTest, machine string, from, to time.Time) map[string]any {
	latest := []model.NetSample{}
	latestIdx := map[netKey]int{}
	groups := map[netKey][]model.NetSample{}
	var order []netKey
	for _, n := range samples { // ordered by ts, so the last one seen is the latest
		if machine != "" && n.Machine != machine {
			continue
		}
		k := netKey{n.Machine, n.Target}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], n)
		if i, ok := latestIdx[k]; ok {
			latest[i] = n
		} else {
			latestIdx[k] = len(latest)
			latest = append(latest, n)
		}
	}
	series := []netSeries{}
	for _, k := range order {
		series = append(series, netSeries{k.machine, k.target, downsample(groups[k], from, to, maxNetPoints)})
	}
	lastSpeed := map[string]model.SpeedTest{}
	for _, sp := range speed {
		if machine == "" || sp.Machine == machine {
			lastSpeed[sp.Machine] = sp
		}
	}
	speeds := make([]model.SpeedTest, 0, len(lastSpeed))
	for _, sp := range lastSpeed {
		speeds = append(speeds, sp)
	}
	sort.Slice(speeds, func(i, j int) bool { return speeds[i].Machine < speeds[j].Machine })
	return map[string]any{"latest": latest, "speed": speeds, "series": series}
}
