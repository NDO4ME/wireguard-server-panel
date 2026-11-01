# NDO4ME WireGuard Server Panel

[![Docker Image](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/docker-publish.yml)
[![CI](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/ci.yml/badge.svg)](https://github.com/NDO4ME/wireguard-server-panel/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A self-hosted WireGuard VPN management panel with built-in Tailscale integration, shipped as a single Ubuntu 24.04 based Docker container. The backend is written in Go (Gin + SQLite), the web UI in React.

## Features

- **Client management** – create, rename, enable/disable and delete clients
- **QR codes & config download** – onboard mobile and desktop clients in seconds; optionally with a hostname as endpoint instead of the IP
- **Live statistics** – online state, last handshake, transfer counters, endpoint
- **Connection logging** – optional endpoint history per client
- **White-label login page** – only a password field and a button; optional logo, heading, tab title and accent colour from Settings
- **REST API** – permanent API token for automation
- **Tailscale integration** – route WireGuard clients into your tailnet
- **Tailscale auto-update** – checks the official stable channel, updates in place, verifies and rolls back on failure
- **Automatic container images** – built and published by GitHub Actions

## Requirements

- Docker (Compose v2 recommended)
- Linux host with WireGuard support (kernel 5.6+ or `wireguard-dkms`)
- TCP port `1881` for the panel (bound to `127.0.0.1` only, published through Caddy), UDP port for WireGuard (default `1881`)
- A domain name pointing to the server (for automatic HTTPS with Caddy)
- Outbound access to `pkgs.tailscale.com` (only for Tailscale updates)

## Quick start

### Docker Compose (recommended)

```bash
git clone https://github.com/NDO4ME/wireguard-server-panel.git
cd wireguard-server-panel

# Set a strong password (also possible via a .env file).
# If you skip this, a random password is generated and printed once in the logs.
export ADMIN_PASSWORD='use-a-long-random-password'

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
  -p 127.0.0.1:1881:1881/tcp \
  -p 1881:51820/udp \
  -v wireguard-data:/data \
  -e ADMIN_PASSWORD='use-a-long-random-password' \
  --restart unless-stopped \
  ghcr.io/ndo4me/wireguard-server-panel:latest
```

The panel is reachable only from the server itself (`http://127.0.0.1:1881`). Publish it with Caddy as described under [Reverse proxy (Caddy)](#reverse-proxy-caddy), then open `https://vpn.example.com` and sign in:

| Field    | Value                              |
|----------|------------------------------------|
| Username | `ndo4me`                           |
| Password | the value of `ADMIN_PASSWORD` (if unset, a random one is printed once in `docker logs`) |

> **Upgrading from an older release:** the fixed admin username changed from `layerweb` to `ndo4me`. It is migrated automatically; password and sessions are kept; the API token is rotated once (it is no longer derived from the password).

## Configuration

All settings are environment variables.

| Variable                | Default        | Description |
|-------------------------|----------------|-------------|
| `ADMIN_PASSWORD`        | random         | Initial admin password (a weak default is refused) |
| `WG_HOST`               | auto-detected  | Public IP or hostname used in client configs |
| `WG_NETWORK`            | `10.8.0.0/24`  | VPN subnet (up to 253 clients per /24) |
| `WG_DNS`                | `1.1.1.1`      | DNS pushed to clients |
| `WG_PORT`               | `51820`        | WireGuard listen port inside the container |
| `WG_PUBLIC_PORT`        | `1881`         | UDP port clients connect to (host side of `-p`) |
| `PANEL_PORT`            | `1881`         | Panel HTTP port |
| `TAILSCALE_AUTO_UPDATE` | `true`         | Default for automatic Tailscale updates (the panel toggle overrides it) |
| `TRUSTED_PROXIES`       | loopback + private ranges | Comma separated CIDRs allowed to set `X-Forwarded-For` |
| `TZ`                    | `UTC`          | Timezone |

### Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 1881 | TCP      | Web panel and API (`127.0.0.1` only; Caddy serves 80/443) |
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

1. Open **Clients**, click **New client**, enter a name (for example `Phone`) and create it.
2. Scan the QR code with the WireGuard app or download the `.conf` file.
3. Use the row actions (power, QR code, download, delete) to disable, share or remove a client; click a name to rename it.
4. Optional: **Settings → Client defaults → Endpoint host** replaces the server IP in downloaded configs and QR codes with a hostname (for example `vpn.example.com`). The port and the server stay unchanged, so when the server IP changes you only update the DNS record and existing clients keep working without new configs. Clients created before keep their old address inside the files already handed out, so use the hostname from the start.
5. Optional: **Settings → Login page** sets a logo, heading, browser tab title and accent colour for the sign-in page. By default it shows no name or text, only a password field and a button.

### REST API

The API token is shown on the **API access** page and is random and independent of the password. It changes only when the admin password changes.

```bash
# List clients
curl -H "Authorization: Bearer $TOKEN" https://vpn.example.com/api/v1/peers

# Create a client
curl -X POST https://vpn.example.com/api/v1/peers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "My Phone"}'

# Delete a client
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  https://vpn.example.com/api/v1/peers/10.8.0.2
```

Health check: `GET /api/health`.

## Tailscale

The image ships Tailscale from the official stable apt repository. `tailscaled` is started and supervised by the panel.

### Connect and route

1. Open **Tailscale** in the sidebar and click **Connect**.
2. Open the displayed auth URL and sign in.
3. Enable **Route WireGuard clients into Tailscale**. WireGuard clients can then reach tailnet peers (`100.x.y.z`) and advertised subnet routes without installing Tailscale. The iptables rules are idempotent and restored automatically after a restart.

### DNS

Tailscale runs with **MagicDNS / DNS management disabled** (`--accept-dns=false`). `tailscaled` never rewrites the server's `/etc/resolv.conf`, so the host's own name resolution is unaffected. The setting is enforced every time `tailscaled` starts, also for nodes created by older releases. Tailnet peers are reached by their `100.x.y.z` address.

### Automatic updates

**Tailscale → Version & updates** shows the installed and the latest stable version.

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

## Updating

Your data lives in the `/data` volume and is kept across updates. Take a backup first (see [Backup and restore](#backup-and-restore)), then pull the new image and recreate the container.

### Docker Compose

```bash
cd wireguard-server-panel
docker compose pull                      # fetch the latest image
docker compose up -d                     # recreate the container with it
docker compose logs -f --tail=50         # watch it come up
docker image prune -f                    # optional: remove the old image
```

### Docker run

```bash
docker pull ghcr.io/ndo4me/wireguard-server-panel:latest
docker stop wireguardpanel               # waits for a clean shutdown (up to 40 s)
docker rm wireguardpanel
# run the same `docker run ...` command as in Quick start again (same -v volume)
```

### Migrating from the old `layerweb/wireguard-panel` image

Servers installed from the previous image keep their data and move over in place. The old image name receives no more updates, so switch to the new one:

```bash
cd /path/to/old/compose-folder            # where docker-compose.yml and ./data live

# 1. Backup (stop first so the SQLite file is consistent)
docker compose down
tar czf ../wireguard-backup-$(date +%F).tar.gz ./data

# 2. Point the compose file at the new image and the localhost-only port
sed -i 's#layerweb/wireguard-panel:latest#ghcr.io/ndo4me/wireguard-server-panel:latest#' docker-compose.yml
sed -i 's#- "1881:1881/tcp"#- "127.0.0.1:1881:1881/tcp"#' docker-compose.yml

# 3. Pull and start
docker compose pull
docker compose up -d
docker compose logs -f --tail=50
```

#### Installed with `docker run`

Keep the **same container name and the same `-v` volume** as the old command (`wireguard-data` in the old README, or `$(pwd)/data` for the local-directory variant; check with `docker inspect wireguardpanel --format '{{ range .Mounts }}{{ .Name }}{{ .Source }} {{ end }}'`).

```bash
# 1. Backup (named volume; for a local directory just tar that folder)
docker stop wireguardpanel
docker run --rm -v wireguard-data:/data -v "$PWD":/backup ubuntu:24.04 \
  tar czf /backup/wireguard-backup-$(date +%F).tar.gz -C /data .

# 2. Pull the new image and remove the old container (data stays in the volume)
docker pull ghcr.io/ndo4me/wireguard-server-panel:latest
docker rm wireguardpanel

# 3. Start it again: same volume, new image, panel port on localhost only
docker run -d \
  --name wireguardpanel \
  --cap-add NET_ADMIN \
  --cap-add SYS_MODULE \
  --sysctl net.ipv4.ip_forward=1 \
  --sysctl net.ipv4.conf.all.src_valid_mark=1 \
  --device /dev/net/tun:/dev/net/tun \
  --stop-timeout 40 \
  -p 127.0.0.1:1881:1881/tcp \
  -p 1881:51820/udp \
  -v wireguard-data:/data \
  -e ADMIN_PASSWORD='your-existing-password' \
  --restart unless-stopped \
  ghcr.io/ndo4me/wireguard-server-panel:latest

# 4. Check
docker logs -f --tail=50 wireguardpanel
```

If you used other `-p`, `-e WG_HOST=...` or `-e WG_NETWORK=...` options before, repeat them. The existing admin password in the database wins; `ADMIN_PASSWORD` only matters for a fresh volume.

What changes for you:

- Clients, keys, settings and the admin password are kept; the admin user `layerweb` becomes `ndo4me` automatically.
- The API token is regenerated once (it is no longer derived from the password). Read the new one on **API access** and update your scripts.
- Browser sessions are signed out once. Sign in again.
- The panel is now only on `127.0.0.1:1881`. Put Caddy in front of it ([Reverse proxy](#reverse-proxy-caddy)) or tunnel with `ssh -L 1881:127.0.0.1:1881 user@server`.
- Tailscale stays connected (its state is in `./data/tailscale`) and can then be updated from the panel.

### Check the result

```bash
docker ps --filter name=wireguardpanel                   # status should become "healthy"
curl -s http://127.0.0.1:1881/api/health                 # panel answers
docker exec wireguardpanel wg show                       # WireGuard peers are back
docker exec wireguardpanel tailscale version             # installed Tailscale version
```

### Roll back

Pin the previous release instead of `latest`, for example `image: ghcr.io/ndo4me/wireguard-server-panel:1.2.0` (or `:sha-<short>`) in `docker-compose.yml`, then run `docker compose up -d`. Available tags are listed under *Packages* on GitHub.

### Tailscale only

Tailscale is updated from the panel without touching the container: **Tailscale → Version & updates → Check for updates → Update**. The same can be done manually:

```bash
docker exec wireguardpanel apt-get update
docker exec wireguardpanel apt-get install -y --only-upgrade tailscale
docker restart wireguardpanel
```

## Operations

```bash
docker logs -f wireguardpanel          # logs
docker exec wireguardpanel wg show     # WireGuard status
docker exec wireguardpanel tailscale version
docker restart wireguardpanel
```

### Reverse proxy (Caddy)

The panel listens on `127.0.0.1:1881` only. [Caddy](https://caddyserver.com) publishes it over HTTPS and handles the certificate automatically. Ready-made file: [`deploy/Caddyfile`](deploy/Caddyfile).

```caddyfile
vpn.example.com {
	encode zstd gzip

	header {
		Strict-Transport-Security "max-age=31536000"
		-Server
	}

	reverse_proxy 127.0.0.1:1881
}
```

```bash
sudo apt install -y caddy                       # or see caddyserver.com/docs/install
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile   # edit the domain first
sudo systemctl reload caddy
```

Open ports 80 and 443 (TCP) for Caddy and the WireGuard UDP port. Do not publish 1881/tcp. Caddy sets `X-Forwarded-For` itself; if Caddy runs in another container, set `TRUSTED_PROXIES` to its network.

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
- **Tailscale update unavailable** – the Version & updates panel states the reason (not root, apt repository missing, externally managed daemon).

## Security notes

- The panel port is bound to `127.0.0.1`; serve it over HTTPS with Caddy.
- There is no default password: set `ADMIN_PASSWORD` or use the random one printed on first start.
- The panel refuses to start with a default or short JWT secret (the Docker image generates a strong one).
- The API token is random (256 bit), never derived from the password, and rotated when the password changes. Upgrading from a release that derived it from the password rotates it once; update your API clients.
- The JWT secret is generated once and persisted in `/data/db`, so sessions survive restarts.
- Login is rate limited per client IP (`X-Forwarded-For` is trusted from local proxies only); passwords are stored as bcrypt hashes.
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
