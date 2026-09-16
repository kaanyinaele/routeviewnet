# Development

## Prerequisites

- Go ≥ 1.25 (see `go.mod`)
- Node ≥ 20 (dashboard)
- Linux (collectors read `/proc`, `/sys`, and run `ip neigh`)
- `libgtk-3-dev` + `libwebkit2gtk-4.1-dev` (desktop window, `make gui`; the
  built binary needs only the runtime libs, which the .deb declares as
  dependencies; note `go vet/test ./...` also needs the headers because
  they compile the cgo package)

## Workflow

```bash
make test              # gofmt check + go vet + all tests
make fmt               # rewrite sources with gofmt
make run               # daemon with ./tmp config, debug logging
cd web && npm run dev  # dashboard on :3000, /api proxied to :4545
```

CI (`.github/workflows/ci.yml`) runs the same gates on every push, plus the
dashboard build and a `dash -n` parse of the maintainer scripts — those run
under dash on Debian/Ubuntu, not bash, and the differences bite (see below).

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
- **Settings**: `config.Store` keeps a live document and a desired one. Read
  `Get()` from collectors and middleware; serve and persist `Desired()`. A
  setting the running process cannot change belongs in `restartOnly`, and
  anything listed there must also be frozen in `Apply` — the two lists have
  to agree or `restart_required` starts lying in one direction or the other.
- **Don't write what nothing reads**: the `connections` table was written
  every cycle and read by no endpoint, engine, or rule. It cost millions of
  rows and hundreds of MB before retention deleted them again. New tables need
  a reader.
- **Maintainer scripts run under dash**, and they run with `set -e`. In
  particular `[ cond ] && var=x` as a standalone statement *exits the script*
  when the test is false; use a full `if`. `read` cannot parse `/proc/sys`
  files directly either (EOF after the first `read(2)`) — pipe through `cat`.

## Publishing a Release

1. Update the version in `Makefile` (`VERSION ?= x.y.z`), `packaging/debian/control`, and `cmd/routeviewnetd/main.go`.
2. Build the Debian package:
   ```bash
   make deb VERSION=x.y.z
   ```
   This compiles the web dashboard and binaries, packages `routeviewnet_x.y.z_amd64.deb`, and automatically computes `SHA256SUMS`.
3. Create the GitHub release (tag `vx.y.z`) and attach **both** artifacts:
   - `routeviewnet_x.y.z_amd64.deb`
   - `SHA256SUMS`
   The installer script (`install.sh`) verifies the downloaded package against `SHA256SUMS` before running `dpkg`/`apt`.
