#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Checking Go plugin CPU architecture" || true

check_files="${NETDATA_INSTALL_PATH}/bin/netdata ${NETDATA_INSTALL_PATH}/usr/libexec/netdata/plugins.d/go.d.plugin"

case "${BUILDARCH}" in
    aarch64) ELF_MACHINE="AArch64" ;;
    armv6l|armv7l) ELF_MACHINE="ARM" ;;
    ppc64le) ELF_MACHINE="PowerPC64" ;;
    x86_64) ELF_MACHINE="X86-64" ;;
    *)
        echo "Buildarch is not recognized for architecture check."
        exit 1
        ;;
esac

for f in ${check_files}; do
    if [ "$(readelf -h "${f}" | grep 'Machine' | rev | awk '{print $1}' | rev)" != "${ELF_MACHINE}" ]; then
        echo "${f} was built for the wrong architecture"
        exit 1
    fi
done

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
