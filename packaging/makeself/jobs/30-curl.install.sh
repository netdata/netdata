#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building curl" || true

cache_key="curl"
build_dir="${CURL_VERSION}"

fetch_git "${build_dir}" "${CURL_SOURCE}" "${CURL_VERSION}" "${cache_key}"

export CFLAGS="${TUNING_FLAGS} -I/openssl-static/include -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS="-static -L/openssl-static/lib64"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/openssl-static/lib64/pkgconfig"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run autoreconf -fi

    run ./configure \
        --prefix="/curl-local" \
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
        --with-ca-bundle=/opt/netdata/etc/ssl/certs/ca-certificates.crt \
        --with-ca-path=/opt/netdata/etc/ssl/certs \
        --without-brotli \
        --disable-dependency-tracking

    # Curl autoconf does not honour the curl_LDFLAGS environment variable
    run sed -i -e "s/LDFLAGS =/LDFLAGS = -all-static/" src/Makefile

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache "${cache_key}" "${build_dir}"

cp /curl-local/bin/curl "${NETDATA_INSTALL_PATH}"/bin/curl
if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/curl
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Preparing build environment" || true
