package storage

import (
	"context"
	"database/sql"
	"time"

	"routeviewnet/internal/models"
)

// --- interfaces & interface metrics (§7.1, §9.2.1–2) ---

func (d *DB) UpsertInterface(ctx context.Context, iface models.Interface) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO interfaces (name, mac_address, ip_address, state, speed_mbps, is_primary, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
		  mac_address=excluded.mac_address, ip_address=excluded.ip_address,
		  state=excluded.state, speed_mbps=excluded.speed_mbps,
		  is_primary=excluded.is_primary, updated_at=excluded.updated_at`,
		iface.Name, iface.MAC, iface.IP, iface.State, iface.SpeedMbps, iface.IsPrimary, ts(time.Now()))
	return err
}

func (d *DB) ListInterfaces(ctx context.Context) ([]models.Interface, error) {
	rows, err := d.read.QueryContext(ctx,
		`SELECT id, name, COALESCE(mac_address,''), COALESCE(ip_address,''), COALESCE(state,''),
		        COALESCE(speed_mbps,0), is_primary FROM interfaces ORDER BY is_primary DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Interface
	for rows.Next() {
		var i models.Interface
		if err := rows.Scan(&i.ID, &i.Name, &i.MAC, &i.IP, &i.State, &i.SpeedMbps, &i.IsPrimary); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (d *DB) InsertInterfaceMetrics(ctx context.Context, ms []models.InterfaceMetric) error {
	if len(ms) == 0 {
		return nil
	}
	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO interface_metrics
		  (interface_name, rx_bytes, tx_bytes, rx_packets, tx_packets, rx_errors, tx_errors,
		   rx_dropped, tx_dropped, rx_rate_bps, tx_rate_bps, collected_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, m := range ms {
		var rx, tx2 interface{}
		if m.HasRates {
			rx, tx2 = m.RxRateBps, m.TxRateBps
		}
		if _, err := stmt.ExecContext(ctx, m.Name, m.RxBytes, m.TxBytes, m.RxPackets, m.TxPackets,
			m.RxErrors, m.TxErrors, m.RxDropped, m.TxDropped, rx, tx2, ts(m.CollectedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BandwidthPoint is one downsampled bucket of an interface's rate history.
type BandwidthPoint struct {
	Time      time.Time `json:"time"`
	RxRateBps float64   `json:"rx_rate_bps"`
	TxRateBps float64   `json:"tx_rate_bps"`
	RxDropped float64   `json:"rx_dropped"`
	TxDropped float64   `json:"tx_dropped"`
	RxErrors  float64   `json:"rx_errors"`
	TxErrors  float64   `json:"tx_errors"`
}

// BandwidthHistory returns bucket-averaged rate history for one interface
// (§10.1.4). bucketSec comes from BucketSeconds.
func (d *DB) BandwidthHistory(ctx context.Context, iface string, since time.Time, bucketSec int) ([]BandwidthPoint, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT (strftime('%s', collected_at)/?)*? AS bucket,
		       AVG(COALESCE(rx_rate_bps,0)), AVG(COALESCE(tx_rate_bps,0)),
		       MAX(rx_dropped), MAX(tx_dropped), MAX(rx_errors), MAX(tx_errors)
		FROM interface_metrics
		WHERE interface_name = ? AND collected_at >= ?
		GROUP BY bucket ORDER BY bucket`,
		bucketSec, bucketSec, iface, ts(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BandwidthPoint
	for rows.Next() {
		var epoch int64
		var p BandwidthPoint
		if err := rows.Scan(&epoch, &p.RxRateBps, &p.TxRateBps, &p.RxDropped, &p.TxDropped, &p.RxErrors, &p.TxErrors); err != nil {
			return nil, err
		}
		p.Time = time.Unix(epoch, 0).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- latency & DNS checks (§7.3, §7.4) ---

func (d *DB) InsertLatencyCheck(ctx context.Context, c models.LatencyCheck) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO latency_checks (target, target_type, method, latency_ms, packet_loss, success, error_message, collected_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		c.Target, c.TargetType, c.Method, c.LatencyMs, c.PacketLoss, c.Success, c.Error, ts(c.CollectedAt))
	return err
}

func (d *DB) RecentLatencyChecks(ctx context.Context, since time.Time, limit int) ([]models.LatencyCheck, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT target, COALESCE(target_type,''), COALESCE(method,''), COALESCE(latency_ms,0),
		       COALESCE(packet_loss,0), success, COALESCE(error_message,''), collected_at
		FROM latency_checks WHERE collected_at >= ? ORDER BY id DESC LIMIT ?`, ts(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.LatencyCheck
	for rows.Next() {
		var c models.LatencyCheck
		var at string
		if err := rows.Scan(&c.Target, &c.TargetType, &c.Method, &c.LatencyMs, &c.PacketLoss, &c.Success, &c.Error, &at); err != nil {
			return nil, err
		}
		c.CollectedAt = parseTS(at)
		out = append(out, c)
	}
	return out, rows.Err()
}

// LatestLatencyByTarget returns the most recent check per target.
func (d *DB) LatestLatencyByTarget(ctx context.Context) (map[string]models.LatencyCheck, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT target, COALESCE(target_type,''), COALESCE(method,''), COALESCE(latency_ms,0),
		       COALESCE(packet_loss,0), success, COALESCE(error_message,''), collected_at
		FROM latency_checks
		WHERE id IN (SELECT MAX(id) FROM latency_checks GROUP BY target)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.LatencyCheck{}
	for rows.Next() {
		var c models.LatencyCheck
		var at string
		if err := rows.Scan(&c.Target, &c.TargetType, &c.Method, &c.LatencyMs, &c.PacketLoss, &c.Success, &c.Error, &at); err != nil {
			return nil, err
		}
		c.CollectedAt = parseTS(at)
		out[c.Target] = c
	}
	return out, rows.Err()
}

func (d *DB) InsertDNSCheck(ctx context.Context, c models.DNSCheck) error {
	_, err := d.write.ExecContext(ctx, `
		INSERT INTO dns_checks (domain, record_type, resolver, latency_ms, success, error_message, collected_at)
		VALUES (?,?,?,?,?,?,?)`,
		c.Domain, c.RecordType, c.Resolver, c.LatencyMs, c.Success, c.Error, ts(c.CollectedAt))
	return err
}

func (d *DB) RecentDNSChecks(ctx context.Context, since time.Time, limit int) ([]models.DNSCheck, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT domain, COALESCE(record_type,'A'), COALESCE(resolver,''), COALESCE(latency_ms,0),
		       success, COALESCE(error_message,''), collected_at
		FROM dns_checks WHERE collected_at >= ? ORDER BY id DESC LIMIT ?`, ts(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DNSCheck
	for rows.Next() {
		var c models.DNSCheck
		var at string
		if err := rows.Scan(&c.Domain, &c.RecordType, &c.Resolver, &c.LatencyMs, &c.Success, &c.Error, &at); err != nil {
			return nil, err
		}
		c.CollectedAt = parseTS(at)
		out = append(out, c)
	}
	return out, rows.Err()
}

// LatestInterfaceMetrics returns the most recent sample per interface.
func (d *DB) LatestInterfaceMetrics(ctx context.Context) (map[string]models.InterfaceMetric, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT interface_name, rx_bytes, tx_bytes, COALESCE(rx_rate_bps,0), COALESCE(tx_rate_bps,0),
		       COALESCE(rx_dropped,0), COALESCE(tx_dropped,0), COALESCE(rx_errors,0), COALESCE(tx_errors,0), collected_at
		FROM interface_metrics
		WHERE id IN (SELECT MAX(id) FROM interface_metrics GROUP BY interface_name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.InterfaceMetric{}
	for rows.Next() {
		var m models.InterfaceMetric
		var at string
		if err := rows.Scan(&m.Name, &m.RxBytes, &m.TxBytes, &m.RxRateBps, &m.TxRateBps,
			&m.RxDropped, &m.TxDropped, &m.RxErrors, &m.TxErrors, &at); err != nil {
			return nil, err
		}
		m.CollectedAt = parseTS(at)
		m.HasRates = true
		out[m.Name] = m
	}
	return out, rows.Err()
}

// AvgRxRate returns the average RX rate for an interface over a window,
// used by the traffic-spike rule (§8.3).
func (d *DB) AvgRxRate(ctx context.Context, iface string, since time.Time) (float64, error) {
	var avg sql.NullFloat64
	err := d.read.QueryRowContext(ctx,
		`SELECT AVG(rx_rate_bps) FROM interface_metrics WHERE interface_name=? AND collected_at >= ?`,
		iface, ts(since)).Scan(&avg)
	if err != nil {
		return 0, err
	}
	return avg.Float64, nil
}
