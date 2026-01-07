#!/usr/bin/env bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/opt/ai-agent"
BACKUP_DIR="/opt/ai-agent.backup.$(date +%Y%m%d_%H%M%S)"

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

# Check if running as root when needed
require_sudo() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This step requires root privileges. Please run with sudo."
        exit 1
    fi
}

# Resolve repo root (directory containing this script)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

log_info "Starting AI Agent build and installation process..."

# ============================================================================
# DEPENDENCY CHECKS
# ============================================================================

# Check for ripgrep (required at runtime for file search operations)
if ! command -v rg &> /dev/null; then
    log_warn "ripgrep (rg) is not installed."
    log_warn "ai-agent requires ripgrep for file search operations."
    log_warn "Install it via your package manager:"
    log_warn "  - Arch/Manjaro: sudo pacman -S ripgrep"
    log_warn "  - Ubuntu/Debian: sudo apt install ripgrep"
    log_warn "  - RHEL/Fedora: sudo dnf install ripgrep"
    log_warn "  - macOS: brew install ripgrep"
    log_warn ""
fi

# ============================================================================
# BUILD PHASE
# ============================================================================

log_info "Building AI Agent from source..."

# Install dependencies if needed
#if [ ! -d "node_modules" ]; then
    log_info "Installing Node.js dependencies..."
    run npm install
#fi

# Build TypeScript
log_info "Compiling TypeScript..."
run npm run build

# Run linting
log_info "Running linter..."
run npm run lint || {
    log_warn "Linting failed, but continuing with installation..."
}

# ============================================================================
# INSTALLATION PHASE (requires sudo)
# ============================================================================

log_info "Preparing installation to $INSTALL_DIR..."

# Check if we need sudo for the installation steps
if [[ -w "/opt" ]] 2>/dev/null; then
    SUDO=""
else
    SUDO="sudo"
    log_info "Installation requires sudo privileges..."
fi

# Create backup if installation exists
if [ -d "$INSTALL_DIR" ]; then
    log_info "Creating backup of existing installation at $BACKUP_DIR..."
    run $SUDO mkdir -p "$BACKUP_DIR"
    run $SUDO cp -a "$INSTALL_DIR/." "$BACKUP_DIR/" 2>/dev/null || true
fi

# Clean up old backups, keeping only the 10 most recent
# Safety: strict pattern matching and individual path validation
cleanup_old_backups() {
    local keep_count=10
    local backup_pattern='^ai-agent\.backup\.[0-9]{8}_[0-9]{6}$'

    # Find all backup directories in /opt that match our naming pattern
    # Using -maxdepth 1 to stay strictly in /opt
    local all_backups=()
    while IFS= read -r dir; do
        local basename
        basename=$(basename "$dir")
        # Strict validation: must match exact pattern YYYYMMDD_HHMMSS
        if [[ "$basename" =~ $backup_pattern ]]; then
            all_backups+=("$dir")
        fi
    done < <(find /opt -maxdepth 1 -type d -name 'ai-agent.backup.*' 2>/dev/null | sort)

    local total=${#all_backups[@]}
    if (( total <= keep_count )); then
        return 0
    fi

    local to_delete=$(( total - keep_count ))
    log_info "Cleaning up old backups: removing $to_delete, keeping $keep_count most recent..."

    # Delete oldest backups (array is sorted, oldest first)
    for (( i=0; i<to_delete; i++ )); do
        local backup_path="${all_backups[$i]}"
        local backup_name
        backup_name=$(basename "$backup_path")

        # Final safety check: validate full path structure
        if [[ "$backup_path" == "/opt/$backup_name" ]] && \
           [[ "$backup_name" =~ $backup_pattern ]] && \
           [[ -d "$backup_path" ]]; then
            log_info "  Removing old backup: $backup_name"
            $SUDO rm -rf "$backup_path"
        else
            log_warn "  Skipping suspicious path: $backup_path"
        fi
    done
}

cleanup_old_backups

# Create directory structure
log_info "Creating installation directory structure..."
run $SUDO mkdir -p "$INSTALL_DIR"/{dist,mcp,cache,tmp,logs}

# ============================================================================
# COPY APPLICATION FILES
# ============================================================================

log_info "Installing application files..."

# Copy compiled JavaScript
log_info "  - Installing compiled application..."
run $SUDO rm -rf "$INSTALL_DIR/dist"
run $SUDO cp -r dist "$INSTALL_DIR/"

# Copy production dependencies
log_info "  - Installing production dependencies..."
run $SUDO rm -rf "$INSTALL_DIR/node_modules"
run $SUDO cp -r node_modules "$INSTALL_DIR/"

# Copy package files
log_info "  - Installing package files..."
run $SUDO cp package.json "$INSTALL_DIR/"
run $SUDO cp package-lock.json "$INSTALL_DIR/"

# Note: Neda installation is handled separately by neda/neda-setup.sh

# ============================================================================
# INSTALL MCP SERVERS
# ============================================================================

log_info "Setting up MCP servers..."

# Copy MCP server files
run $SUDO cp -r mcp/* "$INSTALL_DIR/mcp/" 2>/dev/null || true

# Note: Application-specific MCP servers (like mcp-gsc for neda) should be
# installed by their respective setup scripts (e.g., neda/neda-setup.sh)

# ============================================================================
# SET PERMISSIONS
# ============================================================================

log_info "Setting file permissions..."

# Make main executable accessible
run $SUDO chmod 755 "$INSTALL_DIR/dist/cli.js" 2>/dev/null || true

# Set reasonable permissions for directories
run $SUDO chmod 755 "$INSTALL_DIR"
run $SUDO chmod -R 755 "$INSTALL_DIR/dist"
run $SUDO chmod -R 755 "$INSTALL_DIR/mcp"

# ============================================================================
# INSTALL LAUNCHER SCRIPT
# ============================================================================

log_info "Installing launcher script..."

# Create launcher script in /opt/ai-agent
LAUNCHER="$INSTALL_DIR/ai-agent"
log_info "Creating launcher script..."
run $SUDO tee "$LAUNCHER" > /dev/null <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

# This launcher runs the installed version in /opt/ai-agent
exec node /opt/ai-agent/dist/cli.js "$@"
EOF

run $SUDO chmod 755 "$LAUNCHER"

# Create symlink in /usr/local/bin
TARGET_LINK="/usr/local/bin/ai-agent"
log_info "Creating system-wide symlink: $TARGET_LINK -> $LAUNCHER"
run $SUDO ln -sf "$LAUNCHER" "$TARGET_LINK"

# Note: Service installation is handled by application-specific setup scripts

# ============================================================================
# COMPLETION
# ============================================================================

log_info "AI Agent framework installation completed successfully!"
log_info ""
log_info "Installation summary:"
log_info "  - Installed to: $INSTALL_DIR"
log_info "  - Command: /usr/local/bin/ai-agent"
log_info ""
log_info "The ai-agent command is now available system-wide."
log_info ""
log_info "To install specific applications like Neda:"
log_info "  sudo ./neda/neda-setup.sh"

if [ -d "$BACKUP_DIR" ]; then
    log_info ""
    log_info "Previous installation backed up to: $BACKUP_DIR"
fi
