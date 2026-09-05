#!/usr/bin/env bash

# Don't use set -e to ensure script continues even on errors
set -uo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Configuration
ORG="netdata"
ACTIVITY_CACHE_FILE=".repo-activity-cache"

# Arrays to track issues
declare -a REPOS_WITH_UNCOMMITTED=()
declare -a REPOS_WITH_UNPUSHED=()
declare -a REPOS_UPDATE_FAILED=()
declare -a REPOS_WRONG_BRANCH=()
declare -a REPOS_BRANCH_SWITCHED=()

# Scoped subset of repos (--repo flag, repeatable). Empty => all repos.
declare -a SCOPE_REPOS=()

usage() {
    cat <<EOF
sync-netdata-repos.sh [--repo NAME ...] [-h|--help]

Maintains a local mirror of Netdata-org source repositories at
\${NETDATA_REPOS_DIR} so that cross-repo grep / code review can run
locally without GitHub API round-trips and rate limits.

Phase 1 (always): for each repo in scope, skip if there are staged or
modified changes; otherwise switch to the default branch (master/main/
develop), pull, and recursively update submodules.

Phase 2 (only when no --repo flags AND 'gh' is available + authed):
discover new netdata-org source repos via 'gh repo list netdata
--source --no-archived' and clone any that are missing.

Options:
  --repo NAME    sync ONLY the named repo. Repeatable. Skips Phase 2.
  -h, --help     show this help.

Required environment:
  NETDATA_REPOS_DIR   directory holding the mirror (must exist).

Required tools:
  git, jq           always.
  gh                only for Phase 2; if missing or unauthed, Phase 2
                    is skipped with a warning.
EOF
}

# ---------------------------------------------------------------
# Early help (works without NETDATA_REPOS_DIR or any other env).
for _arg in "$@"; do
    case "$_arg" in
        -h|--help) usage; exit 0 ;;
        *) ;;  # ignore -- main parser handles all other flags
    esac
done

# ---------------------------------------------------------------
# Sanitization (runs before any work).

# 1. NETDATA_REPOS_DIR set and points to an existing directory.
if [ -z "${NETDATA_REPOS_DIR:-}" ]; then
    echo "ERROR: NETDATA_REPOS_DIR is not set." >&2
    echo "       Set it in <repo>/.env (or your shell env) to the directory" >&2
    echo "       that holds (or will hold) your Netdata-org repos mirror." >&2
    exit 2
fi
MIRROR_DIR="$NETDATA_REPOS_DIR"
if [ ! -d "$MIRROR_DIR" ]; then
    echo "ERROR: NETDATA_REPOS_DIR='$MIRROR_DIR' is not an existing directory." >&2
    echo "       Create it first:  mkdir -p \"\$NETDATA_REPOS_DIR\"" >&2
    exit 2
fi

# 2. Required tools.
for _cmd in git jq; do
    if ! command -v "$_cmd" >/dev/null 2>&1; then
        echo "ERROR: '$_cmd' is required but not found in PATH." >&2
        exit 2
    fi
done

# 3. Optional: gh (Phase 2 discovery only).
GH_AVAILABLE=true
GH_REASON=""
if ! command -v gh >/dev/null 2>&1; then
    GH_AVAILABLE=false
    GH_REASON="'gh' is not installed"
elif ! gh auth status >/dev/null 2>&1; then
    GH_AVAILABLE=false
    GH_REASON="'gh' is not authenticated (run: gh auth login)"
fi

cd "$MIRROR_DIR" || { echo "ERROR: cannot cd into NETDATA_REPOS_DIR=$MIRROR_DIR" >&2; exit 2; }

# Function to print colored output
print_status() {
    local msg="$1"
    echo -e "$msg"
}

# Function to check if directory is a git repository
is_git_repo() {
    local repo="$1"
    [ -d "$repo/.git" ]
}

