#!/bin/bash

# LLM Proxy Server Installation Script
# This script installs the LLM proxy server to /opt/llm-proxy

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Installation directory and service configuration
INSTALL_DIR="/opt/llm-proxy"
SERVICE_NAME="llm-proxy"
SERVICE_USER="llm-proxy"
SERVICE_GROUP="llm-proxy"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}Please run this script as root or with sudo${NC}"
    exit 1
fi

echo -e "${GREEN}=== LLM Proxy Server Installation ===${NC}"
echo "Installing to: ${INSTALL_DIR}"

# Create system user and group for the service
echo -e "${YELLOW}Creating system user and group...${NC}"
if ! id -u ${SERVICE_USER} >/dev/null 2>&1; then
    useradd --system --user-group --home-dir ${INSTALL_DIR} --shell /usr/sbin/nologin ${SERVICE_USER}
    echo "   ✓ Created user: ${SERVICE_USER}"
else
    echo "   ✓ User ${SERVICE_USER} already exists"
fi

# Create installation directory
echo -e "${YELLOW}Creating installation directory...${NC}"
mkdir -p "${INSTALL_DIR}"

# Copy backend file
echo -e "${YELLOW}Copying backend files...${NC}"
cp llm-proxy.js "${INSTALL_DIR}/"

# Copy web directory
echo -e "${YELLOW}Copying web files...${NC}"
cp -r web "${INSTALL_DIR}/"

# Copy other necessary files
echo -e "${YELLOW}Copying documentation files...${NC}"
cp README.md "${INSTALL_DIR}/" 2>/dev/null || true
cp CLAUDE.md "${INSTALL_DIR}/" 2>/dev/null || true

# Create logs directory
echo -e "${YELLOW}Creating logs directory...${NC}"
mkdir -p "${INSTALL_DIR}/logs"

# Copy systemd service file
echo -e "${YELLOW}Installing systemd service...${NC}"
cat > "${SERVICE_FILE}" << EOF
[Unit]
Description=LLM Proxy Server and MCP Web Client
After=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=/opt/llm-proxy
ExecStart=/usr/bin/node /opt/llm-proxy/llm-proxy.js
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/llm-proxy/logs
ReadOnlyPaths=/opt/llm-proxy

# Environment
Environment="NODE_ENV=production"

[Install]
WantedBy=multi-user.target
EOF

# Set permissions
echo -e "${YELLOW}Setting permissions...${NC}"
# Set ownership for all files and directories
chown -R ${SERVICE_USER}:${SERVICE_GROUP} "${INSTALL_DIR}"

# Directory permissions: 755 (rwxr-xr-x)
chmod 755 "${INSTALL_DIR}"
chmod 755 "${INSTALL_DIR}/web"
chmod 755 "${INSTALL_DIR}/logs"

# File permissions: 644 (rw-r--r--) for regular files, 755 for executables
find "${INSTALL_DIR}" -type f -exec chmod 644 {} \;
chmod 755 "${INSTALL_DIR}/llm-proxy.js"  # Make the main script executable

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    echo -e "${RED}Node.js is not installed!${NC}"
    echo "Please install Node.js before running the service:"
    echo "  Ubuntu/Debian: sudo apt-get install nodejs"
    echo "  RHEL/CentOS: sudo yum install nodejs"
    echo "  Arch: sudo pacman -S nodejs"
else
    NODE_VERSION=$(node --version)
    echo -e "${GREEN}Node.js ${NODE_VERSION} detected${NC}"
fi

# Reload systemd
echo -e "${YELLOW}Reloading systemd...${NC}"
systemctl daemon-reload

echo -e "${GREEN}=== Installation Complete ===${NC}"
echo ""
echo "Service installed with:"
echo "  • User: ${SERVICE_USER}"
echo "  • Group: ${SERVICE_GROUP}"
echo "  • Home: ${INSTALL_DIR}"
echo ""
echo "Next steps:"
echo "1. Create a configuration file at: ${INSTALL_DIR}/llm-proxy-config.json"
echo "   (The service will create a template on first run)"
echo ""
echo "2. Start the service:"
echo "   sudo systemctl start ${SERVICE_NAME}"
echo ""
echo "3. Enable auto-start on boot:"
echo "   sudo systemctl enable ${SERVICE_NAME}"
echo ""
echo "4. Check service status:"
echo "   sudo systemctl status ${SERVICE_NAME}"
echo ""
echo "5. View logs:"
echo "   sudo journalctl -u ${SERVICE_NAME} -f"
echo ""
echo "The web interface will be available at: http://localhost:8081"