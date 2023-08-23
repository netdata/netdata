#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

version="$(cat "$(dirname "${0}")/../curl.version")"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building cURL" || true

if [ -d "${NETDATA_MAKESELF_PATH}/tmp/curl" ]; then
  rm -rf "${NETDATA_MAKESELF_PATH}/tmp/curl"
fi

cache="${NETDATA_SOURCE_PATH}/artifacts/cache/${BUILDARCH}/curl"

if [ -d "${cache}" ]; then
  echo "Found cached copy of build directory for curl, using it."
  cp -a "${cache}/curl" "${NETDATA_MAKESELF_PATH}/tmp/"
  CACHE_HIT=1
else
  echo "No cached copy of build directory for curl found, fetching sources instead."
  run git clone --branch "${version}" --single-branch --depth 1 'https://github.com/curl/curl.git' "${NETDATA_MAKESELF_PATH}/tmp/curl"
  CACHE_HIT=0
fi

cd "${NETDATA_MAKESELF_PATH}/tmp/curl" || exit 1

export CFLAGS="-I/openssl-static/include -pipe"
export LDFLAGS="-static -L/openssl-static/lib64"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/openssl-static/lib64/pkgconfig"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run autoreconf -fi

    run ./configure \
        --prefix="${NETDATA_INSTALL_PATH}" \
        --enable-optimize \
        --disable-shared \
        --enable-static \
        --enable-http \
        --disable-ldap \
        --disable-ldaps \
        --enable-proxy \
        --disable-dict \
        --disable-telnet \
        --disable-tftp \
        --disable-pop3 \
        --disable-imap \
        --disable-smb \
        --disable-smtp \
        --disable-gopher \
        --enable-ipv6 \
        --enable-cookies \
        --with-ca-fallback \
        --with-openssl \
        --disable-dependency-tracking

    # Curl autoconf does not honour the curl_LDFLAGS environment variable
    run sed -i -e "s/LDFLAGS =/LDFLAGS = -all-static/" src/Makefile

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache curl "${NETDATA_MAKESELF_PATH}/tmp/curl"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/curl
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Preparing build environment" || true
