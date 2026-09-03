#!/usr/bin/env bash
#
# Copyright: 2026 (c) Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Does a long-running powermetrics child die when netdata closes its stdout?
#
# WHY THIS EXISTS
#   netdata stops a spawned child by closing its stdio: the child's next write hits a pipe with no
#   readers and the kernel kills it with SIGPIPE. That only works if the child's SIGPIPE disposition
#   is DEFAULT. Signal HANDLERS are reset by exec, but IGNORED signals are NOT - SIG_IGN survives
#   execve() - and netdata sets SIGPIPE to SIG_IGN (NETDATA_SIGNAL_IGNORE in
#   src/daemon/signal-handler.c). A child that inherits that ignore gets EPIPE instead of dying and
#   keeps running forever. When such a child was started through the setuid-root ndsudo helper,
#   netdata cannot even signal it (it runs unprivileged, the child's real uid is 0), so the process
#   is stranded permanently. That is netdata/netdata#23730: 624 orphaned powermetrics processes
#   after 13 days of uptime, 443% CPU between them.
#
#   The fix makes the spawn server force SIGPIPE back to default in every child. This script checks
#   the assumption that fix rests on: that powermetrics actually DOES die on a broken pipe once the
#   disposition is default - i.e. that it does not install a SIGPIPE handler of its own.
#
# HOW IT CHECKS
#   Runs the real powermetrics twice, identically, changing only the parent's SIGPIPE disposition:
#     - default          -> models netdata WITH the fix    -> child must die of SIGPIPE
#     - trap '' PIPE     -> models netdata on master       -> child is expected to survive
#   `trap '' PIPE` sets SIG_IGN, which is inherited across fork+exec, so plain bash reproduces the
#   daemon's state exactly. A reader takes one byte and exits, which closes the pipe under a
#   powermetrics that is already running and streaming.
#
# REQUIREMENTS
#   macOS, bash, and root (powermetrics requires it). No netdata build, no repo checkout: this can be
#   dropped onto any Mac and run on its own. Deliberately avoids `timeout`, which stock macOS lacks.
#
# EXIT CODES
#   0  premise holds (child dies on a broken pipe), or SKIPPED because powermetrics is unavailable
#   1  PREMISE BROKEN - powermetrics survives even with a default disposition; the spawn-server fix
#      alone does NOT stop the leak and the privileged-child problem must be solved instead
#   2  UNEXPECTED - powermetrics dies even when SIGPIPE is inherited as ignored, so the recorded
#      root-cause model for #23730 does not explain the observed leak and needs revisiting
#   3  usage or harness error

# Deliberately no `set -e`: this script's whole job is to inspect non-zero child statuses, and
# `wait` on a child that died of a signal returns non-zero by design.
set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m'

GRACE_SECONDS="${GRACE_SECONDS:-15}"

# Test-only hook: overrides the streaming command so this harness can be exercised on a
# non-macOS host with a stand-in writer. Leave unset for a real probe.
NDPROBE_STREAM_CMD="${NDPROBE_STREAM_CMD:-}"

info()  { printf '%b==>%b %s\n' "${GRAY}" "${NC}" "$*" >&2; }
ok()    { printf '%b[OK]%b %s\n' "${GREEN}" "${NC}" "$*" >&2; }
warn()  { printf '%b[WARN]%b %s\n' "${YELLOW}" "${NC}" "$*" >&2; }
err()   { printf '%b[ERROR]%b %s\n' "${RED}" "${NC}" "$*" >&2; }

skip() {
    printf '%b%s%b\n' "${YELLOW}" '-------------------------------------------------------------------------------' "${NC}" >&2
    printf '%b[SKIPPED]%b %s\n' "${YELLOW}" "${NC}" "$*" >&2
    printf '%b          This is INCONCLUSIVE, not a pass: the question is unanswered.%b\n' "${YELLOW}" "${NC}" >&2
    printf '%b%s%b\n' "${YELLOW}" '-------------------------------------------------------------------------------' "${NC}" >&2
    exit 0
}

# ---------------------------------------------------------------------------------------------------
# preconditions

if [[ -n "${NDPROBE_STREAM_CMD}" ]]; then
    warn "using the test-only NDPROBE_STREAM_CMD override - this is a harness self-test, NOT a probe"
    stream_cmd="${NDPROBE_STREAM_CMD}"
