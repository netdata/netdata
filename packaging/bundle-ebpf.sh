#!/bin/sh

SRCDIR="${1}"
PLUGINDIR="${2}"

EBPF_VERSION="$(cat "${SRCDIR}/packaging/ebpf.version")"
EBPF_TARBALL="netdata-kernel-collector-glibc-${EBPF_VERSION}.tar.xz"

mkdir -p "${SRCDIR}/tmp/ebpf"
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/kernel-collector/releases/download/${EBPF_VERSION}/${EBPF_TARBALL}" > "${EBPF_TARBALL}" || exit 1
grep "${EBPF_TARBALL}" "${SRCDIR}/packaging/ebpf.checksums" | sha256sum -c - || exit 1
tar -xaf "${EBPF_TARBALL}" -C "${SRCDIR}/tmp/ebpf" || exit 1
# shellcheck disable=SC2046
cp -a $(find "${SRCDIR}/tmp/ebpf" -mindepth 1 -maxdepth 1) "${PLUGINDIR}"
