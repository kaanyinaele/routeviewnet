package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"routeviewnet/internal/models"
)

// --- system memory (§7.6) ---

func (d *DB) InsertMemoryMetrics(ctx context.Context, m models.MemoryMetrics) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO system_memory_metrics
		  (mem_total_bytes, mem_available_bytes, mem_free_bytes, mem_used_bytes, buffers_bytes,
		   cached_bytes, swap_total_bytes, swap_free_bytes, swap_used_bytes,
		   memory_usage_percent, swap_usage_percent, collected_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.MemTotal, m.MemAvailable, m.MemFree, m.MemUsed, m.Buffers, m.Cached,
		m.SwapTotal, m.SwapFree, m.SwapUsed, m.MemoryPercent, m.SwapPercent, ts(m.CollectedAt))
	return err
}

func (d *DB) LatestMemoryMetrics(ctx context.Context) (*models.MemoryMetrics, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT mem_total_bytes, mem_available_bytes, mem_free_bytes, mem_used_bytes, buffers_bytes,
		       cached_bytes, swap_total_bytes, swap_free_bytes, swap_used_bytes,
		       memory_usage_percent, swap_usage_percent, collected_at
		FROM system_memory_metrics ORDER BY id DESC LIMIT 1`)
	var m models.MemoryMetrics
	var at string
	err := row.Scan(&m.MemTotal, &m.MemAvailable, &m.MemFree, &m.MemUsed, &m.Buffers, &m.Cached,
		&m.SwapTotal, &m.SwapFree, &m.SwapUsed, &m.MemoryPercent, &m.SwapPercent, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.CollectedAt = parseTS(at)
	return &m, nil
}

// MemoryHistoryPoint is one downsampled bucket of memory usage.
type MemoryHistoryPoint struct {
	Time          time.Time `json:"time"`
	MemoryPercent float64   `json:"memory_usage_percent"`
	SwapPercent   float64   `json:"swap_usage_percent"`
	MemUsedBytes  float64   `json:"mem_used_bytes"`
}

func (d *DB) MemoryHistory(ctx context.Context, since time.Time, bucketSec int) ([]MemoryHistoryPoint, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT (strftime('%s', collected_at)/?)*? AS bucket,
		       AVG(memory_usage_percent), AVG(swap_usage_percent), AVG(mem_used_bytes)
		FROM system_memory_metrics WHERE collected_at >= ?
		GROUP BY bucket ORDER BY bucket`, bucketSec, bucketSec, ts(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryHistoryPoint
	for rows.Next() {
		var epoch int64
		var p MemoryHistoryPoint
		if err := rows.Scan(&epoch, &p.MemoryPercent, &p.SwapPercent, &p.MemUsedBytes); err != nil {
			return nil, err
		}
		p.Time = time.Unix(epoch, 0).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- load average (§7.7) ---

func (d *DB) InsertLoadMetrics(ctx context.Context, m models.LoadMetrics) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO system_load_metrics (load_avg_1m, load_avg_5m, load_avg_15m, cpu_core_count, collected_at)
		VALUES (?,?,?,?,?)`,
		m.Load1, m.Load5, m.Load15, m.CPUCores, ts(m.CollectedAt))
	return err
}

func (d *DB) LatestLoadMetrics(ctx context.Context) (*models.LoadMetrics, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT load_avg_1m, load_avg_5m, load_avg_15m, cpu_core_count, collected_at
		FROM system_load_metrics ORDER BY id DESC LIMIT 1`)
	var m models.LoadMetrics
	var at string
	err := row.Scan(&m.Load1, &m.Load5, &m.Load15, &m.CPUCores, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.CollectedAt = parseTS(at)
	return &m, nil
}

// --- process memory (§7.8) ---

func (d *DB) InsertProcessMemory(ctx context.Context, ps []models.ProcessMemory) error {
	if len(ps) == 0 {
		return nil
	}
	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO process_memory_metrics (pid, process_name, command, rss_bytes, virtual_memory_bytes, memory_percent, collected_at)
		VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range ps {
		if _, err := stmt.ExecContext(ctx, p.PID, p.Name, p.Command, p.RSSBytes, p.VirtualBytes, p.MemoryPercent, ts(p.CollectedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LatestProcessMemory returns the most recent top-N scan.
func (d *DB) LatestProcessMemory(ctx context.Context) ([]models.ProcessMemory, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT pid, process_name, COALESCE(command,''), rss_bytes, virtual_memory_bytes, memory_percent, collected_at
		FROM process_memory_metrics
		WHERE collected_at = (SELECT MAX(collected_at) FROM process_memory_metrics)
		ORDER BY rss_bytes DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProcessMemory
	for rows.Next() {
		var p models.ProcessMemory
		var at string
		if err := rows.Scan(&p.PID, &p.Name, &p.Command, &p.RSSBytes, &p.VirtualBytes, &p.MemoryPercent, &at); err != nil {
			return nil, err
		}
		p.CollectedAt = parseTS(at)
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- daemon memory (§7.9) ---

func (d *DB) InsertDaemonMemory(ctx context.Context, m models.DaemonMemory) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO daemon_memory_metrics (rss_bytes, heap_alloc_bytes, heap_sys_bytes, sys_bytes, goroutines, collected_at)
		VALUES (?,?,?,?,?,?)`,
		m.RSSBytes, m.HeapAlloc, m.HeapSys, m.SysBytes, m.Goroutines, ts(m.CollectedAt))
	return err
}

func (d *DB) LatestDaemonMemory(ctx context.Context) (*models.DaemonMemory, error) {
	row := d.read.QueryRowContext(ctx, `
		SELECT rss_bytes, heap_alloc_bytes, heap_sys_bytes, sys_bytes, goroutines, collected_at
		FROM daemon_memory_metrics ORDER BY id DESC LIMIT 1`)
	var m models.DaemonMemory
	var at string
	err := row.Scan(&m.RSSBytes, &m.HeapAlloc, &m.HeapSys, &m.SysBytes, &m.Goroutines, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.CollectedAt = parseTS(at)
	return &m, nil
}

// --- health score history (§6.3, §9.2.13) ---

func (d *DB) InsertHealthScore(ctx context.Context, h models.HealthScore) error {
	_, err := d.write.ExecContext(ctx,
		`INSERT INTO health_score_history (score, status, computed_at) VALUES (?,?,?)`,
		h.Score, h.Status, ts(h.ComputedAt))
	return err
}

func (d *DB) HealthScoreHistory(ctx context.Context, since time.Time, bucketSec int) ([]models.HealthScore, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT (strftime('%s', computed_at)/?)*? AS bucket, CAST(MIN(score) AS INTEGER)
		FROM health_score_history WHERE computed_at >= ?
		GROUP BY bucket ORDER BY bucket`, bucketSec, bucketSec, ts(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.HealthScore
	for rows.Next() {
		var epoch int64
		var h models.HealthScore
		if err := rows.Scan(&epoch, &h.Score); err != nil {
			return nil, err
		}
		h.ComputedAt = time.Unix(epoch, 0).UTC()
		h.Status = models.HealthStatusFor(h.Score)
		out = append(out, h)
	}
	return out, rows.Err()
}

func (d *DB) LatestHealthScore(ctx context.Context) (*models.HealthScore, error) {
	row := d.read.QueryRowContext(ctx,
		`SELECT score, status, computed_at FROM health_score_history ORDER BY id DESC LIMIT 1`)
	var h models.HealthScore
	var at string
	err := row.Scan(&h.Score, &h.Status, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h.ComputedAt = parseTS(at)
	return &h, nil
}

// --- settings overrides (§9.2.12): JSON blob of the applied config ---

func (d *DB) SaveSetting(ctx context.Context, key, value string) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, ts(time.Now()))
	return err
}

func (d *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := d.read.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}
