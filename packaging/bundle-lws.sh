#!/bin/sh

startdir="${PWD}"

LWS_TARBALL="v$(cat "${1}/packaging/libwebsockets.version").tar.gz"
LWS_BUILD_PATH="${1}/externaldeps/libwebsockets/libwebsockets-$(cat "${1}/packaging/libwebsockets.version")"

mkdir -p "${1}/externaldeps/libwebsockets" || exit 1
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/warmcat/libwebsockets/archive/${LWS_TARBALL}" > "${LWS_TARBALL}" || exit 1
sha256sum -c "${1}/packaging/libwebsockets.checksums" || exit 1
tar -xzf "${LWS_TARBALL}" -C "${1}/externaldeps/libwebsockets" || exit 1
cd "${LWS_BUILD_PATH}" || exit 1
cmake -D LWS_WITH_SOCKS5:boolean=YES . || exit 1
make || exit 1
cd "${startdir}" || exit 1
cp -a "${LWS_BUILD_PATH}/lib/libwebsockets.a" "${1}/externaldeps/libwebsockets" || exit 1
cp -a "${LWS_BUILD_PATH}/include" "${1}/externaldeps/libwebsockets" || exit 1
