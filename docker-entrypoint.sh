#!/bin/bash
set -e

echo "=========================================="
echo "  NDO4ME WireGuard Panel - Starting..."
echo "=========================================="

# IP forwarding is set via docker run sysctls
# Just verify it's enabled
if [ "$(cat /proc/sys/net/ipv4/ip_forward 2>/dev/null)" != "1" ]; then
    echo "WARNING: IP forwarding is not enabled. VPN may not work correctly."
    echo "Make sure to run with --sysctl net.ipv4.ip_forward=1"
fi

# --- Hardening against CVE-2026-98432 (Linux kernel IPv4 TCP use-after-free) ---
# The container shares the host kernel, so only a host kernel update fixes it.
# These two measures run inside this container's own network namespace, are
# best-effort and never stop the start-up.
#  1. No Multipath TCP for sockets in this namespace (known exploit path).
if [ -w /proc/sys/net/mptcp/enabled ]; then
    echo 0 > /proc/sys/net/mptcp/enabled 2>/dev/null && echo "Hardening: MPTCP disabled in container"
fi
#  2. Drop inbound IPv4 packets that carry IP options (the trigger). Legitimate
#     traffic does not use them. Set IP_OPTIONS_FILTER=false to skip.
if [ "${IP_OPTIONS_FILTER:-true}" != "false" ]; then
    IPOPT_RULE=(INPUT -m u32 --u32 "0>>24&0x0f=6:15" -j DROP)
    if iptables -C "${IPOPT_RULE[@]}" 2>/dev/null; then
        :
    elif iptables -I "${IPOPT_RULE[@]}" 2>/dev/null; then
        echo "Hardening: inbound IPv4 packets with IP options are dropped"
    else
        echo "Hardening: IP options filter unavailable on this host (xt_u32 missing); relying on a patched host kernel"
    fi
fi

# Ensure data directories exist with proper permissions
mkdir -p /data/wireguard /data/db /data/tailscale
chmod 700 /data/wireguard
chmod 755 /data/db
chmod 700 /data/tailscale

# Create symlink for WireGuard config location
if [ ! -L /etc/wireguard ]; then
    rm -rf /etc/wireguard 2>/dev/null || true
    ln -sf /data/wireguard /etc/wireguard
fi

# tailscaled is started and supervised by the panel itself (see pkg/tailscale),
# which lets the admin panel restart it safely when Tailscale is updated.
mkdir -p /var/run/tailscale /var/lib/tailscale

# Static admin username - cannot be changed
export ADMIN_USERNAME="ndo4me"

# JWT secret: generated once and persisted so admin sessions survive restarts
JWT_FILE="/data/db/.jwt_secret"
if [ ! -s "$JWT_FILE" ]; then
    ( umask 077; head -c 64 /dev/urandom | base64 | tr -d '\n=' | head -c 64 > "$JWT_FILE" )
    echo "Generated secure random JWT secret"
fi
JWT_SECRET=$(cat "$JWT_FILE")
export JWT_ACCESS_SECRET="${JWT_SECRET}"
export JWT_REFRESH_SECRET="${JWT_SECRET}-refresh"

# No default password: generate a random one on first start when none is given.
# It is only used to create the admin user the first time.
if [ -z "$ADMIN_PASSWORD" ]; then
    PW_FILE="/data/db/.admin_password"
    if [ ! -s "$PW_FILE" ]; then
        ( umask 077; head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 20 > "$PW_FILE" )
        echo "ADMIN_PASSWORD not set - generated a random admin password: $(cat "$PW_FILE")"
        echo "It is also stored in $PW_FILE; delete that file after you changed the password."
    fi
    export ADMIN_PASSWORD="$(cat "$PW_FILE")"
fi

# Get public IP if not set
if [ -z "$WG_HOST" ]; then
    echo "Detecting public IP..."
    export WG_HOST=$(curl -s -4 --max-time 5 https://api.ipify.org 2>/dev/null || \
                     curl -s -4 --max-time 5 https://ifconfig.me/ip 2>/dev/null || \
                     curl -s -4 --max-time 5 https://icanhazip.com 2>/dev/null || \
                     hostname -I 2>/dev/null | awk '{print $1}')

    if [ -z "$WG_HOST" ]; then
        echo "ERROR: Could not detect public IP. Please set WG_HOST manually."
        exit 1
    fi
    echo "Detected public IP: $WG_HOST"
fi

# Get default interface (used throughout)
DEFAULT_IFACE=$(ip route show default 2>/dev/null | awk '/default/ {print $5}' | head -n1)
if [ -z "$DEFAULT_IFACE" ]; then
    DEFAULT_IFACE="eth0"
fi
echo "Using network interface: $DEFAULT_IFACE"

# Validate the VPN subnet (e.g. 10.8.1.0/24) before anything is configured
if ! [[ "$WG_NETWORK" =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})/([0-9]{1,2})$ ]] \
   || [ "${BASH_REMATCH[1]}" -gt 255 ] || [ "${BASH_REMATCH[2]}" -gt 255 ] \
   || [ "${BASH_REMATCH[3]}" -gt 255 ] || [ "${BASH_REMATCH[4]}" -gt 255 ]; then
    echo "ERROR: WG_NETWORK='$WG_NETWORK' is not a valid IPv4 network. Example: 10.8.1.0/24"
    exit 1
fi

# Parse network for server address
NETWORK_PREFIX=$(echo "$WG_NETWORK" | cut -d'/' -f1)
NETWORK_MASK=$(echo "$WG_NETWORK" | cut -d'/' -f2)
IFS='.' read -r -a octets <<< "$NETWORK_PREFIX"
SERVER_IP="${octets[0]}.${octets[1]}.${octets[2]}.$((octets[3] + 1))"

