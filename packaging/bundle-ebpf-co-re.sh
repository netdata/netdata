#!/bin/sh

SRCDIR="${1}"

CORE_VERSION="$(cat "${SRCDIR}/packaging/ebpf-co-re.version")"
CORE_TARBALL="netdata-ebpf-co-re-glibc-${CORE_VERSION}.tar.xz"
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/ebpf-co-re/releases/download/${CORE_VERSION}/${CORE_TARBALL}" > "${CORE_TARBALL}" || exit 1
grep "${CORE_TARBALL}" "${SRCDIR}/packaging/ebpf-co-re.checksums" | sha256sum -c - || exit 1
tar -xaf "${CORE_TARBALL}" -C "${SRCDIR}/collectors/ebpf.plugin" || exit 1
