#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Install the libmnl and it's dependency libmnl


# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building libmnl" || true

export CFLAGS="${TUNING_FLAGS} -static -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS="-static"
export PKG_CONFIG="pkg-config --static"

fetch "libmnl-${LIBMNL_VERSION}" "${LIBMNL_SOURCE}/libmnl-${LIBMNL_VERSION}.tar.bz2" \
    "${LIBMNL_ARTIFACT_SHA256}" libmnl

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run ./configure \
        --prefix="/libmnl-static" \
        --exec-prefix="/libmnl-static"

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache libmnl "${NETDATA_MAKESELF_PATH}/tmp/libmnl-${LIBMNL_VERSION}"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
