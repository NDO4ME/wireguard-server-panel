# NDO4ME WireGuard Server Panel

[![Docker Image](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/docker-publish.yml)
[![CI](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/ci.yml/badge.svg)](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A self-hosted WireGuard VPN management panel with built-in Tailscale integration, shipped as a single Ubuntu 24.04 based Docker container. The backend is written in Go (Gin + SQLite), the web UI in React.

## Features

- **Client management** – create, rename, enable/disable and delete clients
- **QR codes & config download** – onboard mobile and desktop clients in seconds
- **Live statistics** – online state, last handshake, transfer counters, endpoint
- **Connection logging** – optional endpoint history per client
- **REST API** – permanent API token for automation
- **Tailscale integration** – route WireGuard clients into your tailnet
- **Tailscale auto-update** – checks the official stable channel, updates in place, verifies and rolls back on failure
- **Automatic container images** – built and published by GitHub Actions

## Requirements

- Docker (Compose v2 recommended)
- Linux host with WireGuard support (kernel 5.6+ or `wireguard-dkms`)
- TCP port `1881` for the panel, UDP port for WireGuard (default `1881`)
- Outbound access to `pkgs.tailscale.com` (only for Tailscale updates)

## Quick start

### Docker Compose (recommended)

```bash
git clone https://github.com/NDO4ME/wireguard-server-panel.git
cd wireguard-server-panel

# Set a strong password (also possible via a .env file)
export ADMIN_PASSWORD='change-me'

docker compose up -d
docker compose logs -f
```

The compose file pulls `ghcr.io/ndo4me/wireguard-server-panel:latest`. To build locally instead, comment out `image:` and uncomment `build: .`.

### Docker run

```bash
docker run -d \
  --name wireguardpanel \
  --cap-add NET_ADMIN \
  --cap-add SYS_MODULE \
  --sysctl net.ipv4.ip_forward=1 \
  --sysctl net.ipv4.conf.all.src_valid_mark=1 \
  --device /dev/net/tun:/dev/net/tun \
  --stop-timeout 40 \
  -p 1881:1881/tcp \
  -p 1881:51820/udp \
  -v wireguard-data:/data \
  -e ADMIN_PASSWORD='change-me' \
  --restart unless-stopped \
  ghcr.io/ndo4me/wireguard-server-panel:latest
```

Open `http://YOUR_SERVER_IP:1881` and sign in:

| Field    | Value                              |
|----------|------------------------------------|
| Username | `ndo4me`                           |
| Password | the value of `ADMIN_PASSWORD` (default `admin` – change it) |

> **Upgrading from an older release:** the fixed admin username changed from `layerweb` to `ndo4me`. It is migrated automatically; password, API token and sessions are kept.

## Configuration

All settings are environment variables.

| Variable                | Default        | Description |
|-------------------------|----------------|-------------|
| `ADMIN_PASSWORD`        | `admin`        | Admin password |
| `WG_HOST`               | auto-detected  | Public IP or hostname used in client configs |
| `WG_NETWORK`            | `10.8.0.0/24`  | VPN subnet (up to 253 clients per /24) |
| `WG_DNS`                | `1.1.1.1`      | DNS pushed to clients |
| `WG_PORT`               | `51820`        | WireGuard listen port inside the container |
| `WG_PUBLIC_PORT`        | `1881`         | UDP port clients connect to (host side of `-p`) |
| `PANEL_PORT`            | `1881`         | Panel HTTP port |
| `TAILSCALE_AUTO_UPDATE` | `true`         | Default for automatic Tailscale updates (the panel toggle overrides it) |
| `TZ`                    | `UTC`          | Timezone |

### Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 1881 | TCP      | Web panel and API |
| 1881 | UDP      | WireGuard (mapped to 51820 in the container) |

### Persistent data

Everything lives in the single `/data` volume:

```text
/data/
├── wireguard/   # server keys and wg0.conf
├── db/          # SQLite database and persisted JWT secret
└── tailscale/   # Tailscale node state
```

## Usage

1. Click **+**, enter a name (for example `Phone`) and create the client.
2. Scan the QR code with the WireGuard app or download the `.conf` file.
3. Use the pencil, power, QR, download and trash icons to rename, disable, share or delete a client.

### REST API

The API token is shown on the **Settings** page and stays constant until the admin password changes.

```bash
# List clients
curl -H "Authorization: Bearer $TOKEN" http://YOUR_SERVER:1881/api/v1/peers

# Create a client
curl -X POST http://YOUR_SERVER:1881/api/v1/peers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "My Phone"}'

# Delete a client
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://YOUR_SERVER:1881/api/v1/peers/10.8.0.2
```

Health check: `GET /api/health`.

## Tailscale

The image ships Tailscale from the official stable apt repository. `tailscaled` is started and supervised by the panel.

### Connect and route

1. Open **Settings → Tailscale Integration** and click **Connect**.
2. Open the displayed auth URL and sign in.
3. Enable **Route WireGuard to Tailscale**. WireGuard clients can then reach tailnet peers (`100.x.y.z`) and advertised subnet routes without installing Tailscale. The iptables rules are idempotent and restored automatically after a restart.

### Automatic updates

**Settings → Tailscale Integration → Version** shows the installed and the latest stable version.

- **Check for updates / Update to x.y.z** for manual control.
- **Automatic updates** run every 6 hours when enabled.
- Only newer stable releases are installed – never a downgrade.
- After the update `tailscaled` is restarted by the panel and both the daemon version and the Tailscale connection are verified.
- If anything fails, the previous package version is restored automatically and the failing version is skipped by the automatic updater.
- WireGuard clients stay connected; only the Tailscale link drops for a few seconds.

Updates need the panel to run as root with the bundled apt repository (true for the official image). Elsewhere the panel shows why updates are unavailable.

Pin a version at build time (the panel can still update it later):

```bash
docker build --build-arg TAILSCALE_VERSION=1.80.3 -t wireguardpanel .
```

## Operations

```bash
docker logs -f wireguardpanel          # logs
docker exec wireguardpanel wg show     # WireGuard status
docker exec wireguardpanel tailscale version
docker restart wireguardpanel
```

### Reverse proxy (nginx)

```nginx
server {
    listen 443 ssl http2;
    server_name vpn.example.com;

    location / {
        proxy_pass http://127.0.0.1:1881;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Backup and restore

```bash
# Backup
docker run --rm -v wireguard-data:/data -v "$PWD":/backup ubuntu:24.04 \
  tar czf /backup/wireguard-backup.tar.gz -C /data .

# Restore
docker run --rm -v wireguard-data:/data -v "$PWD":/backup ubuntu:24.04 \
  tar xzf /backup/wireguard-backup.tar.gz -C /data
```

### Reset the admin password

Stop the container, then remove only the database (WireGuard keys are kept; clients must be recreated, because they are stored in the database):

```bash
docker stop wireguardpanel
docker run --rm -v wireguard-data:/data ubuntu:24.04 rm /data/db/wireguard.db
docker start wireguardpanel   # new admin is created from ADMIN_PASSWORD
```

## Troubleshooting

- **Container does not start** – check `docker logs wireguardpanel`; the usual causes are a busy port, a missing `NET_ADMIN` capability or a kernel without WireGuard.
- **Clients cannot connect** – allow UDP `1881` in the firewall, verify `wg show`, and check that the client endpoint matches `WG_HOST` and `WG_PUBLIC_PORT`.
- **Tailscale update unavailable** – the Version section states the reason (not root, apt repository missing, externally managed daemon).

## Security notes

- Change the default admin password immediately and serve the panel over HTTPS.
- The JWT secret is generated once and persisted in `/data/db`, so sessions survive restarts.
- Login is rate limited; passwords are stored as bcrypt hashes.
- Client private keys are stored in the SQLite database **unencrypted** – protect the `/data` volume and backups accordingly.

## Architecture

```text
┌──────────────────── Docker container (Ubuntu 24.04) ────────────────────┐
│  Go API (Gin) ──── React UI (Vite)                                       │
│       │                                                                  │
│  SQLite ── WireGuard (wg0) ── tailscaled (supervised, auto-updated)      │
└──────────────────────────────────────────────────────────────────────────┘
```

## Development

```bash
make web      # build the frontend
make build    # build the Go binary (CGO required)
make test     # run Go tests
make docker   # build the image locally
```

## CI/CD

- **CI** (`.github/workflows/ci.yml`) – `go vet`, `go test`, Go and frontend build, and a Docker image smoke test on every push and pull request.
- **Docker Image** (`.github/workflows/docker-publish.yml`) – builds the image and publishes it to GitHub Container Registry:
  - push to `main` → `latest` and `sha-<short>`
  - tag `vX.Y.Z` → `X.Y.Z` and `X.Y`
  - weekly scheduled rebuild for fresh Ubuntu and Tailscale patches
- **Dependabot** keeps Go modules, npm packages, GitHub Actions and base images current.

Release: `git tag v1.0.0 && git push origin v1.0.0`.

If the package is private after the first publish, set it to public under *Packages → wireguard-server-panel → Package settings*.

## License

MIT – see [LICENSE](LICENSE).
