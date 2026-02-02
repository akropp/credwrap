#!/bin/bash
# credwrap macOS setup script
# Sets up credwrap as a launchd service
#
# Usage: ./setup-macos.sh [options]
#
# Options:
#   --port PORT       Listen port (default: 9876)
#   --user-service    Install as user service (~/Library/LaunchAgents)
#   --system-service  Install as system service (/Library/LaunchDaemons) - requires sudo

set -e

# Defaults
LISTEN_PORT="9876"
SERVICE_TYPE="user"  # or "system"
INSTALL_DIR="/usr/local/bin"

# Paths depend on service type
USER_CONFIG_DIR="$HOME/.config/credwrap"
USER_LOG_DIR="$HOME/.local/log/credwrap"
SYSTEM_CONFIG_DIR="/etc/credwrap"
SYSTEM_LOG_DIR="/var/log/credwrap"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --port) LISTEN_PORT="$2"; shift 2 ;;
        --user-service) SERVICE_TYPE="user"; shift ;;
        --system-service) SERVICE_TYPE="system"; shift ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --port PORT       Listen port (default: 9876)"
            echo "  --user-service    Install as user service (default)"
            echo "  --system-service  Install as system service (requires sudo)"
            exit 0
            ;;
        *) log_error "Unknown option: $1"; exit 1 ;;
    esac
done

# Set paths based on service type
if [[ "$SERVICE_TYPE" == "system" ]]; then
    if [[ $EUID -ne 0 ]]; then
        log_error "System service installation requires sudo"
        exit 1
    fi
    CONFIG_DIR="$SYSTEM_CONFIG_DIR"
    LOG_DIR="$SYSTEM_LOG_DIR"
    PLIST_DIR="/Library/LaunchDaemons"
    PLIST_NAME="com.credwrap.server.plist"
else
    CONFIG_DIR="$USER_CONFIG_DIR"
    LOG_DIR="$USER_LOG_DIR"
    PLIST_DIR="$HOME/Library/LaunchAgents"
    PLIST_NAME="com.credwrap.server.plist"
fi

log_info "Setting up credwrap for macOS..."
log_info "  Service type: $SERVICE_TYPE"
log_info "  Config: $CONFIG_DIR"
log_info "  Port: $LISTEN_PORT"

# Create directories
log_info "Creating directories..."
mkdir -p "$CONFIG_DIR" "$LOG_DIR" "$PLIST_DIR"

# Download or locate binaries
if [[ ! -f "$INSTALL_DIR/credwrap-server" ]]; then
    log_info "Downloading credwrap binaries..."
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        arm64) ARCH="arm64" ;;
        *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    
    RELEASE_URL="https://github.com/akropp/credwrap/releases/latest/download"
    
    if [[ "$SERVICE_TYPE" == "system" ]]; then
        curl -sL "$RELEASE_URL/credwrap-darwin-${ARCH}.tar.gz" | tar xz -C "$INSTALL_DIR"
        curl -sL "$RELEASE_URL/credwrap-server-darwin-${ARCH}.tar.gz" | tar xz -C "$INSTALL_DIR"
    else
        mkdir -p "$HOME/.local/bin"
        curl -sL "$RELEASE_URL/credwrap-darwin-${ARCH}.tar.gz" | tar xz -C "$HOME/.local/bin"
        curl -sL "$RELEASE_URL/credwrap-server-darwin-${ARCH}.tar.gz" | tar xz -C "$HOME/.local/bin"
        INSTALL_DIR="$HOME/.local/bin"
    fi
fi

# Create default config if needed
if [[ ! -f "$CONFIG_DIR/config.yaml" ]]; then
    log_info "Creating default config..."
    cat > "$CONFIG_DIR/config.yaml" << EOF
server:
  listen: "127.0.0.1:${LISTEN_PORT}"
  audit: "${LOG_DIR}/audit.log"

auth:
  tokens:
    - "$(openssl rand -hex 16)"
  allowed_ips:
    - "127.0.0.1"
  require_token: true

tools:
  echo:
    path: /bin/echo
    credentials: []
    pass_args: true
EOF
fi

# Create empty encrypted credentials if needed
if [[ ! -f "$CONFIG_DIR/credentials.enc" ]]; then
    log_info "Initialize credentials with:"
    log_info "  $INSTALL_DIR/credwrap-server secrets init $CONFIG_DIR/credentials.enc"
fi

# Create launchd plist
log_info "Creating launchd service..."
cat > "$PLIST_DIR/$PLIST_NAME" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.credwrap.server</string>
    
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/credwrap-server</string>
        <string>--config</string>
        <string>${CONFIG_DIR}/config.yaml</string>
        <string>--credentials</string>
        <string>${CONFIG_DIR}/credentials.enc</string>
        <string>--encrypted</string>
        <string>--keyfile</string>
        <string>${CONFIG_DIR}/keyfile</string>
    </array>
    
    <key>RunAtLoad</key>
    <true/>
    
    <key>KeepAlive</key>
    <true/>
    
    <key>StandardOutPath</key>
    <string>${LOG_DIR}/stdout.log</string>
    
    <key>StandardErrorPath</key>
    <string>${LOG_DIR}/stderr.log</string>
</dict>
</plist>
EOF

# Print summary
echo ""
echo "=============================================="
echo -e "${GREEN}credwrap macOS setup complete!${NC}"
echo "=============================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Initialize encrypted credentials:"
echo "   $INSTALL_DIR/credwrap-server secrets init $CONFIG_DIR/credentials.enc"
echo ""
echo "2. Add your secrets:"
echo "   $INSTALL_DIR/credwrap-server secrets add $CONFIG_DIR/credentials.enc gog-password"
echo ""
echo "3. Create a keyfile for unattended startup:"
echo "   # Enter your encryption password into this file"
echo "   nano $CONFIG_DIR/keyfile"
echo "   chmod 600 $CONFIG_DIR/keyfile"
echo ""
echo "4. Load the service:"
if [[ "$SERVICE_TYPE" == "system" ]]; then
    echo "   sudo launchctl load $PLIST_DIR/$PLIST_NAME"
else
    echo "   launchctl load $PLIST_DIR/$PLIST_NAME"
fi
echo ""
echo "5. Check status:"
if [[ "$SERVICE_TYPE" == "system" ]]; then
    echo "   sudo launchctl list | grep credwrap"
else
    echo "   launchctl list | grep credwrap"
fi
echo ""
echo "Commands:"
if [[ "$SERVICE_TYPE" == "system" ]]; then
    echo "  Start:   sudo launchctl start com.credwrap.server"
    echo "  Stop:    sudo launchctl stop com.credwrap.server"
    echo "  Unload:  sudo launchctl unload $PLIST_DIR/$PLIST_NAME"
else
    echo "  Start:   launchctl start com.credwrap.server"
    echo "  Stop:    launchctl stop com.credwrap.server"
    echo "  Unload:  launchctl unload $PLIST_DIR/$PLIST_NAME"
fi
echo ""
