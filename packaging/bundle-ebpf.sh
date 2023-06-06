#!/bin/sh

SRCDIR="${1}"
PLUGINDIR="${2}"
FORCE="${3}"

EBPF_VERSION="$(cat "${SRCDIR}/packaging/ebpf.version")"
EBPF_TARBALL="netdata-kernel-collector-glibc-${EBPF_VERSION}.tar.xz"

if [ -x "${PLUGINDIR}/ebpf.plugin" ] || [ "${FORCE}" = "force" ]; then
    mkdir -p "${SRCDIR}/tmp/ebpf"
    curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/kernel-collector/releases/download/${EBPF_VERSION}/${EBPF_TARBALL}" > "${EBPF_TARBALL}" || exit 1
    grep "${EBPF_TARBALL}" "${SRCDIR}/packaging/ebpf.checksums" | sha256sum -c - || exit 1
    tar -xvaf "${EBPF_TARBALL}" -C "${SRCDIR}/tmp/ebpf" || exit 1
    if [ ! -d "${PLUGINDIR}/ebpf.d" ];then
        mkdir "${PLUGINDIR}/ebpf.d"
    fi
    # shellcheck disable=SC2046
    cp -r $(find "${SRCDIR}/tmp/ebpf" -mindepth 1 -maxdepth 1) "${PLUGINDIR}/ebpf.d"
fi
