#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

version='1.3'

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building ioping" || true

fetch "ioping-${version}" "https://github.com/koct9i/ioping/archive/v${version}.tar.gz" \
    7aa48e70aaa766bc112dea57ebbe56700626871052380709df3a26f46766e8c8 ioping

export CFLAGS="-static -pipe"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run make clean
    run make -j "$(nproc)"
fi

run mkdir -p "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/
run install -o root -g root -m 4750 ioping "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/

store_cache ioping "${NETDATA_MAKESELF_PATH}/tmp/ioping-${version}"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/ioping
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
