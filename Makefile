# RouteViewNet build orchestration.
#
#   make web    - build the React dashboard and stage it for embedding
#   make build  - build the daemon (embeds whatever is staged)
#   make gui    - build the desktop window (needs libgtk-3-dev libwebkit2gtk-4.1-dev)
#   make all    - web + build + gui
#   make test   - gofmt check + go vet + Go tests
#   make fmt    - rewrite Go sources with gofmt
#   make deb    - assemble routeviewnet.deb
#   make run    - run locally with a dev config in ./tmp
#
# Reviewing a build on this machine:
#
#   make web build            # as your normal user
#   sudo make install-local   # swap it in behind http://localhost:4545
#   sudo make rollback-local  # put the packaged binary back
#
# The build stays unprivileged on purpose: `make web` runs npm, and running
# that as root leaves root-owned files in ./web.

GO      ?= go
VERSION ?= 1.0.0

# Where the packaged binary is kept so a bad local build is always reversible.
BACKUP  ?= /var/backups/routeviewnet/routeviewnetd.packaged

.PHONY: all build web gui test fmt fmt-check deb run clean install-local rollback-local require-go

all: web build gui

# npm ci, not npm install: install re-resolves and rewrites package-lock.json
# on every build, so the local tree drifted from the one CI builds (which also
# runs npm ci). ci installs exactly what the lockfile says, or fails.
web:
	cd web && npm ci && npm run build
	rm -rf internal/web/dist
	cp -r web/dist internal/web/dist

# Without this, a missing toolchain surfaces as "/bin/sh: 1: go: not found"
# and "Error 127", which says nothing about what to do next. Honours GO=, so
# pointing at a toolchain outside PATH still works.
require-go:
	@command -v $(GO) >/dev/null 2>&1 || { \
	  echo "Go is not installed (it is needed to build the daemon)."; \
	  echo "  On Ubuntu/Debian:  sudo apt install golang-go"; \
	  echo "  Then re-run:       make $(MAKECMDGOALS)"; \
	  exit 1; }

