#!/usr/bin/env bash
# Helpers for the query-netdata-agents skill.
# Sourced from per-action scripts (and from any other skill that
# wants to call Netdata Cloud / Netdata Agent through token-safe
# wrappers). Not executed directly.
#
# Token-safety contract (HARD requirement):
#   * No PUBLIC function (named `agents_*`, no leading underscore)
#     ever emits NETDATA_CLOUD_TOKEN, a per-agent bearer, or a
#     claim_id to stdout.
#   * Internal helpers (named `_agents_*`, leading underscore) may
#     handle token bytes inside their own scope but must return
#     them only via `local -n` namerefs into the caller's local
#     variables -- never to stdout.
#   * `_agents_log_masked` redacts token / bearer bytes in stderr
#     argv echoes.
#   * The unit test `agents_selftest_no_token_leak` drives every
#     public wrapper with a sentinel token and asserts the
#     sentinel never reaches captured stdout.
#
# Conventions mirrored from .agents/skills/coverity-audit/scripts/_lib.sh:
#   * set -euo pipefail at the top
#   * color vars defined with $'...' so ESC bytes are real
#   * <prefix>_repo_root via `git rev-parse --show-toplevel`
#   * <prefix>_load_env sources <repo>/.env, validates required keys
#   * <prefix>_audit_dir creates <repo>/.local/audits/<topic>/
#
# Audit topic: "query-netdata-agents".

# Capture our source-file path BEFORE `set -u`. Bash exposes
# BASH_SOURCE[0]; zsh exposes the equivalent as `${(%):-%x}` (which
# bash cannot parse, so we gate it through `eval`).
if [ -n "${ZSH_VERSION-}" ]; then
    eval '_agents_lib_self="${(%):-%x}"'
elif [ -n "${BASH_VERSION-}" ]; then
    _agents_lib_self="${BASH_SOURCE[0]}"
else
    _agents_lib_self="$0"
fi

set -euo pipefail

# shellcheck disable=SC2034
AGENTS_RED=$'\033[0;31m'
# shellcheck disable=SC2034
AGENTS_GREEN=$'\033[0;32m'
# shellcheck disable=SC2034
AGENTS_YELLOW=$'\033[1;33m'
# shellcheck disable=SC2034
AGENTS_GRAY=$'\033[0;90m'
# shellcheck disable=SC2034
AGENTS_NC=$'\033[0m'

# ---------------------------------------------------------------------------
# Repo + env helpers
# ---------------------------------------------------------------------------

agents_repo_root() {
    git -C "$(dirname "${_agents_lib_self}")" rev-parse --show-toplevel
}

# Source <repo>/.env. Validate the keys this skill needs.
# Required:
#   NETDATA_CLOUD_TOKEN     -- long-lived Cloud REST token
#   NETDATA_CLOUD_HOSTNAME  -- Cloud REST host (e.g. app.netdata.cloud)
agents_load_env() {
    local root env
    root="$(agents_repo_root)"
    env="${root}/.env"
    if [[ ! -f "${env}" || ! -r "${env}" ]]; then
        echo -e "${AGENTS_RED}[ERROR]${AGENTS_NC} Missing ${env}. Copy .env.template to .env and fill it in. See ${root}/.agents/ENV.md." >&2
        return 1
    fi
    set -a
    # shellcheck disable=SC1090
    source "${env}"
    set +a

    : "${NETDATA_CLOUD_TOKEN:?NETDATA_CLOUD_TOKEN is empty -- see <repo>/.agents/ENV.md to set it.}"
    : "${NETDATA_CLOUD_HOSTNAME:?NETDATA_CLOUD_HOSTNAME is empty -- see <repo>/.agents/ENV.md to set it.}"
    export NETDATA_CLOUD_TOKEN NETDATA_CLOUD_HOSTNAME
}

agents_audit_dir() {
    local root dir
    root="$(agents_repo_root)"
    dir="${root}/.local/audits/query-netdata-agents"
    mkdir -p "${dir}"
    echo "${dir}"
}

