#!/usr/bin/env bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m'

ndsudo="${1:?usage: $0 /path/to/ndsudo}"
ndsudo_cmd=(sudo "${ndsudo}")
ndsudo_as_netdata_cmd=(sudo -u netdata "${ndsudo}")

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

run_expect_output() {
    local expected="$1"
    local required="$2"
    local forbidden="$3"
    local output rc
    shift 3

    print_command "$@"
    set +e
    output=$("$@" 2>&1)
    rc=$?
    set -e

    if [[ "${rc}" -ne "${expected}" ]]; then
        printf '%s\n' "${output}" >&2
        fail_command "expected exit code ${expected}, got ${rc}" "$@"
        return 1
    fi
    if [[ "${output}" != *"${required}"* ]]; then
        printf '%s\n' "${output}" >&2
        fail_command "output does not contain the selected first value" "$@"
        return 1
    fi
    if [[ "${output}" == *"${forbidden}"* ]]; then
        printf '%s\n' "${output}" >&2
        fail_command "output contains an ignored duplicate value" "$@"
        return 1
    fi

    printf '%s\n' "${output}" >&2
    printf '%b[OK]%b first duplicate value selected\n' "${GREEN}" "${NC}" >&2
}

run_expect_smartctl_json() {
    local expected_device="$1"
    local output rc
    shift

    print_command "$@"
    set +e
    output=$("$@" 2>&1)
    rc=$?
    set -e

    if ! printf '%s' "${output}" | jq -e --arg device "${expected_device}" \
        '(.smartctl.exit_status | type == "number") and
         (.smartctl.argv == ["smartctl", "--json", "--all", $device, "--device", "nvme", "--nocheck", "standby"])' \
        >/dev/null; then
        printf '%s\n' "${output}" >&2
        fail_command "normal invocation did not execute smartctl with the expected argv (exit code ${rc})" "$@"
        return 1
    fi

    printf '%b[OK]%b normal invocation returned smartctl JSON with the expected argv (exit code %d)\n' \
        "${GREEN}" "${NC}" "${rc}" >&2
}

print_command sudo test -x "${ndsudo}"
if ! sudo test -x "${ndsudo}"; then
    fail_command "ndsudo is not executable" sudo test -x "${ndsudo}"
fi
print_command command -v jq
if ! command -v jq >/dev/null; then
    fail_command "jq is not available" command -v jq
fi
print_command command -v smartctl
if ! command -v smartctl >/dev/null; then
    fail_command "smartctl is not available" command -v smartctl
fi

iokit_path='IOService:/ExampleController(Example)@0/Namespace@1'

# The nonexistent synthetic path exercises normal execution without touching a device.
run_expect_smartctl_json "${iokit_path}" "${ndsudo_as_netdata_cmd[@]}" smartctl-json-device-info \
    --deviceName "${iokit_path}" --deviceType nvme --powerMode standby
run_expect 0 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${iokit_path}" --deviceType nvme --powerMode standby
run_expect 0 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName /dev/disk0 --deviceType nvme --powerMode standby

printf -v suffix '%*s' 498 ''
max_path="IOService:/${suffix// /A}@1"
run_expect 0 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${max_path}" --deviceType nvme --powerMode standby

printf -v suffix '%*s' 499 ''
overlong_path="IOService:/${suffix// /A}@1"
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${overlong_path}" --deviceType nvme --powerMode standby

run_expect 5 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceType nvme --powerMode standby
run_expect_output 0 "${iokit_path}" /dev/disk0 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName "${iokit_path}" --deviceName /dev/disk0 --deviceType nvme --powerMode standby
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName /dev/disk0 --deviceName "${iokit_path}" --deviceType nvme --powerMode standby

run_expect 2 "${ndsudo_cmd[@]}" --test fail2ban-client-status-jail --jail 'name@example'
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName 'disk(Example)@0' --deviceType nvme --powerMode standby
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName /dev/disk0 --deviceType 'nvme@example' --powerMode standby
run_expect 2 "${ndsudo_cmd[@]}" --test smartctl-json-device-info \
    --deviceName 'IOService:/Example;Controller@0' --deviceType nvme --powerMode standby
run_expect 1 "${ndsudo_cmd[@]}" --test