build: require-go
	$(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o routeviewnetd ./cmd/routeviewnetd

gui: require-go
	$(GO) build -trimpath -ldflags "-s -w" -o routeviewnet-gui ./cmd/routeviewnet-gui

test: fmt-check
	$(GO) vet ./...
	$(GO) test ./...

# gofmt is not enforced by `go vet`, so check it explicitly — CI runs the
# same gate.
fmt-check:
	@unformatted="$$($(GO)fmt -l ./cmd ./internal)"; \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

fmt:
	$(GO)fmt -w ./cmd ./internal

deb: all
	rm -rf build/deb
	mkdir -p build/deb/DEBIAN build/deb/usr/bin build/deb/lib/systemd/system build/deb/etc/routeviewnet
	mkdir -p build/deb/usr/share/applications build/deb/usr/share/icons/hicolor/scalable/apps
	mkdir -p build/deb/usr/lib/routeviewnet build/deb/usr/share/doc/routeviewnet
	@# The package version must come from VERSION. control used to be copied
	@# verbatim with a hardcoded "Version: 1.0.0", so `make deb VERSION=1.1.0`
	@# built routeviewnet_1.1.0_amd64.deb that told dpkg it was still 1.0.0 —
	@# and the installer, which compares versions, would never update anyone.
	sed 's/^Version: .*/Version: $(VERSION)/' packaging/debian/control > build/deb/DEBIAN/control
	install -m 0755 packaging/debian/postinst build/deb/DEBIAN/postinst
	install -m 0755 packaging/debian/postrm  build/deb/DEBIAN/postrm
	install -m 0755 packaging/debian/prerm   build/deb/DEBIAN/prerm
	printf '/etc/routeviewnet/config.yaml\n' > build/deb/DEBIAN/conffiles
	install -m 0755 routeviewnetd build/deb/usr/bin/routeviewnetd
	# The GUI binary moves under /usr/lib and /usr/bin gets a wrapper, so that
	# a machine without the GTK/WebKit Recommends opens the dashboard in a
	# browser instead of the menu entry doing nothing at all.
	install -m 0755 routeviewnet-gui build/deb/usr/lib/routeviewnet/routeviewnet-gui
	install -m 0755 packaging/desktop/routeviewnet-gui.sh build/deb/usr/bin/routeviewnet-gui
	install -m 0644 packaging/systemd/routeviewnetd.service build/deb/lib/systemd/system/
	install -m 0644 packaging/desktop/routeviewnet.desktop build/deb/usr/share/applications/
	install -m 0644 packaging/desktop/routeviewnet.svg build/deb/usr/share/icons/hicolor/scalable/apps/
	install -m 0600 packaging/config.yaml build/deb/etc/routeviewnet/config.yaml
	# Ship the user guide: an offline reader had no documentation on disk at all.
	install -m 0644 docs/user-guide.md build/deb/usr/share/doc/routeviewnet/user-guide.md
	install -m 0644 README.md build/deb/usr/share/doc/routeviewnet/README.md
	dpkg-deb --build --root-owner-group build/deb routeviewnet_$(VERSION)_amd64.deb

run: build
	mkdir -p tmp
	printf 'storage:\n  path: "tmp/routeviewnet.db"\nlogging:\n  level: "debug"\n' > tmp/config.yaml
	./routeviewnetd --config tmp/config.yaml

# install-local swaps the freshly built daemon in behind the installed service,
# so http://localhost:4545 always shows the current build and nothing has to
# stay running in a terminal.
#
# It deliberately does NOT depend on `web build`: those run as your normal user
# (npm and the Go module cache should not be touched by root), and only the
# swap itself needs privilege.
#
# /etc/routeviewnet/config.yaml and /var/lib/routeviewnet are left alone, so
# this runs against the real database rather than a fresh one.
install-local:
	@[ "$$(id -u)" = 0 ] || { echo "install-local needs root: sudo make install-local"; exit 1; }
	@[ -f routeviewnetd ] || { echo "no ./routeviewnetd yet — run 'make web build' as your normal user first"; exit 1; }
	@mkdir -p $$(dirname "$(BACKUP)")
	@if [ ! -f "$(BACKUP)" ] && [ -f /usr/bin/routeviewnetd ]; then \
	  cp -a /usr/bin/routeviewnetd "$(BACKUP)"; \
	  echo "kept the packaged binary at $(BACKUP) (rollback-local restores it)"; \
	fi
	systemctl stop routeviewnetd
	install -m 0755 routeviewnetd /usr/bin/routeviewnetd
	@# Overwriting a file drops its capabilities. The package sets this in its
	@# postinst; without it latency checks quietly fall back to TCP probes.
	@if command -v setcap >/dev/null 2>&1; then \
	  setcap cap_net_raw+ep /usr/bin/routeviewnetd \
	    || echo "warning: setcap failed — latency checks will use the TCP fallback"; \
	else \
	  echo "warning: setcap not found — latency checks will use the TCP fallback"; \
	fi
	systemctl start routeviewnetd
	@printf 'waiting for the daemon to answer'
	@i=0; while [ $$i -lt 15 ]; do \
	  if curl -fsS http://127.0.0.1:4545/api/v1/health >/dev/null 2>&1; then \
	    printf '\n'; getcap /usr/bin/routeviewnetd 2>/dev/null; \
	    echo "running: http://localhost:4545"; exit 0; \
	  fi; \
	  printf '.'; i=$$((i+1)); sleep 1; \
	done; \
	printf '\n'; \
	echo "the daemon did not come up. To see why:"; \
	echo "  systemctl status routeviewnetd --no-pager"; \
	echo "  journalctl -u routeviewnetd -n 30 --no-pager"; \
	echo "To go back to the packaged build: sudo make rollback-local"; \
	exit 1

rollback-local:
	@[ "$$(id -u)" = 0 ] || { echo "rollback-local needs root: sudo make rollback-local"; exit 1; }
	@[ -f "$(BACKUP)" ] || { echo "no backup at $(BACKUP) — nothing to roll back to"; exit 1; }
	systemctl stop routeviewnetd
	install -m 0755 "$(BACKUP)" /usr/bin/routeviewnetd
	@if command -v setcap >/dev/null 2>&1; then setcap cap_net_raw+ep /usr/bin/routeviewnetd || true; fi
	systemctl start routeviewnetd
	@echo "restored the packaged binary from $(BACKUP)"

clean:
	rm -rf routeviewnetd routeviewnet-gui build tmp web/dist
