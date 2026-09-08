#!/usr/bin/env bash
#
# Copyright: 2026 (c) Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Runs the spawn-server unit tests against an already-built tree.
#
# Kept separate from run-unit-tests.sh so it can be invoked on its own, by a CI job that has already
# built netdata and wants only this suite. That matters for macOS: the spawn servers use
# platform-specific primitives (posix_spawn with POSIX_SPAWN_SETSIGDEF, and the signal disposition
# and mask a child inherits across exec), so passing on Linux does not establish the behaviour
# there - and macOS is where netdata/netdata#23730 was observed.
#
# NETDATA_BUILD_DIR selects the build tree (default ./build).

set -uo pipefail

build_dir="${NETDATA_BUILD_DIR:-./build}"
tester="${build_dir}/spawn-tester"

if [ ! -x "${tester}" ]; then
    printf 'spawn-tester not found at %s\n' "${tester}" >&2
    printf 'Build it first (it is part of the default target set), or point NETDATA_BUILD_DIR at the\n' >&2
    printf 'build tree.\n' >&2
    exit 1
fi

echo "Running spawn-server unit tests"

# Its own run directory: the tester creates a spawn server there, and every netdata instance on the
# host otherwise shares one (src/libnetdata/os/run_dir.c), so a concurrent agent would collide.
#
# The directory also has to be SHORT. The spawn server binds "<run_dir>/netdata-spawn-<name>.sock",
# AF_UNIX sun_path holds 103 usable bytes, and the tester's longest server name is "test-callback"
# - so the run directory cannot exceed 70 bytes. $TMPDIR is fine on Linux (usually unset, so /tmp)
# but on macOS it is a long per-user path such as
# /var/folders/_5/zjnzxgh147qcg3bb5cg2wvqw0000gn/T/, which blows the budget on its own. Prefer a
# short base and verify the result, rather than letting bind() fail deep inside the spawn server.
MAX_RUN_DIR_LEN=70

run_dir=""
for base in "${NETDATA_SPAWN_TEST_TMPDIR:-}" /tmp "${TMPDIR:-}"; do
    [ -n "${base}" ] || continue
    [ -d "${base}" ] && [ -w "${base}" ] || continue

    candidate="$(mktemp -d "${base%/}/nd-spawn-test.XXXXXX" 2>/dev/null)" || continue

    # Bytes, not characters: sun_path is a byte buffer, while bash's ${#var} counts characters in a
    # multibyte locale. A non-ASCII base (TMPDIR under a name with accents, say) would otherwise
    # measure short enough here and still overflow sun_path, which is the failure being prevented.
    candidate_len=$(printf %s "${candidate}" | wc -c | tr -d '[:space:]')

    if [ "${candidate_len}" -le "${MAX_RUN_DIR_LEN}" ]; then
        run_dir="${candidate}"
        break
    fi

    rm -rf -- "${candidate}"
done

if [ -z "${run_dir}" ]; then
    printf 'could not create a run directory of at most %d bytes.\n' "${MAX_RUN_DIR_LEN}" >&2
    printf 'The spawn server binds a socket inside it and AF_UNIX allows only 103 bytes of path.\n' >&2
    printf 'Set NETDATA_SPAWN_TEST_TMPDIR to a short writable directory.\n' >&2
    exit 1
fi

# INT/TERM as well as EXIT: an interrupted run (Ctrl-C, or a CI step timeout) would otherwise
# orphan the directory along with the AF_UNIX socket the spawn server bound inside it. SIGKILL
# cannot be trapped, so a hard kill still leaves both behind - nothing in the script can change
# that, and the directory name is distinctive enough to find if it happens.
cleanup() {
    rm -rf -- "${run_dir}"
}

trap cleanup EXIT
trap 'cleanup; exit 130' INT
trap 'cleanup; exit 143' TERM
trap 'cleanup; exit 129' HUP

NETDATA_RUN_DIR="${run_dir}" \
ASAN_OPTIONS=detect_leaks=0 \
    "${tester}" test
exit $?
