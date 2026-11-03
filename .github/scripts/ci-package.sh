#!/bin/sh
# Bound the entire invocation, including native retries and multiple downloads.
# GNU and BusyBox timeout are already available on our Linux CI hosts/containers.
if [ "${1-}" != --network-retry ]; then
    case "$1" in
        apt-get)
            shift
            exec timeout -s KILL 900 apt-get -o Acquire::Retries=5 \
                -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 \
                -o APT::Update::Error-Mode=any "$@"
            ;;
        pip|pip3|python|./integrations/pip.sh)
            export PIP_RETRIES=5 PIP_TIMEOUT=30
            exec timeout -s KILL 900 "$@"
            ;;
        cargo)
            export CARGO_NET_RETRY=5 CARGO_HTTP_TIMEOUT=30
            exec timeout -s KILL 900 "$@"
            ;;
        *) exec timeout -s KILL 900 sh "$0" --network-retry "$@" ;;
    esac
fi
shift

# apk and go mod download lack a configurable network-only retry loop.
# Unknown errors fail immediately; never retry dependency or compilation errors.
log=$(mktemp)
trap 'rm -f "$log"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export LC_ALL=C
attempt=1
while :; do
    if [ "$1" = go ]; then
        # Keep stderr diagnostics out of the structured JSON stdout.
        "$@" -json >"$log"
    else
        "$@" >"$log" 2>&1
    fi
    status=$?
    cat "$log"
    [ "$status" -eq 0 ] && exit 0
    [ "$attempt" -ge 5 ] && exit "$status"
    case "$1" in
        apk) pattern=': (temporary error \(try again later\)|network error|connection timed out)$' ;;
        go) pattern='(dial tcp.*(i/o timeout|no such host|connection refused)|TLS handshake timeout|read: connection reset by peer|: (429 Too Many Requests|50[0234] (Internal Server Error|Bad Gateway|Service Unavailable|Gateway Timeout)))' ;;
        *) exit "$status" ;;
    esac
    if [ "$1" = go ]; then
        # Structured errors let us reject mixed network/permanent failures.
        python3 - "$log" "$pattern" <<'PYTHON' || exit "$status"
import json, re, sys
text = open(sys.argv[1]).read().strip()
errors = []
while text:
    item, end = json.JSONDecoder().raw_decode(text)
    if item.get("Error"):
        errors.append(item["Error"])
    text = text[end:].lstrip()
sys.exit(not errors or any(not re.search(sys.argv[2], error) for error in errors))
PYTHON
    else
        grep -Eq "$pattern" "$log" || exit "$status"
        # apk update also prints an unavailable-index count on fetch failure.
        # Every other dependency/configuration error wins over fetch errors.
        if grep -E '^(ERROR|WARNING):' "$log" | grep -Ev "$pattern|^ERROR: [0-9]+ unavailable, [0-9]+ stale; [0-9]+ distinct packages available\.?$"; then
            exit "$status"
        fi
    fi
    echo "Network failure; retrying in 10 seconds ($attempt/5)." >&2
    sleep 10
    attempt=$((attempt + 1))
done
