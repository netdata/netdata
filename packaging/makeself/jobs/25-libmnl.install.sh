#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Install libmnl

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

cache_key="${LIBMNL_VERSION}"
fetch_git libmnl "${LIBMNL_REPO}" "${LIBMNL_VERSION}" "${cache_key}"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run autoreconf -ivf

    run ./configure \
        --prefix="/libmnl-static" \
        --exec-prefix="/libmnl-static"

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache "${cache_key}" "${NETDATA_MAKESELF_PATH}/tmp/libmnl"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