# Function to get last commit timestamp quickly (using filesystem heuristic)
get_last_activity() {
    local repo="$1"
    # Use the modification time of .git/logs/HEAD if it exists (fast heuristic)
    # This file is updated on commits, pulls, etc.
    if [ -f "$repo/.git/logs/HEAD" ]; then
        stat -c %Y "$repo/.git/logs/HEAD" 2>/dev/null || echo "0"
    elif [ -d "$repo/.git" ]; then
        # Fallback to .git directory modification time
        stat -c %Y "$repo/.git" 2>/dev/null || echo "0"
    else
        echo "0"
    fi
}

# Function to check for uncommitted changes (ignoring untracked files)
has_uncommitted_changes() {
    local repo="$1"
    # Safety: ensure we're in the right directory
    if [ ! -d "$repo" ]; then
        return 0  # Treat missing directory as "has changes" to skip it
    fi
    cd "$repo" || return 0  # If cd fails, treat as "has changes"
    
    # First check if we have a valid HEAD (repo might be empty or corrupted)
    if ! git rev-parse HEAD >/dev/null 2>&1; then
        cd "$MIRROR_DIR" 2>/dev/null || true
        return 1  # No HEAD means no commits, so no uncommitted changes to worry about
    fi
    
    # Refresh the index to avoid false positives from timestamp changes
    git update-index --refresh >/dev/null 2>&1 || true
    
    # Check only for staged or modified files, not untracked files
    # git diff-index checks for staged and modified files
    # We ignore untracked files since they don't affect pulls
    git diff-index --quiet HEAD -- 2>/dev/null
    local diff_result=$?
    
    # git diff-index returns 0 if no changes, 1 if changes exist
    # We want to return 0 (true) if changes exist, 1 (false) if no changes
    if [ $diff_result -eq 1 ]; then
        local result=0  # Has changes
    else
        local result=1  # No changes
    fi
    
    cd "$MIRROR_DIR" 2>/dev/null || true  # Try to return to script directory
    return $result
}

# Function to check for unpushed commits
has_unpushed_commits() {
    local repo="$1"
    # Safety: ensure we're in the right directory
    if [ ! -d "$repo" ]; then
        return 1  # No unpushed if directory doesn't exist
    fi
    cd "$repo" || return 1
    local branch
    branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
    local result
    if [ -n "$branch" ] && git rev-parse --verify "origin/$branch" >/dev/null 2>&1; then
        [ -n "$(git log "origin/$branch..HEAD" --oneline 2>/dev/null)" ]
        result=$?
    else
        result=1
    fi
    cd "$MIRROR_DIR" 2>/dev/null || true  # Try to return to script directory
    return $result
}

# Function to get uncommitted changes details
get_uncommitted_details() {
    local repo="$1"
    # Safety: ensure we're in the right directory
    if [ ! -d "$repo" ]; then
        echo "directory not found"
        return
    fi
    cd "$repo" || { echo "cannot access"; return; }
    local staged unstaged
    staged=$(git diff --cached --numstat | wc -l)
    unstaged=$(git diff --numstat | wc -l)
    cd "$MIRROR_DIR" 2>/dev/null || true  # Try to return to script directory
    
    # Report details - note that untracked files are shown but don't block updates
    if [ "$staged" -gt 0 ] || [ "$unstaged" -gt 0 ]; then
        echo "staged: $staged, modified: $unstaged"
    else
        # This can happen if git diff-index failed for other reasons
        echo "git index issue or empty repository"
    fi
}

# Function to update repository activity cache
update_activity_cache() {
    print_status "${CYAN}Updating repository activity cache...${NC}"
    
    # Safety: Use a unique temp file to avoid conflicts
    local temp_file="$ACTIVITY_CACHE_FILE.tmp.$$"
    : > "$temp_file"

    for dir in */; do
        dir="${dir%/}"
        # Only process actual directories that are git repos
        if [ -d "$dir" ] && is_git_repo "$dir"; then
            local timestamp
            timestamp=$(get_last_activity "$dir")
            echo "$timestamp $dir" >> "$temp_file"
        fi
    done
    
    # Sort by timestamp (descending) and keep only the repo names
    if [ -s "$temp_file" ]; then
        sort -rn "$temp_file" | cut -d' ' -f2 > "$ACTIVITY_CACHE_FILE"
    fi
    rm -f "$temp_file"
}

