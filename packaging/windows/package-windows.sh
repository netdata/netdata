#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

# shellcheck source=./win-build-dir.sh
. "${repo_root}/packaging/windows/win-build-dir.sh"

set -eu -o pipefail

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

if ! find_wix > /dev/null; then
    echo "Missing required command: wix. Checked PATH and common WiX installation directories." >&2
    exit 1
fi

# Regenerate keys everytime there is an update
if [ -d /opt/netdata/etc/pki/ ]; then
    rm -rf /opt/netdata/etc/pki/
fi

# Remove previous installation of msys2 script
if [ -f /opt/netdata/usr/bin/bashbug ]; then
    rm -rf /opt/netdata/usr/bin/bashbug
fi

${GITHUB_ACTIONS+echo "::group::Installing"}
cmake --install "${build}"
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ ! -f "/msys2-latest.tar.zst" ]; then
    ${GITHUB_ACTIONS+echo "::group::Fetching MSYS2 files"}
    "${repo_root}/packaging/windows/fetch-msys2-installer.py" /msys2-latest.tar.zst
    ${GITHUB_ACTIONS+echo "::endgroup::"}
fi

${GITHUB_ACTIONS+echo "::group::Licenses"}
if [ ! -f "/gpl-3.0.txt" ]; then
    cp "${repo_root}/LICENSE" /gpl-3.0.txt
fi

if [ ! -f "/cloud.txt" ]; then
    curl -o /cloud.txt "https://app.netdata.cloud/LICENSE.txt"
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Copy Files"}
tar -xf /msys2-latest.tar.zst -C /opt/netdata/ || exit 1
cp -R /opt/netdata/msys64/* /opt/netdata/ || exit 1
cp packaging/windows/copy_files.ps1 /opt/netdata/usr/libexec/netdata/ || exit 1
rm -rf /opt/netdata/msys64/
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Configure Editor"}
if [ -f "/opt/netdata/etc/profile" ]; then
    echo 'EDITOR="/usr/bin/nano.exe"' >> /opt/netdata/etc/profile
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

# TODO: We will have a PR to adjust CAB file creation and sign. This is only adding necessary structure
#${GITHUB_ACTIONS+echo "::group::CAB file"}
#mkdir "${build}/driver"
#cp "${build}/usr/bin/netdata_driver.*" "${build}/driver"
#powershell.exe -ExecutionPolicy Bypass -File "${repo_root}/packaging/windows/generate-driver-catalog.ps1"
#${GITHUB_ACTIONS+echo "::endgroup::"}
