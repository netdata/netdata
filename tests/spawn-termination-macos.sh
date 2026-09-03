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

if [[ -n "$(trap -p PIPE)" ]]; then
    printf 'SIGPIPE is ignored in the shell that invoked this script.\n' >&2
    printf 'POSIX does not allow a signal ignored on entry to be reset from within the shell, so the\n' >&2
    printf 'default-disposition case cannot be modelled here: both cases would run with SIGPIPE\n' >&2
    printf 'ignored and the verdict would be a false "premise broken". Re-run from a shell with the\n' >&2
    printf 'default disposition (a plain terminal, or the CI step directly).\n' >&2
    exit 3
fi

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

stream_argv=()

if [[ -n "${NDPROBE_STREAM_CMD}" ]]; then
    warn "using the test-only NDPROBE_STREAM_CMD override - this is a harness self-test, NOT a probe"
    # Word-split once, here, so the command is held as an argv array and never re-split at use.
    # shellcheck disable=SC2206  # deliberate split of a test-only command line
    stream_argv=( ${NDPROBE_STREAM_CMD} )
    record_delim=$'\n'        # the stand-ins emit lines, not plists
else
    [[ "$(uname -s)" == "Darwin" ]] || skip "not macOS (uname -s = $(uname -s)); powermetrics does not exist here"

    powermetrics_bin="${POWERMETRICS:-/usr/bin/powermetrics}"
    # -x alone would accept a directory, and executing one fails in a way that looks exactly like an
    # unavailable sampler further down - which would skip with a misleading reason.
    [[ -f "${powermetrics_bin}" && -r "${powermetrics_bin}" && -x "${powermetrics_bin}" ]] \
        || skip "${powermetrics_bin} is not a readable, executable file"

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
    stream_argv=( "${powermetrics_bin}" -b 0 -i 1000 -s "${sampler}" -f plist )
    record_delim=""            # -f plist emits NUL-separated documents, one per sample
fi

# ---------------------------------------------------------------------------------------------------
# the probe

# Runs the command under test with stdout on a pipe, waits until it is demonstrably streaming,
# then closes the read end and reports what happened to it.
# Echoes: "<outcome> <seconds> <streamed|silent>" where outcome is
# died-sigpipe | survived | self-exited:<rc> | harness-error.
probe_one() {
    local disposition="$1" started elapsed rc streamed
    local flag="${work_dir}/streamed.$$"
    local fifo="${work_dir}/stream.$$"

    rm -f "${flag}" "${fifo}"
    if ! mkfifo "${fifo}" 2>/dev/null; then
        printf 'harness-error 0 unknown\n'
        return
    fi

    started=$(date +%s)

    # The subshell `exec`s the command, so it IS the command: $! is the writer's own pid and it can
    # be signalled directly. That is deliberate - an earlier version wrapped the writer in a
    # pipeline and had to rely on job control to put it in its own process group, then kill the
    # group, because the writer's pid was unknowable. Any failure of that scheme orphaned a root
    # process, which is the very thing this test exists to catch. Knowing the pid removes the
    # problem instead of guarding against it.
    (
        if [[ "${disposition}" == "ignored" ]]; then
            trap '' PIPE       # SIG_IGN, and it survives exec - this models netdata
        fi
        # No `trap - PIPE` in the default case: it would be a no-op anyway, because POSIX forbids
        # resetting a signal that was ignored on entry to the shell. That case is instead refused
        # outright by the entry check above, so reaching here means the disposition is already
        # default and inherited as such.
        exec "${stream_argv[@]}" > "${fifo}" 2>/dev/null
    ) &
    local writer_pid=$!

    # Two complete records prove the child is still writing, which is the only thing that makes a
    # closed pipe lethal: closing a pipe does not signal anything - the child's NEXT WRITE does. A
    # child that emits one sample and goes quiet can never be killed this way, and reporting that as
    # a verdict is how this probe previously failed a runner that simply stopped sampling.
    # Leaving this block closes the read end, breaking the pipe under a writer that is mid-stream.
    {
        if IFS= read -r -d "${record_delim}" -t "${READ_TIMEOUT}" _ &&
           IFS= read -r -d "${record_delim}" -t "${READ_TIMEOUT}" _; then
            : > "${flag}"
        fi
    } < "${fifo}"

    # Bounded, wall-clock: an immortal child never returns on its own, and stock macOS has no
    # `timeout`. A deadline rather than a sleep count, which drifted badly under load.
    local deadline=$(( started + GRACE_SECONDS ))
    while kill -0 "${writer_pid}" 2>/dev/null && [[ "$(date +%s)" -lt "${deadline}" ]]; do
        sleep 0.2
    done

    # Still alive: immortal by construction in the 'ignored' case. Reap it so no run of this script
    # leaves a survivor behind.
    if kill -0 "${writer_pid}" 2>/dev/null; then
        kill -9 "${writer_pid}" 2>/dev/null
    fi

    wait "${writer_pid}"; rc=$?
    rm -f "${fifo}"

    elapsed=$(( $(date +%s) - started ))
    streamed=silent
    [[ -f "${flag}" ]] && streamed=streamed
    rm -f "${flag}"

    # A shell reports a signalled child as 128+signo. 13 = SIGPIPE, 9 = our own SIGKILL.
    case "${rc}" in
        141) printf 'died-sigpipe %s %s\n' "${elapsed}" "${streamed}" ;;
        137) printf 'survived %s %s\n' "${elapsed}" "${streamed}" ;;
        *)   printf 'self-exited:%s %s %s\n' "${rc}" "${elapsed}" "${streamed}" ;;
    esac
}

info "command under test: ${stream_argv[*]}"
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
    skip "powermetrics did not produce two consecutive samples within ${READ_TIMEOUT}s each (outcome '${default_outcome}'), so it was not writing when the pipe closed and had no next write to be killed by - untestable here, and no evidence either way about the premise"
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
