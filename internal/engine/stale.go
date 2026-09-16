package engine

import (
	"context"
	"log/slog"
	"time"
)

// Stale-alert sweep timing.
const (
	// SweepEvery is how often the scheduler offers a sweep.
	SweepEvery = time.Minute
	// minStaleAfter is the floor on how long an alert may go unobserved. It
	// sits above the new-device alert's 10-minute freshness window, during
	// which that alert is deliberately not re-observed.
	minStaleAfter = 15 * time.Minute
	// minSweepGrace is the floor on the pause after startup or a resume.
	minSweepGrace = time.Minute
)

// SweepTiming derives the stale window and the post-gap grace period from the
// longest interval of any collector that raises alerts. Intervals have no
// upper bound in config, so fixed constants alone would sweep alerts that are
// still failing but checked, say, once an hour.
func SweepTiming(longestInterval time.Duration) (staleAfter, grace time.Duration) {
	staleAfter = max(minStaleAfter, 5*longestInterval)
	grace = max(minSweepGrace, 2*longestInterval)
	return staleAfter, grace
}

// StaleSweeper decides when it is safe to call SweepStale.
//
// The sweep is only honest once every collector has had a chance to re-observe
// the sources that are still failing. That is not true straight after the
// daemon starts, nor straight after the machine wakes from sleep: in both cases
// every alert's updated_at is old, including alerts whose source is failing
// right now and is about to be touched. Sweeping then would resolve them, and
// the next cycle would reopen them — a spurious resolved/opened pair, and a
// webhook notification for each half.
//
// So after either kind of gap the sweeper waits out a grace period first.
//
// Gaps are measured on the wall clock. Go's monotonic clock does not advance
// while a Linux machine is suspended, so a monotonic measurement would not
// notice that an hour of sleep had passed.
type StaleSweeper struct {
	Sweep func(ctx context.Context, cutoff time.Time) (int, error)
	Log   *slog.Logger
	Now   func() time.Time

	lastTick   time.Time
	graceUntil time.Time
}

// NewStaleSweeper wires a sweeper to an alert engine.
func NewStaleSweeper(e *AlertEngine, log *slog.Logger) *StaleSweeper {
	return &StaleSweeper{Sweep: e.SweepStale, Log: log, Now: time.Now}
}

// Tick runs one sweep opportunity. every is the expected spacing between
// ticks; a longer wall-clock gap means the daemon just started or the machine
// was asleep.
func (s *StaleSweeper) Tick(ctx context.Context, every, staleAfter, grace time.Duration) {
	now := s.Now().Round(0) // strip the monotonic reading: see the type comment
	if s.lastTick.IsZero() || now.Sub(s.lastTick) > 2*every {
		s.graceUntil = now.Add(grace)
	}
	s.lastTick = now
	if now.Before(s.graceUntil) {
		return
	}
	n, err := s.Sweep(ctx, now.Add(-staleAfter))
	if err != nil {
		s.Log.Warn("stale alert sweep failed", "error", err)
		return
	}
	if n > 0 {
		s.Log.Info("resolved alerts whose source is no longer observed",
			"count", n, "unobserved_for", staleAfter.String())
	}
}