# Function to get sorted repo list
get_sorted_repos() {
    if [ -f "$ACTIVITY_CACHE_FILE" ]; then
        cat "$ACTIVITY_CACHE_FILE"
    else
        # If no cache exists, create one
        update_activity_cache
        cat "$ACTIVITY_CACHE_FILE"
    fi
}

# Function to get default branch (assumes we're already in the repo directory)
get_default_branch() {
    # Try to get from remote
    local default_branch
    default_branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')
    
    # If that fails, try common defaults
    if [ -z "$default_branch" ]; then
        if git show-ref --verify --quiet refs/remotes/origin/master; then
            default_branch="master"
        elif git show-ref --verify --quiet refs/remotes/origin/main; then
            default_branch="main"
        elif git show-ref --verify --quiet refs/remotes/origin/develop; then
            default_branch="develop"
        fi
    fi
    
    echo "$default_branch"
}

# Function to update a single repository
update_repo() {
    local repo="$1"
    local current_num="$2"
    local total_num="$3"
    
    # Safety: validate repo directory exists and is a git repo
    if [ ! -d "$repo" ]; then
        print_status "${RED}[${current_num}/${total_num}] Skipping $repo - directory not found${NC}"
        return 1
    fi
    
    if ! is_git_repo "$repo"; then
        print_status "${YELLOW}[${current_num}/${total_num}] Skipping $repo - not a git repository${NC}"
        return 1
    fi
    
    print_status "${BLUE}[${current_num}/${total_num}]${NC} ${BOLD}Updating $repo...${NC}"
    
    # Check for uncommitted changes (staged or modified files only)
    if has_uncommitted_changes "$repo"; then
        local details
        details=$(get_uncommitted_details "$repo")
        print_status "  ${YELLOW}⚠️  Skipping - uncommitted changes (${details})${NC}"
        REPOS_WITH_UNCOMMITTED+=("$repo: $details")
        return 1
    fi
    
    cd "$repo" || { print_status "  ${RED}✗ Cannot access directory${NC}"; return 1; }
    
    # Check for untracked files (informational only - doesn't block update)
    local untracked_count
    untracked_count=$(git ls-files --others --exclude-standard 2>/dev/null | wc -l)
    if [ "$untracked_count" -gt 0 ]; then
        print_status "  ${CYAN}ℹ️  Note: ${untracked_count} untracked file(s) present${NC}"
    fi

    # Get current and default branches
    local current_branch default_branch
    current_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
    default_branch=$(get_default_branch)

    # Check for unpushed commits (we're already in the repo directory)
    if [ -n "$current_branch" ] && git rev-parse --verify "origin/$current_branch" >/dev/null 2>&1; then
        if [ -n "$(git log "origin/$current_branch..HEAD" --oneline 2>/dev/null)" ]; then
            local unpushed_count
            unpushed_count=$(git log "origin/${current_branch}..HEAD" --oneline 2>/dev/null | wc -l)
            print_status "  ${YELLOW}⚠️  Warning: ${unpushed_count} unpushed commit(s) on ${current_branch}${NC}"
            REPOS_WITH_UNPUSHED+=("$repo: $unpushed_count commits on $current_branch")
        fi
    fi
    
    # Switch to default branch if needed
    if [ -n "$default_branch" ] && [ "$current_branch" != "$default_branch" ]; then
        print_status "  ${CYAN}→ Switching from ${current_branch} to ${default_branch}${NC}"
        if git checkout "$default_branch" >/dev/null 2>&1; then
            REPOS_BRANCH_SWITCHED+=("$repo: $current_branch → $default_branch")
            current_branch="$default_branch"
        else
            print_status "  ${RED}✗ Failed to switch to ${default_branch}${NC}"
            REPOS_WRONG_BRANCH+=("$repo: stuck on $current_branch, default is $default_branch")
        fi
    fi
    
    # Fetch and pull
    print_status "  → Fetching..."
    if git fetch origin >/dev/null 2>&1; then
        print_status "  → Pulling ${current_branch}..."
        if git pull origin "$current_branch" >/dev/null 2>&1; then
            # Update submodules
            print_status "  → Updating submodules..."
            if git submodule update --init --force --recursive >/dev/null 2>&1; then
                print_status "  ${GREEN}✓ Updated successfully${NC}"
            else
                # Don't fail the whole update if submodules have issues
                print_status "  ${YELLOW}⚠️  Updated but submodule update had issues${NC}"
            fi
        else
            print_status "  ${RED}✗ Pull failed${NC}"
            REPOS_UPDATE_FAILED+=("$repo")
            cd "$MIRROR_DIR" 2>/dev/null || true
            return 1
        fi
    else
        print_status "  ${RED}✗ Fetch failed${NC}"
        REPOS_UPDATE_FAILED+=("$repo")
        cd "$MIRROR_DIR" 2>/dev/null || true
        return 1
    fi
    
    cd "$MIRROR_DIR" 2>/dev/null || true  # Try to return to script directory
    return 0
}

