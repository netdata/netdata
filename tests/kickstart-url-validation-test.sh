#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -eu

test_dir=$(mktemp -d "${TMPDIR:-/tmp}/netdata-kickstart-url-test.XXXXXX")
trap 'rm -rf "${test_dir}"' EXIT HUP INT TERM

script_dir=$(CDPATH='' cd "$(dirname "$0")" && pwd)
kickstart_script="${script_dir}/../packaging/installer/kickstart.sh"
functions_script="${test_dir}/functions.sh"

sed -n '/^is_valid_url()/,/^}/p' "${kickstart_script}" > "${functions_script}"

if ! /bin/sh -c '
    PATH=/usr/bin:/bin:/usr/sbin:/sbin
    export PATH
    . "$1"
    is_valid_url "$2" "http|https"
' sh "${functions_script}" 'https://app.netdata.cloud'; then
    printf '%s\n' 'kickstart URL validator rejected the default HTTPS claim URL' >&2
    exit 1
fi

if /bin/sh -c '
    PATH=/usr/bin:/bin:/usr/sbin:/sbin
    export PATH
    . "$1"
    is_valid_url "$2" "http|https"
' sh "${functions_script}" 'ftp://app.netdata.cloud'; then
    printf '%s\n' 'kickstart URL validator accepted an unsupported FTP claim URL' >&2
    exit 1
fi

printf '%s\n' 'kickstart URL validation tests: OK'
