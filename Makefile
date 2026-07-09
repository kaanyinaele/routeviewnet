# RouteViewNet build orchestration.
#
#   make web    - build the React dashboard and stage it for embedding
#   make build  - build the daemon (embeds whatever is staged)
#   make gui    - build the desktop window (needs libgtk-3-dev libwebkit2gtk-4.1-dev)
#   make all    - web + build + gui
#   make test   - Go tests + vet
#   make deb    - assemble routeviewnet.deb
#   make run    - run locally with a dev config in ./tmp

GO      ?= go
VERSION ?= 1.0.0

.PHONY: all build web gui test deb run clean

all: web build gui

web:
	cd web && npm install && npm run build
	rm -rf internal/web/dist
	cp -r web/dist internal/web/dist

build:
	$(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o routeviewnetd ./cmd/routeviewnetd

gui:
	$(GO) build -trimpath -ldflags "-s -w" -o routeviewnet-gui ./cmd/routeviewnet-gui

test:
	$(GO) vet ./...
	$(GO) test ./...

deb: all
	rm -rf build/deb
	mkdir -p build/deb/DEBIAN build/deb/usr/bin build/deb/lib/systemd/system build/deb/etc/routeviewnet
	mkdir -p build/deb/usr/share/applications build/deb/usr/share/icons/hicolor/scalable/apps
	cp packaging/debian/control build/deb/DEBIAN/control
	install -m 0755 packaging/debian/postinst build/deb/DEBIAN/postinst
	install -m 0755 packaging/debian/postrm  build/deb/DEBIAN/postrm
	install -m 0755 packaging/debian/prerm   build/deb/DEBIAN/prerm
	printf '/etc/routeviewnet/config.yaml\n' > build/deb/DEBIAN/conffiles
	install -m 0755 routeviewnetd build/deb/usr/bin/routeviewnetd
	install -m 0755 routeviewnet-gui build/deb/usr/bin/routeviewnet-gui
	install -m 0644 packaging/systemd/routeviewnetd.service build/deb/lib/systemd/system/
	install -m 0644 packaging/desktop/routeviewnet.desktop build/deb/usr/share/applications/
	install -m 0644 packaging/desktop/routeviewnet.svg build/deb/usr/share/icons/hicolor/scalable/apps/
	install -m 0600 packaging/config.yaml build/deb/etc/routeviewnet/config.yaml
	dpkg-deb --build --root-owner-group build/deb routeviewnet_$(VERSION)_amd64.deb

run: build
	mkdir -p tmp
	printf 'storage:\n  path: "tmp/routeviewnet.db"\nlogging:\n  level: "debug"\n' > tmp/config.yaml
	./routeviewnetd --config tmp/config.yaml

clean:
	rm -rf routeviewnetd routeviewnet-gui build tmp web/dist