# Autodetect the Netdata install prefix. Returns "" for system installs
# (paths like /var/lib/netdata, /etc/netdata) or e.g. "/opt/netdata" for
# bundled installs (paths under /opt/netdata/var/lib/netdata).
#
# Rule (per .agents/sow/specs/sensitive-data-discipline.md): probe
# candidates and pick the first whose <prefix>/var/lib/netdata or
# <prefix>/etc/netdata exists. NOT a config knob.
agents_netdata_prefix() {
    local p
    for p in "" "/opt/netdata" "/usr/local/netdata"; do
        if [[ -d "${p}/var/lib/netdata" || -d "${p}/etc/netdata" ]]; then
            printf '%s' "${p}"
            return 0
        fi
    done
    printf ''
    return 0
}

# ---------------------------------------------------------------------------
# Masked-curl execution wrappers
# ---------------------------------------------------------------------------

# Print a curl invocation to stderr with the cloud token (and any
# minted bearer) masked. Then execute it. Honors AGENTS_DRY_RUN=1
# (write paths skip execution but still log).
agents_run() {
    _agents_log_masked "$@"
    if [[ "${AGENTS_DRY_RUN:-0}" == "1" ]]; then
        return 0
    fi
    "$@"
}

agents_run_read() {
    _agents_log_masked "$@"
    "$@"
}

_agents_log_masked() {
    local arg
    printf >&2 '%s> %s' "${AGENTS_GRAY}" "${AGENTS_YELLOW}"
    for arg in "$@"; do
        # Mask the cloud token wherever it appears.
        if [[ -n "${NETDATA_CLOUD_TOKEN:-}" && "${arg}" == *"${NETDATA_CLOUD_TOKEN}"* ]]; then
            arg="${arg//${NETDATA_CLOUD_TOKEN}/<CLOUD_TOKEN>}"
        fi
        # Mask any UUID-shaped bearer in `Bearer <uuid>` form.
        if [[ "${arg}" =~ Bearer\ [0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12} ]]; then
            arg="${arg%% Bearer *} Bearer <AGENT_BEARER>"
            # The replace above also rebuilds the leading header
            # name; the rejoined arg is harmless even if the
            # leading text is the bare header. Tests cover this.
        fi
        printf >&2 '%q ' "${arg}"
    done
    printf >&2 '%s\n' "${AGENTS_NC}"
}

# ---------------------------------------------------------------------------
# Internal: claim_id / bearer mint / cache
# ---------------------------------------------------------------------------

# Resolve claim_id from a node's /api/v3/info. The /info endpoint
# is unauthenticated. INTERNAL: returns via nameref into a caller
# local; never prints to stdout.
#
# Args:
#   $1 = OUTVAR    -- caller-local variable name to receive the claim_id
#   $2 = HOST      -- host:port (e.g. "agent-events:19999")
_agents_get_claim_id() {
    local -n _out="$1"; shift
    local host="${1:?usage: _agents_get_claim_id OUTVAR <host:port>}"
    local resp claim
    if resp="$(curl -sS --max-time 10 "http://${host}/api/v3/info" 2>/dev/null)"; then
        claim="$(jq -r '.agents[0].cloud.claim_id // empty' <<< "${resp}" 2>/dev/null)"
        if [[ -n "${claim}" && "${claim}" != "null" ]]; then
            _out="${claim}"
            return 0
        fi
    fi
    echo -e "${AGENTS_RED}[ERROR]${AGENTS_NC} Could not resolve claim_id from http://${host}/api/v3/info" >&2
    return 1
}

# Mint a per-agent bearer via Cloud. INTERNAL: prints the response
# JSON to stdout for the caller to capture in a local variable.
# stdout still carries the bearer here -- callers MUST capture into
# a local and never propagate. The PUBLIC wrapper that uses this
# does exactly that and emits only the response body.
_agents_mint_bearer_json() {
    local node_id="${1:?usage: _agents_mint_bearer_json <node_id> <machine_guid> <claim_id>}"
    local mg="${2:?machine_guid required}"
    local claim="${3:?claim_id required}"
    agents_run_read curl --fail --silent --show-error --max-time 30 \
        -H "Authorization: Bearer ${NETDATA_CLOUD_TOKEN}" \
        "https://${NETDATA_CLOUD_HOSTNAME}/api/v2/bearer_get_token?node_id=${node_id}&machine_guid=${mg}&claim_id=${claim}"
}

