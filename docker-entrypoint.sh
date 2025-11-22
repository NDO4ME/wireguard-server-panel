#!/bin/bash
set -e

echo "=========================================="
echo "  WireGuard Panel - Starting..."
echo "=========================================="

# IP forwarding is set via docker run sysctls
# Just verify it's enabled
if [ "$(cat /proc/sys/net/ipv4/ip_forward 2>/dev/null)" != "1" ]; then
    echo "WARNING: IP forwarding is not enabled. VPN may not work correctly."
    echo "Make sure to run with --sysctl net.ipv4.ip_forward=1"
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

# Start tailscaled if available
if command -v tailscaled &> /dev/null; then
    echo "Starting Tailscale daemon..."
    mkdir -p /var/run/tailscale /var/lib/tailscale
    tailscaled --state=/data/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock &
    sleep 2
fi

# Static admin username - cannot be changed
export ADMIN_USERNAME="layerweb"

# Always generate random JWT secret for security
export JWT_SECRET=$(head -c 64 /dev/urandom | base64 | tr -d '\n=' | head -c 64)
echo "Generated secure random JWT secret"

# Set default admin password if not provided
if [ -z "$ADMIN_PASSWORD" ]; then
    export ADMIN_PASSWORD="admin"
    echo "WARNING: Using default admin password. Set ADMIN_PASSWORD env var for security."
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

# Update config file with environment variables
CONFIG_FILE="/app/configs/config.yaml"
if [ -f "$CONFIG_FILE" ]; then
    sed -i "s|port:.*|port: ${PANEL_PORT}|" "$CONFIG_FILE"
    sed -i "s|access_secret:.*|access_secret: \"${JWT_SECRET}\"|" "$CONFIG_FILE"
    sed -i "s|refresh_secret:.*|refresh_secret: \"${JWT_SECRET}-refresh\"|" "$CONFIG_FILE"
    sed -i "s|username:.*|username: \"${ADMIN_USERNAME}\"|" "$CONFIG_FILE"
    sed -i "s|password:.*|password: \"${ADMIN_PASSWORD}\"|" "$CONFIG_FILE"
    sed -i "s|path:.*data.*|path: \"${DATABASE_PATH}\"|" "$CONFIG_FILE"
    sed -i "s|subnet:.*|subnet: \"${WG_NETWORK}\"|" "$CONFIG_FILE"
    sed -i "s|dns:.*|dns: \"${WG_DNS}\"|" "$CONFIG_FILE"
fi

# Export server endpoint for the Go application
export WG_SERVER_ENDPOINT="${WG_HOST}:1881"
export WG_SERVER_PUBLIC_KEY="${SERVER_PUBLIC_KEY}"

echo "=========================================="
echo "  WireGuard Panel Ready!"
echo "=========================================="
echo ""
echo "  Panel URL: http://${WG_HOST}:${PANEL_PORT}"
echo "  Username: layerweb (fixed)"
echo "  Password: ${ADMIN_PASSWORD}"
echo ""
echo "  WireGuard Endpoint: ${WG_HOST}:1881/udp"
echo "  Server Public Key: ${SERVER_PUBLIC_KEY}"
echo ""
echo "=========================================="

# Start the application
exec /app/wireguardpanel
