#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

prefix="${1}"
src="${2}"
buildarch="${3}"

dump_log() {
    cat /netdata.log
}

export NETDATA_LIBEXEC_PREFIX="${prefix}/usr/libexec/netdata"
export NETDATA_SKIP_LIBEXEC_PARTS="freeipmi|xenstat|cups"

case "${buildarch}" in
    x86_64) ;;
    armv6l) NETDATA_SKIP_LIBEXEC_PARTS="${NETDATA_SKIP_LIBEXEC_PARTS}|ebpf|otel|systemd-journal" ;;
    *) NETDATA_SKIP_LIBEXEC_PARTS="${NETDATA_SKIP_LIBEXEC_PARTS}|ebpf" ;;
esac

ln -s "${prefix}" /opt/netdata

trap dump_log EXIT

"${prefix}/bin/netdata" -D > /netdata.log &
"${src}/packaging/runtime-check.sh"
kill "$(jobs -p)"

trap - EXIT
