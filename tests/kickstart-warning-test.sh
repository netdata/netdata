#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Test extracted functions without executing any installer entry point.
# shellcheck disable=SC2034,SC2154
set -eu
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/netdata-warning-test.XXXXXX")
trap 'rm -rf "${test_dir}"' EXIT HUP INT TERM
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root="${script_dir}/.."
TPUT_BGRED='' TPUT_WHITE='' TPUT_BOLD='' TPUT_RESET='' TPUT_BGYELLOW='' TPUT_BLACK=''
NETDATA_SAVE_WARNINGS=1 NETDATA_PROPAGATE_WARNINGS=1 TERM_WIDTH=1
# Shell metacharacters in diagnostics must remain literal.
# shellcheck disable=SC2016
message='100% literal \c \t \141 C:\temp "quotes" $HOME `false`'
expected="
  - ${message}
  - following warning"
for source in kickstart.sh functions.sh netdata-updater.sh netdata-uninstaller.sh; do
    sed -n -e '/^warning()/,/^}/p' -e '/^error()/,/^}/p' -e '/^exit_reason()/,/^}/p' \
        "${root}/packaging/installer/${source}" > "${test_dir}/functions.sh"
    (
        # shellcheck source=/dev/null
        . "${test_dir}/functions.sh"
        NETDATA_WARNINGS='' SAVED_WARNINGS='' script_name=test
        case "${source}" in
            kickstart.sh|functions.sh)
                warning "${message}" 2>/dev/null
                warning 'following warning' 2>/dev/null ;;
            *)
                error "${message}" 2>/dev/null 3>/dev/null
                error 'following warning' 2>/dev/null 3>/dev/null ;;
        esac
        if [ "${source}" = functions.sh ]; then
            [ "${SAVED_WARNINGS}" = "${expected}" ] || exit 1
        else
            [ "${NETDATA_WARNINGS}" = "${expected}" ] || exit 1
        fi
        case "${source}" in
            functions.sh|netdata-updater.sh)
                NETDATA_SCRIPT_STATUS_PATH="${test_dir}/${source}.status"
                exit_reason "${message}" 7
                NETDATA_WARNINGS='' EXIT_REASON='' EXIT_CODE=''
                # shellcheck disable=SC1090
                . "${NETDATA_SCRIPT_STATUS_PATH}"
                [ "${NETDATA_WARNINGS}" = "${expected}" ] || exit 1
                [ "${EXIT_REASON}" = "${message}" ] || exit 1
                [ "${EXIT_CODE}" = 7 ] ;;
        esac
    ) || { printf 'warning preservation failed: %s\n' "${source}" >&2; exit 1; }
done
sed -n '/^deferred_warnings()/,/^}/p' "${root}/packaging/installer/kickstart.sh" > "${test_dir}/render.sh"
# shellcheck source=/dev/null
. "${test_dir}/render.sh"
NETDATA_WARNINGS="${expected}"
deferred_warnings 2> "${test_dir}/actual"
printf 'The following non-fatal warnings or errors were encountered:\n%s\n\n' "${expected}" > "${test_dir}/expected"
cmp "${test_dir}/expected" "${test_dir}/actual"
printf '%s\n' 'installer warning tests: OK'
