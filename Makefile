# sqlitedeploy build targets
#
#   make build               build the CLI for the current platform
#   make fetch-libsql-source clone tursodatabase/libsql @ LIBSQL_VERSION into
#                            build/libsql/ (gitignored). Idempotent.
#   make build-sqld          build sqld for the host platform with -F bottomless
#                            and place it under internal/sqld/bin/.
#                            Cross-platform builds are CI's job (see
#                            .github/workflows/release.yml). Use Phase 1 to
#                            validate the recipe locally before pushing.
#   make build-sqld-target   CI-facing: build sqld for one specific target.
#                            Required vars: GO_OS, GO_ARCH, RUST_TARGET.
#   make release             build static binaries for every embedded platform
#                            (pass VERSION=x.y.z to stamp `sqlitedeploy --version`)
#   make package-npm         copy release binaries into packaging/npm/platforms/
#   make package-pip         build platform-tagged wheels under packaging/pip/dist/
#                            (requires VERSION; needs `hatch` on PATH)
#   make package-maven       copy release binaries into packaging/maven/platforms/
#                            and run `mvn package` (requires VERSION; needs `mvn`)
#   make stamp-versions      rewrite version fields in all packaging manifests
#                            (requires VERSION; needs node + python3)
#
# On Windows without make/bash, use scripts\build-sqld.ps1 instead.

LIBSQL_VERSION ?= libsql-server-v0.24.32
LIBSQL_REPO    ?= https://github.com/tursodatabase/libsql.git
LIBSQL_SRC_DIR := build/libsql
SQLD_EMBED_DIR := internal/sqld/bin
BIN_NAME := sqlitedeploy
BUILD_DIR := dist
VERSION ?= dev

# Wheel platform tags per Go OS/ARCH (used by package-pip).
# Windows dropped in v2: upstream libsql-server doesn't compile on Windows
# (libsql-wal uses POSIX-only syscalls). Windows users: use WSL.
# (go-os : go-arch : python-platform-tag)
PIP_PLATFORMS := \
	linux:amd64:manylinux2014_x86_64 \
	linux:arm64:manylinux2014_aarch64 \
	darwin:amd64:macosx_11_0_x86_64 \
	darwin:arm64:macosx_11_0_arm64

# Maven classifier names per Go OS/ARCH (used by package-maven). Each entry
# corresponds to one packaging/maven/platforms/<plat>/ module. Windows dropped
# in v2 (see PIP_PLATFORMS comment).
# (go-os : go-arch : maven-classifier)
MAVEN_PLATFORMS := \
	linux:amd64:linux-x86_64 \
	linux:arm64:linux-aarch64 \
	darwin:amd64:darwin-x86_64 \
	darwin:arm64:darwin-aarch64

# (go-os : go-arch : rust-target)
#
# Each row maps a Go cross-build target to the Rust target triple sqld is
# built against. Output binaries are written to:
#   internal/sqld/bin/sqld-<go-os>-<go-arch>
# so embed.go (which uses runtime.GOOS/GOARCH) can locate each one.
#
# Windows is intentionally absent: libsql-server's `libsql-wal` dep uses
# POSIX-only syscalls (`pwrite`, `pwritev`, `std::os::unix`) with no Windows
# fallbacks and no Cargo feature flag to opt out. Upstream itself does not
# ship Windows release binaries. Windows users should use WSL.
SQLD_PLATFORMS := \
	linux:amd64:x86_64-unknown-linux-gnu \
	linux:arm64:aarch64-unknown-linux-gnu \
	darwin:amd64:x86_64-apple-darwin \
	darwin:arm64:aarch64-apple-darwin

.PHONY: build
build:
	go build -ldflags="-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BIN_NAME) ./cmd/sqlitedeploy

