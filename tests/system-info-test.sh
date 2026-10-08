#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -eu

test_dir=$(mktemp -d "${TMPDIR:-/tmp}/netdata-system-info-test.XXXXXX")
trap 'rm -rf "${test_dir}"' EXIT HUP INT TERM

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
system_info_script="${script_dir}/../src/daemon/system-info.sh"
functions_script="${test_dir}/functions.sh"

sed -n \
    -e '/^os_release_unescape()/,/^}/p' \
    -e '/^load_os_release()/,/^}/p' \
    -e '/^load_lsb_release()/,/^}/p' \
    -e '/^filter_host_os_label_version()/,/^}/p' \
    -e '/^set_host_os_label_versions()/,/^}/p' \
    "${system_info_script}" > "${functions_script}"

os_release_file="${test_dir}/os-release"
printf '%s\n' \
    'NAME="literal $(printf not-executed)"' \
    'VERSION="a\\bc\\d"' \
    'VERSION_ID="24.04"' \
    'UNSUPPORTED="ignored"' > "${os_release_file}"

os_release_output=$(/bin/sh -c '
    . "$1"
    HOST_NAME=unknown HOST_VERSION=unknown HOST_VERSION_ID=unknown
    load_os_release HOST "$2"
    printf "%s|%s|%s\n" "$HOST_NAME" "$HOST_VERSION" "$HOST_VERSION_ID"
' sh "${functions_script}" "${os_release_file}")

expected_output='literal $(printf not-executed)|a\bc\d|24.04'
[ "${os_release_output}" = "${expected_output}" ] || {
    printf 'unexpected os-release output: %s\n' "${os_release_output}" >&2
    exit 1
}

lsb_release_file="${test_dir}/lsb-release"
printf '%s\n' 'DISTRIB_ID=Test' > "${lsb_release_file}"

invalid_lsb_release_path="${test_dir}/invalid-lsb-release"
mkdir "${invalid_lsb_release_path}"
/bin/sh -c '. "$1"; ! load_lsb_release "$2"' sh "${functions_script}" "${invalid_lsb_release_path}"

if [ "$(id -u)" -ne 0 ]; then
    chmod 000 "${lsb_release_file}"
    /bin/sh -c '. "$1"; ! load_lsb_release "$2"' sh "${functions_script}" "${lsb_release_file}"
fi

label_output=$(/bin/sh -c '
    . "$1"
    KERNEL_NAME=Linux HOST_VERSION_ID=24.04 HOST_VERSION=Ubuntu
    set_host_os_label_versions
    printf "%s|%s\n" "$HOST_OS_LABEL_VERSION" "$HOST_OS_LABEL_RELEASE"
' sh "${functions_script}")
[ "${label_output}" = '|24.04' ] || {
    printf 'unexpected duplicate Linux label output: %s\n' "${label_output}" >&2
    exit 1
}

label_output=$(/bin/sh -c '
    . "$1"
    KERNEL_NAME=Linux HOST_OS_LABEL_VERSION=24.04 HOST_OS_LABEL_RELEASE=24.04.3
    filter_host_os_label_version
    printf "%s|%s\n" "$HOST_OS_LABEL_VERSION" "$HOST_OS_LABEL_RELEASE"
' sh "${functions_script}")
[ "${label_output}" = '24.04|24.04.3' ] || {
    printf 'unexpected distinct Linux label output: %s\n' "${label_output}" >&2
    exit 1
}

label_output=$(/bin/sh -c '
    . "$1"
    KERNEL_NAME=Darwin HOST_VERSION_ID=24.04 HOST_VERSION=macOS
    set_host_os_label_versions
    printf "%s|%s\n" "$HOST_OS_LABEL_VERSION" "$HOST_OS_LABEL_RELEASE"
' sh "${functions_script}")
[ "${label_output}" = '24.04|24.04' ] || {
    printf 'unexpected non-Linux label output: %s\n' "${label_output}" >&2
    exit 1
}

printf '%s\n' 'system-info shell tests: OK'