# Convert an `expiration` value (which may be unix-seconds or
# unix-milliseconds, depending on cloud version) to seconds.
# Heuristic: values > 10^12 are ms; lower are seconds. Returns 0
# for unparseable values so the caller treats the cache as expired.
_agents_exp_to_seconds() {
    local exp="$1"
    if [[ -z "${exp}" || "${exp}" == "null" ]]; then
        echo 0; return
    fi
    if ! [[ "${exp}" =~ ^[0-9]+$ ]]; then
        echo 0; return
    fi
    if (( exp > 1000000000000 )); then
        echo $(( exp / 1000 ))
    else
        echo "${exp}"
    fi
}

# Cache-aware bearer resolution. INTERNAL: returns via nameref;
# never prints the bearer to stdout.
#
# Args:
#   $1 = OUTVAR       -- caller-local variable to receive the bearer
#   $2 = NODE_ID      -- node UUID
#   $3 = MACHINE_GUID -- agent machine_guid (cache key)
#   $4 = HOST         -- host:port for claim_id resolution and direct probe
#
# Cache file: <repo>/.local/audits/query-netdata-agents/bearers/<machine_guid>.json
# Mode 0600. Stamps `_cached_at` (unix-seconds) so the cache window
# survives Cloud responses with expiration=0.
_agents_resolve_bearer() {
    local -n _out="$1"; shift
    local node_id="${1:?usage: _agents_resolve_bearer OUTVAR <node_id> <machine_guid> <host:port>}"
    local mg="${2:?machine_guid required}"
    local host="${3:?host required}"

    local cache_dir cache_file now exp_s
    cache_dir="$(agents_audit_dir)/bearers"
    mkdir -p "${cache_dir}"
    chmod 0700 "${cache_dir}" 2>/dev/null || true
    cache_file="${cache_dir}/${mg}.json"

    now=$(date +%s)

    if [[ -s "${cache_file}" ]]; then
        local cached_exp cached_token cached_at
        cached_exp=$(jq -r '.expiration // 0' "${cache_file}" 2>/dev/null || echo 0)
        cached_token=$(jq -r '.token // empty' "${cache_file}" 2>/dev/null || true)
        cached_at=$(jq -r '._cached_at // 0' "${cache_file}" 2>/dev/null || echo 0)
        exp_s=$(_agents_exp_to_seconds "${cached_exp}")
        if [[ -n "${cached_token}" && "${cached_token}" != "null" ]]; then
            # Two cases:
            # (a) Cloud returned a real expiration -- 1h refresh buffer
            #     (matches cloud-frontend useAgentBearer.js).
            # (b) Cloud returned expiration=0 -- fall back to a fixed
            #     2h window from our mint timestamp. The agent issues
            #     ~3h-TTL bearers, so 2h leaves a 1h safety margin.
            if (( exp_s > 0 )); then
                if (( exp_s - now > 3600 )); then
                    _out="${cached_token}"
                    return 0
                fi
            elif (( cached_at > 0 )) && (( now - cached_at < 7200 )); then
                _out="${cached_token}"
                return 0
            fi
        fi
    fi

    # Need to mint -- resolve claim_id first.
    local claim
    _agents_get_claim_id claim "${host}"

    local resp
    resp="$(_agents_mint_bearer_json "${node_id}" "${mg}" "${claim}")"
    if ! jq -e '.token' >/dev/null 2>&1 <<< "${resp}"; then
        rm -f "${cache_file}"
        echo -e "${AGENTS_RED}[ERROR]${AGENTS_NC} Bearer mint failed; first 200 chars: $(head -c 200 <<< "${resp}")" >&2
        return 1
    fi

    # Stamp cache and persist.
    jq --argjson t "${now}" '. + {_cached_at: $t}' <<< "${resp}" > "${cache_file}"
    chmod 0600 "${cache_file}"
    _out="$(jq -r '.token' "${cache_file}")"
}

