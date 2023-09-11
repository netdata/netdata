#!/bin/bash

if [ "$(uname -m)" = x86_64 ]; then
    lib_subdir="lib64"
else
    lib_subdir="lib"
fi

if [ "${2}" != "centos7" ]; then
  cp "${1}/packaging/current_libbpf.checksums" "${1}/packaging/libbpf.checksums"
  cp "${1}/packaging/current_libbpf.version" "${1}/packaging/libbpf.version"
else
  cp "${1}/packaging/libbpf_0_0_9.checksums" "${1}/packaging/libbpf.checksums"
  cp "${1}/packaging/libbpf_0_0_9.version" "${1}/packaging/libbpf.version"
fi

LIBBPF_TARBALL="v$(cat "${1}/packaging/libbpf.version").tar.gz"
LIBBPF_BUILD_PATH="${1}/externaldeps/libbpf/libbpf-$(cat "${1}/packaging/libbpf.version")"

mkdir -p "${1}/externaldeps/libbpf" || exit 1
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/libbpf/archive/${LIBBPF_TARBALL}" > "${LIBBPF_TARBALL}" || exit 1
sha256sum -c "${1}/packaging/libbpf.checksums" || exit 1
tar -xzf "${LIBBPF_TARBALL}" -C "${1}/externaldeps/libbpf" || exit 1
make -C "${LIBBPF_BUILD_PATH}/src" BUILD_STATIC_ONLY=1 OBJDIR=build/ DESTDIR=../ install || exit 1
cp -r "${LIBBPF_BUILD_PATH}/usr/${lib_subdir}/libbpf.a" "${1}/externaldeps/libbpf" || exit 1
cp -r "${LIBBPF_BUILD_PATH}/usr/include" "${1}/externaldeps/libbpf" || exit 1
cp -r "${LIBBPF_BUILD_PATH}/include/uapi" "${1}/externaldeps/libbpf/include" || exit 1
