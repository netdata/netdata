#!/usr/bin/env bash

set -euo pipefail

# Configuration
NEDA_HOME="${NEDA_HOME:-/opt/neda}"
REPOS_DIR="$NEDA_HOME/netdata-repos"
LOG_FILE="$NEDA_HOME/logs/repo-sync.log"
GITHUB_APP_CONFIG="$NEDA_HOME/.github-app.json"

# Repositories to exclude from syncing
EXCLUDED_REPOS=(
    "binary-packages"  # Large binary files repository
    "test-repo"        # Test repository
    "test-repo-2"      # Test repository
    "learn-writerside" # WriterSide documentation repository
    "chris-temp"       # Temporary repository
    "go.d.plugin"      # Legacy go.d plugin
    "netdata-demo-site" # Demo site repository
    "hr"               # HR repository
    "bash.d.plugin"    # Bash plugin
    "helper-images"    # Helper images
    "go-recaptcha"     # Go recaptcha library
    "go-orchestrator"  # Go orchestrator
    "go-statsd"        # Go statsd library
    "blog"             # Blog repository
    "ioping"           # IOPing repository
    "homebrew-core"    # Homebrew core fork
    "netdata-portage-overlay" # Portage overlay
    "cloud-be-legacy"  # Legacy cloud backend
    "netdata-learn"    # Learn documentation
    "stargazers"       # Stargazers repository
    "vernemq-docker"   # VerneMQ Docker
    "internal"         # Internal repository
    "paho.golang"      # Paho Go client
    "incidents"        # Incidents repository
    "helm-exporter"    # Helm exporter
    "libjudy"          # LibJudy library
    "pulsar-client-go" # Pulsar Go client
    "netdata-logs"     # Logs repository
    "developer-relations" # Developer relations
    "Discourse-easy-footer" # Discourse footer
    "discourse-brand-header" # Discourse header
    "discourse-netdata-theme" # Discourse theme
    "discourse-google-font-component" # Discourse font
    "local-netdatas"   # Local Netdata instances
    ".github"          # GitHub configuration
    "auth-test"        # Auth testing
    "cloud-vernemq-test" # Cloud VerneMQ test
    "be-challenge"     # Backend challenge
    "customer-success" # Customer success
    "dygraphs"         # Dygraphs library
    "documentation"    # Documentation
    "demo-env"         # Demo environment
    "ecs"              # ECS repository
    "privacy_check"    # Privacy check
    "docs-images"      # Documentation images
    "hubspot"          # HubSpot repository
    "product"          # Product repository
    "ml-research"      # ML research
    "handbook"         # Handbook
    "mosquitto"        # Mosquitto MQTT
    "team-sre-shared-creds" # SRE shared credentials
    "watercooler"      # Watercooler
    "traefik-helm-chart" # Traefik Helm chart
    "ppr"              # PPR repository
    "k8s-csr-approver" # K8s CSR approver
    "gcp-log-fetcher"  # GCP log fetcher
    "static-pages"     # Static pages
    "analytics-bi"     # Analytics BI
    "ml"               # Machine learning
    "haproxy-helm-chart" # HAProxy Helm chart
    "test-automation"  # Test automation
    "react-filter-box" # React filter box
    "community"        # Community
    "netdata-posthog"  # PostHog integration
    "redsync"          # Redsync
    "cole"             # Cole repository
    "dh-client"        # DH client
    "tmp-test-github-oidc" # GitHub OIDC test
    "demo_for_the_main" # Demo repository
    "corrosion"        # Corrosion
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Create directories if they don't exist
mkdir -p "$REPOS_DIR"
mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
    echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1"
}

error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $1" >> "$LOG_FILE"
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

