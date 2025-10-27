#!/usr/bin/env bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Execute command with visibility
run() {
    printf >&2 "${GRAY}$(pwd) >${NC} "
    printf >&2 "${YELLOW}"
    printf >&2 "%q " "$@"
    printf >&2 "${NC}\n"

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

header() {
    echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

# Track overall status
LINT_FAILED=0

# Check if required tools are installed
check_tool() {
    if ! command -v "$1" &> /dev/null && ! npx "$1" --version &> /dev/null 2>&1; then
        warning "Tool '$1' not found. Install with: npm install --save-dev $2"
        return 1
    fi
    return 0
}

header "1. TypeScript Build Check"
if run npm run build; then
    success "Build passed"
else
    error "Build failed"
    LINT_FAILED=1
fi

header "2. ESLint (Standard Linting)"
if run npm run lint; then
    success "ESLint passed"
else
    error "ESLint failed"
    LINT_FAILED=1
fi

header "3. Dead Code Detection (knip)"
if check_tool knip knip; then
    if run npx knip --no-exit-code; then
        success "No dead code found"
    else
        warning "Dead code detected (see output above)"
        # Don't fail the script for dead code warnings
    fi
else
    warning "Skipping dead code detection - knip not installed"
fi

header "4. Unused Exports (ts-prune)"
if check_tool ts-prune ts-prune; then
    echo "Scanning for unused exports..."
    PRUNE_OUTPUT=$(npx ts-prune --ignore 'src/index.ts|src/ai-agent.ts' 2>&1 || true)
    if echo "$PRUNE_OUTPUT" | grep -q "used in module"; then
        echo "$PRUNE_OUTPUT"
        warning "Unused exports detected"
    else
        success "No unused exports found"
    fi
else
    warning "Skipping unused exports check - ts-prune not installed"
fi

header "5. Complexity Analysis"
echo "Checking function complexity and size..."
echo "  - Max cyclomatic complexity: 10"
echo "  - Max function lines: 100"
echo "  - Max nesting depth: 4"
echo ""

if node check-complexity.cjs 2>&1; then
    success "No complexity issues found"
else
    warning "Complexity issues detected (see output above)"
fi

header "6. Duplicate Code Detection"
if check_tool jscpd jscpd; then
    echo "Scanning for duplicate code blocks (min 10 lines, 50 tokens)..."
    echo ""
    # Clean up old reports to avoid linting them
    rm -rf report/ 2>/dev/null || true

    # Run jscpd and capture output
    JSCPD_OUTPUT=$(npx jscpd src --min-lines 10 --min-tokens 50 --format 'typescript' --ignore '**/*.d.ts,**/dist/**' 2>&1 || true)

    # Show the output
    echo "$JSCPD_OUTPUT"

    # Check if duplicates were found
    if echo "$JSCPD_OUTPUT" | grep -q "Clones found"; then
        warning "Code duplication detected (see output above)"
    else
        success "No significant code duplication found"
    fi
else
    warning "Skipping duplicate code detection - jscpd not installed"
fi

header "7. Unused Dependencies"
if check_tool depcheck depcheck; then
    DEPCHECK_OUTPUT=$(npx depcheck --ignores='@types/*,tsx,eslint-*' 2>&1)
    echo "$DEPCHECK_OUTPUT"
    if echo "$DEPCHECK_OUTPUT" | grep -q -E "(Unused dependencies|Missing dependencies|Unused devDependencies)"; then
        warning "Dependency issues detected (see output above)"
    else
        success "No unused dependencies found"
    fi
else
    warning "Skipping dependency check - depcheck not installed"
fi

# Summary
echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [ $LINT_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All critical checks passed${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 0
else
    echo -e "${RED}✗ Some critical checks failed${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 1
fi