# Shallow-clone tursodatabase/libsql at $(LIBSQL_VERSION). Idempotent: if the
# directory already exists, do nothing (delete it to refetch). Avoids submodule
# bloat for normal contributors — only the release pipeline needs the source.
.PHONY: fetch-libsql-source
fetch-libsql-source:
	@if [ -d "$(LIBSQL_SRC_DIR)/.git" ]; then \
		echo "✓ libsql source already at $(LIBSQL_SRC_DIR) (rm -rf to refetch)"; \
	else \
		mkdir -p "$$(dirname $(LIBSQL_SRC_DIR))"; \
		echo "→ git clone --depth 1 --branch $(LIBSQL_VERSION) $(LIBSQL_REPO) $(LIBSQL_SRC_DIR)"; \
		git clone --depth 1 --branch $(LIBSQL_VERSION) $(LIBSQL_REPO) $(LIBSQL_SRC_DIR); \
	fi

# Build sqld for the host platform only and copy it under internal/sqld/bin/.
# Use this for Phase 1 validation; CI builds the cross-platform matrix via
# build-sqld-target (one job per Rust target on the matching native runner).
#
# IMPORTANT: cargo MUST be invoked from inside $(LIBSQL_SRC_DIR) so it picks
# up libsql's workspace `.cargo/config.toml` — that file sets rustflags
# `--cfg tokio_unstable` which the libsql-wal crate requires. Cargo's config
# discovery only walks UP from cwd, not down from --manifest-path, so running
# from the repo root makes libsql-wal fail with E0425 on `consume_budget`.
#
# To keep `cp` paths from breaking after the cd, we pre-compute absolute
# source/dest paths via $$(pwd) before cd-ing into the workspace.
#
# `&&` between commands ensures any failure aborts the recipe (the original
# `;`-joined version masked cp failures behind a trailing `echo "✓"`).
.PHONY: build-sqld
build-sqld: fetch-libsql-source
	@mkdir -p $(SQLD_EMBED_DIR)
	@host_os=$$(go env GOOS); \
	host_arch=$$(go env GOARCH); \
	ext=""; [ "$$host_os" = "windows" ] && ext=".exe"; \
	out="$$(pwd)/$(SQLD_EMBED_DIR)/sqld-$$host_os-$$host_arch$$ext"; \
	src="$$(pwd)/$(LIBSQL_SRC_DIR)/target/release/sqld$$ext"; \
	echo "→ building sqld for host ($$host_os-$$host_arch); bottomless is a path dep, no feature flag needed" && \
	cd $(LIBSQL_SRC_DIR) && \
	cargo build --release -p libsql-server --bin sqld && \
	cp "$$src" "$$out" && \
	chmod +x "$$out" && \
	test -x "$$out" && \
	ls -lh "$$out" && \
	echo "✓ sqld at $$out"

# CI-facing: build sqld for one specific Go OS/arch + Rust target. Required
# vars: GO_OS, GO_ARCH, RUST_TARGET. Each release.yml matrix entry calls this.
.PHONY: build-sqld-target
build-sqld-target: fetch-libsql-source
	@if [ -z "$(GO_OS)" ] || [ -z "$(GO_ARCH)" ] || [ -z "$(RUST_TARGET)" ]; then \
		echo "Usage: make build-sqld-target GO_OS=<os> GO_ARCH=<arch> RUST_TARGET=<rust-triple>"; \
		exit 1; \
	fi
	@mkdir -p $(SQLD_EMBED_DIR)
	@ext=""; [ "$(GO_OS)" = "windows" ] && ext=".exe"; \
	out="$$(pwd)/$(SQLD_EMBED_DIR)/sqld-$(GO_OS)-$(GO_ARCH)$$ext"; \
	src="$$(pwd)/$(LIBSQL_SRC_DIR)/target/$(RUST_TARGET)/release/sqld$$ext"; \
	echo "→ building sqld for $(GO_OS)-$(GO_ARCH) ($(RUST_TARGET))" && \
	cd $(LIBSQL_SRC_DIR) && \
	cargo build --release -p libsql-server --bin sqld --target $(RUST_TARGET) && \
	cp "$$src" "$$out" && \
	chmod +x "$$out" && \
	test -x "$$out" && \
	ls -lh "$$out" && \
	echo "✓ sqld at $$out"