warn() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN: $1" >> "$LOG_FILE"
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Check for required tools
check_requirements() {
    local missing=()
    
    command -v git >/dev/null 2>&1 || missing+=("git")
    command -v python3 >/dev/null 2>&1 || missing+=("python3")
    command -v jq >/dev/null 2>&1 || missing+=("jq")
    
    if [ ${#missing[@]} -gt 0 ]; then
        error "Missing required tools: ${missing[*]}"
        error "Install with: sudo apt-get install ${missing[*]}"
        exit 1
    fi
    
    if [ ! -f "$GITHUB_APP_CONFIG" ]; then
        error "GitHub App configuration not found at $GITHUB_APP_CONFIG"
        error "Expected format:"
        error '  {'
        error '    "app_id": 123456,'
        error '    "installation_id": 789012,'
        error '    "private_key_path": "/opt/neda/.github-neda-app.private-key.pem"'
        error '  }'
        exit 1
    fi
}

# Generate JWT token for GitHub App
generate_jwt() {
    local app_id=$(jq -r .app_id "$GITHUB_APP_CONFIG")
    local private_key_path=$(jq -r .private_key_path "$GITHUB_APP_CONFIG")
    
    if [ ! -f "$private_key_path" ]; then
        error "Private key not found at $private_key_path"
        exit 1
    fi
    
    python - <<EOF
import json
import jwt
import time
from pathlib import Path

app_id = "$app_id"
private_key = Path("$private_key_path").read_text()

# JWT expires in 10 minutes
now = int(time.time())
payload = {
    "iat": now,
    "exp": now + 600,
    "iss": app_id
}

token = jwt.encode(payload, private_key, algorithm="RS256")
print(token)
EOF
}

# Get installation access token
get_access_token() {
    local jwt_token="$1"
    local installation_id=$(jq -r .installation_id "$GITHUB_APP_CONFIG")
    
    local response=$(curl -s -X POST \
        -H "Authorization: Bearer $jwt_token" \
        -H "Accept: application/vnd.github.v3+json" \
        "https://api.github.com/app/installations/$installation_id/access_tokens")
    
    echo "$response" | jq -r .token
}

# Get all repositories from the organization
get_all_repos() {
    local token="$1"
    local page=1
    
    while true; do
        local response=$(curl -s --max-time 30 \
            -H "Authorization: token $token" \
            -H "Accept: application/vnd.github.v3+json" \
            "https://api.github.com/orgs/netdata/repos?per_page=100&page=$page&type=all")
        
        local count=$(echo "$response" | jq '. | length')
        
        if [ "$count" -eq 0 ]; then
            break
        fi
        
        # Output repo info directly as we fetch it
        echo "$response" | jq -r '.[] | "\(.name)|\(.clone_url)|\(.private)"'
        
        ((page++))
    done
}

# Clone or update a repository
sync_repo() {
    local repo_name="$1"
    local clone_url="$2"
    local is_private="$3"
    local token="$4"
    
    local repo_path="$REPOS_DIR/$repo_name"
    
    # For private repos, add token to URL
    if [ "$is_private" = "true" ]; then
        clone_url="https://x-access-token:${token}@github.com/netdata/${repo_name}.git"
    fi
    
    if [ -d "$repo_path" ]; then
        cd "$repo_path"
        
        # Fetch with depth=1 to get only latest
        if ! timeout 30 git fetch --depth=1 origin 2>/dev/null; then
            warn "    Could not fetch $repo_name, trying to re-clone..."
            cd "$REPOS_DIR"
            rm -rf "$repo_path"
            if ! git clone --depth=1 "$clone_url" "$repo_name" 2>/dev/null; then
                error "    Failed to clone $repo_name"
                return 1
            fi
        else
            # Reset to latest
            git reset --hard origin/HEAD 2>/dev/null || \
            git reset --hard origin/main 2>/dev/null || \
            git reset --hard origin/master 2>/dev/null || \
            warn "    Could not reset $repo_name"
        fi
    else
        cd "$REPOS_DIR"
        if ! git clone --depth=1 "$clone_url" "$repo_name" 2>/dev/null; then
            error "    Failed to clone $repo_name"
            return 1
        fi
    fi
    
    return 0
}

# Main sync process
main() {
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "Starting Netdata repositories sync using GitHub App"
    
    check_requirements
    
    # Setup Python virtual environment if needed
    VENV_DIR="$NEDA_HOME/.venv"
    
    if [ ! -d "$VENV_DIR" ]; then
        log "Creating Python virtual environment..."
        python3 -m venv "$VENV_DIR" || {
            error "Failed to create virtual environment"
            error "Install python3-venv with: sudo pacman -S python-virtualenv"
            exit 1
        }
    fi
    
    # Activate virtual environment
    source "$VENV_DIR/bin/activate"
    
    # Check if PyJWT is installed
    if ! python -c "import jwt" 2>/dev/null; then
        log "Installing PyJWT in virtual environment..."
        pip install --upgrade pip >/dev/null 2>&1
        pip install PyJWT cryptography || {
            error "Failed to install PyJWT"
            exit 1
        }
    fi
    
    log "Generating GitHub App JWT token..."
    JWT_TOKEN=$(generate_jwt) || {
        error "Failed to generate JWT token"
        exit 1
    }
    
    log "Getting installation access token..."
    ACCESS_TOKEN=$(get_access_token "$JWT_TOKEN") || {
        error "Failed to get access token"
        exit 1
    }
    
    if [ -z "$ACCESS_TOKEN" ] || [ "$ACCESS_TOKEN" = "null" ]; then
        error "Invalid access token received"
        exit 1
    fi
    
    log "Fetching repository list from GitHub..."
    
    # Debug: Test the token
    local test_response=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: token $ACCESS_TOKEN" \
        -H "Accept: application/vnd.github.v3+json" \
        "https://api.github.com/orgs/netdata")
    
    if [ "$test_response" != "200" ]; then
        error "Failed to authenticate with GitHub (HTTP $test_response)"
        error "Check that the GitHub App is installed on the netdata organization"
        exit 1
    fi
    
    # Get all repos and process them
    local total=0
    local success=0
    local failed=0
    
    # Save repos to temp file to avoid subshell issues
    local temp_repos="/tmp/netdata-repos-$$.txt"
    
    # Ensure temp file is cleaned up on exit
    trap "rm -f '$temp_repos'" EXIT INT TERM
    
    log "Writing repository list to $temp_repos..."
    get_all_repos "$ACCESS_TOKEN" > "$temp_repos"
    
    local repo_count=$(wc -l < "$temp_repos")
    log "Found $repo_count repositories to process"
    
    while IFS='|' read -r repo_name clone_url is_private; do
        total=$((total + 1))
        
        # Skip empty lines
        if [ -z "$repo_name" ]; then
            continue
        fi
        
        # Skip excluded repositories
        for excluded in "${EXCLUDED_REPOS[@]}"; do
            if [ "$repo_name" = "$excluded" ]; then
                log "[$total] Skipping: $repo_name (excluded)"
                continue 2  # Continue outer loop
            fi
        done
        
        local privacy_flag=""
        if [ "$is_private" = "true" ]; then
            privacy_flag=" [PRIVATE]"
        fi
        
        echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} [$total] Processing: ${YELLOW}$repo_name${NC}$privacy_flag"
        
        if sync_repo "$repo_name" "$clone_url" "$is_private" "$ACCESS_TOKEN"; then
            success=$((success + 1))
        else
            failed=$((failed + 1))
        fi
    done < "$temp_repos"
    
    rm -f "$temp_repos"
    
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "Repository sync completed!"
    log "  Total repositories: $total"
    log "  Successfully synced: $success"
    if [ $failed -gt 0 ]; then
        warn "  Failed: $failed"
    fi
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# Run main function
main "$@"