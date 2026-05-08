# sqlitedeploy build targets
#
#   make build              build the CLI for the current platform
#   make fetch-litestream   download upstream litestream releases into the
#                           embed dir so they get baked into the binary
#   make release            build static binaries for every embedded platform
#                           (pass VERSION=x.y.z to stamp `sqlitedeploy --version`)
#   make package-npm        copy release binaries into packaging/npm/platforms/
#   make package-pip        build platform-tagged wheels under packaging/pip/dist/
#                           (requires VERSION; needs `hatch` on PATH)
#   make stamp-versions     rewrite version fields in all packaging manifests
#                           (requires VERSION; needs node + python3)
#
# On Windows without make/bash, use scripts\fetch-litestream.ps1 instead.

LITESTREAM_VERSION ?= 0.5.11
EMBED_DIR := internal/litestream/bin
BIN_NAME := sqlitedeploy
BUILD_DIR := dist
VERSION ?= dev

# Wheel platform tags per Go OS/ARCH (used by package-pip).
# (go-os : go-arch : python-platform-tag)
PIP_PLATFORMS := \
	linux:amd64:manylinux2014_x86_64 \
	linux:arm64:manylinux2014_aarch64 \
	darwin:amd64:macosx_11_0_x86_64 \
	darwin:arm64:macosx_11_0_arm64 \
	windows:amd64:win_amd64 \
	windows:arm64:win_arm64

# (go-os : go-arch : litestream-arch : archive-ext)
#
# Litestream's release filenames use `x86_64` where Go uses `amd64`, but agree
# on `arm64`. We download with the litestream-arch and save with the go-arch so
# embed.go (which uses runtime.GOOS/GOARCH) can find each binary.
PLATFORMS := \
	linux:amd64:x86_64:tar.gz \
	linux:arm64:arm64:tar.gz \
	darwin:amd64:x86_64:tar.gz \
	darwin:arm64:arm64:tar.gz \
	windows:amd64:x86_64:zip \
	windows:arm64:arm64:zip

.PHONY: build
build:
	go build -ldflags="-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BIN_NAME) ./cmd/sqlitedeploy

.PHONY: fetch-litestream
fetch-litestream:
	@mkdir -p $(EMBED_DIR)
	@for spec in $(PLATFORMS); do \
		os=$$(echo $$spec | cut -d: -f1); \
		goarch=$$(echo $$spec | cut -d: -f2); \
		lsarch=$$(echo $$spec | cut -d: -f3); \
		ext=$$(echo $$spec | cut -d: -f4); \
		url="https://github.com/benbjohnson/litestream/releases/download/v$(LITESTREAM_VERSION)/litestream-$(LITESTREAM_VERSION)-$$os-$$lsarch.$$ext"; \
		out="$(EMBED_DIR)/litestream-$$os-$$goarch"; \
		[ "$$os" = "windows" ] && out="$$out.exe"; \
		echo "→ $$url"; \
		tmp=$$(mktemp -d); \
		curl -fsSL "$$url" -o "$$tmp/archive.$$ext" || { echo "fetch failed"; exit 1; }; \
		if [ "$$ext" = "tar.gz" ]; then \
			tar -xzf "$$tmp/archive.$$ext" -C "$$tmp"; \
			cp "$$tmp/litestream" "$$out"; \
		else \
			(cd "$$tmp" && unzip -q archive.$$ext); \
			cp "$$tmp/litestream.exe" "$$out"; \
		fi; \
		chmod +x "$$out"; \
		rm -rf "$$tmp"; \
		ls -lh "$$out"; \
	done
	@echo "✓ litestream binaries cached in $(EMBED_DIR)"

.PHONY: release
release: fetch-litestream
	@for spec in $(PLATFORMS); do \
		os=$$(echo $$spec | cut -d: -f1); \
		goarch=$$(echo $$spec | cut -d: -f2); \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="$(BUILD_DIR)/$(BIN_NAME)-$$os-$$goarch$$ext"; \
		echo "→ building $$out (version=$(VERSION))"; \
		GOOS=$$os GOARCH=$$goarch go build \
			-ldflags="-s -w -X main.version=$(VERSION)" \
			-o "$$out" ./cmd/sqlitedeploy; \
	done
	@ls -lh $(BUILD_DIR)

# Map Go GOOS/GOARCH to npm platform-arch convention:
#   linux/amd64 → linux-x64        windows/amd64 → win32-x64
#   linux/arm64 → linux-arm64      windows/arm64 → win32-arm64
#   darwin/amd64 → darwin-x64      darwin/arm64 → darwin-arm64
.PHONY: package-npm
package-npm:
	@for spec in $(PLATFORMS); do \
		os=$$(echo $$spec | cut -d: -f1); \
		goarch=$$(echo $$spec | cut -d: -f2); \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		case $$os in windows) npmos=win32 ;; *) npmos=$$os ;; esac; \
		case $$goarch in amd64) npmarch=x64 ;; *) npmarch=$$goarch ;; esac; \
		bin_in="$(BUILD_DIR)/$(BIN_NAME)-$$os-$$goarch$$ext"; \
		out_dir="packaging/npm/platforms/$$npmos-$$npmarch/bin"; \
		out_file="$$out_dir/$(BIN_NAME)$$ext"; \
		[ -f "$$bin_in" ] || { echo "missing $$bin_in — run \`make release\` first"; exit 1; }; \
		mkdir -p "$$out_dir"; \
		cp "$$bin_in" "$$out_file"; \
		chmod +x "$$out_file"; \
		echo "→ $$out_file"; \
	done

.PHONY: package-pip
package-pip:
	@if [ "$(VERSION)" = "dev" ]; then echo "VERSION=x.y.z required"; exit 1; fi
	@command -v hatch >/dev/null || { echo "hatch not found — pip install hatch"; exit 1; }
	rm -rf packaging/pip/dist
	@for spec in $(PIP_PLATFORMS); do \
		go_os=$$(echo $$spec | cut -d: -f1); \
		go_arch=$$(echo $$spec | cut -d: -f2); \
		py_tag=$$(echo $$spec | cut -d: -f3); \
		ext=""; [ "$$go_os" = "windows" ] && ext=".exe"; \
		bin="$(abspath $(BUILD_DIR))/$(BIN_NAME)-$$go_os-$$go_arch$$ext"; \
		[ -f "$$bin" ] || { echo "missing $$bin — run \`make release\` first"; exit 1; }; \
		echo "→ wheel for $$py_tag (binary: $$bin)"; \
		(cd packaging/pip && \
			SQLITEDEPLOY_BINARY="$$bin" SQLITEDEPLOY_PLAT="$$py_tag" \
			hatch build --target wheel) || exit 1; \
	done
	@ls -lh packaging/pip/dist

.PHONY: stamp-versions
stamp-versions:
	@if [ "$(VERSION)" = "dev" ]; then echo "VERSION=x.y.z required"; exit 1; fi
	@command -v node >/dev/null || { echo "node not found"; exit 1; }
	@command -v python3 >/dev/null || command -v python >/dev/null || { echo "python not found"; exit 1; }
	node scripts/stamp-versions.js $(VERSION)
	@(command -v python3 >/dev/null && python3 scripts/stamp-versions.py $(VERSION)) || python scripts/stamp-versions.py $(VERSION)

.PHONY: test
test:
	go test ./...

# Packaging integration tests — pack/install the npm and pip wrappers
# locally and confirm they can find and exec the bundled Go binary.
# Requires a host-platform binary in dist/ (build with `make build`).
.PHONY: test-packaging
test-packaging:
	bash test/run-all.sh

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