# Cross-compile the Go CLI for every embedded platform. Assumes the matching
# sqld binaries are already in $(SQLD_EMBED_DIR) (CI populates these via
# build-sqld-target before invoking `release`; locally, `make build-sqld` only
# produces the host platform's binary, so non-host targets won't find a sqld
# to embed and will use the placeholder fallback).
.PHONY: release
release:
	@for spec in $(SQLD_PLATFORMS); do \
		os=$$(echo $$spec | cut -d: -f1); \
		goarch=$$(echo $$spec | cut -d: -f2); \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="$(BUILD_DIR)/$(BIN_NAME)-$$os-$$goarch$$ext"; \
		echo "→ building $$out (version=$(VERSION))"; \
		GOOS=$$os GOARCH=$$goarch go build \
			-ldflags="-s -w -X main.version=$(VERSION) -X github.com/Khangdang1690/sqlitedeploy/internal/sqld.downloadVersion=$(VERSION)" \
			-o "$$out" ./cmd/sqlitedeploy; \
	done
	@ls -lh $(BUILD_DIR)

# Map Go GOOS/GOARCH to npm platform-arch convention:
#   linux/amd64 → linux-x64        windows/amd64 → win32-x64
#   linux/arm64 → linux-arm64      windows/arm64 → win32-arm64
#   darwin/amd64 → darwin-x64      darwin/arm64 → darwin-arm64
.PHONY: package-npm
package-npm:
	@for spec in $(SQLD_PLATFORMS); do \
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

# prepare-maven copies the cross-compiled binaries into each platform
# module's resources dir, but stops short of running maven. The release
# workflow uses this so it can call `mvn deploy` directly without rebuilding
# via `mvn package` first.
.PHONY: prepare-maven
prepare-maven:
	@if [ "$(VERSION)" = "dev" ]; then echo "VERSION=x.y.z required"; exit 1; fi
	@rm -rf packaging/maven/launcher/target packaging/maven/platforms/*/target
	@for spec in $(MAVEN_PLATFORMS); do \
		go_os=$$(echo $$spec | cut -d: -f1); \
		go_arch=$$(echo $$spec | cut -d: -f2); \
		mvn_plat=$$(echo $$spec | cut -d: -f3); \
		ext=""; [ "$$go_os" = "windows" ] && ext=".exe"; \
		bin="$(BUILD_DIR)/$(BIN_NAME)-$$go_os-$$go_arch$$ext"; \
		[ -f "$$bin" ] || { echo "missing $$bin — run \`make release\` first"; exit 1; }; \
		dst_dir="packaging/maven/platforms/$$mvn_plat/src/main/resources/META-INF/native"; \
		rm -rf "$$dst_dir"; \
		mkdir -p "$$dst_dir"; \
		cp "$$bin" "$$dst_dir/$(BIN_NAME)$$ext"; \
		chmod +x "$$dst_dir/$(BIN_NAME)$$ext" || true; \
		echo "→ $$dst_dir/$(BIN_NAME)$$ext"; \
	done

.PHONY: package-maven
package-maven: prepare-maven
	@command -v mvn >/dev/null || { echo "mvn not found — install from https://maven.apache.org/install.html"; exit 1; }
	@echo "→ mvn -B package (in packaging/maven)"
	cd packaging/maven && mvn -B package
	@ls -lh packaging/maven/launcher/target/*.jar packaging/maven/platforms/*/target/*.jar

.PHONY: stamp-versions
stamp-versions:
	@if [ "$(VERSION)" = "dev" ]; then echo "VERSION=x.y.z required"; exit 1; fi
	@command -v node >/dev/null || { echo "node not found"; exit 1; }
	@command -v python3 >/dev/null || command -v python >/dev/null || { echo "python not found"; exit 1; }
	node scripts/stamp-versions.js $(VERSION)
	node scripts/stamp-versions-maven.js $(VERSION)
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
