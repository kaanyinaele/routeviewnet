package engine

import (
	"context"
	"log/slog"
	"time"

	"routeviewnet/internal/models"
	"routeviewnet/internal/storage"
)

// penalty maps rule keys to their category and weight (§6.2).
type penalty struct {
	category string
	points   int
}

var penalties = map[string]penalty{
	RuleGatewayUnreachable:  {"connectivity", 40},
	RuleInternetUnreachable: {"connectivity", 30},
	RuleHighPacketLoss:      {"connectivity", 15},
	RuleDNSFailure:          {"dns", 20},
	RuleHighDNSLatency:      {"dns", 10},
	RuleInterfaceDown:       {"interface", 15},
	RuleDroppedIncreasing:   {"interface", 10},
	RuleHighMemory:          {"memory", 15},
	RuleHighSwap:            {"memory", 10},
	RuleDaemonMemoryHigh:    {"memory", 5},
	RuleTrafficSpike:        {"traffic", 5},
	RuleHighLoad:            {"memory", 10}, // load shares the memory/system category cap
}

var categoryCaps = map[string]int{
	"connectivity": 40,
	"dns":          25,
	"interface":    20,
	"memory":       20,
	"traffic":      5,
}

// ComputeScore is the pure §6.2 formula: reproducible from open alerts alone.
func ComputeScore(open []models.Alert) models.HealthScore {
	byCat := map[string]int{}
	seen := map[string]bool{} // a rule contributes its penalty once, however many sources
	for _, a := range open {
		p, ok := penalties[a.RuleKey]
		if !ok || seen[a.RuleKey] {
			continue
		}
		seen[a.RuleKey] = true
		byCat[p.category] += p.points
	}
	total := 0
	for cat, pts := range byCat {
		if cap := categoryCaps[cat]; pts > cap {
			pts = cap
		}
		total += pts
	}
	score := 100 - total
	if score < 0 {
		score = 0
	}
	return models.HealthScore{
		Score:      score,
		Status:     models.HealthStatusFor(score),
		ComputedAt: time.Now().UTC(),
	}
}

// HealthEngine recomputes and persists the score on alert changes (§6.3).
type HealthEngine struct {
	db  *storage.DB
	bus *Bus
	log *slog.Logger
}

func NewHealthEngine(db *storage.DB, bus *Bus, log *slog.Logger) *HealthEngine {
	return &HealthEngine{db: db, bus: bus, log: log}
}

func (h *HealthEngine) Recompute(ctx context.Context) {
	open, err := h.db.OpenAlerts(ctx)
	if err != nil {
		h.log.Warn("health recompute: open alerts query failed", "error", err)
		return
	}
	score := ComputeScore(open)
	if err := h.db.InsertHealthScore(ctx, score); err != nil {
		h.log.Warn("health score insert failed", "error", err)
	}
	h.bus.Publish("health.updated", score)
}
