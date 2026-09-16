# RouteViewNet

A **local-first Linux network and memory health monitor** that explains
problems in plain English. One daemon, one SQLite file, one dashboard at
`http://localhost:4545`. No cloud, no account, no telemetry.

**New to Linux or networking?** Start with the
[User Guide](docs/user-guide.md). It explains every page of the app without
jargon. It is also installed to `/usr/share/doc/routeviewnet/user-guide.md`,
so it is readable when the internet is the thing that is broken. A web copy
lives at [routeviewnet.com/user-guide.html](https://routeviewnet.com/user-guide.html).

Built to spec **v1.2**. The `§` references throughout the source point at
that internal build spec, which is not distributed with the repository.

## What it does

- **Network health**: interfaces & bandwidth (with primary-interface
  detection on multi-homed machines), per-app data usage (TCP byte counters
  attributed via socket inodes), gateway/internet reachability, packet
  loss, DNS latency (A + AAAA, measured as a real round trip to the resolver
  in `/etc/resolv.conf` rather than through the local stub cache), and LAN
  device discovery via `ip neigh` with nickname/trusted labeling.
- **Memory & load health**: system RAM, swap, top memory consumers,
  1/5/15-minute load average, and the daemon's own footprint.
- **Interpretation, not just charts**: a deterministic 0–100 health score,
  deduplicated non-flapping alerts (debounce + hysteresis), and a rule-based
  troubleshooting page ("your gateway is fine, DNS is slow; check your
  resolver").
- **Alerts that reach you**: desktop notifications while the dashboard is
  open, and an outbound webhook (`alerts.webhook_url`) for when it is not.
  Settings has a test button that fires both. Note that browsers only allow
  notifications on HTTPS or `localhost`, so over a LAN address the webhook is
  the only channel.

## Quick start (development)

Requires Go ≥ 1.25, Node ≥ 20, and, for the desktop window, the GTK/WebKit
dev headers:

```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
```

```bash
make all        # build dashboard + daemon (embeds the dashboard) + desktop app
make test       # go vet + unit/integration tests
make run        # run with a local ./tmp config (debug logging)
```

Then open <http://localhost:4545>. In dev, `cd web && npm run dev` serves the
dashboard on `:3000` with `/api` proxied to the daemon.

## Install (Debian/Ubuntu)

One-liner (downloads the latest release from GitHub and installs it):

```bash
curl -fsSL https://raw.githubusercontent.com/kaanyinaele/routeviewnet/main/install.sh | sudo sh
```

Or build and install from source:

```bash
make deb
sudo dpkg -i routeviewnet_1.0.0_amd64.deb
```

The daemon is **enabled and started automatically** on install (opt out with
`sudo systemctl disable routeviewnetd`). Launch "RouteViewNet" from the
applications menu, or open <http://localhost:4545>. If the daemon is ever
stopped, the app offers to start it via the system password prompt.

- Installs a **desktop app** ("RouteViewNet" in your applications menu): a
  native WebKitGTK window onto the dashboard. The daemon keeps collecting
  when the window is closed; the browser URL keeps working too. A custom
  daemon address can be passed with `routeviewnet-gui --url http://…`.
- Runs as the dedicated **`routeviewnet` system user**, sandboxed by systemd.
- The GTK/WebKit libraries the desktop window needs are a `Recommends`, not a
  `Depends`: `apt` installs them by default on a desktop, and a headless
  server install can skip them with `--no-install-recommends` and still run
  the daemon and the browser dashboard.
- Ping privilege ladder: unprivileged ICMP datagram sockets
  (`net.ipv4.ping_group_range`, enabled by the installer) → `CAP_NET_RAW`
  raw sockets → TCP-connect probing (clearly labeled in the UI).
- `sudo dpkg -r routeviewnet` removes the software but **keeps** your data
  and config; `sudo dpkg -P routeviewnet` purges everything including the
  system user.

## Configuration

`/etc/routeviewnet/config.yaml` (see `packaging/config.yaml` for the
annotated default). Most settings are **hot-reloadable** from the dashboard's
Settings page and take effect on the next collection cycle.

Four settings cannot change in a running process — `server.host`,
`server.port`, `storage.path`, and `checks.ping_mode`: the listener is already
bound, the database is already open, and the ping mode was resolved against
the process's privileges at startup. Changing one is saved and reported with
`restart_required: true`; it takes effect on the next
`systemctl restart routeviewnetd`. The Settings page keeps showing the
requested value in the meantime, so a pending change never looks discarded.

Settings saved from the dashboard are stored in the database and re-applied at
startup, so they take precedence over `config.yaml`.

## Security posture

- API binds to `127.0.0.1` and is served under `/api/v1`.
- **Host-header validation** on every request (DNS-rebinding protection) and
  **Origin validation** on the WebSocket (cross-site WebSocket hijacking
  protection): a malicious web page cannot read your telemetry through your
  own browser.
- Restrictive CORS (no wildcard), strict settings schema with a 64KB body cap.
- Process command lines are **not** stored by default (they can contain
  secrets); enabling it is an explicit setting with an inline warning.
- Per-app traffic attribution requires read-only visibility into other
  processes' open sockets (`CAP_SYS_PTRACE` + `CAP_DAC_READ_SEARCH` in the
  systemd unit; /proc fd directories are owner-only and gated by a ptrace
  read check); the daemon never attaches to or modifies other processes.
  Be aware of the size of that second grant: **`CAP_DAC_READ_SEARCH`
  bypasses every file-read permission check on the system**, not only the
  ones under `/proc`. The unit fences off the obvious credential stores
  (`InaccessiblePaths` for `/etc/shadow`, `/etc/ssh`, `/etc/ssl/private`,
  `/root`) and `ProtectHome=true` hides `/home`. If you do not want the
  feature, set `collection.enable_app_traffic_monitoring: false` **and**
  drop both capabilities from the unit — disabling the feature stops the
  reads but cannot drop a capability systemd already granted.
- Binding to `0.0.0.0` (LAN access) triggers a persistent warning: v1 has no
  authentication.

## Repository layout

```
cmd/routeviewnetd/    daemon entrypoint
cmd/routeviewnet-gui/ desktop window (GTK3 + WebKitGTK 4.1 via cgo)
internal/collector/   /proc parsers, ping ladder, ip-neigh discovery
internal/engine/      alert debounce/dedup, health score, explanations, event bus
internal/storage/     SQLite (WAL), versioned migrations, repos, retention
internal/api/         chi router, handlers, WebSocket fan-out
internal/scheduler/   per-collector interval loops (hot-reloaded)
internal/web/         embedded dashboard build
web/                  Vite + React + TS + Tailwind + Recharts dashboard
packaging/            systemd unit, debian scripts, default config, desktop entry
docs/                 architecture, API, development notes
```

## Honest limitations (v1)

- Device discovery shows **devices visible from this machine** (the IPv4
  neighbor table), not a full LAN scan. IPv6 neighbors are not shown.
- CPU coverage is the load average only, by design; no per-process CPU.
- Virtual and VPN interfaces (`tun*`, `wg*`, `tailscale*`, `docker*`, `br-*`,
  `veth*`, …) are excluded from interface stats and primary-interface
  selection, so on a VPN the bandwidth shown is the underlying physical
  link, not the tunnel. Override with `checks.primary_interface_override`.
- No authentication for LAN access, no HTTPS; keep it on localhost or a
  trusted network.

## License

MIT. See [LICENSE](LICENSE).
