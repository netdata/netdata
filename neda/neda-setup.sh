#!/usr/bin/env bash

set -euo pipefail

# Check if running as root FIRST, before anything else
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
NEDA_USER="neda"
NEDA_GROUP="neda"
AI_AGENT_DIR="/opt/ai-agent"
NETDATA_REPOS_DIR="$NEDA_HOME/netdata-repos"
GCLOUD_HOME="$NEDA_HOME/google-cloud-sdk"

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

# Detect OS and package manager
detect_os() {
    if [ -f /etc/debian_version ]; then
        OS_FAMILY="debian"
    elif [ -f /etc/redhat-release ]; then
        OS_FAMILY="redhat"
    elif [ -f /etc/arch-release ]; then
        OS_FAMILY="arch"
    else
        OS_FAMILY="unknown"
    fi
}

# Generic package installer
install_pkg() {
    local pkg_debian="$1"
    local pkg_redhat="${2:-$1}"
    local pkg_arch="${3:-$1}"

    case "$OS_FAMILY" in
        debian)
            run apt-get install -y "$pkg_debian"
            ;;
        redhat)
            run dnf install -y "$pkg_redhat" || run yum install -y "$pkg_redhat"
            ;;
        arch)
            run pacman -S --needed --noconfirm "$pkg_arch"
            ;;
        *)
            log_error "Unsupported OS. Please install $pkg_debian manually."
            exit 1
            ;;
    esac
}

# Detect OS early
detect_os
log_info "Detected OS family: $OS_FAMILY"

# ============================================================================
# PREREQUISITES - Batch install all required packages
# ============================================================================

log_info "Checking prerequisites..."

# Arrays to collect missing packages (distro-specific names)
MISSING_PKGS=()

# Check curl
if ! command -v curl &>/dev/null; then
    log_info "  - curl: missing"
    MISSING_PKGS+=(curl)
else
    log_info "  - curl: found"
fi

# Check git
if ! command -v git &>/dev/null; then
    log_info "  - git: missing"
    MISSING_PKGS+=(git)
else
    log_info "  - git: found"
fi

# Check python3
if ! command -v python3 &>/dev/null; then
    log_info "  - python3: missing"
    case "$OS_FAMILY" in
        debian) MISSING_PKGS+=(python3) ;;
        redhat) MISSING_PKGS+=(python3) ;;
        arch)   MISSING_PKGS+=(python) ;;
    esac
else
    log_info "  - python3: found"
fi

# Check python3-venv (Debian-specific, included in python3 on others)
if [ "$OS_FAMILY" = "debian" ]; then
    if command -v python3 &>/dev/null; then
        if ! python3 -m venv --help &>/dev/null 2>&1; then
            log_info "  - python3-venv: missing"
            MISSING_PKGS+=(python3-venv)
        else
            log_info "  - python3-venv: found"
        fi
    else
        # python3 not installed yet, venv will be missing
        log_info "  - python3-venv: missing (python3 not installed)"
        MISSING_PKGS+=(python3-venv)
    fi
fi

# Check ripgrep
if ! command -v rg &>/dev/null; then
    log_info "  - ripgrep: missing"
    MISSING_PKGS+=(ripgrep)
else
    log_info "  - ripgrep: found"
fi

# Check pipx
if ! command -v pipx &>/dev/null; then
    log_info "  - pipx: missing"
    case "$OS_FAMILY" in
        debian) MISSING_PKGS+=(pipx) ;;
        redhat) MISSING_PKGS+=(pipx) ;;
        arch)   MISSING_PKGS+=(python-pipx) ;;
    esac
else
    log_info "  - pipx: found"
fi

# Install all missing prerequisites in one command
if [ ${#MISSING_PKGS[@]} -gt 0 ]; then
    log_info "Installing missing prerequisites: ${MISSING_PKGS[*]}"
    case "$OS_FAMILY" in
        debian)
            run apt-get update
            run apt-get install -y "${MISSING_PKGS[@]}"
            ;;
        redhat)
            run dnf install -y "${MISSING_PKGS[@]}" || run yum install -y "${MISSING_PKGS[@]}"
            ;;
        arch)
            run pacman -S --needed --noconfirm "${MISSING_PKGS[@]}"
            ;;
        *)
            log_error "Unsupported OS family: $OS_FAMILY"
            log_error "Please install manually: ${MISSING_PKGS[*]}"
            exit 1
            ;;
    esac
    log_info "Prerequisites installed successfully"
