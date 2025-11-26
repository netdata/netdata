#!/bin/bash
# Example: Push system metrics to Netdata Alternative UI using curl
#
# This script collects basic system metrics and pushes them to the server.
# Only requires curl and standard Unix utilities.
#
# Usage:
#   ./push_metrics.sh [--url URL] [--node-id NODE_ID] [--interval SECONDS]

URL="${URL:-http://localhost:19998}"
NODE_ID="${NODE_ID:-$(hostname)}"
NODE_NAME="${NODE_NAME:-$NODE_ID}"
INTERVAL="${INTERVAL:-1}"
API_KEY="${API_KEY:-}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --url) URL="$2"; shift 2 ;;
        --node-id) NODE_ID="$2"; shift 2 ;;
        --node-name) NODE_NAME="$2"; shift 2 ;;
        --interval) INTERVAL="$2"; shift 2 ;;
        --api-key) API_KEY="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo "Pushing metrics to $URL"
echo "Node ID: $NODE_ID"
echo "Node Name: $NODE_NAME"
echo "Interval: ${INTERVAL}s"
echo

# Function to get CPU usage
get_cpu_usage() {
    if [ -f /proc/stat ]; then
        # Linux
        local cpu_line=$(head -1 /proc/stat)
        local user=$(echo "$cpu_line" | awk '{print $2}')
        local nice=$(echo "$cpu_line" | awk '{print $3}')
        local system=$(echo "$cpu_line" | awk '{print $4}')
        local idle=$(echo "$cpu_line" | awk '{print $5}')
        local total=$((user + nice + system + idle))

        if [ -n "$PREV_TOTAL" ] && [ "$PREV_TOTAL" != "$total" ]; then
            local diff_total=$((total - PREV_TOTAL))
            local diff_user=$((user - PREV_USER))
            local diff_system=$((system - PREV_SYSTEM))
            local diff_idle=$((idle - PREV_IDLE))

            CPU_USER=$(echo "scale=2; $diff_user * 100 / $diff_total" | bc)
            CPU_SYSTEM=$(echo "scale=2; $diff_system * 100 / $diff_total" | bc)
            CPU_IDLE=$(echo "scale=2; $diff_idle * 100 / $diff_total" | bc)
        else
            CPU_USER="0"
            CPU_SYSTEM="0"
            CPU_IDLE="100"
        fi

        PREV_TOTAL=$total
        PREV_USER=$user
        PREV_SYSTEM=$system
        PREV_IDLE=$idle
    elif command -v top &> /dev/null; then
        # macOS/BSD
        local cpu_info=$(top -l 1 | grep "CPU usage" | head -1)
        CPU_USER=$(echo "$cpu_info" | awk '{print $3}' | tr -d '%')
        CPU_SYSTEM=$(echo "$cpu_info" | awk '{print $5}' | tr -d '%')
        CPU_IDLE=$(echo "$cpu_info" | awk '{print $7}' | tr -d '%')
    else
        CPU_USER="0"
        CPU_SYSTEM="0"
        CPU_IDLE="100"
    fi
}

# Function to get memory usage
get_memory_usage() {
    if [ -f /proc/meminfo ]; then
        # Linux
        local mem_total=$(grep MemTotal /proc/meminfo | awk '{print $2}')
        local mem_free=$(grep MemFree /proc/meminfo | awk '{print $2}')
        local mem_available=$(grep MemAvailable /proc/meminfo | awk '{print $2}')
        local mem_cached=$(grep "^Cached:" /proc/meminfo | awk '{print $2}')
        local mem_buffers=$(grep Buffers /proc/meminfo | awk '{print $2}')

        MEM_USED=$(echo "scale=2; ($mem_total - $mem_available) / 1024" | bc)
        MEM_CACHED=$(echo "scale=2; $mem_cached / 1024" | bc)
        MEM_BUFFERS=$(echo "scale=2; $mem_buffers / 1024" | bc)
        MEM_FREE=$(echo "scale=2; $mem_available / 1024" | bc)
    else
        # macOS/BSD
        local page_size=$(sysctl -n hw.pagesize 2>/dev/null || echo 4096)
        local pages_free=$(sysctl -n vm.page_free_count 2>/dev/null || echo 0)
        local mem_total=$(sysctl -n hw.memsize 2>/dev/null || echo 0)

        MEM_FREE=$(echo "scale=2; $pages_free * $page_size / 1024 / 1024" | bc)
        MEM_USED=$(echo "scale=2; ($mem_total / 1024 / 1024) - $MEM_FREE" | bc)
        MEM_CACHED="0"
        MEM_BUFFERS="0"
    fi
}