# Initialize WireGuard server if not configured
if [ ! -f /etc/wireguard/wg0.conf ]; then
    echo "Initializing WireGuard server..."

    # Generate server keys
    SERVER_PRIVATE_KEY=$(wg genkey)
    SERVER_PUBLIC_KEY=$(echo "$SERVER_PRIVATE_KEY" | wg pubkey)

    # Create WireGuard config
    cat > /etc/wireguard/wg0.conf << EOF
[Interface]
Address = ${SERVER_IP}/${NETWORK_MASK}
ListenPort = ${WG_PORT}
PrivateKey = ${SERVER_PRIVATE_KEY}
PostUp = iptables -t nat -A POSTROUTING -s ${WG_NETWORK} -o ${DEFAULT_IFACE} -j MASQUERADE; iptables -A INPUT -p udp -m udp --dport ${WG_PORT} -j ACCEPT; iptables -A FORWARD -i wg0 -j ACCEPT; iptables -A FORWARD -o wg0 -j ACCEPT
PostDown = iptables -t nat -D POSTROUTING -s ${WG_NETWORK} -o ${DEFAULT_IFACE} -j MASQUERADE; iptables -D INPUT -p udp -m udp --dport ${WG_PORT} -j ACCEPT; iptables -D FORWARD -i wg0 -j ACCEPT; iptables -D FORWARD -o wg0 -j ACCEPT
EOF

    chmod 600 /etc/wireguard/wg0.conf

    echo "WireGuard server initialized"
    echo "  Server IP: ${SERVER_IP}"
    echo "  Public Key: ${SERVER_PUBLIC_KEY}"
else
    echo "WireGuard configuration found, checking for network changes..."
    SERVER_PUBLIC_KEY=$(grep PrivateKey /etc/wireguard/wg0.conf | awk '{print $3}' | wg pubkey)
    SERVER_PRIVATE_KEY=$(grep PrivateKey /etc/wireguard/wg0.conf | awk '{print $3}')

    # Get current network from config
    CURRENT_NETWORK=$(grep -oP 'POSTROUTING -s \K[0-9./]+' /etc/wireguard/wg0.conf 2>/dev/null || echo "")
    CURRENT_ADDRESS=$(grep -oP 'Address = \K[0-9./]+' /etc/wireguard/wg0.conf 2>/dev/null || echo "")

    # Check if WG_NETWORK changed
    if [ -n "$CURRENT_NETWORK" ] && [ "$CURRENT_NETWORK" != "$WG_NETWORK" ]; then
        echo "Network changed from $CURRENT_NETWORK to $WG_NETWORK, updating configuration..."

        # Update config with new network
        cat > /etc/wireguard/wg0.conf << EOF
[Interface]
Address = ${SERVER_IP}/${NETWORK_MASK}
ListenPort = ${WG_PORT}
PrivateKey = ${SERVER_PRIVATE_KEY}
PostUp = iptables -t nat -A POSTROUTING -s ${WG_NETWORK} -o ${DEFAULT_IFACE} -j MASQUERADE; iptables -A INPUT -p udp -m udp --dport ${WG_PORT} -j ACCEPT; iptables -A FORWARD -i wg0 -j ACCEPT; iptables -A FORWARD -o wg0 -j ACCEPT
PostDown = iptables -t nat -D POSTROUTING -s ${WG_NETWORK} -o ${DEFAULT_IFACE} -j MASQUERADE; iptables -D INPUT -p udp -m udp --dport ${WG_PORT} -j ACCEPT; iptables -D FORWARD -i wg0 -j ACCEPT; iptables -D FORWARD -o wg0 -j ACCEPT
EOF

        # Preserve existing peers
        if [ -f /etc/wireguard/wg0.conf.bak ]; then
            grep -A3 '^\[Peer\]' /etc/wireguard/wg0.conf.bak >> /etc/wireguard/wg0.conf 2>/dev/null || true
        fi

        chmod 600 /etc/wireguard/wg0.conf
        echo "Configuration updated with new network: $WG_NETWORK"
    fi
fi

# Start WireGuard interface
echo "Starting WireGuard interface..."
if wg show wg0 &>/dev/null; then
    echo "Interface wg0 already running, reloading config..."
    wg syncconf wg0 <(wg-quick strip wg0)
else
    # Remove stale interface if exists
    ip link del wg0 2>/dev/null || true

    # Start WireGuard
    wg-quick up wg0 || {
        echo "ERROR: Failed to start WireGuard interface"
        exit 1
    }
fi

echo ""
echo "WireGuard Status:"
wg show wg0
echo ""

# Configuration is passed to the panel through environment variables
# (no sed rewriting of config.yaml: it broke on special characters in passwords
# and overwrote every "port:" key).
export PANEL_PORT DATABASE_PATH WG_NETWORK WG_DNS ADMIN_PASSWORD

# Export server endpoint for the Go application
# Public UDP port clients connect to (host side of the -p mapping)
WG_PUBLIC_PORT="${WG_PUBLIC_PORT:-1881}"
export WG_SERVER_ENDPOINT="${WG_HOST}:${WG_PUBLIC_PORT}"
export WG_SERVER_PUBLIC_KEY="${SERVER_PUBLIC_KEY}"

echo "=========================================="
echo "  WireGuard Panel Ready!"
echo "=========================================="
echo ""
echo "  Panel URL: http://${WG_HOST}:${PANEL_PORT}"
echo "  Username: ndo4me (fixed)"
echo "  Password: ${ADMIN_PASSWORD}"
echo ""
echo "  WireGuard Endpoint: ${WG_HOST}:${WG_PUBLIC_PORT}/udp"
echo "  Server Public Key: ${SERVER_PUBLIC_KEY}"
echo ""
echo "=========================================="

# Start the application
exec /app/wireguardpanel
