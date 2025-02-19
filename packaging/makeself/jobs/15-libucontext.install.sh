#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version"


case "${BUILDARCH}" in
    armv6l|armv7l) ;;
    *) exit 0 ;;
esac

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building libucontext" || true

export CFLAGS="${TUNING_FLAGS} -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS=""

if [ -d "${NETDATA_MAKESELF_PATH}/tmp/libucontext" ]; then
  rm -rf "${NETDATA_MAKESELF_PATH}/tmp/libucontext"
fi

fetch_git libucontext "${LIBUCONTEXT_SOURCE}" "${LIBUCONTEXT_VERSION}" "${LIBUCONTEXT_VERSION}"

case "${BUILDARCH}" in
    armv6l|armv7l) arch=arm ;;
    ppc64le) arch=ppc64 ;;
    *) arch="${BUILDARCH}" ;;
esac

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run make ARCH="${arch}" EXPORT_UNPREFIXED="yes" -j "$(nproc)"
fi

run make ARCH="${arch}" EXPORT_UNPREFIXED="yes" DESTDIR="/libucontext-static" -j "$(nproc)" install

store_cache libucontext "${NETDATA_MAKESELF_PATH}/tmp/libucontext"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