else
    log_info "All prerequisites already installed"
fi

# Resolve script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

log_info "Starting Neda installation and setup..."

# ============================================================================
# CHECK AI-AGENT INSTALLATION
# ============================================================================

if [ ! -d "$AI_AGENT_DIR" ] || [ ! -f "$AI_AGENT_DIR/ai-agent" ]; then
    log_error "AI Agent not installed at $AI_AGENT_DIR"
    log_error "Please run build-and-install.sh first"
    exit 1
fi

# ============================================================================
# CREATE NEDA USER
# ============================================================================

log_info "Setting up neda user and group..."

if ! id "$NEDA_USER" &>/dev/null; then
    log_info "Creating system user '$NEDA_USER'..."
    run useradd -r -s /bin/false -d "$NEDA_HOME" -m "$NEDA_USER"
else
    log_info "User '$NEDA_USER' already exists"
fi

# ============================================================================
# CREATE DIRECTORY STRUCTURE
# ============================================================================

log_info "Creating Neda directory structure..."

run mkdir -p "$NEDA_HOME"
run mkdir -p "$NEDA_HOME"/{bin,logs,cache,tmp,mcp}
run mkdir -p "$NETDATA_REPOS_DIR"

# Set ownership immediately after creating directories
run chown -R "$NEDA_USER:$NEDA_GROUP" "$NEDA_HOME"

# ============================================================================
# COPY ALL NEDA FILES
# ============================================================================

# If git repo exists, check for and commit any manual changes before updating
if [ -d "$NEDA_HOME/.git" ]; then
    # Check if there are any uncommitted changes
    if ! sudo -u "$NEDA_USER" git -C "$NEDA_HOME" diff --quiet || \
       ! sudo -u "$NEDA_USER" git -C "$NEDA_HOME" diff --cached --quiet || \
       [ -n "$(sudo -u "$NEDA_USER" git -C "$NEDA_HOME" ls-files --others --exclude-standard)" ]; then
        log_info "Saving manual changes before update..."
        run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" add -A
        run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" commit -m "Manual changes before update $(date +%Y-%m-%d_%H:%M:%S)"
        log_info "Manual changes committed. You can rollback with: git -C $NEDA_HOME checkout HEAD~1"
    fi
fi

log_info "Copying entire neda directory contents..."

# Copy everything from neda/ to /opt/neda/
# This includes all .ai files, .md files, configs, etc.
run cp -r neda/. "$NEDA_HOME/"

# Replace the source .gitignore with the deployment version
if [ -f "neda/.gitignore.deployment" ]; then
    log_info "Installing deployment .gitignore..."
    run cp "neda/.gitignore.deployment" "$NEDA_HOME/.gitignore"
fi

# Make all .ai files executable
log_info "Setting executable permissions on agent files..."
run find "$NEDA_HOME" -name "*.ai" -exec chmod 755 {} \;

# Set secure permissions on .ai-agent.env if it exists
if [ -f "$NEDA_HOME/.ai-agent.env" ]; then
    log_info "Securing .ai-agent.env..."
    run chmod 600 "$NEDA_HOME/.ai-agent.env"
fi

# ============================================================================
# NODE.JS TOOLS
# ============================================================================

log_info "Checking Node.js tools..."

# Function to get major node version
get_node_major_version() {
    node --version 2>/dev/null | sed 's/v\([0-9]*\).*/\1/'
}

# Function to install/upgrade Node.js based on OS
install_nodejs() {
    local version=$1

    case "$OS_FAMILY" in
        debian)
            run curl -fsSL https://deb.nodesource.com/setup_${version}.x | bash -
            run apt-get install -y nodejs
            ;;
        redhat)
            run curl -fsSL https://rpm.nodesource.com/setup_${version}.x | bash -
            run dnf install -y nodejs || run yum install -y nodejs
            ;;
        arch)
            # Arch repos have recent Node.js versions
            run pacman -S --needed --noconfirm nodejs npm
            ;;
        *)
            log_error "Unsupported OS. Please install Node.js $version manually."
            exit 1
            ;;
    esac
}

REQUIRED_NODE_VERSION=22

