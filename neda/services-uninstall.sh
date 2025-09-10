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

log_info "Uninstalling Neda systemd services..."

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Removing main Neda service..."

# Stop main service if running
if systemctl is-active --quiet neda; then
    log_info "Stopping neda service..."
    run systemctl stop neda
else
    log_info "Neda service is not running"
fi

# Disable service if enabled
if systemctl is-enabled --quiet neda 2>/dev/null; then
    log_info "Disabling neda service..."
    run systemctl disable neda
else
    log_info "Neda service is not enabled"
fi

# Remove service file
if [ -f /etc/systemd/system/neda.service ]; then
    log_info "Removing neda.service..."
    run rm /etc/systemd/system/neda.service
else
    log_info "neda.service not found in systemd"
fi

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Removing repository sync service and timer..."

# Stop timer if running
if systemctl is-active --quiet neda-repos-sync.timer; then
    log_info "Stopping repository sync timer..."
    run systemctl stop neda-repos-sync.timer
else
    log_info "Repository sync timer is not running"
fi

# Stop service if running (unlikely for oneshot, but check anyway)
if systemctl is-active --quiet neda-repos-sync.service; then
    log_info "Stopping repository sync service..."
    run systemctl stop neda-repos-sync.service
fi

# Disable timer if enabled
if systemctl is-enabled --quiet neda-repos-sync.timer 2>/dev/null; then
    log_info "Disabling repository sync timer..."
    run systemctl disable neda-repos-sync.timer
else
    log_info "Repository sync timer is not enabled"
fi

# Remove service and timer files
if [ -f /etc/systemd/system/neda-repos-sync.service ]; then
    log_info "Removing neda-repos-sync.service..."
    run rm /etc/systemd/system/neda-repos-sync.service
else
    log_info "neda-repos-sync.service not found in systemd"
fi

if [ -f /etc/systemd/system/neda-repos-sync.timer ]; then
    log_info "Removing neda-repos-sync.timer..."
    run rm /etc/systemd/system/neda-repos-sync.timer
else
    log_info "neda-repos-sync.timer not found in systemd"
fi

# Reload systemd daemon
log_info "Reloading systemd daemon..."
run systemctl daemon-reload

# Reset failed units if any
if systemctl is-failed --quiet neda 2>/dev/null; then
    log_info "Resetting failed state for neda..."
    run systemctl reset-failed neda
fi

if systemctl is-failed --quiet neda-repos-sync.service 2>/dev/null; then
    log_info "Resetting failed state for neda-repos-sync.service..."
    run systemctl reset-failed neda-repos-sync.service
fi

if systemctl is-failed --quiet neda-repos-sync.timer 2>/dev/null; then
    log_info "Resetting failed state for neda-repos-sync.timer..."
    run systemctl reset-failed neda-repos-sync.timer
fi

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Service uninstallation completed!"
log_info ""
log_info "The Neda installation at /opt/neda has NOT been removed."
log_info "To completely remove Neda, you would need to:"
log_info "  1. Remove the neda user: sudo userdel -r neda"
log_info "  2. Remove the installation: sudo rm -rf /opt/neda"
log_info ""
log_info "Service files have been removed from systemd."
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"