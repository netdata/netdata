#!/bin/bash
# Documentation Tester Agent Wrapper
# This script launches the documentation tester as an agent

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TESTER_DIR="${SCRIPT_DIR}/docs_tester"

# Check if Python is available
if ! command -v python3 &> /dev/null; then
    echo "Error: python3 not found"
    exit 1
fi

# Check if tester script exists
if [[ ! -f "${TESTER_DIR}/tester.py" ]]; then
    echo "Error: tester.py not found in ${TESTER_DIR}"
    exit 1
fi

# Check if dependencies are installed
if ! python3 -c "import requests" 2>/dev/null; then
    echo "Error: requests module not installed"
    echo "Run: pip install -r ${TESTER_DIR}/requirements.txt"
    exit 1
fi

# Check if sshpass is available
if ! command -v sshpass &> /dev/null; then
    echo "Error: sshpass not found"
    echo "Install with: brew install sshpass (macOS) or apt-get install sshpass (Debian/Ubuntu)"
    exit 1
fi

# Run the tester with provided arguments
cd "${TESTER_DIR}"
python3 tester.py "$@"
