# Development

## Prerequisites

- Go ≥ 1.24
- Node ≥ 20 (dashboard)
- Linux (collectors read `/proc`, `/sys`, and run `ip neigh`)
- `libgtk-3-dev` + `libwebkit2gtk-4.1-dev` (desktop window, `make gui`; the
  built binary needs only the runtime libs, which the .deb declares as
  dependencies; note `go vet/test ./...` also needs the headers because
  they compile the cgo package)

## Workflow

```bash
make test              # go vet + all tests
make run               # daemon with ./tmp config, debug logging
cd web && npm run dev  # dashboard on :3000, /api proxied to :4545
```

`make all` builds the dashboard, stages it into `internal/web/dist`, and
compiles the daemon with it embedded. A placeholder `index.html` is committed
so `go build` works before the first web build.

## Testing notes

- Parsers are pure functions; see `internal/collector/parsers_test.go` for
  the fixture style.
- Storage/engine tests run against a real temp-file SQLite DB (WAL mode),
  covering migrations, pagination, retention, and alert debounce.
- The §16.3 soak test is a release gate: 7 days on a reference VM, watching
  daemon RSS (fail if >2× steady-state), goroutine count, DB size vs
  retention, and `journalctl` for panics.

## Gotchas

- **Timestamps**: stored as UTC `"2006-01-02 15:04:05"` TEXT. Columns
  declared `DATETIME` may round-trip through `database/sql` as RFC3339 when
  scanned into a string; `parseTS` accepts both. Keep it that way.
- **Single writer**: all writes go through the 1-connection pool. Don't add
  a second write path.
- **WebSocket handler**: the write loop runs inline in the handler;
  returning early would cancel the request context and drop the hijacked
  connection.
- **Unprivileged ping**: needs the daemon's GID inside
  `net.ipv4.ping_group_range`. If your dev machine has it disabled
  (`sysctl net.ipv4.ping_group_range` → `1 0`), checks fall back to TCP and
  are labeled as such; that's expected, not a bug.
