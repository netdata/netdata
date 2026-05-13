#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# PromQL endpoint smoke test harness for SOW-0017 Phase 1.
#
# Runs the queries listed in Acceptance Criterion #4 against a live
# Netdata daemon. Each check verifies:
#   - HTTP status matches expected
#   - response is valid JSON
#   - `status` field is success or error as expected
#   - `data.resultType` is vector / matrix / scalar as expected
#   - structural shape (non-empty result where data should exist)
#
# Usage:
#   tests/promql-smoke/run-smoke.sh [URL]
#
# URL defaults to http://localhost:19999. Exit code is 0 on success, 1
# on any failure. The harness does not assert numeric values -- it
# verifies the contract shape. Numeric correctness is covered by the
# Rust unit tests under src/crates/netdata_promql/.

set -euo pipefail

URL="${1:-http://localhost:19999}"
PASS=0
FAIL=0

c_red()  { printf '\033[31m%s\033[0m' "$*"; }
c_grn()  { printf '\033[32m%s\033[0m' "$*"; }
c_yel()  { printf '\033[33m%s\033[0m' "$*"; }

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Issue an instant query. Args: name, query, expected_status, expected_result_type
check_instant() {
    local name="$1" query="$2" expected_status="$3" expected_result_type="$4"
    _check "$name" "/api/v3/promql/query" "$query" "" "$expected_status" "$expected_result_type"
}

# Issue an instant query against the Prometheus mirror path.
check_instant_v1() {
    local name="$1" query="$2" expected_status="$3" expected_result_type="$4"
    _check "$name" "/api/v1/query" "$query" "" "$expected_status" "$expected_result_type"
}

# Issue a range query. Args: name, query, range_args, expected_status, expected_result_type
check_range() {
    local name="$1" query="$2" range_args="$3" expected_status="$4" expected_result_type="$5"
    _check "$name" "/api/v3/promql/query_range" "$query" "$range_args" "$expected_status" "$expected_result_type"
}

_check() {
    local name="$1" path="$2" query="$3" extra="$4" expected_status="$5" expected_result_type="$6"
    local resp http_code body
    resp=$(curl -sG -w '\n__HTTP__:%{http_code}\n' "$URL$path" \
        --data-urlencode "query=$query" \
        $extra 2>&1) || true
    http_code=$(printf '%s' "$resp" | awk -F: '/^__HTTP__:/ { print $2 }')
    body=$(printf '%s' "$resp" | awk '/^__HTTP__:/{exit} {print}')

    if [[ "$http_code" != "$expected_status" ]]; then
        printf '  %s %s -- expected HTTP %s, got %s\n' \
            "$(c_red FAIL)" "$name" "$expected_status" "$http_code"
        FAIL=$((FAIL + 1))
        return
    fi

    # Parse JSON via python3 to keep deps minimal.
    local status result_type
    if ! python3 -c "import json,sys; json.loads('''$body''')" >/dev/null 2>&1; then
        printf '  %s %s -- response is not valid JSON\n' "$(c_red FAIL)" "$name"
        FAIL=$((FAIL + 1))
        return
    fi
    status=$(printf '%s' "$body" | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo '')
    if [[ "$expected_status" == "200" ]]; then
        if [[ "$status" != "success" ]]; then
            printf '  %s %s -- status=%s (expected success)\n' \
                "$(c_red FAIL)" "$name" "$status"
            FAIL=$((FAIL + 1))
            return
        fi
        result_type=$(printf '%s' "$body" | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('resultType',''))" 2>/dev/null || echo '')
        if [[ "$expected_result_type" != "" && "$result_type" != "$expected_result_type" ]]; then
            printf '  %s %s -- resultType=%s (expected %s)\n' \
                "$(c_red FAIL)" "$name" "$result_type" "$expected_result_type"
            FAIL=$((FAIL + 1))
            return
        fi
    else
        if [[ "$status" != "error" ]]; then
            printf '  %s %s -- expected error envelope, got status=%s\n' \
                "$(c_red FAIL)" "$name" "$status"
            FAIL=$((FAIL + 1))
            return
        fi
    fi

    printf '  %s %s\n' "$(c_grn PASS)" "$name"
    PASS=$((PASS + 1))
}

# ---------------------------------------------------------------------------
# Reachability
# ---------------------------------------------------------------------------

echo "==> Verifying daemon at $URL"
if ! curl -sf -m 5 "$URL/api/v1/info" -o /dev/null; then
    echo "Daemon at $URL is not responding. Start netdata first."
    exit 2