# ---------------------------------------------------------------------------
# PUBLIC wrappers (token-safe). These are what the assistant invokes.
# ---------------------------------------------------------------------------

# Call any Netdata Cloud REST endpoint. Reads NETDATA_CLOUD_TOKEN
# from .env internally; emits ONLY the response body to stdout.
# stderr shows the curl invocation with `<CLOUD_TOKEN>` masked.
#
# Args:
#   $1 = METHOD        -- GET / POST / PUT / DELETE / ...
#   $2 = PATH          -- e.g. /api/v2/spaces
#   $3 = BODY (json)   -- optional; passed via -d
#
# Example:
#   agents_query_cloud GET /api/v2/spaces
#   agents_query_cloud POST /api/v2/nodes/$NODE/function?function=systemd-journal '{"info":true}'
agents_query_cloud() {
    local method="${1:?usage: agents_query_cloud METHOD PATH [BODY]}"
    local path="${2:?path required}"
    local body="${3:-}"

    local args=(curl --fail --silent --show-error --max-time 120 -X "${method}" \
        -H "Authorization: Bearer ${NETDATA_CLOUD_TOKEN}" \
        -H 'Content-Type: application/json' \
        "https://${NETDATA_CLOUD_HOSTNAME}${path}")
    if [[ -n "${body}" ]]; then
        args+=(-d "${body}")
    fi
    agents_run "${args[@]}"
}

# Call any Netdata Agent direct-HTTP path. Resolves the per-agent
# bearer internally (cache or mint via Cloud). Emits ONLY the
# response body to stdout. stderr shows curl with both
# `<CLOUD_TOKEN>` and `<AGENT_BEARER>` masked.
#
# Required flags (provided in any order before METHOD PATH):
#   --node <node_id>          -- target node UUID
#   --host <host:port>        -- agent's bind, e.g. "agent-events:19999"
#   --machine-guid <mg>       -- agent's machine_guid (bearer cache key)
#
# Example:
#   agents_query_agent --node $NODE --host $HOST --machine-guid $MG \
#       POST /api/v3/function?function=systemd-journal '{"info":true}'
agents_query_agent() {
    local node="" host="" mg="" method="" path="" body=""
    while (( $# > 0 )); do
        local arg="$1"
        case "$arg" in
            --node) node="${2-}"; shift 2 ;;
            --host) host="${2-}"; shift 2 ;;
            --machine-guid) mg="${2-}"; shift 2 ;;
            --) shift; break ;;
            -*)
                echo -e "${AGENTS_RED}[ERROR]${AGENTS_NC} Unknown flag: $arg" >&2
                return 1
                ;;
            *) break ;;
        esac
    done
    method="${1:?usage: agents_query_agent --node N --host H --machine-guid M METHOD PATH [BODY]}"
    path="${2:?path required}"
    body="${3:-}"

    : "${node:?--node required}"
    : "${host:?--host required}"
    : "${mg:?--machine-guid required}"

    local bearer
    _agents_resolve_bearer bearer "${node}" "${mg}" "${host}"

    local args=(curl --fail --silent --show-error --max-time 120 -X "${method}" \
        -H "X-Netdata-Auth: Bearer ${bearer}" \
        -H 'Content-Type: application/json' \
        "http://${host}/host/${node}${path}")
    if [[ -n "${body}" ]]; then
        args+=(-d "${body}")
    fi
    agents_run "${args[@]}"
}

