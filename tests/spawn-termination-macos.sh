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
# How long to wait for the child's first byte before giving up on it streaming at all. Must stay
# below GRACE_SECONDS so the reader closes the pipe while the writer is still under the guard.
READ_TIMEOUT="${READ_TIMEOUT:-8}"

if [[ "${READ_TIMEOUT}" -ge "${GRACE_SECONDS}" ]]; then
    printf 'READ_TIMEOUT (%s) must be below GRACE_SECONDS (%s), or the child is killed before the\n' \
        "${READ_TIMEOUT}" "${GRACE_SECONDS}" >&2
    printf 'reader ever closes the pipe, and every run degrades into a silent skip.\n' >&2
    exit 3
fi

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/nd-spawn-termination.XXXXXX")" || {
    printf 'cannot create a work directory\n' >&2
    exit 3
}
trap 'rm -rf "${work_dir}"' EXIT

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
    # Exit status alone is not enough: on a VM powermetrics can exit 0 while emitting nothing, and a
    # child that never writes can never be killed by a closed pipe - which would make this probe
    # report a broken premise when the truth is that it cannot be tested here. Require real output.
    sampler=""
    for candidate in "thermal,smc,gpu_power" "thermal,gpu_power" "thermal,smc" "thermal"; do
        info "trying samplers: ${candidate}"
        if [[ -n "$("${powermetrics_bin}" -n 1 -i 1000 -s "${candidate}" -f plist 2>/dev/null)" ]]; then
            sampler="${candidate}"
            ok "samplers available: ${sampler}"
            break
        fi
    done
    [[ -n "${sampler}" ]] || skip "powermetrics produced no output for any of the collector's sampler sets here (typical on a VM)"

    # The exact loop-mode invocation the collector uses: stream until stopped, never self-terminate.
    stream_cmd="${powermetrics_bin} -b 0 -i 1000 -s ${sampler} -f plist"
fi

# ---------------------------------------------------------------------------------------------------
# the probe

# Runs $stream_cmd with stdout on a pipe, waits for it to actually produce output, then closes the
# read end. Echoes: "<outcome> <seconds> <streamed|silent>" where outcome is
# died-sigpipe | survived | self-exited:<rc>, and the last field says whether any byte ever arrived.
probe_one() {
    local disposition="$1" started elapsed rc streamed
    local flag="${work_dir}/streamed.$$"
    rm -f "${flag}"
    started=$(date +%s)

    # Job control puts the subshell in its own process group, so the guard below can kill the WHOLE
    # group. Killing only the subshell would orphan powermetrics: it is deliberately immortal in the
    # 'ignored' case, so it would be reparented to launchd and left running as root - this harness
    # would strand exactly the process this test exists to prevent.
    set -m
    (
        if [[ "${disposition}" == "ignored" ]]; then
            trap '' PIPE            # SIG_IGN, inherited across fork+exec - models netdata
        fi
        # The reader waits for one real byte, then exits and closes the pipe while the writer is
        # mid-stream. On timeout it exits anyway (still closing the pipe) but leaves no flag, which
        # is how we tell "immortal child" apart from "child that never wrote".
        # PIPESTATUS[0] is the writer's wait status, surfaced as this subshell's exit code.
        # shellcheck disable=SC2086  # stream_cmd is a deliberately word-split command line
        ${stream_cmd} 2>/dev/null | { IFS= read -r -t "${READ_TIMEOUT}" -n 1 _ && : > "${flag}"; }
        exit "${PIPESTATUS[0]}"
    ) &
    local probe_pid=$!
    set +m

    # Bound the wait by polling rather than with a background guard. Stock macOS has no `timeout`,
    # and an immortal child never returns on its own. A backgrounded `sleep` guard is worse than it
    # looks: killing the guard subshell leaves its `sleep` alive, that orphan holds this function's
    # command-substitution pipe open until it expires, and every probe then costs the full grace
    # even when the child died immediately - besides leaving stray processes behind.
    local waited=0 limit=$(( GRACE_SECONDS * 5 ))
    while kill -0 "${probe_pid}" 2>/dev/null && [[ "${waited}" -lt "${limit}" ]]; do
        sleep 0.2
        waited=$(( waited + 1 ))
    done

    # Still alive: it is immortal by construction in the 'ignored' case. Kill the whole process
    # group, so the command the subshell spawned cannot be orphaned and reparented - this harness
    # must never strand the very kind of process the test exists to prevent.
    #
    # The group is resolved HERE rather than at launch: a child that dies immediately (the expected
    # 'default' outcome) is already gone by then, and looking it up early would race with its exit.
    # At this point it is known alive, so ps can see it.
    if kill -0 "${probe_pid}" 2>/dev/null; then
        local pgid own_pgid
        pgid=$(ps -o pgid= -p "${probe_pid}" 2>/dev/null | tr -d ' ')
        own_pgid=$(ps -o pgid= -p $$ 2>/dev/null | tr -d ' ')

        if [[ -n "${pgid}" && "${pgid}" != "${own_pgid}" ]]; then
            kill -9 -- "-${pgid}" 2>/dev/null
        else
            # No distinct group to kill: the streaming child cannot be reached, and killing the
            # subshell alone would orphan it - as root, and deliberately immortal. Report a harness
            # error instead of a verdict; the caller refuses to draw a conclusion.
            kill -9 "${probe_pid}" 2>/dev/null
            wait "${probe_pid}" 2>/dev/null
            printf 'harness-error %s %s\n' "$(( $(date +%s) - started ))" "unknown"
            return
        fi
    fi

    wait "${probe_pid}"; rc=$?

    elapsed=$(( $(date +%s) - started ))
    streamed=silent
    [[ -f "${flag}" ]] && streamed=streamed
    rm -f "${flag}"

    # A shell reports a signalled child as 128+signo. 13 = SIGPIPE, 9 = the guard's SIGKILL.
    case "${rc}" in
        141) printf 'died-sigpipe %s %s\n' "${elapsed}" "${streamed}" ;;
        137) printf 'survived %s %s\n' "${elapsed}" "${streamed}" ;;
        *)   printf 'self-exited:%s %s %s\n' "${rc}" "${elapsed}" "${streamed}" ;;
    esac
}