if command -v node &> /dev/null; then
    CURRENT_NODE_VERSION=$(get_node_major_version)
    if [ "$CURRENT_NODE_VERSION" -lt "$REQUIRED_NODE_VERSION" ]; then
        log_info "Node.js $CURRENT_NODE_VERSION found, upgrading to Node.js $REQUIRED_NODE_VERSION..."
        install_nodejs $REQUIRED_NODE_VERSION
        log_info "Node.js upgraded to $(node --version)"
    else
        log_info "Node.js $(node --version) already meets requirement (>= $REQUIRED_NODE_VERSION)"
    fi
else
    log_info "Installing Node.js $REQUIRED_NODE_VERSION..."
    install_nodejs $REQUIRED_NODE_VERSION
fi

if ! command -v npx &> /dev/null; then
    log_error "Failed to install npx"
    exit 1
fi

# ============================================================================
# GOOGLE CLOUD SDK
# ============================================================================

log_info "Setting up Google Cloud SDK for neda..."

if [ ! -d "$GCLOUD_HOME" ]; then
    log_info "Installing Google Cloud SDK to $GCLOUD_HOME..."
    
    GCLOUD_ARCHIVE="/tmp/google-cloud-cli-$(date +%s).tar.gz"
    run curl -L -o "$GCLOUD_ARCHIVE" https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-x86_64.tar.gz
    
    run tar -xzf "$GCLOUD_ARCHIVE" -C "$NEDA_HOME"
    run rm "$GCLOUD_ARCHIVE"
    
    run chown -R "$NEDA_USER:$NEDA_GROUP" "$GCLOUD_HOME"
    
    run sudo -u "$NEDA_USER" bash "$GCLOUD_HOME/install.sh" \
        --quiet \
        --path-update false \
        --command-completion false
    
    log_info "Google Cloud SDK installed"
else
    log_info "Google Cloud SDK already installed at $GCLOUD_HOME"
fi

# ============================================================================
# TOOLBOX FOR BIGQUERY MCP
# ============================================================================

log_info "Installing Google AI Toolbox for BigQuery MCP..."

TOOLBOX_DEST="$NEDA_HOME/bin/toolbox"

# Download and install Google AI Toolbox binary
if [ ! -f "$TOOLBOX_DEST" ]; then
    log_info "Downloading Google AI Toolbox binary..."
    
    # Detect architecture
    ARCH=$(uname -m)
    if [ "$ARCH" = "x86_64" ]; then
        TOOLBOX_ARCH="amd64"
    elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
        TOOLBOX_ARCH="arm64"
    else
        log_error "Unsupported architecture: $ARCH"
        exit 1
    fi
    
    # Download toolbox binary
    TOOLBOX_VERSION="v0.14.0"
    TOOLBOX_URL="https://storage.googleapis.com/genai-toolbox/${TOOLBOX_VERSION}/linux/${TOOLBOX_ARCH}/toolbox"
    
    run curl -fsSL -o "$TOOLBOX_DEST" "$TOOLBOX_URL" || {
        log_error "Failed to download toolbox from $TOOLBOX_URL"
        exit 1
    }
    
    # Make it executable
    run chmod +x "$TOOLBOX_DEST"
    
    # Change ownership
    run chown "$NEDA_USER:$NEDA_GROUP" "$TOOLBOX_DEST"
    
    # Verify installation
    if sudo -u "$NEDA_USER" "$TOOLBOX_DEST" --version >/dev/null 2>&1; then
        log_info "Google AI Toolbox installed successfully"
    else
        log_error "Failed to verify toolbox installation"
        exit 1
    fi
else
    log_info "Toolbox already installed at $TOOLBOX_DEST"
fi



# ============================================================================
# NETDATA REPOSITORIES SETUP
# ============================================================================

log_info "Setting up Netdata repositories directory..."

# Create public-facing symlink structure for source code access
# This provides clean paths like github.com/netdata/netdata pointing to actual repos
NETDATA_REPOS_PUBLIC="$NEDA_HOME/netdata-repos-public"

log_info "Creating public repository symlink structure at $NETDATA_REPOS_PUBLIC..."

run mkdir -p "$NETDATA_REPOS_PUBLIC/github.com/netdata"
run mkdir -p "$NETDATA_REPOS_PUBLIC/learn.netdata.cloud"

