#!/bin/bash
# credwrap server setup script
# Run as root or with sudo
#
# Usage: sudo ./setup-server.sh [options]
#
# Options:
#   --user NAME       Service user (default: credwrap)
#   --port PORT       Listen port (default: 9876)
#   --bind ADDR       Bind address (default: 127.0.0.1)
#   --no-systemd      Skip systemd service setup

set -e

# Defaults
SERVICE_USER="credwrap"
LISTEN_PORT="9876"
BIND_ADDR="127.0.0.1"
SETUP_SYSTEMD="yes"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/credwrap"
DATA_DIR="/var/lib/credwrap"
LOG_DIR="/var/log/credwrap"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --user) SERVICE_USER="$2"; shift 2 ;;
        --port) LISTEN_PORT="$2"; shift 2 ;;
        --bind) BIND_ADDR="$2"; shift 2 ;;
        --no-systemd) SETUP_SYSTEMD="no"; shift ;;
        -h|--help)
            echo "Usage: sudo $0 [options]"
            echo ""
            echo "Options:"
            echo "  --user NAME       Service user (default: credwrap)"
            echo "  --port PORT       Listen port (default: 9876)"
            echo "  --bind ADDR       Bind address (default: 127.0.0.1)"
            echo "  --no-systemd      Skip systemd service setup"
            exit 0
            ;;
        *) log_error "Unknown option: $1"; exit 1 ;;
    esac
done

# Check root
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root (use sudo)"
    exit 1
fi

log_info "Setting up credwrap server..."
log_info "  User: $SERVICE_USER"
log_info "  Bind: $BIND_ADDR:$LISTEN_PORT"

# 1. Create service user
if id "$SERVICE_USER" &>/dev/null; then
    log_info "User '$SERVICE_USER' already exists"
else
    log_info "Creating user '$SERVICE_USER'..."
    useradd --system --shell /usr/sbin/nologin --home-dir "$DATA_DIR" --create-home "$SERVICE_USER"
fi

# 2. Create directories
log_info "Creating directories..."
mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
chown "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR" "$LOG_DIR"
chmod 750 "$CONFIG_DIR" "$DATA_DIR"
chmod 755 "$LOG_DIR"

# 3. Download or copy binaries
log_info "Installing binaries..."

# Check if we have local binaries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "$SCRIPT_DIR/bin/credwrap-server" ]]; then
    log_info "Using local binaries from $SCRIPT_DIR/bin/"
    cp "$SCRIPT_DIR/bin/credwrap" "$INSTALL_DIR/"
    cp "$SCRIPT_DIR/bin/credwrap-server" "$INSTALL_DIR/"
else
    # Download latest release
    log_info "Downloading latest release..."
    ARCH=$(uname -m)
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    
    RELEASE_URL="https://github.com/akropp/credwrap/releases/latest/download"
    
    curl -sL "$RELEASE_URL/credwrap-${OS}-${ARCH}.tar.gz" | tar xz -C "$INSTALL_DIR"
    curl -sL "$RELEASE_URL/credwrap-server-${OS}-${ARCH}.tar.gz" | tar xz -C "$INSTALL_DIR"
fi

chmod 755 "$INSTALL_DIR/credwrap" "$INSTALL_DIR/credwrap-server"
log_info "Installed: $INSTALL_DIR/credwrap, $INSTALL_DIR/credwrap-server"

# 4. Create default config
if [[ ! -f "$CONFIG_DIR/config.yaml" ]]; then
    log_info "Creating default config..."
    cat > "$CONFIG_DIR/config.yaml" << EOF
# credwrap server configuration
# Edit this file to add your tools

server:
  listen: "${BIND_ADDR}:${LISTEN_PORT}"
  audit: "${LOG_DIR}/audit.log"

auth:
  tokens:
    - "CHANGE-ME-$(openssl rand -hex 16)"
  allowed_ips:
    - "127.0.0.1"
  require_token: true

tools:
  # Example: Google Workspace CLI
  # gog:
  #   path: /usr/local/bin/gog
  #   credentials:
  #     - env: GOG_KEYRING_PASSWORD
  #       secret: gog-password
  #   pass_args: true

  # Basic echo for testing
  echo:
    path: /bin/echo
    credentials: []
    pass_args: true
EOF
    chown root:$SERVICE_USER "$CONFIG_DIR/config.yaml"
    chmod 640 "$CONFIG_DIR/config.yaml"
fi

# 5. Create empty credentials file
if [[ ! -f "$CONFIG_DIR/credentials.yaml" ]]; then
    log_info "Creating empty credentials file..."
    cat > "$CONFIG_DIR/credentials.yaml" << EOF
# Add your credentials here
# Example:
# gog-password: "your-password-here"
#
# Then encrypt with:
#   age -p -o credentials.enc credentials.yaml
#   rm credentials.yaml
#   # Update systemd to use --encrypted flag
EOF
    chown root:$SERVICE_USER "$CONFIG_DIR/credentials.yaml"
    chmod 640 "$CONFIG_DIR/credentials.yaml"
fi

# 6. Set up systemd service
if [[ "$SETUP_SYSTEMD" == "yes" ]]; then
    log_info "Creating systemd service..."
    cat > /etc/systemd/system/credwrap.service << EOF
[Unit]
Description=credwrap credential injection server
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
ExecStart=$INSTALL_DIR/credwrap-server \\
    --config $CONFIG_DIR/config.yaml \\
    --credentials $CONFIG_DIR/credentials.yaml
Restart=on-failure
RestartSec=5

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$LOG_DIR
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    log_info "Systemd service created: credwrap.service"
fi

# 7. Print summary
echo ""
echo "=============================================="
echo -e "${GREEN}credwrap server setup complete!${NC}"
echo "=============================================="
echo ""
echo "Configuration:"
echo "  Config:      $CONFIG_DIR/config.yaml"
echo "  Credentials: $CONFIG_DIR/credentials.yaml"
echo "  Audit log:   $LOG_DIR/audit.log"
echo ""
echo "Next steps:"
echo ""
echo "1. Edit the config to add your tools:"
echo "   sudo nano $CONFIG_DIR/config.yaml"
echo ""
echo "2. Add your credentials:"
echo "   sudo nano $CONFIG_DIR/credentials.yaml"
echo ""
echo "3. (Optional) Encrypt credentials:"
echo "   cd $CONFIG_DIR"
echo "   sudo age -p -o credentials.enc credentials.yaml"
echo "   sudo rm credentials.yaml"
echo "   sudo chown root:$SERVICE_USER credentials.enc"
echo "   sudo chmod 640 credentials.enc"
echo "   # Then edit credwrap.service to add --encrypted flag"
echo ""
echo "4. Start the service:"
echo "   sudo systemctl start credwrap"
echo "   sudo systemctl enable credwrap  # auto-start on boot"
echo ""
echo "5. Configure client (~/.credwrap.yaml):"
echo "   server: \"${BIND_ADDR}:${LISTEN_PORT}\""
echo "   token: \"<token from config.yaml>\""
echo ""
echo "6. Test:"
echo "   credwrap echo 'Hello from credwrap!'"
echo ""
