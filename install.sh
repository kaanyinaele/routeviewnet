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

# --- release lookup (tests load the functions between these markers) ---

# http_get URL FILE saves the response body to FILE and prints the HTTP status,
# or 000 when no response arrived at all. It keeps the body even for error
# statuses: `curl -f` and plain wget discard it, which made GitHub's
# rate-limit explanation unreadable.
if command -v curl >/dev/null 2>&1; then
  http_get() { curl -sL -o "$2" -w '%{http_code}' "$1" 2>/dev/null || true; }
  fetch()    { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  http_get() {
    code=$(wget -S --content-on-error -O "$2" "$1" 2>&1 | awk '$1 ~ /^HTTP\// {c = $2} END {print c}')
    echo "${code:-000}"
  }
  fetch()    { wget -qO "$2" "$1"; }
else
  fail "needs curl or wget"
fi

# find_release API_URL META_FILE prints the .deb download URL, or fails with a
# message about what actually went wrong. Each failure gets its own message:
# a single "could not reach GitHub" used to blame the network when GitHub had
# answered perfectly well that the repository or release does not exist.
find_release() {
  status=$(http_get "$1" "$2")
  case "$status" in
    200) ;;
    000)
      fail "could not reach GitHub to look up the latest release.
  Check this machine's internet connection and try again.
  Behind a proxy? Set https_proxy before running this." ;;
    403 | 429)
      if grep -qi 'rate limit' "$2" 2>/dev/null; then
        fail "GitHub is rate-limiting this network, so the release could not be looked up.
  This is a limit on unauthenticated requests, not a problem with your machine.
  Wait an hour, or download the .deb by hand:
  https://github.com/$REPO/releases/latest"
      fi
      fail "GitHub refused the release lookup (HTTP $status)." ;;
    404)
      fail "GitHub has no public release for $REPO.
  The repository may be private or renamed, or it has not published a release yet.
  If you have access, download the .deb from https://github.com/$REPO/releases" ;;
    *)
      fail "GitHub answered the release lookup with HTTP $status. Try again shortly." ;;
  esac
  url=$(grep -o '"browser_download_url": *"[^"]*_amd64\.deb"' "$2" | head -1 | cut -d'"' -f4) || true
  [ -n "${url:-}" ] || fail "the latest release has no amd64 .deb attached.
  See https://github.com/$REPO/releases"
  echo "$url"
}

# --- end release lookup ---

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM
DEB="$TMP/routeviewnet.deb"
META="$TMP/release.json"
LOG="$TMP/install.log"

say "Finding the latest RouteViewNet release..."
URL=$(find_release "$API" "$META")

say "Downloading $URL"
fetch "$URL" "$DEB" || fail "the download failed. Check your connection and try again."

# This script is piped into a root shell and then installs whatever it fetched,
# so the download is a root-level trust decision. Verify it against the
# checksum published with the release rather than trusting the transport alone.
SUMS_URL=$(grep -o '"browser_download_url": *"[^"]*SHA256SUMS[^"]*"' "$META" | head -1 | cut -d'"' -f4) || true
if [ -n "${SUMS_URL:-}" ] && command -v sha256sum >/dev/null 2>&1; then
  say "Verifying the download..."
  SUMS="$TMP/SHA256SUMS"
  fetch "$SUMS_URL" "$SUMS" || fail "could not download the checksum file at $SUMS_URL"
  WANT=$(grep -i "$(basename "$URL")" "$SUMS" | awk '{print $1}' | head -1)
  GOT=$(sha256sum "$DEB" | awk '{print $1}')
  [ -n "$WANT" ] || fail "the release publishes a checksum file, but it has no entry for $(basename "$URL").
  Refusing to install an unverified package."
  if [ "$WANT" != "$GOT" ]; then
    fail "the downloaded package does not match its published checksum.
  expected: $WANT
  got:      $GOT
  Do not install it. This means the download was corrupted or tampered with."
  fi
  say "Checksum OK."
elif [ -z "${SUMS_URL:-}" ]; then
  say ""
  say "Note: this release publishes no SHA256SUMS file, so the download could"
  say "not be verified. It was fetched over HTTPS, but nothing proves it is the"
  say "package the maintainer built."
  say ""
else
  say "Note: sha256sum is not available, so the download could not be verified."
fi

say "Installing..."
# apt resolves the package's dependencies (iproute2, GTK/WebKit runtime);
# plain dpkg is the fallback for minimal systems.
#
# The output goes to a file rather than /dev/null: the package's own postinst
# prints why it could not start the daemon, and discarding that was throwing
# away the one clue a stuck user needs.
installed=0
if command -v apt-get >/dev/null 2>&1; then
  if apt-get install -y "$DEB" >"$LOG" 2>&1; then installed=1
  elif dpkg -i "$DEB" >>"$LOG" 2>&1; then installed=1
  elif apt-get -f install -y >>"$LOG" 2>&1; then installed=1
  fi
else
  if dpkg -i "$DEB" >"$LOG" 2>&1; then installed=1; fi
fi

if [ "$installed" -ne 1 ]; then
  say ""
  say "The package could not be installed. The package manager reported:"
  say ""
  tail -n 20 "$LOG" | sed 's/^/  /'
  fail "installation failed"
fi

# The package enables and starts the daemon, but its postinst always exits 0 —
# so dpkg reports success even when the daemon died on startup. Ask the daemon
# itself rather than trusting the exit code, otherwise this script cheerfully
# sends people to a dashboard that will refuse the connection.
say "Checking that the monitor started..."
waited=0
running=0
while [ "$waited" -lt 15 ]; do
  if fetch_stdout "http://127.0.0.1:4545/api/v1/health" >/dev/null 2>&1; then
    running=1
    break
  fi
  waited=$((waited + 1))
  sleep 1
done

say ""
if [ "$running" -eq 1 ]; then
  say "RouteViewNet is installed and running."
  say "Open it from your applications menu, or visit http://localhost:4545"
  say ""
  say "The first readings take a minute or two to appear."
else
  say "RouteViewNet is installed, but its background monitor did not start."
  say "The dashboard at http://localhost:4545 will not load until it does."
  say ""
  say "To see why:"
  say "  systemctl status routeviewnetd --no-pager"
  say "  journalctl -u routeviewnetd -n 30 --no-pager"
  say ""
  say "A common cause is another program already using port 4545."
  say "You can change the port in /etc/routeviewnet/config.yaml, then run:"
  say "  sudo systemctl restart routeviewnetd"
  exit 1
fi
