# Architecture

One Go process, four layers (spec §21):

```
collectors ──▶ storage (SQLite/WAL) ──▶ API (chi, /api/v1) ──▶ dashboard (embedded React)
     │                                        ▲
     └──▶ engines (alerts → health score → explanations) ──▶ event bus ──▶ WebSocket fan-out
```

## Collection

`internal/scheduler` runs one goroutine per collector group (network, checks,
discovery, memory, load, process-memory, retention). Intervals are read from
the mutex-guarded config store **after every run**, so settings changes apply
without restart (§12.4). A panicking collector is recovered and logged; it
never takes the daemon down (§7.10).

`internal/collector.Manager` owns each cycle: collect → persist → publish a
WebSocket event → feed rule observations to the alert engine. Parsers
(`/proc/net/dev`, `/proc/meminfo`, `/proc/loadavg`, `/proc/[pid]/status`,
`ip neigh` output) are pure functions over strings, unit-tested without a
live system.

### Ping privilege ladder (§7.3 [v1.2])

1. Unprivileged ICMP datagram socket (`net.ipv4.ping_group_range`)
2. Raw ICMP socket (`CAP_NET_RAW`)
3. TCP connect probe to `:443`/`:53`, results tagged `method: "tcp"`

The active mode is reported by `GET /api/v1/health` and shown on the
Internet Health page when degraded.

## Alert engine (§8.4)

Single open alert per `rule_key + source`. In-memory consecutive-failure /
consecutive-success counters implement trigger debounce (default 2) and
resolve hysteresis (default 2); `new_device` fires immediately. Re-observed
open alerts bump `occurrence_count` instead of creating rows. Counters reset
on restart (documented §8.4.5).

## Health score (§6)

Pure function of the open alerts: per-rule penalties, capped per category,
floored at 0. Recomputed event-driven on every alert open/resolve and stored
in `health_score_history` for the Overview sparkline.

## Storage (§9)

SQLite in WAL mode, `busy_timeout=5000`, `synchronous=NORMAL`. Two pools:
a single-connection writer (serialized writes, no `SQLITE_BUSY`) and a
4-connection reader. Versioned migrations tracked in `schema_migrations`.
Retention runs at startup + hourly, deleting in 5000-row batches; open
alerts are never purged. History endpoints downsample server-side with
`GROUP BY (strftime('%s', collected_at) / bucket)` so `range=7d` returns at
most `max_history_points` (default 500) points.

## API security (§10, §11.8)

- Host-header validation (DNS-rebinding defense) on every request.
- WebSocket Origin validation (CORS does not cover WebSockets).
- Restrictive CORS: only explicitly allow-listed origins, never `*`.
- Keyset cursor pagination (opaque base64 row id, `id DESC`).
- Settings POST: 64KB cap, unknown fields rejected, hot-reload vs
  `restart_required` split.

## Shutdown (§14.9)

`SIGTERM`/`SIGINT` → collector loops stop, WebSocket clients get close
`1001`, HTTP drains ≤10s, then exit. The systemd unit sets
`TimeoutStopSec=15`.
