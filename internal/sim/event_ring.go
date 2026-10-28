package sim

import (
	"sync"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// DefaultEventRingCapacity is the in-memory history kept for tutor tools.
// ~50 KB at the average wire-event size; covers ~1 second of run time at
// 1000 Hz, more than enough for "what happened in the last context switch?"
// questions. Bigger values trade memory for longer lookback.
const DefaultEventRingCapacity = 1000

// EventRing is a thread-safe circular buffer of the most recent CPU events
// the simulator has produced. The tutor's get_recent_events tool reads from
// it without ever blocking the run loop: pushes are O(1) under a small mutex.
type EventRing struct {
	mu     sync.RWMutex
	buf    []ringEntry
	head   int  // next write position
	filled bool // true once we wrap past capacity
}

// ringEntry stores both the wire form (what the WS already sends) and the
// raw type+tick so filters can run without re-decoding the map.
type ringEntry struct {
	Type string
	Tick uint64
	Wire map[string]any
}

// NewEventRing builds an empty ring of the requested capacity.
func NewEventRing(capacity int) *EventRing {
	if capacity <= 0 {
		capacity = DefaultEventRingCapacity
	}
	return &EventRing{buf: make([]ringEntry, capacity)}
}

// push appends one event. Overwrites the oldest entry when full.
func (r *EventRing) push(ev cpu.Event, tick uint64) {
	if r == nil {
		return
	}
	wire := EventToWire(ev)
	typ, _ := wire["type"].(string)
	r.mu.Lock()
	r.buf[r.head] = ringEntry{Type: typ, Tick: tick, Wire: wire}
	r.head++
	if r.head >= len(r.buf) {
		r.head = 0
		r.filled = true
	}
	r.mu.Unlock()
}

// Clear drops all retained history. Called on load/reset so tutor tools never
// surface a previous demo's events under the restarted clock.
func (r *EventRing) Clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.head = 0
	r.filled = false
	for i := range r.buf {
		r.buf[i] = ringEntry{}
	}
	r.mu.Unlock()
}

// Recent returns up to n entries whose Type matches one of `types` (or every
// entry if `types` is empty). Newest-first ordering.
func (r *EventRing) Recent(types []string, n int) []map[string]any {
	if r == nil || n <= 0 {
		return nil
	}
	want := make(map[string]struct{}, len(types))
	for _, t := range types {
		if t != "" {
			want[t] = struct{}{}
		}
	}
	filter := len(want) > 0

	r.mu.RLock()
	defer r.mu.RUnlock()

	size := len(r.buf)
	count := r.head
	if r.filled {
		count = size
	}
	out := make([]map[string]any, 0, min(n, count))

	// Walk backwards from head-1 (newest) to head (oldest).
	for i := 0; i < count && len(out) < n; i++ {
		idx := r.head - 1 - i
		if idx < 0 {
			idx += size
		}
		e := r.buf[idx]
		if filter {
			if _, ok := want[e.Type]; !ok {
				continue
			}
		}
		out = append(out, e.Wire)
	}
	return out
}

// Len reports how many entries are currently held.
func (r *EventRing) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.filled {
		return len(r.buf)
	}
	return r.head
}
