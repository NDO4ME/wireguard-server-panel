# Build stage - Go backend
FROM golang:1.21-alpine AS builder

# Install build dependencies for CGO (SQLite requires it)
RUN apk add --no-cache git make gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o wireguardpanel ./cmd/api

# Build stage - Frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web

COPY web/package*.json ./
# Install all dependencies (including devDependencies for build)
RUN npm ci

COPY web/ ./
RUN npm run build

# Final stage - Production
FROM alpine:3.19

LABEL maintainer="WireGuard Panel"
LABEL description="WireGuard VPN Management Panel"

# Install runtime dependencies including Tailscale
RUN apk add --no-cache \
    wireguard-tools \
    iptables \
    ip6tables \
    iproute2 \
    curl \
    bash \
    libgcc \
    ca-certificates \
    tzdata \
    jq \
    tailscale \
    && rm -rf /var/cache/apk/* /tmp/*

# Create non-root user for security (but we need root for WireGuard)
# Running as root is required for network operations

# Create app directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/wireguardpanel .

# Copy frontend from builder
COPY --from=frontend-builder /app/web/dist ./web/dist

# Copy config template
COPY configs/config.yaml ./configs/config.yaml

# Create unified data directory structure
# /data will be the single external volume containing everything
RUN mkdir -p /data/wireguard /data/db && \
    chmod 700 /data/wireguard && \
    chmod 755 /data/db

# Expose ports
EXPOSE 1881/tcp
EXPOSE 51820/udp

# Environment variables with defaults
# ADMIN_USERNAME is fixed to 'layerweb' in entrypoint
# JWT_SECRET is auto-generated randomly in entrypoint for security
# ADMIN_PASSWORD should be set at runtime via -e flag
ENV WG_HOST="" \
    WG_PORT=51820 \
    WG_NETWORK="10.8.0.0/24" \
    WG_DNS="1.1.1.1" \
    PANEL_PORT=1881 \
    DATABASE_PATH="/data/db/wireguard.db" \
    TZ="UTC" \
    GOGC=100 \
    GOMEMLIMIT=512MiB

# Define volume for persistent data
VOLUME /data

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -sf http://localhost:${PANEL_PORT}/api/health || exit 1

# Copy and setup entrypoint
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]
