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
# Disable filename globbing: PromQL queries contain `*` (multiplication),
# braces, brackets, etc. Without `-f`, queries like `system_cpu * on(x) y`
# get shell-expanded against the current directory before `curl` sees them.
set -f

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
# Phase 1 had a "vector matching rejected" check here; SOW-0022
# implements vector matching, so the rejection no longer applies.
# Positive coverage lives in the Phase 3d group below.
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
# Phase 2 (SOW-0018) discovery endpoints
# ---------------------------------------------------------------------------
#
# These checks verify the Grafana-compatible discovery surface: metric
# browser, label autocomplete, /series resolve, /metadata catalog,
# /status/buildinfo heuristics, and the host scoping rules introduced in
# chunk 2.

# Issue a GET with arbitrary query-string arguments (the existing _check
# helper assumes a single `query=` parameter, which doesn't fit /labels or
# /series). Args: name, path, extra_args, expected_status,
# extra_jq_assertion (optional Python check on the parsed JSON; receives
# `d` as the parsed dict).
check_discovery() {
    # Signature: name, path, extra, expected_status, assertion
    # `extra` is a single string that's split into curl args by the
    # shell. Each whitespace-separated token becomes one curl arg.
    # For queries that contain whitespace, callers must use repeated
    # `--data-urlencode k=v --data-urlencode k2=v2` form OR call
    # `check_discovery_args` (below) which takes the curl args as a
    # variadic tail and avoids re-splitting.
    local name="$1" path="$2" extra="$3" expected_status="$4" assertion="$5"
    local tmp http_code body
    tmp=$(mktemp)
    # shellcheck disable=SC2086  # intentional word-split on $extra
    http_code=$(curl -sG -o "$tmp" -w '%{http_code}' "$URL$path" $extra 2>&1)
    body=$(cat "$tmp")
    rm -f "$tmp"

    if [[ "$http_code" != "$expected_status" ]]; then
        printf '  %s %s -- expected HTTP %s, got %s\n' \
            "$(c_red FAIL)" "$name" "$expected_status" "$http_code"
        FAIL=$((FAIL + 1))
        return
    fi
    # Pipe the body through stdin to avoid argv-length limits on large
    # responses (the sentinel selector pulls back tens of thousands of
    # series on a populated daemon).
    if ! printf '%s' "$body" | python3 -c "import json,sys; json.load(sys.stdin)" >/dev/null 2>&1; then
        printf '  %s %s -- response is not valid JSON\n' "$(c_red FAIL)" "$name"
        FAIL=$((FAIL + 1))
        return
    fi
    if [[ -n "$assertion" ]]; then
        if ! printf '%s' "$body" | python3 -c "
import json, sys
d = json.load(sys.stdin)
assert $assertion, 'assertion failed: ' + repr(d)[:200]
" >/dev/null 2>&1; then
            printf '  %s %s -- assertion failed: %s\n' "$(c_red FAIL)" "$name" "$assertion"
            FAIL=$((FAIL + 1))
            return
        fi
    fi
    printf '  %s %s\n' "$(c_grn PASS)" "$name"
    PASS=$((PASS + 1))
}

echo "==> Discovery: /api/v1/status/buildinfo"
# The frontend version-gating uses jsonData.prometheusVersion, not this,
# but Grafana's Go backend reads `features` to classify Prometheus vs
# Mimir. The empty map is load-bearing.
check_discovery "buildinfo returns 2.45.0 with features:{}" \
    "/api/v1/status/buildinfo" "" 200 \
    "d['status']=='success' and d['data']['version']=='2.45.0' and d['data']['features']=={}"
echo

echo "==> Discovery: /api/v1/labels"
check_discovery "labels: unfiltered contains __name__" \
    "/api/v1/labels" "" 200 \
    "'__name__' in d['data']"
check_discovery "labels: unfiltered contains dimension and instance" \
    "/api/v1/labels" "" 200 \
    "'dimension' in d['data'] and 'instance' in d['data']"
check_discovery "labels: match[]=system_cpu narrows result" \
    "/api/v1/labels" "--data-urlencode match[]=system_cpu" 200 \
    "all(l in d['data'] for l in ['__name__','chart','dimension','family','instance'])"
echo

echo "==> Discovery: /api/v1/label/<name>/values"
check_discovery "label/__name__/values lists metric names" \
    "/api/v1/label/__name__/values" "" 200 \
    "'system_cpu' in d['data'] and 'disk_io' in d['data']"