# Function to clone new repositories
clone_new_repos() {
    print_status "\n${BOLD}Checking for new repositories to clone...${NC}"
    
    local new_repos_count=0
    
    # Get list of all repos from GitHub
    print_status "Fetching repository list from GitHub..."
    local repos_list
    repos_list=$(gh repo list "$ORG" --limit 1000 --json name,sshUrl,defaultBranchRef --source --no-archived)
    
    echo "$repos_list" | jq -r '.[] | "\(.name) \(.sshUrl) \(.defaultBranchRef.name)"' | while read -r name url default_branch; do
        if [ ! -d "$name" ]; then
            new_repos_count=$((new_repos_count + 1))
            print_status "${GREEN}→ Cloning new repo: $name (default branch: $default_branch, with submodules)${NC}"
            if git clone --quiet --recursive "$url" "$name" 2>/dev/null; then
                # Set up tracking for default branch
                # Safety: validate we can enter the directory
                if cd "$name" 2>/dev/null; then
                    git symbolic-ref refs/remotes/origin/HEAD "refs/remotes/origin/$default_branch" 2>/dev/null || true
                    cd "$MIRROR_DIR" 2>/dev/null || true
                else
                    print_status "  ${YELLOW}⚠️  Warning: Could not enter cloned directory${NC}"
                fi
                print_status "  ${GREEN}✓ Cloned successfully${NC}"
            else
                print_status "  ${RED}✗ Clone failed${NC}"
            fi
        fi
    done
    
    if [ $new_repos_count -eq 0 ]; then
        print_status "No new repositories to clone."
    fi
}

