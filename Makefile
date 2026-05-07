# sqlitedeploy build targets
#
#   make build              build the CLI for the current platform
#   make fetch-litestream   download upstream litestream releases into the
#                           embed dir so they get baked into the binary
#   make release            build static binaries for every embedded platform
#
# On Windows without make/bash, use scripts\fetch-litestream.ps1 instead.

LITESTREAM_VERSION ?= 0.5.11
EMBED_DIR := internal/litestream/bin
BIN_NAME := sqlitedeploy
BUILD_DIR := dist

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
	go build -o $(BUILD_DIR)/$(BIN_NAME) ./cmd/sqlitedeploy

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
		echo "→ building $$out"; \
		GOOS=$$os GOARCH=$$goarch go build -ldflags="-s -w" -o "$$out" ./cmd/sqlitedeploy; \
	done
	@ls -lh $(BUILD_DIR)

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
