#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building ioping" || true

cache_key="ioping-${IOPING_VERSION}"
fetch_git ioping "${IOPING_REPO}" "${IOPING_VERSION}" "${cache_key}"

export CFLAGS="${TUNING_FLAGS} -static -pipe"
export CXXFLAGS="${CFLAGS}"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run make clean
    run make -j "$(nproc)"
fi

run mkdir -p "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/
run install -o root -g root -m 4750 ioping "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/

store_cache "${cache_key}" "${NETDATA_MAKESELF_PATH}/tmp/ioping"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/ioping
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
