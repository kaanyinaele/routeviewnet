package engine

import (
	"context"
	"fmt"
	"testing"
)

func (e *AlertEngine) counterCount() (fails, oks int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.failures), len(e.healthy)
}

// The debounce counters are keyed per rule+source, and the new-device rule's
// source is an ip/mac pair — one more key for every address a DHCP lease ever
// hands out. Steady-state healthy observations with nothing open carry no
// debounce state, so they must not accumulate.
func TestDebounceCountersDoNotGrowForHealthyObservations(t *testing.T) {
	eng, _ := newTestEngine(t)
	ctx := context.Background()

	for i := 0; i < 500; i++ {
		eng.Observe(ctx, Observation{
			RuleKey: RuleNewDevice,
			Source:  fmt.Sprintf("192.168.1.%d/aa:bb:cc:dd:ee:%02x", i%256, i%256),
			Failing: false,
		})
	}
	fails, oks := eng.counterCount()
	if fails != 0 || oks != 0 {
		t.Errorf("healthy observations with no open alert should leave no state, got %d failing / %d healthy keys", fails, oks)
	}
}

// While a debounce is genuinely in progress the counters must still be kept,
// or an alert would never trigger or resolve.
func TestDebounceCountersKeptWhileDebouncing(t *testing.T) {
	eng, db := newTestEngine(t)
	ctx := context.Background()

	// One failure: below the trigger threshold, so the count must survive.
	eng.Observe(ctx, failing(RuleDNSFailure))
	if fails, _ := eng.counterCount(); fails != 1 {
		t.Fatalf("a pending trigger debounce must be remembered, got %d keys", fails)
	}
	if n, _ := db.OpenAlertCount(ctx); n != 0 {
		t.Fatalf("want 0 open alerts mid-debounce, got %d", n)
	}

	// Second failure opens the alert and clears the failing counter's job.
	eng.Observe(ctx, failing(RuleDNSFailure))
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Fatalf("want 1 open alert, got %d", n)
	}

	// One healthy observation: resolve debounce in progress, must be kept.
	eng.Observe(ctx, healthy(RuleDNSFailure))
	if _, oks := eng.counterCount(); oks != 1 {
		t.Fatalf("a pending resolve debounce must be remembered, got %d keys", oks)
	}
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Fatalf("alert should still be open mid-resolve-debounce, got %d", n)
	}

	// Second healthy observation resolves it, and the state is dropped.
	eng.Observe(ctx, healthy(RuleDNSFailure))
	if n, _ := db.OpenAlertCount(ctx); n != 0 {
		t.Fatalf("alert should have resolved, got %d open", n)
	}
	eng.Observe(ctx, healthy(RuleDNSFailure))
	if fails, oks := eng.counterCount(); fails != 0 || oks != 0 {
		t.Errorf("after resolution no debounce state should linger, got %d/%d", fails, oks)
	}
}
