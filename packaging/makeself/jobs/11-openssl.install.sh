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

cache_key="openssl"
build_dir="${OPENSSL_VERSION}"

fetch_git "${build_dir}" "${OPENSSL_SOURCE}" "${OPENSSL_VERSION}" "${cache_key}"

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

store_cache "${cache_key}" "${build_dir}"

perl configdata.pm --dump

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
