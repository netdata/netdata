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
run_dir="$(mktemp -d "${TMPDIR:-/tmp}/netdata-spawn-tester.XXXXXX")" || exit 1
trap 'rm -rf -- "${run_dir}"' EXIT

NETDATA_RUN_DIR="${run_dir}" \
ASAN_OPTIONS=detect_leaks=0 \
    "${tester}" test
exit $?
