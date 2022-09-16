#!/bin/sh

PROTOBUF_TARBALL="protobuf-cpp-$(cat "${1}/packaging/protobuf.version").tar.gz"
PROTOBUF_BUILD_PATH="${1}/externaldeps/protobuf/protobuf-$(cat "${1}/packaging/protobuf.version")"

mkdir -p "${1}/externaldeps/protobuf" || exit 1
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/protocolbuffers/protobuf/releases/download/v$(cat "${1}/packaging/protobuf.version")/${PROTOBUF_TARBALL}" > "${PROTOBUF_TARBALL}" || exit 1
sha256sum -c "${1}/packaging/protobuf.checksums" || exit 1
tar -xzf "${PROTOBUF_TARBALL}" -C "${1}/externaldeps/protobuf" || exit 1
OLDPWD="${PWD}"
cd "${PROTOBUF_BUILD_PATH}" || exit 1
./configure --disable-shared --without-zlib --disable-dependency-tracking --with-pic || exit 1
make -j "$(nproc)" || exit 1
cd "${OLDPWD}" || exit 1

cp -a "${PROTOBUF_BUILD_PATH}/src" "${1}/externaldeps/protobuf" || exit 1
