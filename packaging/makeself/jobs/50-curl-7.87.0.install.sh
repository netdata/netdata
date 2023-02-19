#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

version="7.87.0"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building cURL" || true

fetch "curl-${version}" "https://curl.haxx.se/download/curl-${version}.tar.gz" \
    8a063d664d1c23d35526b87a2bf15514962ffdd8ef7fd40519191b3c23e39548 curl

export CFLAGS="-I/openssl-static/include -pipe"
export LDFLAGS="-static -L/openssl-static/lib"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/openssl-static/lib/pkgconfig"

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

store_cache curl "${NETDATA_MAKESELF_PATH}/tmp/curl-${version}"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/curl
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Preparing build environment" || true
