#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Performing basic runtime checks" || true

dump_log() {
  cat ./netdata.log
}

trap dump_log EXIT

export NETDATA_LIBEXEC_PREFIX="${NETDATA_INSTALL_PATH}/usr/libexec/netdata"
export NETDATA_SKIP_LIBEXEC_PARTS="freeipmi|xenstat|cups"

if [ "$(uname -m)" != "x86_64" ]; then
    export NETDATA_SKIP_LIBEXEC_PARTS="${NETDATA_SKIP_LIBEXEC_PARTS}|ebpf"
fi

"${NETDATA_INSTALL_PATH}/bin/netdata" -D > ./netdata.log 2>&1 &

"${NETDATA_SOURCE_PATH}/packaging/runtime-check.sh" || exit 1

trap - EXIT

run rm -rv "${NETDATA_INSTALL_PATH}/var/lib/netdata" "${NETDATA_INSTALL_PATH}/var/cache/netdata"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
