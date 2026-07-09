// Package engine holds the interpretation layer: event bus, alert engine
// with debounce/dedup (§8.4), health score (§6), and explanations (§8.5).
package engine

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is the wire format broadcast to WebSocket clients (§10.3).
type Event struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// Bus is a broadcast fan-out event bus (§10.3.3). Subscribers with full
// buffers are skipped (slow clients must not back-pressure collectors).
type Bus struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewBus() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

func (b *Bus) Subscribe() chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Bus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
	// Drain so a concurrent Publish never blocks on a dead channel.
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func (b *Bus) Publish(typ string, payload interface{}) {
	ev := Event{Type: typ, Timestamp: time.Now().UTC(), Payload: payload}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // drop for slow subscribers (§10.3.4)
		}
	}
}

// MarshalPayload renders a payload for the events table.
func MarshalPayload(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