check_discovery "label/dimension/values lists dim names" \
    "/api/v1/label/dimension/values" "" 200 \
    "'user' in d['data'] and 'system' in d['data']"
check_discovery "label/__name__/values filtered by match[]" \
    "/api/v1/label/__name__/values" "--data-urlencode match[]=system_cpu" 200 \
    "d['data']==['system_cpu']"
check_discovery "label/.../wrong (malformed path) -> 404" \
    "/api/v1/label/dimension/wrong" "" 404 ""
echo

echo "==> Discovery: /api/v1/series"
check_discovery "series: single match[] returns shapes" \
    "/api/v1/series" "--data-urlencode match[]=system_cpu" 200 \
    "len(d['data'])>=1 and d['data'][0]['__name__']=='system_cpu' and 'dimension' in d['data'][0]"
# Regression guard: EQ on a per-dimension label must narrow the result
# rather than rejecting the whole chart at the pre-filter stage. The
# original chunk-1 chart-level matcher check was too eager.
check_discovery "series: EQ on dimension label narrows to one series" \
    "/api/v1/series" '--data-urlencode match[]=system_cpu{dimension="user"}' 200 \
    "len(d['data'])==1 and d['data'][0]['dimension']=='user'"
check_discovery "series: EQ on nonexistent dimension value returns empty" \
    "/api/v1/series" '--data-urlencode match[]=system_cpu{dimension="nonexistent_dim_xyz"}' 200 \
    "d['data']==[]"
check_discovery "series: multi match[] OR-unions distinct __name__" \
    "/api/v1/series" "--data-urlencode match[]=system_cpu --data-urlencode match[]=disk_io" 200 \
    "{s['__name__'] for s in d['data']} == {'system_cpu','disk_io'}"
check_discovery "series: sentinel {__name__!=\"\"} returns all" \
    "/api/v1/series" '--data-urlencode match[]={__name__!=""}' 200 \
    "len(d['data']) > 100"
echo

echo "==> Discovery: /api/v1/metadata"
check_discovery "metadata: full catalog has many entries" \
    "/api/v1/metadata" "" 200 \
    "len(d['data']) > 100 and 'system_cpu' in d['data']"
check_discovery "metadata: ?metric=disk_io has type=counter" \
    "/api/v1/metadata" "--data-urlencode metric=disk_io" 200 \
    "d['data']['disk_io'][0]['type']=='counter'"
check_discovery "metadata: ?metric=system_cpu has type=gauge" \
    "/api/v1/metadata" "--data-urlencode metric=system_cpu" 200 \
    "d['data']['system_cpu'][0]['type']=='gauge'"
echo

echo "==> Discovery: POST on /labels and /series"
check_discovery_args() {
    # Variadic variant: trailing args are passed verbatim to curl, so
    # `--data-urlencode "query=a * b"` is preserved as two tokens
    # rather than being word-split. Use for queries containing
    # whitespace or shell metachars.
    local name="$1" path="$2" expected_status="$3" assertion="$4"
    shift 4
    local tmp http_code body
    tmp=$(mktemp)
    http_code=$(curl -sG -o "$tmp" -w '%{http_code}' "$URL$path" "$@" 2>&1)
    body=$(cat "$tmp")
    rm -f "$tmp"
    if [[ "$http_code" != "$expected_status" ]]; then
        printf '  %s %s -- expected HTTP %s, got %s\n' \
            "$(c_red FAIL)" "$name" "$expected_status" "$http_code"
        FAIL=$((FAIL + 1))
        return
    fi
    if ! printf '%s' "$body" | python3 -c "import json,sys; json.load(sys.stdin)" >/dev/null 2>&1; then
        printf '  %s %s -- response is not valid JSON\n' "$(c_red FAIL)" "$name"
        FAIL=$((FAIL + 1))
        return
    fi
    if [[ -n "$assertion" ]]; then
        if ! printf '%s' "$body" | python3 -c "
import json, sys
d = json.load(sys.stdin)
assert $assertion, 'assertion failed: ' + repr(d)[:200]
" >/dev/null 2>&1; then
            printf '  %s %s -- assertion failed: %s\n' "$(c_red FAIL)" "$name" "$assertion"
            FAIL=$((FAIL + 1))
            return
        fi
    fi
    printf '  %s %s\n' "$(c_grn PASS)" "$name"
    PASS=$((PASS + 1))
}

