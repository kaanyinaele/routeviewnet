package collector

import (
	"testing"
	"time"
)

// Hostname lookups used to run serially under one shared 5s budget, so a
// handful of silent devices consumed it and every device after them got no
// hostname — deterministically the same ones, cycle after cycle. Answers are
// now cached, including misses, so repeat cycles cost nothing.
func TestHostnameCacheServesRepeatLookups(t *testing.T) {
	c := NewDeviceCollector()
	now := time.Now()

	if _, ok := c.cached("192.168.1.10", now); ok {
		t.Fatal("empty cache must report a miss")
	}

	c.store("192.168.1.10", "tv.local", now)
	name, ok := c.cached("192.168.1.10", now)
	if !ok || name != "tv.local" {
		t.Errorf("want cached tv.local, got %q ok=%v", name, ok)
	}

	// A device that did not answer is cached too, or it gets re-probed every
	// single cycle at full timeout cost.
	c.store("192.168.1.11", "", now)
	if _, ok := c.cached("192.168.1.11", now); !ok {
		t.Error("a miss must be cached so it is not re-probed every cycle")
	}
}

// Misses expire sooner than hits, so a device that comes online is picked up
// without re-probing silent ones constantly.
func TestHostnameCacheExpiry(t *testing.T) {
	c := NewDeviceCollector()
	now := time.Now()
	c.store("192.168.1.10", "tv.local", now)
	c.store("192.168.1.11", "", now)

	justAfterMiss := now.Add(c.MissTTL + time.Second)
	if _, ok := c.cached("192.168.1.11", justAfterMiss); ok {
		t.Error("a miss should expire after MissTTL")
	}
	if _, ok := c.cached("192.168.1.10", justAfterMiss); !ok {
		t.Error("a hit should outlive MissTTL")
	}

	justAfterHit := now.Add(c.HostnameTTL + time.Second)
	if _, ok := c.cached("192.168.1.10", justAfterHit); ok {
		t.Error("a hit should expire after HostnameTTL")
	}
}

// A network with churning DHCP leases must not grow the cache without bound.
func TestHostnameCachePruning(t *testing.T) {
	c := NewDeviceCollector()
	old := time.Now().Add(-3 * c.HostnameTTL)
	for i := 0; i < 100; i++ {
		c.store(string(rune('a'+i%26))+"-old", "name", old)
	}
	c.store("current", "name", time.Now())

	c.pruneCache(time.Now())

	c.mu.Lock()
	size := len(c.cache)
	c.mu.Unlock()
	if size != 1 {
		t.Errorf("stale entries should be pruned, %d left", size)
	}
	if _, ok := c.cached("current", time.Now()); !ok {
		t.Error("pruning must not evict a fresh entry")
	}
}
