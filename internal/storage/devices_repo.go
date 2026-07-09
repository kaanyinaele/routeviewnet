package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"routeviewnet/internal/models"
)

// ErrNotFound is returned when a row lookup misses.
var ErrNotFound = errors.New("not found")

// UpsertDevice implements the first-seen / seen-again behavior of §7.2.
// It returns the stored device and whether it was newly discovered.
// Check-then-write is safe: the DB has a single writer connection (§9.1).
func (d *DB) UpsertDevice(ctx context.Context, dev models.Device) (models.Device, bool, error) {
	now := ts(time.Now())
	var id int64
	err := d.write.QueryRowContext(ctx,
		`SELECT id FROM devices WHERE ip_address=? AND mac_address=?`, dev.IP, dev.MAC).Scan(&id)
	isNew := errors.Is(err, sql.ErrNoRows)
	if err != nil && !isNew {
		return dev, false, err
	}
	if isNew {
		res, err := d.write.ExecContext(ctx, `
			INSERT INTO devices (ip_address, mac_address, hostname, vendor, interface_name, state, first_seen_at, last_seen_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			dev.IP, dev.MAC, dev.Hostname, dev.Vendor, dev.InterfaceName, dev.State, now, now)
		if err != nil {
			return dev, false, err
		}
		id, _ = res.LastInsertId()
	} else {
		if _, err := d.write.ExecContext(ctx, `
			UPDATE devices SET state=?, interface_name=?, last_seen_at=?,
			  hostname=CASE WHEN ? != '' THEN ? ELSE hostname END,
			  vendor=CASE WHEN ? != '' THEN ? ELSE vendor END
			WHERE id=?`,
			dev.State, dev.InterfaceName, now,
			dev.Hostname, dev.Hostname, dev.Vendor, dev.Vendor, id); err != nil {
			return dev, false, err
		}
	}
	row := d.write.QueryRowContext(ctx, `
		SELECT id, COALESCE(hostname,''), COALESCE(vendor,''), COALESCE(nickname,''), trusted,
		       first_seen_at, last_seen_at
		FROM devices WHERE id=?`, id)
	var first, last string
	if err := row.Scan(&dev.ID, &dev.Hostname, &dev.Vendor, &dev.Nickname, &dev.Trusted, &first, &last); err != nil {
		return dev, false, err
	}
	f, l := parseTS(first), parseTS(last)
	dev.FirstSeenAt, dev.LastSeenAt = &f, &l
	return dev, isNew, nil
}

func (d *DB) ListDevices(ctx context.Context, limit int, cursor int64) ([]models.Device, string, error) {
	q := `SELECT id, ip_address, COALESCE(mac_address,''), COALESCE(hostname,''), COALESCE(vendor,''),
	             COALESCE(nickname,''), trusted, COALESCE(interface_name,''), COALESCE(state,''),
	             COALESCE(first_seen_at,''), COALESCE(last_seen_at,'')
	      FROM devices`
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
	var out []models.Device
	for rows.Next() {
		var dev models.Device
		var first, last string
		if err := rows.Scan(&dev.ID, &dev.IP, &dev.MAC, &dev.Hostname, &dev.Vendor, &dev.Nickname,
			&dev.Trusted, &dev.InterfaceName, &dev.State, &first, &last); err != nil {
			return nil, "", err
		}
		if first != "" {
			f := parseTS(first)
			dev.FirstSeenAt = &f
		}
		if last != "" {
			l := parseTS(last)
			dev.LastSeenAt = &l
		}
		out = append(out, dev)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = EncodeCursor(out[len(out)-1].ID)
	}
	return out, next, rows.Err()
}

// UpdateDeviceLabel updates nickname and/or trusted (§10.2 PATCH endpoint).
func (d *DB) UpdateDeviceLabel(ctx context.Context, id int64, nickname *string, trusted *bool) (models.Device, error) {
	if nickname != nil {
		if _, err := d.write.ExecContext(ctx, `UPDATE devices SET nickname=? WHERE id=?`, *nickname, id); err != nil {
			return models.Device{}, err
		}
	}
	if trusted != nil {
		if _, err := d.write.ExecContext(ctx, `UPDATE devices SET trusted=? WHERE id=?`, *trusted, id); err != nil {
			return models.Device{}, err
		}
	}
	row := d.read.QueryRowContext(ctx, `
		SELECT id, ip_address, COALESCE(mac_address,''), COALESCE(hostname,''), COALESCE(vendor,''),
		       COALESCE(nickname,''), trusted, COALESCE(interface_name,''), COALESCE(state,''),
		       COALESCE(first_seen_at,''), COALESCE(last_seen_at,'')
		FROM devices WHERE id=?`, id)
	var dev models.Device
	var first, last string
	err := row.Scan(&dev.ID, &dev.IP, &dev.MAC, &dev.Hostname, &dev.Vendor, &dev.Nickname,
		&dev.Trusted, &dev.InterfaceName, &dev.State, &first, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return dev, ErrNotFound
	}
	if err != nil {
		return dev, err
	}
	if first != "" {
		f := parseTS(first)
		dev.FirstSeenAt = &f
	}
	if last != "" {
		l := parseTS(last)
		dev.LastSeenAt = &l
	}
	return dev, nil
}

func (d *DB) DeviceCount(ctx context.Context) (int, error) {
	var n int
	err := d.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices`).Scan(&n)
	return n, err
}