# Function to print summary
print_summary() {
    print_status "\n${BOLD}═══════════════════════════════════════════════════════════${NC}"
    print_status "${BOLD}Summary Report${NC}"
    print_status "${BOLD}═══════════════════════════════════════════════════════════${NC}"
    
    if [ ${#REPOS_BRANCH_SWITCHED[@]} -gt 0 ]; then
        print_status "\n${CYAN}📌 Branches switched to default:${NC}"
        for repo in "${REPOS_BRANCH_SWITCHED[@]}"; do
            print_status "  • $repo"
        done
    fi
    
    if [ ${#REPOS_WITH_UNCOMMITTED[@]} -gt 0 ]; then
        print_status "\n${YELLOW}⚠️  Repositories with uncommitted changes (skipped):${NC}"
        for repo in "${REPOS_WITH_UNCOMMITTED[@]}"; do
            print_status "  • $repo"
        done
    fi
    
    if [ ${#REPOS_WITH_UNPUSHED[@]} -gt 0 ]; then
        print_status "\n${YELLOW}📤 Repositories with unpushed commits:${NC}"
        for repo in "${REPOS_WITH_UNPUSHED[@]}"; do
            print_status "  • $repo"
        done
    fi
    
    if [ ${#REPOS_WRONG_BRANCH[@]} -gt 0 ]; then
        print_status "\n${YELLOW}🔀 Repositories on wrong branch:${NC}"
        for repo in "${REPOS_WRONG_BRANCH[@]}"; do
            print_status "  • $repo"
        done
    fi
    
    if [ ${#REPOS_UPDATE_FAILED[@]} -gt 0 ]; then
        print_status "\n${RED}✗ Repositories that failed to update:${NC}"
        for repo in "${REPOS_UPDATE_FAILED[@]}"; do
            print_status "  • $repo"
        done
    fi
    
    if [ ${#REPOS_WITH_UNCOMMITTED[@]} -eq 0 ] && \
       [ ${#REPOS_WITH_UNPUSHED[@]} -eq 0 ] && \
       [ ${#REPOS_WRONG_BRANCH[@]} -eq 0 ] && \
       [ ${#REPOS_UPDATE_FAILED[@]} -eq 0 ]; then
        print_status "\n${GREEN}✅ All repositories are clean and up to date!${NC}"
    fi
}

# Main execution
main() {
    # Parse CLI flags.
    while [ $# -gt 0 ]; do
        local arg="$1"
        case "$arg" in
            --repo)
                if [ $# -lt 2 ]; then
                    echo "ERROR: --repo requires a repository name" >&2
                    exit 2
                fi
                local val="$2"
                SCOPE_REPOS+=("$val")
                shift 2
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                echo "ERROR: Unknown option: $arg" >&2
                usage >&2
                exit 2
                ;;
        esac
    done

    print_status "${BOLD}═══════════════════════════════════════════════════════════${NC}"
    print_status "${BOLD}Netdata Repository Sync Tool${NC}"
    print_status "${BOLD}═══════════════════════════════════════════════════════════${NC}"
    print_status "${CYAN}Mirror: ${MIRROR_DIR}${NC}"
    if [ ${#SCOPE_REPOS[@]} -gt 0 ]; then
        print_status "${CYAN}Scope:  --repo flags ->${NC} ${SCOPE_REPOS[*]}"
    fi

    # Phase 1: Update repositories.
    if [ ${#SCOPE_REPOS[@]} -gt 0 ]; then
        print_status "\n${BOLD}Phase 1: Updating scoped repositories${NC}"
    else
        print_status "\n${BOLD}Phase 1: Updating existing repositories${NC}"
        print_status "Sorting repositories by last activity..."
    fi

    # Build the working list.
    local sorted_repos=()
    if [ ${#SCOPE_REPOS[@]} -gt 0 ]; then
        # Scoped run: validate each --repo entry exists locally.
        for repo in "${SCOPE_REPOS[@]}"; do
            if [ -d "$repo" ] && is_git_repo "$repo"; then
                sorted_repos+=("$repo")
            else
                print_status "${YELLOW}⚠️  Skipping --repo $repo: not found at $MIRROR_DIR/$repo${NC}"
            fi
        done
    else
        # Default: activity-cache-sorted full set.
        while IFS= read -r repo; do
            sorted_repos+=("$repo")
        done < <(get_sorted_repos)
    fi

    local total_repos=${#sorted_repos[@]}

    if [ "$total_repos" -eq 0 ]; then
        print_status "No repositories to update."
    else
        print_status "Found ${total_repos} repositories to update.\n"

        local current=0
        for repo in "${sorted_repos[@]}"; do
            current=$((current + 1))
            if [ -d "$repo" ] && is_git_repo "$repo"; then
                update_repo "$repo" "$current" "$total_repos" || true  # Continue even if update fails
            fi
        done
    fi

    # Phase 2: Clone new repositories.
    if [ ${#SCOPE_REPOS[@]} -gt 0 ]; then
        print_status "\n${CYAN}Skipping Phase 2 (discovery): --repo flags scoped this run.${NC}"
    elif ! $GH_AVAILABLE; then
        print_status "\n${YELLOW}⚠️  Skipping Phase 2 (discovery): ${GH_REASON}.${NC}"
    else
        print_status "\n${BOLD}Phase 2: Checking for new repositories${NC}"
        clone_new_repos || true
    fi

    # Update activity cache for next run.
    print_status "\n${CYAN}Updating activity cache for next run...${NC}"
    update_activity_cache || true

    # Print summary.
    print_summary || true

    print_status "\n${GREEN}${BOLD}✅ Sync complete!${NC}"
}

# Run main function with all CLI args.
main "$@"