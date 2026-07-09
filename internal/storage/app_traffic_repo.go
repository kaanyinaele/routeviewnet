package storage

import (
	"context"
	"time"

	"routeviewnet/internal/models"
)

// --- per-app traffic (Data usage by app) ---

func (d *DB) InsertAppTraffic(ctx context.Context, rows []models.AppTraffic) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO app_traffic_metrics (app_name, rx_bytes, tx_bytes, collected_at)
		VALUES (?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.App, r.RxBytes, r.TxBytes, ts(r.CollectedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AppTrafficTotals aggregates per-app usage since the given time, heaviest
// apps first.
func (d *DB) AppTrafficTotals(ctx context.Context, since time.Time) ([]models.AppTrafficTotal, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT app_name, SUM(rx_bytes), SUM(tx_bytes)
		FROM app_traffic_metrics
		WHERE collected_at >= ?
		GROUP BY app_name
		ORDER BY SUM(rx_bytes) + SUM(tx_bytes) DESC`, ts(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AppTrafficTotal
	for rows.Next() {
		var t models.AppTrafficTotal
		if err := rows.Scan(&t.App, &t.RxBytes, &t.TxBytes); err != nil {
			return nil, err
		}
		t.TotalBytes = t.RxBytes + t.TxBytes
		out = append(out, t)
	}
	return out, rows.Err()
}