# Create symlinks using ln -sfn for atomic replacement (no disruption to running queries)
# -s = symbolic link, -f = force (replace), -n = no-dereference (don't follow existing symlink)
run ln -sfn ../../../netdata-repos/netdata "$NETDATA_REPOS_PUBLIC/github.com/netdata/netdata"
run ln -sfn ../../../netdata-repos/helmchart "$NETDATA_REPOS_PUBLIC/github.com/netdata/helmchart"
run ln -sfn ../../../netdata-repos/netdata-grafana-datasource-plugin "$NETDATA_REPOS_PUBLIC/github.com/netdata/netdata-grafana-datasource-plugin"
run ln -sfn ../../../netdata-repos/community "$NETDATA_REPOS_PUBLIC/github.com/netdata/community"
run ln -sfn ../../netdata-repos/learn/docs "$NETDATA_REPOS_PUBLIC/learn.netdata.cloud/docs"
run ln -sfn ../netdata-repos/website/content "$NETDATA_REPOS_PUBLIC/www.netdata.cloud"

run chown -R "$NEDA_USER:$NEDA_GROUP" "$NETDATA_REPOS_PUBLIC"

log_info "Public repository structure created"

# Copy GitHub App sync script
if [ -f "neda/sync-netdata-repos.sh" ]; then
    log_info "Installing GitHub App repository sync script..."
    run cp "neda/sync-netdata-repos.sh" "$NEDA_HOME/bin/sync-netdata-repos.sh"
    run chmod 755 "$NEDA_HOME/bin/sync-netdata-repos.sh"
    run chown "$NEDA_USER:$NEDA_GROUP" "$NEDA_HOME/bin/sync-netdata-repos.sh"
    log_info "Repository sync script installed at $NEDA_HOME/bin/sync-netdata-repos.sh"
else
    log_warn "GitHub App sync script not found, skipping repository sync setup"
fi

# ============================================================================
# MCP-GSC (Google Search Console)
# ============================================================================

log_info "Setting up mcp-gsc for Google Search Console access..."

MCP_GSC_DIR="$NEDA_HOME/mcp/mcp-gsc"

if [ ! -d "$MCP_GSC_DIR" ]; then
    log_info "Installing mcp-gsc..."
    
    # Ensure mcp directory has correct ownership
    run chown -R "$NEDA_USER:$NEDA_GROUP" "$NEDA_HOME/mcp"
    
    run sudo -u "$NEDA_USER" git clone https://github.com/AminForou/mcp-gsc "$MCP_GSC_DIR"
    
    run sudo -u "$NEDA_USER" python3 -m venv --upgrade-deps "$MCP_GSC_DIR/.venv"

    run sudo -u "$NEDA_USER" "$MCP_GSC_DIR/.venv/bin/pip" install -r "$MCP_GSC_DIR/requirements.txt"
    
    log_info "mcp-gsc installed successfully"
else
    log_info "mcp-gsc already installed at $MCP_GSC_DIR"
fi

# ============================================================================
# CREATE LAUNCHER SCRIPT
# ============================================================================

log_info "Creating neda launcher script..."

cat > "$NEDA_HOME/bin/neda" <<EOF
#!/usr/bin/env bash
# Neda launcher script

export PATH="\$PATH:$GCLOUD_HOME/bin:$NEDA_HOME/bin:$NEDA_HOME/.local/bin"
export HOME="$NEDA_HOME"

# Change to neda directory for relative paths
cd "$NEDA_HOME"

# Execute ai-agent with neda configuration
exec $AI_AGENT_DIR/ai-agent "\$@"
EOF

run chmod 755 "$NEDA_HOME/bin/neda"

# ============================================================================
# FINAL OWNERSHIP AND PERMISSIONS
# ============================================================================

log_info "Setting final ownership and permissions..."

# Ensure everything in /opt/neda is owned by neda user
run chown -R "$NEDA_USER:$NEDA_GROUP" "$NEDA_HOME"

# ============================================================================
# INITIALIZE GIT REPOSITORY
# ============================================================================

log_info "Initializing local git repository for configuration management..."

# Check if git repo already exists
if [ ! -d "$NEDA_HOME/.git" ]; then
    # Initialize git repo as neda user
    run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" init
    
    # Configure git for neda user
    run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" config user.name "Neda Setup"
    run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" config user.email "neda@localhost"
    
    # Initial commit with all configuration
    run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" add -A
    run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" commit -m "Initial Neda installation" || \
        log_warn "Nothing to commit (this is normal for reinstalls)"
    
    log_info "Git repository initialized. Future updates can be tracked with commits."
else
    log_info "Git repository already exists at $NEDA_HOME/.git"
    
    # Still commit any changes from the update
    run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" add -A
    run sudo -u "$NEDA_USER" git -C "$NEDA_HOME" commit -m "Neda update $(date +%Y-%m-%d_%H:%M:%S)" || \
        log_warn "Nothing to commit (no changes detected)"
