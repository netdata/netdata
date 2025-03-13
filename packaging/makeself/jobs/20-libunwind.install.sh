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
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building libunwind" || true

export CFLAGS="${TUNING_FLAGS} -I/libucontext-static/usr/include -fno-lto -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS="-static -L/libucontext-static/usr/lib/ -lucontext"
export PKG_CONFIG="pkg-config --static"

cache_key="libunwind"
build_dir="libunwind-${LIBUNWIND_VERSION}"

fetch_git "${build_dir}" "${LIBUNWIND_SOURCE}" "${LIBUNWIND_VERSION}" "${cache_key}" fetch-via-checkout

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
  run autoreconf -ivf

  run ./configure \
    --prefix=/libunwind-static \
    --build="$(gcc -dumpmachine)" \
    --disable-cxx-exceptions \
    --disable-documentation \
    --disable-tests \
    --disable-shared \
    --enable-static \
    --disable-dependency-tracking

  run make -j "$(nproc)"
fi

run make -j "$(nproc)" install

store_cache "${cache_key}" "${build_dir}"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
