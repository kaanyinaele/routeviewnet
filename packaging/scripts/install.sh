#!/bin/sh
# Development install script — for packaged installs prefer the .deb
# (make deb; sudo dpkg -i routeviewnet.deb).
set -e

if [ "$(id -u)" != "0" ]; then
  echo "run as root: sudo $0" >&2
  exit 1
fi

BIN="${1:-./routeviewnetd}"
[ -x "$BIN" ] || { echo "binary not found: $BIN (run 'make build' first)" >&2; exit 1; }

install -D -m 0755 "$BIN" /usr/bin/routeviewnetd
install -D -m 0644 packaging/systemd/routeviewnetd.service /lib/systemd/system/routeviewnetd.service
if [ ! -f /etc/routeviewnet/config.yaml ]; then
  install -D -m 0600 packaging/config.yaml /etc/routeviewnet/config.yaml
fi

# Same user/permission/ICMP logic as the .deb postinst.
sh packaging/debian/postinst configure

echo "done — start with: systemctl enable --now routeviewnetd"