fi

# Ensure logs directory is writable
run chmod 755 "$NEDA_HOME/logs"

# ============================================================================
# SERVICE FILES ARE NOW STATIC - COPIED DURING SETUP
# ============================================================================

log_info "Service files have been copied to $NEDA_HOME"
log_info "Use services-install.sh to install them to systemd"

# ============================================================================
# COMPLETION
# ============================================================================

log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Install Python tools (pipx, uv/uvx) for MCP servers
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
log_info "Setting up Python tools for MCP servers..."

# pipx is installed in prerequisites section
# Ensure neda user's pipx is configured properly
run sudo -u "$NEDA_USER" env HOME="$NEDA_HOME" pipx ensurepath

# Install uv for the neda user (uvx is part of uv)
# uv installs to ~/.local/bin by default
UV_BIN="$NEDA_HOME/.local/bin/uv"
if [ ! -f "$UV_BIN" ]; then
    log_info "Installing uv/uvx for neda user..."
    run sudo -u "$NEDA_USER" env HOME="$NEDA_HOME" bash -c 'curl -LsSf https://astral.sh/uv/install.sh | sh'

    if [ -f "$UV_BIN" ]; then
        log_info "uv installed successfully at $UV_BIN"
    else
        log_error "Failed to install uv for neda user"
        exit 1
    fi
else
    log_info "uv already installed at $UV_BIN"
fi

# Note: analytics-mcp and freshdesk-mcp are run via 'pipx run' and 'uvx' respectively
# They don't need pre-installation as they're fetched and cached on first use

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Install Playwright browsers for fetcher MCP server
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
log_info "Installing Playwright browsers for fetcher MCP server..."
# Set the cache directory for neda user
export PLAYWRIGHT_BROWSERS_PATH="$NEDA_HOME/.cache/ms-playwright"
# Change to neda home directory where user has permissions
cd "$NEDA_HOME"
# Install specific Playwright version to avoid conflicts with system version
# First install playwright package locally to ensure consistent version
# Use neda's home for npm cache to avoid permission issues
run sudo -u "$NEDA_USER" env HOME="$NEDA_HOME" npm install --prefix "$NEDA_HOME" playwright@latest

# Install all Playwright browsers that fetcher might need
run sudo -u "$NEDA_USER" env HOME="$NEDA_HOME" PLAYWRIGHT_BROWSERS_PATH="$NEDA_HOME/.cache/ms-playwright" npx --prefix "$NEDA_HOME" playwright install

# Install system dependencies required by Playwright browsers
log_info "Installing Playwright system dependencies..."