info "command under test: ${stream_cmd}"
info "grace before declaring a child immortal: ${GRACE_SECONDS}s"

info "case 1/2: parent SIGPIPE = DEFAULT (models netdata WITH the fix)"
read -r default_outcome default_secs default_stream <<<"$(probe_one default)"
info "  -> ${default_outcome} after ${default_secs}s (${default_stream})"

info "case 2/2: parent SIGPIPE = IGNORED (models netdata on master)"
read -r ignored_outcome ignored_secs ignored_stream <<<"$(probe_one ignored)"
info "  -> ${ignored_outcome} after ${ignored_secs}s (${ignored_stream})"

# ---------------------------------------------------------------------------------------------------
# verdict

printf '\n'
printf 'SIGPIPE default -> %s (%ss, %s)\n' "${default_outcome}" "${default_secs}" "${default_stream}" >&2
printf 'SIGPIPE ignored -> %s (%ss, %s)\n' "${ignored_outcome}" "${ignored_secs}" "${ignored_stream}" >&2
printf '\n'

# Only two default-case outcomes actually exercise the premise: the child died of SIGPIPE, or it
# outlived the closed pipe. Anything else - it never wrote (its next write never comes), or it
# exited for its own reasons - leaves the question unanswered. That is untestable here, NOT a broken
# premise, so it must skip rather than fail a build. Tracking whether any byte arrived is what lets
# the probe tell these apart.
if [[ "${default_outcome}" == "harness-error" || "${ignored_outcome}" == "harness-error" ]]; then
    err "the probe could not isolate its child in a process group, so it cannot guarantee cleanup"
    err "and refuses to draw a conclusion. A survivor may have been left behind - check with"
    err "'pgrep -x powermetrics'."
    exit 3
fi

if [[ "${default_stream}" == "silent" ]]; then
    skip "powermetrics never wrote to its stdout within ${READ_TIMEOUT}s (outcome '${default_outcome}'), so closing the pipe cannot be exercised here - this says nothing about the premise either way"
fi

if [[ "${default_outcome}" != "died-sigpipe" && "${default_outcome}" != "survived" ]]; then
    skip "powermetrics exited on its own (outcome '${default_outcome}') instead of being killed by the closed pipe, so the premise was never exercised here"
fi

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
