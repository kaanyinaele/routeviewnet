package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"routeviewnet/internal/models"
)

func scanAlert(row interface{ Scan(...interface{}) error }) (models.Alert, error) {
	var a models.Alert
	var source, created, updated sql.NullString
	var resolved sql.NullString
	err := row.Scan(&a.ID, &a.RuleKey, &a.Severity, &a.Title, &a.Message, &source,
		&a.Status, &a.OccurrenceCount, &created, &updated, &resolved)
	if err != nil {
		return a, err
	}
	a.Source = source.String
	a.CreatedAt = parseTS(created.String)
	a.UpdatedAt = parseTS(updated.String)
	if resolved.Valid && resolved.String != "" {
		r := parseTS(resolved.String)
		a.ResolvedAt = &r
	}
	return a, nil
}

const alertCols = `id, rule_key, severity, title, message, source, status,
	occurrence_count, created_at, updated_at, resolved_at`

// FindOpenAlert returns the open alert for rule+source, if any (§8.4.2).
func (d *DB) FindOpenAlert(ctx context.Context, ruleKey, source string) (*models.Alert, error) {
	row := d.write.QueryRowContext(ctx,
		`SELECT `+alertCols+` FROM alerts WHERE rule_key=? AND source=? AND status='open'`, ruleKey, source)
	a, err := scanAlert(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (d *DB) CreateAlert(ctx context.Context, a models.Alert) (models.Alert, error) {
	now := ts(time.Now())
	res, err := d.write.ExecContext(ctx, `
		INSERT INTO alerts (rule_key, severity, title, message, source, status, occurrence_count, created_at, updated_at)
		VALUES (?,?,?,?,?,'open',1,?,?)`,
		a.RuleKey, a.Severity, a.Title, a.Message, a.Source, now, now)
	if err != nil {
		return a, err
	}
	a.ID, _ = res.LastInsertId()
	a.Status = models.StatusOpen
	a.OccurrenceCount = 1
	a.CreatedAt = parseTS(now)
	a.UpdatedAt = a.CreatedAt
	return a, nil
}

// TouchAlert re-observes an open alert: bump occurrence_count, refresh
// message/updated_at, leave created_at untouched (§8.4.2).
func (d *DB) TouchAlert(ctx context.Context, id int64, message string) error {
	_, err := d.write.ExecContext(ctx, `
		UPDATE alerts SET message=?, occurrence_count=occurrence_count+1, updated_at=? WHERE id=?`,
		message, ts(time.Now()), id)
	return err
}

func (d *DB) ResolveAlert(ctx context.Context, id int64) error {
	now := ts(time.Now())
	_, err := d.write.ExecContext(ctx,
		`UPDATE alerts SET status='resolved', resolved_at=?, updated_at=? WHERE id=?`, now, now, id)
	return err
}

// OpenAlerts returns all currently open alerts (health score input, §6).
func (d *DB) OpenAlerts(ctx context.Context) ([]models.Alert, error) {
	rows, err := d.read.QueryContext(ctx,
		`SELECT `+alertCols+` FROM alerts WHERE status='open' ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAlerts supports status/severity filters with keyset pagination (§10.2).
func (d *DB) ListAlerts(ctx context.Context, status, severity string, limit int, cursor int64) ([]models.Alert, string, error) {
	q := `SELECT ` + alertCols + ` FROM alerts WHERE 1=1`
	args := []interface{}{}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	if severity != "" {
		q += ` AND severity=?`
		args = append(args, severity)
	}
	if cursor > 0 {
		q += ` AND id < ?`
		args = append(args, cursor)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := d.read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []models.Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, a)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = EncodeCursor(out[len(out)-1].ID)
	}
	return out, next, rows.Err()
}

func (d *DB) OpenAlertCount(ctx context.Context) (int, error) {
	var n int
	err := d.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE status='open'`).Scan(&n)
	return n, err
}

// --- events (§9.2.11) ---

func (d *DB) InsertEvent(ctx context.Context, typ, severity, payload string) error {
	_, err := d.write.ExecContext(ctx,
		`INSERT INTO events (type, severity, payload, created_at) VALUES (?,?,?,?)`,
		typ, severity, payload, ts(time.Now()))
	return err
}

func (d *DB) ListEvents(ctx context.Context, limit int, cursor int64) ([]models.Event, string, error) {
	q := `SELECT id, type, COALESCE(severity,''), COALESCE(payload,''), created_at FROM events`
	args := []interface{}{}
	if cursor > 0 {
		q += ` WHERE id < ?`
		args = append(args, cursor)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := d.read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []models.Event
	for rows.Next() {
		var e models.Event
		var at string
		if err := rows.Scan(&e.ID, &e.Type, &e.Severity, &e.Payload, &at); err != nil {
			return nil, "", err
		}
		e.CreatedAt = parseTS(at)
		out = append(out, e)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = EncodeCursor(out[len(out)-1].ID)
	}
	return out, next, rows.Err()
}
