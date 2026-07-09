package storage

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"routeviewnet/internal/models"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCursorRoundTrip(t *testing.T) {
	c := EncodeCursor(12345)
	id, err := DecodeCursor(c)
	if err != nil || id != 12345 {
		t.Errorf("round trip failed: %d, %v", id, err)
	}
	if id, err := DecodeCursor(""); err != nil || id != 0 {
		t.Errorf("empty cursor: want (0, nil), got (%d, %v)", id, err)
	}
	if _, err := DecodeCursor("!!!"); err == nil {
		t.Error("invalid base64 must error")
	}
	if _, err := DecodeCursor(EncodeCursor(-5)); err == nil {
		t.Error("negative id must error")
	}
}

func TestClampLimit(t *testing.T) {
	for in, want := range map[int]int{0: 50, -1: 50, 10: 10, 200: 200, 999: 200} {
		if got := ClampLimit(in); got != want {
			t.Errorf("ClampLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestBucketSeconds(t *testing.T) {
	// 7d at max 500 points → 1209.6s buckets, well above the 5s interval.
	if got := BucketSeconds(7*24*3600, 500, 5); got != 1209 {
		t.Errorf("7d bucket: got %d", got)
	}
	// 15m at max 500 points would be 1.8s — clamps up to the interval.
	if got := BucketSeconds(900, 500, 5); got != 5 {
		t.Errorf("15m bucket: want 5, got %d", got)
	}
}

func TestMigrationsAndIndexes(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	var mode string
	if err := db.read.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode: want wal, got %s", mode)
	}

	rows, err := db.read.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if count < 12 {
		t.Errorf("expected at least 12 indexes from migration, got %d", count)
	}
}

func TestDeviceUpsertAndPagination(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	d1, isNew, err := db.UpsertDevice(ctx, models.Device{IP: "192.168.0.10", MAC: "aa:bb:cc:dd:ee:ff", State: "REACHABLE"})
	if err != nil || !isNew {
		t.Fatalf("first upsert: want new, got new=%v err=%v", isNew, err)
	}
	_, isNew, err = db.UpsertDevice(ctx, models.Device{IP: "192.168.0.10", MAC: "aa:bb:cc:dd:ee:ff", State: "STALE"})
	if err != nil || isNew {
		t.Fatalf("second upsert: want not-new, got new=%v err=%v", isNew, err)
	}
	if n, _ := db.DeviceCount(ctx); n != 1 {
		t.Fatalf("want 1 device, got %d", n)
	}

	// Label update (§10.2 PATCH).
	nick := "Ada's Laptop"
	trusted := true
	dev, err := db.UpdateDeviceLabel(ctx, d1.ID, &nick, &trusted)
	if err != nil || dev.Nickname != nick || !dev.Trusted {
		t.Fatalf("label update failed: %+v, %v", dev, err)
	}
	if _, err := db.UpdateDeviceLabel(ctx, 9999, &nick, nil); err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}

	// Pagination: 3 devices, page size 2 → next_cursor set, second page 1 row.
	db.UpsertDevice(ctx, models.Device{IP: "192.168.0.11", MAC: "11:11:11:11:11:11"})
	db.UpsertDevice(ctx, models.Device{IP: "192.168.0.12", MAC: "22:22:22:22:22:22"})
	page1, next, err := db.ListDevices(ctx, 2, 0)
	if err != nil || len(page1) != 2 || next == "" {
		t.Fatalf("page1: %d rows, next=%q, err=%v", len(page1), next, err)
	}
	cur, _ := DecodeCursor(next)
	page2, next2, err := db.ListDevices(ctx, 2, cur)
	if err != nil || len(page2) != 1 || next2 != "" {
		t.Fatalf("page2: %d rows, next=%q, err=%v", len(page2), next2, err)
	}
}

func TestRetentionCleanup(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	old := time.Now().AddDate(0, 0, -10)
	fresh := time.Now()
	for _, at := range []time.Time{old, fresh} {
		if err := db.InsertLatencyCheck(ctx, models.LatencyCheck{
			Target: "1.1.1.1", TargetType: "internet", Success: true, CollectedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// An old but still-open alert must never be purged (§9.3).
	a, err := db.CreateAlert(ctx, models.Alert{RuleKey: "x", Severity: "warning", Title: "t", Message: "m", Source: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.write.ExecContext(ctx,
		`UPDATE alerts SET created_at=?, updated_at=? WHERE id=?`, ts(old), ts(old), a.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.Cleanup(ctx, 7); err != nil {
		t.Fatal(err)
	}
	checks, err := db.RecentLatencyChecks(ctx, time.Now().AddDate(0, 0, -30), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 {
		t.Errorf("want 1 surviving check, got %d", len(checks))
	}
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Error("open alert must survive retention regardless of age")
	}
}

func TestBandwidthHistoryDownsampling(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	base := time.Now().Add(-10 * time.Minute)
	var ms []models.InterfaceMetric
	for i := 0; i < 120; i++ { // 10 min of 5s samples
		ms = append(ms, models.InterfaceMetric{
			Name: "eth0", RxBytes: uint64(i * 1000), TxBytes: uint64(i * 500),
			RxRateBps: 200, TxRateBps: 100, HasRates: true,
			CollectedAt: base.Add(time.Duration(i) * 5 * time.Second),
		})
	}
	if err := db.InsertInterfaceMetrics(ctx, ms); err != nil {
		t.Fatal(err)
	}
	// Bucket to 60s → at most ~11 points from 120 rows.
	points, err := db.BandwidthHistory(ctx, "eth0", base.Add(-time.Minute), 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 || len(points) > 12 {
		t.Errorf("downsampling failed: got %d points", len(points))
	}
	if points[0].RxRateBps != 200 {
		t.Errorf("bucket average wrong: %v", points[0].RxRateBps)
	}
}
