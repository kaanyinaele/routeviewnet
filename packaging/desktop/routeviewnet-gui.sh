#!/bin/sh
# Launcher for the RouteViewNet desktop window, installed as
# /usr/bin/routeviewnet-gui with the real binary alongside it in
# /usr/lib/routeviewnet/.
#
# GTK3 and WebKitGTK are Recommends rather than Depends, so that a headless or
# minimal install does not pull ~40MB of rendering libraries it will never use.
# The consequence was that on such a machine the applications-menu entry did
# nothing at all: the dynamic linker failed before main() ran, and the desktop
# entry sets Terminal=false, so the error went nowhere and the user was left
# with a menu item that appeared broken.
#
# The dashboard is a web page, and any machine with an applications menu has a
# browser. So when the window cannot run, open the dashboard instead of failing
# silently.
set -u

REAL="/usr/lib/routeviewnet/routeviewnet-gui"
URL="http://localhost:4545"

open_in_browser() {
  printf 'routeviewnet: desktop window unavailable, opening %s in your browser\n' "$URL" >&2
  for opener in xdg-open gio sensible-browser x-www-browser firefox; do
    command -v "$opener" >/dev/null 2>&1 || continue
    if [ "$opener" = "gio" ]; then
      exec gio open "$URL"
    else
      exec "$opener" "$URL"
    fi
  done
  printf 'routeviewnet: no browser found. Open %s yourself.\n' "$URL" >&2
  exit 1
}

[ -x "$REAL" ] || open_in_browser

# Ask the linker before launching, so the window never half-starts.
if ldd "$REAL" 2>/dev/null | grep -q 'not found'; then
  open_in_browser
fi

"$REAL" "$@"
status=$?

# 127 is what the dynamic linker reports when it still cannot satisfy a
# library — a belt-and-braces check for the case ldd is absent or disagrees.
if [ "$status" -eq 127 ]; then
  open_in_browser
fi
exit "$status"