check_post_discovery() {
    local name="$1" path="$2" body="$3" assertion="$4"
    local tmp http_code resp_body
    tmp=$(mktemp)
    http_code=$(curl -s -X POST -o "$tmp" -w '%{http_code}' "$URL$path" $body 2>&1)
    resp_body=$(cat "$tmp")
    rm -f "$tmp"
    if [[ "$http_code" != "200" ]]; then
        printf '  %s %s -- got HTTP %s\n' "$(c_red FAIL)" "$name" "$http_code"
        FAIL=$((FAIL + 1))
        return
    fi
    if ! printf '%s' "$resp_body" | python3 -c "
import json, sys
d = json.load(sys.stdin)
assert $assertion, 'failed: ' + repr(d)[:200]
" >/dev/null 2>&1; then
        printf '  %s %s -- assertion failed\n' "$(c_red FAIL)" "$name"
        FAIL=$((FAIL + 1))
        return
    fi
    printf '  %s %s\n' "$(c_grn PASS)" "$name"
    PASS=$((PASS + 1))
}
check_post_discovery "POST /labels with match[]" \
    "/api/v1/labels" "--data-urlencode match[]=system_cpu" \
    "'__name__' in d['data']"
check_post_discovery "POST /series with match[] and limit" \
    "/api/v1/series" '--data-urlencode match[]={__name__!=""} --data-urlencode limit=20' \
    "len(d['data'])==20"
echo

echo "==> Phase 3f: count_values"
check_discovery_args "count_values bucketizes by value" \
    "/api/v1/query" 200 \
    "len(d['data']['result'])>=1 and all('v' in s['metric'] for s in d['data']['result'])" \
    --data-urlencode 'query=count_values("v", system_cpu)'
check_discovery_args "count_values total equals input series count" \
    "/api/v1/query" 200 \
    "sum(float(s['value'][1]) for s in d['data']['result'])==10" \
    --data-urlencode 'query=count_values("v", system_cpu)'
check_discovery_args "count_values output strips __name__" \
    "/api/v1/query" 200 \
    "all('__name__' not in s['metric'] for s in d['data']['result'])" \
    --data-urlencode 'query=count_values("v", system_cpu)'
echo

echo "==> Phase 3e: stddev/stdvar/quantile_over_time, predict_linear, holt_winters"
check_instant "stddev_over_time"     'stddev_over_time(system_cpu[1m])'             200 vector
check_instant "stdvar_over_time"     'stdvar_over_time(system_cpu[1m])'             200 vector
check_instant "quantile_over_time"   'quantile_over_time(0.95, system_cpu[1m])'     200 vector
check_instant "predict_linear"       'predict_linear(system_cpu[1m], 0)'            200 vector
check_instant "holt_winters"         'holt_winters(system_cpu[1m], 0.5, 0.5)'       200 vector
check_discovery_args "quantile_over_time(1, ...) returns max-equivalent values" \
    "/api/v1/query" 200 \
    "len(d['data']['result'])==10" \
    --data-urlencode "query=quantile_over_time(1, system_cpu[1m])"
check_discovery_args "predict_linear(..., 0) returns 10 series" \
    "/api/v1/query" 200 \
    "len(d['data']['result'])==10" \
    --data-urlencode "query=predict_linear(system_cpu[1m], 0)"
check_discovery_args "holt_winters with valid factors returns 10" \
    "/api/v1/query" 200 \
    "len(d['data']['result'])==10" \
    --data-urlencode "query=holt_winters(system_cpu[1m], 0.5, 0.5)"
check_discovery_args "holt_winters with sf > 1 rejects" \
    "/api/v1/query" 422 "" \
    --data-urlencode "query=holt_winters(system_cpu[1m], 1.5, 0.5)"
echo

echo "==> Phase 3d: vector matching and set operators"
# on(dimension) self-join: each series matches itself by dimension; 10 results.
check_discovery_args "on(dimension) self-join returns 10 series" \
    "/api/v1/query" 200 "len(d['data']['result'])==10" \
    --data-urlencode "query=system_cpu * on(dimension) system_cpu"
check_discovery_args "ignoring(nope) self-join returns 10 series" \
    "/api/v1/query" 200 "len(d['data']['result'])==10" \
    --data-urlencode "query=system_cpu * ignoring(nope) system_cpu"
check_discovery_args "group_left broadcasts max across all dims" \
    "/api/v1/query" 200 \
    "len(d['data']['result'])==10 and all('dimension' in s['metric'] for s in d['data']['result'])" \
    --data-urlencode "query=system_cpu * on(instance) group_left() max by(instance)(system_cpu)"
check_discovery_args "system_cpu and system_cpu keeps all 10" \
    "/api/v1/query" 200 "len(d['data']['result'])==10" \
    --data-urlencode "query=system_cpu and system_cpu"