else
    [[ "$(uname -s)" == "Darwin" ]] || skip "not macOS (uname -s = $(uname -s)); powermetrics does not exist here"

    powermetrics_bin="${POWERMETRICS:-/usr/bin/powermetrics}"
    [[ -x "${powermetrics_bin}" ]] || skip "${powermetrics_bin} is not executable"

    [[ "$(id -u)" -eq 0 ]] || skip "must run as root (powermetrics requires it) - re-run with sudo"

    # Pick the first sampler set that works, in the same order the collector's probe tries them
    # (src/collectors/macos.plugin/macos_powermetrics.c). A VM runner may support none of them.
    sampler=""
    for candidate in "thermal,smc,gpu_power" "thermal,gpu_power" "thermal,smc" "thermal"; do
        info "trying samplers: ${candidate}"
        if "${powermetrics_bin}" -n 1 -i 1000 -s "${candidate}" -f plist >/dev/null 2>&1; then
            sampler="${candidate}"
            ok "samplers available: ${sampler}"
            break
        fi
    done
    [[ -n "${sampler}" ]] || skip "powermetrics cannot sample any of the collector's sampler sets here (typical on a VM)"

    # The exact loop-mode invocation the collector uses: stream until stopped, never self-terminate.
    stream_cmd="${powermetrics_bin} -b 0 -i 1000 -s ${sampler} -f plist"
fi

# ---------------------------------------------------------------------------------------------------
# the probe

# Runs $stream_cmd with stdout on a pipe, lets it produce output, then closes the read end.
# Echoes: "<outcome> <seconds>"  where outcome is died-sigpipe | survived | self-exited:<rc>
probe_one() {
    local disposition="$1" started elapsed rc
    started=$(date +%s)

    (
        if [[ "${disposition}" == "ignored" ]]; then
            trap '' PIPE            # SIG_IGN, inherited across fork+exec - models netdata
        fi
        # The reader takes one byte and exits, closing the pipe while the writer is mid-stream.
        # PIPESTATUS[0] is the writer's wait status, surfaced as this subshell's exit code.
        # shellcheck disable=SC2086  # stream_cmd is a deliberately word-split command line
        ${stream_cmd} 2>/dev/null | head -c 1 >/dev/null
        exit "${PIPESTATUS[0]}"
    ) &
    local probe_pid=$!

    # Bound the wait by hand: stock macOS has no `timeout`, and an immortal child never returns.
    ( sleep "${GRACE_SECONDS}"; kill -9 "${probe_pid}" 2>/dev/null ) &
    local guard_pid=$!

    wait "${probe_pid}"; rc=$?
    kill "${guard_pid}" 2>/dev/null
    wait "${guard_pid}" 2>/dev/null
    elapsed=$(( $(date +%s) - started ))

    # A shell reports a signalled child as 128+signo. 13 = SIGPIPE, 9 = the guard's SIGKILL.
    case "${rc}" in
        141) printf 'died-sigpipe %s\n' "${elapsed}" ;;
        137) printf 'survived %s\n' "${elapsed}" ;;
        *)   printf 'self-exited:%s %s\n' "${rc}" "${elapsed}" ;;
    esac
}

info "command under test: ${stream_cmd}"
info "grace before declaring a child immortal: ${GRACE_SECONDS}s"

info "case 1/2: parent SIGPIPE = DEFAULT (models netdata WITH the fix)"
read -r default_outcome default_secs <<<"$(probe_one default)"
info "  -> ${default_outcome} after ${default_secs}s"

info "case 2/2: parent SIGPIPE = IGNORED (models netdata on master)"
read -r ignored_outcome ignored_secs <<<"$(probe_one ignored)"
info "  -> ${ignored_outcome} after ${ignored_secs}s"

# ---------------------------------------------------------------------------------------------------
# verdict

printf '\n'
printf 'SIGPIPE default -> %s (%ss)\n' "${default_outcome}" "${default_secs}" >&2
printf 'SIGPIPE ignored -> %s (%ss)\n' "${ignored_outcome}" "${ignored_secs}" >&2
printf '\n'

case "${default_outcome}:${ignored_outcome}" in
    died-sigpipe:survived)
        ok "PREMISE HOLDS: closing stdout kills powermetrics once SIGPIPE is default (${default_secs}s),"
        ok "and it survives when the ignore is inherited - exactly the #23730 failure mode."
        ok "The spawn-server fix is sufficient to stop the leak."
        exit 0
        ;;
    died-sigpipe:*)
        err "UNEXPECTED: powermetrics also stopped when SIGPIPE was inherited as ignored"
        err "(outcome '${ignored_outcome}'). The recorded root cause for #23730 predicts it survives,"
        err "so it does not explain the observed leak. Do not ship on this evidence - re-open the"
        err "root-cause analysis before relying on the spawn-server fix."
        exit 2
        ;;
    *)
        err "PREMISE BROKEN: powermetrics did NOT die of SIGPIPE with a default disposition"
        err "(outcome '${default_outcome}' after ${default_secs}s). It must be handling SIGPIPE or"
        err "the write error itself, so forcing the disposition back to default does NOT stop the"
        err "leak. Fixing the spawn server is still correct, but #23730 additionally needs the"
        err "privileged-child problem solved: netdata cannot signal a setuid-root child at all."
        exit 1
        ;;
esac
