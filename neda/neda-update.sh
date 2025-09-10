#!/usr/bin/env bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

NEDA_HOME="/opt/neda"
NEDA_USER="neda"

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}ERROR:${NC} This script must be run with sudo"
    echo "Usage: sudo $0 [commit message]"
    exit 1
fi

# Check if neda is installed
if [ ! -d "$NEDA_HOME/.git" ]; then
    echo -e "${RED}ERROR:${NC} Git repository not found in $NEDA_HOME"
    echo "Please run neda-setup.sh first"
    exit 1
fi

# Get commit message
if [ $# -eq 0 ]; then
    COMMIT_MSG="Configuration update $(date '+%Y-%m-%d %H:%M:%S')"
else
    COMMIT_MSG="$*"
fi

echo -e "${GREEN}[INFO]${NC} Creating backup commit in $NEDA_HOME"

# Check for changes
if sudo -u "$NEDA_USER" git -C "$NEDA_HOME" diff --quiet && \
   sudo -u "$NEDA_USER" git -C "$NEDA_HOME" diff --cached --quiet; then
    echo -e "${YELLOW}[WARN]${NC} No changes to commit"
    exit 0
fi

# Show what will be committed
echo -e "${GREEN}[INFO]${NC} Changes to be committed:"
sudo -u "$NEDA_USER" git -C "$NEDA_HOME" status --short

# Add all changes
sudo -u "$NEDA_USER" git -C "$NEDA_HOME" add -A

# Create commit
sudo -u "$NEDA_USER" git -C "$NEDA_HOME" commit -m "$COMMIT_MSG"

echo -e "${GREEN}[INFO]${NC} Backup commit created successfully"
echo -e "${GREEN}[INFO]${NC} To view history: sudo -u neda git -C $NEDA_HOME log --oneline"
echo -e "${GREEN}[INFO]${NC} To rollback: sudo -u neda git -C $NEDA_HOME reset --hard HEAD~1"