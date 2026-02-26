#!/bin/bash
#
# Netdata Documentation Testing Agent Wrapper Script
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON="${PYTHON:-python3}"

export PYTHONPATH="${SCRIPT_DIR}:${PYTHONPATH}"

echo "Netdata Documentation Testing Agent"
echo "================================="
echo ""

MODE="cli"

# Check for interactive mode
if [ "$1" == "--interactive" ] || [ "$1" == "-i" ]; then
    MODE="interactive"
    shift
fi

if [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
    echo "Usage: $0 [options] [<documentation-path> | --from-pr PR_NUMBER>]"
    echo ""
    echo "Modes:"
    echo "  (default)     CLI mode - run tests from command line"
    echo "  --interactive  Interactive menu-driven mode"
    echo ""
    echo "CLI Options:"
    echo "  --from-pr N        Fetch docs from GitHub PR N"
    echo "  --owner OWNER      GitHub repo owner (default: netdata)"
    echo "  --repo NAME        GitHub repo name (default: netdata)"
    echo "  --dry-run         Show test plan without executing"
    echo "  --verbose, -v     Show detailed output"
    echo "  --filter, -f      Only test claims matching pattern"
    echo "  --section, -s     Test specific line range (start:end)"
    echo "  --report-format   Output format (markdown|json|junit|tap)"
    echo "  --output, -o      Output file for report"
    echo "  --no-cleanup       Keep test configurations after run"
    echo "  --parallel, -p    Run N tests concurrently"
    echo ""
    echo "Examples:"
    echo "  $0 docs/dashboards-and-charts/badges.md"
    echo "  $0 --from-pr 21711"
    echo "  $0 --from-pr 21711 --owner netdata --repo netdata"
    echo "  $0 --filter api --verbose docs/"
    echo "  $0 --dry-run --section 100:200 docs/health.md"
    echo "  $0 --interactive"
    exit 0
fi

if [ -z "$1" ] && [ "$MODE" == "cli" ]; then
    echo "Usage: $0 [options] [<documentation-path> | --from-pr PR_NUMBER>]"
    echo ""
    echo "Run '$0 --help' for options."
    echo "Or use '--interactive' for menu mode."
    exit 0
fi

if [ "$MODE" == "interactive" ]; then
    exec $PYTHON -m netdata_tester.interactive
else
    exec $PYTHON -m netdata_tester.cli "$@"
fi