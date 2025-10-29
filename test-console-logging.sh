#!/bin/bash

# Test script to demonstrate console vs server logging modes

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Testing Console vs Server Logging Modes${NC}"
echo -e "${GREEN}========================================${NC}"
echo

echo -e "${YELLOW}1. Interactive Console Mode (when running in a real TTY)${NC}"
echo "   When you run ai-agent directly in your terminal (with TTY):"
echo "   - Simplified format: [HH:MM:SS] [SEVERITY] message"
echo "   - With --verbose: adds context like t1.2 @agent tool() → ←"
echo

echo -e "${YELLOW}2. Server Mode (MCP/Slack headends)${NC}"
echo "   When running with --mcp stdio or other headends:"
echo "   - Full logfmt format: ts=... level=... priority=... type=... etc."
echo "   - All structured fields for monitoring and debugging"
echo

echo -e "${YELLOW}3. Non-TTY Mode (pipes, redirects, CI/CD)${NC}"
echo "   When output is piped or redirected:"
echo "   - Full logfmt format (same as server mode)"
echo "   - No color codes"
echo

echo -e "${GREEN}Example Commands:${NC}"
echo
echo "# Interactive console (in a real terminal with TTY):"
echo "$ ai-agent --verbose --models anthropic/claude-3-haiku-20240307 'You are helpful' 'Say hello'"
echo "# Output: [20:30:45] [DEBUG] t1.0 llm:claude-3-haiku-20240307 → Processing request"
echo
echo "# Server mode:"
echo "$ ai-agent --mcp stdio"
echo "# Output: ts=2025-10-28T20:30:45.123Z level=vrb priority=6 type=llm ..."
echo
echo "# Piped/redirected (non-TTY):"
echo "$ ai-agent --verbose ... 2>&1 | tee log.txt"
echo "# Output: ts=2025-10-28T20:30:45.123Z level=vrb priority=6 type=llm ..."
echo

echo -e "${GREEN}Current Environment:${NC}"
if [ -t 2 ]; then
    echo "stderr IS a TTY - console format would be used (if not in server mode)"
else
    echo "stderr is NOT a TTY - logfmt format will be used"
fi
echo

echo -e "${GREEN}To force a specific format:${NC}"
echo "$ ai-agent --telemetry-log-format logfmt ..."  # Force logfmt
echo "$ ai-agent --telemetry-log-format json ..."    # Force JSON
echo "$ ai-agent --telemetry-log-format journald ..." # Force journald (if available)
echo
echo "Note: The simplified console format is automatic and cannot be forced via CLI options."
echo "      It's only used when running interactively in a terminal (TTY) without server mode."