case "$OS_FAMILY" in
    debian)
        # Debian/Ubuntu: playwright install-deps works natively
        run npx --prefix "$NEDA_HOME" playwright install-deps
        ;;
    redhat)
        # Fedora/RHEL: install known Playwright dependencies
        PLAYWRIGHT_DEPS_REDHAT=(
            nss nspr at-spi2-atk at-spi2-core cups-libs libdrm dbus-libs
            libxcb libxkbcommon libX11 libXcomposite libXdamage libXext
            libXfixes libXrandr mesa-libgbm pango cairo alsa-lib
            libxshmfence gtk3 libgbm
        )
        log_info "Installing Playwright deps for Fedora/RHEL: ${PLAYWRIGHT_DEPS_REDHAT[*]}"
        run dnf install -y "${PLAYWRIGHT_DEPS_REDHAT[@]}" || run yum install -y "${PLAYWRIGHT_DEPS_REDHAT[@]}"
        ;;
    arch)
        # Arch/Manjaro: install official repo packages
        PLAYWRIGHT_DEPS_ARCH=(
            nss nspr at-spi2-core libcups libdrm dbus libxcb libxkbcommon
            libx11 libxcomposite libxdamage libxext libxfixes libxrandr
            mesa pango cairo alsa-lib gtk3 xorg-server-xvfb
        )
        log_info "Installing Playwright deps for Arch/Manjaro: ${PLAYWRIGHT_DEPS_ARCH[*]}"
        if ! pacman -S --needed --noconfirm "${PLAYWRIGHT_DEPS_ARCH[@]}"; then
            log_warn "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            log_warn "Some Playwright dependencies failed to install."
            log_warn "This usually happens when your system needs updating. Try:"
            log_warn "  sudo pacman -Syu"
            log_warn "Then re-run this setup script."
            log_warn "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        fi

        # AUR packages that may be needed for Playwright browsers
        # Must run as non-root user (yay/paru refuse to run as root)
        PLAYWRIGHT_AUR_PKGS=(icu66-bin libffi7 libwebp0.5)

        # Determine which user to run AUR helper as
        AUR_USER="${SUDO_USER:-$NEDA_USER}"

        if command -v yay &>/dev/null; then
            log_info "Installing AUR packages via yay (as $AUR_USER): ${PLAYWRIGHT_AUR_PKGS[*]}"
            sudo -u "$AUR_USER" yay -S --needed --noconfirm "${PLAYWRIGHT_AUR_PKGS[@]}" || \
                log_warn "Some AUR packages failed to install. Playwright browsers may not work correctly."
        elif command -v paru &>/dev/null; then
            log_info "Installing AUR packages via paru (as $AUR_USER): ${PLAYWRIGHT_AUR_PKGS[*]}"
            sudo -u "$AUR_USER" paru -S --needed --noconfirm "${PLAYWRIGHT_AUR_PKGS[@]}" || \
                log_warn "Some AUR packages failed to install. Playwright browsers may not work correctly."
        else
            log_warn "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            log_warn "Arch/Manjaro: yay/paru not found. Some Playwright browsers may require AUR packages."
            log_warn "If browsers fail to launch, install yay/paru and run:"
            log_warn "  yay -S --needed ${PLAYWRIGHT_AUR_PKGS[*]}"
            log_warn "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        fi
        ;;
    *)
        log_warn "Unknown OS family. Skipping Playwright system dependencies."
        log_warn "You may need to install browser dependencies manually."
        ;;
esac

# Create compatibility copies for common version mismatches
# This handles version differences between fetcher-mcp and installed Playwright
for source_dir in "$NEDA_HOME/.cache/ms-playwright"/chromium_headless_shell-*; do
    if [ -d "$source_dir" ]; then
        base_name=$(basename "$source_dir")
        version="${base_name#chromium_headless_shell-}"
        # Create copies for commonly expected versions
        for target_version in 1187 1169 1194; do
            target_dir="$NEDA_HOME/.cache/ms-playwright/chromium_headless_shell-$target_version"
            if [ "$version" != "$target_version" ] && [ ! -e "$target_dir" ]; then
                log_info "Creating compatibility copy for chromium headless shell version $target_version..."
                run sudo -u "$NEDA_USER" cp -r "$source_dir" "$target_dir"
            fi
        done
        break # Only need to copy from one source
    fi
done

cd - > /dev/null

log_info "Neda installation completed!"
log_info ""
log_info "Installation summary:"
log_info "  ✓ Neda user: $NEDA_USER"
log_info "  ✓ Installation directory: $NEDA_HOME"
log_info "  ✓ All neda files copied from source"
log_info "  ✓ Google Cloud SDK installed"
log_info "  ✓ Repository sync script created"
log_info "  ✓ Public repository symlinks created"
log_info "  ✓ MCP-GSC installed"
log_info "  ✓ Playwright browsers installed"
log_info "  ✓ Systemd service files created (not installed)"
log_info "  ✓ Git repository initialized for configuration tracking"
log_info ""
log_info "Configuration files have been copied to $NEDA_HOME"
log_info "Additional configuration may be needed for API keys and service accounts."
log_info ""
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "SYSTEMD SERVICE MANAGEMENT"
log_info ""
log_info "Service files have been copied to $NEDA_HOME:"
log_info "  • neda.service             - Main Neda service"
log_info "  • neda-repos-sync.service  - Repository sync service"
log_info "  • neda-repos-sync.timer    - Automatic sync every 6 hours"
log_info ""
log_info "Service management scripts:"
log_info "  • services-install.sh      - Install/update and start services"
log_info "  • services-uninstall.sh    - Stop and remove services"
log_info ""
log_info "To INSTALL services:"
log_info "  sudo $NEDA_HOME/services-install.sh"
log_info ""
log_info "To UNINSTALL services:"
log_info "  sudo $NEDA_HOME/services-uninstall.sh"
log_info ""
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info ""
log_info "Clone Netdata repositories manually (requires GitHub App setup):"
log_info "  sudo -u neda /opt/neda/bin/sync-netdata-repos.sh"
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"