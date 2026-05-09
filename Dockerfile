# Multi-stage build for sqlitedeploy. Lets Windows/macOS users run sqld
# without WSL: build inside a Linux container, mount your project dir,
# run `dev` or `up` from anywhere.
#
# Build:   docker build -t sqlitedeploy .
# Dev:     docker run --rm -it -v ${PWD}:/work sqlitedeploy dev
# Up:      docker run --rm -it -v ${PWD}:/work \
#              -v sqlitedeploy-creds:/root/.config/sqlitedeploy \
#              sqlitedeploy up

# ── Stage 1: build sqld (Rust) ──────────────────────────────────────────────
FROM rust:1.82-slim-bookworm AS sqld-builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    git pkg-config libssl-dev ca-certificates protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*
ARG LIBSQL_VERSION=libsql-server-v0.24.32
RUN git clone --depth 1 --branch ${LIBSQL_VERSION} \
    https://github.com/tursodatabase/libsql.git /libsql
WORKDIR /libsql
# `.cargo/config.toml` in the libsql workspace sets `--cfg tokio_unstable`,
# which libsql-wal needs. Cargo only walks UP from cwd, so we must build
# from inside the workspace, not via --manifest-path from /.
RUN cargo build --release -p libsql-server --bin sqld
# Stage the binary under the name our Go embed code looks up at runtime
# (sqld-linux-<dpkg-arch>). This is what makes the image work on both
# x86_64 (most desktops) and aarch64 (Oracle Free Tier ARM, Apple Silicon
# CI runners, etc.) without a separate build matrix.
RUN mkdir -p /artifact && \
    cp target/release/sqld "/artifact/sqld-linux-$(dpkg --print-architecture)"

# ── Stage 2: build sqlitedeploy (Go) with sqld embedded ─────────────────────
FROM golang:1.25-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p internal/sqld/bin
COPY --from=sqld-builder /artifact/ internal/sqld/bin/
RUN go build -ldflags="-s -w" -o /out/sqlitedeploy ./cmd/sqlitedeploy

# ── Stage 3: runtime ────────────────────────────────────────────────────────
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
# Pre-install cloudflared so the first `up` doesn't pay a 30 MB download.
# Cloudflare publishes both amd64 and arm64 with matching dpkg-arch suffixes.
ARG CLOUDFLARED_VERSION=2024.11.1
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://github.com/cloudflare/cloudflared/releases/download/${CLOUDFLARED_VERSION}/cloudflared-linux-${ARCH}" \
        -o /usr/local/bin/cloudflared \
    && chmod +x /usr/local/bin/cloudflared
COPY --from=go-builder /out/sqlitedeploy /usr/local/bin/sqlitedeploy
WORKDIR /work
ENTRYPOINT ["sqlitedeploy"]
CMD ["--help"]
