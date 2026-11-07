#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
# shellcheck source=./win-build-dir.sh
source "${repo_root}/packaging/windows/win-build-dir.sh"

if [[ "${MSYSTEM:-}" != "UCRT64" ]]; then
    echo "Run this script from an MSYS2 UCRT64 terminal (MSYSTEM=UCRT64)." >&2
    exit 1
fi

required_wix_version="5.0.2"
wix_candidates=()
if [[ -n "${WIX_BIN:-}" ]]; then
    wix_candidates+=("${WIX_BIN}")
else
    if [[ -n "${DOTNET_CLI_HOME:-}" ]]; then
        wix_candidates+=("$(cygpath -u "${DOTNET_CLI_HOME}")/.dotnet/tools/wix.exe")
    fi
    if [[ -n "${USERPROFILE:-}" ]]; then
        wix_candidates+=("$(cygpath -u "${USERPROFILE}")/.dotnet/tools/wix.exe")
    fi
    for command_name in wix.exe wix; do
        command_path="$(command -v "${command_name}" 2>/dev/null || true)"
        if [[ -n "${command_path}" ]]; then
            wix_candidates+=("${command_path}")
        fi
    done
fi

wix=""
for candidate in "${wix_candidates[@]}"; do
    if [[ "${candidate}" =~ ^[A-Za-z]: ]]; then
        candidate="$(cygpath -u "${candidate}")"
    fi
    if [[ ! -x "${candidate}" ]]; then
        if command -v "${candidate}" >/dev/null 2>&1; then
            candidate="$(command -v "${candidate}")"
        else
            continue
        fi
    fi
    version_output="$("${candidate}" --version 2>&1)" || continue
    if grep -Eq "(^|[^0-9.])${required_wix_version//./\\.}([^0-9.]|$)" <<<"${version_output}"; then
        wix="${candidate}"
        break
    fi
done
if [[ -z "${wix}" ]]; then
    echo "WiX ${required_wix_version} was not found. Run install-dependencies.ps1 or set WIX_BIN to that version." >&2
    exit 1
fi
wix_dir="$(cd -- "$(dirname -- "${wix}")" && pwd -P)"
wix="${wix_dir}/$(basename -- "${wix}")"

if [[ "${WIX_ARCH:-x64}" != "x64" ]]; then
    echo "This build produces x64 binaries; WIX_ARCH must be x64." >&2
    exit 1
fi
if [[ ! -f "${build}/netdata.wxs" ]]; then
    echo "No generated ${build}/netdata.wxs found. Run compile-on-windows.sh first." >&2
    exit 1
fi
stage_dir="${build}/stage/opt/netdata"
stage_marker="${build}/.netdata-package-stage-ready"
for component in "${build}/stage" "${build}/stage/opt" "${stage_dir}"; do
    if [[ -L "${component}" ]]; then
        echo "Install stage contains a symlink: ${component}. Run package-windows.sh to recreate it." >&2
        exit 1
    fi
done
if [[ ! -d "${stage_dir}" || ! -f "${stage_marker}" || -L "${stage_marker}" ]]; then
    echo "No completed install stage found at ${stage_dir}. Run package-windows.sh first." >&2
    exit 1
fi

output_msi="${repo_root}/packaging/windows/netdata-x64.msi"
cd "${build}"
"${wix}" build \
    -arch x64 \
    -ext WixToolset.Util.wixext \
    -ext WixToolset.UI.wixext \
    -out "$(cygpath -aw "${output_msi}")" \
    netdata.wxs
echo "Installer created: ${output_msi}"
