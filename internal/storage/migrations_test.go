package storage

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// openAtVersion builds a database with only the first n migrations applied,
// standing in for an installation that predates the current release.
func openAtVersion(t *testing.T, path string, n int) {
	t.Helper()
	full := migrations
	migrations = migrations[:n]
	defer func() { migrations = full }()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := Open(path, log)
	if err != nil {
		t.Fatalf("open at migration %d: %v", n, err)
	}
	db.Close()
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&found)
	switch err {
	case nil:
		return true
	case sql.ErrNoRows:
		return false
	default:
		t.Fatal(err)
		return false
	}
}

func indexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&found)
	switch err {
	case nil:
		return true
	case sql.ErrNoRows:
		return false
	default:
		t.Fatal(err)
		return false
	}
}

// Migration 3 must upgrade a database created by an earlier release, not just
// a fresh one: it drops the write-only connections table and adds the indexes
// /overview's latest-row-per-key lookups need.
func TestMigrationUpgradesExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	openAtVersion(t, path, 2)

	// An old install has the table, and rows in it.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, raw, "connections") {
		t.Fatal("fixture should start with a connections table")
	}
	if _, err := raw.Exec(`INSERT INTO connections (protocol, local_ip, local_port, remote_ip, remote_port, state, collected_at)
		VALUES ('tcp','127.0.0.1',1,'10.0.0.1',2,'ESTABLISHED','2024-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	// Reopening applies the remaining migrations.
	db := func() *DB {
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		db, err := Open(path, log)
		if err != nil {
			t.Fatalf("upgrade open: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}()

	raw, err = sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	if tableExists(t, raw, "connections") {
		t.Error("migration 3 should drop the write-only connections table")
	}
	for _, idx := range []string{"idx_iface_metrics_name_id", "idx_latency_target_id"} {
		if !indexExists(t, raw, idx) {
			t.Errorf("migration 3 should create %s", idx)
		}
	}

	var version int
	if err := raw.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Errorf("schema_migrations at %d, want %d", version, len(migrations))
	}

	// The upgraded database still works.
	ctx := context.Background()
	if !db.Writable(ctx) {
		t.Error("upgraded database should be writable")
	}
	if _, err := db.LatestInterfaceMetrics(ctx); err != nil {
		t.Errorf("LatestInterfaceMetrics after upgrade: %v", err)
	}
	if _, err := db.LatestLatencyByTarget(ctx); err != nil {
		t.Errorf("LatestLatencyByTarget after upgrade: %v", err)
	}
	if err := db.Cleanup(ctx, 7); err != nil {
		t.Errorf("retention after upgrade: %v", err)
	}
}

// Migrations are idempotent: reopening an already-current database is a no-op.
func TestMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repeat.db")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for i := 0; i < 3; i++ {
		db, err := Open(path, log)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		db.Close()
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(migrations) {
		t.Errorf("want %d migration rows, got %d", len(migrations), n)
	}
}

// The latest-row-per-key queries must actually use the new indexes rather than
// scanning tables that grow to millions of rows.
func TestLatestRowQueriesUseIndexes(t *testing.T) {
	db := testDB(t)

	for _, tc := range []struct {
		name, query, wantIndex string
	}{
		{
			"interface metrics",
			`SELECT interface_name FROM interface_metrics WHERE id IN (SELECT MAX(id) FROM interface_metrics GROUP BY interface_name)`,
			"idx_iface_metrics_name_id",
		},
		{
			"latency checks",
			`SELECT target FROM latency_checks WHERE id IN (SELECT MAX(id) FROM latency_checks GROUP BY target)`,
			"idx_latency_target_id",
		},
	} {
		rows, err := db.read.Query("EXPLAIN QUERY PLAN " + tc.query)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		plan := ""
		for rows.Next() {
			var a, b, c int
			var detail string
			if err := rows.Scan(&a, &b, &c, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan += detail + "\n"
		}
		rows.Close()
		if !strings.Contains(plan, tc.wantIndex) {
			t.Errorf("%s should use %s, plan was:\n%s", tc.name, tc.wantIndex, plan)
		}
	}
}