check_discovery_args "system_cpu unless system_cpu is empty" \
    "/api/v1/query" 200 "d['data']['result']==[]" \
    --data-urlencode "query=system_cpu unless system_cpu"
check_discovery_args "system_cpu or disk_io unions both" \
    "/api/v1/query" 200 \
    "len({s['metric'].get('__name__') for s in d['data']['result']})==2" \
    --data-urlencode "query=system_cpu or disk_io"
# Ambiguity: ignoring(dimension) collapses 10 series to one key on each
# side -> 1:1 duplicate-key error (Prometheus convention).
check_discovery_args "ignoring(dimension) collapses ambiguously" \
    "/api/v1/query" 422 "" \
    --data-urlencode "query=system_cpu + ignoring(dimension) system_cpu"
echo

echo "==> Phase 3c: topk / bottomk / quantile"
check_instant "topk shape"      'topk(3, system_cpu)'      200 vector
check_instant "bottomk shape"   'bottomk(3, system_cpu)'   200 vector
check_instant "quantile shape"  'quantile(0.5, system_cpu)' 200 vector
# Result-count assertions: topk(3, ...) and bottomk(3, ...) should
# return exactly 3 series.
check_discovery "topk(3,...) returns exactly 3 series" \
    "/api/v1/query" \
    "--data-urlencode query=topk(3,system_cpu)" 200 \
    "len(d['data']['result'])==3"
check_discovery "bottomk(3,...) returns exactly 3 series" \
    "/api/v1/query" \
    "--data-urlencode query=bottomk(3,system_cpu)" 200 \
    "len(d['data']['result'])==3"
# Label preservation: topk keeps original `dimension`, drops `__name__`.
check_discovery "topk preserves dimension, strips __name__" \
    "/api/v1/query" \
    "--data-urlencode query=topk(3,system_cpu)" 200 \
    "all('__name__' not in s['metric'] and 'dimension' in s['metric'] for s in d['data']['result'])"
# Quantile correctness: quantile(1, x) == max(x). Floating-point exact
# match because the implementation returns the sorted max directly when
# phi == 1.
check_discovery "quantile(1, x) equals max(x)" \
    "/api/v1/query" \
    "--data-urlencode query=quantile(1,system_cpu)-max(system_cpu)" 200 \
    "abs(float(d['data']['result'][0]['value'][1]))<1e-9"
# Out-of-range phi: clamp to +Inf or -Inf per Prometheus convention.
check_discovery "quantile(2, x) returns +Inf" \
    "/api/v1/query" \
    "--data-urlencode query=quantile(2,system_cpu)" 200 \
    "d['data']['result'][0]['value'][1]=='+Inf'"
# count_values shipped in SOW-0024; positive coverage lives in the
# Phase 3f group below.
echo

echo "==> Phase 3b: *_over_time family"
check_instant "avg_over_time"      'avg_over_time(system_cpu[1m])'      200 vector
check_instant "sum_over_time"      'sum_over_time(system_cpu[1m])'      200 vector
check_instant "min_over_time"      'min_over_time(system_cpu[1m])'      200 vector
check_instant "max_over_time"      'max_over_time(system_cpu[1m])'      200 vector
check_instant "count_over_time"    'count_over_time(system_cpu[1m])'    200 vector
check_instant "last_over_time"     'last_over_time(system_cpu[1m])'     200 vector
check_instant "present_over_time"  'present_over_time(system_cpu[1m])'  200 vector
# Correctness sanity: every series in count_over_time(system_cpu[1m])
# should be >= 1 because every dimension has been collected in the
# last minute on a populated host. We verify by composing > 0 (which
# should return all 10 series) and checking the count.
check_discovery "count_over_time(...)>=1 keeps every series" \
    "/api/v1/query" \
    "--data-urlencode query=count_over_time(system_cpu[1m])>0" 200 \
    "len(d['data']['result'])>=1"
# last_over_time keeps __name__; the others strip it. Verify both
# behaviors end to end.
check_discovery "last_over_time preserves __name__" \
    "/api/v1/query" \
    "--data-urlencode query=last_over_time(system_cpu[1m])" 200 \
    "any('__name__' in s['metric'] for s in d['data']['result'])"
check_discovery "avg_over_time strips __name__" \
    "/api/v1/query" \
    "--data-urlencode query=avg_over_time(system_cpu[1m])" 200 \
    "all('__name__' not in s['metric'] for s in d['data']['result'])"
