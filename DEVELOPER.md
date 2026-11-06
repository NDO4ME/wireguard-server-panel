# NDO4 WG – Developer & Operations Guide

Full reference for installing, configuring, updating, securing and developing the NDO4 WG panel. For a short overview see the [README](README.md).

## Contents

- [Features](#features) · [Requirements](#requirements) · [Quick start](#quick-start)
- [Configuration](#configuration) · [Usage and REST API](#usage)
- [Tailscale](#tailscale) · [WireGuard tools updates](#wireguard-tools-updates)
- [Updating](#updating) · [Rolling back](#roll-back) · [Migrating from the old image](#migrating-from-the-old-layerwebwireguard-panel-image)
- [Operations](#operations) · [Reverse proxy (Caddy)](#reverse-proxy-caddy) · [Backup and restore](#backup-and-restore)
- [Troubleshooting](#troubleshooting) · [Security notes](#security-notes)
- [Architecture](#architecture) · [Development](#development) · [CI/CD](#cicd)

## Features

- **Client management** – create, rename, enable/disable and delete clients
- **QR codes & config download** – onboard mobile and desktop clients in seconds; optionally with a hostname as endpoint instead of the IP
- **Client expiry & notes** – optional expiry date per client; expired clients are disabled automatically and cannot be re-enabled until the date is changed
- **Live statistics** – online state, last handshake, transfer counters, endpoint
- **Connection logging** – optional endpoint history per client
- **White-label login page** – only a password field and a button; optional logo, heading, tab title and accent colour from Settings
- **REST API** – random API token for automation, rotatable from the UI
- **System page** – interface health, client counts, versions and WireGuard tools updates
- **Activity log** – the last 1000 state-changing requests and sign-in attempts (time, route, client IP, result; never request contents or secrets)
- **Database backup** – one-click consistent snapshot download (Settings → Backup)
- **Tailscale integration** – route WireGuard clients into your tailnet
- **Tailscale node options** – exit node, subnet routes, accept routes and shields-up, applied live with `tailscale set`
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
  --log-opt max-size=10m --log-opt max-file=3 \
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
| `WG_NETWORK`            | `10.8.0.0/24`  | VPN subnet, a private `/24` such as `10.8.1.0/24` (up to 253 clients). Choose it before the first start; see [Custom VPN subnet](#custom-vpn-subnet) |
| `WG_DNS`                | `1.1.1.1`      | DNS pushed to clients |
| `WG_PORT`               | `51820`        | WireGuard listen port inside the container |
| `WG_PUBLIC_PORT`        | `1881`         | UDP port clients connect to (host side of `-p`) |
| `PANEL_PORT`            | `1881`         | Panel HTTP port |
| `TAILSCALE_AUTO_UPDATE` | `true`         | Default for automatic Tailscale updates (the panel toggle overrides it) |
| `IP_OPTIONS_FILTER`     | `true`         | Drop inbound IPv4 packets with IP options inside the container (CVE-2026-98432 hardening) |
| `WIREGUARD_AUTO_UPDATE` | `true`         | Default for automatic WireGuard tools updates (System page toggle overrides it) |
| `TRUSTED_PROXIES`       | loopback + private ranges | Comma separated CIDRs allowed to set `X-Forwarded-For` |
| `TZ`                    | `UTC`          | Timezone |

### Custom VPN subnet

The default subnet is `10.8.0.0/24`. To use another one, set `WG_NETWORK` to a private `/24` **on the first start**:

```bash
# Docker Compose
WG_NETWORK=10.8.1.0/24 docker compose up -d

# docker run: add to the command
  -e WG_NETWORK=10.8.1.0/24 \
```

- The server takes `.1`, clients get `.2` to `.254`.
- Use a range that does not clash with your LAN, other VPNs or Tailscale (`100.64.0.0/10` is rejected).
- Invalid values stop the container with a clear message; a host-bits-set value such as `10.8.1.5/24` is rejected too.
- Changing it later affects only new clients: existing clients keep their old addresses and stop working. The panel logs a warning for them. Delete and recreate those clients, or set the old value back.

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

### Node options

**Tailscale → Node options** applies settings to the running daemon without a reconnect: accept routes, advertise as exit node, advertise subnet routes (CIDR, one per line) and shields-up. Routes and exit nodes still need approval in the Tailscale admin console. Default routes (`0.0.0.0/0`, `::/0`) are rejected in the route list; use the exit node switch.

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
  --log-opt max-size=10m --log-opt max-file=3 \
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

## WireGuard tools updates

**System → WireGuard tools updates** works like the Tailscale updater and is enabled by default (`WIREGUARD_AUTO_UPDATE=true`; the switch in the UI overrides it).

- Only the userspace tools (`wireguard-tools`) are updated. The data path is the host kernel module and is updated together with the host.
- The interface and all client tunnels stay up. Peer changes are paused while the package is replaced, the result is verified, and the previous version is restored automatically on failure.

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

**Settings → Backup** downloads a consistent database snapshot (API: `GET /api/v1/settings/backup`). It contains private keys; keep it safe. The API token can be rotated under **API access → Rotate token**.

Volume-level backup:

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
- **CVE-2026-98432 (Linux kernel IPv4 TCP use-after-free):** a container shares the host kernel, so the real fix is a patched **host** kernel (`apt update && apt full-upgrade`, then reboot, as soon as your distribution ships it). Inside the image the panel adds defence in depth: Multipath TCP is disabled for the panel listener (`GODEBUG=multipathtcp=0`) and in the container's network namespace, and inbound IPv4 packets carrying IP options (the exploit trigger) are dropped by an `iptables` rule. The rule needs the `xt_u32` kernel module; if the host lacks it the container starts normally and logs a notice. Set `IP_OPTIONS_FILTER=false` to skip the rule.
- Client private keys are stored in the SQLite database **unencrypted** – protect the `/data` volume and backups accordingly.

## Resource usage

The panel is built to stay small and stable over months of uptime:

- **Logs:** no per-request logging; Docker log files are rotated (`max-size 10m`, 3 files; add the `--log-opt` flags shown above to `docker run` installs).
- **Database:** SQLite in WAL mode with incremental vacuum and an hourly maintenance run. Expired sessions are removed, the activity log keeps the last 1000 entries, and the connection history keeps at most 200 entries and 180 days per client.
- **Updaters:** apt package lists and cached `.deb` files are deleted after every update check or update, so the container layer does not grow. WireGuard and Tailscale updates never run at the same time.
- **Memory:** `GOMEMLIMIT=512MiB`, a single database connection and an hourly garbage collection that returns memory to the OS.
- **CPU:** background work is limited to a one-minute expiry check, a 6-hour update check per component and the web UI pauses its refresh while the browser tab is hidden.

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