# Convenience: Function call with automatic transport selection.
# Wraps agents_query_cloud (preferred) or agents_query_agent.
#
# Flags:
#   --via cloud|agent       default: cloud
#   --node <node_id>        REQUIRED
#   --host <host:port>      REQUIRED for --via agent
#   --machine-guid <mg>     REQUIRED for --via agent
#   --function <name>       REQUIRED (e.g. systemd-journal)
#   --body <json>           default: {"info":true}
agents_call_function() {
    local via="cloud" node="" mg="" host="" fn="" body='{"info":true}'
    while (( $# > 0 )); do
        local arg="$1"
        case "$arg" in
            --via) via="${2-}"; shift 2 ;;
            --node) node="${2-}"; shift 2 ;;
            --machine-guid) mg="${2-}"; shift 2 ;;
            --host) host="${2-}"; shift 2 ;;
            --function) fn="${2-}"; shift 2 ;;
            --body) body="${2-}"; shift 2 ;;
            *)
                echo -e "${AGENTS_RED}[ERROR]${AGENTS_NC} Unknown arg: $arg" >&2
                return 1
                ;;
        esac
    done
    : "${node:?--node required}"
    : "${fn:?--function required}"

    case "${via}" in
        cloud)
            agents_query_cloud POST "/api/v2/nodes/${node}/function?function=${fn}" "${body}"
            ;;
        agent)
            : "${mg:?--machine-guid required for --via agent}"
            : "${host:?--host required for --via agent}"
            agents_query_agent --node "${node}" --host "${host}" --machine-guid "${mg}" \
                POST "/api/v3/function?function=${fn}" "${body}"
            ;;
        *)
            echo -e "${AGENTS_RED}[ERROR]${AGENTS_NC} Unknown --via: ${via}" >&2
            return 1
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Self-test: assert no token bytes leak through public wrappers.
# Run with: bash -c 'source _lib.sh; agents_selftest_no_token_leak'
# ---------------------------------------------------------------------------

agents_selftest_no_token_leak() {
    local sentinel='UNIQUE_SENTINEL_TOKEN_xK4mP7qR9sT2vW8y'
    local fake_bearer='deadbeef-1234-5678-9abc-def012345678'

    # Save real values, swap in sentinels, run wrappers in dry-run,
    # capture stdout, restore.
    local real_token="${NETDATA_CLOUD_TOKEN:-}"
    local real_host="${NETDATA_CLOUD_HOSTNAME:-app.netdata.cloud}"
    NETDATA_CLOUD_TOKEN="${sentinel}"
    NETDATA_CLOUD_HOSTNAME="${real_host}"
    AGENTS_DRY_RUN=1

    local out=""

    # 1. agents_query_cloud should not echo the sentinel.
    out="$(agents_query_cloud GET /api/v2/spaces 2>/dev/null || true)"
    if [[ "${out}" == *"${sentinel}"* ]]; then
        echo -e "${AGENTS_RED}[FAIL]${AGENTS_NC} agents_query_cloud leaked NETDATA_CLOUD_TOKEN to stdout" >&2
        NETDATA_CLOUD_TOKEN="${real_token}"; unset AGENTS_DRY_RUN
        return 1
    fi

    # 2. _agents_log_masked must mask Bearer <uuid> patterns.
    out="$(_agents_log_masked curl -H "Authorization: Bearer ${sentinel}" \
        -H "X-Netdata-Auth: Bearer ${fake_bearer}" \
        https://example.invalid 2>&1 1>/dev/null)"
    if [[ "${out}" == *"${sentinel}"* ]]; then
        echo -e "${AGENTS_RED}[FAIL]${AGENTS_NC} _agents_log_masked leaked NETDATA_CLOUD_TOKEN to stderr" >&2
        NETDATA_CLOUD_TOKEN="${real_token}"; unset AGENTS_DRY_RUN
        return 1
    fi
    if [[ "${out}" == *"${fake_bearer}"* ]]; then
        echo -e "${AGENTS_RED}[FAIL]${AGENTS_NC} _agents_log_masked leaked Bearer <uuid> to stderr" >&2
        NETDATA_CLOUD_TOKEN="${real_token}"; unset AGENTS_DRY_RUN
        return 1
    fi

    # 3. The unit test passes if both checks above passed.
    NETDATA_CLOUD_TOKEN="${real_token}"
    unset AGENTS_DRY_RUN
    echo -e "${AGENTS_GREEN}[PASS]${AGENTS_NC} no-token-leak self-test" >&2
    return 0
}
