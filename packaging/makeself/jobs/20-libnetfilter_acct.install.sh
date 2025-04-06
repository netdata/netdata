#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Install the libnetfilter_acct and it's dependency libmnl


# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building libnetfilter_acct" || true

export CFLAGS="${TUNING_FLAGS} -static -I/usr/include/libmnl -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS="-static -L/usr/lib -lmnl"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/usr/lib/pkgconfig"

cache_key="libnetfilter_acct"
build_dir="libnetfilter_acct-${LIBNETFILTER_ACT_VERSION}"

fetch "${build_dir}" "${LIBNETFILTER_ACT_SOURCE}/libnetfilter_acct-${LIBNETFILTER_ACT_VERSION}.tar.bz2" \
    "${LIBNETFILTER_ACT_ARTIFACT_SHA256}" "${cache_key}"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run ./configure \
        --prefix="/libnetfilter-acct-static" \
        --exec-prefix="/libnetfilter-acct-static"

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache "${cache_key}" "${build_dir}"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