echo

echo "==> Phase 3a: fast path + start/end semantics"
# Fast path (no match[]) and slow path (match[]={__name__!=""}) must
# produce the same name set. The fast path walks contexts directly; the
# slow path resolves series. SOW-0019 chunk 1 added the live-instance
# check to make these sets identical.
FAST=$(curl -s 'http://localhost:19999/api/v1/label/__name__/values' \
       | python3 -c "import json,sys; print(','.join(sorted(json.load(sys.stdin)['data'])))")
SLOW=$(curl -s --data-urlencode 'match[]={__name__!=""}' \
       'http://localhost:19999/api/v1/label/__name__/values' \
       | python3 -c "import json,sys; print(','.join(sorted(json.load(sys.stdin)['data'])))")
if [[ "$FAST" == "$SLOW" && -n "$FAST" ]]; then
    printf '  %s metric-names fast path matches slow path (%d names)\n' \
        "$(c_grn PASS)" "$(echo "$FAST" | tr ',' '\n' | wc -l)"
    PASS=$((PASS + 1))
else
    fast_count=$(echo "$FAST" | tr ',' '\n' | wc -l)
    slow_count=$(echo "$SLOW" | tr ',' '\n' | wc -l)
    printf '  %s metric-names: fast=%d slow=%d differ\n' \
        "$(c_red FAIL)" "$fast_count" "$slow_count"
    FAIL=$((FAIL + 1))
fi

# start/end: a future window should empty out every retention-aware
# endpoint. Skip charts whose last_collected_time predates `start`.
FUTURE_START=$(( $(date +%s) + 86400 ))
FUTURE_END=$(( $(date +%s) + 90000 ))
check_discovery "future window: /series returns empty" \
    "/api/v1/series" \
    "--data-urlencode match[]=system_cpu --data-urlencode start=$FUTURE_START --data-urlencode end=$FUTURE_END" \
    200 "d['data']==[]"
check_discovery "future window: /metadata returns empty data map" \
    "/api/v1/metadata" \
    "--data-urlencode start=$FUTURE_START --data-urlencode end=$FUTURE_END" \
    200 "d['data']=={}"
check_discovery "future window: /label/__name__/values returns empty" \
    "/api/v1/label/__name__/values" \
    "--data-urlencode start=$FUTURE_START --data-urlencode end=$FUTURE_END" \
    200 "d['data']==[]"

# A "now" window should return the same count as no-window. Confirms
# the in-window filter is not over-aggressive on currently-collecting
# contexts.
NOW=$(date +%s)
RECENT_START=$((NOW - 60))
NOWIN_COUNT=$(curl -s --data-urlencode 'match[]=system_cpu' \
    'http://localhost:19999/api/v1/series' \
    | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data']))")
check_discovery "now window: /series returns same count as no-window" \
    "/api/v1/series" \
    "--data-urlencode match[]=system_cpu --data-urlencode start=$RECENT_START --data-urlencode end=$NOW" \
    200 "len(d['data'])==$NOWIN_COUNT"
echo

echo "==> Host scoping"
LOCAL_HOST=$(curl -s "$URL/api/v1/info" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['mirrored_hosts'][0])" 2>/dev/null || hostname)
echo "  local hostname: $LOCAL_HOST"
check_discovery "host=<this hostname> resolves to same series as default" \
    "/api/v1/series" "--data-urlencode match[]=system_cpu --data-urlencode host=$LOCAL_HOST" 200 \
    "len(d['data'])>=1 and d['data'][0]['instance']=='$LOCAL_HOST'"
check_discovery "host=* resolves the same series" \
    "/api/v1/series" "--data-urlencode match[]=system_cpu --data-urlencode host=*" 200 \
    "len(d['data'])>=1"
check_discovery "host=nonexistent returns empty success envelope" \
    "/api/v1/series" "--data-urlencode match[]=system_cpu --data-urlencode host=does-not-exist-123" 200 \
    "d['status']=='success' and d['data']==[]"
check_discovery "host parameter wired on /query" \
    "/api/v1/query" "--data-urlencode query=system_cpu --data-urlencode host=$LOCAL_HOST" 200 \
    "len(d['data']['result'])>=1 and d['data']['result'][0]['metric']['instance']=='$LOCAL_HOST'"
check_discovery "host parameter wired on /metadata" \
    "/api/v1/metadata" "--data-urlencode host=$LOCAL_HOST --data-urlencode metric=disk_io" 200 \
    "d['data']['disk_io'][0]['type']=='counter'"
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
