// Package model holds the rows homebase stores and the batches agents send to the hub.
// These types are the shared contract between tokens, netq, github, store and hub;
// change them only in their own PR.
package model

import "time"

// Agent names, as stored in Usage.Agent.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
	AgentGrok   = "grok"
)

// Usage is one machine's token use for one agent and model in one UTC hour.
// Hour is truncated to the hour in UTC. Counts are totals for that hour, not deltas.
type Usage struct {
	Machine    string    `json:"machine"`
	Agent      string    `json:"agent"`
	Model      string    `json:"model"`
	Hour       time.Time `json:"hour"`
	Input      int64     `json:"input"`      // uncached input tokens
	Output     int64     `json:"output"`     // output tokens, reasoning included where the agent counts it there
	CacheRead  int64     `json:"cacheRead"`  // input tokens served from cache
	CacheWrite int64     `json:"cacheWrite"` // input tokens written to cache
	Reasoning  int64     `json:"reasoning"`  // reasoning tokens when reported separately
	Sessions   int64     `json:"sessions"`   // distinct sessions active in the hour
}

// NetSample is one latency probe result. Nil means unknown, never zero.
type NetSample struct {
	Machine  string    `json:"machine"`
	Target   string    `json:"target"` // "gateway" or a host such as "1.1.1.1"
	TS       time.Time `json:"ts"`
	RTTMs    *float64  `json:"rttMs"`
	JitterMs *float64  `json:"jitterMs"`
	LossPct  *float64  `json:"lossPct"`
}

// SpeedTest is one download/upload measurement. Nil means the leg failed.
type SpeedTest struct {
	Machine  string    `json:"machine"`
	TS       time.Time `json:"ts"`
	DownMbps *float64  `json:"downMbps"`
	UpMbps   *float64  `json:"upMbps"`
	Server   string    `json:"server"`
}

// GitHubDay is Keith's authored activity in one repo on one UTC day (YYYY-MM-DD).
type GitHubDay struct {
	Repo      string `json:"repo"` // owner/name
	Day       string `json:"day"`
	Commits   int64  `json:"commits"`
	Additions int64  `json:"additions"`
	Deletions int64  `json:"deletions"`
	PRsOpened int64  `json:"prsOpened"`
	PRsMerged int64  `json:"prsMerged"`
	Issues    int64  `json:"issues"`
	Reviews   int64  `json:"reviews"`
}

// UsageBatch is the body of POST /api/ingest/usage.
type UsageBatch struct {
	Machine string  `json:"machine"`
	Rows    []Usage `json:"rows"`
}

// NetBatch is the body of POST /api/ingest/net.
type NetBatch struct {
	Machine string      `json:"machine"`
	Samples []NetSample `json:"samples"`
	Speed   []SpeedTest `json:"speed"`
}

// IngestReply is the hub's JSON reply to both ingest endpoints.
type IngestReply struct {
	RunSpeedtest bool `json:"runSpeedtest"` // the dashboard asked this machine for a speed test now
}
