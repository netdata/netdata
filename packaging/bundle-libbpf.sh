#!/bin/sh

LIBBPF_TARBALL="v$(cat "${1}/packaging/libbpf.version").tar.gz"
LIBBPF_BUILD_PATH="${1}/externaldeps/libbpf/libbpf-$(cat "${1}/packaging/libbpf.version")"

if [ "$(uname -m)" = x86_64 ]; then
    lib_subdir="lib64"
else
    lib_subdir="lib"
fi

mkdir -p "${1}/externaldeps/libbpf" || exit 1
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/libbpf/archive/${LIBBPF_TARBALL}" > "${LIBBPF_TARBALL}" || exit 1
sha256sum -c "${1}/packaging/libbpf.checksums" || exit 1
tar -xzf "${LIBBPF_TARBALL}" -C "${1}/externaldeps/libbpf" || exit 1
make -C "${LIBBPF_BUILD_PATH}/src" BUILD_STATIC_ONLY=1 OBJDIR=build/ DESTDIR=../ install || exit 1
cp -a "${LIBBPF_BUILD_PATH}/usr/${lib_subdir}/libbpf.a" "${1}/externaldeps/libbpf" || exit 1
cp -a "${LIBBPF_BUILD_PATH}/usr/include" "${1}/externaldeps/libbpf" || exit 1
