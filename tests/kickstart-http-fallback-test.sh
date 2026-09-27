#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

# Globals and mocks are consumed by the dynamically extracted installer functions.
# shellcheck disable=SC2034,SC2154,SC2329
set -eu
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/netdata-kickstart-http-test.XXXXXX")
trap 'rm -rf "${test_dir}"' EXIT HUP INT TERM
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
sed -n \
    -e '/^handle_http_status()/,/^}/p' \
    -e '/^handle_curl_result()/,/^}/p' \
    -e '/^deferred_warnings()/,/^}/p' \
    -e '/^try_build_install()/,/^}/p' \
    "${script_dir}/../packaging/installer/kickstart.sh" > "${test_dir}/functions.sh"
# shellcheck source=/dev/null
. "${test_dir}/functions.sh"

warning() { :; }
fatal() { exit 99; }
failed=0
for scenario in '56 404 1' '56 403 5' '56 503 6' '56 200 99' '56 000 99' '22 404 1' '78 404 1' '0 200 0' '6 000 2' '60 000 3' '23 000 99'; do
    # Each tuple contains curl exit status, HTTP status, expected helper status.
    # Intentional splitting of the numeric test tuple.
    # shellcheck disable=SC2086
    set -- ${scenario}
    printf '%s' "$2" > "${test_dir}/download.log"
    : > "${test_dir}/partial"
    actual=0
    (handle_curl_result "$1" 'https://example.invalid/archive' "${test_dir}/download.log" downloading "${test_dir}/partial") || actual=$?
    if [ "${actual}" -ne "$3" ]; then
        printf 'curl %s / HTTP %s: expected %s, got %s\n' "$1" "$2" "$3" "${actual}" >&2
        failed=1
    fi
    if [ "$1" -ne 0 ] && [ -e "${test_dir}/partial" ]; then
        printf 'partial download not removed for curl %s\n' "$1" >&2
        failed=1
    fi
done

NETDATA_WARNINGS='\n  - curl --write-out "%{http_code}" failed (100%).'
warning_status=0
deferred_warnings 2> "${test_dir}/warnings" || warning_status=$?
if [ "${warning_status}" -ne 0 ] || ! grep -F '  - curl --write-out "%{http_code}" failed (100%).' "${test_dir}/warnings" >/dev/null; then
    printf '%s\n' 'deferred warning text was not preserved' >&2
    failed=1
fi

# Exercise the actual source-install selection without building or installing.
if ! (
    cd "${test_dir}"
    tmpdir="${test_dir}"
    DRY_RUN=1
    SELECTED_RELEASE_CHANNEL=stable
    set_tmpdir() { :; }
    progress() { :; }
    install_local_build_dependencies() { return 0; }
    set_source_archive_urls() {
        NETDATA_SOURCE_ARCHIVE_BASEURL='https://example.invalid/netdata.tar'
        NETDATA_SOURCE_ARCHIVE_BASE_NAME='netdata.tar'
        NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL='https://example.invalid/sha256sums.txt'
    }
    zstd() { :; }
    run() { :; }
    build_and_install() { : > built; }
    download() {
        printf '%s\n' "$1" >> requests
        case "$1" in
            *.zst)
                printf '404' > download.log
                handle_curl_result 56 "$1" "${test_dir}/download.log" downloading "$2"
                ;;
            *) return 0 ;;
        esac
    }
    try_build_install
    [ -f built ] && [ "${archive_name}" = netdata.tar.gz ] &&
        grep -Fx 'https://example.invalid/netdata.tar.gz' requests >/dev/null
); then
    printf '%s\n' 'source install did not fall back to gzip' >&2
    failed=1
fi
[ "${failed}" -eq 0 ] || exit 1
printf '%s\n' 'kickstart HTTP fallback tests: OK'
