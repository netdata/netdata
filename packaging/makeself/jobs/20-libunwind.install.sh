#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version"
# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building libunwind" || true

export CFLAGS="${TUNING_FLAGS} -I/libucontext-static/usr/include -fno-lto -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS="-static -L/libucontext-static/usr/lib/ -lucontext"
export PKG_CONFIG="pkg-config --static"

if [ -d "${NETDATA_MAKESELF_PATH}/tmp/libunwind" ]; then
  rm -rf "${NETDATA_MAKESELF_PATH}/tmp/libunwind"
fi

cache="${NETDATA_SOURCE_PATH}/artifacts/cache/${BUILDARCH}/libunwind"

if [ -d "${cache}" ]; then
  echo "Found cached copy of build directory for libunwind, using it."
  cp -a "${cache}/libunwind" "${NETDATA_MAKESELF_PATH}/tmp/"
  CACHE_HIT=1
else
  echo "No cached copy of build directory for libunwind found, fetching sources instead."
  run git clone "${LIBUNWIND_SOURCE}" "${NETDATA_MAKESELF_PATH}/tmp/libunwind"
  cd "${NETDATA_MAKESELF_PATH}/tmp/libunwind" && run git checkout "${LIBUNWIND_VERSION}"
  CACHE_HIT=0
fi

cd "${NETDATA_MAKESELF_PATH}/tmp/libunwind" || exit 1

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

store_cache libunwind "${NETDATA_MAKESELF_PATH}/tmp/libunwind"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
