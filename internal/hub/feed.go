package hub

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ContractorKeith/homebase/internal/mm"
)

// subBuffer is how many messages a browser may fall behind before it is dropped; its
// EventSource reconnects and gets a fresh snapshot.
const subBuffer = 64

// feed follows the machine-monitor stream, keeps the latest card per machine and fans
// updates out to browsers.
type feed struct {
	mu       sync.Mutex
	machines map[string]Machine
	ready    bool   // a snapshot has arrived
	mmErr    string // why machine-monitor is unreachable; empty while connected
	subs     map[chan []byte]struct{}
}

func newFeed() *feed {
	return &feed{machines: map[string]Machine{}, subs: map[chan []byte]struct{}{}}
}

// run consumes mm events until ctx ends.
func (f *feed) run(ctx context.Context, src Source) {
	for e := range src.Stream(ctx) {
		f.apply(e)
	}
}

func (f *feed) apply(e mm.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch e.Type {
	case mm.EventConnected:
		f.mmErr = ""
		f.broadcast(map[string]any{"type": "mm", "connected": true})
	case mm.EventDisconnected:
		f.mmErr = "machine-monitor unreachable"
		if e.Err != nil {
			f.mmErr = e.Err.Error()
		}
		f.broadcast(map[string]any{"type": "mm", "connected": false, "error": f.mmErr})
	case mm.EventSnapshot:
		f.machines = map[string]Machine{}
		for _, m := range machines(e.Devices) {
			f.machines[m.ID] = m
		}
		f.ready = true
		f.broadcast(f.snapshot())
	case mm.EventReport, mm.EventDevice:
		if e.Device == nil {
			return
		}
		m := toMachine(*e.Device)
		f.machines[m.ID] = m
		f.broadcast(map[string]any{"type": "machine", "machine": m})
	case mm.EventStatus:
		for id, online := range e.Online {
			if m, ok := f.machines[id]; ok {
				m.Online = online
				f.machines[id] = m
			}
		}
		f.broadcast(map[string]any{"type": "status", "online": e.Online})
	case mm.EventRemoved:
		delete(f.machines, e.ID)
		f.broadcast(map[string]any{"type": "removed", "id": e.ID})
	}
}

// snapshot is the browser's opening message. Callers hold f.mu.
func (f *feed) snapshot() map[string]any {
	out := make([]Machine, 0, len(f.machines))
	for _, m := range f.machines {
		out = append(out, m)
	}
	sortMachines(out)
	return map[string]any{"type": "snapshot", "machines": out}
}

// current returns the cached cards; ok is false until the first snapshot.
func (f *feed) current() (list []Machine, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ready {
		return nil, false
	}
	return f.snapshot()["machines"].([]Machine), true
}

// broadcast sends v to every subscriber, dropping any that cannot keep up. Callers hold f.mu.
func (f *feed) broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	msg := append(append([]byte("data: "), b...), '\n', '\n')
	for ch := range f.subs {
		select {
		case ch <- msg:
		default:
			delete(f.subs, ch)
			close(ch)
		}
	}
}

// subscribe returns the opening snapshot message and a channel of later ones; the channel
// closes if the subscriber falls behind. Call cancel when done.
func (f *feed) subscribe() (first []byte, ch chan []byte, cancel func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := json.Marshal(f.snapshot())
	first = append(append([]byte("data: "), b...), '\n', '\n')
	ch = make(chan []byte, subBuffer)
	f.subs[ch] = struct{}{}
	if f.mmErr != "" {
		b, _ := json.Marshal(map[string]any{"type": "mm", "connected": false, "error": f.mmErr})
		ch <- append(append([]byte("data: "), b...), '\n', '\n')
	}
	return first, ch, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.subs[ch]; ok {
			delete(f.subs, ch)
			close(ch)
		}
	}
}
