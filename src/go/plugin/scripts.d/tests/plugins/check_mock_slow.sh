#!/usr/bin/env bash
set -euo pipefail
sleep "${1:-3}"
echo "OK - slow execution | slept=${1:-3}s"
