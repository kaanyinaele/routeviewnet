# API

Base: `http://127.0.0.1:4545/api/v1`. JSON responses, error envelope
`{"error": {"code", "message"}}`. All list endpoints use keyset pagination:
`?limit=` (1–200, default 50) and opaque `?cursor=`; responses carry
`next_cursor` (null on the last page). History endpoints accept
`?range=15m|1h|6h|24h|7d` and downsample to ≤ `max_history_points`.

| Method & path | Purpose |
|---|---|
| `GET /health` | `{status, version, uptime_seconds, db_writable, ping_mode}` |
| `GET /overview` | health score, primary interface, rates, gateway/internet/DNS status, memory, load, daemon memory, device & alert counts, recent events |
| `GET /interfaces` | known interfaces (primary flagged) |
| `GET /metrics/bandwidth?range=&interface=` | downsampled rate history |
| `GET /traffic/apps?range=` | per-app data usage totals over the range (TCP only), heaviest first |
| `GET /checks/latency?range=` | recent latency checks |
| `GET /checks/dns?range=` | recent DNS checks (A/AAAA) |
| `GET /devices?limit=&cursor=` | discovered devices |
| `PATCH /devices/{id}` | `{"nickname"?, "trusted"?}` |
| `GET /alerts?status=&severity=&limit=&cursor=` | alerts |
| `GET /events?limit=&cursor=` | event log |
| `GET /troubleshoot` | current rule-based diagnosis |
| `GET /settings` / `POST /settings` | full config document; POST returns `restart_required` |
| `GET /system/memory` · `/system/memory/history?range=` | RAM/swap |
| `GET /system/load` | load average + core count |
| `GET /system/processes/memory` | latest top-10 scan |
| `GET /system/daemon/memory` | daemon self-report |
| `GET /health-score/history?range=` | score trend |
| `GET /live` | WebSocket upgrade |

## WebSocket events

Frame: `{"type", "timestamp", "payload"}`. Types:
`metrics.interface.updated`, `device.discovered`, `device.updated`,
`alert.created`, `alert.resolved`, `health.updated`, `dns.check.updated`,
`latency.check.updated`, `system.memory.updated`,
`system.process_memory.updated`, `system.daemon_memory.updated`,
`system.load.updated`, `traffic.apps.updated`.

Max 20 concurrent clients (configurable); over the cap new connections are
rejected with a clear reason. Foreign `Origin` headers are rejected 403.
Non-browser clients (no Origin) may connect.
