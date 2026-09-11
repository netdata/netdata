#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -eu

test_dir=$(mktemp -d "${TMPDIR:-/tmp}/netdata-kickstart-test.XXXXXX")
trap 'rm -rf "${test_dir}"' EXIT HUP INT TERM

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
kickstart_script="${script_dir}/../packaging/installer/kickstart.sh"
functions_script="${test_dir}/functions.sh"

sed -n \
    -e '/^warning()/,/^}/p' \
    -e '/^sanitize_path()/,/^}/p' \
    "${kickstart_script}" > "${functions_script}"

safe_path_output=$(/bin/sh -c '
    . "$1"
    sanitize_path "/usr/local"
' sh "${functions_script}")

[ "${safe_path_output}" = '/usr/local' ] || {
    printf 'unexpected sanitized safe path: %s\n' "${safe_path_output}" >&2
    exit 1
}

unsafe_path_output=$(/bin/sh -c '
    . "$1"
    sanitize_path "/usr/local;tmp" 2>/dev/null
' sh "${functions_script}")

[ "${unsafe_path_output}" = '/usr/local_tmp' ] || {
    printf 'unexpected sanitized unsafe path: %s\n' "${unsafe_path_output}" >&2
    exit 1
}

printf '%s\n' 'kickstart path sanitizer tests: OK'
