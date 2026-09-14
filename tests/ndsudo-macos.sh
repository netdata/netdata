#!/usr/bin/env bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m'

ndsudo="${1:?usage: $0 /path/to/ndsudo}"
ndsudo_cmd=(sudo "${ndsudo}")

print_command() {
    printf '%b%s >%b %b' "${GRAY}" "$(pwd)" "${NC}" "${YELLOW}" >&2
    printf '%q ' "$@" >&2
    printf '%b\n' "${NC}" >&2
}

fail_command() {
    local message="$1"
    shift

    printf '%b%s%b\n' "${RED}" '-------------------------------------------------------------------------------' "${NC}" >&2
    printf '%b[ERROR]%b %s\n' "${RED}" "${NC}" "${message}" >&2
    printf '%b        Command:%b ' "${RED}" "${NC}" >&2
    printf '%q ' "$@" >&2
    printf '\n%b        Working dir:%b %s\n' "${RED}" "${NC}" "$(pwd)" >&2
    printf '%b%s%b\n' "${RED}" '-------------------------------------------------------------------------------' "${NC}" >&2
    return 1
}

run_expect() {
    local expected="$1"
    local rc
    shift

    print_command "$@"
    set +e
    "$@"
    rc=$?
    set -e

    if [[ "${rc}" -ne "${expected}" ]]; then
        fail_command "expected exit code ${expected}, got ${rc}" "$@"
        return 1
    fi

    printf '%b[OK]%b exit code %d\n' "${GREEN}" "${NC}" "${rc}" >&2
}

# A well-formed invocation that reaches the executable search must be refused
# with exit 4 and a clear message, because the tool is not in ndsudo's search
# path on macOS.
run_expect_not_in_path() {
    local output rc

    print_command "$@"
    set +e
    output=$("$@" 2>&1)
    rc=$?
    set -e

    if [[ "${rc}" -ne 4 || "${output}" != *"not available in PATH"* ]]; then
        printf '%s\n' "${output}" >&2
        fail_command "expected exit 4 with 'not available in PATH', got ${rc}" "$@"
        return 1
    fi

    printf '%b[OK]%b refused cleanly: not available in PATH (exit 4)\n' "${GREEN}" "${NC}" >&2
}

print_command sudo test -x "${ndsudo}"
if ! sudo test -x "${ndsudo}"; then
    fail_command "ndsudo is not executable" sudo test -x "${ndsudo}"
fi

iokit_path='IOService:/ExampleController(Example)@0/Namespace@1'

# On macOS ndsudo searches Apple-owned directories only (/bin, /sbin,
# /usr/bin, /usr/sbin): /usr/local and /opt/homebrew are writable by the
# console admin, so a setuid-root binary searching them would execute
# admin-planted binaries as root. Apple ships no smartctl, so every
# invocation that reaches the executable search must fail closed with
# exit 4 - even when an admin-writable smartctl exists. Argument
# validation runs before the search and keeps its own exit codes.

# Make sure an admin-writable smartctl exists, so the refusals below prove
# the search path is closed rather than merely that the tool is missing.
# The marker file catches any execution of the plant.
planted=""
if ! command -v smartctl > /dev/null 2>&1; then
    sudo mkdir -p /usr/local/bin
    printf '#!/bin/sh\ntouch /tmp/ndsudo-planted-executed\n' | sudo tee /usr/local/bin/smartctl > /dev/null
    sudo chmod 755 /usr/local/bin/smartctl
    planted="/usr/local/bin/smartctl"
fi
sudo rm -f /tmp/ndsudo-planted-executed

# Well-formed invocations: refused at the executable search, fail closed.
run_expect_not_in_path "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${iokit_path}" --deviceType nvme --powerMode standby
run_expect_not_in_path "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName /dev/disk0 --deviceType nvme --powerMode standby
run_expect_not_in_path "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${iokit_path}" --deviceName /dev/disk0 --deviceType nvme --powerMode standby
run_expect_not_in_path "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceType nvme --powerMode standby

printf -v suffix '%*s' 498 ''
max_path="IOService:/${suffix// /A}@1"
run_expect_not_in_path "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${max_path}" --deviceType nvme --powerMode standby

# Argument validation rejects bad input before any search happens.
printf -v suffix '%*s' 499 ''
overlong_path="IOService:/${suffix// /A}@1"
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${overlong_path}" --deviceType nvme --powerMode standby

run_expect 2 "${ndsudo_cmd[@]}" --test fail2ban-client-status-jail --jail 'name@example'
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName 'disk(Example)@0' --deviceType nvme --powerMode standby
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName /dev/disk0 --deviceType 'nvme@example' --powerMode standby
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName 'IOService:/Example;Controller@0' --deviceType nvme --powerMode standby
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName /dev/disk0 --deviceName "${iokit_path}" --deviceType nvme --powerMode standby

run_expect 3 "${ndsudo_cmd[@]}" not-a-command
run_expect 1 "${ndsudo_cmd[@]}" --test

if [[ -e /tmp/ndsudo-planted-executed ]]; then
    fail_command "the planted smartctl was EXECUTED by ndsudo" test -e /tmp/ndsudo-planted-executed
fi
printf '%b[OK]%b admin-writable smartctl was never executed\n' "${GREEN}" "${NC}" >&2

if [[ -n "${planted}" ]]; then
    sudo rm -f "${planted}"
fi
