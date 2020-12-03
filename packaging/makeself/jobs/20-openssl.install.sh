#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

version="$(cat "$(dirname "${0}")/../openssl.version")"

export LDFLAGS='-static'
export PKG_CONFIG="pkg-config --static"

# Might be bind-mounted
if [ ! -d "${NETDATA_MAKESELF_PATH}/tmp/openssl" ]; then
  run git clone --branch "${version}" --single-branch git://git.openssl.org/openssl.git "${NETDATA_MAKESELF_PATH}/tmp/openssl"
fi
cd "${NETDATA_MAKESELF_PATH}/tmp/openssl" || exit 1

run ./config no-shared no-tests --prefix=/openssl-static --openssldir=/opt/netdata/etc/ssl
run make -j "$(nproc)"
run make -j "$(nproc)" install_sw
