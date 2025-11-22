#!/bin/bash
# WGEasy Go Secure Edition - Installation Script
# Run as root: sudo bash install.sh

set -e

APP_USER="wgeasygo"
APP_DIR="/opt/wgeasygo"

echo "=== WGEasy Go Secure Edition Installation ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Error: Please run as root (sudo bash install.sh)"
    exit 1
fi

# Create service user
if ! id "$APP_USER" &>/dev/null; then
    echo "Creating service user: $APP_USER"
    useradd --system --no-create-home --shell /sbin/nologin "$APP_USER"
fi

# Create application directory
echo "Creating application directory..."
mkdir -p "$APP_DIR"/{data,configs}
chown -R "$APP_USER:$APP_USER" "$APP_DIR"
chmod 750 "$APP_DIR"

# Copy binary (assumes it's built and in current directory)
if [ -f "wgeasygo" ]; then
    echo "Installing binary..."
    cp wgeasygo "$APP_DIR/"
    chown "$APP_USER:$APP_USER" "$APP_DIR/wgeasygo"
    chmod 750 "$APP_DIR/wgeasygo"
fi

# Copy configuration
if [ -f "configs/config.yaml" ]; then
    echo "Installing configuration..."
    cp configs/config.yaml "$APP_DIR/configs/"
    chown "$APP_USER:$APP_USER" "$APP_DIR/configs/config.yaml"
    chmod 640 "$APP_DIR/configs/config.yaml"
fi

# Setup environment file
if [ ! -f "$APP_DIR/.env" ]; then
    echo "Creating environment file..."
    cp deployments/.env.example "$APP_DIR/.env"
    chown "$APP_USER:$APP_USER" "$APP_DIR/.env"
    chmod 600 "$APP_DIR/.env"
    echo "IMPORTANT: Edit $APP_DIR/.env with your secure values!"
fi

# Install systemd service
echo "Installing systemd service..."
cp deployments/wgeasygo.service /etc/systemd/system/
systemctl daemon-reload

# Install sudoers rules
echo "Installing sudoers rules..."
cp deployments/sudoers.wgeasygo /etc/sudoers.d/wgeasygo
chmod 440 /etc/sudoers.d/wgeasygo
visudo -c

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "1. Edit $APP_DIR/.env with secure JWT secrets and admin password"
echo "2. Configure WireGuard server and update WG_SERVER_ENDPOINT and WG_SERVER_PUBLIC_KEY"
echo "3. Start the service: systemctl enable --now wgeasygo"
echo "4. Check status: systemctl status wgeasygo"
echo "5. View logs: journalctl -u wgeasygo -f"
echo ""
echo "SECURITY REMINDERS:"
echo "- Change default admin password immediately after first login"
echo "- Set up HTTPS using nginx reverse proxy"
echo "- Configure firewall to only allow necessary ports"
