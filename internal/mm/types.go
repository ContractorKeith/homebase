// Package mm is a client for the machine-monitor hub API. It decodes only what the
// dashboard shows; an unknown metric stays nil (never 0). The shapes are copied from
// machine-monitor's report and history packages, which are internal to that module.
package mm

// Device is one machine as /api/devices and the stream show it.
type Device struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Hostname *string `json:"hostname"`
	Platform string  `json:"platform"`
	OS       *string `json:"os"`
	Order    int     `json:"order"`
	LastSeen int64   `json:"lastSeen"` // Unix ms
	Online   bool    `json:"online"`
	Latest   *Latest `json:"latest"` // nil until the device has reported
}

// Latest is a device's last report.
type Latest struct {
	Uptime     *float64 `json:"uptime"` // seconds
	CPU        CPU      `json:"cpu"`
	GPUs       []GPU    `json:"gpus"`
	Temps      []Temp   `json:"temps"`
	Fans       []Fan    `json:"fans"`
	Net        *Net     `json:"net"`
	ReceivedAt int64    `json:"receivedAt"` // Unix ms
}

// CPU is processor load, clock and temperature. Percentages are 0-100.
type CPU struct {
	Name     *string    `json:"name"`
	Usage    *float64   `json:"usage"`
	Cores    []*float64 `json:"cores"`
	Threads  *float64   `json:"threads"`
	ClockMHz *float64   `json:"clockMHz"`
	Temp     *float64   `json:"temp"`
}

// GPU is one graphics adapter. Memory is in bytes, power in watts.
type GPU struct {
	Name     *string  `json:"name"`
	Usage    *float64 `json:"usage"`
	Temp     *float64 `json:"temp"`
	MemUsed  *float64 `json:"memUsed"`
	MemTotal *float64 `json:"memTotal"`
	Power    *float64 `json:"power"`
}

// Temp is one temperature sensor in °C.
type Temp struct {
	Name  *string  `json:"name"`
	Value *float64 `json:"value"`
}

// Fan is one fan sensor.
type Fan struct {
	Name *string  `json:"name"`
	RPM  *float64 `json:"rpm"`
}

// Net is total throughput in bytes per second.
type Net struct {
	RX *float64 `json:"rx"`
	TX *float64 `json:"tx"`
}

// Point is one history bucket. T is Unix ms.
type Point struct {
	T        float64  `json:"t"`
	CPU      *float64 `json:"cpu"`
	GPU      *float64 `json:"gpu"`
	CPUTemp  *float64 `json:"cpuTemp"`
	GPUTemp  *float64 `json:"gpuTemp"`
	GPUPower *float64 `json:"gpuPower"`
	RX       *float64 `json:"rx"`
	TX       *float64 `json:"tx"`
}

// History is the body of GET /api/history/:id. Resolution is the bucket width in ms.
type History struct {
	From       float64 `json:"from"`
	To         float64 `json:"to"`
	Resolution float64 `json:"resolution"`
	Points     []Point `json:"points"`
}
