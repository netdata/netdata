#!/usr/bin/env bash

set -euo pipefail

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    echo "ERROR: This script must be run with sudo"
    echo "Usage: sudo $0"
    exit 1
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Configuration
NEDA_HOME="/opt/neda"

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Execute command with visibility
run() {
    # Print the command being executed
    printf >&2 "${GRAY}$(pwd) >${NC} "
    printf >&2 "${YELLOW}"
    printf >&2 "%q " "$@"
    printf >&2 "${NC}\n"
    
    # Execute the command
    if ! "$@"; then
        local exit_code=$?
        echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${exit_code}: ${YELLOW}$1${NC}"
        echo -e >&2 "${RED}        Full command:${NC} $*"
        echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
        echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        return $exit_code
    fi
}

log_info "Installing/updating Neda systemd services..."

# Check if neda is installed
if [ ! -d "$NEDA_HOME" ]; then
    log_error "Neda is not installed at $NEDA_HOME"
    log_error "Please run neda-setup.sh first"
    exit 1
fi

# Check if service files exist in neda home
if [ ! -f "$NEDA_HOME/neda.service" ]; then
    log_error "Service files not found in $NEDA_HOME"
    log_error "Please ensure neda-setup.sh has been run"
    exit 1
fi

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Installing main Neda service..."

# Stop services if they're running
if systemctl is-active --quiet neda; then
    log_info "Stopping existing neda service..."
    run systemctl stop neda
fi

# Copy service files to systemd directory
log_info "Copying service files to /etc/systemd/system/..."
run cp "$NEDA_HOME/neda.service" /etc/systemd/system/

# Reload systemd daemon
log_info "Reloading systemd daemon..."
run systemctl daemon-reload

# Enable and start main service
log_info "Enabling neda service..."
run systemctl enable neda

log_info "Starting neda service..."
run systemctl start neda

# Check service status
if systemctl is-active --quiet neda; then
    log_info "✓ Neda service is running"
else
    log_error "Neda service failed to start"
    log_info "Check logs with: journalctl --namespace=neda -u neda -xe"
    exit 1
fi

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Installing repository sync service and timer..."

# Check if sync script exists
if [ -f "$NEDA_HOME/bin/sync-netdata-repos.sh" ]; then
    # Stop timer if running
    if systemctl is-active --quiet neda-repos-sync.timer; then
        log_info "Stopping existing sync timer..."
        run systemctl stop neda-repos-sync.timer
    fi
    
    # Copy service and timer files
    run cp "$NEDA_HOME/neda-repos-sync.service" /etc/systemd/system/
    run cp "$NEDA_HOME/neda-repos-sync.timer" /etc/systemd/system/
    
    # Reload daemon
    run systemctl daemon-reload
    
    # Enable and start timer
    log_info "Enabling repository sync timer..."
    run systemctl enable neda-repos-sync.timer
    
    log_info "Starting repository sync timer..."
    run systemctl start neda-repos-sync.timer
    
    # Check timer status
    if systemctl is-active --quiet neda-repos-sync.timer; then
        log_info "✓ Repository sync timer is active"
        log_info "Next sync scheduled for:"
        systemctl status neda-repos-sync.timer --no-pager | grep "Trigger:" || true
    else
        log_warn "Repository sync timer failed to start"
    fi
else
    log_warn "Repository sync script not found, skipping sync service installation"
fi

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Service installation completed!"
log_info ""
log_info "Service status:"
systemctl status neda --no-pager --lines=0 || true
echo ""
systemctl list-timers neda-repos-sync.timer --no-pager || true
log_info ""
log_info "Useful commands:"
log_info "  View main service logs:     journalctl --namespace=neda -u neda -f"
log_info "  View sync service logs:     journalctl --namespace=neda -u neda-repos-sync -f"
log_info "  Restart main service:       sudo systemctl restart neda"
log_info "  Stop main service:          sudo systemctl stop neda"
log_info "  Trigger manual sync:        sudo systemctl start neda-repos-sync.service"
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"