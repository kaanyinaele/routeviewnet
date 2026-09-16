# Architecture

One Go process, four layers (spec §21):

```
collectors ──▶ storage (SQLite/WAL) ──▶ API (chi, /api/v1) ──▶ dashboard (embedded React)
     │                                        ▲
     └──▶ engines (alerts → health score → explanations) ──▶ event bus ──▶ WebSocket fan-out
```

## Collection

`internal/scheduler` runs one goroutine per collector group (network, checks,
discovery, memory, load, process-memory, app-traffic, retention). Intervals
and feature toggles are read from the mutex-guarded config store **after every
run**, so settings changes apply without restart (§12.4). A panicking
collector is recovered and logged; it never takes the daemon down (§7.10).

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
Internet Health page when degraded. Echo replies are matched back to the
request by peer address and sequence (and by echo ID on the raw socket, where
the kernel does not demultiplex for us) — a raw ICMP socket sees *every* echo
reply on the host, including replies to other processes' pings.

### DNS checks (§7.4)

Queries go straight to the first `nameserver` in `/etc/resolv.conf` over UDP
using `golang.org/x/net/dns/dnsmessage`, with a random transaction ID that the
reply must echo. Going through `net.Resolver` instead would resolve through
nsswitch and the local stub (systemd-resolved, nscd), so re-asking for the
same domain every few seconds would mostly time a cache hit — a number that
stays flat whether the resolver is healthy or failing, and that the latency
alert could never fire on.

## Alert engine (§8.4)

Single open alert per `rule_key + source`. In-memory consecutive-failure /
consecutive-success counters implement trigger debounce (default 2) and
resolve hysteresis (default 2); `new_device` fires immediately. Re-observed
open alerts bump `occurrence_count` instead of creating rows. Counters reset
on restart (documented §8.4.5), and a healthy observation with nothing open
drops its counters entirely — the keys are `rule + source`, and `source` for
`new_device` is an ip/mac pair, so keeping them would mean one entry per
address a DHCP lease ever hands out.

## Alert delivery

Two channels with different failure modes, so neither is a substitute for
the other. Browser notifications are drawn by the dashboard from the
WebSocket frames it already receives — free to add, but they only fire while
a tab is open, and browsers gate the Notifications API on a secure context
(HTTPS or `localhost`), which a plain-HTTP LAN address is not. The webhook
runs in the daemon and is the only channel that works with nothing open.

The `Notifier` subscribes to the bus like any other client but never does
network I/O on the subscription goroutine. The bus drops events for
subscribers whose buffer is full, and that same buffer carries the
high-frequency metric events, so posting inline would let an endpoint that
takes a few seconds to answer crowd out the one alert that mattered. The
reader drains into a queue of its own and a second goroutine posts; there is
a test that fails under the inline arrangement.

During shutdown queued alerts still get one attempt each (retries are
skipped), so a graceful stop stays bounded but does not swallow whatever the
last collection cycle found.

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

`/overview` needs the newest row per interface and per latency target, which
is `WHERE id IN (SELECT MAX(id) ... GROUP BY ...)`. Those tables grow to
millions of rows, and the dashboard re-fetches `/overview` on nearly every
WebSocket event, so migration 3 adds `(interface_name, id DESC)` and
`(target, id DESC)` indexes to keep the lookup off a full table scan.

Nothing writes a table no endpoint reads. Migration 3 drops `connections`,
which was written every collection cycle and read by nothing — no endpoint, no
engine, no rule — accumulating millions of rows purely to be deleted again by
retention.

## API security (§10, §11.8)

- Host-header validation (DNS-rebinding defense) on every request.
- WebSocket Origin validation (CORS does not cover WebSockets).
- Restrictive CORS: only explicitly allow-listed origins, never `*`.
- Keyset cursor pagination (opaque base64 row id, `id DESC`).
- Settings POST: 64KB cap, unknown fields rejected, hot-reload vs
  `restart_required` split.

### Settings: live vs desired (§12.4)

`config.Store` holds two documents. `Get()` is what is in effect right now —
what collectors and middleware read every cycle. `Desired()` is that plus any
restart-only change (`server.host`, `server.port`, `storage.path`,
`checks.ping_mode`) that cannot apply to a running process whose listener is
bound and whose database is open.

`Desired()` is what `GET`/`POST /settings` serve and persist, so a change
reported as `restart_required` survives the restart it asks for; `Store.Adopt`
is the startup path that installs the stored document wholesale, restart-only
fields included, before anything is bound. Persisting `Get()` instead would
write back the pre-change values and quietly discard what the response just
promised.

## Shutdown (§14.9)

`SIGTERM`/`SIGINT` → collector loops stop, WebSocket clients get close
`1001`, HTTP drains ≤10s, queued alert webhooks get a last attempt, then
exit. The systemd unit sets `TimeoutStopSec=15`.
