#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building OpenSSL" || true

version="$(cat "$(dirname "${0}")/../openssl.version")"

export CFLAGS='-fno-lto -pipe'
export LDFLAGS='-static'
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
  run git clone --branch "${version}" --single-branch --depth 1 https://github.com/openssl/openssl.git "${NETDATA_MAKESELF_PATH}/tmp/openssl"
  CACHE_HIT=0
fi

cd "${NETDATA_MAKESELF_PATH}/tmp/openssl" || exit 1

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run ./config -static no-tests --prefix=/openssl-static --openssldir=/opt/netdata/etc/ssl
    run make -j "$(nproc)"
fi

run make -j "$(nproc)" install_sw

store_cache openssl "${NETDATA_MAKESELF_PATH}/tmp/openssl"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
