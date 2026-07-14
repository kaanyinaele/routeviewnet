#!/bin/sh
# RouteViewNet installer: fetches the latest .deb release and installs it.
#
#   curl -fsSL https://raw.githubusercontent.com/kaanyinaele/routeviewnet/main/install.sh | sudo sh
#
# Debian/Ubuntu (amd64) only. The daemon is enabled and started by the
# package itself; nothing else is written outside the package manager.
set -eu

REPO="kaanyinaele/routeviewnet"
API="https://api.github.com/repos/$REPO/releases/latest"

say()  { printf '%s\n' "$*"; }
fail() { printf 'routeviewnet install: %s\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = "Linux" ] || fail "this installer is Linux-only"
case "$(uname -m)" in
  x86_64|amd64) ;;
  *) fail "only amd64 builds are published (this machine is $(uname -m))" ;;
esac
command -v dpkg >/dev/null 2>&1 || fail "needs a Debian/Ubuntu system (dpkg not found)"
[ "$(id -u)" = 0 ] || fail "must run as root, e.g.: curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | sudo sh"

if command -v curl >/dev/null 2>&1; then
  fetch()        { curl -fsSL "$1" -o "$2"; }
  fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch()        { wget -qO "$2" "$1"; }
  fetch_stdout() { wget -qO- "$1"; }
else
  fail "needs curl or wget"
fi

say "Finding the latest RouteViewNet release..."
URL=$(fetch_stdout "$API" \
  | grep -o '"browser_download_url": *"[^"]*_amd64\.deb"' \
  | head -1 | cut -d'"' -f4) || true
[ -n "${URL:-}" ] || fail "no .deb asset in the latest release; see https://github.com/$REPO/releases"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM
DEB="$TMP/routeviewnet.deb"

say "Downloading $URL"
fetch "$URL" "$DEB"

say "Installing..."
# apt resolves the package's dependencies (iproute2, GTK/WebKit runtime);
# plain dpkg is the fallback for minimal systems.
if command -v apt-get >/dev/null 2>&1; then
  apt-get install -y "$DEB" >/dev/null || { dpkg -i "$DEB" || apt-get -f install -y; }
else
  dpkg -i "$DEB"
fi

say ""
say "RouteViewNet is installed and running."
say "Open it from your applications menu, or visit http://localhost:4545"