# Function to get load average
get_load_average() {
    if [ -f /proc/loadavg ]; then
        # Linux
        read LOAD1 LOAD5 LOAD15 _ < /proc/loadavg
    else
        # macOS/BSD
        local load=$(sysctl -n vm.loadavg 2>/dev/null | tr -d '{}')
        LOAD1=$(echo "$load" | awk '{print $1}')
        LOAD5=$(echo "$load" | awk '{print $2}')
        LOAD15=$(echo "$load" | awk '{print $3}')
    fi

    LOAD1=${LOAD1:-0}
    LOAD5=${LOAD5:-0}
    LOAD15=${LOAD15:-0}
}

# Build JSON payload
build_payload() {
    local timestamp=$(($(date +%s) * 1000))
    local os_info="$(uname -s) $(uname -r)"

    cat <<EOF
{
  "node_id": "$NODE_ID",
  "node_name": "$NODE_NAME",
  "hostname": "$(hostname)",
  "os": "$os_info",
  "timestamp": $timestamp,
  "charts": [
    {
      "id": "system.cpu",
      "type": "system",
      "title": "CPU Usage",
      "units": "%",
      "family": "cpu",
      "chart_type": "stacked",
      "priority": 100,
      "dimensions": [
        {"id": "user", "name": "User", "value": ${CPU_USER:-0}},
        {"id": "system", "name": "System", "value": ${CPU_SYSTEM:-0}},
        {"id": "idle", "name": "Idle", "value": ${CPU_IDLE:-100}}
      ]
    },
    {
      "id": "system.memory",
      "type": "system",
      "title": "Memory Usage",
      "units": "MiB",
      "family": "memory",
      "chart_type": "stacked",
      "priority": 200,
      "dimensions": [
        {"id": "used", "name": "Used", "value": ${MEM_USED:-0}},
        {"id": "cached", "name": "Cached", "value": ${MEM_CACHED:-0}},
        {"id": "buffers", "name": "Buffers", "value": ${MEM_BUFFERS:-0}},
        {"id": "free", "name": "Free", "value": ${MEM_FREE:-0}}
      ]
    },
    {
      "id": "system.load",
      "type": "system",
      "title": "System Load Average",
      "units": "load",
      "family": "load",
      "chart_type": "line",
      "priority": 150,
      "dimensions": [
        {"id": "load1", "name": "1 min", "value": ${LOAD1:-0}},
        {"id": "load5", "name": "5 min", "value": ${LOAD5:-0}},
        {"id": "load15", "name": "15 min", "value": ${LOAD15:-0}}
      ]
    }
  ]
}
EOF
}

# Push metrics
push_metrics() {
    local headers=(-H "Content-Type: application/json")
    if [ -n "$API_KEY" ]; then
        headers+=(-H "X-API-Key: $API_KEY")
    fi

    local payload=$(build_payload)

    local response=$(curl -s -w "%{http_code}" -o /dev/null \
        "${headers[@]}" \
        -X POST \
        -d "$payload" \
        "${URL}/api/v1/push")

    if [ "$response" = "200" ]; then
        echo "[$(date +%H:%M:%S)] Metrics pushed successfully"
    else
        echo "[$(date +%H:%M:%S)] Failed to push metrics (HTTP $response)"
    fi
}

# Main loop
echo "Starting metrics collection..."
while true; do
    get_cpu_usage
    get_memory_usage
    get_load_average
    push_metrics
    sleep "$INTERVAL"
done
