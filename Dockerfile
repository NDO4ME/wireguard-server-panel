# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Build stage - Go backend (glibc, CGO for SQLite)
# ---------------------------------------------------------------------------
FROM golang:1.26-bookworm AS builder

ARG APP_VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X main.appVersion=${APP_VERSION}" \
    -o wireguardpanel ./cmd/api

# ---------------------------------------------------------------------------
# Build stage - Frontend
# ---------------------------------------------------------------------------
FROM node:24-alpine AS frontend-builder

WORKDIR /app/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ---------------------------------------------------------------------------
# Final stage - Ubuntu 24.04 (LTS)
# ---------------------------------------------------------------------------
FROM ubuntu:24.04

LABEL org.opencontainers.image.title="NDO4ME WireGuard Panel" \
      org.opencontainers.image.description="WireGuard VPN management panel with Tailscale integration" \
      org.opencontainers.image.vendor="NDO4ME" \
      org.opencontainers.image.licenses="MIT"

# Optional: pin a Tailscale version at build time (default: latest stable).
# The admin panel can update Tailscale at runtime either way.
ARG TAILSCALE_VERSION=""

ENV DEBIAN_FRONTEND=noninteractive

# Runtime dependencies + Tailscale from the official stable apt repository.
# The repository stays configured so the panel can update Tailscale in place.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        wireguard-tools \
        iptables \
        iproute2 \
        curl \
        bash \
        ca-certificates \
        tzdata \
        jq \
        tini \
    && curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/noble.noarmor.gpg \
        -o /usr/share/keyrings/tailscale-archive-keyring.gpg \
    && curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/noble.tailscale-keyring.list \
        -o /etc/apt/sources.list.d/tailscale.list \
    && apt-get update \
    && if [ -n "${TAILSCALE_VERSION}" ]; then \
         apt-get install -y --no-install-recommends "tailscale=${TAILSCALE_VERSION}"; \
       else \
         apt-get install -y --no-install-recommends tailscale; \
       fi \
    && tailscale version \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/* /tmp/*

WORKDIR /app

COPY --from=builder /app/wireguardpanel .
COPY --from=frontend-builder /app/web/dist ./web/dist
COPY configs/config.yaml ./configs/config.yaml

# Single external volume containing everything
RUN mkdir -p /data/wireguard /data/db /data/tailscale && \
    chmod 700 /data/wireguard /data/tailscale && \
    chmod 755 /data/db

EXPOSE 1881/tcp
EXPOSE 51820/udp

# ADMIN_USERNAME is fixed to 'ndo4me' in the entrypoint
# JWT secret is generated once and persisted in /data/db by the entrypoint
# ADMIN_PASSWORD should be set at runtime via -e flag
ENV WG_HOST="" \
    WG_PORT=51820 \
    WG_NETWORK="10.8.0.0/24" \
    WG_DNS="1.1.1.1" \
    PANEL_PORT=1881 \
    DATABASE_PATH="/data/db/wireguard.db" \
    TAILSCALE_AUTO_UPDATE="true" \
    WIREGUARD_AUTO_UPDATE="true" \
    TZ="UTC" \
    GOGC=100 \
    GOMEMLIMIT=512MiB

VOLUME /data

HEALTHCHECK --interval=30s --timeout=10s --start-period=20s --retries=3 \
    CMD curl -sf http://localhost:${PANEL_PORT}/api/health || exit 1

COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# tini reaps zombie processes and forwards signals (tailscaled child process)
ENTRYPOINT ["/usr/bin/tini", "--", "/docker-entrypoint.sh"]
