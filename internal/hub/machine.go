package hub

import (
	"sort"

	"github.com/ContractorKeith/homebase/internal/mm"
)

// Machine is the stable JSON shape of one card. Every metric may be null (unknown), and
// NetQ is the slot M3/M4 fill with latency, jitter and loss.
type Machine struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Platform string       `json:"platform"`
	Order    int          `json:"order"`
	Online   bool         `json:"online"`
	LastSeen int64        `json:"lastSeen"`
	Uptime   *float64     `json:"uptime"`
	CPU      MachineCPU   `json:"cpu"`
	GPUs     []MachineGPU `json:"gpus"`
	Temps    []NamedValue `json:"temps"`
	Fans     []NamedValue `json:"fans"`
	Net      MachineNet   `json:"net"`
	NetQ     any          `json:"netq"`
}

// MachineCPU is processor load (percent), clock (MHz) and package temperature (°C).
type MachineCPU struct {
	Usage    *float64   `json:"usage"`
	Cores    []*float64 `json:"cores"`
	Threads  *float64   `json:"threads"`
	ClockMHz *float64   `json:"clockMHz"`
	Temp     *float64   `json:"temp"`
}

// MachineGPU is one adapter; memory in bytes, power in watts.
type MachineGPU struct {
	Name     *string  `json:"name"`
	Usage    *float64 `json:"usage"`
	Temp     *float64 `json:"temp"`
	MemUsed  *float64 `json:"memUsed"`
	MemTotal *float64 `json:"memTotal"`
	Power    *float64 `json:"power"`
}

// NamedValue is a temperature (°C) or fan (RPM) reading.
type NamedValue struct {
	Name  *string  `json:"name"`
	Value *float64 `json:"value"`
}

// MachineNet is throughput in bytes per second.
type MachineNet struct {
	RX *float64 `json:"rx"`
	TX *float64 `json:"tx"`
}

// toMachine merges a machine-monitor device into the card shape. Lists are never null.
func toMachine(d mm.Device) Machine {
	m := Machine{ID: d.ID, Name: d.Name, Platform: d.Platform, Order: d.Order, Online: d.Online,
		LastSeen: d.LastSeen, CPU: MachineCPU{Cores: []*float64{}}, GPUs: []MachineGPU{}, Temps: []NamedValue{}, Fans: []NamedValue{}}
	l := d.Latest
	if l == nil {
		return m
	}
	m.Uptime = l.Uptime
	m.CPU = MachineCPU{Usage: l.CPU.Usage, Cores: l.CPU.Cores, Threads: l.CPU.Threads,
		ClockMHz: l.CPU.ClockMHz, Temp: l.CPU.Temp}
	if m.CPU.Cores == nil {
		m.CPU.Cores = []*float64{}
	}
	for _, g := range l.GPUs {
		m.GPUs = append(m.GPUs, MachineGPU(g))
	}
	for _, t := range l.Temps {
		m.Temps = append(m.Temps, NamedValue(t))
	}
	for _, f := range l.Fans {
		m.Fans = append(m.Fans, NamedValue{Name: f.Name, Value: f.RPM})
	}
	if l.Net != nil {
		m.Net = MachineNet(*l.Net)
	}
	return m
}

// machines converts and orders a device list.
func machines(devs []mm.Device) []Machine {
	out := make([]Machine, 0, len(devs))
	for _, d := range devs {
		out = append(out, toMachine(d))
	}
	sortMachines(out)
	return out
}

// sortMachines orders cards by machine-monitor's order, then name, so the layout is stable.
func sortMachines(ms []Machine) {
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].Order != ms[j].Order {
			return ms[i].Order < ms[j].Order
		}
		return ms[i].Name < ms[j].Name
	})
}
