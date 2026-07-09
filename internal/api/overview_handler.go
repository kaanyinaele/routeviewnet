package api

import (
	"net/http"
	"time"

	"routeviewnet/internal/engine"
	"routeviewnet/internal/models"
)

// statusSummary condenses the latest checks into per-area status strings.
type statusSummary struct {
	Status    string  `json:"status"` // ok | degraded | failing | unknown
	LatencyMs float64 `json:"latency_ms"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"version":        s.Version,
		"uptime_seconds": int(time.Since(s.Started).Seconds()),
		"db_writable":    s.DB.Writable(r.Context()),
		"ping_mode":      s.Mgr.PingMode(),
	})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	score, _ := s.DB.LatestHealthScore(ctx)
	if score == nil {
		open, _ := s.DB.OpenAlerts(ctx)
		computed := engine.ComputeScore(open)
		score = &computed
	}

	gateway := statusSummary{Status: "unknown"}
	internet := statusSummary{Status: "unknown"}
	dns := statusSummary{Status: "unknown"}

	if latest, err := s.DB.LatestLatencyByTarget(ctx); err == nil {
		var internetChecks, internetOK int
		var internetLatency float64
		for _, c := range latest {
			switch c.TargetType {
			case "gateway":
				gateway = checkStatus(c.Success, c.LatencyMs)
			case "internet":
				internetChecks++
				if c.Success {
					internetOK++
					internetLatency += c.LatencyMs
				}
			}
		}
		if internetChecks > 0 {
			switch {
			case internetOK == 0:
				internet = statusSummary{Status: "failing"}
			case internetOK < internetChecks:
				internet = statusSummary{Status: "degraded", LatencyMs: internetLatency / float64(internetOK)}
			default:
				internet = statusSummary{Status: "ok", LatencyMs: internetLatency / float64(internetOK)}
			}
		}
	}

	if checks, err := s.DB.RecentDNSChecks(ctx, time.Now().Add(-2*time.Minute), 10); err == nil && len(checks) > 0 {
		var ok, total int
		var lat float64
		for _, c := range checks {
			if c.RecordType != "A" {
				continue
			}
			total++
			if c.Success {
				ok++
				lat += c.LatencyMs
			}
		}
		if total > 0 {
			switch {
			case ok == 0:
				dns = statusSummary{Status: "failing"}
			case ok < total:
				dns = statusSummary{Status: "degraded", LatencyMs: lat / float64(ok)}
			default:
				dns = statusSummary{Status: "ok", LatencyMs: lat / float64(ok)}
			}
		}
	}

	primary := s.Mgr.Primary()
	var bandwidth *models.InterfaceMetric
	if latest, err := s.DB.LatestInterfaceMetrics(ctx); err == nil {
		if m, ok := latest[primary]; ok {
			bandwidth = &m
		}
	}

	memory, _ := s.DB.LatestMemoryMetrics(ctx)
	daemonMem, _ := s.DB.LatestDaemonMemory(ctx)
	load, _ := s.DB.LatestLoadMetrics(ctx)
	deviceCount, _ := s.DB.DeviceCount(ctx)
	alertCount, _ := s.DB.OpenAlertCount(ctx)
	events, _, _ := s.DB.ListEvents(ctx, 10, 0)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"health":            score,
		"primary_interface": primary,
		"gateway_ip":        s.Mgr.Gateway(),
		"bandwidth":         bandwidth,
		"gateway":           gateway,
		"internet":          internet,
		"dns":               dns,
		"memory":            memory,
		"daemon_memory":     daemonMem,
		"load":              load,
		"device_count":      deviceCount,
		"open_alert_count":  alertCount,
		"recent_events":     events,
	})
}

func checkStatus(success bool, latency float64) statusSummary {
	if !success {
		return statusSummary{Status: "failing"}
	}
	return statusSummary{Status: "ok", LatencyMs: latency}
}

func (s *Server) handleTroubleshoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, engine.Diagnose(r.Context(), s.DB))
}
