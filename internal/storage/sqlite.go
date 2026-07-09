// Package storage owns the SQLite database: WAL-mode setup (§9.1),
// versioned migrations (§9.4), repositories, and retention cleanup (§9.3).
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// timeFormat is how all timestamps are stored: UTC, second precision (§14.4).
const timeFormat = "2006-01-02 15:04:05"

func ts(t time.Time) string { return t.UTC().Format(timeFormat) }

// parseTS accepts both storage formats: our canonical "2006-01-02 15:04:05"
// TEXT, and RFC3339 — which the driver produces when a DATETIME-declared
// column passes through database/sql's time.Time conversion.
func parseTS(s string) time.Time {
	if t, err := time.Parse(timeFormat, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// DB wraps a single-writer / multi-reader pair of connection pools so
// collector writes never contend with each other (§9.1).
type DB struct {
	write *sql.DB
	read  *sql.DB
	log   *slog.Logger
}

func Open(path string, log *slog.Logger) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"

	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(4)

	db := &DB{write: write, read: read, log: log}
	if err := db.write.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// Owner-only DB file permissions (§9.1).
	_ = os.Chmod(path, 0o600)
	if err := db.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) Close() error {
	if d.read != nil {
		d.read.Close()
	}
	if d.write != nil {
		return d.write.Close()
	}
	return nil
}

// Writable reports whether the DB currently accepts writes (used by
// GET /api/v1/health).
func (d *DB) Writable(ctx context.Context) bool {
	_, err := d.write.ExecContext(ctx, `UPDATE schema_migrations SET version=version WHERE 0`)
	return err == nil
}

// retentionTables are the time-series tables purged by the retention job
// (§9.3), mapped to their timestamp column.
var retentionTables = map[string]string{
	"interface_metrics":      "collected_at",
	"latency_checks":         "collected_at",
	"dns_checks":             "collected_at",
	"connections":            "collected_at",
	"system_memory_metrics":  "collected_at",
	"process_memory_metrics": "collected_at",
	"daemon_memory_metrics":  "collected_at",
	"system_load_metrics":    "collected_at",
	"app_traffic_metrics":    "collected_at",
	"health_score_history":   "computed_at",
	"events":                 "created_at",
}

// Cleanup deletes rows older than the retention window in bounded batches
// (§9.3) so a large backlog never holds the write path. Open alerts are
// never purged.
func (d *DB) Cleanup(ctx context.Context, retentionDays int) error {
	cutoff := ts(time.Now().AddDate(0, 0, -retentionDays))
	const batch = 5000
	for table, col := range retentionTables {
		for {
			res, err := d.write.ExecContext(ctx, fmt.Sprintf(
				`DELETE FROM %s WHERE id IN (SELECT id FROM %s WHERE %s < ? LIMIT %d)`,
				table, table, col, batch), cutoff)
			if err != nil {
				return fmt.Errorf("cleanup %s: %w", table, err)
			}
			n, _ := res.RowsAffected()
			if n > 0 {
				d.log.Debug("retention cleanup", "table", table, "deleted", n)
			}
			if n < batch {
				break
			}
		}
	}
	// Resolved alerts age out with the same window; open alerts never do.
	_, err := d.write.ExecContext(ctx,
		`DELETE FROM alerts WHERE status='resolved' AND updated_at < ?`, cutoff)
	return err
}
