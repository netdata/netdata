#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

# shellcheck source=packaging/windows/win-build-dir.sh
. "${repo_root}/packaging/windows/win-build-dir.sh"

set -eu -o pipefail

wix_arch="${WIX_ARCH:-x64}"
WIX_BIN="${WIX_BIN:-}"

find_cmd() {
    local cmd

    for cmd in "$@"; do
        if command -v "${cmd}" > /dev/null 2>&1; then
            command -v "${cmd}"
            return 0
        fi
    done

    return 1
}

find_wix() {
    local candidate=""

    if candidate="$(find_cmd wix wix.exe)"; then
        printf '%s\n' "${candidate}"
        return 0
    fi

    for candidate in \
        ${HOME:+"${HOME}/.dotnet/tools/wix.exe"} \
        "/c/Program Files/WixToolset v6.0/bin/wix.exe" \
        "/c/Program Files/WiX Toolset v6.0/bin/wix.exe" \
        "/c/Program Files/WixToolset v5.0/bin/wix.exe" \
        "/c/Program Files/WiX Toolset v5.0/bin/wix.exe" \
        "/c/Program Files (x86)/WixToolset v6.0/bin/wix.exe" \
        "/c/Program Files (x86)/WiX Toolset v6.0/bin/wix.exe" \
        "/c/Program Files (x86)/WixToolset v5.0/bin/wix.exe" \
        "/c/Program Files (x86)/WiX Toolset v5.0/bin/wix.exe"
    do
        if [ -f "${candidate}" ]; then
            printf '%s\n' "${candidate}"
            return 0
        fi
    done

    return 1
}

if [ -z "${WIX_BIN}" ]; then
    WIX_BIN="$(find_wix || true)"
fi

# shellcheck disable=SC2154  # build is set by the sourced win-build-dir.sh
if [ -n "${WIX_BIN}" ] && [ -f "${build}/netdata.wxs" ]; then
    ${GITHUB_ACTIONS+echo "::group::MSI"}
    cd "${build}"
    "${WIX_BIN}" build \
        -arch "${wix_arch}" \
        -ext WixToolset.Util.wixext \
        -ext WixToolset.UI.wixext \
        -out "${repo_root}/packaging/windows/netdata-${wix_arch}.msi" \
        "netdata.wxs"
    ${GITHUB_ACTIONS+echo "::endgroup::"}
fi