fi
echo "$(c_grn OK) daemon reachable"
echo

# ---------------------------------------------------------------------------
# Acceptance Criterion #4 corpus
# ---------------------------------------------------------------------------

echo "==> Instant queries (Netdata-namespaced paths)"
check_instant "scalar literal"            '42'                                      200 scalar
check_instant "vector selector by name"   'system_cpu'                              200 vector
check_instant "matched dimension label"   'system_cpu{dimension="user"}'            200 vector
check_instant "regex matcher"             'system_cpu{dimension=~"user|system"}'    200 vector
check_instant "sum by dimension"          'sum by (dimension) (system_cpu)'         200 vector
check_instant "comparison filter"         'system_cpu > 50'                         200 vector
check_instant "scalar arithmetic"         'system_cpu * 2'                          200 vector
check_instant "offset"                    'system_cpu offset 30s'                   200 vector
echo

echo "==> Counter family (against disk_io, which Netdata models as INCREMENTAL)"
check_instant "rate"                      'rate(disk_io[2m])'                       200 vector
check_instant "irate"                     'irate(disk_io[2m])'                      200 vector
check_instant "increase"                  'increase(disk_io[1m])'                   200 vector
check_instant "delta on gauge"            'delta(system_cpu[1m])'                   200 vector
echo

echo "==> Composition"
check_instant "sum by + rate"             'sum by (dimension) (rate(disk_io[2m]))'  200 vector
echo

echo "==> Error paths"
check_instant "parse error"               'not_a_real_query{'                       400 ""
check_instant "subquery rejected"         'foo[5m:1m]'                              400 ""
check_instant "vector matching rejected"  'a + on(x) b'                             400 ""
check_instant "histogram_quantile no le"  'histogram_quantile(0.95, system_cpu)'    422 ""
echo

echo "==> Prometheus mirror paths produce identical shapes"
check_instant_v1 "v1 scalar"              '42'                                      200 scalar
check_instant_v1 "v1 vector"              'system_cpu'                              200 vector
echo

# POST is what Grafana uses for queries past a certain length. The handler
# should accept form-encoded bodies the same way it accepts URL params.
check_post() {
    local name="$1" path="$2" query="$3" expected_status="$4" expected_result_type="$5"
    local resp http_code body
    resp=$(curl -s -X POST -w '\n__HTTP__:%{http_code}\n' "$URL$path" \
        --data-urlencode "query=$query" 2>&1) || true
    http_code=$(printf '%s' "$resp" | awk -F: '/^__HTTP__:/ { print $2 }')
    body=$(printf '%s' "$resp" | awk '/^__HTTP__:/{exit} {print}')
    if [[ "$http_code" != "$expected_status" ]]; then
        printf '  %s %s -- expected HTTP %s, got %s\n' \
            "$(c_red FAIL)" "$name" "$expected_status" "$http_code"
        FAIL=$((FAIL + 1))
        return
    fi
    local rt
    rt=$(printf '%s' "$body" | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('resultType',''))" 2>/dev/null || echo '')
    if [[ "$expected_result_type" != "" && "$rt" != "$expected_result_type" ]]; then
        printf '  %s %s -- resultType=%s (expected %s)\n' \
            "$(c_red FAIL)" "$name" "$rt" "$expected_result_type"
        FAIL=$((FAIL + 1))
        return
    fi
    printf '  %s %s\n' "$(c_grn PASS)" "$name"
    PASS=$((PASS + 1))
}

echo "==> POST method (Grafana long-query path)"
check_post "POST v1 scalar"               "/api/v1/query"        '42'                       200 scalar
check_post "POST v1 vector"               "/api/v1/query"        'system_cpu'               200 vector
check_post "POST v3 with encoded payload" "/api/v3/promql/query" 'rate(disk_io[2m])'        200 vector
echo

echo "==> Range queries"
NOW=$(date +%s)
START=$((NOW - 60))
check_range "range over 60s"  'system_cpu' \
    "--data-urlencode start=$START --data-urlencode end=$NOW --data-urlencode step=15" \
    200 matrix
check_range "range range vector at top level (must fail)"  'system_cpu[5m]' \
    "--data-urlencode start=$START --data-urlencode end=$NOW --data-urlencode step=15" \
    400 ""
echo

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

echo "----"
printf '%d passed, %d failed\n' "$PASS" "$FAIL"
if [[ "$FAIL" -gt 0 ]]; then
    printf '%s\n' "$(c_red FAIL)"
    exit 1
fi
printf '%s\n' "$(c_grn OK)"
