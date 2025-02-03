#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version"
# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building OpenSSL" || true

export CFLAGS="${TUNING_FLAGS} -fno-lto -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS="-static"
export PKG_CONFIG="pkg-config --static"

if [ -d "${NETDATA_MAKESELF_PATH}/tmp/openssl" ]; then
  rm -rf "${NETDATA_MAKESELF_PATH}/tmp/openssl"
fi

if [ -d "${NETDATA_MAKESELF_PATH}/tmp/openssl" ]; then
  rm -rf "${NETDATA_MAKESELF_PATH}/tmp/openssl"
fi

cache="${NETDATA_SOURCE_PATH}/artifacts/cache/${BUILDARCH}/openssl"

if [ -d "${cache}" ]; then
  echo "Found cached copy of build directory for openssl, using it."
  cp -a "${cache}/openssl" "${NETDATA_MAKESELF_PATH}/tmp/"
  CACHE_HIT=1
else
  echo "No cached copy of build directory for openssl found, fetching sources instead."
  run git clone --branch "${OPENSSL_VERSION}" --single-branch --depth 1 "${OPENSSL_SOURCE}" "${NETDATA_MAKESELF_PATH}/tmp/openssl"
  CACHE_HIT=0
fi

cd "${NETDATA_MAKESELF_PATH}/tmp/openssl" || exit 1

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
  COMMON_CONFIG="-static threads no-tests --prefix=/openssl-static --openssldir=/opt/netdata/etc/ssl"

  sed -i "s/disable('static', 'pic', 'threads');/disable('static', 'pic');/" Configure

  # shellcheck disable=SC2086
  case "${BUILDARCH}" in
    armv6l|armv7l) run ./config ${COMMON_CONFIG} linux-armv4 ;;
    *) run ./config ${COMMON_CONFIG} ;;
  esac

  run make -j "$(nproc)"
fi

run make -j "$(nproc)" install_sw

if [ -d "/openssl-static/lib" ]; then
  cd "/openssl-static" || exit 1
  ln -s "lib" "lib64" || true
  cd - || exit 1
fi

store_cache openssl "${NETDATA_MAKESELF_PATH}/tmp/openssl"

perl configdata.pm --dump

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
