#!/usr/bin/env bash

# Synthetic log generator for the systemd journal.
#
# Writes log entries at varying priorities using logger(1) to exercise
# the OTel journald receiver. Generates a mix of info, warning, error,
# and debug messages from two fake units.
#
# Usage:
#   chmod +x journald_load_generator.sh
#   ./journald_load_generator.sh [--rate 10]
#
#   --rate  Messages per second (approximate). Default: 10.

set -euo pipefail

RATE=${1:-10}
if [[ "${1:-}" == "--rate" ]]; then
    RATE=${2:-10}
fi

INTERVAL=$(awk "BEGIN {printf \"%.4f\", 1.0 / $RATE}")
OPS=0
START_TIME=$(date +%s)

TAGS=("myapp" "myapp-worker")
PRIORITIES=("info" "info" "info" "warning" "err" "debug")
MESSAGES_INFO=(
    "Request processed successfully"
    "User logged in"
    "Health check passed"
    "Cache hit for session data"
    "Scheduled task completed"
    "Connection pool: 12 active, 3 idle"
)
MESSAGES_WARNING=(
    "Response time exceeded 500ms threshold"
    "Retry attempt 2 of 3 for upstream call"
    "Memory usage at 85%"
    "Certificate expires in 7 days"
)
MESSAGES_ERROR=(
    "Failed to connect to database: connection refused"
    "Unhandled exception in request handler"
    "Disk usage critical: 95% full"
    "Authentication token expired for user admin"
)
MESSAGES_DEBUG=(
    "Parsing request body: 1284 bytes"
    "SQL query executed in 23ms"
    "Cache key: session:abc123"
)

random_element() {
    local arr=("$@")
    echo "${arr[$((RANDOM % ${#arr[@]}))]}"
}

echo "Journald load generator"
echo "Rate: ~${RATE} msg/s  (Ctrl+C to stop)"
echo ""

trap 'echo -e "\nDone. $OPS messages in $(( $(date +%s) - START_TIME ))s"' EXIT

while true; do
    TAG=$(random_element "${TAGS[@]}")
    PRIORITY=$(random_element "${PRIORITIES[@]}")

    case "$PRIORITY" in
        info)    MSG=$(random_element "${MESSAGES_INFO[@]}") ;;
        warning) MSG=$(random_element "${MESSAGES_WARNING[@]}") ;;
        err)     MSG=$(random_element "${MESSAGES_ERROR[@]}") ;;
        debug)   MSG=$(random_element "${MESSAGES_DEBUG[@]}") ;;
    esac

    MSG="$MSG [request_id=$(printf '%04x%04x' $RANDOM $RANDOM)]"

    logger -t "$TAG" -p "user.${PRIORITY}" "$MSG"

    OPS=$((OPS + 1))
    if (( OPS % 100 == 0 )); then
        ELAPSED=$(( $(date +%s) - START_TIME ))
        if (( ELAPSED > 0 )); then
            echo "  ${OPS} msgs in ${ELAPSED}s  (~$(( OPS / ELAPSED )) msg/s)"
        fi
    fi

    sleep "$INTERVAL"
done
